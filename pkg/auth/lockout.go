package auth

import (
	"context"
	"database/sql"
	"time"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

const (
	MaxFailedAttempts  = 5
	LockoutDuration    = 15 * time.Minute
	LoginAttemptWindow = 15 * time.Minute
)

// RecordLoginAttempt records a login attempt for a user (success or failure).
func RecordLoginAttempt(ctx context.Context, db *sql.DB, userID int64, success bool) error {
	store := pgstore.New(db)
	return store.LoginAttempts().Record(ctx, userID, time.Now().UTC(), success)
}

// IsAccountLocked checks if the user account is locked due to too many failed attempts.
func IsAccountLocked(ctx context.Context, db *sql.DB, userID int64) (bool, error) {
	store := pgstore.New(db)
	return store.LoginAttempts().IsLocked(ctx, userID, LoginAttemptWindow, MaxFailedAttempts, time.Now(), LockoutDuration)
}

// ClearLoginAttempts clears login attempts for a user (on successful login).
func ClearLoginAttempts(ctx context.Context, db *sql.DB, userID int64) error {
	store := pgstore.New(db)
	return store.LoginAttempts().Clear(ctx, userID)
}
