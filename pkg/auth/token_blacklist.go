package auth

import (
	"context"
	"database/sql"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// EnsureTokenBlacklistTable creates the auth_token_blacklist table if it does not exist.
func EnsureTokenBlacklistTable(db *sql.DB) error {
	store := pgstore.New(db)
	return store.TokenBlacklist().EnsureSchema(context.Background())
}
