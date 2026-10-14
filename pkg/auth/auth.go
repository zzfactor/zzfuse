// Package auth provides user authentication logic for the Tracer extension.
// It supports registration, login, password hashing, and JWT issuance/validation.
package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// User represents a user in the system.
type User struct {
	ID            int64                  `json:"id"`
	Username      string                 `json:"username,omitempty"`
	Email         string                 `json:"email"`
	PasswordHash  string                 `json:"-"`
	CreatedAt     string                 `json:"created_at"`
	EmailVerified bool                   `json:"email_verified"`
	Name          string                 `json:"name,omitempty"`
	FirstName     string                 `json:"first_name,omitempty"`
	LastName      string                 `json:"last_name,omitempty"`
	Address       string                 `json:"address,omitempty"`
	City          string                 `json:"city,omitempty"`
	State         string                 `json:"state,omitempty"`
	Country       string                 `json:"country,omitempty"`
	Company       string                 `json:"company,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// GetUserByID fetches a user by numeric ID.
func GetUserByID(ctx context.Context, db *sql.DB, id int64) (*User, error) {
	var u User
	var metadataJSON []byte
	q := fmt.Sprintf(`
        SELECT id, COALESCE(username,''), email, created_at, COALESCE(name,''), COALESCE(first_name,''), COALESCE(last_name,''), COALESCE(address,''), COALESCE(city,''), COALESCE(state,''), COALESCE(country,''), COALESCE(company,''), metadata
        FROM %s.auth_users
        WHERE id=$1
    `, pgstore.DefaultConfigSchema)
	err := db.QueryRowContext(ctx, q, id).Scan(
		&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.Name, &u.FirstName, &u.LastName,
		&u.Address, &u.City, &u.State, &u.Country, &u.Company, &metadataJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &u.Metadata)
	}
	return &u, nil
}

// RegisterWithUsername registers a new user with the given username, email, password, and optional profile fields.
func RegisterWithUsername(ctx context.Context, db *sql.DB, username, email, password, name, firstName, lastName, address, city, state, country, company string, metadata map[string]interface{}) (*User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(strings.ToLower(email))
	if username == "" || password == "" {
		return nil, errors.New("username and password required")
	}
	// Check if user already exists
	var exists bool
	err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM %s.auth_users WHERE LOWER(username)=LOWER($1) OR LOWER(email)=LOWER($2))", pgstore.DefaultConfigSchema), username, email).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("check existing user: %w", err)
	}
	if exists {
		return nil, errors.New("user already exists")
	}
	// Hash password
	hash, err := HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	// Marshal metadata
	var metadataJSON []byte
	if metadata != nil {
		metadataJSON, err = json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
	}
	// Serialize first-administrator selection with administrative changes.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", authMutationLock); err != nil {
		return nil, err
	}
	// Insert user
	var id int64
	var createdAt time.Time
	q2 := fmt.Sprintf(`INSERT INTO %s.auth_users (username, email, password_hash, email_verified, name, first_name, last_name, address, city, state, country, company, metadata) VALUES ($1, $2, $3, FALSE, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id, created_at`, pgstore.DefaultConfigSchema)
	err = tx.QueryRowContext(ctx,
		q2,
		username, email, hash, name, firstName, lastName, address, city, state, country, company, metadataJSON,
	).Scan(&id, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	// Assign 'admin' role to the first user
	var userCount int
	err = tx.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s.auth_users", pgstore.DefaultConfigSchema)).Scan(&userCount)
	if err != nil {
		return nil, err
	}
	if userCount == 1 {
		_, errRole := tx.ExecContext(ctx, "INSERT INTO admin.auth_user_roles(user_id,role) VALUES ($1,'admin') ON CONFLICT(user_id,role) DO NOTHING", id)
		if errRole != nil {
			return nil, errRole
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &User{
		ID:            id,
		Username:      username,
		Email:         email,
		PasswordHash:  hash,
		CreatedAt:     createdAt.Format(time.RFC3339),
		EmailVerified: false,
		Name:          name,
		FirstName:     firstName,
		LastName:      lastName,
		Address:       address,
		City:          city,
		State:         state,
		Country:       country,
		Company:       company,
		Metadata:      metadata,
	}, nil
}

// Register preserves the previous email-first signature for compatibility. It will set username to the provided email
// unless a non-empty "username" is present in metadata (string), in which case that will be used.
func Register(ctx context.Context, db *sql.DB, email, password, name, firstName, lastName, address, city, state, country, company string, metadata map[string]interface{}) (*User, error) {
	var username string
	if v, ok := metadata["username"]; ok {
		if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
			username = s
		}
	}
	if username == "" {
		username = email
	}
	return RegisterWithUsername(ctx, db, username, email, password, name, firstName, lastName, address, city, state, country, company, metadata)
}

// LoginByIdentifier authenticates a user by username or email and returns a JWT if successful.
func LoginByIdentifier(ctx context.Context, db *sql.DB, identifier, password string) (string, error) {
	if db == nil {
		return "", ErrSessionDatabaseRequired
	}
	idLower := strings.TrimSpace(strings.ToLower(identifier))
	if idLower == "" || password == "" {
		return "", errors.New("username/email and password required")
	}
	var id int64
	var hash string
	var email, username string
	var authVersion int64
	var disabled bool
	// Allow login by username or email (username can be an email string)
	q := fmt.Sprintf("SELECT id, password_hash, email, COALESCE(username,''), auth_version, disabled FROM %s.auth_users WHERE LOWER(username)=$1 OR LOWER(email)=$1", pgstore.DefaultConfigSchema)
	err := db.QueryRowContext(ctx, q, idLower).Scan(&id, &hash, &email, &username, &authVersion, &disabled)
	if err == sql.ErrNoRows {
		return "", errors.New("invalid username/email or password")
	} else if err != nil {
		return "", fmt.Errorf("query user: %w", err)
	}
	if disabled || authVersion < 1 {
		return "", ErrInvalidSession
	}
	// Lockout check before verifying password
	locked, err := IsAccountLocked(ctx, db, id)
	if err != nil {
		return "", fmt.Errorf("lockout check: %w", err)
	}
	if locked {
		_ = LogAuditEvent(ctx, db, id, "login_locked", &id, "", "")
		return "", errors.New("account locked due to failed attempts")
	}
	if err := CheckPassword(hash, password); err != nil {
		_ = RecordLoginAttempt(ctx, db, id, false)
		_ = LogAuditEvent(ctx, db, id, "login_failed", &id, "", "")
		return "", errors.New("invalid username/email or password")
	}
	// Success
	_ = ClearLoginAttempts(ctx, db, id)
	_ = RecordLoginAttempt(ctx, db, id, true)
	_ = LogAuditEvent(ctx, db, id, "login_success", &id, "", "")

	// TODO: Enforce MFA: if enabled, require VerifyTOTP in separate step or include a flag
	// Issue JWT with TTL controlled by TRAZR_JWT_TTL (default 24h)
	expiry := 24 * time.Hour
	if ttlStr := strings.TrimSpace(os.Getenv("TRAZR_JWT_TTL")); ttlStr != "" {
		if d, perr := time.ParseDuration(ttlStr); perr == nil && d > 0 {
			expiry = d
		}
	}
	// Issue token including username and roles
	token, err := issueSessionJWT(ctx, db, id, expiry, authVersion, "")
	if err != nil {
		return "", fmt.Errorf("issue jwt: %w", err)
	}
	return token, nil
}

// Login preserves the previous email-first signature for compatibility. It authenticates using the identifier value
// equal to the provided email parameter.
func Login(ctx context.Context, db *sql.DB, email, password string) (string, error) {
	return LoginByIdentifier(ctx, db, email, password)
}

// EnsureUsersTable creates the auth_users table with all required columns if it does not exist.
func EnsureUsersTable(db *sql.DB) error {
	store, err := pgstore.NewWithSchema(db, pgstore.DefaultConfigSchema)
	if err != nil {
		return fmt.Errorf("create pg store: %w", err)
	}
	ctx := context.Background()
	if err := store.Users().EnsureSchema(ctx); err != nil {
		return err
	}
	return nil
}

// EnsureAllAuthTables creates all authentication-related tables if they do not exist.
func EnsureAllAuthTables(db *sql.DB) error {
	// Ensure via storage with default schema
	store, err := pgstore.NewWithSchema(db, pgstore.DefaultConfigSchema)
	if err != nil {
		return fmt.Errorf("create pg store: %w", err)
	}
	ctx := context.Background()
	if err := store.Users().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.PasswordResets().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.EmailVerifications().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.LoginAttempts().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.TokenBlacklist().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.Roles().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.MFASecrets().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.AuditLogs().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.Identities().EnsureSchema(ctx); err != nil {
		return err
	}
	if err := store.APIKeys().EnsureSchema(ctx); err != nil {
		return err
	}
	return nil
}

// ListUsers returns all auth_users in the system (admin use only).
func ListUsers(ctx context.Context, db *sql.DB) ([]*User, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf(`SELECT id, COALESCE(username,''), email, created_at, COALESCE(name,''), COALESCE(first_name,''), COALESCE(last_name,''), COALESCE(address,''), COALESCE(city,''), COALESCE(state,''), COALESCE(country,''), COALESCE(company,''), metadata FROM %s.auth_users`, pgstore.DefaultConfigSchema))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		var u User
		var metadataJSON []byte
		if err := rows.Scan(
			&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.Name, &u.FirstName, &u.LastName,
			&u.Address, &u.City, &u.State, &u.Country, &u.Company, &metadataJSON,
		); err != nil {
			return nil, err
		}
		if len(metadataJSON) > 0 {
			_ = json.Unmarshal(metadataJSON, &u.Metadata)
		}
		users = append(users, &u)
	}
	return users, nil
}

// DeleteUser deletes a user by ID (admin use only).
func DeleteUser(ctx context.Context, db *sql.DB, userID int64) error {
	_, err := db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s.auth_users WHERE id=$1`, pgstore.DefaultConfigSchema), userID)
	return err
}

