package auth

import (
	"context"
	"database/sql"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// EnsureUserRolesTable creates the auth_user_roles table if it does not exist.
func EnsureUserRolesTable(db *sql.DB) error {
	store := pgstore.New(db)
	return store.Roles().EnsureSchema(context.Background())
}
