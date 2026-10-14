package event

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	metrics "github.com/medxops/trazr-otel/pkg/metrics"
)

// NATSEventBus implements distributed event communication using NATS
type NATSEventBus struct {
	nc          *nats.Conn
	handlers    map[EventType]map[string]EventHandler
	mu          sync.RWMutex
	subscribers map[string]*nats.Subscription
	url         string
	idSeq       uint64

	// worker pool & queue
	bufferSize int
	maxWorkers int
	queue      chan func()
	workersWg  sync.WaitGroup
	done       chan struct{}
	policy      string
	enqueueTimeout time.Duration

	// metrics
	busType string
}

// NewNATSEventBus creates a new NATS event bus
func NewNATSEventBus(natsURL string) (*NATSEventBus, error) {
	nc, err := nats.Connect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	cfg := GetEventBusConfig()
	// derive worker/queue from config (reuse InMemory section), with fallbacks
	buf := cfg.InMemory.BufferSize
	if buf <= 0 {
		buf = 1024
	}
	workers := cfg.InMemory.MaxWorkers
	if workers <= 0 {
		workers = 4
	}
	policy := cfg.InMemory.BackpressurePolicy
	if policy == "" {
		policy = "drop_newest"
	}
	timeout := cfg.InMemory.EnqueueTimeout
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}

	eb := &NATSEventBus{
		nc:          nc,
		handlers:    make(map[EventType]map[string]EventHandler),
		subscribers: make(map[string]*nats.Subscription),
		url:         natsURL,
		bufferSize:  buf,
		maxWorkers:  workers,
		queue:       make(chan func(), buf),
		done:        make(chan struct{}),
		policy:      policy,
		enqueueTimeout: timeout,
		busType:     "nats",
	}
	eb.startWorkers()
	metrics.EventBusTotalWorkers.WithLabelValues(eb.busType).Set(float64(workers))
	// connection status gauge
	metrics.EventBusNATSConnectionStatus.WithLabelValues(natsURL).Set(1)
	return eb, nil
}

// RegisterHandler registers an event handler for a specific event type
func (eb *NATSEventBus) RegisterHandler(eventType EventType, handler EventHandler) {
	_ = eb.Subscribe(eventType, handler)
}

// Subscribe registers and returns a subscription for the handler.
func (eb *NATSEventBus) Subscribe(eventType EventType, handler EventHandler) Subscription {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	hm, ok := eb.handlers[eventType]
	if !ok {
		hm = make(map[string]EventHandler)
		eb.handlers[eventType] = hm
	}
	eb.idSeq++
	id := fmt.Sprintf("%s-%d", eventType, eb.idSeq)
	hm[id] = handler
	// handler gauge
	metrics.EventBusHandlersRegistered.WithLabelValues(string(eventType), eb.busType).Set(float64(len(hm)))

	// Ensure NATS subscription exists
	topic := string(eventType)
	if _, exists := eb.subscribers[topic]; !exists {
		sub, err := eb.nc.Subscribe(topic, func(msg *nats.Msg) {
			eb.handleNATSMessage(eventType, msg)
		})
		if err != nil {
			// Log error; still return a sub that can be closed which is a no-op
			fmt.Printf("Failed to subscribe to NATS topic %s: %v\n", topic, err)
		} else {
			eb.subscribers[topic] = sub
		}
	}

	return &natsSub{bus: eb, et: eventType, id: id}
}

// Publish publishes an event to NATS
func (eb *NATSEventBus) Publish(ctx context.Context, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	topic := string(event.Type)
	start := time.Now()
	err = eb.nc.Publish(topic, payload)
	if err != nil {
		return fmt.Errorf("failed to publish to NATS topic %s: %w", topic, err)
	}
	// metrics
	metrics.EventBusNATSMessagesPublished.WithLabelValues(topic).Inc()
	metrics.EventBusNATSPublishLatency.WithLabelValues(topic).Observe(time.Since(start).Seconds())
	metrics.EventBusEventsPublishedTotal.WithLabelValues(string(event.Type), eb.busType).Inc()
	return nil
}

