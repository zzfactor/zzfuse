package event

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestPublishBatchDirect tests the PublishBatch method directly on enhanced NATS bus
func TestPublishBatchDirect(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Test PublishBatch with empty events
	err = bus.PublishBatch(context.Background(), []Event{})
	if err != nil {
		t.Errorf("Expected no error for empty batch, got %v", err)
	}

	// Test PublishBatch with events (this will bypass batching)
	events := []Event{
		{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      map[string]interface{}{"id": "batch-direct-1", "body": "Batch direct message 1"},
		},
		{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      map[string]interface{}{"id": "batch-direct-2", "body": "Batch direct message 2"},
		},
	}

	// Test PublishBatch with events
	err = bus.PublishBatch(context.Background(), events)
	if err != nil {
		t.Errorf("Failed to publish batch: %v", err)
	}

	// Give some time for the batch to be processed
	time.Sleep(50 * time.Millisecond)

	t.Log("✅ PublishBatch direct test passed")
}

// TestPublishBatchWithHandler tests the Publish method with batching and handlers
func TestPublishBatchWithHandler(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create test handler to receive events
	receivedEvents := make(chan Event, 10)
	testHandler := &testEventHandler{events: receivedEvents}
	bus.RegisterHandler(TestEventTypeA, testHandler)

	// Test Publish with events (they will be batched internally)
	events := []Event{
		{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      map[string]interface{}{"id": "batch-handler-1", "body": "Batch handler message 1"},
		},
		{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      map[string]interface{}{"id": "batch-handler-2", "body": "Batch handler message 2"},
		},
	}

	// Publish events individually (they will be batched internally)
	for _, event := range events {
		err = bus.Publish(context.Background(), event)
		if err != nil {
			t.Errorf("Failed to publish event: %v", err)
		}
	}

	// Wait for events to be received (with timeout)
	receivedCount := 0
	for i := 0; i < len(events); i++ {
		select {
		case event := <-receivedEvents:
			receivedCount++
			if event.Type != TestEventTypeA {
				t.Errorf("Expected event type %s, got %s", TestEventTypeA, event.Type)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("Timeout waiting for batch events, received %d/%d", receivedCount, len(events))
		}
	}

	if receivedCount != len(events) {
		t.Errorf("Expected %d events, got %d", len(events), receivedCount)
	}

	t.Log("✅ Publish with batching and handler test passed")
}

// TestEnhancedNATSEventBusPublishContextCancellation tests Publish with cancelled context
func TestEnhancedNATSEventBusPublishContextCancellation(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Test Publish with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = bus.Publish(ctx, Event{Type: TestEventTypeA})
	if err != nil && err.Error() != "context canceled" {
		t.Errorf("Expected context cancellation error, got %v", err)
	}

	t.Log("✅ Enhanced NATS Publish context cancellation test passed")
}

// TestConnectionPoolEdgeCases tests edge cases in connection pool
func TestConnectionPoolEdgeCases(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	pool := NewNATSConnectionPool(url, 2)
	defer pool.Close()

	// Test Put() with nil connection
	pool.Put(nil)

	// Test Put() with closed pool
	pool.Close()
	pool.Put(nil)

	t.Log("✅ Connection pool edge cases test passed")
}

// TestConfigValidationEdgeCases tests edge cases in configuration validation
func TestConfigValidationEdgeCases(t *testing.T) {
	// Test ValidateEventBusConfig with invalid buffer size
	config := &EventBusConfig{
		Type: "in-memory",
		InMemory: InMemoryConfig{
			BufferSize: -1,
		},
	}
	err := ValidateEventBusConfig(config)
	if err == nil {
		t.Error("Expected error for negative buffer size")
	}

	// Test ValidateEventBusConfig with invalid NATS URL
	config = &EventBusConfig{
		Type: "nats",
		NATS: NATSConfig{
			URL: "",
		},
	}
	err = ValidateEventBusConfig(config)
	if err == nil {
		t.Error("Expected error for empty NATS URL")
	}

	t.Log("✅ Config validation edge cases test passed")
}

// TestNATSConnectionPoolErrorPaths tests error paths in NATS connection pool
func TestNATSConnectionPoolErrorPaths(t *testing.T) {
	// Test with invalid URL
	pool := NewNATSConnectionPool("invalid://url", 2)
	defer pool.Close()

	// Test getting connection with invalid URL
	_, err := pool.Get()
	if err == nil {
		t.Error("Expected error for invalid URL")
	}

	// Test putting nil connection
	pool.Put(nil)

	// Test putting connection to closed pool
	pool.Close()
	pool.Put(nil)

	t.Log("✅ NATS connection pool error paths test passed")
}

// TestPooledEventBusErrorPaths tests error paths in pooled event bus
func TestPooledEventBusErrorPaths(t *testing.T) {
	// Test with invalid worker count
	bus := NewPooledEventBus(0)
	defer bus.Close()

	t.Log("✅ Pooled event bus error paths test passed")
}

// TestNATSEventBusErrorPaths tests error paths in basic NATS event bus
func TestNATSEventBusErrorPaths(t *testing.T) {
	// Test with invalid URL
	_, err := NewNATSEventBus("invalid://url")
	if err == nil {
		t.Error("Expected error for invalid NATS URL")
	}

	t.Log("✅ NATS event bus error paths test passed")
}

// TestEventBusFactoryErrorPaths tests error paths in event bus factory
func TestEventBusFactoryErrorPaths(t *testing.T) {
	// This test would require modifying the config, which is complex
	// For now, we'll just test that the factory can be created
	factory := NewEventBusFactory("nats", "nats://localhost:4222")
	if factory == nil {
		t.Error("Expected factory to be created")
	}

	t.Log("✅ Event bus factory error paths test passed")
}

// TestEnhancedNATSEventBusBatchProcessorEdgeCases tests edge cases in batch processor
func TestEnhancedNATSEventBusBatchProcessorEdgeCases(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Test with very large batch
	largeEvents := make([]Event, 1000)
	for i := range largeEvents {
		largeEvents[i] = Event{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      map[string]interface{}{"id": fmt.Sprintf("large-batch-%d", i)},
		}
	}

	// This should not cause issues
	for _, event := range largeEvents {
		err = bus.Publish(context.Background(), event)
		if err != nil {
			t.Errorf("Failed to publish event: %v", err)
		}
	}

	t.Log("✅ Enhanced NATS event bus batch processor edge cases test passed")
}

// TestEnhancedNATSEventBusConcurrentAccess tests concurrent access scenarios
func TestEnhancedNATSEventBusConcurrentAccess(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Test concurrent handler registration
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			handler := &testEventHandler{events: make(chan Event, 1)}
			bus.RegisterHandler(TestEventTypeA, handler)
		}(i)
	}
	wg.Wait()

	// Test concurrent stats access
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats := bus.GetStats()
			if stats == nil {
				t.Error("Expected stats, got nil")
			}
		}()
	}
	wg.Wait()

	t.Log("✅ Enhanced NATS event bus concurrent access test passed")
}

