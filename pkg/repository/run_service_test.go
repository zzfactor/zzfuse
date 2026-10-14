package repository

import (
	"context"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/stretchr/testify/assert"
)

// mockEventHandler captures events for testing
type mockEventHandler struct {
	events []event.Event
}

func (h *mockEventHandler) HandleEvent(ctx context.Context, e event.Event) error {
	h.events = append(h.events, e)
	return nil
}

func TestRunServiceEventPublishing(t *testing.T) {
	// This test verifies that the run service can be created with an event bus
	// In a real integration test, you would set up a test database and verify event publishing

	// Create a mock event bus using the factory
	factory := event.NewEventBusFactory("", "")
	eventBus, err := factory.CreateEventBus(context.Background())
	if err != nil {
		t.Fatalf("Failed to create event bus: %v", err)
	}
	defer func() {
		if closer, ok := eventBus.(interface{ Close() error }); ok {
			closer.Close()
		}
	}()

	// Test that event bus is properly created
	assert.NotNil(t, eventBus)

	// Test that we can register handlers for run events
	handler := &mockEventHandler{}
	eventBus.RegisterHandler(EventTypeRunCreated.ToEventType(), handler)
	eventBus.RegisterHandler(EventTypeRunUpdated.ToEventType(), handler)

	t.Log("Run service event publishing test setup completed")
}

func TestRunEntryResponseConversion(t *testing.T) {
	// Test the ToRunEntryResponse function
	now := time.Now()
	run := &dbconfig.Run{
		ID:               123,
		SessionID:        stringPtr("session-123"),
		ServiceName:      "test-service",
		ScopeName:        stringPtr("test-scope"),
		Name:             "test-run",
		StartTimestampNs: now.UnixNano(),
		EndTimestampNs:   int64Ptr(now.Add(time.Hour).UnixNano()),
		TraceID:          stringPtr("trace-123"),
		ProcessorID:      int64Ptr(456),
		RunType:          int16Ptr(1),
		Metadata: map[string]interface{}{
			"SPAN_COUNT": int64(100),
			"LOG_COUNT":  int64(50),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	response := ToRunEntryResponse(run)

	assert.Equal(t, int64(123), response.ID)
	assert.Equal(t, "session-123", *response.SessionID)
	assert.Equal(t, "test-service", response.ServiceName)
	assert.Equal(t, "test-scope", *response.ScopeName)
	assert.Equal(t, "test-run", response.Name)
	assert.Equal(t, now.UnixNano(), response.StartTimestampNs)
	assert.Equal(t, now.Add(time.Hour).UnixNano(), *response.EndTimestampNs)
	assert.Equal(t, "trace-123", *response.TraceID)
	assert.Equal(t, int64(456), *response.ProcessorID)
	assert.Equal(t, int16(1), *response.RunType)
	assert.Equal(t, map[string]interface{}{
		"SPAN_COUNT": int64(100),
		"LOG_COUNT":  int64(50),
	}, response.Metadata)
	assert.Equal(t, now.Format("2006-01-02T15:04:05.999999999Z07:00"), response.CreatedAt)
	assert.Equal(t, now.Format("2006-01-02T15:04:05.999999999Z07:00"), response.UpdatedAt)
}
