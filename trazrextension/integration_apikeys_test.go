package trazrextension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestIntegration_APIKey_Enqueue_UserReceives(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}

	base, user, pass := readTestCreds(t)
	// User logs in (will receive notifications)
	userTok := tokenFromLogin(t, base, user, pass)

	// Create API key for this user and exchange it to JWT (client sender)
	plaintext, _ := createAPIKey(t, base, userTok)
	// Debug: list keys and log expiry of latest
	keys := listAPIKeys(t, base, userTok)
	if len(keys) > 0 {
		t.Logf("latest key: name=%v expires_at=%v created_at=%v", keys[0]["name"], keys[0]["expires_at"], keys[0]["created_at"])
	}
	m2mTok := exchangeAPIKey(t, base, plaintext)

	// Connect user's WS listener
	wsBase := wsURLFromHTTP(strings.TrimRight(base, "/"))
	wsURL := fmt.Sprintf("%s/api/v1/stream/user/notifications?topics=alerts&wait_for_established=true&ping_interval=5", wsBase)
	dialer := websocket.Dialer{}
	head := http.Header{}
	head.Set("Authorization", "Bearer "+userTok)
	c, _, err := dialer.Dial(wsURL, head)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	// Expect ready
	if _, msg, err := c.ReadMessage(); err != nil {
		t.Fatalf("ws read ready: %v", err)
	} else {
		var m map[string]any
		_ = json.Unmarshal(msg, &m)
		if m["type"] != "ready" {
			t.Fatalf("expected ready, got %s", string(msg))
		}
	}

	// Enqueue with API key's exchanged JWT
	var enqResp struct{ ID string `json:"id"` }
	code, data := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/user/notifications", map[string]any{
		"topic": "alerts",
		"data":  map[string]any{"title": "Hello via API key", "body": "from integration test"},
	}, map[string]string{"Authorization": "Bearer " + m2mTok}, &enqResp)
	if code != 200 || enqResp.ID == "" {
		t.Fatalf("enqueue (apikey) failed: code=%d body=%s", code, string(data))
	}

	// Receive notification as the user listener
	deadline := time.Now().Add(10 * time.Second)
	match := regexp.MustCompile(`"type"\s*:\s*"notification"`)
	for time.Now().Before(deadline) {
		_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, msg, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("ws read notif: %v", err)
		}
		if match.Match(msg) && strings.Contains(string(msg), "\"topic\":\"alerts\"") {
			return // success
		}
	}
	t.Fatalf("did not receive expected notification before deadline")
}

func TestIntegration_APIKey_Revoke_Then_Exchange_Unauthorized(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, user, pass := readTestCreds(t)
	userTok := tokenFromLogin(t, base, user, pass)
	plaintext, name := createAPIKey(t, base, userTok)
	// find id by name
	keys := listAPIKeys(t, base, userTok)
	var id string
	for _, k := range keys {
		if k["name"] == name {
			if s, ok := k["id"].(string); ok { id = s; break }
		}
	}
	if id == "" {
		t.Fatalf("created key %q not found in list", name)
	}
	// revoke
	status, body := httpJSON(t, http.MethodDelete, strings.TrimRight(base, "/")+"/api/v1/auth/apikeys/"+id, nil, map[string]string{"Authorization": "Bearer "+userTok}, nil)
	if status != http.StatusNoContent {
		t.Fatalf("revoke failed: %d body=%s", status, string(body))
	}
	// exchange should now fail 401
	var resp struct{ Token string `json:"token"` }
	code, data := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/apikeys/exchange", nil, map[string]string{"X-API-Key": plaintext}, &resp)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revoke, got %d body=%s", code, string(data))
	}
}

func TestIntegration_APIKey_Exchange_InvalidKey_Unauthorized(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, _, _ := readTestCreds(t)
	var out map[string]any
	code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/apikeys/exchange", nil, map[string]string{"X-API-Key": "invalid-api-key-value"}, &out)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid api key exchange, got %d body=%s", code, string(body))
	}
}

func TestIntegration_APIKey_Enqueue_MissingScope_Forbidden(t *testing.T) {
	if os.Getenv("TRAZR_REQUIRE_NOTIF_SCOPE") != "1" {
		t.Skip("set TRAZR_REQUIRE_NOTIF_SCOPE=1 when server enforces notifications:write scope")
	}
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, user, pass := readTestCreds(t)
	userTok := tokenFromLogin(t, base, user, pass)
	// Create API key WITHOUT notifications:write scope
	t.Helper()
	name := fmt.Sprintf("it-key-noscope-%d", time.Now().UnixNano())
	var resp struct{ Key string `json:"key"` }
	code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/auth/apikeys", map[string]any{
		"name":   name,
		"scopes": []string{"logs:write"},
	}, map[string]string{"Authorization": "Bearer "+userTok}, &resp)
	if code != http.StatusCreated || resp.Key == "" {
		t.Fatalf("create noscope apikey failed: %d %s", code, string(body))
	}
	m2m := exchangeAPIKey(t, base, resp.Key)
	// Attempt enqueue; expect 403 once enforced
	var enqResp struct{ ID string `json:"id"` }
	code, body = httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/user/notifications", map[string]any{
		"topic": "alerts",
		"data":  map[string]any{"title": "hi", "body": "noscope"},
	}, map[string]string{"Authorization": "Bearer "+m2m}, &enqResp)
	if code != http.StatusForbidden {
		t.Fatalf("expected 403 without notifications:write, got %d body=%s", code, string(body))
	}
	if os.Getenv("TRAZR_IT_CLEANUP") == "1" {
		cleanupAPIKeyByName(t, base, userTok, name)
	}
}