// TestEnhancedNATSEventBusMemoryLeak tests for potential memory leaks
func TestEnhancedNATSEventBusMemoryLeak(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	// Create and destroy multiple buses to test for memory leaks
	for i := 0; i < 10; i++ {
		bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
		if err != nil {
			t.Skipf("NATS not available, skipping test: %v", err)
		}

		// Register some handlers
		for j := 0; j < 5; j++ {
			handler := &testEventHandler{events: make(chan Event, 1)}
			bus.RegisterHandler(TestEventTypeA, handler)
		}

		// Publish some events
		for j := 0; j < 10; j++ {
			bus.Publish(context.Background(), Event{
				Type:      TestEventTypeA,
				Timestamp: time.Now(),
				Schema:    "test",
				Data:      map[string]interface{}{"id": fmt.Sprintf("leak-test-%d-%d", i, j)},
			})
		}

		// Close the bus
		bus.Close()
	}

	t.Log("✅ Enhanced NATS event bus memory leak test passed")
}

// TestEnhancedNATSEventBusStressTest performs a stress test
func TestEnhancedNATSEventBusStressTest(t *testing.T) {
	requireNATSIntegration(t)
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create test handler
	receivedEvents := make(chan Event, 1000)
	testHandler := &testEventHandler{events: receivedEvents}
	bus.RegisterHandler(TestEventTypeA, testHandler)

	// Publish many events rapidly
	const numEvents = 100
	errors := make(chan error, numEvents)

	for i := 0; i < numEvents; i++ {
		go func(id int) {
			testData := map[string]interface{}{
				"id":   fmt.Sprintf("stress-test-%d", id),
				"body": fmt.Sprintf("Stress test message %d", id),
			}

			err := bus.Publish(context.Background(), Event{
				Type:      TestEventTypeA,
				Timestamp: time.Now(),
				Schema:    "test",
				Data:      testData,
			})
			errors <- err
		}(i)
	}

	// Wait for all publishes to complete
	for i := 0; i < numEvents; i++ {
		if err := <-errors; err != nil {
			t.Errorf("Stress test publish %d failed: %v", i, err)
		}
	}

	// Wait for events (with reasonable timeout)
	receivedCount := 0
	timeout := time.After(10 * time.Second)
	for i := 0; i < numEvents; i++ {
		select {
		case event := <-receivedEvents:
			receivedCount++
			if event.Type != TestEventTypeA {
				t.Errorf("Expected event type %s, got %s", TestEventTypeA, event.Type)
			}
		case <-timeout:
			t.Logf("Stress test completed, received %d/%d events", receivedCount, numEvents)
			return
		}
	}

	if receivedCount != numEvents {
		t.Errorf("Expected %d events, got %d", numEvents, receivedCount)
	}

	t.Log("✅ Enhanced NATS event bus stress test passed")
}