// handleNATSMessage processes incoming NATS messages
func (eb *NATSEventBus) handleNATSMessage(eventType EventType, msg *nats.Msg) {
	var event Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		fmt.Printf("Failed to unmarshal NATS message: %v\n", err)
		return
	}

	// metrics
	metrics.EventBusNATSMessagesReceived.WithLabelValues(msg.Subject).Inc()

	eb.mu.RLock()
	var snapshot []EventHandler
	hm, ok := eb.handlers[eventType]
	if ok {
		snapshot = make([]EventHandler, 0, len(hm))
		for _, h := range hm {
			snapshot = append(snapshot, h)
		}
	}
	eb.mu.RUnlock()

	// enqueue work with backpressure handling
	for _, handler := range snapshot {
		h := handler
		task := func() {
			// panic safety and metrics
			start := time.Now()
			defer func() {
				if r := recover(); r != nil {
					metrics.EventBusEventsFailedTotal.WithLabelValues(string(eventType), eb.busType, "panic").Inc()
					fmt.Printf("NATS handler panic for event %s: %v\n", eventType, r)
				}
				metrics.EventBusProcessingLatency.WithLabelValues(string(eventType), eb.busType).Observe(time.Since(start).Seconds())
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := h.HandleEvent(ctx, event); err != nil {
				metrics.EventBusEventsFailedTotal.WithLabelValues(string(eventType), eb.busType, "handler_error").Inc()
			} else {
				metrics.EventBusEventsDeliveredTotal.WithLabelValues(string(eventType), eb.busType).Inc()
			}
		}

		enqueued := false
		for !enqueued {
			select {
			case eb.queue <- task:
				enqueued = true
			default:
				switch eb.policy {
				case "drop_newest":
					metrics.EventBusEventsFailedTotal.WithLabelValues(string(eventType), eb.busType, "queue_full_drop_newest").Inc()
					enqueued = true
				case "drop_oldest":
					select {
					case <-eb.queue:
						// dropped oldest
						continue
					default:
						metrics.EventBusEventsFailedTotal.WithLabelValues(string(eventType), eb.busType, "queue_full_drop_newest").Inc()
						enqueued = true
					}
				case "block":
					t := time.NewTimer(eb.enqueueTimeout)
					select {
					case eb.queue <- task:
						enqueued = true
						if !t.Stop() { <-t.C }
					case <-t.C:
						metrics.EventBusEventsFailedTotal.WithLabelValues(string(eventType), eb.busType, "queue_block_timeout").Inc()
						enqueued = true
					}
				default:
					metrics.EventBusEventsFailedTotal.WithLabelValues(string(eventType), eb.busType, "queue_full_unknown_policy").Inc()
					enqueued = true
				}
			}
		}
	}
	// queue gauge
	metrics.EventBusQueueSize.WithLabelValues(eb.busType).Set(float64(len(eb.queue)))
}

// Close closes the NATS connection and cleans up subscriptions
func (eb *NATSEventBus) Close() error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Unsubscribe from all topics
	for topic, sub := range eb.subscribers {
		if err := sub.Unsubscribe(); err != nil {
			fmt.Printf("Failed to unsubscribe from %s: %v\n", topic, err)
		}
	}

	// Close NATS connection
	if eb.nc != nil {
		eb.nc.Close()
	}

	// stop workers
	close(eb.done)
	eb.workersWg.Wait()
	metrics.EventBusNATSConnectionStatus.WithLabelValues(eb.url).Set(0)

	return nil
}

// HealthCheck checks if the NATS connection is healthy
func (eb *NATSEventBus) HealthCheck() error {
	if eb.nc == nil {
		return fmt.Errorf("NATS not connected")
	}

	if !eb.nc.IsConnected() {
		return fmt.Errorf("NATS connection lost")
	}

	// Try to publish a health check message
	healthMsg := []byte("health_check")
	err := eb.nc.Publish("health_check", healthMsg)
	if err != nil {
		return fmt.Errorf("NATS health check failed: %w", err)
	}

	return nil
}

// UnregisterHandler removes a handler by its ID for a given event type.
func (eb *NATSEventBus) UnregisterHandler(eventType EventType, handlerID string) error {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	hm, ok := eb.handlers[eventType]
	if ok {
		if _, exists := hm[handlerID]; exists {
			delete(hm, handlerID)
			// handlers gauge
			metrics.EventBusHandlersRegistered.WithLabelValues(string(eventType), eb.busType).Set(float64(len(hm)))
			// If no handlers remain, unsubscribe
			if len(hm) == 0 {
				delete(eb.handlers, eventType)
				topic := string(eventType)
				if sub, ok := eb.subscribers[topic]; ok {
					_ = sub.Unsubscribe()
					delete(eb.subscribers, topic)
				}
			}
			return nil
		}
	}
	return fmt.Errorf("handler not found")
}

type natsSub struct {
	bus *NATSEventBus
	et  EventType
	id  string
}

func (s *natsSub) Close() error {
	return s.bus.UnregisterHandler(s.et, s.id)
}

// startWorkers launches worker goroutines to process tasks from the queue
func (eb *NATSEventBus) startWorkers() {
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
						metrics.EventBusActiveWorkers.WithLabelValues(eb.busType).Inc()
						task()
						metrics.EventBusActiveWorkers.WithLabelValues(eb.busType).Dec()
						metrics.EventBusQueueSize.WithLabelValues(eb.busType).Set(float64(len(eb.queue)))
					}
				}
			}
		}()
	}
}
