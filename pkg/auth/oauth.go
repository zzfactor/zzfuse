package auth

import (
	"context"
	"database/sql"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// EnsureOAuthTables creates all tables required for OAuth login and linking.
func EnsureOAuthTables(db *sql.DB) error {
	store := pgstore.New(db)
	return store.Identities().EnsureSchema(context.Background())
}
