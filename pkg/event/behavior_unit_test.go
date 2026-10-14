package event

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestInMemoryEventBusClientBehaviorUnit demonstrates that client code works correctly for in-memory event bus
func TestInMemoryEventBusClientBehaviorUnit(t *testing.T) {
	// Set environment variables for in-memory
	t.Setenv("EVENT_BUS_TYPE", "in-memory")

	// CLIENT CODE - IDENTICAL FOR BOTH TYPES
	// =========================================

	// 1. Create event bus (same code regardless of type)
	factory := NewEventBusFactory("", "")
	eventBus, err := factory.CreateEventBus(context.Background())
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}

	// 2. Register handlers (same code regardless of type)
	receivedEvents := make(chan Event, 10)
	eventBus.RegisterHandler(TestEventTypeA, &unitTestEventHandler{events: receivedEvents})
	eventBus.RegisterHandler(TestEventTypeB, &unitTestEventHandler{events: receivedEvents})

	// 3. Publish events (same code regardless of type)
	testEvents := []Event{
		{
			Type:      TestEventTypeA,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      "test event a data",
		},
		{
			Type:      TestEventTypeB,
			Timestamp: time.Now(),
			Schema:    "test",
			Data:      "test event b data",
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Publish events (same API regardless of type)
	for _, event := range testEvents {
		err := eventBus.Publish(ctx, event)
		if err != nil {
			t.Errorf("Failed to publish event: %v", err)
		}
	}

	// 4. Receive events (same code regardless of type)
	receivedCount := 0
	expectedCount := len(testEvents)

	for i := 0; i < expectedCount; i++ {
		select {
		case receivedEvent := <-receivedEvents:
			receivedCount++
			t.Logf("✅ Received event: Type=%s, Schema=%s", receivedEvent.Type, receivedEvent.Schema)
		case <-time.After(1 * time.Second):
			t.Errorf("Timeout waiting for event %d", i+1)
		}
	}

	if receivedCount != expectedCount {
		t.Errorf("Expected %d events, received %d", expectedCount, receivedCount)
		return
	}

	t.Log("✅ in-memory client behavior test passed - identical API")
}

// TestClientCodeExampleUnit demonstrates how client code remains identical for in-memory event bus
func TestClientCodeExampleUnit(t *testing.T) {
	// Example: In-Memory Event Bus
	t.Run("in-memory-example", func(t *testing.T) {
		t.Setenv("EVENT_BUS_TYPE", "in-memory")
		runIdenticalClientCodeUnit(t, "in-memory")
	})
}

// runIdenticalClientCodeUnit demonstrates that the same client code works for in-memory event bus
func runIdenticalClientCodeUnit(t *testing.T, busType string) {
	// CLIENT CODE - IDENTICAL FOR ALL EVENT BUS TYPES
	// ===============================================

	// 1. Create event bus using factory
	factory := NewEventBusFactory("", "")
	eventBus, err := factory.CreateEventBus(context.Background())
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}

	// 2. Register event handlers
	receivedEvents := make(chan Event, 10)
	eventBus.RegisterHandler(TestEventTypeA, &unitTestEventHandler{events: receivedEvents})

	// 3. Publish an event
	testEvent := Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      "hello world",
	}

	err = eventBus.Publish(context.Background(), testEvent)
	if err != nil {
		t.Fatalf("Failed to publish event: %v", err)
	}

	// 4. Receive the event
	select {
	case receivedEvent := <-receivedEvents:
		t.Logf("✅ Received event via %s: %v", busType, receivedEvent.Data)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}

	t.Logf("✅ %s client code example completed - identical API", busType)
}

// TestPerformanceComparisonUnit compares performance for in-memory event bus
func TestPerformanceComparisonUnit(t *testing.T) {
	tests := []struct {
		name    string
		busType string
	}{
		{
			name:    "in-memory performance",
			busType: "in-memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			t.Setenv("EVENT_BUS_TYPE", tt.busType)

			// Create event bus
			factory := NewEventBusFactory("", "")
			eventBus, err := factory.CreateEventBus(context.Background())
			if err != nil {
				t.Fatalf("Failed to create event bus: %v", err)
			}

			// Register handler
			receivedEvents := make(chan Event, 100)
			eventBus.RegisterHandler(TestEventTypeA, &unitTestEventHandler{events: receivedEvents})

			// Performance test: publish 10 events
			const numEvents = 10
			start := time.Now()

			for i := 0; i < numEvents; i++ {
				event := Event{
					Type:      TestEventTypeA,
					Timestamp: time.Now(),
					Schema:    "test",
					Data:      fmt.Sprintf("performance test event %d", i),
				}

				err := eventBus.Publish(context.Background(), event)
				if err != nil {
					t.Errorf("Failed to publish event %d: %v", i, err)
				}
			}

			duration := time.Since(start)
			eventsPerSecond := float64(numEvents) / duration.Seconds()

			t.Logf("✅ %s performance: %d events in %v (%.2f events/sec)",
				tt.name, numEvents, duration, eventsPerSecond)

			// Verify all events were received
			receivedCount := 0
			for i := 0; i < numEvents; i++ {
				select {
				case <-receivedEvents:
					receivedCount++
				case <-time.After(1 * time.Second):
					t.Errorf("Timeout waiting for event %d", i+1)
				}
			}

			if receivedCount != numEvents {
				t.Errorf("Expected %d events, received %d", numEvents, receivedCount)
			}
		})
	}
}

// unitTestEventHandler is a simple test event handler for unit tests
type unitTestEventHandler struct {
	events chan<- Event
}

func (h *unitTestEventHandler) HandleEvent(ctx context.Context, event Event) error {
	select {
	case h.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
