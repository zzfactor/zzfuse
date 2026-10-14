package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// BlacklistToken stores a JWT jti in the blacklist until its expiry.
func BlacklistToken(ctx context.Context, db *sql.DB, jti string, expiresAt time.Time) error {
	store := pgstore.New(db)
	return store.TokenBlacklist().Insert(ctx, jti, expiresAt)
}

// IsTokenBlacklisted checks if a JWT jti is blacklisted.
func IsTokenBlacklisted(ctx context.Context, db *sql.DB, jti string) (bool, error) {
	store := pgstore.New(db)
	return store.TokenBlacklist().ExistsActive(ctx, jti)
}

// CleanupExpiredBlacklistedTokens removes expired tokens from the blacklist.
func CleanupExpiredBlacklistedTokens(ctx context.Context, db *sql.DB) error {
	store := pgstore.New(db)
	return store.TokenBlacklist().CleanupExpired(ctx)
}

// ValidateJWTAndCheckDenylist validates the token signature/claims and checks the denylist by jti.
func ValidateJWTAndCheckDenylist(ctx context.Context, db *sql.DB, tokenString string) (*Claims, error) {
	claims, err := ValidateJWT(tokenString)
	if err != nil {
		return nil, err
	}
	if claims.ID == "" {
		return nil, errors.New("missing jti")
	}
	blacklisted, err := IsTokenBlacklisted(ctx, db, claims.ID)
	if err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, errors.New("token revoked")
	}
	return claims, nil
}
