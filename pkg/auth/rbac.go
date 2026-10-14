package auth

import (
	"context"
	"database/sql"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// AssignRole assigns a role to a user.
func AssignRole(ctx context.Context, db *sql.DB, userID int64, role string) error {
	store := pgstore.New(db)
	return store.Roles().Assign(ctx, userID, role)
}

// RemoveRole removes a role from a user.
func RemoveRole(ctx context.Context, db *sql.DB, userID int64, role string) error {
	store := pgstore.New(db)
	return store.Roles().Remove(ctx, userID, role)
}

// HasRole checks if a user has a specific role.
func HasRole(ctx context.Context, db *sql.DB, userID int64, role string) (bool, error) {
	store := pgstore.New(db)
	return store.Roles().Has(ctx, userID, role)
}

// ListRoles lists all roles assigned to a user.
func ListRoles(ctx context.Context, db *sql.DB, userID int64) ([]string, error) {
	store := pgstore.New(db)
	return store.Roles().List(ctx, userID)
}
