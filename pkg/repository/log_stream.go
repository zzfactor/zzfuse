package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/medxops/trazr-otel/pkg/event"
)

// LogFilter represents the filter criteria for log streaming subscriptions.
type LogFilter struct {
	CommonBaseFilters
	CommonAdvancedFilters

	// Log-specific streaming filters
	Severity string
}

type LogStreamRepository interface {
	SubscribeLogs(ctx context.Context, filter LogFilter) (<-chan LogEntryResponse, func(), <-chan struct{}, error)
}

type logStreamRepository struct {
	// Event bus used for streaming
	bus event.EventBusInterface
}

func NewLogStream(bus event.EventBusInterface) LogStreamRepository {
	return &logStreamRepository{bus: bus}
}

func (r *logStreamRepository) SubscribeLogs(ctx context.Context, filter LogFilter) (<-chan LogEntryResponse, func(), <-chan struct{}, error) {
	ebRepo := newLogStream(r.bus)
	return ebRepo.SubscribeLogs(ctx, filter)
}

type TraceFilter struct {
	CommonBaseFilters
	CommonAdvancedFilters

	// Trace-specific streaming filters
	TraceID string
	SpanID  string
	Name    string
}

type TraceStreamRepository interface {
	SubscribeSpans(ctx context.Context, filter TraceFilter) (<-chan TraceEntry, func(), <-chan struct{}, error)
}

type traceStreamRepository struct {
	bus event.EventBusInterface
}

// NewTraceStream constructs a trace repository that uses the Event Bus path.
func NewTraceStream(bus event.EventBusInterface) TraceStreamRepository {
	return &traceStreamRepository{bus: bus}
}

func (r *traceStreamRepository) SubscribeSpans(ctx context.Context, filter TraceFilter) (<-chan TraceEntry, func(), <-chan struct{}, error) {
	if r.bus == nil {
		return nil, func() {}, nil, fmt.Errorf("event bus is required for trace subscriptions; notifier-based path removed")
	}
	ebRepo := NewEventBusTraceStreamRepository(r.bus)
	return ebRepo.SubscribeSpans(ctx, filter)
}

// EventBusLogStreamRepository implements LogStreamRepository using the event bus.
type EventBusLogStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan LogEntryResponse]struct{}
}

// newLogStream creates a new EventBusLogStreamRepository used by the public wrapper.
func newLogStream(bus event.EventBusInterface) *EventBusLogStreamRepository {
	return &EventBusLogStreamRepository{
		bus:         bus,
		subscribers: make(map[chan LogEntryResponse]struct{}),
	}
}

// SubscribeLogs subscribes to log events via the event bus.
func (r *EventBusLogStreamRepository) SubscribeLogs(ctx context.Context, filter LogFilter) (<-chan LogEntryResponse, func(), <-chan struct{}, error) {
	msgCh := make(chan LogEntryResponse, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &logEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	// Use Subscribe instead of RegisterHandler to get a Subscription that can be closed
	subscription := r.bus.Subscribe(EventTypeLogInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		// Signal handler to stop sending BEFORE unregistering.
		close(done)
		// Unregister the handler from the event bus to allow garbage collection
		if subscription != nil {
			_ = subscription.Close()
		}
		// Remove from local subscriber tracking
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
		// Do NOT close msgCh here: the handler may still be in-flight during Close().
		// Leaving it open avoids send-on-closed-channel panics. Consumers exit via context.
	}

	return msgCh, cancel, established, nil
}

// logEventHandler implements event.EventHandler for log events.
type logEventHandler struct {
	filter LogFilter
	msgCh  chan<- LogEntryResponse
	done   <-chan struct{}
}

func (h *logEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	// Handle slice of LogEntryResponse (standardized format for Go-to-Go communication)
	if logs, ok := ev.Data.([]LogEntryResponse); ok {
		for _, resp := range logs {
			if h.matchesFilter(resp) {
				// Ensure we never send after cancellation; prefer done if closed.
				select {
				case <-h.done:
					return nil
				case h.msgCh <- resp:
				}
			}
		}
		return nil
	}

	// NATS path: ev.Data will be []map[string]interface{} after JSON unmarshal
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			// Marshal then unmarshal into typed struct for safety
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var resp LogEntryResponse
			if err := json.Unmarshal(b, &resp); err != nil {
				continue
			}
			if h.matchesFilter(resp) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- resp:
				}
			}
		}
	}
	return nil
}

// matchesFilter checks if the log entry matches the filter criteria
func (h *logEventHandler) matchesFilter(resp LogEntryResponse) bool {
	// Create filter matcher for consistent filtering
	matcher := NewFilterMatcher()

	// Check common base filters (service name, etc.)
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, resp.ServiceName) {
		return false
	}

	// Extract attributes and resource attributes from interface{} fields
	attributes := h.extractAttributesFromInterface(resp.Attributes)
	resourceAttributes := h.extractAttributesFromInterface(resp.ResourceAttributes)

	// Extract body text for text search
	bodyText := resp.Body

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, bodyText) {
		return false
	}

	// Check log-specific filters
	if h.filter.Severity != "" {
		if strings.ToLower(resp.SeverityText) != strings.ToLower(h.filter.Severity) {
			return false
		}
	}

	// Schema filtering is handled at the event level in HandleEvent
	return true
}

// extractAttributesFromInterface safely extracts attributes from interface{} fields
func (h *logEventHandler) extractAttributesFromInterface(data interface{}) map[string]interface{} {
	if data == nil {
		return make(map[string]interface{})
	}

	if attrs, ok := data.(map[string]interface{}); ok {
		return attrs
	}

	// If it's not already a map, try to convert via JSON
	if bytes, err := json.Marshal(data); err == nil {
		var attrs map[string]interface{}
		if json.Unmarshal(bytes, &attrs) == nil {
			return attrs
		}
	}

	return make(map[string]interface{})
}
