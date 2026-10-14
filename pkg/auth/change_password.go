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
	result, err := db.ExecContext(ctx, "UPDATE admin.auth_users SET password_hash=$1, auth_version=auth_version+1 WHERE id=$2 AND password_hash=$3 AND disabled=FALSE", hashNew, userID, hash)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.New("password changed or account unavailable")
	}
	return nil
}
