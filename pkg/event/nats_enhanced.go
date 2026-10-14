package event

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/metrics"
	"github.com/nats-io/nats.go"
)

// NATSConnectionPool manages a pool of NATS connections for high throughput
type NATSConnectionPool struct {
	connections chan *nats.Conn
	factory     func() (*nats.Conn, error)
	maxConns    int
	url         string
	mu          sync.RWMutex
	closed      bool
}

// NewNATSConnectionPool creates a new NATS connection pool
func NewNATSConnectionPool(url string, maxConns int) *NATSConnectionPool {
	pool := &NATSConnectionPool{
		connections: make(chan *nats.Conn, maxConns),
		maxConns:    maxConns,
		url:         url,
		closed:      false,
	}

	pool.factory = func() (*nats.Conn, error) {
		return nats.Connect(url, nats.Name("trazr-event-bus"))
	}

	// Pre-populate the pool (non-blocking)
	go func() {
		for i := 0; i < maxConns; i++ {
			// Check if pool is closed before creating connection
			pool.mu.RLock()
			if pool.closed {
				pool.mu.RUnlock()
				return
			}
			pool.mu.RUnlock()

			conn, err := pool.factory()
			if err != nil {
				continue
			}

			// Check if pool is closed before sending to channel
			pool.mu.RLock()
			if pool.closed {
				pool.mu.RUnlock()
				conn.Close()
				return
			}
			pool.mu.RUnlock()

			// Use select with default to avoid blocking
			select {
			case pool.connections <- conn:
				// Successfully added to pool
			default:
				// Pool is full or closed, close the connection
				conn.Close()
			}
		}
	}()

	return pool
}

// Get returns a connection from the pool or creates a new one
func (pool *NATSConnectionPool) Get() (*nats.Conn, error) {
	pool.mu.RLock()
	if pool.closed {
		pool.mu.RUnlock()
		return nil, fmt.Errorf("connection pool is closed")
	}
	pool.mu.RUnlock()

	select {
	case conn := <-pool.connections:
		if conn != nil && conn.IsConnected() {
			return conn, nil
		}
		// Connection is dead, create new one
		return pool.factory()
	default:
		return pool.factory()
	}
}

// Put returns a connection to the pool
func (pool *NATSConnectionPool) Put(conn *nats.Conn) {
	if conn == nil || !conn.IsConnected() {
		return
	}

	pool.mu.RLock()
	if pool.closed {
		pool.mu.RUnlock()
		return
	}
	pool.mu.RUnlock()

	select {
	case pool.connections <- conn:
		// Successfully returned to pool
	default:
		// Pool is full, close the connection
		conn.Close()
	}
}

// Close closes all connections in the pool
func (pool *NATSConnectionPool) Close() error {
	pool.mu.Lock()
	if pool.closed {
		pool.mu.Unlock()
		return nil // Already closed
	}
	pool.closed = true
	pool.mu.Unlock()

	// Close the channel safely
	close(pool.connections)

	var closeErrs []error
	// Drain the channel and close all connections
	for {
		select {
		case conn, ok := <-pool.connections:
			if !ok {
				// Channel is closed
				goto done
			}
			if conn != nil {
				conn.Close()
			}
		default:
			// Channel is empty
			goto done
		}
	}
done:

	if len(closeErrs) > 0 {
		return fmt.Errorf("errors closing connections: %v", closeErrs)
	}
	return nil
}

