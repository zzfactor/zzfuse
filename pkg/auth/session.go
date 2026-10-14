package auth

import (
	"context"
	"database/sql"
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
	return ValidateSessionJWT(ctx, db, tokenString)
}
