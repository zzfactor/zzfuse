package util

import (
	"fmt"
	"sync"
	"time"
)

// CircuitBreaker provides protection against database overload
type CircuitBreaker struct {
	failureThreshold int
	resetTimeout     time.Duration
	state            CircuitBreakerState
	failureCount     int
	lastFailureTime  time.Time
	mutex            sync.RWMutex
}

// CircuitBreakerState represents the current state of the circuit breaker
type CircuitBreakerState int

const (
	CircuitBreakerClosed CircuitBreakerState = iota
	CircuitBreakerOpen
	CircuitBreakerHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker with default settings
func NewCircuitBreaker() *CircuitBreaker {
	return NewCircuitBreakerWithConfig(5, 30*time.Second)
}

// NewCircuitBreakerWithConfig creates a circuit breaker with custom settings
func NewCircuitBreakerWithConfig(failureThreshold int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		resetTimeout:     resetTimeout,
		state:            CircuitBreakerClosed,
		failureCount:     0,
	}
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()

	switch cb.state {
	case CircuitBreakerOpen:
		// Check if enough time has passed to transition to half-open
		if time.Since(cb.lastFailureTime) >= cb.resetTimeout {
			cb.mutex.RUnlock()
			cb.mutex.Lock()
			cb.state = CircuitBreakerHalfOpen
			cb.mutex.Unlock()
			cb.mutex.RLock()
			return false
		}
		return true
	case CircuitBreakerHalfOpen:
		return false
	default:
		return false
	}
}

// Execute runs the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(operation func() error) error {
	if cb.IsOpen() {
		return fmt.Errorf("circuit breaker is open")
	}

	err := operation()

	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	if err != nil {
		cb.failureCount++
		cb.lastFailureTime = time.Now()

		if cb.state == CircuitBreakerHalfOpen || cb.failureCount >= cb.failureThreshold {
			cb.state = CircuitBreakerOpen
		}
	} else {
		// Success - reset failure count and close circuit
		cb.failureCount = 0
		cb.state = CircuitBreakerClosed
	}

	return err
}

// GetState returns the current circuit breaker state
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetFailureCount returns the current failure count
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.failureCount
}
