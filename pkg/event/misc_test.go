package event

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestInMemoryEventBusCleanup verifies that the event bus properly cleans up resources
func TestInMemoryEventBusCleanup(t *testing.T) {
	// Create event bus
	bus := NewInMemoryEventBus(100, 5)

	// Register some handlers
	handler1 := &testEventHandler{events: make(chan Event, 10)}
	handler2 := &testEventHandler{events: make(chan Event, 10)}

	bus.RegisterHandler(TestEventTypeA, handler1)
	bus.RegisterHandler(TestEventTypeB, handler2)

	// Verify handlers are registered
	if bus.GetHandlerCount(TestEventTypeA) != 1 {
		t.Errorf("Expected 1 handler for log.inserted, got %d", bus.GetHandlerCount(TestEventTypeA))
	}
	if bus.GetHandlerCount(TestEventTypeB) != 1 {
		t.Errorf("Expected 1 handler for trace.inserted, got %d", bus.GetHandlerCount(TestEventTypeB))
	}
	if bus.GetTotalHandlerCount() != 2 {
		t.Errorf("Expected 2 total handlers, got %d", bus.GetTotalHandlerCount())
	}

	// Publish some events to create active operations
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		err := bus.Publish(ctx, Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      "test data",
		})
		if err != nil {
			t.Errorf("Failed to publish event: %v", err)
		}
	}

	// Give some time for operations to start
	time.Sleep(10 * time.Millisecond)

	// Close the event bus
	start := time.Now()
	err := bus.Close()
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Failed to close event bus: %v", err)
	}

	// Verify closing doesn't take too long (should be quick for in-memory)
	if duration > 100*time.Millisecond {
		t.Errorf("Event bus close took too long: %v", duration)
	}

	// Verify the event bus is marked as closing
	if !bus.IsClosing() {
		t.Error("Event bus should be marked as closing after Close()")
	}

	// Verify handlers are cleared
	if bus.GetTotalHandlerCount() != 0 {
		t.Errorf("Expected 0 handlers after close, got %d", bus.GetTotalHandlerCount())
	}

	t.Log("✅ Event bus cleanup test passed")
}

// TestInMemoryEventBusGracefulShutdown verifies graceful shutdown behavior
func TestInMemoryEventBusGracefulShutdown(t *testing.T) {
	bus := NewInMemoryEventBus(100, 5)

	// Create a slow handler that takes time to process
	slowHandler := &slowTestHandler{
		processingTime: 50 * time.Millisecond,
		events:         make(chan Event, 10),
	}

	bus.RegisterHandler(TestEventTypeA, slowHandler)

	// Publish events to create active operations
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		err := bus.Publish(ctx, Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      "slow test data",
		})
		if err != nil {
			t.Errorf("Failed to publish event: %v", err)
		}
	}

	// Give time for operations to start
	time.Sleep(10 * time.Millisecond)

	// Close should wait for all operations to complete
	start := time.Now()
	err := bus.Close()
	duration := time.Since(start)

	if err != nil {
		t.Errorf("Failed to close event bus: %v", err)
	}

	// Should take at least the processing time of the slowest operation (with some tolerance)
	if duration < 35*time.Millisecond {
		t.Errorf("Close completed too quickly: %v (expected at least 35ms)", duration)
	}

	// Should not take too long (with some buffer for overhead)
	if duration > 200*time.Millisecond {
		t.Errorf("Close took too long: %v (expected around 50ms)", duration)
	}

	t.Logf("✅ Graceful shutdown test passed - took %v", duration)
}

// TestInMemoryEventBusNoNewOperationsDuringShutdown verifies that new operations are rejected during shutdown
func TestInMemoryEventBusNoNewOperationsDuringShutdown(t *testing.T) {
	bus := NewInMemoryEventBus(100, 5)

	// Start closing the event bus
	go func() {
		time.Sleep(10 * time.Millisecond) // Give time for test to set up
		if err := bus.Close(); err != nil {
			t.Errorf("failed to close bus: %v", err)
		}
	}()

	// Wait for closing to start
	time.Sleep(15 * time.Millisecond)

	// Try to register a new handler (should be ignored)
	handler := &testEventHandler{events: make(chan Event, 10)}
	bus.RegisterHandler(TestEventTypeA, handler)

	// Verify no handler was registered
	if bus.GetHandlerCount(TestEventTypeA) != 0 {
		t.Error("Handler should not be registered during shutdown")
	}

	// Try to publish an event (should return immediately)
	err := bus.Publish(context.Background(), Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      "test data",
	})

	if err != nil {
		t.Errorf("Publish should return nil during shutdown, got: %v", err)
	}

	t.Log("✅ No new operations during shutdown test passed")
}

// TestInMemoryEventBusConcurrentAccess verifies thread safety during cleanup
func TestInMemoryEventBusConcurrentAccess(t *testing.T) {
	bus := NewInMemoryEventBus(100, 10)

	// Register handlers
	handler := &testEventHandler{events: make(chan Event, 100)}
	bus.RegisterHandler(TestEventTypeA, handler)

	// Start concurrent operations
	var wg sync.WaitGroup
	ctx := context.Background()

	// Start publishing goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				err := bus.Publish(ctx, Event{
					Type:      TestEventTypeA,
					Timestamp: time.Now(),
					Schema:    "test",
					Data:      "concurrent test data",
				})
				if err != nil {
					t.Errorf("Failed to publish event: %v", err)
				}
				time.Sleep(1 * time.Millisecond)
			}
		}(i)
	}

	// Start closing goroutine
	go func() {
		time.Sleep(20 * time.Millisecond) // Let some operations start
		if err := bus.Close(); err != nil {
			t.Errorf("failed to close bus: %v", err)
		}
	}()

	// Wait for all operations to complete
	wg.Wait()

	// Give some time for cleanup to complete
	time.Sleep(10 * time.Millisecond)

	// Verify cleanup completed
	if bus.GetTotalHandlerCount() != 0 {
		t.Error("Handlers should be cleared after close")
	}

	t.Log("✅ Concurrent access test passed")
}

// TestInMemoryEventBusMemoryCleanup verifies that memory is properly freed
func TestInMemoryEventBusMemoryCleanup(t *testing.T) {
	// Create event bus with many handlers
	bus := NewInMemoryEventBus(100, 5)

	// Register many handlers
	for i := 0; i < 100; i++ {
		handler := &testEventHandler{events: make(chan Event, 10)}
		bus.RegisterHandler(TestEventTypeA, handler)
	}

	// Verify handlers are registered
	if bus.GetTotalHandlerCount() != 100 {
		t.Errorf("Expected 100 handlers, got %d", bus.GetTotalHandlerCount())
	}

	// Close the event bus
	err := bus.Close()
	if err != nil {
		t.Errorf("Failed to close event bus: %v", err)
	}

	// Verify all handlers are cleared
	if bus.GetTotalHandlerCount() != 0 {
		t.Errorf("Expected 0 handlers after close, got %d", bus.GetTotalHandlerCount())
	}

	t.Log("✅ Memory cleanup test passed")
}

// slowTestHandler is a test handler that takes time to process events
type slowTestHandler struct {
	processingTime time.Duration
	events         chan Event
}

func (h *slowTestHandler) HandleEvent(ctx context.Context, event Event) error {
	// Simulate slow processing
	time.Sleep(h.processingTime)
	h.events <- event
	return nil
}
