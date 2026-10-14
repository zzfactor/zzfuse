package trazrextension

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/event"
)

func TestStreamUserNotificationsWS_Ready_NoTopics(t *testing.T) {
	upgrader := websocket.Upgrader{}
	// Test server that upgrades and calls our WS streamer with no topics
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()
		bus := event.NewInMemoryEventBus(1024, 4)
		StreamNotificationsWS(bus, conn, r)
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	u.Scheme = "ws"
	q := u.Query()
	// no topics => triggers immediate ready
	u.RawQuery = q.Encode()

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))

	_, msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(msg, &payload); err != nil {
		t.Fatalf("json: %v", err)
	}
	if payload["type"] != "ready" {
		t.Fatalf("expected ready, got %v", payload)
	}
}