// GetUserByEmail fetches a user by email address.
func GetUserByEmail(ctx context.Context, db *sql.DB, email string) (*User, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	var u User
	var metadataJSON []byte
	q := fmt.Sprintf(`SELECT id, COALESCE(username,''), email, created_at, COALESCE(name,''), COALESCE(first_name,''), COALESCE(last_name,''), COALESCE(address,''), COALESCE(city,''), COALESCE(state,''), COALESCE(country,''), COALESCE(company,''), metadata FROM %s.auth_users WHERE email=$1`, pgstore.DefaultConfigSchema)
	err := db.QueryRowContext(ctx, q, email).
		Scan(&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.Name, &u.FirstName, &u.LastName, &u.Address, &u.City, &u.State, &u.Country, &u.Company, &metadataJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &u.Metadata)
	}
	return &u, nil
}

// GetUserByIdentifier fetches a user by username or email (case-insensitive).
func GetUserByIdentifier(ctx context.Context, db *sql.DB, identifier string) (*User, error) {
	ident := strings.TrimSpace(strings.ToLower(identifier))
	var u User
	var metadataJSON []byte
	q3 := fmt.Sprintf(`
		SELECT id, COALESCE(username,''), email, created_at, COALESCE(name,''), COALESCE(first_name,''), COALESCE(last_name,''), COALESCE(address,''), COALESCE(city,''), COALESCE(state,''), COALESCE(country,''), COALESCE(company,''), metadata
		FROM %s.auth_users
		WHERE LOWER(username)=$1 OR LOWER(email)=$1
	`, pgstore.DefaultConfigSchema)
	err := db.QueryRowContext(ctx, q3, ident).Scan(
		&u.ID, &u.Username, &u.Email, &u.CreatedAt, &u.Name, &u.FirstName, &u.LastName,
		&u.Address, &u.City, &u.State, &u.Country, &u.Company, &metadataJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	if len(metadataJSON) > 0 {
		_ = json.Unmarshal(metadataJSON, &u.Metadata)
	}
	return &u, nil
}
