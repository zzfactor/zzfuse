package event

import (
	"context"
	"os"
	"testing"
)

func requireNATSIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run tests against a dedicated NATS server")
	}
}

func testNATSURL(t *testing.T) string {
	t.Helper()
	requireNATSIntegration(t)
	url := os.Getenv("NATS_URL")
	if url == "" {
		t.Fatal("set NATS_URL to a dedicated integration server")
	}
	return url
}

// TestEventBusFactory_CreateInMemory tests creating an in-memory event bus
func TestEventBusFactory_CreateInMemoryEventBus(t *testing.T) {
	// Set environment variable for in-memory configuration
	t.Setenv("EVENT_BUS_TYPE", "in-memory")
	t.Setenv("EVENT_BUS_BUFFER_SIZE", "500")
	t.Setenv("EVENT_BUS_MAX_WORKERS", "5")

	factory := NewEventBusFactory(EventBusTypeInMemory, "")

	bus, err := factory.CreateEventBus(context.Background())
	if err != nil {
		t.Fatalf("Failed to create in-memory event bus: %v", err)
	}

	if bus == nil {
		t.Fatal("Expected non-nil event bus")
	}

	// Verify it's the correct type
	if _, ok := bus.(*InMemoryEventBus); !ok {
		t.Errorf("Expected *EventBus, got %T", bus)
	}

	t.Log("✅ In-memory event bus created successfully")
}

// TestEventBusFactory_CreateNATS tests creating a NATS event bus
func TestEventBusFactory_CreateNATS(t *testing.T) {
	requireNATSIntegration(t)
	// Set environment variable for NATS configuration
	t.Setenv("EVENT_BUS_TYPE", "nats")
	natsURL := testNATSURL(t)
	t.Setenv("NATS_URL", natsURL)

	factory := NewEventBusFactory(EventBusTypeNATS, natsURL)

	bus, err := factory.CreateEventBus(context.Background())
	if err != nil {
		t.Skipf("NATS not available, skipping test: %v", err)
	}

	if bus == nil {
		t.Fatal("Expected non-nil event bus")
	}

	// Verify it's the correct type
	if _, ok := bus.(*NATSEventBus); !ok {
		t.Errorf("Expected *NATSEventBus, got %T", bus)
	}

	t.Log("✅ NATS event bus created successfully")
}

// TestEventBusFactory_InvalidType tests creating an invalid event bus type
func TestEventBusFactory_InvalidType(t *testing.T) {
	// Set environment variable for invalid configuration
	t.Setenv("EVENT_BUS_TYPE", "invalid")

	factory := NewEventBusFactory("invalid", "")

	_, err := factory.CreateEventBus(context.Background())
	if err == nil {
		t.Fatal("Expected error for invalid event bus type")
	}

	t.Log("✅ Invalid event bus type correctly rejected")
}
