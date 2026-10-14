package trazrextension

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/auth"
	"go.uber.org/zap"
)

func TestSignalWebSocketRejectsSchemaBeforeUpgrade(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "unit-test-schema-stream-secret")
	db, mock := sessionTestDatabase(t)
	token := sessionTestToken(t, 42, "")
	for _, schema := range []string{"tenant_b", ""} {
		h := &TrazrExtension{}
		called := false
		handler := h.wsSchemaAuthHandlerWrapper(func(http.ResponseWriter, *http.Request, *websocket.Conn) { called = true })
		req := httptest.NewRequest("GET", "/api/v1/stream/logs?schema="+schema, nil)
		req = req.WithContext(auth.WithSessionDatabase(req.Context(), db))
		expectSessionLookup(mock, 42, []string{"user"}, []string{"tenant_a"})
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		handler(rr, req)
		if rr.Code != http.StatusForbidden || called {
			t.Fatalf("schema %q: status = %d, handler called = %v", schema, rr.Code, called)
		}
		if rr.Header().Get("Upgrade") != "" {
			t.Fatal("denied request upgraded the connection")
		}
	}
}

func TestSignalWebSocketRetainsAuthenticatedContext(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "unit-test-schema-stream-secret")
	db, mock := sessionTestDatabase(t)
	token := sessionTestToken(t, 42, "")
	for i := 0; i < 3; i++ {
		expectSessionLookup(mock, 42, []string{"user"}, []string{"tenant_a"})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &TrazrExtension{logger: zap.NewNop(), cfg: &Config{}, wsUpgrader: newWSUpgrader(nil), shutdownCtx: ctx}
	users := make(chan *auth.UserContext, 1)
	server := httptest.NewServer(auth.SessionUserContextMiddleware(db, h.wsScopedAuthHandlerWrapper(func(_ http.ResponseWriter, r *http.Request, _ *websocket.Conn) {
		user, err := auth.GetUserContext(r.Context())
		if err == nil {
			users <- user
		}
	})))
	defer server.Close()
	headers := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/?schema=tenant_a", headers)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	select {
	case user := <-users:
		if user.UserID != 42 || len(user.Schemas) != 1 || user.Schemas[0] != "tenant_a" {
			t.Fatalf("unexpected authenticated context: %#v", user)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("authenticated user context did not reach stream handler")
	}
}

func TestSignalWebSocketClosesAfterGrantRevocation(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "unit-test-schema-stream-secret")
	db, mock := sessionTestDatabase(t)
	token := sessionTestToken(t, 42, "")
	for i := 0; i < 3; i++ {
		expectSessionLookup(mock, 42, []string{"user"}, []string{"tenant_a"})
	}
	expectSessionLookup(mock, 42, []string{"user"}, []string{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &TrazrExtension{logger: zap.NewNop(), cfg: &Config{}, wsUpgrader: newWSUpgrader(nil), shutdownCtx: ctx}
	wrapped := h.wsSchemaAuthHandlerWrapper(func(_ http.ResponseWriter, r *http.Request, _ *websocket.Conn) {
		<-r.Context().Done()
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped(w, r.WithContext(auth.WithSessionDatabase(r.Context(), db)))
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/?schema=tenant_a",
		http.Header{"Authorization": []string{"Bearer " + token}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("revoked subscription stayed active or closed incorrectly: %v", err)
	}
}
