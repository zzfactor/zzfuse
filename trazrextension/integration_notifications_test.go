package trazrextension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Verifies that an authenticated WS connection receives the ready event.
func TestIntegration_Notifications_WS_Ready(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, user, pass := readTestCreds(t)
    tok := tokenFromLogin(t, base, user, pass)

    wsBase := wsURLFromHTTP(strings.TrimRight(base, "/"))
    wsURL := fmt.Sprintf("%s/api/v1/stream/user/notifications?topics=alerts&wait_for_established=true&ping_interval=5", wsBase)
    dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
    head := http.Header{}
    head.Set("Authorization", "Bearer "+tok)
    c, _, err := dialer.Dial(wsURL, head)
    if err != nil {
        t.Fatalf("ws dial: %v", err)
    }
    defer c.Close()
    _ = c.SetReadDeadline(time.Now().Add(10 * time.Second))

    _, msg, err := c.ReadMessage()
    if err != nil {
        t.Fatalf("ws read ready: %v", err)
    }
    var m map[string]any
    _ = json.Unmarshal(msg, &m)
    if m["type"] != "ready" {
        t.Fatalf("expected ready, got %s", string(msg))
    }
}

// Verifies that Authorization is required for WS connection.
func TestIntegration_Notifications_WS_AuthRequired(t *testing.T) {
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run integration tests against live server")
	}
	base, _, _ := readTestCreds(t)
	wsBase := wsURLFromHTTP(strings.TrimRight(base, "/"))
	wsURL := fmt.Sprintf("%s/api/v1/stream/user/notifications?topics=alerts", wsBase)
	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	c, resp, err := dialer.Dial(wsURL, nil)
	if c != nil {
		c.Close()
	}
	if err == nil {
		t.Fatalf("expected ws auth failure, got nil error and status=%v", resp)
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized, got resp=%v err=%v", resp, err)
	}
}

// POST /api/v1/user/notifications requires Authorization; without it returns 401.
func TestIntegration_Notifications_Enqueue_AuthRequired(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, _, _ := readTestCreds(t)
    var out map[string]any
    code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/user/notifications", map[string]any{
        "topic": "alerts",
        "data": map[string]any{"msg": "x"},
    }, nil, &out)
    if code != http.StatusUnauthorized {
        t.Fatalf("expected 401 without auth, got %d body=%s", code, string(body))
    }
}

// With Authorization but invalid payload (missing topic/data) returns 400.
func TestIntegration_Notifications_Enqueue_BadPayload(t *testing.T) {
    if os.Getenv("TRAZR_IT") != "1" {
        t.Skip("set TRAZR_IT=1 to run integration tests against live server")
    }
    base, user, pass := readTestCreds(t)
    tok := tokenFromLogin(t, base, user, pass)
    var out map[string]any
    // missing topic and data
    code, body := httpJSON(t, http.MethodPost, strings.TrimRight(base, "/")+"/api/v1/user/notifications", map[string]any{}, map[string]string{"Authorization": "Bearer "+tok}, &out)
    if code != http.StatusBadRequest {
        t.Fatalf("expected 400 for bad payload, got %d body=%s", code, string(body))
    }
}
