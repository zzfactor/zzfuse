package trazrextension

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository"
	"go.uber.org/zap"
)

type shutdownOrderManager struct {
	repository.ServiceResourceManagerInterface
	closed chan struct{}
}

func (m *shutdownOrderManager) Shutdown() error {
	close(m.closed)
	return nil
}

func TestShutdownDrainsHTTPBeforeClosingDatabase(t *testing.T) {
	started, release, draining := make(chan struct{}), make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)
	server.Config.RegisterOnShutdown(func() { close(draining) })
	requestDone := make(chan error, 1)
	go func() {
		response, err := server.Client().Get(server.URL)
		if err == nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		requestDone <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	select {
	case <-started:
	case <-ctx.Done():
		close(release)
		t.Fatal("request did not start")
	}
	manager := &shutdownOrderManager{closed: make(chan struct{})}
	ext := &TrazrExtension{logger: zap.NewNop(), server: server.Config, rsMgr: manager}
	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- ext.Shutdown(ctx) }()
	select {
	case <-draining:
	case <-ctx.Done():
		close(release)
		t.Fatal("HTTP shutdown did not start")
	}
	select {
	case <-manager.closed:
		close(release)
		t.Fatal("database closed while an HTTP request was active")
	default:
	}
	close(release)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
	if err := <-shutdownDone; err != nil {
		t.Fatal(err)
	}
	select {
	case <-manager.closed:
	default:
		t.Fatal("database was not closed after HTTP drained")
	}
}
