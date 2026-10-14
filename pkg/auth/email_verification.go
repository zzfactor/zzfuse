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

// generateEmailVerificationToken creates a secure random token for email verification.
func generateEmailVerificationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashEmailToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateEmailVerificationToken creates and stores a verification token for a user.
func CreateEmailVerificationToken(ctx context.Context, db *sql.DB, userID int64, duration time.Duration) (string, error) {
	token, err := generateEmailVerificationToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().UTC().Add(duration)
	h := hashEmailToken(token)
	store := pgstore.New(db)
	if err := store.EmailVerifications().Create(ctx, userID, h, expiresAt); err != nil {
		return "", err
	}
	return token, nil
}

// VerifyEmailVerificationToken checks if a token is valid and returns the user ID.
func VerifyEmailVerificationToken(ctx context.Context, db *sql.DB, token string) (int64, error) {
	store := pgstore.New(db)
	h := hashEmailToken(token)
	userID, expiresAt, used, err := store.EmailVerifications().Lookup(ctx, h)
	if err != nil || userID == 0 {
		return 0, errors.New("invalid token")
	}
	if used || time.Now().UTC().After(expiresAt) {
		return 0, errors.New("token expired or already used")
	}
	return userID, nil
}

// MarkEmailVerificationUsed marks a token as used.
func MarkEmailVerificationUsed(ctx context.Context, db *sql.DB, token string) error {
	store := pgstore.New(db)
	h := hashEmailToken(token)
	return store.EmailVerifications().MarkUsed(ctx, h)
}

// EnsureEmailVerificationsTable creates the auth_email_verifications table if it does not exist.
func EnsureEmailVerificationsTable(db *sql.DB) error {
	store := pgstore.New(db)
	return store.EmailVerifications().EnsureSchema(context.Background())
}
