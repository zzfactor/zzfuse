package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidSession          = errors.New("session is invalid or revoked")
	ErrSessionDatabaseRequired = errors.New("session database is required")
	ErrAccessDenied            = errors.New("administrator access required")
	ErrUserNotFound            = fmt.Errorf("user not found: %w", sql.ErrNoRows)
	ErrLastAdministrator       = errors.New("cannot remove the last enabled administrator")
	ErrInvalidAccess           = errors.New("invalid access update")
	ErrInvalidGrant            = fmt.Errorf("%w: schema grant", ErrInvalidAccess)
	ErrInvalidRole             = fmt.Errorf("%w: role", ErrInvalidAccess)
	ErrLastAdmin               = ErrLastAdministrator
)

type sessionDatabaseKey struct{}

// WithSessionDatabase binds the instance-owned auth database to this request.
// It never changes process-global state or implicitly trusts a token's privileges.
func WithSessionDatabase(ctx context.Context, db *sql.DB) context.Context {
	return context.WithValue(ctx, sessionDatabaseKey{}, db)
}
func SessionDatabaseFromContext(ctx context.Context) *sql.DB {
	db, _ := ctx.Value(sessionDatabaseKey{}).(*sql.DB)
	return db
}

// UserAccess is current persisted account authorization, without credentials.
type UserAccess struct {
	UserID      int64    `json:"user_id"`
	Email       string   `json:"email"`
	Username    string   `json:"username"`
	AuthVersion int64    `json:"auth_version"`
	Disabled    bool     `json:"disabled"`
	TenantID    string   `json:"tenant_id"`
	Roles       []string `json:"roles"`
	Schemas     []string `json:"schemas"`
}

type sessionStateQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const sessionStateSQL = `SELECT u.id,u.email,COALESCE(u.username,''),u.auth_version,u.disabled,u.tenant_id,
 COALESCE((SELECT jsonb_agg(r.role ORDER BY r.role) FROM admin.auth_user_roles r WHERE r.user_id=u.id),'[]'::jsonb),
 COALESCE((SELECT jsonb_agg(g.schema_name ORDER BY g.schema_name) FROM admin.auth_user_schema_grants g WHERE g.user_id=u.id),'[]'::jsonb)
 FROM admin.auth_users u WHERE u.id=$1`

func loadSessionState(ctx context.Context, q sessionStateQuery, userID int64) (*UserAccess, error) {
	if userID <= 0 {
		return nil, ErrUserNotFound
	}
	var state UserAccess
	var roles, grants []byte
	if err := q.QueryRowContext(ctx, sessionStateSQL, userID).Scan(&state.UserID, &state.Email, &state.Username, &state.AuthVersion, &state.Disabled, &state.TenantID, &roles, &grants); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("load session account: %w", err)
	}
	if err := json.Unmarshal(roles, &state.Roles); err != nil {
		return nil, fmt.Errorf("load session roles: %w", err)
	}
	if err := json.Unmarshal(grants, &state.Schemas); err != nil {
		return nil, fmt.Errorf("load session grants: %w", err)
	}
	if state.AuthVersion < 1 {
		return nil, ErrInvalidSession
	}
	return &state, nil
}

func refreshSessionClaims(claims *Claims, state *UserAccess) {
	claims.Email, claims.Username, claims.TenantID = state.Email, state.Username, state.TenantID
	claims.Roles = append([]string{}, state.Roles...)
	claims.Schemas = append([]string{}, state.Schemas...)
	// User sessions have no caller-selected machine privilege scope.
	claims.Scopes = nil
	claims.OrgID, claims.ProjectID = nil, nil
}

