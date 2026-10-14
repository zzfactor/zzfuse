package trazrextension

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// E2E: login issues a short-lived token; near expiry we can renew successfully.
// This test REQUIRES the server to be started with a very small login JWT TTL
// (e.g., TRAZR_JWT_TTL set low) and exports TRAZR_IT_LOGIN_TTL_MS to tell the test
// what TTL to expect. Otherwise it will skip.
func TestIntegration_LoginShortTTL_Renew_Succeeds(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	ttlStr := os.Getenv("TRAZR_IT_LOGIN_TTL_MS")
	if strings.TrimSpace(ttlStr) == "" {
		t.Skip("skipping: set TRAZR_IT_LOGIN_TTL_MS to a small value (e.g. 2000) matching server login TTL")
	}
	ttlMs, err := strconv.Atoi(ttlStr)
	if err != nil || ttlMs <= 0 {
		t.Skipf("skipping: invalid TRAZR_IT_LOGIN_TTL_MS=%q", ttlStr)
	}

	base, user, pass := readTestCreds(t)
	// Issue short-lived token via normal login
	tok := tokenFromLogin(t, base, user, pass)

	// Wait until near-expiry but not expired. Keep a safety margin of 300ms.
	wait := time.Duration(ttlMs)*time.Millisecond - 300*time.Millisecond
	if wait < 100*time.Millisecond {
		wait = 100 * time.Millisecond
	}
	t.Logf("waiting %s before renewing (ttl=%dms)", wait, ttlMs)
	time.Sleep(wait)

	// Attempt renew — should succeed and return a fresh token
	var renewResp struct{ Token string `json:"token"` }
	code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/renew", nil, map[string]string{"Authorization": "Bearer " + tok}, &renewResp)
	if code != http.StatusOK || strings.TrimSpace(renewResp.Token) == "" {
		t.Fatalf("expected renew success for near-expiry login token, got code=%d body=%s", code, string(body))
	}

	// Sanity: call whoami with new token
	code, body = httpJSON(t, http.MethodGet, strings.TrimRight(base, "/")+"/api/v1/auth/whoami", nil, map[string]string{"Authorization": "Bearer " + renewResp.Token}, nil)
	if code != http.StatusOK {
		t.Fatalf("whoami with renewed token failed: code=%d body=%s", code, string(body))
	}
}
