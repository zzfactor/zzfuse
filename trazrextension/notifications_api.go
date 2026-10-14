package trazrextension

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/event"
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/trazrextension/internal"
	"go.uber.org/zap"
)

// NotificationEnqueueRequest mirrors OpenAPI schema NotificationEnqueueRequest
// topic is required; data is arbitrary object; optional ttl_seconds and meta
// Note: We don't persist; ttl is advisory for clients.
type NotificationEnqueueRequest struct {
	Topic      string         `json:"topic"`
	Data       map[string]any `json:"data"`
	TTLSeconds int64          `json:"ttl_seconds,omitempty"`
	Meta       map[string]any `json:"meta,omitempty"`
}

// NewNotificationsEnqueueHandler publishes notifications to the EventBus.
// Auth: Bearer JWT.
func NewNotificationsEnqueueHandler(bus event.EventBusInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}

		// Authenticate: require Bearer JWT
		claimsMap, err := ValidateJWTFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized: "+err.Error(), zap.NewNop())
			return
		}

		var req NotificationEnqueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if strings.TrimSpace(req.Topic) == "" || req.Data == nil {
			writeJSONError(w, http.StatusBadRequest, "topic and data are required", zap.NewNop())
			return
		}

		// Authorization: require notifications:write scope
		hasNotificationsWrite := false
		if v, ok := claimsMap["scopes"]; ok {
			switch s := v.(type) {
			case []interface{}:
				for _, it := range s {
					if sv, ok := it.(string); ok && strings.EqualFold(strings.TrimSpace(sv), "notifications:write") {
						hasNotificationsWrite = true
						break
					}
				}
			case []string:
				for _, sv := range s {
					if strings.EqualFold(strings.TrimSpace(sv), "notifications:write") {
						hasNotificationsWrite = true
						break
					}
				}
			}
		}
		if !hasNotificationsWrite {
			writeJSONError(w, http.StatusForbidden, "forbidden: missing notifications:write scope", zap.NewNop())
			return
		}

		// prepare payload
		id := fmt.Sprintf("evt_%d", time.Now().UnixNano())
		if req.Meta == nil {
			req.Meta = map[string]any{}
		}
		if v, ok := claimsMap["org_id"]; ok {
			req.Meta["org_id"] = v
		}
		if v, ok := claimsMap["project_id"]; ok {
			req.Meta["project_id"] = v
		}
		payload := map[string]any{
			"type":  "notification",
			"topic": sanitizeTopic(req.Topic),
			"data":  req.Data,
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"id":    id,
			"meta":  req.Meta,
		}

		// Publish to EventBus
		_ = bus.Publish(r.Context(), event.Event{
			Type: repository.EventTypeNotificationPublished.ToEventType(),
			Data: payload,
		})

		// TTL advisory
		expires := time.Now().UTC()
		if req.TTLSeconds > 0 {
			expires = expires.Add(time.Duration(req.TTLSeconds) * time.Second)
		} else {
			expires = expires.Add(14 * 24 * time.Hour)
		}
		writeJSON(w, http.StatusOK, NotificationEnqueueResponse{ID: id, Status: "queued", ExpiresAt: expires})
	}
}

type NotificationEnqueueResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewNotificationsEnqueueHandler handles POST /api/v1/user/notifications
// Auth: Bearer JWT.
// NewNotificationsEnqueueHandlerDB handles POST /api/v1/user/notifications using legacy pg_notify.
// Auth: Bearer JWT.
func NewNotificationsEnqueueHandlerDB(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}

		// Authenticate: require Bearer JWT
		claimsMap, err := ValidateJWTFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized: "+err.Error(), zap.NewNop())
			return
		}

		var req NotificationEnqueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if strings.TrimSpace(req.Topic) == "" || req.Data == nil {
			writeJSONError(w, http.StatusBadRequest, "topic and data are required", zap.NewNop())
			return
		}

		// Authorization: require notifications:write scope
		hasNotificationsWrite := false
		if v, ok := claimsMap["scopes"]; ok {
			switch s := v.(type) {
			case []interface{}:
				for _, it := range s {
					if sv, ok := it.(string); ok && strings.EqualFold(strings.TrimSpace(sv), "notifications:write") {
						hasNotificationsWrite = true
						break
					}
				}
			case []string:
				for _, sv := range s {
					if strings.EqualFold(strings.TrimSpace(sv), "notifications:write") {
						hasNotificationsWrite = true
						break
					}
				}
			}
		}
		if !hasNotificationsWrite {
			writeJSONError(w, http.StatusForbidden, "forbidden: missing notifications:write scope", zap.NewNop())
			return
		}

		// prepare payload
		id := fmt.Sprintf("evt_%d", time.Now().UnixNano())
		if req.Meta == nil {
			req.Meta = map[string]any{}
		}
		// propagate org/project scopes if available
		if v, ok := claimsMap["org_id"]; ok {
			req.Meta["org_id"] = v
		}
		if v, ok := claimsMap["project_id"]; ok {
			req.Meta["project_id"] = v
		}
		payload := map[string]any{
			"type":  "notification",
			"topic": req.Topic,
			"data":  req.Data,
			"ts":    time.Now().UTC().Format(time.RFC3339Nano),
			"id":    id,
			"meta":  req.Meta,
		}
		b, _ := json.Marshal(payload)

		channel := fmt.Sprintf("notify.%s", sanitizeTopic(req.Topic))
		// Send via pg_notify using database/sql (legacy path)
		if _, err := db.ExecContext(r.Context(), "SELECT pg_notify($1, $2)", channel, string(b)); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to enqueue notification", zap.NewNop())
			return
		}
		// TTL advisory
		expires := time.Now().UTC()
		if req.TTLSeconds > 0 {
			expires = expires.Add(time.Duration(req.TTLSeconds) * time.Second)
		} else {
			// default 14 days per schema description
			expires = expires.Add(14 * 24 * time.Hour)
		}
		writeJSON(w, http.StatusOK, NotificationEnqueueResponse{ID: id, Status: "queued", ExpiresAt: expires})
	}
}

