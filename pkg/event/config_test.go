package event

import (
	"testing"
	"time"
)

func TestGetEventBusConfig(t *testing.T) {
	// Test default configuration
	config := GetEventBusConfig()

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

	t.Log("✅ Default configuration loaded correctly")
}

func TestGetEventBusConfig_EnvironmentOverride(t *testing.T) {
	// Set environment variables
	t.Setenv("EVENT_BUS_TYPE", "nats")
	t.Setenv("NATS_URL", "nats://test-server:4222")
	t.Setenv("NATS_TIMEOUT", "10s")
	t.Setenv("EVENT_BUS_BUFFER_SIZE", "2000")
	t.Setenv("EVENT_BUS_MAX_WORKERS", "20")

	config := GetEventBusConfig()

	if config.Type != "nats" {
		t.Errorf("Expected type 'nats', got %s", config.Type)
	}

	if config.NATS.URL != "nats://test-server:4222" {
		t.Errorf("Expected NATS URL 'nats://test-server:4222', got %s", config.NATS.URL)
	}

	if config.NATS.Timeout != 10*time.Second {
		t.Errorf("Expected NATS timeout 10s, got %v", config.NATS.Timeout)
	}

	if config.InMemory.BufferSize != 2000 {
		t.Errorf("Expected buffer size 2000, got %d", config.InMemory.BufferSize)
	}

	if config.InMemory.MaxWorkers != 20 {
		t.Errorf("Expected max workers 20, got %d", config.InMemory.MaxWorkers)
	}

	t.Log("✅ Environment variable overrides work correctly")
}

func TestValidateEventBusConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *EventBusConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "valid in-memory config",
			config: &EventBusConfig{
				Type: "in-memory",
				InMemory: InMemoryConfig{
					BufferSize:         1000,
					MaxWorkers:         10,
					BackpressurePolicy: "drop_newest",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid in-memory buffer size",
			config: &EventBusConfig{
				Type: "in-memory",
				InMemory: InMemoryConfig{
					BufferSize: 0,
					MaxWorkers: 10,
				},
			},
			wantErr: true,
		},
		{
			name: "valid nats config",
			config: &EventBusConfig{
				Type: "nats",
				NATS: NATSConfig{
					URL:           "nats://localhost:4222",
					Timeout:       5 * time.Second,
					MaxReconnects: 60,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid nats url",
			config: &EventBusConfig{
				Type: "nats",
				NATS: NATSConfig{
					URL:           "",
					Timeout:       5 * time.Second,
					MaxReconnects: 60,
				},
			},
			wantErr: true,
		},
		{
			name: "unsupported type",
			config: &EventBusConfig{
				Type: "unsupported",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEventBusConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEventBusConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}

	t.Log("✅ Configuration validation works correctly")
}
