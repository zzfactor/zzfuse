package trazrextension

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestIntegration_LoginRenew(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, user, pass := readTestCreds(t)
    // Health
    code, _ := httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/health", nil, nil, nil)
    if code != 200 {
        t.Fatalf("health check failed: %d", code)
    }
    // Login
    tok := tokenFromLogin(t, base, user, pass)
    // Renew
    var renewResp struct{ Token string `json:"token"` }
    code, data := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/renew", nil, map[string]string{"Authorization": "Bearer " + tok}, &renewResp)
    if code != 200 || renewResp.Token == "" {
        t.Fatalf("renew failed: code=%d body=%s", code, string(data))
    }
}

// Invalid JWT in Authorization should return 401 on whoami.
func TestIntegration_WhoAmI_InvalidJWT_Unauthorized(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, _, _ := readTestCreds(t)
    headers := map[string]string{"Authorization": "Bearer not-a-jwt"}
    code, body := httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/auth/whoami", nil, headers, nil)
    if code != http.StatusUnauthorized {
        t.Fatalf("expected 401 for whoami with invalid jwt, got %d body=%s", code, string(body))
    }
}

// Invalid JWT in Authorization should return 401 on renew.
func TestIntegration_Renew_InvalidJWT_Unauthorized(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, _, _ := readTestCreds(t)
    headers := map[string]string{"Authorization": "Bearer not-a-jwt"}
    code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/renew", nil, headers, nil)
    if code != http.StatusUnauthorized {
        t.Fatalf("expected 401 for renew with invalid jwt, got %d body=%s", code, string(body))
    }
}
// (duplicate LoginRenew body removed)

// Method not allowed on whoami: POST should return 405 when authenticated.
func TestIntegration_WhoAmI_MethodNotAllowed(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, user, pass := readTestCreds(t)
    tok := tokenFromLogin(t, base, user, pass)
    code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/whoami", nil, map[string]string{"Authorization": "Bearer "+tok}, nil)
    if code != http.StatusMethodNotAllowed {
        t.Fatalf("expected 405 for POST whoami, got %d body=%s", code, string(body))
    }
}

// Method not allowed on renew: GET should return 405 when authenticated.
func TestIntegration_Renew_MethodNotAllowed(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, user, pass := readTestCreds(t)
    tok := tokenFromLogin(t, base, user, pass)
    code, body := httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/auth/renew", nil, map[string]string{"Authorization": "Bearer "+tok}, nil)
    if code != http.StatusMethodNotAllowed {
        t.Fatalf("expected 405 for GET renew, got %d body=%s", code, string(body))
    }
}

// GET /api/v1/auth/whoami without Authorization should return 401.
func TestIntegration_WhoAmI_Unauthorized(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, _, _ := readTestCreds(t)
	code, body := httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/auth/whoami", nil, nil, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for whoami without auth, got %d body=%s", code, string(body))
	}
}

// POST /api/v1/auth/renew without Authorization should return 401.
func TestIntegration_Renew_Unauthorized(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, _, _ := readTestCreds(t)
	code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/renew", nil, nil, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for renew without auth, got %d body=%s", code, string(body))
	}
}

// POST /api/v1/auth/login with invalid password should return 401.
func TestIntegration_Login_InvalidCredentials(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, user, _ := readTestCreds(t)
	var out map[string]any
	code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/login", map[string]any{
		"username": user,
		"password": "definitely-wrong-password",
	}, nil, &out)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid login, got %d body=%s", code, string(body))
	}
}

// Method not allowed checks: GET on login/register should return 405.
func TestIntegration_Auth_MethodNotAllowed(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, _, _ := readTestCreds(t)
	// login GET
	code, _ := httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/auth/login", nil, nil, nil)
	if code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET login, got %d", code)
	}
	// register GET
	code, _ = httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/auth/register", nil, nil, nil)
	if code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET register, got %d", code)
	}
}

// Bad payload checks: POST with invalid body should return 400 on register/reset flows.
func TestIntegration_Auth_BadPayloads(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, _, _ := readTestCreds(t)
	// register bad payload (JSON string instead of object)
	code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/register", "oops", nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad register payload, got %d body=%s", code, string(body))
	}
	// request-password-reset bad payload
	code, body = httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/request-password-reset", 12345, nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad request-password-reset payload, got %d body=%s", code, string(body))
	}
	// reset-password bad payload
	code, body = httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/reset-password", true, nil, nil)
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad reset-password payload, got %d body=%s", code, string(body))
	}
}
