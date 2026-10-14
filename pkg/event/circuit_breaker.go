package event

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/metrics"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	CircuitStateClosed CircuitState = iota
	CircuitStateOpen
	CircuitStateHalfOpen
)

// String returns the string representation of the circuit state
func (s CircuitState) String() string {
	switch s {
	case CircuitStateClosed:
		return "closed"
	case CircuitStateOpen:
		return "open"
	case CircuitStateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for a circuit breaker
type CircuitBreakerConfig struct {
	FailureThreshold int64         `json:"failure_threshold"`
	RecoveryTimeout  time.Duration `json:"recovery_timeout"`
	HalfOpenMaxCalls int64         `json:"half_open_max_calls"`
	WindowSize       time.Duration `json:"window_size"`
	MinRequestCount  int64         `json:"min_request_count"`
}

// DefaultCircuitBreakerConfig returns default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		RecoveryTimeout:  30 * time.Second,
		HalfOpenMaxCalls: 3,
		WindowSize:       60 * time.Second,
		MinRequestCount:  3,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config CircuitBreakerConfig

	// State management
	state   CircuitState
	stateMu sync.RWMutex

	// Failure tracking
	failures    int64
	lastFailure time.Time
	windowStart time.Time

	// Half-open state tracking
	halfOpenCalls int64

	// Metrics
	busType   string
	eventType string
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig, busType, eventType string) *CircuitBreaker {
	return &CircuitBreaker{
		config:    config,
		state:     CircuitStateClosed,
		busType:   busType,
		eventType: eventType,
	}
}

// Execute executes a function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	if !cb.canExecute() {
		// Record circuit open metric
		metrics.EventBusEventsFailedTotal.WithLabelValues(
			cb.eventType, cb.busType, "circuit_open").Inc()

		// Update circuit breaker state metric
		metrics.EventBusCircuitBreakerState.WithLabelValues(
			cb.eventType, cb.busType).Set(float64(CircuitStateOpen))

		return fmt.Errorf("circuit breaker is open")
	}

	// Execute the function
	err := fn()
	cb.recordResult(err)
	return err
}

// canExecute determines if the circuit breaker allows execution
func (cb *CircuitBreaker) canExecute() bool {
	cb.stateMu.RLock()
	defer cb.stateMu.RUnlock()

	switch cb.state {
	case CircuitStateClosed:
		return true
	case CircuitStateOpen:
		// Check if recovery timeout has passed
		if time.Since(cb.lastFailure) >= cb.config.RecoveryTimeout {
			cb.stateMu.RUnlock()
			cb.stateMu.Lock()
			cb.state = CircuitStateHalfOpen
			cb.halfOpenCalls = 0
			cb.stateMu.Unlock()
			cb.stateMu.RLock()
			return true
		}
		return false
	case CircuitStateHalfOpen:
		return cb.halfOpenCalls < cb.config.HalfOpenMaxCalls
	default:
		return false
	}
}

