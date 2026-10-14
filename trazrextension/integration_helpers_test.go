package trazrextension

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func requireExtensionIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run external integration tests")
	}
	if os.Getenv("TEST_API_BASE_URL") == "" {
		t.Fatal("set TEST_API_BASE_URL to a dedicated test server")
	}
}

// readTestCreds requires explicit credentials for dedicated integration infrastructure.
func readTestCreds(t *testing.T) (string, string, string) {
	t.Helper()
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run external integration tests")
	}
	endpoint := os.Getenv("TEST_API_BASE_URL")
	username := os.Getenv("TEST_USERNAME")
	password := os.Getenv("TEST_PASSWORD")
	if endpoint == "" || username == "" || password == "" {
		t.Fatal("set TEST_API_BASE_URL, TEST_USERNAME, and TEST_PASSWORD for integration tests")
	}
	return endpoint, username, password
}

// createAPIKey creates an API key for the logged-in user and returns the plaintext key and name.
func createAPIKey(t *testing.T, baseURL, userToken string) (string, string) {
	t.Helper()
	var resp struct {
		Key    string         `json:"key"`
		APIKey map[string]any `json:"api_key"`
	}
	name := fmt.Sprintf("it-key-%d", time.Now().UnixNano())
	code, data := httpJSON(t, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/auth/apikeys", map[string]any{
		"name":   name,
		"scopes": []string{"notifications:write"},
	}, map[string]string{"Authorization": "Bearer " + userToken}, &resp)
	if code != http.StatusCreated || strings.TrimSpace(resp.Key) == "" {
		t.Fatalf("create apikey failed: code=%d body=%s", code, string(data))
	}
	return resp.Key, name
}

// exchangeAPIKey exchanges a plaintext API key for a short-lived JWT.
func exchangeAPIKey(t *testing.T, baseURL, plaintext string) string {
	t.Helper()
	var resp struct {
		Token string `json:"token"`
	}
	code, data := httpJSON(t, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/auth/apikeys/exchange", nil, map[string]string{"X-API-Key": plaintext}, &resp)
	if code != http.StatusOK || strings.TrimSpace(resp.Token) == "" {
		t.Fatalf("exchange apikey failed: code=%d body=%s", code, string(data))
	}
	return resp.Token
}

// listAPIKeys fetches user's API keys for debugging (no plaintext).
func listAPIKeys(t *testing.T, baseURL, userToken string) []map[string]any {
	t.Helper()
	var resp struct {
		Keys []map[string]any `json:"keys"`
	}
	code, data := httpJSON(t, http.MethodGet, strings.TrimRight(baseURL, "/")+"/api/v1/auth/apikeys", nil, map[string]string{"Authorization": "Bearer " + userToken}, &resp)
	if code != http.StatusOK {
		t.Logf("list apikeys failed: code=%d body=%s", code, string(data))
	}
	return resp.Keys
}

// cleanupAPIKeyByName looks up an API key by name and attempts to revoke it. Non-204 results are logged but ignored.
func cleanupAPIKeyByName(t *testing.T, baseURL, userToken, name string) {
	t.Helper()
	keys := listAPIKeys(t, baseURL, userToken)
	var id string
	for _, k := range keys {
		if k["name"] == name {
			if s, ok := k["id"].(string); ok {
				id = s
				break
			}
		}
	}
	if id == "" {
		t.Logf("cleanup: key with name %q not found", name)
		return
	}
	status, body := httpJSON(t, http.MethodDelete, strings.TrimRight(baseURL, "/")+"/api/v1/auth/apikeys/"+id, nil, map[string]string{"Authorization": "Bearer " + userToken}, nil)
	if status != http.StatusNoContent {
		t.Logf("cleanup: revoke key id=%s returned %d body=%s (route may not be enabled)", id, status, string(body))
	}
}

func httpJSON(t *testing.T, method, url string, body any, headers map[string]string, out any) (int, []byte) {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = strings.NewReader(string(b))
	} else {
		reader = strings.NewReader("")
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	data, _ := ioReadAllLimit(resp.Body, 1<<20)
	if out != nil {
		_ = json.Unmarshal(data, out)
	}
	return resp.StatusCode, data
}

func ioReadAllLimit(r io.Reader, limit int64) ([]byte, error) {
	var b strings.Builder
	buf := make([]byte, 4096)
	var n int
	var err error
	var total int64
	for {
		n, err = r.Read(buf)
		if n > 0 {
			if total+int64(n) > limit {
				return []byte(b.String()), errors.New("limit exceeded")
			}
			b.Write(buf[:n])
			total += int64(n)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return []byte(b.String()), nil
			}
			return []byte(b.String()), err
		}
	}
}

// tokenFromLogin performs username/password login and returns token.
func tokenFromLogin(t *testing.T, baseURL, username, password string) string {
	t.Helper()
	var resp struct {
		Token string `json:"token"`
	}
	code, data := httpJSON(t, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api/v1/auth/login", map[string]string{
		"username": username,
		"password": password,
	}, nil, &resp)
	if code < 200 || code >= 300 || resp.Token == "" {
		t.Fatalf("login failed: code=%d body=%s", code, string(data))
	}
	return resp.Token
}

func wsURLFromHTTP(httpURL string) string {
	u, _ := url.Parse(httpURL)
	u.Scheme = map[string]string{"http": "ws", "https": "wss"}[u.Scheme]
	return u.String()
}
