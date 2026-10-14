package pg

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"regexp"
	"text/template"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth/storage"
)

const DefaultConfigSchema = "admin"

// validSchemaPattern matches valid PostgreSQL identifier names.
// Only allows lowercase letters, numbers, and underscores, must start with letter or underscore.
var validSchemaPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// ValidateSQLSchema validates a schema name is safe for SQL interpolation.
// Returns an error if the schema contains invalid characters that could lead to SQL injection.
func ValidateSQLSchema(schema string) error {
	if schema == "" {
		return errors.New("schema name cannot be empty")
	}
	if len(schema) > 63 { // PostgreSQL max identifier length
		return errors.New("schema name exceeds maximum length of 63 characters")
	}
	if !validSchemaPattern.MatchString(schema) {
		return fmt.Errorf("invalid schema name %q: must contain only lowercase letters, numbers, and underscores, and start with a letter or underscore", schema)
	}
	return nil
}

// PGStore implements storage.Store using Postgres.
type PGStore struct {
	db     *sql.DB
	tx     *sql.Tx // optional tx context
	schema string
}

func New(db *sql.DB) *PGStore { return &PGStore{db: db, schema: DefaultConfigSchema} }

// NewWithSchema creates a PGStore with a custom schema name.
// Returns an error if the schema name is invalid to prevent SQL injection.
func NewWithSchema(db *sql.DB, schema string) (*PGStore, error) {
	if err := ValidateSQLSchema(schema); err != nil {
		return nil, fmt.Errorf("invalid schema for PGStore: %w", err)
	}
	return &PGStore{db: db, schema: schema}, nil
}

func (s *PGStore) APIKeys() storage.APIKeys               { return &apiKeysRepo{s: s} }
func (s *PGStore) Users() storage.Users                   { return &usersRepo{s: s} }
func (s *PGStore) Identities() storage.Identities         { return &identitiesRepo{s: s} }
func (s *PGStore) PasswordResets() storage.PasswordResets { return &passwordResetsRepo{s: s} }
func (s *PGStore) EmailVerifications() storage.EmailVerifications {
	return &emailVerificationsRepo{s: s}
}
func (s *PGStore) LoginAttempts() storage.LoginAttempts   { return &loginAttemptsRepo{s: s} }
func (s *PGStore) MFASecrets() storage.MFASecrets         { return &mfaSecretsRepo{s: s} }
func (s *PGStore) TokenBlacklist() storage.TokenBlacklist { return &tokenBlacklistRepo{s: s} }
func (s *PGStore) AuditLogs() storage.AuditLogs           { return &auditLogsRepo{s: s} }
func (s *PGStore) Roles() storage.Roles                   { return &rolesRepo{s: s} }

func (s *PGStore) WithTx(ctx context.Context, fn func(tx storage.Store) error) error {
	if s.tx != nil {
		return fn(s) // already in tx
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("PGStore.WithTx: begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	txs := &PGStore{db: s.db, tx: tx, schema: s.schema}
	if err = fn(txs); err != nil {
		return err
	}
	return tx.Commit()
}

// repo helpers
func (s *PGStore) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if s.tx != nil {
		return s.tx.ExecContext(ctx, query, args...)
	}
	return s.db.ExecContext(ctx, query, args...)
}
func (s *PGStore) query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if s.tx != nil {
		return s.tx.QueryContext(ctx, query, args...)
	}
	return s.db.QueryContext(ctx, query, args...)
}
func (s *PGStore) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
	if s.tx != nil {
		return s.tx.QueryRowContext(ctx, query, args...)
	}
	return s.db.QueryRowContext(ctx, query, args...)
}

// --- DDL Embedding and rendering ---

//go:embed ddl/auth_token_blacklist.sql
var ddlTokenBlacklist string

//go:embed ddl/auth_audit_logs.sql
var ddlAuditLogs string

//go:embed ddl/auth_user_roles.sql
var ddlUserRoles string

//go:embed ddl/auth_mfa_secrets.sql
var ddlMFASecrets string

//go:embed ddl/auth_login_attempts.sql
var ddlLoginAttempts string

//go:embed ddl/auth_apikeys.sql
var ddlAPIKeys string

//go:embed ddl/auth_users.sql
var ddlUsers string

//go:embed ddl/auth_user_identities.sql
var ddlUserIdentities string

//go:embed ddl/auth_password_resets.sql
var ddlPasswordResets string

//go:embed ddl/auth_email_verifications.sql
var ddlEmailVerifications string

