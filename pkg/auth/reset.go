package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// generateResetToken creates a secure random token for password resets.
func generateResetToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Housekeeping: Delete expired or used password reset tokens
func CleanupPasswordResetTokens(ctx context.Context, db *sql.DB) error {
	store := pgstore.New(db)
	return store.PasswordResets().Cleanup(ctx)
}

// CreatePasswordResetToken creates and stores a reset token for a user.
func CreatePasswordResetToken(ctx context.Context, db *sql.DB, email string, duration time.Duration) (string, error) {
	// Housekeeping before creating a new token
	_ = CleanupPasswordResetTokens(ctx, db)
	store := pgstore.New(db)
	u, err := store.Users().GetByEmail(ctx, email)
	if err != nil || u == nil {
		return "", errors.New("user not found")
	}
	token, err := generateResetToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().UTC().Add(duration)
	tokenHash := hashToken(token)
	if err := store.PasswordResets().Create(ctx, u.ID, tokenHash, expiresAt); err != nil {
		return "", err
	}
	return token, nil
}

// VerifyPasswordResetToken checks if a token is valid and returns the user ID.
func VerifyPasswordResetToken(ctx context.Context, db *sql.DB, token string) (int64, error) {
	// Housekeeping before verifying a token
	_ = CleanupPasswordResetTokens(ctx, db)
	store := pgstore.New(db)
	tokenHash := hashToken(token)
	userID, expiresAt, used, err := store.PasswordResets().Lookup(ctx, tokenHash)
	if err != nil || userID == 0 {
		return 0, errors.New("invalid token")
	}
	if used || time.Now().UTC().After(expiresAt) {
		return 0, errors.New("token expired or already used")
	}
	return userID, nil
}

// MarkPasswordResetUsed marks a token as used.
func MarkPasswordResetUsed(ctx context.Context, db *sql.DB, token string) error {
	store := pgstore.New(db)
	tokenHash := hashToken(token)
	return store.PasswordResets().MarkUsed(ctx, tokenHash)
}

// ResetPassword sets a new password for the user.
func ResetPassword(ctx context.Context, db *sql.DB, userID int64, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	store := pgstore.New(db)
	return store.Users().UpdatePasswordHash(ctx, userID, hash)
}

// EnsurePasswordResetsTable delegating to storage implementation for schema.
func EnsurePasswordResetsTable(db *sql.DB) error {
	store := pgstore.New(db)
	return store.PasswordResets().EnsureSchema(context.Background())
}
