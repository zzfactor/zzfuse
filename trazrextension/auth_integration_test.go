//go:build integration
// +build integration

package trazrextension

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
)

// getJWTFromEnv returns a Bearer-formatted token from TEST_JWT
func getJWTFromEnv(t *testing.T) string {
	t.Helper()
	tok := os.Getenv("TEST_JWT")
	if tok == "" {
		t.Fatalf("TEST_JWT not set; provide a valid JWT to run auth endpoint tests")
	}
	if !strings.HasPrefix(tok, "Bearer ") {
		tok = "Bearer " + tok
	}
	return tok
}

func getAPIURL() string {
	if v := os.Getenv("TEST_API_BASE_URL"); v != "" {
		return v
	}
	return ""
}

func TestAuthEndpoints_WhoAmI(t *testing.T) {
	requireExtensionIntegration(t)
	api := getAPIURL()
	jwt := getJWTFromEnv(t)

	req, _ := http.NewRequest(http.MethodGet, api+"/api/v1/auth/whoami", nil)
	req.Header.Set("Authorization", jwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Either we have user+roles or claims
	if _, ok := body["user"]; ok {
		if _, ok := body["roles"]; !ok {
			t.Fatalf("expected roles with user in whoami response: %v", body)
		}
	} else if _, ok := body["claims"]; !ok {
		t.Fatalf("expected user+roles or claims in whoami response: %v", body)
	}
}

func TestAuthEndpoints_Renew(t *testing.T) {
	requireExtensionIntegration(t)
	api := getAPIURL()
	jwt := getJWTFromEnv(t)

	req, _ := http.NewRequest(http.MethodPost, api+"/api/v1/auth/renew", nil)
	req.Header.Set("Authorization", jwt)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Token == "" {
		t.Fatalf("empty token returned from renew")
	}
}
