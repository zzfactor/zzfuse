package repository

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/medxops/trazr-otel/pkg/event"
)

// Notifier-based implementation removed; RunStream now uses only the EventBus-based repository below.

// EventBusRunStreamRepository implements RunStreamRepository using event bus
type EventBusRunStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan RunEntryResponse]struct{}
}

// NewEventBusRunStreamRepository creates a new event bus run stream repository
func NewEventBusRunStreamRepository(bus event.EventBusInterface) RunStreamRepository {
	return &EventBusRunStreamRepository{
		bus:         bus,
		subscribers: make(map[chan RunEntryResponse]struct{}),
	}
}

func (r *EventBusRunStreamRepository) SubscribeRuns(ctx context.Context, filter RunFilter) (<-chan RunEntryResponse, func(), <-chan struct{}, error) {
	msgCh := make(chan RunEntryResponse, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &runEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	// Use Subscribe to get Subscriptions that can be closed for proper cleanup
	subCreated := r.bus.Subscribe(EventTypeRunCreated.ToEventType(), handler)
	subUpdated := r.bus.Subscribe(EventTypeRunUpdated.ToEventType(), handler)
	close(established)

	cancel := func() {
		// Signal handler to stop before any potential channel operations
		close(done)
		// Unregister handlers from the event bus
		if subCreated != nil {
			_ = subCreated.Close()
		}
		if subUpdated != nil {
			_ = subUpdated.Close()
		}
		// Remove from tracking; don't close msgCh here to avoid send-on-closed-channel panics
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
	}

	return msgCh, cancel, established, nil
}

// runEventHandler implements event.EventHandler for run events.
type runEventHandler struct {
	filter RunFilter
	msgCh  chan<- RunEntryResponse
	done   <-chan struct{}
}

func (h *runEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	// Handle both single RunEntryResponse and slice of RunEntryResponse
	if singleRun, ok := ev.Data.(RunEntryResponse); ok {
		if h.matchesFilter(singleRun) {
			select {
			case <-h.done:
				return nil
			case h.msgCh <- singleRun:
			}
		}
		return nil
	}

	if runSlice, ok := ev.Data.([]RunEntryResponse); ok {
		for _, resp := range runSlice {
			if h.matchesFilter(resp) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- resp:
				}
			}
		}
		return nil
	}

	// NATS path: ev.Data may be map[string]interface{} or []interface{}
	if m, ok := ev.Data.(map[string]interface{}); ok {
		b, err := json.Marshal(m)
		if err == nil {
			var resp RunEntryResponse
			if json.Unmarshal(b, &resp) == nil && h.matchesFilter(resp) {
				select {
				case <-h.done:
					return nil
				case h.msgCh <- resp:
				}
			}
		}
		return nil
	}
	if rawSlice, ok := ev.Data.([]interface{}); ok {
		for _, raw := range rawSlice {
			b, err := json.Marshal(raw)
			if err != nil {
				continue
			}
			var resp RunEntryResponse
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
		return nil
	}

	return nil
}

// matchesFilter checks if the run entry matches the filter criteria
func (h *runEventHandler) matchesFilter(resp RunEntryResponse) bool {
	// Create FilterMatcher for consistent filtering
	matcher := &FilterMatcher{}

	// Check common base filters (service name, etc.)
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, resp.ServiceName) {
		return false
	}

	// Runs have metadata field instead of separate attributes/resource_attributes
	// Use metadata as attributes for advanced filtering
	attributes := resp.Metadata
	if attributes == nil {
		attributes = make(map[string]interface{})
	}
	emptyResourceAttributes := make(map[string]interface{})

	// Use run name as the "body" text for text search
	runNameText := resp.Name

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, emptyResourceAttributes, runNameText) {
		return false
	}

	// Run-specific filters
	if h.filter.Name != "" && resp.Name != h.filter.Name {
		return false
	}
	if h.filter.SessionID != "" && (resp.SessionID == nil || *resp.SessionID != h.filter.SessionID) {
		return false
	}
	if h.filter.RunType != nil && (resp.RunType == nil || *resp.RunType != *h.filter.RunType) {
		return false
	}
	// For ActiveOnly, we still allow terminal events (with EndTimestampNs) to pass
	// so clients can detect closure. Non-terminal updates for ended runs are filtered elsewhere by producers.
	// Schema filtering is handled at the event level in HandleEvent
	return true
}
