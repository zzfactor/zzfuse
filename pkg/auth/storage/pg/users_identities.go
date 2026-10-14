package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth/storage"
)

type usersRepo struct{ s *PGStore }

type identitiesRepo struct{ s *PGStore }

// EnsureSchema creates the auth_users table, index, and backfill if needed.
func (r *usersRepo) EnsureSchema(ctx context.Context) error {
	// Ensure target schema exists before applying DDL
	if _, err := r.s.exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", r.s.schema)); err != nil {
		return fmt.Errorf("usersRepo.EnsureSchema: create schema: %w", err)
	}
	ddl, err := r.s.render(ddlUsers)
	if err != nil {
		return fmt.Errorf("usersRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("usersRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

// EnsureSchema creates the identities table if missing.
func (r *identitiesRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlUserIdentities)
	if err != nil {
		return fmt.Errorf("identitiesRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("identitiesRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

// --- Users basic operations ---

func (r *usersRepo) GetByEmail(ctx context.Context, email string) (*storage.UserRecord, error) {
	var u storage.UserRecord
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT id, email, name FROM %s.auth_users WHERE email=$1", r.s.schema), email)
	if err := row.Scan(&u.ID, &u.Email, &u.Name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("usersRepo.GetByEmail: %w", err)
	}
	return &u, nil
}

func (r *usersRepo) Create(ctx context.Context, p storage.CreateUserParams) (*storage.UserRecord, error) {
	var u storage.UserRecord
	if err := r.s.queryRow(ctx,
		fmt.Sprintf("INSERT INTO %s.auth_users (email, password_hash, email_verified, name) VALUES ($1,$2,$3,$4) RETURNING id, email, name", r.s.schema),
		p.Email, p.PasswordHash, p.EmailVerified, p.Name,
	).Scan(&u.ID, &u.Email, &u.Name); err != nil {
		return nil, fmt.Errorf("usersRepo.Create: %w", err)
	}
	return &u, nil
}

func (r *usersRepo) ExistsByID(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	if err := r.s.queryRow(ctx, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s.auth_users WHERE id=$1)", r.s.schema), userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("usersRepo.ExistsByID: %w", err)
	}
	return exists, nil
}

func (r *usersRepo) GetEmailByID(ctx context.Context, userID int64) (string, error) {
	var email string
	if err := r.s.queryRow(ctx, fmt.Sprintf("SELECT email FROM %s.auth_users WHERE id=$1", r.s.schema), userID).Scan(&email); err != nil {
		if err == sql.ErrNoRows {
			return "", errors.New("not found")
		}
		return "", fmt.Errorf("usersRepo.GetEmailByID: %w", err)
	}
	return email, nil
}

func (r *usersRepo) GetByID(ctx context.Context, userID int64) (*storage.UserRecord, error) {
	var u storage.UserRecord
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT id, email, name FROM %s.auth_users WHERE id=$1", r.s.schema), userID)
	if err := row.Scan(&u.ID, &u.Email, &u.Name); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("usersRepo.GetByID: %w", err)
	}
	return &u, nil
}

func (r *usersRepo) UpdateEmailAndName(ctx context.Context, userID int64, email, name string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("UPDATE %s.auth_users SET email=$1, name=$2 WHERE id=$3", r.s.schema), email, name, userID)
	if err != nil {
		return fmt.Errorf("usersRepo.UpdateEmailAndName: %w", err)
	}
	return nil
}

func (r *usersRepo) UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("UPDATE %s.auth_users SET password_hash=$1 WHERE id=$2", r.s.schema), passwordHash, userID)
	if err != nil {
		return fmt.Errorf("usersRepo.UpdatePasswordHash: %w", err)
	}
	return nil
}

func (r *usersRepo) GetPasswordHashByID(ctx context.Context, userID int64) (string, error) {
	var hash string
	if err := r.s.queryRow(ctx, fmt.Sprintf("SELECT password_hash FROM %s.auth_users WHERE id=$1", r.s.schema), userID).Scan(&hash); err != nil {
		return "", fmt.Errorf("usersRepo.GetPasswordHashByID: %w", err)
	}
	return hash, nil
}

// --- Users extended operations ---

func (r *usersRepo) ExistsByUsernameOrEmail(ctx context.Context, usernameLower, emailLower string) (bool, error) {
	var exists bool
	if err := r.s.queryRow(ctx, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s.auth_users WHERE LOWER(username)=LOWER($1) OR LOWER(email)=LOWER($2))", r.s.schema), usernameLower, emailLower).Scan(&exists); err != nil {
		return false, fmt.Errorf("usersRepo.ExistsByUsernameOrEmail: %w", err)
	}
	return exists, nil
}