// sanitizeTopic ensures the topic is safe for use after the 'notify.' prefix.
// it keeps letters, digits, underscore, and dots, replacing other chars with underscore.
func sanitizeTopic(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "general"
	}
	return out
}

// StreamNotificationsWS streams notifications from the EventBus.
func StreamNotificationsWS(bus event.EventBusInterface, conn *websocket.Conn, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// parse topics
	topics := r.URL.Query()["topics"]
	if len(topics) == 0 {
		// send ready and return when connection closes
		_ = internal.SafeWriteJSON(conn, map[string]string{"type": "ready"})
		wsControlDrain(ctx, conn)
		<-ctx.Done()
		return
	}

	// EventBus subscription: one handler filtering by topics
	topicsSet := map[string]struct{}{}
	for _, t := range topics {
		topicsSet[sanitizeTopic(strings.TrimSpace(t))] = struct{}{}
	}
	done := make(chan struct{})
	handler := &notificationEventHandler{
		topics: topicsSet,
		conn:   conn,
		done:   done,
	}
	bus.RegisterHandler(repository.EventTypeNotificationPublished.ToEventType(), handler)
	_ = internal.SafeWriteJSON(conn, map[string]string{"type": "ready"})

	// start reading control frames (pings/pongs) to keep connection alive
	wsControlDrain(ctx, conn)

	// Optional server-initiated pings, similar to other WS wrappers
	// Enable with ?ping=true or ?ping_interval=SECONDS
	if pi := strings.TrimSpace(r.URL.Query().Get("ping")); strings.EqualFold(pi, "true") || r.URL.Query().Get("ping_interval") != "" {
		interval := 30 * time.Second
		if v := strings.TrimSpace(r.URL.Query().Get("ping_interval")); v != "" {
			if sec, err := time.ParseDuration(v + "s"); err == nil && sec > 0 {
				interval = sec
			}
		}
		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					deadline := time.Now().Add(5 * time.Second)
					_ = internal.SafeWriteControl(conn, websocket.PingMessage, nil, deadline)
				}
			}
		}()
	}

	// drain loop: wait for ctx.Done and signal handler to stop
	<-ctx.Done()
	close(done)
}

// notificationEventHandler delivers notification events over the WebSocket.
type notificationEventHandler struct {
	topics map[string]struct{}
	conn   *websocket.Conn
	done   <-chan struct{}
}

func (h *notificationEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}
	// Preferred path: single payload map[string]any
	if m, ok := ev.Data.(map[string]any); ok {
		topic, _ := m["topic"].(string)
		if _, want := h.topics[sanitizeTopic(topic)]; want {
			select {
			case <-h.done:
				return nil
			default:
				_ = internal.SafeWriteJSON(h.conn, m)
			}
		}
		return nil
	}
	// Batch path: []interface{} of maps
	if arr, ok := ev.Data.([]interface{}); ok {
		for _, it := range arr {
			if m, ok := it.(map[string]any); ok {
				topic, _ := m["topic"].(string)
				if _, want := h.topics[sanitizeTopic(topic)]; want {
					select {
					case <-h.done:
						return nil
					default:
						_ = internal.SafeWriteJSON(h.conn, m)
					}
				}
			}
		}
	}
	return nil
}

// wsControlDrain continuously reads messages to ensure Gorilla processes control frames (Ping/Pong).
// It stops when context is done or a read error occurs (e.g., client disconnect).
func wsControlDrain(ctx context.Context, conn *websocket.Conn) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}
