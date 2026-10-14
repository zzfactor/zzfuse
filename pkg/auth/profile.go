package auth

import (
	"context"
	"database/sql"
	"errors"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

type UserProfile struct {
	ID    int64  `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

// GetUserProfile retrieves a user's profile by ID.
func GetUserProfile(ctx context.Context, db *sql.DB, userID int64) (*UserProfile, error) {
	store := pgstore.New(db)
	u, err := store.Users().GetByID(ctx, userID)
	if err != nil || u == nil {
		return nil, errors.New("user not found")
	}
	return &UserProfile{ID: u.ID, Email: u.Email, Name: u.Name}, nil
}

// UpdateUserProfile updates a user's profile fields (email, name).
func UpdateUserProfile(ctx context.Context, db *sql.DB, userID int64, email, name string) error {
	store := pgstore.New(db)
	return store.Users().UpdateEmailAndName(ctx, userID, email, name)
}