// ValidateSessionJWT checks persisted account/version/JTI state on every call.
// Cryptographically valid legacy JWTs without auth_version are rejected.
func ValidateSessionJWT(ctx context.Context, db *sql.DB, token string) (*Claims, error) {
	if db == nil {
		return nil, ErrSessionDatabaseRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	claims, err := ValidateJWT(token)
	if err != nil {
		return nil, ErrInvalidSession
	}
	if claims.UserID <= 0 || claims.AuthVersion < 1 || claims.ID == "" || claims.ExpiresAt == nil || claims.IssuedAt == nil {
		return nil, ErrInvalidSession
	}
	state, err := loadSessionState(ctx, db, claims.UserID)
	if err != nil {
		return nil, err
	}
	if state.Disabled || state.AuthVersion != claims.AuthVersion {
		return nil, ErrInvalidSession
	}
	blacklisted, err := IsTokenBlacklisted(ctx, db, claims.ID)
	if err != nil {
		return nil, err
	}
	if blacklisted {
		return nil, ErrInvalidSession
	}
	refreshSessionClaims(claims, state)
	if claims.APIKeyID != "" {
		if err := refreshAPIKeyClaims(ctx, db, claims); err != nil {
			return nil, err
		}
	}
	return claims, nil
}

func ValidateSessionJWTFromRequest(r *http.Request, db *sql.DB) (*Claims, error) {
	token, err := ExtractTokenFromRequest(r)
	if err != nil {
		return nil, ErrInvalidSession
	}
	return ValidateSessionJWT(r.Context(), db, token)
}

// IssueSessionJWT is a trusted server issuance operation. Normal user sessions
// derive privileges from the database; arbitrary scope parameters are rejected.
func IssueSessionJWT(ctx context.Context, db *sql.DB, userID int64, expiry time.Duration, orgID, projectID *int64, scopes []string) (string, error) {
	if orgID != nil || projectID != nil || len(scopes) != 0 {
		return "", errors.New("user sessions cannot select machine scopes")
	}
	return issueSessionJWT(ctx, db, userID, expiry, 0, "")
}

func issueSessionJWT(ctx context.Context, db *sql.DB, userID int64, expiry time.Duration, expectedVersion int64, apiKeyID string) (string, error) {
	if db == nil {
		return "", ErrSessionDatabaseRequired
	}
	if expiry <= 0 {
		return "", ErrInvalidSession
	}
	state, err := loadSessionState(ctx, db, userID)
	if err != nil {
		return "", err
	}
	if state.Disabled || (expectedVersion > 0 && state.AuthVersion != expectedVersion) {
		return "", ErrInvalidSession
	}
	claims := &Claims{UserID: state.UserID, AuthVersion: state.AuthVersion, APIKeyID: apiKeyID}
	refreshSessionClaims(claims, state)
	if apiKeyID != "" {
		if err := refreshAPIKeyClaims(ctx, db, claims); err != nil {
			return "", err
		}
	}
	return signSessionJWT(claims, expiry)
}

func signSessionJWT(claims *Claims, expiry time.Duration) (string, error) {
	fallback, issuer, audience, kid, secrets, err := getJWTConfig()
	if err != nil {
		return "", fmt.Errorf("jwt config: %w", err)
	}
	jti, err := generateJTI()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	claims.RegisteredClaims = jwt.RegisteredClaims{ID: jti, Subject: strconv.FormatInt(claims.UserID, 10), Issuer: issuer, IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(expiry))}
	if audience != "" {
		claims.Audience = jwt.ClaimStrings{audience}
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	key := fallback
	if kid != "" {
		var ok bool
		key, ok = secrets[kid]
		if !ok {
			return "", errors.New("jwt signing key unavailable")
		}
		token.Header["kid"] = kid
	}
	return token.SignedString(key)
}

func RenewSessionJWT(ctx context.Context, db *sql.DB, token string, expiry time.Duration) (string, error) {
	claims, err := ValidateSessionJWT(ctx, db, token)
	if err != nil {
		return "", err
	}
	if claims.APIKeyID != "" {
		return "", errors.New("API-key sessions must be exchanged again")
	}
	return issueSessionJWT(ctx, db, claims.UserID, expiry, claims.AuthVersion, "")
}

func RevokeSessionJWT(ctx context.Context, db *sql.DB, token string) error {
	claims, err := ValidateSessionJWT(ctx, db, token)
	if err != nil {
		return err
	}
	return BlacklistToken(ctx, db, claims.ID, claims.ExpiresAt.Time.Add(5*time.Second))
}

// API-key JWTs use current key scope, never inherit administrator roles, and
// remain valid only while the owner/key exists, is enabled, and has not expired.
func refreshAPIKeyClaims(ctx context.Context, db *sql.DB, claims *Claims) error {
	var scopes []byte
	var expires sql.NullTime
	err := db.QueryRowContext(ctx, `SELECT org_id,project_id,COALESCE(scopes,'[]'::jsonb),expires_at FROM admin.auth_apikeys WHERE id=$1 AND user_id=$2`, claims.APIKeyID, claims.UserID).Scan(&claims.OrgID, &claims.ProjectID, &scopes, &expires)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInvalidSession
		}
		return fmt.Errorf("load session API key: %w", err)
	}
	if expires.Valid && !expires.Time.After(time.Now().UTC()) {
		return ErrInvalidSession
	}
	if err := json.Unmarshal(scopes, &claims.Scopes); err != nil {
		return ErrInvalidSession
	}
	if err := validateAPIKeyScopes(claims.Scopes); err != nil {
		return ErrInvalidSession
	}
	claims.Roles = []string{}
	return nil
}

func SessionUserContextMiddleware(db *sql.DB, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		UserContextMiddleware(next).ServeHTTP(w, r.WithContext(WithSessionDatabase(r.Context(), db)))
	})
}