func (s *PGStore) render(sqlTmpl string) (string, error) {
	var buf bytes.Buffer
	t := template.Must(template.New("ddl").Parse(sqlTmpl))
	if err := t.Execute(&buf, struct{ Schema string }{Schema: s.schema}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// --- Token blacklist repo ---

type tokenBlacklistRepo struct{ s *PGStore }

func (r *tokenBlacklistRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlTokenBlacklist)
	if err != nil {
		return fmt.Errorf("tokenBlacklistRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("tokenBlacklistRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}
func (r *tokenBlacklistRepo) Insert(ctx context.Context, jti string, expiresAt time.Time) error {
	q := fmt.Sprintf("INSERT INTO %s.auth_token_blacklist (jti, expires_at) VALUES ($1,$2) ON CONFLICT (jti) DO UPDATE SET expires_at=GREATEST(%s.auth_token_blacklist.expires_at,EXCLUDED.expires_at)", r.s.schema, r.s.schema)
	_, err := r.s.exec(ctx, q, jti, expiresAt)
	if err != nil {
		return fmt.Errorf("tokenBlacklistRepo.Insert: %w", err)
	}
	return nil
}

func (r *tokenBlacklistRepo) ExistsActive(ctx context.Context, jti string) (bool, error) {
	var count int
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s.auth_token_blacklist WHERE jti=$1 AND expires_at > NOW()", r.s.schema), jti)
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("tokenBlacklistRepo.ExistsActive: %w", err)
	}
	return count > 0, nil
}

func (r *tokenBlacklistRepo) CleanupExpired(ctx context.Context) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("DELETE FROM %s.auth_token_blacklist WHERE expires_at <= NOW()", r.s.schema))
	if err != nil {
		return fmt.Errorf("tokenBlacklistRepo.CleanupExpired: %w", err)
	}
	return nil
}

// --- Audit logs repo ---

type auditLogsRepo struct{ s *PGStore }

func (r *auditLogsRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlAuditLogs)
	if err != nil {
		return fmt.Errorf("auditLogsRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("auditLogsRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

func (r *auditLogsRepo) Insert(ctx context.Context, actorID int64, action string, targetUserID *int64, details, ip string, at time.Time) error {
	q := fmt.Sprintf("INSERT INTO %s.auth_audit_logs (actor_id, action, target_user_id, details, ip_address, created_at) VALUES ($1,$2,$3,$4,$5,$6)", r.s.schema)
	_, err := r.s.exec(ctx, q, actorID, action, targetUserID, details, ip, at)
	if err != nil {
		return fmt.Errorf("auditLogsRepo.Insert: %w", err)
	}
	return nil
}

// --- Login attempts repo ---

type loginAttemptsRepo struct{ s *PGStore }

func (r *loginAttemptsRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlLoginAttempts)
	if err != nil {
		return fmt.Errorf("loginAttemptsRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("loginAttemptsRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

func (r *loginAttemptsRepo) Record(ctx context.Context, userID int64, when time.Time, success bool) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("INSERT INTO %s.auth_login_attempts (user_id, attempted_at, success) VALUES ($1,$2,$3)", r.s.schema), userID, when, success)
	if err != nil {
		return fmt.Errorf("loginAttemptsRepo.Record: %w", err)
	}
	return nil
}

func (r *loginAttemptsRepo) IsLocked(ctx context.Context, userID int64, window time.Duration, maxFailures int, now time.Time, lockout time.Duration) (bool, error) {
	var lastFailed sql.NullTime
	var count int
	durSeconds := int(window.Seconds())
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT COALESCE(MAX(attempted_at), 'epoch'::timestamptz), COUNT(*) FROM %s.auth_login_attempts WHERE user_id=$1 AND success=FALSE AND attempted_at > NOW() - ($2 * interval '1 second')", r.s.schema), userID, durSeconds)
	if err := row.Scan(&lastFailed, &count); err != nil {
		return false, fmt.Errorf("loginAttemptsRepo.IsLocked: %w", err)
	}
	if count >= maxFailures && lastFailed.Valid && now.Sub(lastFailed.Time) < lockout {
		return true, nil
	}
	return false, nil
}

func (r *loginAttemptsRepo) Clear(ctx context.Context, userID int64) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("DELETE FROM %s.auth_login_attempts WHERE user_id=$1", r.s.schema), userID)
	if err != nil {
		return fmt.Errorf("loginAttemptsRepo.Clear: %w", err)
	}
	return nil
}

// --- MFA secrets repo ---

type mfaSecretsRepo struct{ s *PGStore }

