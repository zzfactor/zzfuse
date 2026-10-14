package auth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth/storage"
)

type resetTestStore struct {
	storage.Store
	users  *resetTestUsers
	resets *resetTestResets
	mu     sync.Mutex
}
type resetTestUsers struct {
	storage.Users
	user         *storage.UserRecord
	passwordHash string
	updateError  error
}
type resetTestResets struct {
	storage.PasswordResets
	userID                    int64
	hash                      string
	expires                   time.Time
	used                      bool
	createCount, consumeCount int
	revokeContextErr          error
}

func newResetTestStore() *resetTestStore {
	return &resetTestStore{users: &resetTestUsers{user: &storage.UserRecord{ID: 7, Email: "person@example.test"}}, resets: &resetTestResets{}}
}
func (s *resetTestStore) Users() storage.Users                   { return s.users }
func (s *resetTestStore) PasswordResets() storage.PasswordResets { return s.resets }
func (s *resetTestStore) WithTx(_ context.Context, fn func(storage.Store) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldUsed, oldHash := s.resets.used, s.users.passwordHash
	if err := fn(s); err != nil {
		s.resets.used, s.users.passwordHash = oldUsed, oldHash
		return err
	}
	return nil
}
func (u *resetTestUsers) GetByEmail(_ context.Context, email string) (*storage.UserRecord, error) {
	if u.user != nil && u.user.Email == email {
		return u.user, nil
	}
	return nil, nil
}
func (u *resetTestUsers) UpdatePasswordHash(_ context.Context, _ int64, hash string) error {
	if u.updateError != nil {
		return u.updateError
	}
	u.passwordHash = hash
	return nil
}
func (r *resetTestResets) Create(_ context.Context, userID int64, hash string, expires time.Time) error {
	r.userID, r.hash, r.expires, r.used = userID, hash, expires, false
	r.createCount++
	return nil
}
func (r *resetTestResets) MarkUsed(ctx context.Context, hash string) error {
	r.revokeContextErr = ctx.Err()
	if hash != r.hash {
		return errors.New("wrong token hash")
	}
	r.used = true
	return nil
}
func (r *resetTestResets) Consume(_ context.Context, hash string) (int64, error) {
	r.consumeCount++
	if hash != r.hash || r.used || !r.expires.After(time.Now()) {
		return 0, nil
	}
	r.used = true
	return r.userID, nil
}
func tokenInResetMail(t *testing.T, body string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://") {
			u, err := url.Parse(line)
			if err != nil {
				t.Fatal("invalid private reset link")
			}
			fragment, err := url.ParseQuery(u.Fragment)
			if err != nil || u.Query().Get("token") != "" {
				t.Fatal("reset token must appear only in URL fragment")
			}
			token := fragment.Get("token")
			if token != "" {
				return token
			}
		}
	}
	t.Fatal("email lacks private reset link")
	return ""
}
func TestPasswordResetPrivateDeliveryAndMissingAccount(t *testing.T) {
	store := newResetTestStore()
	var recipient, mailBody string
	calls := 0
	sender := emailSenderFunc(func(_ context.Context, to, subject, body string) error {
		calls++
		recipient, mailBody = to, body
		if subject != "Reset your zzfuse password" {
			t.Fatal("unexpected subject")
		}
		return nil
	})
	delivery, err := NewPasswordResetDelivery(sender, "https://example.test/reset-password?source=test")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := delivery.Request(context.Background(), store, "  PERSON@example.test  "); err != nil {
		t.Fatal(err)
	}
	token := tokenInResetMail(t, mailBody)
	if recipient != store.users.user.Email || calls != 1 || store.resets.createCount != 1 || len(token) != 64 || store.resets.hash == token || store.resets.hash != hashToken(token) || store.resets.used || store.resets.expires.Before(start.Add(59*time.Minute)) || store.resets.expires.After(time.Now().Add(61*time.Minute)) {
		t.Fatal("private delivery recipient, token hashing or lifetime incorrect")
	}
	if err := delivery.Request(context.Background(), store, "missing@example.test"); err != nil || calls != 1 || store.resets.createCount != 1 {
		t.Fatal("missing account issued a token or sent mail")
	}
}
func TestPasswordResetFailedDeliveryRevokesAfterCancellation(t *testing.T) {
	store := newResetTestStore()
	ctx, cancel := context.WithCancel(context.Background())
	var issued string
	delivery, _ := NewPasswordResetDelivery(emailSenderFunc(func(_ context.Context, _, _, body string) error {
		issued = tokenInResetMail(t, body)
		cancel()
		return errors.New("private transport error that must not be logged")
	}), "https://example.test/reset-password")
	if err := delivery.Request(ctx, store, "person@example.test"); err == nil || strings.Contains(err.Error(), issued) || strings.Contains(err.Error(), "private transport error") {
		t.Fatal("delivery failure accepted or exposed private details")
	}
	if !store.resets.used || store.resets.revokeContextErr != nil {
		t.Fatal("failed/canceled delivery left token usable")
	}
}
func TestPasswordResetAtomicConsumptionAndReplay(t *testing.T) {
	t.Setenv("BCRYPT_COST", "4")
	t.Setenv("TRAZR_PASSWORD_PEPPER", "")
	t.Setenv("PASSWORD_PEPPER", "")
	store := newResetTestStore()
	token := "test-token"
	store.resets.userID, store.resets.hash, store.resets.expires = 7, hashToken(token), time.Now().Add(time.Hour)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- resetPasswordWithStore(context.Background(), store, token, "NewPassword123")
		}()
	}
	wg.Wait()
	close(results)
	accepted, rejected := 0, 0
	for err := range results {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrInvalidResetToken) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 1 || !store.resets.used || CheckPassword(store.users.passwordHash, "NewPassword123") != nil {
		t.Fatal("reset token was replayable or new password not stored")
	}
}
func TestPasswordResetFailedUpdateRollsBackConsumption(t *testing.T) {
	t.Setenv("BCRYPT_COST", "4")
	store := newResetTestStore()
	token := "test-token"
	store.resets.userID, store.resets.hash, store.resets.expires = 7, hashToken(token), time.Now().Add(time.Hour)
	store.users.updateError = errors.New("database update failed")
	if err := resetPasswordWithStore(context.Background(), store, token, "NewPassword123"); err == nil || store.resets.used || store.users.passwordHash != "" {
		t.Fatal("failed password update consumed token")
	}
	store.users.updateError = nil
	if err := resetPasswordWithStore(context.Background(), store, token, "NewPassword123"); err != nil {
		t.Fatal(err)
	}
}
func TestPasswordResetExpiredAndInvalidTokens(t *testing.T) {
	t.Setenv("BCRYPT_COST", "4")
	for _, expired := range []bool{true, false} {
		store := newResetTestStore()
		store.resets.userID, store.resets.hash, store.resets.expires = 7, hashToken("good-token"), time.Now().Add(time.Hour)
		token := "wrong-token"
		if expired {
			store.resets.expires = time.Now().Add(-time.Second)
			token = "good-token"
		}
		if err := resetPasswordWithStore(context.Background(), store, token, "NewPassword123"); !errors.Is(err, ErrInvalidResetToken) || store.resets.used || store.users.passwordHash != "" {
			t.Fatal("invalid or expired token accepted")
		}
	}
}
