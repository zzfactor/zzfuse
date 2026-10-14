package repository

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/medxops/trazr-otel/pkg/event"
)

// ProfileFilter represents the filter criteria for profile streaming subscriptions.
type ProfileFilter struct {
	CommonBaseFilters
	CommonAdvancedFilters

	// Profile-specific streaming filters
	ProfileType string
}

type ProfileStreamRepository interface {
	SubscribeProfiles(ctx context.Context, filter ProfileFilter) (<-chan ProfileEntryResponse, func(), <-chan struct{}, error)
}

type profileStreamRepository struct {
	// Event bus used for streaming
	bus event.EventBusInterface
}

func NewProfileStream(bus event.EventBusInterface) ProfileStreamRepository {
	return &profileStreamRepository{bus: bus}
}

func (r *profileStreamRepository) SubscribeProfiles(ctx context.Context, filter ProfileFilter) (<-chan ProfileEntryResponse, func(), <-chan struct{}, error) {
	ebRepo := newProfileStream(r.bus)
	return ebRepo.SubscribeProfiles(ctx, filter)
}

// EventBusProfileStreamRepository implements ProfileStreamRepository using the event bus.
type EventBusProfileStreamRepository struct {
	bus         event.EventBusInterface
	subscribeMu sync.Mutex
	subscribers map[chan ProfileEntryResponse]struct{}
}

// newProfileStream creates a new EventBusProfileStreamRepository used by the public wrapper.
func newProfileStream(bus event.EventBusInterface) *EventBusProfileStreamRepository {
	return &EventBusProfileStreamRepository{
		bus:         bus,
		subscribers: make(map[chan ProfileEntryResponse]struct{}),
	}
}

// SubscribeProfiles subscribes to profile events via the event bus.
func (r *EventBusProfileStreamRepository) SubscribeProfiles(ctx context.Context, filter ProfileFilter) (<-chan ProfileEntryResponse, func(), <-chan struct{}, error) {
	msgCh := make(chan ProfileEntryResponse, streamBufferSize)
	done := make(chan struct{})
	established := make(chan struct{})

	handler := &profileEventHandler{
		filter: filter,
		msgCh:  msgCh,
		done:   done,
	}

	r.subscribeMu.Lock()
	r.subscribers[msgCh] = struct{}{}
	r.subscribeMu.Unlock()

	// Use Subscribe to get a Subscription that can be closed for proper cleanup
	subscription := r.bus.Subscribe(EventTypeProfileInserted.ToEventType(), handler)
	close(established)

	cancel := func() {
		// Signal handler to stop sending BEFORE unregistering.
		close(done)
		// Unregister the handler from the event bus
		if subscription != nil {
			_ = subscription.Close()
		}
		// Remove from local subscriber tracking
		r.subscribeMu.Lock()
		delete(r.subscribers, msgCh)
		r.subscribeMu.Unlock()
		// Do NOT close msgCh here: handler may still be in-flight during Close().
	}

	return msgCh, cancel, established, nil
}

// profileEventHandler implements event.EventHandler for profile events.
type profileEventHandler struct {
	filter ProfileFilter
	msgCh  chan<- ProfileEntryResponse
	done   <-chan struct{}
}

func (h *profileEventHandler) HandleEvent(ctx context.Context, ev event.Event) error {
	select {
	case <-h.done:
		return nil
	default:
	}

	// Check schema filter first - if schema doesn't match, skip processing
	if h.filter.Schema != "" && ev.Schema != h.filter.Schema {
		return nil
	}

	// Handle slice of ProfileEntryResponse (standardized format for Go-to-Go communication)
	if profiles, ok := ev.Data.([]ProfileEntryResponse); ok {
		for _, resp := range profiles {
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
			var resp ProfileEntryResponse
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

// matchesFilter checks if the profile entry matches the filter criteria
func (h *profileEventHandler) matchesFilter(resp ProfileEntryResponse) bool {
	// Create filter matcher for consistent filtering
	matcher := NewFilterMatcher()

	// Check common base filters (service name, etc.)
	serviceName := ""
	if resp.ServiceName != nil {
		serviceName = *resp.ServiceName
	}
	if !matcher.MatchesCommonBaseFilters(h.filter.CommonBaseFilters, serviceName) {
		return false
	}

	// Extract attributes and resource attributes from interface{} fields
	attributes := h.extractAttributesFromInterface(resp.Attributes)
	resourceAttributes := h.extractAttributesFromInterface(resp.ResourceAttributes)

	// Extract profile type text for text search
	profileTypeText := ""
	if resp.ProfileType != nil {
		profileTypeText = *resp.ProfileType
	}

	// Check common advanced filters (attributes, text search)
	if !matcher.MatchesCommonAdvancedFilters(h.filter.CommonAdvancedFilters, attributes, resourceAttributes, profileTypeText) {
		return false
	}

	// Check profile-specific filters
	if h.filter.ProfileType != "" {
		if resp.ProfileType == nil || *resp.ProfileType != h.filter.ProfileType {
			return false
		}
	}

	// Schema filtering is handled at the event level in HandleEvent
	return true
}

// extractAttributesFromInterface safely extracts attributes from interface{} fields
func (h *profileEventHandler) extractAttributesFromInterface(data interface{}) map[string]interface{} {
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
