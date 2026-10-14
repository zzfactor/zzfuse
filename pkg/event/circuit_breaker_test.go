package event

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestCircuitBreakerCreation tests circuit breaker creation
func TestCircuitBreakerCreation(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	if breaker == nil {
		t.Fatal("Expected circuit breaker, got nil")
	}

	// Test initial state
	if breaker.GetState() != CircuitStateClosed {
		t.Errorf("Expected initial state to be closed, got %v", breaker.GetState())
	}
}

// TestCircuitBreakerExecuteSuccess tests successful execution
func TestCircuitBreakerExecuteSuccess(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Test successful execution
	err := breaker.Execute(context.Background(), func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if breaker.GetState() != CircuitStateClosed {
		t.Errorf("Expected state to remain closed, got %v", breaker.GetState())
	}
}

// TestCircuitBreakerExecuteFailure tests failure handling
func TestCircuitBreakerExecuteFailure(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 3 // Higher threshold for testing
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	testErr := errors.New("test error")

	// Execute failing function multiple times (below threshold)
	for i := 0; i < int(config.FailureThreshold)-1; i++ {
		err := breaker.Execute(context.Background(), func() error {
			return testErr
		})

		if err != testErr {
			t.Errorf("Expected test error, got %v", err)
		}
	}

	// Circuit should still be closed (threshold not exceeded)
	if breaker.GetState() != CircuitStateClosed {
		t.Errorf("Expected state to be closed, got %v", breaker.GetState())
	}

	// One more failure should open the circuit
	err := breaker.Execute(context.Background(), func() error {
		return testErr
	})

	if err != testErr {
		t.Errorf("Expected test error, got %v", err)
	}

	// Circuit should now be open
	if breaker.GetState() != CircuitStateOpen {
		t.Errorf("Expected state to be open, got %v", breaker.GetState())
	}
}

// TestCircuitBreakerOpenState tests behavior when circuit is open
func TestCircuitBreakerOpenState(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 1
	config.RecoveryTimeout = 100 * time.Millisecond
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Trigger circuit to open
	breaker.Execute(context.Background(), func() error {
		return errors.New("test error")
	})

	// Circuit should be open
	if breaker.GetState() != CircuitStateOpen {
		t.Errorf("Expected state to be open, got %v", breaker.GetState())
	}

	// Execute should fail immediately
	err := breaker.Execute(context.Background(), func() error {
		return nil
	})

	if err == nil {
		t.Error("Expected error when circuit is open")
	}

	if err.Error() != "circuit breaker is open" {
		t.Errorf("Expected 'circuit breaker is open' error, got %v", err)
	}
}

// TestCircuitBreakerRecovery tests circuit recovery
func TestCircuitBreakerRecovery(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 1
	config.RecoveryTimeout = 50 * time.Millisecond
	config.HalfOpenMaxCalls = 2
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Trigger circuit to open
	err := breaker.Execute(context.Background(), func() error {
		return errors.New("test error")
	})

	// Circuit should be open
	if breaker.GetState() != CircuitStateOpen {
		t.Errorf("Expected state to be open, got %v", breaker.GetState())
	}

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)

	// Try to execute to trigger half-open transition
	_ = breaker.Execute(context.Background(), func() error {
		return nil
	})

	// Circuit should be in half-open state after first successful call
	if breaker.GetState() != CircuitStateHalfOpen {
		t.Errorf("Expected state to be half-open, got %v", breaker.GetState())
	}

	// Execute should succeed in half-open state
	err = breaker.Execute(context.Background(), func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error in half-open state, got %v", err)
	}

	// Execute one more successful call
	err = breaker.Execute(context.Background(), func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error in half-open state, got %v", err)
	}

	// Circuit should be closed again
	if breaker.GetState() != CircuitStateClosed {
		t.Errorf("Expected state to be closed, got %v", breaker.GetState())
	}
}

// TestCircuitBreakerHalfOpenFailure tests failure in half-open state
func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 1
	config.RecoveryTimeout = 50 * time.Millisecond
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Trigger circuit to open
	breaker.Execute(context.Background(), func() error {
		return errors.New("test error")
	})

	// Wait for recovery timeout
	time.Sleep(100 * time.Millisecond)

	// Execute should fail in half-open state
	err := breaker.Execute(context.Background(), func() error {
		return errors.New("test error")
	})

	if err == nil {
		t.Error("Expected error in half-open state")
	}

	// Circuit should be open again
	if breaker.GetState() != CircuitStateOpen {
		t.Errorf("Expected state to be open, got %v", breaker.GetState())
	}
}

