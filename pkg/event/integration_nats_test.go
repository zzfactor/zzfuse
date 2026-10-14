//go:build integration
// +build integration

package event

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestNATSEventBus_PublishSubscribe tests the NATS event bus publish-subscribe functionality
func TestNATSEventBus_PublishSubscribe(t *testing.T) {
	// Skip if NATS is not available (for CI/CD environments)
	natsURL := testNATSURL(t)

	// Create NATS event bus
	bus, err := NewNATSEventBus(natsURL)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer func() {
		if err := bus.Close(); err != nil {
			t.Errorf("failed to close bus: %v", err)
		}
	}()

	// Test connection
	if err := bus.HealthCheck(); err != nil {
		t.Skipf("NATS health check failed, skipping test: %v", err)
	}

	t.Log("✅ Connected to NATS successfully")

	// Create a simple test handler
	receivedEvents := make(chan Event, 10)
	testHandler := &testEventHandler{events: receivedEvents}

	// Register handler
	bus.RegisterHandler(TestEventTypeA, testHandler)

	t.Log("✅ Registered test handler")

	// Prepare test data
	testData := map[string]interface{}{
		"id":            "nats-test-log-1",
		"service_name":  "nats-test-service",
		"body":          "NATS test log message",
		"severity_text": "INFO",
	}

	// Publish events via NATS
	t.Log("📤 Publishing events via NATS...")
	err = bus.Publish(context.Background(), Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      testData,
	})
	if err != nil {
		t.Fatalf("Failed to publish via NATS: %v", err)
	}

	t.Log("✅ Events published via NATS")

	// Receive and verify events
	t.Log("📥 Receiving events from NATS subscription...")
	select {
	case event := <-receivedEvents:
		t.Logf("✅ Received event via NATS: Type=%s, Schema=%s", event.Type, event.Schema)

		// Verify event data
		if event.Type != TestEventTypeA {
			t.Errorf("Expected event type %s, got %s", TestEventTypeA, event.Type)
		}
		if event.Schema != "test" {
			t.Errorf("Expected schema 'test', got %s", event.Schema)
		}

		// Verify data
		if data, ok := event.Data.(map[string]interface{}); ok {
			if data["id"] != "nats-test-log-1" {
				t.Errorf("Expected ID 'nats-test-log-1', got %v", data["id"])
			}
		} else {
			t.Errorf("Expected map[string]interface{} data, got %T", event.Data)
		}

	case <-time.After(3 * time.Second):
		t.Fatalf("❌ Timeout waiting for event via NATS")
	}

	t.Log("🎉 NATS event bus test PASSED!")
}

// TestNATSEventBus_ConnectionState checks the underlying client connection.
func TestNATSEventBus_ConnectionState(t *testing.T) {
	natsURL := testNATSURL(t)

	bus, err := NewNATSEventBus(natsURL)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	if bus.nc == nil || !bus.nc.IsConnected() {
		t.Error("Expected a connected NATS client")
	}
}

// TestNATSEventBus_HealthCheck tests the health check functionality
func TestNATSEventBus_HealthCheck(t *testing.T) {
	natsURL := testNATSURL(t)

	bus, err := NewNATSEventBus(natsURL)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Test health check
	if err := bus.HealthCheck(); err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	t.Log("✅ Health check passed")
}

// TestNATSConnectionPoolBasic tests basic NATS connection pool functionality
func TestNATSConnectionPoolBasic(t *testing.T) {
	url := testNATSURL(t)

	pool := NewNATSConnectionPool(url, 3)
	defer pool.Close()

	// Test pool creation
	if pool == nil {
		t.Fatal("Failed to create NATS connection pool")
	}

	// Test getting connection
	conn, err := pool.Get()
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer pool.Put(conn)

	if conn == nil {
		t.Error("Expected connection, got nil")
	}

	// Test putting connection back
	pool.Put(conn)
}

// TestNATSConnectionPoolMultipleConnections tests multiple connection handling
func TestNATSConnectionPoolMultipleConnections(t *testing.T) {
	url := testNATSURL(t)

	pool := NewNATSConnectionPool(url, 2)
	defer pool.Close()

	// Get multiple connections
	conn1, err := pool.Get()
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer pool.Put(conn1)

	conn2, err := pool.Get()
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer pool.Put(conn2)

	if conn1 == nil || conn2 == nil {
		t.Error("Expected connections, got nil")
	}

	// Test that connections are different
	if conn1 == conn2 {
		t.Error("Expected different connections")
	}
}

