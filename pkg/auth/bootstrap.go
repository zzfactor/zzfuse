package auth

import (
	"context"
	"database/sql"
	"strings"
)

// BootstrapUser ensures auth tables exist and that a user with the given email exists.
// If the user does not exist, it registers the user with the provided password.
// If role is non-empty, it assigns that role (idempotent overall).
func BootstrapUser(ctx context.Context, db *sql.DB, email, password, role string) error {
	if db == nil {
		return nil
	}
	// Ensure all auth tables exist
	if err := EnsureAllAuthTables(db); err != nil {
		return err
	}
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" || password == "" {
		return nil
	}
	// If user exists, nothing to do (idempotent)
	u, err := GetUserByEmail(ctx, db, email)
	if err == nil && u != nil {
		return nil
	}
	// Create user
	created, err := Register(ctx, db, email, password, "", "", "", "", "", "", "", "", nil)
	if err != nil {
		return err
	}
	// Assign role if provided
	if role = strings.TrimSpace(role); role != "" {
		_ = AssignRole(ctx, db, created.ID, role)
	}
	return nil
}
