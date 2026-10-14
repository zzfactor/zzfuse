package internal

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSRateLimiter provides per-connection rate limiting for WebSocket messages
type WSRateLimiter struct {
	mu               sync.RWMutex
	connections      map[*websocket.Conn]*connectionState
	maxMsgsPerSecond int
	windowSize       time.Duration
	burstSize        int
}

type connectionState struct {
	mu           sync.Mutex
	messageCount int
	windowStart  time.Time
	burstTokens  int
	lastRefill   time.Time
}

// DefaultWSRateLimiter configuration constants
const (
	DefaultMaxMsgsPerSecond = 100    // Max sustained messages per second
	DefaultWindowSize       = time.Second
	DefaultBurstSize        = 50     // Allow bursts up to 50 messages
)

// Global rate limiter instance
var globalWSRateLimiter *WSRateLimiter
var rateLimiterOnce sync.Once

// GetWSRateLimiter returns the global WebSocket rate limiter
func GetWSRateLimiter() *WSRateLimiter {
	rateLimiterOnce.Do(func() {
		globalWSRateLimiter = NewWSRateLimiter(DefaultMaxMsgsPerSecond, DefaultWindowSize, DefaultBurstSize)
	})
	return globalWSRateLimiter
}

// NewWSRateLimiter creates a new rate limiter with specified limits
func NewWSRateLimiter(maxMsgsPerSecond int, windowSize time.Duration, burstSize int) *WSRateLimiter {
	return &WSRateLimiter{
		connections:      make(map[*websocket.Conn]*connectionState),
		maxMsgsPerSecond: maxMsgsPerSecond,
		windowSize:       windowSize,
		burstSize:        burstSize,
	}
}

// RegisterConnection adds a connection to the rate limiter
func (rl *WSRateLimiter) RegisterConnection(conn *websocket.Conn) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.connections[conn] = &connectionState{
		windowStart: time.Now(),
		burstTokens: rl.burstSize,
		lastRefill:  time.Now(),
	}
}

// UnregisterConnection removes a connection from the rate limiter
func (rl *WSRateLimiter) UnregisterConnection(conn *websocket.Conn) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.connections, conn)
}

// AllowMessage checks if a message is allowed for this connection
// Returns true if allowed, false if rate limited
func (rl *WSRateLimiter) AllowMessage(conn *websocket.Conn) bool {
	rl.mu.RLock()
	state, exists := rl.connections[conn]
	rl.mu.RUnlock()

	if !exists {
		// Connection not registered, allow (but should register first)
		return true
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	now := time.Now()

	// Refill burst tokens based on time elapsed
	elapsed := now.Sub(state.lastRefill)
	tokensToAdd := int(elapsed.Seconds() * float64(rl.maxMsgsPerSecond))
	if tokensToAdd > 0 {
		state.burstTokens += tokensToAdd
		if state.burstTokens > rl.burstSize {
			state.burstTokens = rl.burstSize
		}
		state.lastRefill = now
	}

	// Check if within current window
	if now.Sub(state.windowStart) >= rl.windowSize {
		// Reset window
		state.windowStart = now
		state.messageCount = 0
	}

	// Check rate limit
	if state.messageCount >= rl.maxMsgsPerSecond {
		// Check if we have burst tokens available
		if state.burstTokens > 0 {
			state.burstTokens--
			state.messageCount++
			return true
		}
		return false
	}

	state.messageCount++
	return true
}

// GetConnectionStats returns rate limiting stats for a connection
func (rl *WSRateLimiter) GetConnectionStats(conn *websocket.Conn) (messageCount int, burstRemaining int, ok bool) {
	rl.mu.RLock()
	state, exists := rl.connections[conn]
	rl.mu.RUnlock()

	if !exists {
		return 0, 0, false
	}

	state.mu.Lock()
	defer state.mu.Unlock()

	return state.messageCount, state.burstTokens, true
}

// GetAllStats returns stats for all connections
func (rl *WSRateLimiter) GetAllStats() map[*websocket.Conn]map[string]int {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	stats := make(map[*websocket.Conn]map[string]int)
	for conn, state := range rl.connections {
		state.mu.Lock()
		stats[conn] = map[string]int{
			"message_count":   state.messageCount,
			"burst_remaining": state.burstTokens,
		}
		state.mu.Unlock()
	}

	return stats
}

// RateLimitedWriteJSON writes JSON with rate limiting
// Returns error if rate limited or write fails
func RateLimitedWriteJSON(conn *websocket.Conn, v interface{}) error {
	rl := GetWSRateLimiter()

	if !rl.AllowMessage(conn) {
		// Send rate limit notification
		_ = SafeWriteJSON(conn, map[string]interface{}{
			"type":    "error",
			"code":    "RATE_LIMITED",
			"message": "Too many messages, please slow down",
		})
		return ErrRateLimited
	}

	return SafeWriteJSON(conn, v)
}

// ErrRateLimited is returned when a message is rate limited
var ErrRateLimited = &RateLimitError{message: "rate limit exceeded"}

// RateLimitError represents a rate limiting error
type RateLimitError struct {
	message string
}

func (e *RateLimitError) Error() string {
	return e.message
}

// IsRateLimitError checks if an error is a rate limit error
func IsRateLimitError(err error) bool {
	_, ok := err.(*RateLimitError)
	return ok
}
