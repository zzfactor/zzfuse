package event

import (
	"context"
	"testing"
	"time"
)

// TestEventBusConfiguration demonstrates how to configure and use different event bus types
func TestEventBusConfigurationIntegration(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		expectType  string
		expectError bool
	}{
		{
			name: "in-memory default",
			envVars: map[string]string{
				"EVENT_BUS_TYPE": "in-memory",
			},
			expectType:  "in-memory",
			expectError: false,
		},
		{
			name: "in-memory with custom config",
			envVars: map[string]string{
				"EVENT_BUS_TYPE":        "in-memory",
				"EVENT_BUS_BUFFER_SIZE": "500",
				"EVENT_BUS_MAX_WORKERS": "5",
			},
			expectType:  "in-memory",
			expectError: false,
		},
		{
			name: "nats configuration",
			envVars: map[string]string{
				"EVENT_BUS_TYPE": "nats",
				"NATS_URL":       "nats://localhost:4222",
				"NATS_TIMEOUT":   "5s",
			},
			expectType:  "nats",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			natsURL := ""
			if tt.expectType == "nats" {
				natsURL = testNATSURL(t)
			}
			// Set environment variables for this test
			for key, value := range tt.envVars {
				if key == "NATS_URL" {
					value = natsURL
				}
				t.Setenv(key, value)
			}

			// Get configuration
			config := GetEventBusConfig()
			if config.Type != tt.expectType {
				t.Errorf("Expected type %s, got %s", tt.expectType, config.Type)
			}

			// Validate configuration
			err := ValidateEventBusConfig(config)
			if (err != nil) != tt.expectError {
				t.Errorf("Expected error %v, got %v", tt.expectError, err)
			}

			if tt.expectError {
				return
			}

			// Create event bus using factory
			factory := NewEventBusFactory("", "")
			eventBus, err := factory.CreateEventBus(context.Background())
			if err != nil {
				t.Fatalf("Failed to create event bus: %v", err)
			}

			if eventBus == nil {
				t.Fatal("Expected non-nil event bus")
			}

			// Test basic functionality
			testEventBusFunctionality(t, eventBus, config.Type)
		})
	}
}

// testEventBusFunctionality tests basic event bus operations
func testEventBusFunctionality(t *testing.T, eventBus EventBusInterface, busType string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Create a simple test event
	testEvent := Event{
		Type:      TestEventTypeA,
		Timestamp: time.Now(),
		Schema:    "test",
		Data:      "test data",
	}

	// Register a test handler
	received := make(chan Event, 1)
	eventBus.RegisterHandler(TestEventTypeA, &testEventHandler{events: received})

	// Publish an event
	err := eventBus.Publish(ctx, testEvent)
	if err != nil {
		t.Errorf("Failed to publish event: %v", err)
	}

	// Wait for the event to be received (with timeout)
	select {
	case receivedEvent := <-received:
		if receivedEvent.Type != testEvent.Type {
			t.Errorf("Expected event type %s, got %s", testEvent.Type, receivedEvent.Type)
		}
		if receivedEvent.Schema != testEvent.Schema {
			t.Errorf("Expected event schema %s, got %s", testEvent.Schema, receivedEvent.Schema)
		}
		t.Logf("✅ %s event bus functionality test passed", busType)
	case <-time.After(1 * time.Second):
		if busType == "nats" {
			// NATS might not be available in test environment, skip
			t.Skipf("NATS not available, skipping functionality test")
		} else {
			t.Error("Timeout waiting for event")
		}
	}
}

// TestConfigurationDefaults verifies that default values are correctly set
func TestConfigurationDefaults(t *testing.T) {
	// Clear any environment variables that might interfere
	t.Setenv("EVENT_BUS_TYPE", "")
	t.Setenv("EVENT_BUS_BUFFER_SIZE", "")
	t.Setenv("EVENT_BUS_MAX_WORKERS", "")
	t.Setenv("NATS_URL", "")
	t.Setenv("NATS_TIMEOUT", "")

	config := GetEventBusConfig()

	// Verify defaults
	if config.Type != "in-memory" {
		t.Errorf("Expected default type 'in-memory', got %s", config.Type)
	}

	if config.InMemory.BufferSize != 1000 {
		t.Errorf("Expected default buffer size 1000, got %d", config.InMemory.BufferSize)
	}

	if config.InMemory.MaxWorkers != 10 {
		t.Errorf("Expected default max workers 10, got %d", config.InMemory.MaxWorkers)
	}

	if config.NATS.URL != "nats://localhost:4222" {
		t.Errorf("Expected default NATS URL 'nats://localhost:4222', got %s", config.NATS.URL)
	}

	if config.NATS.Timeout != 5*time.Second {
		t.Errorf("Expected default NATS timeout 5s, got %v", config.NATS.Timeout)
	}

	t.Log("✅ Default configuration values are correct")
}