// TestCircuitBreakerReset tests circuit breaker reset
func TestCircuitBreakerReset(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 1
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Trigger circuit to open
	breaker.Execute(context.Background(), func() error {
		return errors.New("test error")
	})

	// Circuit should be open
	if breaker.GetState() != CircuitStateOpen {
		t.Errorf("Expected state to be open, got %v", breaker.GetState())
	}

	// Reset the circuit breaker
	breaker.Reset()

	// Circuit should be closed
	if breaker.GetState() != CircuitStateClosed {
		t.Errorf("Expected state to be closed after reset, got %v", breaker.GetState())
	}

	// Execute should succeed
	err := breaker.Execute(context.Background(), func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error after reset, got %v", err)
	}
}

// TestCircuitBreakerStats tests statistics functionality
func TestCircuitBreakerStats(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Get initial stats
	stats := breaker.GetStats()

	if stats["state"] != "closed" {
		t.Errorf("Expected initial state 'closed', got %v", stats["state"])
	}

	if stats["bus_type"] != "test-bus" {
		t.Errorf("Expected bus_type 'test-bus', got %v", stats["bus_type"])
	}

	if stats["event_type"] != "test-event" {
		t.Errorf("Expected event_type 'test-event', got %v", stats["event_type"])
	}
}

// TestCircuitBreakerManagerBasic tests circuit breaker manager
func TestCircuitBreakerManagerBasic(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	manager := NewCircuitBreakerManager(config)

	// Get or create circuit breakers
	breaker1 := manager.GetOrCreate("bus1", "event1")
	breaker2 := manager.GetOrCreate("bus2", "event2")
	breaker3 := manager.GetOrCreate("bus1", "event1") // Should return same instance

	if breaker1 == nil || breaker2 == nil {
		t.Fatal("Expected circuit breakers, got nil")
	}

	if breaker1 != breaker3 {
		t.Error("Expected same circuit breaker instance for same key")
	}

	if breaker1 == breaker2 {
		t.Error("Expected different circuit breaker instances for different keys")
	}

	// Test manager stats
	stats := manager.GetStats()
	if len(stats) == 0 {
		t.Error("Expected manager stats to contain circuit breakers")
	}

	// Test reset all
	manager.ResetAll()

	// Verify all breakers are reset
	for _, breaker := range manager.breakers {
		if breaker.GetState() != CircuitStateClosed {
			t.Errorf("Expected all breakers to be closed after reset, got %v", breaker.GetState())
		}
	}
}

// TestCircuitBreakerConcurrentAccess tests concurrent access to circuit breaker
func TestCircuitBreakerConcurrentAccess(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 5
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Test concurrent execution
	const numGoroutines = 10
	results := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			err := breaker.Execute(context.Background(), func() error {
				return nil
			})
			results <- err
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		err := <-results
		if err != nil {
			t.Errorf("Expected no error from concurrent execution, got %v", err)
		}
	}

	// Circuit should still be closed
	if breaker.GetState() != CircuitStateClosed {
		t.Errorf("Expected state to be closed after concurrent access, got %v", breaker.GetState())
	}
}

// TestCircuitBreakerContextCancellation tests context cancellation
func TestCircuitBreakerContextCancellation(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Execute with cancelled context - should still work since context is checked in the function
	err := breaker.Execute(ctx, func() error {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return context.Canceled
		default:
			return nil
		}
	})

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

// TestCircuitBreakerStringMethod tests string representation
func TestCircuitBreakerStringMethod(t *testing.T) {
	// Test all circuit states
	states := []CircuitState{
		CircuitStateClosed,
		CircuitStateOpen,
		CircuitStateHalfOpen,
	}

	expectedStrings := []string{
		"closed",
		"open",
		"half-open",
	}

	for i, state := range states {
		if state.String() != expectedStrings[i] {
			t.Errorf("Expected state string '%s', got '%s'", expectedStrings[i], state.String())
		}
	}

	// Test unknown state
	unknownState := CircuitState(999)
	if unknownState.String() != "unknown" {
		t.Errorf("Expected unknown state string 'unknown', got '%s'", unknownState.String())
	}
}
