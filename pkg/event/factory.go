package event

import (
	"context"
	"fmt"
)

// EventBusType represents the type of event bus to use
type EventBusType string

const (
	EventBusTypeInMemory EventBusType = "in-memory"
	EventBusTypeNATS     EventBusType = "nats"
)

// EventBusFactory creates event buses based on configuration
type EventBusFactory struct {
	busType EventBusType
	natsURL string
}

// NewEventBusFactory creates a new event bus factory
func NewEventBusFactory(busType EventBusType, natsURL string) *EventBusFactory {
	return &EventBusFactory{
		busType: busType,
		natsURL: natsURL,
	}
}

// CreateEventBus creates an event bus based on the configuration
func (f *EventBusFactory) CreateEventBus(ctx context.Context) (EventBusInterface, error) {
	config := GetEventBusConfig()

	// Validate configuration
	if err := ValidateEventBusConfig(config); err != nil {
		return nil, fmt.Errorf("invalid event bus configuration: %w", err)
	}

	switch config.Type {
	case "in-memory":
		return NewInMemoryEventBusWithOptions(
			config.InMemory.BufferSize,
			config.InMemory.MaxWorkers,
			config.InMemory.BackpressurePolicy,
			config.InMemory.EnqueueTimeout,
		), nil
	case "nats":
		return NewNATSEventBus(config.NATS.URL)
	default:
		return nil, fmt.Errorf("unsupported event bus type: %s", config.Type)
	}
}

// EventBusInterface defines the common interface for all event bus implementations
type EventBusInterface interface {
	RegisterHandler(eventType EventType, handler EventHandler)
	// Subscribe registers a handler and returns a Subscription that can be closed to unregister.
	Subscribe(eventType EventType, handler EventHandler) Subscription
	// UnregisterHandler removes a handler by its ID for a given event type.
	UnregisterHandler(eventType EventType, handlerID string) error
	Publish(ctx context.Context, event Event) error
	// Close gracefully shuts down the event bus.
	Close() error
}

// Subscription represents a registration that can be closed to unregister the handler.
type Subscription interface {
	Close() error
}

// Ensure all event bus types implement the interface
var _ EventBusInterface = (*InMemoryEventBus)(nil)
var _ EventBusInterface = (*NATSEventBus)(nil)
