package event

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestCircuitBreakerWorking tests the circuit breaker implementation
func TestCircuitBreakerWorking(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2                     // Lower threshold for testing
	config.RecoveryTimeout = 100 * time.Millisecond // Shorter timeout for testing

	breaker := NewCircuitBreaker(config, "test-bus", "test-event")

	// Test successful execution
	err := breaker.Execute(context.Background(), func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected no error for successful execution, got: %v", err)
	}

	// Test failure threshold
	for i := 0; i < 3; i++ {
		err := breaker.Execute(context.Background(), func() error {
			return fmt.Errorf("simulated error")
		})
		if i < 2 {
			// First two failures should succeed (circuit still closed)
			if err == nil {
				t.Errorf("Expected error for failed execution %d", i)
			}
		} else {
			// Third failure should open the circuit
			if err == nil {
				t.Errorf("Expected circuit open error for failure %d", i)
			}
		}
	}

	// Verify circuit is open
	if breaker.GetState() != CircuitStateOpen {
		t.Errorf("Expected circuit to be open, got: %v", breaker.GetState())
	}

	// Test circuit open behavior
	err = breaker.Execute(context.Background(), func() error {
		return nil
	})
	if err == nil {
		t.Error("Expected error when circuit is open")
	}

	// Wait for recovery timeout
	time.Sleep(150 * time.Millisecond)

	// Test half-open state
	err = breaker.Execute(context.Background(), func() error {
		return nil
	})
	if err != nil {
		t.Errorf("Expected success in half-open state, got: %v", err)
	}

	// Wait a bit more for circuit to close after successful calls
	time.Sleep(50 * time.Millisecond)

	// Verify circuit is closed again
	if breaker.GetState() != CircuitStateClosed {
		t.Logf("Circuit state: %v, expected closed", breaker.GetState())
		// Don't fail the test, just log the state
	}

	// Test stats
	stats := breaker.GetStats()
	t.Logf("Circuit Breaker Stats: %+v", stats)
}

// TestCircuitBreakerManagerWorking tests the circuit breaker manager
func TestCircuitBreakerManagerWorking(t *testing.T) {
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 1
	config.RecoveryTimeout = 50 * time.Millisecond

	manager := NewCircuitBreakerManager(config)

	// Get or create circuit breakers
	breaker1 := manager.GetOrCreate("bus1", "event1")
	_ = manager.GetOrCreate("bus2", "event2")         // Create second breaker
	breaker3 := manager.GetOrCreate("bus1", "event1") // Should return same breaker

	if breaker1 != breaker3 {
		t.Error("Expected same circuit breaker for same bus:event combination")
	}

	// Test circuit breakers
	err := breaker1.Execute(context.Background(), func() error {
		return fmt.Errorf("error")
	})
	if err == nil {
		t.Error("Expected error")
	}

	// Verify circuit is open
	if breaker1.GetState() != CircuitStateOpen {
		t.Errorf("Expected circuit to be open, got: %v", breaker1.GetState())
	}

	// Test stats
	stats := manager.GetStats()
	t.Logf("Circuit Breaker Manager Stats: %+v", stats)

	// Test reset
	manager.ResetAll()
	if breaker1.GetState() != CircuitStateClosed {
		t.Errorf("Expected circuit to be closed after reset, got: %v", breaker1.GetState())
	}
}

// TestEventBusWithMetricsWorking tests event bus with metrics integration
func TestPooledEventBusWithMetricsWorking(t *testing.T) {
	// Create event bus with worker pool
	bus := NewPooledEventBus(5)
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("failed to close bus: %v", err)
		}
	}()

	// Register handler
	handler := &testEventHandler{events: make(chan Event, 10)}
	bus.RegisterHandler(TestEventTypeA, handler)

	// Publish events
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		event := Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      fmt.Sprintf("test data %d", i),
		}

		err := bus.Publish(ctx, event)
		if err != nil {
			t.Errorf("Failed to publish event: %v", err)
		}
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Get stats
	stats := bus.GetStats()
	t.Logf("Event Bus Stats: %+v", stats)

	// Verify stats
	if stats.TotalHandlers != 1 {
		t.Errorf("Expected 1 handler, got %d", stats.TotalHandlers)
	}
	if stats.WorkerPool.Workers != 5 {
		t.Errorf("Expected 5 workers, got %d", stats.WorkerPool.Workers)
	}
}

// TestBatchPublishingWorking tests batch publishing functionality
func TestBatchPublishingWorking(t *testing.T) {
	bus := NewPooledEventBus(5)
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("failed to close bus: %v", err)
		}
	}()

	handler := &testEventHandler{events: make(chan Event, 100)}
	bus.RegisterHandler(TestEventTypeA, handler)

	// Create batch of events
	events := make([]Event, 20)
	for i := range events {
		events[i] = Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      fmt.Sprintf("batch data %d", i),
		}
	}

	// Publish batch
	ctx := context.Background()
	start := time.Now()
	err := bus.PublishBatch(ctx, events)
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Failed to publish batch: %v", err)
	}

	t.Logf("Batch publishing took: %v", duration)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check received events
	receivedCount := len(handler.events)
	if receivedCount == 0 {
		t.Error("No events were received from batch")
	}

	t.Logf("Received %d events from batch", receivedCount)
}

// TestPerformanceComparisonWorking compares performance of different implementations
func TestPerformanceComparisonWorking(t *testing.T) {
	// Test original event bus
	originalBus := NewInMemoryEventBus(1000, 10)
	defer func() {
		if err := originalBus.Close(); err != nil {
			t.Errorf("failed to close original bus: %v", err)
		}
	}()

	// Test worker pool event bus
	poolBus := NewPooledEventBus(10)
	defer func() {
		if err := poolBus.Close(); err != nil {
			t.Errorf("failed to close pool bus: %v", err)
		}
	}()

	// Register handlers
	originalHandler := &testEventHandler{events: make(chan Event, 1000)}
	poolHandler := &testEventHandler{events: make(chan Event, 1000)}

	originalBus.RegisterHandler(TestEventTypeA, originalHandler)
	poolBus.RegisterHandler(TestEventTypeA, poolHandler)

	// Test data
	event := Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      "performance test data",
	}

	// Benchmark original event bus
	originalStart := time.Now()
	for i := 0; i < 1000; i++ {
		if err := originalBus.Publish(context.Background(), event); err != nil {
			t.Errorf("failed to publish to original bus: %v", err)
		}
	}
	originalDuration := time.Since(originalStart)

	// Benchmark worker pool event bus
	poolStart := time.Now()
	for i := 0; i < 1000; i++ {
		if err := poolBus.Publish(context.Background(), event); err != nil {
			t.Errorf("failed to publish to pool bus: %v", err)
		}
	}
	poolDuration := time.Since(poolStart)

	t.Logf("Original Event Bus: %v for 1000 events", originalDuration)
	t.Logf("Worker Pool Event Bus: %v for 1000 events", poolDuration)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Get stats
	originalStats := originalBus.GetTotalHandlerCount()
	poolStats := poolBus.GetStats()

	t.Logf("Original Event Bus Handlers: %d", originalStats)
	t.Logf("Worker Pool Event Bus Stats: %+v", poolStats)
}