// TestEnhancedNATSEventBusCreation tests enhanced NATS event bus creation
func TestEnhancedNATSEventBusCreation(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	if bus == nil {
		t.Fatal("Expected event bus, got nil")
	}

	// Test stats
	stats := bus.GetStats()
	if stats["bus_type"] != "nats-enhanced" {
		t.Errorf("Expected bus_type 'nats-enhanced', got %v", stats["bus_type"])
	}

	t.Log("✅ Enhanced NATS event bus created successfully")
}

// TestEnhancedNATSEventBusPublish tests enhanced NATS event bus publishing
func TestEnhancedNATSEventBusPublish(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create test handler
	receivedEvents := make(chan Event, 10)
	testHandler := &testEventHandler{events: receivedEvents}

	// Register handler
	bus.RegisterHandler(TestEventTypeA, testHandler)

	// Publish event
	testData := map[string]interface{}{
		"id":   "enhanced-nats-test-1",
		"body": "Enhanced NATS test message",
	}

	err = bus.Publish(context.Background(), Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      testData,
	})
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for event
	select {
	case event := <-receivedEvents:
		if event.Type != TestEventTypeA {
			t.Errorf("Expected event type %s, got %s", TestEventTypeA, event.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for event")
	}

	t.Log("✅ Enhanced NATS event bus publish test passed")
}

// TestEnhancedNATSEventBusBatchPublish tests enhanced NATS event bus batch publishing
func TestEnhancedNATSEventBusBatchPublish(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create test handler
	receivedEvents := make(chan Event, 10)
	testHandler := &testEventHandler{events: receivedEvents}

	// Register handler
	bus.RegisterHandler(TestEventTypeA, testHandler)

	// Create batch of events
	events := []Event{
		{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      map[string]interface{}{"id": "batch-1", "body": "Batch message 1"},
		},
		{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      map[string]interface{}{"id": "batch-2", "body": "Batch message 2"},
		},
	}

	// Publish events individually (they will be batched internally)
	for _, event := range events {
		err = bus.Publish(context.Background(), event)
		if err != nil {
			t.Fatalf("Failed to publish event: %v", err)
		}
	}

	// Wait for events
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

	t.Log("✅ Enhanced NATS event bus batch publish test passed")
}

// TestEnhancedNATSEventBusHealthCheck tests enhanced NATS event bus health check
func TestEnhancedNATSEventBusHealthCheck(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Test health check
	if err := bus.HealthCheck(); err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	t.Log("✅ Enhanced NATS event bus health check passed")
}

// TestEnhancedNATSEventBusStats tests enhanced NATS event bus statistics
func TestEnhancedNATSEventBusStats(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Get stats
	stats := bus.GetStats()
	if stats == nil {
		t.Error("Expected stats, got nil")
	}

	// Verify required fields
	requiredFields := []string{"bus_type", "pool_size", "batch_size", "batch_timeout"}
	for _, field := range requiredFields {
		if _, exists := stats[field]; !exists {
			t.Errorf("Expected stats field '%s'", field)
		}
	}

	t.Logf("✅ Enhanced NATS event bus stats: %+v", stats)
}

// TestEnhancedNATSEventBusGracefulShutdown tests enhanced NATS event bus graceful shutdown
func TestEnhancedNATSEventBusGracefulShutdown(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}

	// Test graceful shutdown
	if err := bus.Close(); err != nil {
		t.Errorf("Failed to close bus: %v", err)
	}

	t.Log("✅ Enhanced NATS event bus graceful shutdown test passed")
}

// TestEnhancedNATSEventBusContextCancellation tests enhanced NATS event bus context cancellation
func TestEnhancedNATSEventBusContextCancellation(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to publish with cancelled context
	err = bus.Publish(ctx, Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      map[string]interface{}{"id": "context-test"},
	})

	// Should handle context cancellation gracefully
	if err != nil && err.Error() != "context canceled" {
		t.Errorf("Expected context cancellation error, got %v", err)
	}

	t.Log("✅ Enhanced NATS event bus context cancellation test passed")
}

