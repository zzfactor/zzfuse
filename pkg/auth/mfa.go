package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
	"github.com/pquerna/otp/totp"
)

// EnableMFA enables TOTP MFA for a user and stores the secret.
func EnableMFA(ctx context.Context, db *sql.DB, userID int64, secret string) error {
	q := fmt.Sprintf(`
		INSERT INTO %s.auth_mfa_secrets (user_id, secret, enabled, created_at)
		VALUES ($1, $2, TRUE, $3)
		ON CONFLICT (user_id) DO UPDATE SET secret=$2, enabled=TRUE, created_at=$3
	`, pgstore.DefaultConfigSchema)
	_, err := db.ExecContext(ctx, q, userID, secret, time.Now())
	return err
}

// DisableMFA disables TOTP MFA for a user.
func DisableMFA(ctx context.Context, db *sql.DB, userID int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE %s.auth_mfa_secrets SET enabled=FALSE WHERE user_id=$1
	`, pgstore.DefaultConfigSchema), userID)
	return err
}

// IsMFAEnabled checks if MFA is enabled for a user.
func IsMFAEnabled(ctx context.Context, db *sql.DB, userID int64) (bool, error) {
	var enabled bool
	err := db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT enabled FROM %s.auth_mfa_secrets WHERE user_id=$1
	`, pgstore.DefaultConfigSchema), userID).Scan(&enabled)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return enabled, nil
}

// VerifyTOTP verifies a TOTP code for a user.
func VerifyTOTP(ctx context.Context, db *sql.DB, userID int64, code string) (bool, error) {
	var secret string
	err := db.QueryRowContext(ctx, fmt.Sprintf(`
		SELECT secret FROM %s.auth_mfa_secrets WHERE user_id=$1 AND enabled=TRUE
	`, pgstore.DefaultConfigSchema), userID).Scan(&secret)
	if err != nil {
		return false, errors.New("MFA not enabled")
	}
	return totp.Validate(code, secret), nil
}

// EnsureMFASecretsTable creates the auth_mfa_secrets table if it does not exist.
func EnsureMFASecretsTable(db *sql.DB) error {
	store, err := pgstore.NewWithSchema(db, pgstore.DefaultConfigSchema)
	if err != nil {
		return fmt.Errorf("create pg store: %w", err)
	}
	return store.MFASecrets().EnsureSchema(context.Background())
}
