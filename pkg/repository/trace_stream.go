package repository

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/medxops/trazr-otel/pkg/event"
)

// EventBusTraceStreamRepository implements TraceStreamRepository using the event bus.
type EventBusTraceStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan TraceEntry]struct{}
}

func NewEventBusTraceStreamRepository(bus event.EventBusInterface) *EventBusTraceStreamRepository {
	return &EventBusTraceStreamRepository{
		bus:         bus,
		subscribers: make(map[chan TraceEntry]struct{}),
	}
}

// SubscribeSpans subscribes to trace events via the event bus.
func (r *EventBusTraceStreamRepository) SubscribeSpans(ctx context.Context, filter TraceFilter) (<-chan TraceEntry, func(), <-chan struct{}, error) {
	msgCh := make(chan TraceEntry, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &traceEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	// Use Subscribe to get a Subscription that can be closed for proper cleanup
	subscription := r.bus.Subscribe(EventTypeTraceInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		close(done)
		if subscription != nil {
			_ = subscription.Close()
		}
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
	}

	return msgCh, cancel, established, nil
}

type traceEventHandler struct {
	filter TraceFilter
	msgCh  chan<- TraceEntry
	done   <-chan struct{}
}

func (h *traceEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	// Preferred Go-to-Go path: []TraceEntry
	if traces, ok := ev.Data.([]TraceEntry); ok {
		for _, te := range traces {
			if h.matchesFilter(te) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- te:
				}
			}
		}
		return nil
	}

	// NATS JSON path: []interface{} -> marshal->unmarshal
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var te TraceEntry
			if err := json.Unmarshal(b, &te); err != nil {
				continue
			}
			if h.matchesFilter(te) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- te:
				}
			}
		}
	}
	return nil
}

func (h *traceEventHandler) matchesFilter(te TraceEntry) bool {
	// Create FilterMatcher for consistent filtering
	matcher := &FilterMatcher{}

	// Check common base filters
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, te.ServiceName) {
		return false
	}

	// Extract attributes from JSONB fields
	attributes := matcher.ExtractAttributesFromJSON(te.Attributes)
	resourceAttributes := matcher.ExtractAttributesFromJSON(te.ResourceAttributes)

	// Check common advanced filters (attributes, text search)
	// Use span name as the "body" text for text search
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, te.Name) {
		return false
	}

	// Trace-specific filters
	if h.filter.TraceID != "" && te.TraceID != h.filter.TraceID {
		return false
	}
	if h.filter.SpanID != "" && te.SpanID != h.filter.SpanID {
		return false
	}
	if h.filter.Name != "" && !strings.Contains(strings.ToLower(te.Name), strings.ToLower(h.filter.Name)) {
		return false
	}
	// Schema filtering is handled at the event level in HandleEvent
	return true
}
