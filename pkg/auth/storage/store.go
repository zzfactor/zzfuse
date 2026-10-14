package storage

import (
	"context"
	"time"
)

// Store is the unit-of-work entrypoint. Implementations may wrap a DB or a transaction.
type Store interface {
	APIKeys() APIKeys
	Users() Users
	Identities() Identities
	LoginAttempts() LoginAttempts
	MFASecrets() MFASecrets
	PasswordResets() PasswordResets
	EmailVerifications() EmailVerifications
	TokenBlacklist() TokenBlacklist
	AuditLogs() AuditLogs
	Roles() Roles
	// WithTx runs fn within a transaction boundary, providing a tx-scoped Store.
	WithTx(ctx context.Context, fn func(tx Store) error) error
}

// APIKeys provides persistence operations for API keys.
type APIKeys interface {
	EnsureSchema(ctx context.Context) error
	Insert(ctx context.Context, p InsertAPIKeyParams) (*APIKeyRecord, error)
	ListByUser(ctx context.Context, userID int64) ([]*APIKeyRecord, error)
	FindByPrefix(ctx context.Context, prefix string) ([]StoredAPIKey, error)
	MarkUsedNow(ctx context.Context, prefix string, keyHash string) error
	DeleteByID(ctx context.Context, userID int64, keyID string) error
}

// InsertAPIKeyParams carries create-time fields persisted for a new key.
type InsertAPIKeyParams struct {
	ID         string
	UserID     int64
	OrgID      *int64
	ProjectID  *int64
	Name       string
	Prefix     string
	KeyHash    string
	ScopesJSON []byte
	ExpiresAt  *time.Time
}

// APIKeyRecord represents a persisted API key for list/read flows.
type APIKeyRecord struct {
	ID         string
	UserID     int64
	OrgID      *int64
	ProjectID  *int64
	Name       string
	Prefix     string
	ScopesJSON []byte
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
}

// StoredAPIKey is used for validation paths and includes hash and expiry.
type StoredAPIKey struct {
	ID         string
	UserID     int64
	OrgID      *int64
	ProjectID  *int64
	ScopesJSON []byte
	KeyHash    string
	ExpiresAt  *time.Time
}

// Users repository abstracts user persistence.
type Users interface {
	EnsureSchema(ctx context.Context) error
	GetByEmail(ctx context.Context, email string) (*UserRecord, error)
	Create(ctx context.Context, p CreateUserParams) (*UserRecord, error)
	UpdatePasswordHash(ctx context.Context, userID int64, passwordHash string) error
	ExistsByID(ctx context.Context, userID int64) (bool, error)
	GetEmailByID(ctx context.Context, userID int64) (string, error)
	GetByID(ctx context.Context, userID int64) (*UserRecord, error)
	UpdateEmailAndName(ctx context.Context, userID int64, email, name string) error
	GetPasswordHashByID(ctx context.Context, userID int64) (string, error)

	// Extended helpers used by auth flows
	ExistsByUsernameOrEmail(ctx context.Context, usernameLower, emailLower string) (bool, error)
	InsertFull(ctx context.Context, p CreateUserFullParams) (id int64, createdAt time.Time, err error)
	Count(ctx context.Context) (int, error)
	GetByEmailFull(ctx context.Context, emailLower string) (*UserFull, error)
	GetByIdentifierFull(ctx context.Context, identLower string) (*UserFull, error)
	ListAll(ctx context.Context) ([]*UserFull, error)
	DeleteByID(ctx context.Context, userID int64) error
}

// Identities repository abstracts social identity linking.
type Identities interface {
	EnsureSchema(ctx context.Context) error
	GetUserIDByProvider(ctx context.Context, provider, providerUserID string) (int64, error)
	Link(ctx context.Context, userID int64, provider, providerUserID, email string) error
}

// PasswordResets repository abstracts password reset token storage.
type PasswordResets interface {
	EnsureSchema(ctx context.Context) error
	Cleanup(ctx context.Context) error
	Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	Lookup(ctx context.Context, tokenHash string) (userID int64, expiresAt time.Time, used bool, err error)
	MarkUsed(ctx context.Context, tokenHash string) error
	Consume(ctx context.Context, tokenHash string) (int64, error)
}

// EmailVerifications repository abstracts email verification token storage.
type EmailVerifications interface {
	EnsureSchema(ctx context.Context) error
	Create(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error
	Lookup(ctx context.Context, tokenHash string) (userID int64, expiresAt time.Time, used bool, err error)
	MarkUsed(ctx context.Context, tokenHash string) error
}

// TokenBlacklist manages JWT denylist entries
type TokenBlacklist interface {
	EnsureSchema(ctx context.Context) error
	Insert(ctx context.Context, jti string, expiresAt time.Time) error
	ExistsActive(ctx context.Context, jti string) (bool, error)
	CleanupExpired(ctx context.Context) error
}

// AuditLogs stores admin/auth audit events
type AuditLogs interface {
	EnsureSchema(ctx context.Context) error
	Insert(ctx context.Context, actorID int64, action string, targetUserID *int64, details, ip string, at time.Time) error
}

// Roles manages user roles
type Roles interface {
	EnsureSchema(ctx context.Context) error
	Assign(ctx context.Context, userID int64, role string) error
	Remove(ctx context.Context, userID int64, role string) error
	Has(ctx context.Context, userID int64, role string) (bool, error)
	List(ctx context.Context, userID int64) ([]string, error)
}

// LoginAttempts manages login attempt tracking and lockouts
type LoginAttempts interface {
	EnsureSchema(ctx context.Context) error
	Record(ctx context.Context, userID int64, when time.Time, success bool) error
	IsLocked(ctx context.Context, userID int64, window time.Duration, maxFailures int, now time.Time, lockout time.Duration) (bool, error)
	Clear(ctx context.Context, userID int64) error
}

// MFASecrets stores per-user MFA data
type MFASecrets interface {
	EnsureSchema(ctx context.Context) error
	Enable(ctx context.Context, userID int64, secret string, at time.Time) error
	Disable(ctx context.Context, userID int64) error
	IsEnabled(ctx context.Context, userID int64) (bool, error)
	GetSecretIfEnabled(ctx context.Context, userID int64) (string, error)
}

// UserRecord minimal data used by auth flows.
type UserRecord struct {
	ID    int64
	Email string
	Name  string
}

// UserFull matches broader user fields used by admin/auth flows.
type UserFull struct {
	ID            int64
	Username      string
	Email         string
	PasswordHash  string
	CreatedAt     time.Time
	EmailVerified bool
	Name          string
	FirstName     string
	LastName      string
	Address       string
	City          string
	State         string
	Country       string
	Company       string
	MetadataJSON  []byte
}

// CreateUserFullParams for inserting a full user record
type CreateUserFullParams struct {
	Username      string
	Email         string
	PasswordHash  string
	EmailVerified bool
	Name          string
	FirstName     string
	LastName      string
	Address       string
	City          string
	State         string
	Country       string
	Company       string
	MetadataJSON  []byte
}

// CreateUserParams carries fields for new user creation.
type CreateUserParams struct {
	Email         string
	PasswordHash  string
	EmailVerified bool
	Name          string
}
