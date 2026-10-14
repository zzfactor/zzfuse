package event

import (
	"context"
	"sync"
	"time"
)

// WorkerPool manages a pool of workers to handle events efficiently
type WorkerPool struct {
	workers    int
	jobQueue   chan EventJob
	workerPool chan struct{}
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	closing    bool
	closeMu    sync.RWMutex
}

// EventJob represents a job to be processed by a worker
type EventJob struct {
	Event   Event
	Handler EventHandler
	Context context.Context
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(workers int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		workers:    workers,
		jobQueue:   make(chan EventJob, workers*2), // Buffer for job queue
		workerPool: make(chan struct{}, workers),   // Semaphore for worker slots
		ctx:        ctx,
		cancel:     cancel,
	}

	// Start workers
	pool.startWorkers()

	return pool
}

// startWorkers starts the worker goroutines
func (wp *WorkerPool) startWorkers() {
	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go wp.worker()
	}
}

// worker is the main worker loop
func (wp *WorkerPool) worker() {
	defer wp.wg.Done()

	for {
		select {
		case job := <-wp.jobQueue:
			// Process the job
			wp.processJob(job)
		case <-wp.ctx.Done():
			return
		}
	}
}

// processJob processes a single event job
func (wp *WorkerPool) processJob(job EventJob) {
	// Use a timeout context to prevent hanging handlers
	handlerCtx, cancel := context.WithTimeout(job.Context, 30*time.Second)
	defer cancel()

	// Execute the handler
	_ = job.Handler.HandleEvent(handlerCtx, job.Event)
}

// Submit submits a job to the worker pool
func (wp *WorkerPool) Submit(ctx context.Context, event Event, handler EventHandler) error {
	wp.closeMu.RLock()
	defer wp.closeMu.RUnlock()

	if wp.closing {
		return nil // Don't submit jobs if we're closing
	}

	job := EventJob{
		Event:   event,
		Handler: handler,
		Context: ctx,
	}

	select {
	case wp.jobQueue <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-wp.ctx.Done():
		return context.Canceled
	}
}

// SubmitBatch submits multiple jobs to the worker pool
func (wp *WorkerPool) SubmitBatch(ctx context.Context, events []Event, handlers []EventHandler) error {
	wp.closeMu.RLock()
	defer wp.closeMu.RUnlock()

	if wp.closing {
		return nil
	}

	// Submit jobs in batches to avoid blocking
	for i, event := range events {
		handler := handlers[i%len(handlers)] // Round-robin handlers

		job := EventJob{
			Event:   event,
			Handler: handler,
			Context: ctx,
		}

		select {
		case wp.jobQueue <- job:
			// Job submitted successfully
		case <-ctx.Done():
			return ctx.Err()
		case <-wp.ctx.Done():
			return context.Canceled
		}
	}

	return nil
}

// Close gracefully shuts down the worker pool
func (wp *WorkerPool) Close() error {
	wp.closeMu.Lock()
	wp.closing = true
	wp.closeMu.Unlock()

	// Cancel context to stop workers
	wp.cancel()

	// Wait for all workers to finish
	wp.wg.Wait()

	// Close job queue
	close(wp.jobQueue)

	return nil
}

// GetStats returns statistics about the worker pool
func (wp *WorkerPool) GetStats() WorkerPoolStats {
	return WorkerPoolStats{
		Workers:       wp.workers,
		QueueSize:     len(wp.jobQueue),
		QueueCapacity: cap(wp.jobQueue),
		ActiveWorkers: len(wp.workerPool),
		IsClosing:     wp.closing,
	}
}

// WorkerPoolStats contains statistics about the worker pool
type WorkerPoolStats struct {
	Workers       int  `json:"workers"`
	QueueSize     int  `json:"queue_size"`
	QueueCapacity int  `json:"queue_capacity"`
	ActiveWorkers int  `json:"active_workers"`
	IsClosing     bool `json:"is_closing"`
}

// PooledEventBus is an event bus implementation that uses a worker pool for processing events.
type PooledEventBus struct {
	handlers   map[EventType][]EventHandler
	mu         sync.RWMutex
	workerPool *WorkerPool

	// Cleanup management
	closing bool
	closeMu sync.RWMutex
}

// NewPooledEventBus creates a new event bus with worker pool
func NewPooledEventBus(workers int) *PooledEventBus {
	return &PooledEventBus{
		handlers:   make(map[EventType][]EventHandler),
		workerPool: NewWorkerPool(workers),
		closing:    false,
	}
}

// RegisterHandler registers an event handler for a specific event type
func (eb *PooledEventBus) RegisterHandler(eventType EventType, handler EventHandler) {
	eb.closeMu.RLock()
	defer eb.closeMu.RUnlock()

	if eb.closing {
		return
	}

	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.handlers[eventType] == nil {
		eb.handlers[eventType] = make([]EventHandler, 0, 4) // Pre-allocate capacity
	}
	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// Publish publishes an event to all registered handlers using the worker pool
func (eb *PooledEventBus) Publish(ctx context.Context, event Event) error {
	eb.closeMu.RLock()
	defer eb.closeMu.RUnlock()

	if eb.closing {
		return nil
	}

	eb.mu.RLock()
	handlers := eb.handlers[event.Type]
	eb.mu.RUnlock()

	// Submit jobs to worker pool
	for _, handler := range handlers {
		if err := eb.workerPool.Submit(ctx, event, handler); err != nil {
			return err
		}
	}

	return nil
}

// PublishBatch publishes multiple events efficiently
func (eb *PooledEventBus) PublishBatch(ctx context.Context, events []Event) error {
	eb.closeMu.RLock()
	defer eb.closeMu.RUnlock()

	if eb.closing {
		return nil
	}

	eb.mu.RLock()
	handlers := eb.handlers[events[0].Type] // Assume all events have same type
	eb.mu.RUnlock()

	// Create batch of jobs
	allHandlers := make([]EventHandler, 0, len(events)*len(handlers))
	allEvents := make([]Event, 0, len(events)*len(handlers))

	for _, event := range events {
		for _, handler := range handlers {
			allEvents = append(allEvents, event)
			allHandlers = append(allHandlers, handler)
		}
	}

	return eb.workerPool.SubmitBatch(ctx, allEvents, allHandlers)
}

// Close gracefully shuts down the event bus
func (eb *PooledEventBus) Close() error {
	eb.closeMu.Lock()
	eb.closing = true
	eb.closeMu.Unlock()

	// Close worker pool
	if err := eb.workerPool.Close(); err != nil {
		return err
	}

	// Clear handlers
	eb.mu.Lock()
	defer eb.mu.Unlock()

	for eventType := range eb.handlers {
		delete(eb.handlers, eventType)
	}

	return nil
}

// GetStats returns statistics about the event bus and worker pool
func (eb *PooledEventBus) GetStats() EventBusStats {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	totalHandlers := 0
	for _, handlers := range eb.handlers {
		totalHandlers += len(handlers)
	}

	return EventBusStats{
		TotalHandlers: totalHandlers,
		EventTypes:    len(eb.handlers),
		WorkerPool:    eb.workerPool.GetStats(),
		IsClosing:     eb.closing,
	}
}

// EventBusStats contains statistics about the event bus
type EventBusStats struct {
	TotalHandlers int             `json:"total_handlers"`
	EventTypes    int             `json:"event_types"`
	WorkerPool    WorkerPoolStats `json:"worker_pool"`
	IsClosing     bool            `json:"is_closing"`
}
