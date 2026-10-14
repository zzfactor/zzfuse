package event

import (
	"context"
	"fmt"
	"sync"
	"time"

	m "github.com/medxops/trazr-otel/pkg/metrics"
)

// EventType represents the type of event in the system.
// This is a generic type that can be used by any domain.
type EventType string

// Standard event types for OTEL signals
const (
	// Log events
	EventLogInserted EventType = "log.inserted"

	// Trace events
	EventTraceInserted EventType = "trace.inserted"

	// Metric events
	EventMetricGaugeInserted              EventType = "metric.gauge.inserted"
	EventMetricSumInserted                EventType = "metric.sum.inserted"
	EventMetricHistogramInserted          EventType = "metric.histogram.inserted"
	EventMetricSummaryInserted            EventType = "metric.summary.inserted"
	EventMetricExponentialHistogramInserted EventType = "metric.exponential_histogram.inserted"

	// Profile events
	EventProfileInserted EventType = "profile.inserted"
)

// Event represents a generic event in the system.
type Event struct {
	Type      EventType   `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Schema    string      `json:"schema"`
	Data      interface{} `json:"data"`
	IDs       []string    `json:"ids,omitempty"`
}

// OptimizedEvent is used for high-throughput scenarios.
type OptimizedEvent struct {
	Type      EventType `json:"type"`
	Timestamp int64     `json:"timestamp"` // Unix timestamp in nanoseconds
	Schema    string    `json:"schema"`
	Count     int       `json:"count"` // Number of records inserted
	IDs       []string  `json:"ids"`   // Only IDs, no full data
	Summary   struct {
		ServiceNames []string `json:"service_names,omitempty"`
		TimeRange    struct {
			Earliest int64 `json:"earliest"`
			Latest   int64 `json:"latest"`
		} `json:"time_range"`
	} `json:"summary,omitempty"`
}

// EventHandler defines the interface for handling events.
type EventHandler interface {
	HandleEvent(ctx context.Context, event Event) error
}

// InMemoryEventBus is the central event distribution system using in-memory storage.
type InMemoryEventBus struct {
	// handler registry by event type, each handler gets a unique ID
	handlers map[EventType]map[string]EventHandler
	mu       sync.RWMutex

	// worker pool & queue
	bufferSize int
	maxWorkers int
	queue      chan func()
	workersWg  sync.WaitGroup
	done       chan struct{}
	policy         string
	enqueueTimeout time.Duration

	// Cleanup management
	closing   bool
	closeMu   sync.RWMutex
	activeOps sync.WaitGroup

	// metrics (simple atomics/counters for now)
	// labels: event_type, bus_type
	busType string

	// id generator
	idSeq uint64
}

// NewInMemoryEventBus creates a new InMemoryEventBus with the given buffer size and worker count.
// Backward-compatible constructor used by existing tests/callers. Uses sensible defaults.
func NewInMemoryEventBus(bufferSize, maxWorkers int) *InMemoryEventBus {
    return NewInMemoryEventBusWithOptions(bufferSize, maxWorkers, "drop_newest", 100*time.Millisecond)
}

// NewInMemoryEventBusWithOptions creates a new InMemoryEventBus with backpressure options.
func NewInMemoryEventBusWithOptions(bufferSize, maxWorkers int, policy string, enqueueTimeout time.Duration) *InMemoryEventBus {
    if bufferSize <= 0 {
        bufferSize = 1024
    }
    if maxWorkers <= 0 {
        maxWorkers = 4
    }
    if policy == "" {
        policy = "drop_newest"
    }
    eb := &InMemoryEventBus{
        handlers:   make(map[EventType]map[string]EventHandler),
        bufferSize: bufferSize,
        maxWorkers: maxWorkers,
        queue:      make(chan func(), bufferSize),
        done:       make(chan struct{}),
        policy:     policy,
        enqueueTimeout: enqueueTimeout,
        closing:    false,
        busType:    "in_memory",
    }
    eb.startWorkers()
    // metrics: set total workers gauge once
    m.EventBusTotalWorkers.WithLabelValues(eb.busType).Set(float64(maxWorkers))
    return eb
}

// RegisterHandler registers an event handler for a specific event type.
func (eb *InMemoryEventBus) RegisterHandler(eventType EventType, handler EventHandler) {
	_ = eb.Subscribe(eventType, handler) // ignore subscription handle
}

// Subscribe registers and returns a subscription that can be closed to unregister.
func (eb *InMemoryEventBus) Subscribe(eventType EventType, handler EventHandler) Subscription {
	eb.closeMu.RLock()
	defer eb.closeMu.RUnlock()

	if eb.closing {
		return noopSub{}
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()
	hm, ok := eb.handlers[eventType]
	if !ok {
		hm = make(map[string]EventHandler)
		eb.handlers[eventType] = hm
	}
	id := eb.nextID(eventType)
	hm[id] = handler
	// update handler registration gauge
	m.EventBusHandlersRegistered.WithLabelValues(string(eventType), eb.busType).Set(float64(len(hm)))
	return &memSub{bus: eb, et: eventType, id: id}
}

// Publish publishes an event to all registered handlers asynchronously.
func (eb *InMemoryEventBus) Publish(ctx context.Context, event Event) error {
	eb.closeMu.RLock()
	defer eb.closeMu.RUnlock()

	if eb.closing {
		return nil // Don't publish if we're closing
	}

	eb.mu.RLock()
	snapshot := make([]EventHandler, 0, 8)
	if hm, ok := eb.handlers[event.Type]; ok {
		for _, h := range hm {
			snapshot = append(snapshot, h)
		}
	}
	eb.mu.RUnlock()

	eb.activeOps.Add(len(snapshot))
	// metrics: published
	m.EventBusEventsPublishedTotal.WithLabelValues(string(event.Type), eb.busType).Inc()
	for _, h := range snapshot {
		handler := h
		enqueueTask := func() {
			defer eb.activeOps.Done()
			// panic safety and metrics
			start := time.Now()
			defer func() {
				if r := recover(); r != nil {
					// count failure; best-effort log via fmt
					m.EventBusEventsFailedTotal.WithLabelValues(string(event.Type), eb.busType, "panic").Inc()
					fmt.Printf("event bus handler panic: %v\n", r)
				}
				m.EventBusProcessingLatency.WithLabelValues(string(event.Type), eb.busType).Observe(time.Since(start).Seconds())
			}()

			handlerCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := handler.HandleEvent(handlerCtx, event); err != nil {
				m.EventBusEventsFailedTotal.WithLabelValues(string(event.Type), eb.busType, "handler_error").Inc()
			} else {
				m.EventBusEventsDeliveredTotal.WithLabelValues(string(event.Type), eb.busType).Inc()
			}
		}

		// backpressure handling
		enqueued := false
		for !enqueued {
			select {
			case eb.queue <- enqueueTask:
				enqueued = true
			default:
				// queue full
				switch eb.policy {
				case "drop_newest":
					eb.activeOps.Done()
					m.EventBusEventsFailedTotal.WithLabelValues(string(event.Type), eb.busType, "queue_full_drop_newest").Inc()
					enqueued = true
				case "drop_oldest":
					// try drop one oldest task
					select {
					case <-eb.queue:
						// dropped oldest; try enqueue again immediately
						continue
					default:
						// nothing to drop; act like drop_newest
						eb.activeOps.Done()
						m.EventBusEventsFailedTotal.WithLabelValues(string(event.Type), eb.busType, "queue_full_drop_newest").Inc()
						enqueued = true
					}
				case "block":
					// wait with timeout
					timeout := eb.enqueueTimeout
					if timeout <= 0 {
						timeout = 100 * time.Millisecond
					}
					t := time.NewTimer(timeout)
					select {
					case eb.queue <- enqueueTask:
						enqueued = true
						if !t.Stop() {
							<-t.C
						}
					case <-t.C:
						// timeout, drop
						eb.activeOps.Done()
						m.EventBusEventsFailedTotal.WithLabelValues(string(event.Type), eb.busType, "queue_block_timeout").Inc()
						enqueued = true
					}
				default:
					// unknown policy fallback
					eb.activeOps.Done()
					m.EventBusEventsFailedTotal.WithLabelValues(string(event.Type), eb.busType, "queue_full_unknown_policy").Inc()
					enqueued = true
				}
			}
		}
	}
	// update queue gauge
	m.EventBusQueueSize.WithLabelValues(eb.busType).Set(float64(len(eb.queue)))
	return nil
}

// Close gracefully shuts down the event bus and waits for all pending operations to complete.
func (eb *InMemoryEventBus) Close() error {
	eb.closeMu.Lock()
	eb.closing = true
	eb.closeMu.Unlock()

	// Wait for all active operations to complete
	eb.activeOps.Wait()

	// Clear handlers to free memory
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Clear the handlers map to free memory
	for eventType := range eb.handlers {
		delete(eb.handlers, eventType)
	}

	// stop workers
	close(eb.done)
	eb.workersWg.Wait()

	return nil
}

// IsClosing returns true if the event bus is in the process of closing.
func (eb *InMemoryEventBus) IsClosing() bool {
	eb.closeMu.RLock()
	defer eb.closeMu.RUnlock()
	return eb.closing
}

// GetHandlerCount returns the number of registered handlers for a given event type.
func (eb *InMemoryEventBus) GetHandlerCount(eventType EventType) int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	hm, exists := eb.handlers[eventType]
	if !exists {
		return 0
	}
	return len(hm)
}

// GetTotalHandlerCount returns the total number of registered handlers across all event types.
func (eb *InMemoryEventBus) GetTotalHandlerCount() int {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	total := 0
	for _, hm := range eb.handlers {
		total += len(hm)
	}
	return total
}

// EventBusMetrics holds Prometheus metrics for the event bus.
type EventBusMetrics struct {
	// These fields should be initialized with Prometheus metrics in the actual implementation.
	EventsPublished   interface{}
	EventsDelivered   interface{}
	EventsFailed      interface{}
	ProcessingLatency interface{}
	QueueSize         interface{}
	ActiveSubscribers interface{}
}

// testEventHandler is a simple test event handler used across multiple test files
type testEventHandler struct {
	events chan<- Event
}

func (h *testEventHandler) HandleEvent(ctx context.Context, event Event) error {
	select {
	case h.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// internal helpers

type memSub struct {
	bus *InMemoryEventBus
	et  EventType
	id  string
}

func (s *memSub) Close() error {
	return s.bus.UnregisterHandler(s.et, s.id)
}

type noopSub struct{}

func (noopSub) Close() error { return nil }

func (eb *InMemoryEventBus) nextID(et EventType) string {
	eb.idSeq++
	return fmt.Sprintf("%s-%d", et, eb.idSeq)
}

// UnregisterHandler removes a handler by ID for a given event type.
func (eb *InMemoryEventBus) UnregisterHandler(eventType EventType, handlerID string) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	if hm, ok := eb.handlers[eventType]; ok {
		if _, exists := hm[handlerID]; exists {
			delete(hm, handlerID)
			// update handler registration gauge
			m.EventBusHandlersRegistered.WithLabelValues(string(eventType), eb.busType).Set(float64(len(hm)))
			return nil
		}
	}
	return fmt.Errorf("handler not found")
}

func (eb *InMemoryEventBus) startWorkers() {
	for i := 0; i < eb.maxWorkers; i++ {
		eb.workersWg.Add(1)
		go func() {
			defer eb.workersWg.Done()
			for {
				select {
				case <-eb.done:
					return
				case task := <-eb.queue:
					if task != nil {
						m.EventBusActiveWorkers.WithLabelValues(eb.busType).Inc()
						task()
						m.EventBusActiveWorkers.WithLabelValues(eb.busType).Dec()
						// update queue gauge after processing
						m.EventBusQueueSize.WithLabelValues(eb.busType).Set(float64(len(eb.queue)))
					}
				}
			}
		}()
	}
}