// EnhancedNATSEventBus implements high-performance NATS event communication
type EnhancedNATSEventBus struct {
	pool        *NATSConnectionPool
	handlers    map[EventType][]EventHandler
	mu          sync.RWMutex
	subscribers map[string]*nats.Subscription
	url         string
	busType     string

	// Batch processing
	batchSize    int
	batchTimeout time.Duration
	batchChan    chan Event
	batchTicker  *time.Ticker
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewEnhancedNATSEventBus creates a new enhanced NATS event bus
func NewEnhancedNATSEventBus(url string, maxConns, batchSize int, batchTimeout time.Duration) (*EnhancedNATSEventBus, error) {
	ctx, cancel := context.WithCancel(context.Background())

	bus := &EnhancedNATSEventBus{
		pool:         NewNATSConnectionPool(url, maxConns),
		handlers:     make(map[EventType][]EventHandler),
		subscribers:  make(map[string]*nats.Subscription),
		url:          url,
		busType:      "nats-enhanced",
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		batchChan:    make(chan Event, batchSize*2),
		batchTicker:  time.NewTicker(batchTimeout),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start batch processor
	bus.wg.Add(1)
	go bus.batchProcessor()

	return bus, nil
}

// batchProcessor processes events in batches for better performance
func (bus *EnhancedNATSEventBus) batchProcessor() {
	defer bus.wg.Done()

	batch := make([]Event, 0, bus.batchSize)

	for {
		select {
		case event := <-bus.batchChan:
			batch = append(batch, event)
			if len(batch) >= bus.batchSize {
				bus.processBatch(batch)
				batch = batch[:0] // Reset slice
			}
		case <-bus.batchTicker.C:
			if len(batch) > 0 {
				bus.processBatch(batch)
				batch = batch[:0]
			}
		case <-bus.ctx.Done():
			// Process remaining events
			if len(batch) > 0 {
				bus.processBatch(batch)
			}
			return
		}
	}
}

// processBatch publishes a batch of events efficiently
func (bus *EnhancedNATSEventBus) processBatch(events []Event) {
	if len(events) == 0 {
		return
	}

	start := time.Now()

	// Get connection from pool
	conn, err := bus.pool.Get()
	if err != nil {
		// Record failure
		metrics.EventBusEventsFailedTotal.WithLabelValues(
			string(events[0].Type), bus.busType, "connection_error").Inc()
		return
	}
	defer bus.pool.Put(conn)

	// Publish events in batch
	for _, event := range events {
		bus.publishEvent(conn, event)
	}

	// Record batch metrics
	duration := time.Since(start).Seconds()
	metrics.EventBusBatchLatency.WithLabelValues(bus.busType).Observe(duration)
	metrics.EventBusBatchSize.WithLabelValues(bus.busType).Observe(float64(len(events)))
}

// publishEvent publishes a single event with metrics
func (bus *EnhancedNATSEventBus) publishEvent(conn *nats.Conn, event Event) {
	start := time.Now()

	// Marshal event
	payload, err := json.Marshal(event)
	if err != nil {
		metrics.EventBusEventsFailedTotal.WithLabelValues(
			string(event.Type), bus.busType, "marshal_error").Inc()
		return
	}

	// Record message size
	metrics.EventBusNATSMessageSize.WithLabelValues(string(event.Type)).Observe(float64(len(payload)))

	// Publish to NATS
	topic := string(event.Type)
	err = conn.Publish(topic, payload)
	if err != nil {
		metrics.EventBusEventsFailedTotal.WithLabelValues(
			string(event.Type), bus.busType, "publish_error").Inc()
		return
	}

	// Record success metrics
	duration := time.Since(start).Seconds()
	metrics.EventBusNATSPublishLatency.WithLabelValues(topic).Observe(duration)
	metrics.EventBusNATSMessagesPublished.WithLabelValues(topic).Inc()
	metrics.EventBusEventsPublishedTotal.WithLabelValues(string(event.Type), bus.busType).Inc()
}

// RegisterHandler registers an event handler for a specific event type
func (bus *EnhancedNATSEventBus) RegisterHandler(eventType EventType, handler EventHandler) {
	bus.mu.Lock()
	defer bus.mu.Unlock()

	bus.handlers[eventType] = append(bus.handlers[eventType], handler)

	// Subscribe to NATS topic if not already subscribed
	topic := string(eventType)
	if _, exists := bus.subscribers[topic]; !exists {
		conn, err := bus.pool.Get()
		if err != nil {
			return
		}
		defer bus.pool.Put(conn)

		sub, err := conn.Subscribe(topic, func(msg *nats.Msg) {
			bus.handleNATSMessage(eventType, msg)
		})
		if err != nil {
			return
		}
		bus.subscribers[topic] = sub
	}

	// Update metrics
	metrics.EventBusHandlersRegistered.WithLabelValues(string(eventType), bus.busType).Set(
		float64(len(bus.handlers[eventType])))
}

// Publish publishes an event (adds to batch for processing)
func (bus *EnhancedNATSEventBus) Publish(ctx context.Context, event Event) error {
	select {
	case bus.batchChan <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-bus.ctx.Done():
		return context.Canceled
	}
}

// PublishBatch publishes multiple events directly (bypasses batching)
func (bus *EnhancedNATSEventBus) PublishBatch(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return nil
	}

	conn, err := bus.pool.Get()
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}
	defer bus.pool.Put(conn)

	for _, event := range events {
		bus.publishEvent(conn, event)
	}

	return nil
}

