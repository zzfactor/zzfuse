package trazrextension

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	authpkg "github.com/medxops/trazr-otel/pkg/auth"
)

// Expired token should be unauthorized on whoami
func TestIntegration_TokenExpired_WhoAmI_Unauthorized(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, email, _ := readTestCreds(t)
	// Generate a token that expired 1 minute ago
	tok, err := authpkg.GenerateJWTWithUsername(1, email, "", -1*time.Minute)
	if err != nil {
		t.Fatalf("generate expired jwt: %v", err)
	}
	headers := map[string]string{"Authorization": "Bearer " + tok}
	code, body := httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/auth/whoami", nil, headers, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for whoami with expired jwt, got %d body=%s", code, string(body))
	}
}

// Expired token should be unauthorized on renew
func TestIntegration_TokenExpired_Renew_Unauthorized(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, email, _ := readTestCreds(t)
	// Generate a token that expired 1 minute ago
	tok, err := authpkg.GenerateJWTWithUsername(1, email, "", -1*time.Minute)
	if err != nil {
		t.Fatalf("generate expired jwt: %v", err)
	}
	headers := map[string]string{"Authorization": "Bearer " + tok}
	code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/renew", nil, headers, nil)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for renew with expired jwt, got %d body=%s", code, string(body))
	}
}
