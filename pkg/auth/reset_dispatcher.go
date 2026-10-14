package auth

import (
	"context"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth/storage"
)

const passwordResetQueueCapacity = 128
const passwordResetDeliveryWorkers = 2
const passwordResetDeliveryTimeout = 30 * time.Second

// PasswordResetDispatcher owns a bounded private delivery queue. HTTP requests
// enqueue only the address; no account lookup or SMTP work delays their response.
// Stop must run before its database is closed.
type PasswordResetDispatcher struct {
	delivery *PasswordResetDelivery
	store    storage.Store
	queue    chan string
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

func NewPasswordResetDispatcher(delivery *PasswordResetDelivery, store storage.Store) (*PasswordResetDispatcher, error) {
	if delivery == nil || delivery.Sender == nil || delivery.ResetURL == nil || store == nil {
		return nil, ErrEmailNotConfigured
	}
	ctx, cancel := context.WithCancel(context.Background())
	d := &PasswordResetDispatcher{delivery: delivery, store: store, queue: make(chan string, passwordResetQueueCapacity), ctx: ctx, cancel: cancel, done: make(chan struct{})}
	var workers sync.WaitGroup
	for i := 0; i < passwordResetDeliveryWorkers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case email := <-d.queue:
					if ctx.Err() != nil {
						return
					}
					job, cancel := context.WithTimeout(ctx, passwordResetDeliveryTimeout)
					_ = d.delivery.Request(job, d.store, email)
					cancel()
				}
			}
		}()
	}
	go func() { workers.Wait(); close(d.done) }()
	return d, nil
}
func (d *PasswordResetDispatcher) Submit(email string) bool {
	if d == nil || d.ctx.Err() != nil {
		return false
	}
	select {
	case d.queue <- email:
		return true
	default:
		return false
	}
}
func (d *PasswordResetDispatcher) Stop(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.cancel()
	select {
	case <-d.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Compatibility factories use an on-demand shared pool. It is bounded across
// requests and has no idle workers: all goroutines exit when queued work finishes.
// Collector production uses the explicitly owned dispatcher above.
var resetFallbackPool struct {
	mu      sync.Mutex
	workers int
	queue   []func(context.Context)
}

func submitResetFallback(job func(context.Context)) bool {
	resetFallbackPool.mu.Lock()
	defer resetFallbackPool.mu.Unlock()
	if len(resetFallbackPool.queue) >= passwordResetQueueCapacity {
		return false
	}
	resetFallbackPool.queue = append(resetFallbackPool.queue, job)
	for resetFallbackPool.workers < passwordResetDeliveryWorkers {
		resetFallbackPool.workers++
		go runResetFallbackWorker()
	}
	return true
}
func runResetFallbackWorker() {
	for {
		resetFallbackPool.mu.Lock()
		if len(resetFallbackPool.queue) == 0 {
			resetFallbackPool.workers--
			resetFallbackPool.mu.Unlock()
			return
		}
		job := resetFallbackPool.queue[0]
		resetFallbackPool.queue[0] = nil
		resetFallbackPool.queue = resetFallbackPool.queue[1:]
		if len(resetFallbackPool.queue) == 0 {
			resetFallbackPool.queue = nil
		}
		resetFallbackPool.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), passwordResetDeliveryTimeout)
		job(ctx)
		cancel()
	}
}

// SubmitPasswordResetFallback is a bounded compatibility bridge for standalone
// handler factories. Its workers exit when the queue drains.
func SubmitPasswordResetFallback(delivery *PasswordResetDelivery, store storage.Store, email string) bool {
	return submitResetFallback(func(ctx context.Context) { _ = delivery.Request(ctx, store, email) })
}
