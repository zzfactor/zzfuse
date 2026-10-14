package trazrextension

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var resetGuardSequence atomic.Int64

type resetGuardDriver struct{ calls *atomic.Int64 }

func (d resetGuardDriver) Open(string) (driver.Conn, error) {
	d.calls.Add(1)
	return nil, errors.New("password reset must not access the database without private delivery")
}

func resetGuardDatabase(t *testing.T) (*sql.DB, *atomic.Int64) {
	t.Helper()
	calls := new(atomic.Int64)
	name := fmt.Sprintf("reset-guard-%d", resetGuardSequence.Add(1))
	sql.Register(name, resetGuardDriver{calls: calls})
	db, err := sql.Open(name, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, calls
}

func TestRequestPasswordResetRequiresPrivateDelivery(t *testing.T) {
	t.Setenv("TRAZR_SMTP_HOST", "")
	db, calls := resetGuardDatabase(t)
	handler := NewRequestPasswordResetHandler(db)
	var previousResponse string
	for _, email := range []string{"existing@example.com", "missing@example.com"} {
		t.Run(email, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			body := fmt.Sprintf(`{"email":%q}`, email)
			handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/request-password-reset", strings.NewReader(body)))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503; response: %s", recorder.Code, recorder.Body.String())
			}
			var response map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if _, ok := response["reset_token"]; ok {
				t.Fatal("public response disclosed a password reset token")
			}
			if response["error"] != "password reset is unavailable until secure email delivery is configured" {
				t.Fatalf("unexpected response: %v", response)
			}
			if previousResponse != "" && previousResponse != recorder.Body.String() {
				t.Fatal("response must not depend on whether the account exists")
			}
			previousResponse = recorder.Body.String()
		})
	}
	if calls.Load() != 0 {
		t.Fatalf("database accessed %d times; no token or account lookup should occur", calls.Load())
	}
}

func TestRequestPasswordResetInvalidInput(t *testing.T) {
	t.Setenv("TRAZR_SMTP_HOST", "")
	for _, tc := range []struct {
		name, method, body string
		status             int
	}{
		{"wrong method", http.MethodGet, `{"email":"person@example.com"}`, http.StatusMethodNotAllowed},
		{"malformed JSON", http.MethodPost, `{`, http.StatusBadRequest},
		{"missing email", http.MethodPost, `{}`, http.StatusBadRequest},
		{"blank email", http.MethodPost, `{"email":"   "}`, http.StatusBadRequest},
		{"no database configured", http.MethodPost, `{"email":"person@example.com"}`, http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			NewRequestPasswordResetHandler(nil)(recorder, httptest.NewRequest(tc.method, "/api/v1/auth/request-password-reset", strings.NewReader(tc.body)))
			if recorder.Code != tc.status {
				t.Fatalf("status = %d, want %d; response: %s", recorder.Code, tc.status, recorder.Body.String())
			}
		})
	}
}

func TestResetPasswordRejectsInvalidInputBeforeDatabaseAccess(t *testing.T) {
	db, calls := resetGuardDatabase(t)
	for _, body := range []string{
		`{`,
		`{}`,
		`{"token":"","new_password":"Password123"}`,
		`{"token":"   ","new_password":"Password123"}`,
		`{"token":"token","new_password":"short"}`,
	} {
		recorder := httptest.NewRecorder()
		NewResetPasswordHandler(db)(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset-password", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, recorder.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("database accessed %d times for invalid requests", calls.Load())
	}
}

func configureResetTestSMTP(t *testing.T) {
	t.Helper()
	t.Setenv("TRAZR_SMTP_HOST", "127.0.0.1")
	t.Setenv("TRAZR_SMTP_PORT", "1025")
	t.Setenv("TRAZR_SMTP_FROM", "noreply@example.test")
	t.Setenv("TRAZR_SMTP_TLS_MODE", "plain")
	t.Setenv("TRAZR_SMTP_USERNAME", "")
	t.Setenv("TRAZR_SMTP_PASSWORD", "")
	t.Setenv("TRAZR_SMTP_CA_FILE", "")
	t.Setenv("TRAZR_SMTP_TIMEOUT", "1s")
	t.Setenv("TRAZR_PASSWORD_RESET_URL", "http://localhost:3000/reset-password")
}
func TestRequestPasswordResetConfiguredFailureIsPrivate(t *testing.T) {
	configureResetTestSMTP(t)
	db, calls := resetGuardDatabase(t)
	handler := NewRequestPasswordResetHandler(db)
	var previous string
	for _, email := range []string{"existing@example.test", "missing@example.test"} {
		recorder := httptest.NewRecorder()
		handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/request-password-reset", strings.NewReader(fmt.Sprintf(`{"email":%q}`, email))))
		if recorder.Code != http.StatusOK {
			t.Fatal("configured reset disclosed storage failure through status")
		}
		var body map[string]interface{}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 || body["status"] != "if the account exists, a password reset email will be sent" || strings.Contains(recorder.Body.String(), email) || strings.Contains(recorder.Body.String(), "token") {
			t.Fatal("reset response disclosed account, token, or internal failure")
		}
		if previous != "" && previous != recorder.Body.String() {
			t.Fatal("reset response depended on account")
		}
		previous = recorder.Body.String()
	}
	deadline := time.Now().Add(time.Second)
	for calls.Load() != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatal("configured handler did not use asynchronous reset flow")
	}
}
func TestRequestPasswordResetMalformedConfigurationDoesNotIssue(t *testing.T) {
	configureResetTestSMTP(t)
	t.Setenv("TRAZR_PASSWORD_RESET_URL", "http://external.example/reset-password")
	db, calls := resetGuardDatabase(t)
	handler := NewRequestPasswordResetHandler(db)
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/request-password-reset", strings.NewReader(`{"email":"person@example.test"}`)))
	if recorder.Code != http.StatusServiceUnavailable || calls.Load() != 0 {
		t.Fatal("unsafe delivery configuration issued or looked up token/account")
	}
}

func TestRequestPasswordResetDoesNotWaitForAccountOrSMTP(t *testing.T) {
	// A slow private worker must not hold the public response for either account.
	submitted := make(chan string, 2)
	handler := newPasswordResetRequestHandler(func(email string) bool { submitted <- email; return true })
	for _, email := range []string{"existing@example.test", "missing@example.test"} {
		recorder := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			handler(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/request-password-reset", strings.NewReader(fmt.Sprintf(`{"email":%q}`, email))))
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("public request waited for private delivery")
		}
		if recorder.Code != http.StatusOK || <-submitted != email {
			t.Fatal("public submission contract incorrect")
		}
	}
	full := newPasswordResetRequestHandler(func(string) bool { return false })
	recorder := httptest.NewRecorder()
	full(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/auth/request-password-reset", strings.NewReader(`{"email":"person@example.test"}`)))
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "token") {
		t.Fatal("queue saturation exposed account/availability details")
	}
}