func (r *mfaSecretsRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlMFASecrets)
	if err != nil {
		return fmt.Errorf("mfaSecretsRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("mfaSecretsRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

func (r *mfaSecretsRepo) Enable(ctx context.Context, userID int64, secret string, at time.Time) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("INSERT INTO %s.auth_mfa_secrets (user_id, secret, enabled, created_at) VALUES ($1,$2,TRUE,$3) ON CONFLICT (user_id) DO UPDATE SET secret=$2, enabled=TRUE, created_at=$3", r.s.schema), userID, secret, at)
	if err != nil {
		return fmt.Errorf("mfaSecretsRepo.Enable: %w", err)
	}
	return nil
}

func (r *mfaSecretsRepo) Disable(ctx context.Context, userID int64) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("UPDATE %s.auth_mfa_secrets SET enabled=FALSE WHERE user_id=$1", r.s.schema), userID)
	if err != nil {
		return fmt.Errorf("mfaSecretsRepo.Disable: %w", err)
	}
	return nil
}

func (r *mfaSecretsRepo) IsEnabled(ctx context.Context, userID int64) (bool, error) {
	var enabled bool
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT enabled FROM %s.auth_mfa_secrets WHERE user_id=$1", r.s.schema), userID)
	if err := row.Scan(&enabled); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("mfaSecretsRepo.IsEnabled: %w", err)
	}
	return enabled, nil
}

func (r *mfaSecretsRepo) GetSecretIfEnabled(ctx context.Context, userID int64) (string, error) {
	var secret string
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT secret FROM %s.auth_mfa_secrets WHERE user_id=$1 AND enabled=TRUE", r.s.schema), userID)
	if err := row.Scan(&secret); err != nil {
		return "", fmt.Errorf("mfaSecretsRepo.GetSecretIfEnabled: %w", err)
	}
	return secret, nil
}

type rolesRepo struct{ s *PGStore }

func (r *rolesRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlUserRoles)
	if err != nil {
		return fmt.Errorf("rolesRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("rolesRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

func (r *rolesRepo) Assign(ctx context.Context, userID int64, role string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("INSERT INTO %s.auth_user_roles (user_id, role) VALUES ($1,$2) ON CONFLICT (user_id, role) DO NOTHING", r.s.schema), userID, role)
	if err != nil {
		return fmt.Errorf("rolesRepo.Assign: %w", err)
	}
	return nil
}

func (r *rolesRepo) Remove(ctx context.Context, userID int64, role string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("DELETE FROM %s.auth_user_roles WHERE user_id=$1 AND role=$2", r.s.schema), userID, role)
	if err != nil {
		return fmt.Errorf("rolesRepo.Remove: %w", err)
	}
	return nil
}

func (r *rolesRepo) Has(ctx context.Context, userID int64, role string) (bool, error) {
	var count int
	row := r.s.queryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s.auth_user_roles WHERE user_id=$1 AND role=$2", r.s.schema), userID, role)
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("rolesRepo.Has: %w", err)
	}
	return count > 0, nil
}

