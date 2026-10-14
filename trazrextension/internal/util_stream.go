package internal

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const wsWriteTimeout = 5 // seconds
const wsMaxConsecutiveWriteErrors = 3

type filterState struct {
	serviceIn       []string
	severityMin     *int
	severityMax     *int
	severityTextIn  []string
	severityGroups  []string
	groupRanges     [][2]int
	q               string
	qMode           string
	qFlags          []string
	compiledPattern *regexp.Regexp
	attrEq          map[string]string
	attrIn          map[string][]string
	attrExists      map[string]bool
	resAttrEq       map[string]string
	resAttrIn       map[string][]string
	resAttrExists   map[string]bool
}

func wsWriteWithTimeout(conn *websocket.Conn, v interface{}, consecutiveErrors *int) error {
	conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout * time.Second))
	if err := SafeWriteJSON(conn, v); err != nil {
		*consecutiveErrors++
		if *consecutiveErrors >= wsMaxConsecutiveWriteErrors {
			_ = SafeWriteJSON(conn, map[string]interface{}{"type": "dropped", "count": *consecutiveErrors})
		}
		return err
	}
	*consecutiveErrors = 0
	return nil
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// wsWriteLocks stores a per-connection mutex to serialize writes
var wsWriteLocks sync.Map // map[*websocket.Conn]*sync.Mutex

// RegisterWSSafe initializes a write lock for the given conn
func RegisterWSSafe(conn *websocket.Conn) {
	wsWriteLocks.Store(conn, &sync.Mutex{})
}

// UnregisterWSSafe removes the write lock for the given conn
func UnregisterWSSafe(conn *websocket.Conn) {
	wsWriteLocks.Delete(conn)
}

func withWSLock(conn *websocket.Conn, fn func() error) error {
	if v, ok := wsWriteLocks.Load(conn); ok {
		mu := v.(*sync.Mutex)
		mu.Lock()
		defer mu.Unlock()
	}
	return fn()
}

// SafeWriteControl serializes control writes (Ping/Pong/Close)
func SafeWriteControl(conn *websocket.Conn, messageType int, data []byte, deadline time.Time) error {
	return withWSLock(conn, func() error {
		return conn.WriteControl(messageType, data, deadline)
	})
}

// SafeWriteJSON serializes JSON writes
func SafeWriteJSON(conn *websocket.Conn, v interface{}) error {
	return withWSLock(conn, func() error {
		return conn.WriteJSON(v)
	})
}

// startWSControlRead launches a lightweight reader that continuously reads (and discards)
// incoming messages so that Gorilla processes control frames (Ping/Pong) on this connection.
// Use this in handlers that otherwise do not read from the WebSocket. The loop exits when
// the context is done or when ReadMessage returns an error (e.g., connection closed).
func startWSControlRead(ctx context.Context, conn *websocket.Conn) {
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

func compileRegex(pattern string, flags []string) *regexp.Regexp {
	if strings.TrimSpace(pattern) == "" {
		return nil
	}
	prefix := ""
	for _, f := range flags {
		if f == "i" {
			prefix += "(?i)"
		}
		if f == "m" {
			prefix += "(?m)"
		}
	}
	re, err := regexp.Compile(prefix + pattern)
	if err != nil {
		return nil
	}
	return re
}

func buildSeverityGroupRanges(groups []string) [][2]int {
	var ranges [][2]int
	for _, g := range groups {
		switch strings.ToUpper(g) {
		case "TRACE":
			ranges = append(ranges, [2]int{1, 4})
		case "DEBUG":
			ranges = append(ranges, [2]int{5, 8})
		case "INFO":
			ranges = append(ranges, [2]int{9, 12})
		case "WARN":
			ranges = append(ranges, [2]int{13, 16})
		case "ERROR":
			ranges = append(ranges, [2]int{17, 20})
		case "FATAL":
			ranges = append(ranges, [2]int{21, 24})
		}
	}
	return ranges
}

func toStringSlice(in []interface{}) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func toStringMap(in map[string]interface{}) map[string]string {
	m := make(map[string]string)
	for k, v := range in {
		if s, ok := v.(string); ok {
			m[k] = s
		} else {
			m[k] = fmt.Sprint(v)
		}
	}
	return m
}

func toCSVMap(in map[string]interface{}) map[string][]string {
	m := make(map[string][]string)
	for k, v := range in {
		switch vv := v.(type) {
		case []interface{}:
			m[k] = toStringSlice(vv)
		case string:
			parts := strings.Split(vv, ",")
			arr := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					arr = append(arr, p)
				}
			}
			m[k] = arr
		default:
			m[k] = []string{fmt.Sprint(v)}
		}
	}
	return m
}