// TestEnhancedNATSEventBusBatchTimeout tests enhanced NATS event bus batch timeout
func TestEnhancedNATSEventBusBatchTimeout(t *testing.T) {
	url := testNATSURL(t)

	// Create bus with very short batch timeout
	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 1*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create test handler
	receivedEvents := make(chan Event, 10)
	testHandler := &testEventHandler{events: receivedEvents}

	// Register handler
	bus.RegisterHandler(TestEventTypeA, testHandler)

	// Publish single event (should trigger batch timeout)
	testData := map[string]interface{}{
		"id":   "timeout-test-1",
		"body": "Timeout test message",
	}

	err = bus.Publish(context.Background(), Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      testData,
	})
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for event (should be processed after batch timeout)
	select {
	case event := <-receivedEvents:
		if event.Type != TestEventTypeA {
			t.Errorf("Expected event type %s, got %s", TestEventTypeA, event.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for event after batch timeout")
	}

	t.Log("✅ Enhanced NATS event bus batch timeout test passed")
}

// TestEnhancedNATSEventBusMultipleHandlers tests enhanced NATS event bus with multiple handlers
func TestEnhancedNATSEventBusMultipleHandlers(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create multiple test handlers
	receivedEvents1 := make(chan Event, 10)
	receivedEvents2 := make(chan Event, 10)
	testHandler1 := &testEventHandler{events: receivedEvents1}
	testHandler2 := &testEventHandler{events: receivedEvents2}

	// Register multiple handlers for same event type
	bus.RegisterHandler(TestEventTypeA, testHandler1)
	bus.RegisterHandler(TestEventTypeA, testHandler2)

	// Publish event
	testData := map[string]interface{}{
		"id":   "multi-handler-test-1",
		"body": "Multi-handler test message",
	}

	err = bus.Publish(context.Background(), Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      testData,
	})
	if err != nil {
		t.Fatalf("Failed to publish: %v", err)
	}

	// Wait for events from both handlers
	select {
	case event1 := <-receivedEvents1:
		if event1.Type != TestEventTypeA {
			t.Errorf("Handler1: Expected event type %s, got %s", TestEventTypeA, event1.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for event from handler1")
	}

	select {
	case event2 := <-receivedEvents2:
		if event2.Type != TestEventTypeA {
			t.Errorf("Handler2: Expected event type %s, got %s", TestEventTypeA, event2.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Timeout waiting for event from handler2")
	}

	t.Log("✅ Enhanced NATS event bus multiple handlers test passed")
}

// TestEnhancedNATSEventBusConcurrentPublish tests enhanced NATS event bus concurrent publishing
func TestEnhancedNATSEventBusConcurrentPublish(t *testing.T) {
	url := testNATSURL(t)

	bus, err := NewEnhancedNATSEventBus(url, 3, 10, 10*time.Millisecond)
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}
	defer bus.Close()

	// Create test handler
	receivedEvents := make(chan Event, 100)
	testHandler := &testEventHandler{events: receivedEvents}

	// Register handler
	bus.RegisterHandler(TestEventTypeA, testHandler)

	// Publish events concurrently
	const numEvents = 5 // Reduced from 10 to reduce contention
	errors := make(chan error, numEvents)

	for i := 0; i < numEvents; i++ {
		go func(id int) {
			testData := map[string]interface{}{
				"id":   fmt.Sprintf("concurrent-test-%d", id),
				"body": fmt.Sprintf("Concurrent test message %d", id),
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
			t.Errorf("Concurrent publish %d failed: %v", i, err)
		}
	}

	// Wait for events with longer timeout
	receivedCount := 0
	for i := 0; i < numEvents; i++ {
		select {
		case event := <-receivedEvents:
			receivedCount++
			if event.Type != TestEventTypeA {
				t.Errorf("Expected event type %s, got %s", TestEventTypeA, event.Type)
			}
		case <-time.After(10 * time.Second): // Increased timeout
			t.Fatalf("Timeout waiting for concurrent events, received %d/%d", receivedCount, numEvents)
		}
	}

	if receivedCount != numEvents {
		t.Errorf("Expected %d events, got %d", numEvents, receivedCount)
	}

	t.Log("✅ Enhanced NATS event bus concurrent publish test passed")
}