func (r *usersRepo) InsertFull(ctx context.Context, p storage.CreateUserFullParams) (int64, time.Time, error) {
	var id int64
	var createdAt time.Time
	row := r.s.queryRow(ctx, fmt.Sprintf("INSERT INTO %s.auth_users (username, email, password_hash, email_verified, name, first_name, last_name, address, city, state, country, company, metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id, created_at", r.s.schema),
		p.Username, p.Email, p.PasswordHash, p.EmailVerified, p.Name, p.FirstName, p.LastName, p.Address, p.City, p.State, p.Country, p.Company, p.MetadataJSON,
	)
	if err := row.Scan(&id, &createdAt); err != nil {
		return 0, time.Time{}, fmt.Errorf("usersRepo.InsertFull: %w", err)
	}
	return id, createdAt, nil
}

func (r *usersRepo) Count(ctx context.Context) (int, error) {
	var n int
	if err := r.s.queryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s.auth_users", r.s.schema)).Scan(&n); err != nil {
		return 0, fmt.Errorf("usersRepo.Count: %w", err)
	}
	return n, nil
}

func (r *usersRepo) GetByEmailFull(ctx context.Context, emailLower string) (*storage.UserFull, error) {
	var u storage.UserFull
	var meta []byte
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT id, username, email, password_hash, created_at, email_verified, name, first_name, last_name, address, city, state, country, company, metadata FROM %s.auth_users WHERE LOWER(email)=LOWER($1)", r.s.schema), emailLower)
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.EmailVerified, &u.Name, &u.FirstName, &u.LastName, &u.Address, &u.City, &u.State, &u.Country, &u.Company, &meta); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("usersRepo.GetByEmailFull: %w", err)
	}
	u.MetadataJSON = meta
	return &u, nil
}

func (r *usersRepo) GetByIdentifierFull(ctx context.Context, identLower string) (*storage.UserFull, error) {
	var u storage.UserFull
	var meta []byte
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT id, username, email, password_hash, created_at, email_verified, name, first_name, last_name, address, city, state, country, company, metadata FROM %s.auth_users WHERE LOWER(username)=LOWER($1) OR LOWER(email)=LOWER($1)", r.s.schema), identLower)
	if err := row.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.EmailVerified, &u.Name, &u.FirstName, &u.LastName, &u.Address, &u.City, &u.State, &u.Country, &u.Company, &meta); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("usersRepo.GetByIdentifierFull: %w", err)
	}
	u.MetadataJSON = meta
	return &u, nil
}

func (r *usersRepo) ListAll(ctx context.Context) ([]*storage.UserFull, error) {
	rows, err := r.s.query(ctx, fmt.Sprintf("SELECT id, username, email, password_hash, created_at, email_verified, name, first_name, last_name, address, city, state, country, company, metadata FROM %s.auth_users", r.s.schema))
	if err != nil {
		return nil, fmt.Errorf("usersRepo.ListAll: %w", err)
	}
	defer rows.Close()
	var list []*storage.UserFull
	for rows.Next() {
		var u storage.UserFull
		var meta []byte
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.CreatedAt, &u.EmailVerified, &u.Name, &u.FirstName, &u.LastName, &u.Address, &u.City, &u.State, &u.Country, &u.Company, &meta); err != nil {
			return nil, fmt.Errorf("usersRepo.ListAll: scan: %w", err)
		}
		u.MetadataJSON = meta
		list = append(list, &u)
	}
	return list, nil
}

func (r *usersRepo) DeleteByID(ctx context.Context, userID int64) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("DELETE FROM %s.auth_users WHERE id=$1", r.s.schema), userID)
	if err != nil {
		return fmt.Errorf("usersRepo.DeleteByID: %w", err)
	}
	return nil
}

// --- Identities operations ---

func (r *identitiesRepo) GetUserIDByProvider(ctx context.Context, provider, providerUserID string) (int64, error) {
	var userID int64
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT user_id FROM %s.auth_identities WHERE provider=$1 AND provider_user_id=$2", r.s.schema), provider, providerUserID)
	if err := row.Scan(&userID); err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("identitiesRepo.GetUserIDByProvider: %w", err)
	}
	return userID, nil
}

func (r *identitiesRepo) Link(ctx context.Context, userID int64, provider, providerUserID, email string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("INSERT INTO %s.auth_identities (user_id, provider, provider_user_id, email) VALUES ($1,$2,$3,$4) ON CONFLICT (provider, provider_user_id) DO UPDATE SET user_id=EXCLUDED.user_id, email=EXCLUDED.email", r.s.schema), userID, provider, providerUserID, email)
	if err != nil {
		return fmt.Errorf("identitiesRepo.Link: %w", err)
	}
	return nil
}
