package trazrextension

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth"
)

// minimal capture driver to observe Exec calls without external deps
type execCall struct {
	query string
	args  []driver.NamedValue
}

type captureDriver struct{}
type captureConn struct{}

var captureCh chan execCall

func (d *captureDriver) Open(name string) (driver.Conn, error)   { return &captureConn{}, nil }
func (c *captureConn) Prepare(query string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *captureConn) Close() error                              { return nil }
func (c *captureConn) Begin() (driver.Tx, error)                 { return nil, driver.ErrSkip }

// Implement ExecerContext to capture Exec calls
func (c *captureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if captureCh != nil {
		captureCh <- execCall{query: query, args: args}
	}
	return driver.RowsAffected(1), nil
}

// Fallback Exec for older paths
func (c *captureConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	if captureCh != nil {
		// convert to NamedValue
		nvs := make([]driver.NamedValue, len(args))
		for i, v := range args {
			nvs[i] = driver.NamedValue{Ordinal: i + 1, Value: v}
		}
		captureCh <- execCall{query: query, args: nvs}
	}
	return driver.RowsAffected(1), nil
}

func init() {
	sql.Register("capture", &captureDriver{})
}

func newJSONBody(t *testing.T, v any) *bytes.Reader {
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func TestNotificationsEnqueue_Success(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "unit-test-notification-signing-secret")
	// Arrange: capture driver
	captureCh = make(chan execCall, 1)
	db, err := sql.Open("capture", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	handler := NewNotificationsEnqueueHandlerDB(db)

	token, err := auth.GenerateJWTWith(123, "user@example.com", time.Hour, nil, nil, []string{"notifications:write"})
	if err != nil {
		t.Fatalf("failed to generate jwt with scope: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/notifications", newJSONBody(t, map[string]any{
		"topic": "alerts",
		"data":  map[string]any{"title": "hi"},
	}))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Act
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Assert HTTP
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp.Status != "queued" || resp.ID == "" {
		t.Fatalf("unexpected resp: %+v", resp)
	}

	// Assert DB exec
	select {
	case call := <-captureCh:
		if call.query == "" || call.args == nil || len(call.args) < 2 {
			t.Fatalf("unexpected exec call: %+v", call)
		}
		if call.args[0].Value != "notify.alerts" {
			t.Fatalf("expected channel notify.alerts, got %v", call.args[0].Value)
		}
		if _, ok := call.args[1].Value.(string); !ok {
			t.Fatalf("expected payload string, got %T", call.args[1].Value)
		}
	default:
		t.Fatalf("no exec captured")
	}
}

func TestNotificationsEnqueue_Unauthorized_NoToken(t *testing.T) {
	captureCh = make(chan execCall, 1)
	db, err := sql.Open("capture", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	h := NewNotificationsEnqueueHandlerDB(db)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/user/notifications", newJSONBody(t, map[string]any{"topic": "alerts", "data": map[string]any{"x": 1}}))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}
