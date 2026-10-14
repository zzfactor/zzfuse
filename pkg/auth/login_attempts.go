package auth

import (
	"context"
	"database/sql"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// EnsureLoginAttemptsTable creates the auth_login_attempts table if it does not exist.
func EnsureLoginAttemptsTable(db *sql.DB) error {
	store := pgstore.New(db)
	return store.LoginAttempts().EnsureSchema(context.Background())
}