func toBoolMap(in map[string]interface{}) map[string]bool {
	m := make(map[string]bool)
	for k, v := range in {
		if b, ok := v.(bool); ok {
			m[k] = b
		} else if s, ok := v.(string); ok {
			m[k] = strings.EqualFold(s, "true")
		}
	}
	return m
}

// StreamingHandler defines the interface for WebSocket streaming handlers
type StreamingHandler[T any] interface {
	// Subscribe creates a subscription channel for streaming data
	Subscribe(ctx context.Context, filter any) (<-chan T, func(), <-chan struct{}, error)
	// Match determines if an entry matches the given filter parameters
	Match(entry T, params any) bool
}

// PreloadExecutor defines the interface for executing preload queries
type PreloadExecutor[T any] interface {
	// ExecutePreload runs a preload query and sends results to WebSocket
	ExecutePreload(ctx context.Context, commonParams *CommonQueryParams, conn *websocket.Conn) error
}

// StreamingConfig holds configuration for WebSocket streaming
type StreamingConfig struct {
	WaitForEstablished bool
	EnablePreload      bool
	BufferSize         int
	WriteTimeout       time.Duration
	MaxConsecutiveErrs int
}

// DefaultStreamingConfig returns default streaming configuration
func DefaultStreamingConfig() StreamingConfig {
	return StreamingConfig{
		WaitForEstablished: false,
		EnablePreload:      true,
		BufferSize:         1024,
		WriteTimeout:       5 * time.Second,
		MaxConsecutiveErrs: 3,
	}
}

// HandleWebSocketStream provides a generic WebSocket streaming handler
func HandleWebSocketStream[T any](
	handler StreamingHandler[T],
	preloader PreloadExecutor[T],
	filterParams any,
	conn *websocket.Conn,
	r *http.Request,
	config StreamingConfig,
) error {
	ctx := r.Context()
	
	// Parse common query parameters
	commonParams := ParseCommonQueryParams(r, 1000)
	
	// Execute preload if enabled and parameters are present
	if config.EnablePreload && preloader != nil && (commonParams.Cursor != "" || commonParams.Start != nil) {
		if err := preloader.ExecutePreload(ctx, commonParams, conn); err != nil {
			return fmt.Errorf("preload failed: %w", err)
		}
		// Send ready marker after preload
		_ = SafeWriteJSON(conn, map[string]string{"type": "ready"})
	}
	
	// Subscribe to live stream
	ch, cancel, establishedC, err := handler.Subscribe(ctx, filterParams)
	if err != nil {
		return fmt.Errorf("subscribe failed: %w", err)
	}
	defer cancel()
	
	// Wait for established if requested
	if config.WaitForEstablished && establishedC != nil {
		<-establishedC
	}
	
	// Setup connection cleanup
	defer func() {
		_ = SafeWriteControl(conn, websocket.CloseMessage, 
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "stream ended"), 
			time.Now().Add(time.Second))
		conn.Close()
	}()
	
	// Start control message reader
	startWSControlRead(ctx, conn)
	
	// Stream live data
	consecutiveWriteErrors := 0
	for entry := range ch {
		if !handler.Match(entry, filterParams) {
			continue
		}
		
		if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
			return fmt.Errorf("write failed: %w", err)
		}
	}
	
	return nil
}

// SimpleStreamingHandler provides a basic implementation for simple streaming cases
type SimpleStreamingHandler[T any] struct {
	SubscribeFunc func(ctx context.Context, filter any) (<-chan T, func(), <-chan struct{}, error)
	MatchFunc     func(entry T, params any) bool
}

func (h SimpleStreamingHandler[T]) Subscribe(ctx context.Context, filter any) (<-chan T, func(), <-chan struct{}, error) {
	return h.SubscribeFunc(ctx, filter)
}

func (h SimpleStreamingHandler[T]) Match(entry T, params any) bool {
	return h.MatchFunc(entry, params)
}

func atoiPtr(s string) *int {
	if s == "" {
		return nil
	}
	if v, err := strconv.Atoi(s); err == nil {
		return &v
	}
	return nil
}

func equalAttr(v interface{}, want string) bool {
	if v == nil {
		return false
	}
	s := fmt.Sprint(v)
	return s == want
}

func inList(v interface{}, list []string) bool {
	if v == nil {
		return false
	}
	s := fmt.Sprint(v)
	for _, e := range list {
		if s == e {
			return true
		}
	}
	return false
}
