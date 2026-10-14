package auth

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth/storage"
)

type blockedResetUsers struct {
	storage.Users
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (u *blockedResetUsers) GetByEmail(ctx context.Context, _ string) (*storage.UserRecord, error) {
	u.calls.Add(1)
	u.entered <- struct{}{}
	select {
	case <-u.release:
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type blockedResetLookupStore struct {
	storage.Store
	users *blockedResetUsers
}

func (s *blockedResetLookupStore) Users() storage.Users { return s.users }
func TestPasswordResetDispatcherIsBoundedAndStops(t *testing.T) {
	users := &blockedResetUsers{entered: make(chan struct{}, 2), release: make(chan struct{})}
	store := &blockedResetLookupStore{users: users}
	delivery, _ := NewPasswordResetDelivery(emailSenderFunc(func(context.Context, string, string, string) error {
		t.Error("missing accounts must not send mail")
		return nil
	}), "https://example.test/reset-password")
	dispatcher, err := NewPasswordResetDispatcher(delivery, store)
	if err != nil {
		t.Fatal(err)
	}
	if !dispatcher.Submit("first@example.test") || !dispatcher.Submit("second@example.test") {
		t.Fatal("queue rejected available capacity")
	}
	for i := 0; i < 2; i++ {
		select {
		case <-users.entered:
		case <-time.After(time.Second):
			t.Fatal("workers did not begin")
		}
	}
	for i := 0; i < passwordResetQueueCapacity; i++ {
		if !dispatcher.Submit("queued@example.test") {
			t.Fatal("queue lost available capacity")
		}
	}
	if dispatcher.Submit("overflow@example.test") || users.calls.Load() != 2 {
		t.Fatal("delivery concurrency or pending queue exceeded bounds")
	}
	stop, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.Stop(stop); err != nil {
		t.Fatal(err)
	}
	if dispatcher.Submit("after-stop@example.test") || dispatcher.Stop(stop) != nil {
		t.Fatal("stopped dispatcher accepted work or stop was not idempotent")
	}
}
func TestPasswordResetDispatcherCancellationRevokesIssuedToken(t *testing.T) {
	store := newResetTestStore()
	sending := make(chan struct{})
	delivery, _ := NewPasswordResetDelivery(emailSenderFunc(func(ctx context.Context, _, _, _ string) error { close(sending); <-ctx.Done(); return ctx.Err() }), "https://example.test/reset-password")
	dispatcher, err := NewPasswordResetDispatcher(delivery, store)
	if err != nil {
		t.Fatal(err)
	}
	if !dispatcher.Submit("person@example.test") {
		t.Fatal("request rejected")
	}
	select {
	case <-sending:
	case <-time.After(time.Second):
		t.Fatal("private delivery did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dispatcher.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if !store.resets.used || store.resets.revokeContextErr != nil {
		t.Fatal("shutdown left issued, undelivered token usable")
	}
}
func TestPasswordResetFallbackWorkersExit(t *testing.T) {
	finished := make(chan struct{})
	if !submitResetFallback(func(ctx context.Context) {
		if ctx.Err() != nil {
			t.Error("independent context canceled")
		}
		close(finished)
	}) {
		t.Fatal("fallback rejected request")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("fallback never processed")
	}
	deadline := time.Now().Add(time.Second)
	for {
		resetFallbackPool.mu.Lock()
		idle := resetFallbackPool.workers == 0 && len(resetFallbackPool.queue) == 0
		resetFallbackPool.mu.Unlock()
		if idle {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("fallback retained idle workers")
		}
		time.Sleep(time.Millisecond)
	}
}
func TestPasswordResetDispatcherMissingDelivery(t *testing.T) {
	if _, err := NewPasswordResetDispatcher(nil, newResetTestStore()); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatal("unconfigured queue accepted")
	}
}