// recordResult records the result of an execution
func (cb *CircuitBreaker) recordResult(err error) {
	cb.stateMu.Lock()
	defer cb.stateMu.Unlock()

	now := time.Now()
	previousState := cb.state

	// Update window if needed
	if now.Sub(cb.windowStart) > cb.config.WindowSize {
		cb.windowStart = now
		cb.failures = 0
	}

	if err != nil {
		cb.failures++
		cb.lastFailure = now

		// Record failure metric
		metrics.EventBusCircuitBreakerFailures.WithLabelValues(
			cb.eventType, cb.busType).Inc()

		// Update last failure timestamp
		metrics.EventBusCircuitBreakerLastFailureTime.WithLabelValues(
			cb.eventType, cb.busType).Set(float64(now.Unix()))

		// Check if we should open the circuit
		if cb.state == CircuitStateClosed && cb.failures >= cb.config.FailureThreshold {
			cb.state = CircuitStateOpen
		} else if cb.state == CircuitStateHalfOpen {
			// Any failure in half-open state opens the circuit
			cb.state = CircuitStateOpen
		}
	} else {
		// Success
		if cb.state == CircuitStateHalfOpen {
			cb.halfOpenCalls++
			if cb.halfOpenCalls >= cb.config.HalfOpenMaxCalls {
				// Circuit is healthy again, close it
				cb.state = CircuitStateClosed
				cb.failures = 0
				cb.halfOpenCalls = 0
			}
		}
	}

	// Record state transition if state changed
	if previousState != cb.state {
		metrics.EventBusCircuitBreakerTransitions.WithLabelValues(
			cb.eventType, cb.busType,
			previousState.String(), cb.state.String()).Inc()
	}

	// Update circuit breaker state metric
	metrics.EventBusCircuitBreakerState.WithLabelValues(
		cb.eventType, cb.busType).Set(float64(cb.state))

	// Calculate and update success rate
	if cb.failures > 0 {
		totalCalls := cb.failures + cb.halfOpenCalls
		successRate := float64(totalCalls-cb.failures) / float64(totalCalls)
		metrics.EventBusCircuitBreakerSuccessRate.WithLabelValues(
			cb.eventType, cb.busType).Set(successRate)
	} else {
		// No failures, 100% success rate
		metrics.EventBusCircuitBreakerSuccessRate.WithLabelValues(
			cb.eventType, cb.busType).Set(1.0)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitState {
	cb.stateMu.RLock()
	defer cb.stateMu.RUnlock()
	return cb.state
}

// GetStats returns statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.stateMu.RLock()
	defer cb.stateMu.RUnlock()

	return map[string]interface{}{
		"state":           cb.state.String(),
		"failures":        cb.failures,
		"last_failure":    cb.lastFailure,
		"half_open_calls": cb.halfOpenCalls,
		"window_start":    cb.windowStart,
		"bus_type":        cb.busType,
		"event_type":      cb.eventType,
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.stateMu.Lock()
	defer cb.stateMu.Unlock()

	previousState := cb.state
	cb.state = CircuitStateClosed
	cb.failures = 0
	cb.halfOpenCalls = 0
	cb.lastFailure = time.Time{}
	cb.windowStart = time.Time{}

	// Record transition to closed state
	if previousState != CircuitStateClosed {
		metrics.EventBusCircuitBreakerTransitions.WithLabelValues(
			cb.eventType, cb.busType,
			previousState.String(), cb.state.String()).Inc()
	}

	// Update metrics
	metrics.EventBusCircuitBreakerState.WithLabelValues(
		cb.eventType, cb.busType).Set(float64(CircuitStateClosed))
	metrics.EventBusCircuitBreakerSuccessRate.WithLabelValues(
		cb.eventType, cb.busType).Set(1.0)
	metrics.EventBusCircuitBreakerLastFailureTime.WithLabelValues(
		cb.eventType, cb.busType).Set(0)
}

// CircuitBreakerManager manages multiple circuit breakers
type CircuitBreakerManager struct {
	breakers map[string]*CircuitBreaker
	config   CircuitBreakerConfig
	mu       sync.RWMutex
}

// NewCircuitBreakerManager creates a new circuit breaker manager
func NewCircuitBreakerManager(config CircuitBreakerConfig) *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
		config:   config,
	}
}

// GetOrCreate gets an existing circuit breaker or creates a new one
func (mgr *CircuitBreakerManager) GetOrCreate(busType, eventType string) *CircuitBreaker {
	key := fmt.Sprintf("%s:%s", busType, eventType)

	mgr.mu.RLock()
	breaker, exists := mgr.breakers[key]
	mgr.mu.RUnlock()

	if exists {
		return breaker
	}

	mgr.mu.Lock()
	defer mgr.mu.Unlock()

	// Double-check after acquiring write lock
	if breaker, exists = mgr.breakers[key]; exists {
		return breaker
	}

	breaker = NewCircuitBreaker(mgr.config, busType, eventType)
	mgr.breakers[key] = breaker
	return breaker
}

// GetStats returns statistics for all circuit breakers
func (mgr *CircuitBreakerManager) GetStats() map[string]interface{} {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	stats := make(map[string]interface{})
	for key, breaker := range mgr.breakers {
		stats[key] = breaker.GetStats()
	}

	return stats
}

// ResetAll resets all circuit breakers
func (mgr *CircuitBreakerManager) ResetAll() {
	mgr.mu.RLock()
	defer mgr.mu.RUnlock()

	for _, breaker := range mgr.breakers {
		breaker.Reset()
	}
}