func (r *rolesRepo) List(ctx context.Context, userID int64) ([]string, error) {
	rows, err := r.s.query(ctx, fmt.Sprintf("SELECT role FROM %s.auth_user_roles WHERE user_id=$1", r.s.schema), userID)
	if err != nil {
		return nil, fmt.Errorf("rolesRepo.List: %w", err)
	}
	defer rows.Close()
	var roles []string
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("rolesRepo.List: scan: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// --- API Keys repo ---

type apiKeysRepo struct{ s *PGStore }

func (r *apiKeysRepo) EnsureSchema(ctx context.Context) error {
	ddl, err := r.s.render(ddlAPIKeys)
	if err != nil {
		return fmt.Errorf("apiKeysRepo.EnsureSchema: render: %w", err)
	}
	_, err = r.s.exec(ctx, ddl)
	if err != nil {
		return fmt.Errorf("apiKeysRepo.EnsureSchema: exec: %w", err)
	}
	return nil
}

func (r *apiKeysRepo) Insert(ctx context.Context, p storage.InsertAPIKeyParams) (*storage.APIKeyRecord, error) {
	var created time.Time
	if p.ExpiresAt != nil {
		q := fmt.Sprintf("INSERT INTO %s.auth_apikeys (id, user_id, org_id, project_id, name, prefix, key_hash, scopes, expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING created_at", r.s.schema)
		if err := r.s.queryRow(ctx, q,
			p.ID, p.UserID, p.OrgID, p.ProjectID, p.Name, p.Prefix, p.KeyHash, p.ScopesJSON, *p.ExpiresAt,
		).Scan(&created); err != nil {
			return nil, fmt.Errorf("apiKeysRepo.Insert: %w", err)
		}
	} else {
		q := fmt.Sprintf("INSERT INTO %s.auth_apikeys (id, user_id, org_id, project_id, name, prefix, key_hash, scopes) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at", r.s.schema)
		if err := r.s.queryRow(ctx, q,
			p.ID, p.UserID, p.OrgID, p.ProjectID, p.Name, p.Prefix, p.KeyHash, p.ScopesJSON,
		).Scan(&created); err != nil {
			return nil, fmt.Errorf("apiKeysRepo.Insert: %w", err)
		}
	}
	k := &storage.APIKeyRecord{ID: p.ID, UserID: p.UserID, OrgID: p.OrgID, ProjectID: p.ProjectID, Name: p.Name, Prefix: p.Prefix, CreatedAt: created, ExpiresAt: p.ExpiresAt}
	return k, nil
}

func (r *apiKeysRepo) ListByUser(ctx context.Context, userID int64) ([]*storage.APIKeyRecord, error) {
	rows, err := r.s.query(ctx, fmt.Sprintf("SELECT id, user_id, org_id, project_id, name, prefix, scopes, created_at, expires_at, last_used_at FROM %s.auth_apikeys WHERE user_id=$1 ORDER BY created_at DESC", r.s.schema), userID)
	if err != nil {
		return nil, fmt.Errorf("apiKeysRepo.ListByUser: %w", err)
	}
	defer rows.Close()
	var out []*storage.APIKeyRecord
	for rows.Next() {
		var k storage.APIKeyRecord
		var org, proj sql.NullInt64
		var scopesJSON []byte
		var expires, lastUsed sql.NullTime
		if err := rows.Scan(&k.ID, &k.UserID, &org, &proj, &k.Name, &k.Prefix, &scopesJSON, &k.CreatedAt, &expires, &lastUsed); err != nil {
			return nil, fmt.Errorf("apiKeysRepo.ListByUser: scan: %w", err)
		}
		if org.Valid {
			v := org.Int64
			k.OrgID = &v
		}
		if proj.Valid {
			v := proj.Int64
			k.ProjectID = &v
		}
		if len(scopesJSON) > 0 {
			k.ScopesJSON = make([]byte, len(scopesJSON))
			copy(k.ScopesJSON, scopesJSON)
		}
		if expires.Valid {
			k.ExpiresAt = &expires.Time
		}
		if lastUsed.Valid {
			k.LastUsedAt = &lastUsed.Time
		}
		out = append(out, &k)
	}
	return out, nil
}

func (r *apiKeysRepo) FindByPrefix(ctx context.Context, prefix string) ([]storage.StoredAPIKey, error) {
	rows, err := r.s.query(ctx, fmt.Sprintf("SELECT id, user_id, org_id, project_id, scopes, key_hash, expires_at FROM %s.auth_apikeys WHERE prefix=$1", r.s.schema), prefix)
	if err != nil {
		return nil, fmt.Errorf("apiKeysRepo.FindByPrefix: %w", err)
	}
	defer rows.Close()
	var out []storage.StoredAPIKey
	for rows.Next() {
		var rec storage.StoredAPIKey
		var org, proj sql.NullInt64
		var expires sql.NullTime
		if err := rows.Scan(&rec.ID, &rec.UserID, &org, &proj, &rec.ScopesJSON, &rec.KeyHash, &expires); err != nil {
			return nil, fmt.Errorf("apiKeysRepo.FindByPrefix: scan: %w", err)
		}
		if org.Valid {
			v := org.Int64
			rec.OrgID = &v
		}
		if proj.Valid {
			v := proj.Int64
			rec.ProjectID = &v
		}
		if expires.Valid {
			t := expires.Time
			rec.ExpiresAt = &t
		}
		out = append(out, rec)
	}
	return out, nil
}

func (r *apiKeysRepo) MarkUsedNow(ctx context.Context, prefix string, keyHash string) error {
	_, err := r.s.exec(ctx, fmt.Sprintf("UPDATE %s.auth_apikeys SET last_used_at=NOW() WHERE prefix=$1 AND key_hash=$2", r.s.schema), prefix, keyHash)
	if err != nil {
		return fmt.Errorf("apiKeysRepo.MarkUsedNow: %w", err)
	}
	return nil
}

func (r *apiKeysRepo) DeleteByID(ctx context.Context, userID int64, keyID string) error {
	res, err := r.s.exec(ctx, fmt.Sprintf("DELETE FROM %s.auth_apikeys WHERE id=$1 AND user_id=$2", r.s.schema), keyID, userID)
	if err != nil {
		return fmt.Errorf("apiKeysRepo.DeleteByID: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("not found")
	}
	return nil
}
