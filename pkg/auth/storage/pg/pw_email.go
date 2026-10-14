package pg

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type passwordResetsRepo struct{ s *PGStore }

type emailVerificationsRepo struct{ s *PGStore }

// PasswordResets implementation
func (r *passwordResetsRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlPasswordResets)
	if err != nil {
		return fmt.Errorf("passwordResetsRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("passwordResetsRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

func (r *passwordResetsRepo) Cleanup(ctx context.Context) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("DELETE FROM %s.auth_password_resets WHERE (used_at IS NOT NULL) OR (expires_at < NOW())", r.s.schema))
	if err != nil {
		return fmt.Errorf("passwordResetsRepo.Cleanup: %w", err)
	}
	return nil
}

func (r *passwordResetsRepo) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("INSERT INTO %s.auth_password_resets (user_id, token_hash, expires_at) VALUES ($1,$2,$3)", r.s.schema), userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("passwordResetsRepo.Create: %w", err)
	}
	return nil
}

func (r *passwordResetsRepo) Lookup(ctx context.Context, tokenHash string) (int64, time.Time, bool, error) {
	var userID int64
	var expiresAt time.Time
	var usedAt sql.NullTime
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT user_id, expires_at, used_at FROM %s.auth_password_resets WHERE token_hash=$1", r.s.schema), tokenHash)
	if err := row.Scan(&userID, &expiresAt, &usedAt); err != nil {
		if err == sql.ErrNoRows {
			return 0, time.Time{}, false, nil
		}
		return 0, time.Time{}, false, fmt.Errorf("passwordResetsRepo.Lookup: %w", err)
	}
	return userID, expiresAt, usedAt.Valid, nil
}

func (r *passwordResetsRepo) MarkUsed(ctx context.Context, tokenHash string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("UPDATE %s.auth_password_resets SET used_at=NOW() WHERE token_hash=$1", r.s.schema), tokenHash)
	if err != nil {
		return fmt.Errorf("passwordResetsRepo.MarkUsed: %w", err)
	}
	return nil
}

// Consume atomically claims an unexpired, unused reset token. When called through
// PGStore.WithTx, the claim and password update commit or roll back together.
func (r *passwordResetsRepo) Consume(ctx context.Context, tokenHash string) (int64, error) {
	var userID int64
	err := r.s.queryRow(ctx, fmt.Sprintf("UPDATE %s.auth_password_resets SET used_at=NOW() WHERE token_hash=$1 AND used_at IS NULL AND expires_at>NOW() RETURNING user_id", r.s.schema), tokenHash).Scan(&userID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("passwordResetsRepo.Consume: %w", err)
	}
	return userID, nil
}

// EmailVerifications implementation
func (r *emailVerificationsRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlEmailVerifications)
	if err != nil {
		return fmt.Errorf("emailVerificationsRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("emailVerificationsRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

func (r *emailVerificationsRepo) Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("INSERT INTO %s.auth_email_verifications (user_id, token_hash, expires_at) VALUES ($1,$2,$3)", r.s.schema), userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("emailVerificationsRepo.Create: %w", err)
	}
	return nil
}

func (r *emailVerificationsRepo) Lookup(ctx context.Context, tokenHash string) (int64, time.Time, bool, error) {
	var userID int64
	var expiresAt time.Time
	var usedAt sql.NullTime
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT user_id, expires_at, used_at FROM %s.auth_email_verifications WHERE token_hash=$1", r.s.schema), tokenHash)
	if err := row.Scan(&userID, &expiresAt, &usedAt); err != nil {
		if err == sql.ErrNoRows {
			return 0, time.Time{}, false, nil
		}
		return 0, time.Time{}, false, fmt.Errorf("emailVerificationsRepo.Lookup: %w", err)
	}
	return userID, expiresAt, usedAt.Valid, nil
}

func (r *emailVerificationsRepo) MarkUsed(ctx context.Context, tokenHash string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("UPDATE %s.auth_email_verifications SET used_at=NOW() WHERE token_hash=$1", r.s.schema), tokenHash)
	if err != nil {
		return fmt.Errorf("emailVerificationsRepo.MarkUsed: %w", err)
	}
	return nil
}
