package trazrextension

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/extension"
)

func TestShutdownBeforeStart(t *testing.T) {
	h, err := newTrazrExtension(context.Background(), extension.Settings{}, &Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestConfigEndpointValidation(t *testing.T) {
	for _, endpoint := range []string{"", ":18888", "127.0.0.1:18888", "[::1]:0", "localhost:8080"} {
		if err := (&Config{Endpoint: endpoint}).Validate(); err != nil {
			t.Errorf("valid endpoint %q: %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"localhost", "http://localhost:18888", "127.0.0.1:-1", "127.0.0.1:65536", "127.0.0.1:http"} {
		if err := (&Config{Endpoint: endpoint}).Validate(); err == nil {
			t.Errorf("invalid endpoint %q was accepted", endpoint)
		}
	}
}
