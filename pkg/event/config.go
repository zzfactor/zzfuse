package event

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// EventBusConfig holds the configuration for the event bus
type EventBusConfig struct {
	Type     string         `json:"type"`
	NATS     NATSConfig     `json:"nats"`
	InMemory InMemoryConfig `json:"in_memory"`

	// Performance tuning
	WorkerPoolSize    int           `json:"worker_pool_size"`
	BatchSize         int           `json:"batch_size"`
	BatchTimeout      time.Duration `json:"batch_timeout"`
	MaxEventSize      int           `json:"max_event_size"`
	EnableCompression bool          `json:"enable_compression"`

	// Circuit breaker
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
}

// NATSConfig holds NATS-specific configuration
type NATSConfig struct {
	URL           string        `json:"url"`
	Timeout       time.Duration `json:"timeout"`
	MaxReconnects int           `json:"max_reconnects"`
	ReconnectWait time.Duration `json:"reconnect_wait"`
	PingInterval  time.Duration `json:"ping_interval"`
	PingMaxOut    int           `json:"ping_max_out"`
}

// InMemoryConfig holds in-memory event bus configuration
type InMemoryConfig struct {
	BufferSize int           `json:"buffer_size"`
	MaxWorkers int           `json:"max_workers"`
	// Backpressure policy for enqueue when queue is full: drop_newest, drop_oldest, block
	BackpressurePolicy string        `json:"backpressure_policy"`
	EnqueueTimeout     time.Duration `json:"enqueue_timeout"`
}

// GetEventBusConfig returns the event bus configuration from environment variables
// Follows the same pattern as GetDSN() in pkg/repository/database_util.go
func GetEventBusConfig() *EventBusConfig {
	return &EventBusConfig{
		Type: getEnvString("EVENT_BUS_TYPE", "in-memory"),
		NATS: NATSConfig{
			URL:           getEnvString("NATS_URL", "nats://localhost:4222"),
			Timeout:       getEnvDuration("NATS_TIMEOUT", 5*time.Second),
			MaxReconnects: getEnvInt("NATS_MAX_RECONNECTS", 60),
			ReconnectWait: getEnvDuration("NATS_RECONNECT_WAIT", 1*time.Second),
			PingInterval:  getEnvDuration("NATS_PING_INTERVAL", 30*time.Second),
			PingMaxOut:    getEnvInt("NATS_PING_MAX_OUT", 3),
		},
		InMemory: InMemoryConfig{
			BufferSize: getEnvInt("EVENT_BUS_BUFFER_SIZE", 1000),
			MaxWorkers: getEnvInt("EVENT_BUS_MAX_WORKERS", 10),
			BackpressurePolicy: getEnvString("EVENT_BUS_BACKPRESSURE_POLICY", "drop_newest"),
			EnqueueTimeout:     getEnvDuration("EVENT_BUS_ENQUEUE_TIMEOUT", 100*time.Millisecond),
		},

		// Performance tuning
		WorkerPoolSize:    getEnvInt("EVENT_BUS_WORKER_POOL_SIZE", 10),
		BatchSize:         getEnvInt("EVENT_BUS_BATCH_SIZE", 100),
		BatchTimeout:      getEnvDuration("EVENT_BUS_BATCH_TIMEOUT", 10*time.Millisecond),
		MaxEventSize:      getEnvInt("EVENT_BUS_MAX_EVENT_SIZE", 1024*1024), // 1MB
		EnableCompression: getEnvString("EVENT_BUS_ENABLE_COMPRESSION", "false") == "true",

		// Circuit breaker
		CircuitBreaker: CircuitBreakerConfig{
			FailureThreshold: int64(getEnvInt("EVENT_BUS_CIRCUIT_BREAKER_FAILURE_THRESHOLD", 5)),
			RecoveryTimeout:  getEnvDuration("EVENT_BUS_CIRCUIT_BREAKER_RECOVERY_TIMEOUT", 30*time.Second),
			HalfOpenMaxCalls: int64(getEnvInt("EVENT_BUS_CIRCUIT_BREAKER_HALF_OPEN_MAX_CALLS", 3)),
			WindowSize:       getEnvDuration("EVENT_BUS_CIRCUIT_BREAKER_WINDOW_SIZE", 60*time.Second),
			MinRequestCount:  int64(getEnvInt("EVENT_BUS_CIRCUIT_BREAKER_MIN_REQUEST_COUNT", 3)),
		},
	}
}

// getEnvString gets an environment variable or returns the default value
func getEnvString(key, def string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return def
}

// getEnvInt gets an environment variable as int or returns the default value
func getEnvInt(key string, def int) int {
	if val := os.Getenv(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return def
}

// getEnvDuration gets an environment variable as duration or returns the default value
func getEnvDuration(key string, def time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			return duration
		}
	}
	return def
}

// ValidateEventBusConfig validates the event bus configuration
func ValidateEventBusConfig(config *EventBusConfig) error {
	if config == nil {
		return fmt.Errorf("event bus config is nil")
	}

	switch config.Type {
	case "in-memory":
		if config.InMemory.BufferSize <= 0 {
			return fmt.Errorf("in-memory buffer size must be positive, got %d", config.InMemory.BufferSize)
		}
		if config.InMemory.MaxWorkers <= 0 {
			return fmt.Errorf("in-memory max workers must be positive, got %d", config.InMemory.MaxWorkers)
		}
		switch config.InMemory.BackpressurePolicy {
		case "drop_newest", "drop_oldest", "block":
		default:
			return fmt.Errorf("invalid backpressure policy: %s", config.InMemory.BackpressurePolicy)
		}
	case "nats":
		if config.NATS.URL == "" {
			return fmt.Errorf("NATS URL is required when type is 'nats'")
		}
		if config.NATS.Timeout <= 0 {
			return fmt.Errorf("NATS timeout must be positive, got %v", config.NATS.Timeout)
		}
		if config.NATS.MaxReconnects < 0 {
			return fmt.Errorf("NATS max reconnects cannot be negative, got %d", config.NATS.MaxReconnects)
		}
	default:
		return fmt.Errorf("unsupported event bus type: %s (supported: in-memory, nats)", config.Type)
	}

	return nil
}
