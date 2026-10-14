package auth

import (
	"context"
	"database/sql"
	"errors"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// ChangePassword changes a user's password, requiring the old password for verification.
func ChangePassword(ctx context.Context, db *sql.DB, userID int64, oldPassword, newPassword string) error {
	store := pgstore.New(db)
	hash, err := store.Users().GetPasswordHashByID(ctx, userID)
	if err != nil {
		return errors.New("user not found")
	}
	if err := CheckPassword(hash, oldPassword); err != nil {
		return errors.New("old password is incorrect")
	}
	hashNew, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return store.Users().UpdatePasswordHash(ctx, userID, hashNew)
}