// handleNATSMessage processes incoming NATS messages
func (bus *EnhancedNATSEventBus) handleNATSMessage(eventType EventType, msg *nats.Msg) {
	start := time.Now()

	var event Event
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		metrics.EventBusEventsFailedTotal.WithLabelValues(
			string(eventType), bus.busType, "unmarshal_error").Inc()
		return
	}

	// Record message received
	metrics.EventBusNATSMessagesReceived.WithLabelValues(string(eventType)).Inc()

	bus.mu.RLock()
	handlers := bus.handlers[eventType]
	bus.mu.RUnlock()

	// Process handlers
	for _, handler := range handlers {
		go func(h EventHandler) {
			handlerCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := h.HandleEvent(handlerCtx, event); err != nil {
				metrics.EventBusEventsFailedTotal.WithLabelValues(
					string(eventType), bus.busType, "handler_error").Inc()
			} else {
				metrics.EventBusEventsDeliveredTotal.WithLabelValues(
					string(eventType), bus.busType).Inc()
			}
		}(handler)
	}

	// Record processing latency
	duration := time.Since(start).Seconds()
	metrics.EventBusProcessingLatency.WithLabelValues(string(eventType), bus.busType).Observe(duration)
}

// Close closes the enhanced NATS event bus
func (bus *EnhancedNATSEventBus) Close() error {
	// Stop batch processor
	bus.cancel()
	bus.batchTicker.Stop()
	bus.wg.Wait()

	// Close subscribers
	bus.mu.Lock()
	for topic, sub := range bus.subscribers {
		_ = sub.Unsubscribe()
		delete(bus.subscribers, topic)
	}
	bus.mu.Unlock()

	// Close connection pool
	return bus.pool.Close()
}

// HealthCheck checks if the NATS connection pool is healthy
func (bus *EnhancedNATSEventBus) HealthCheck() error {
	conn, err := bus.pool.Get()
	if err != nil {
		return fmt.Errorf("failed to get connection: %w", err)
	}
	defer bus.pool.Put(conn)

	if !conn.IsConnected() {
		return fmt.Errorf("NATS connection lost")
	}

	// Update connection status metric
	metrics.EventBusNATSConnectionStatus.WithLabelValues(bus.url).Set(1.0)

	return nil
}

// GetStats returns statistics about the enhanced NATS event bus
func (bus *EnhancedNATSEventBus) GetStats() map[string]interface{} {
	bus.mu.RLock()
	defer bus.mu.RUnlock()

	totalHandlers := 0
	for _, handlers := range bus.handlers {
		totalHandlers += len(handlers)
	}

	return map[string]interface{}{
		"bus_type":       bus.busType,
		"url":            bus.url,
		"total_handlers": totalHandlers,
		"event_types":    len(bus.handlers),
		"subscribers":    len(bus.subscribers),
		"pool_size":      bus.pool.maxConns,
		"batch_size":     bus.batchSize,
		"batch_timeout":  bus.batchTimeout.String(),
		"queue_size":     len(bus.batchChan),
		"queue_capacity": cap(bus.batchChan),
	}
}
