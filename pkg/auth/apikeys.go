// Package auth provides API key management and exchange for JWTs.
//
// Key format:
//
//	trz-<8 hex prefix>_<32 hex secret>
//
// Security notes:
//   - HMAC-SHA256 over the exact plaintext key with a server-side pepper.
//   - TRAZR_API_KEY_PEPPER is required; TRAZR_API_KEY_PEPPER_OLD (optional) supports rotation.
//   - All validation errors are collapsed to a generic "invalid api key" to avoid
//     aiding brute force attempts.
//   - created_at/expires_at/last_used_at are stored as TIMESTAMPTZ in the DB.
//
// Rotation procedure (recommended):
//  1. Set TRAZR_API_KEY_PEPPER to the new pepper; move the previous pepper to TRAZR_API_KEY_PEPPER_OLD.
//  2. Restart services. Both peppers validate; new keys use the new pepper.
//  3. After a grace period, unset API_KEY_PEPPER_OLD and restart.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	stor "github.com/medxops/trazr-otel/pkg/auth/storage"
	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

const keyPrefix = "trz-"

var ErrInvalidAPIKey = errors.New("invalid api key")

// APIKey represents a user-scoped API key (sans plaintext).
// Plaintext value is only returned at creation via CreateAPIKey.
type APIKey struct {
	ID         string     `json:"id"`
	UserID     int64      `json:"user_id"`
	OrgID      *int64     `json:"org_id,omitempty"`
	ProjectID  *int64     `json:"project_id,omitempty"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

var (
	pepperOnce sync.Once
	cachedPeppers []string
	pepperErr     error
)

// getPeppers lazily loads the API key peppers on first use via sync.Once.
// Returns an error if TRAZR_API_KEY_PEPPER is not set.
func getPeppers() ([]string, error) {
	pepperOnce.Do(func() {
		primary := strings.TrimSpace(os.Getenv("TRAZR_API_KEY_PEPPER"))
		if primary == "" {
			pepperErr = errors.New("TRAZR_API_KEY_PEPPER must be set")
			return
		}
		old := strings.TrimSpace(os.Getenv("TRAZR_API_KEY_PEPPER_OLD"))
		if old != "" {
			cachedPeppers = []string{primary, old}
		} else {
			cachedPeppers = []string{primary}
		}
	})
	return cachedPeppers, pepperErr
}

func getAPIKeyPepper() string { // kept for backward compatibility in this package
	return os.Getenv("TRAZR_API_KEY_PEPPER")
}

// EnsureAPIKeysTable creates the API keys table if it does not exist.
// EnsureAPIKeysTable creates the API keys table, indexes, and constraints if they do not exist.
// Safe to call at startup.
func EnsureAPIKeysTable(db *sql.DB) error {
	ctx := context.Background()
	store := pgstore.New(db)
	return store.APIKeys().EnsureSchema(ctx)
}

// ListAPIKeys returns API keys for a user (without plaintext value).
func ListAPIKeys(ctx context.Context, db *sql.DB, userID int64) ([]*APIKey, error) {
	store := pgstore.New(db)
	recs, err := store.APIKeys().ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	var out []*APIKey
	for _, r := range recs {
		k := &APIKey{ID: r.ID, UserID: r.UserID, Name: r.Name, Prefix: r.Prefix, CreatedAt: r.CreatedAt}
		if r.OrgID != nil {
			v := *r.OrgID
			k.OrgID = &v
		}
		if r.ProjectID != nil {
			v := *r.ProjectID
			k.ProjectID = &v
		}
		if len(r.ScopesJSON) > 0 {
			if err := json.Unmarshal(r.ScopesJSON, &k.Scopes); err != nil {
				k.Scopes = []string{}
			}
		}
		if r.ExpiresAt != nil {
			t := *r.ExpiresAt
			k.ExpiresAt = &t
		}
		if r.LastUsedAt != nil {
			t := *r.LastUsedAt
			k.LastUsedAt = &t
		}
		out = append(out, k)
	}
	return out, nil
}

// CreateAPIKey creates a new API key for the given user and returns the plaintext value once.
// expiresIn can be a duration string like "30d", "90d", or RFC3339 duration; if empty, does not expire.
// CreateAPIKey creates an API key, stores only its peppered HMAC hash, and returns the plaintext once.
// The plaintext is never retrievable after creation.
func CreateAPIKey(ctx context.Context, db *sql.DB, userID int64, name string, orgID, projectID *int64, scopes []string, expiresIn string) (plaintext string, key *APIKey, err error) {
	if _, err := getPeppers(); err != nil {
		return "", nil, fmt.Errorf("api key config: %w", err)
	}
	name = strings.TrimSpace(name)
	if userID == 0 || name == "" {
		return "", nil, errors.New("user_id and name required")
	}
	if len(name) > 255 {
		return "", nil, errors.New("name too long (max 255)")
	}
	if scopes == nil {
		scopes = []string{}
	}
	// Validate user exists via storage
	store := pgstore.New(db)
	exists, err := store.Users().ExistsByID(ctx, userID)
	if err != nil {
		return "", nil, fmt.Errorf("check user: %w", err)
	}
	if !exists {
		return "", nil, errors.New("user not found")
	}
	// Generate key: trz-<prefix>_<random>
	prefix := randomString(8)
	secret := randomString(32)
	core := fmt.Sprintf("%s_%s", prefix, secret)
	plaintext = keyPrefix + core
	// Hash with peppered HMAC-SHA256 (hash the exact plaintext as presented to the user)
	h := hmac.New(sha256.New, []byte(getAPIKeyPepper()))
	h.Write([]byte(plaintext))
	keyHash := hex.EncodeToString(h.Sum(nil))
	// Marshal scopes
	scopesJSON, _ := json.Marshal(scopes)
	// Determine expiry
	var expiresAt *time.Time
	if s := strings.TrimSpace(expiresIn); s != "" {
		d, perr := parseFlexibleDuration(s)
		if perr != nil {
			return "", nil, fmt.Errorf("invalid expires_in: %w", perr)
		}
		t := time.Now().Add(d)
		expiresAt = &t
	}
	// Generate ID for key
	kid := strings.ToLower(randomString(20))
	// Persist via repository
	rec, err := store.APIKeys().Insert(ctx, stor.InsertAPIKeyParams{ID: kid, UserID: userID, OrgID: orgID, ProjectID: projectID, Name: name, Prefix: prefix, KeyHash: keyHash, ScopesJSON: scopesJSON, ExpiresAt: expiresAt})
	if err != nil {
		return "", nil, fmt.Errorf("insert apikey: %w", err)
	}
	k := &APIKey{ID: rec.ID, UserID: rec.UserID, OrgID: rec.OrgID, ProjectID: rec.ProjectID, Name: name, Prefix: prefix, Scopes: scopes, CreatedAt: rec.CreatedAt, ExpiresAt: rec.ExpiresAt}
	return plaintext, k, nil
}

// RevokeAPIKey deletes an API key by ID for a user.
// RevokeAPIKey irrevocably deletes an API key by ID for a user.
func RevokeAPIKey(ctx context.Context, db *sql.DB, userID int64, keyID string) error {
	store := pgstore.New(db)
	if err := store.APIKeys().DeleteByID(ctx, userID, keyID); err != nil {
		if err.Error() == "not found" {
			return sql.ErrNoRows
		}
		return err
	}
	return nil
}

// ExchangeAPIKeyForJWT validates the API key and issues a short-lived JWT for the associated user.
// The JWT TTL can be configured via TRAZR_JWT_M2M_TTL (default 15m).
// ExchangeAPIKeyForJWT validates a presented API key and issues a short-lived JWT for the associated user.
// It requires keys to be in the "trz-" format and uses constant-time HMAC comparison.
func ExchangeAPIKeyForJWT(ctx context.Context, db *sql.DB, apiKey string) (string, error) {
	peppers, err := getPeppers()
	if err != nil {
		return "", fmt.Errorf("api key config: %w", err)
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return "", ErrInvalidAPIKey
	}
	if !strings.HasPrefix(apiKey, keyPrefix) {
		return "", ErrInvalidAPIKey
	}
	core := strings.TrimPrefix(apiKey, keyPrefix)
	parts := strings.Split(core, "_")
	if len(parts) < 2 {
		return "", ErrInvalidAPIKey
	}
	prefix := parts[0]
	// Lookup candidates by stored prefix (via storage)
	store := pgstore.New(db)
	recs, err := store.APIKeys().FindByPrefix(ctx, prefix)
	if err != nil {
		return "", fmt.Errorf("lookup apikey by prefix: %w", err)
	}
	var userID int64
	var matchedOrgID, matchedProjectID *int64
	var matchedScopes []string
	var matched bool
	var expired bool
	var matchedHash string
	// Try all peppers to allow rotation
	for _, rec := range recs {
		for _, pepper := range peppers {
			h := hmac.New(sha256.New, []byte(pepper))
			h.Write([]byte(apiKey))
			suppliedHash := hex.EncodeToString(h.Sum(nil))
			if hmac.Equal([]byte(suppliedHash), []byte(rec.KeyHash)) {
				userID = rec.UserID
				matchedOrgID = rec.OrgID
				matchedProjectID = rec.ProjectID
				if len(rec.ScopesJSON) > 0 {
					if err := json.Unmarshal(rec.ScopesJSON, &matchedScopes); err != nil {
						matchedScopes = []string{}
					}
				}
				matched = true
				matchedHash = suppliedHash
				if rec.ExpiresAt != nil && rec.ExpiresAt.Before(time.Now().UTC()) {
					expired = true
				}
				break
			}
		}
		if matched {
			break
		}
	}
	if !matched || expired {
		return "", ErrInvalidAPIKey
	}
	// Check if account is locked (prevents API key bypass of lockout)
	locked, err := IsAccountLocked(ctx, db, userID)
	if err != nil {
		return "", fmt.Errorf("lockout check: %w", err)
	}
	if locked {
		return "", errors.New("account locked due to failed attempts")
	}
	// Get email for user via storage
	email, err := store.Users().GetEmailByID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("load user: %w", err)
	}
	// Issue short-lived JWT
	expiry := 15 * time.Minute
	if ttlStr := strings.TrimSpace(os.Getenv("TRAZR_JWT_M2M_TTL")); ttlStr != "" {
		if d, perr := time.ParseDuration(ttlStr); perr == nil && d > 0 {
			expiry = d
		}
	}
	token, err := GenerateJWTWith(userID, email, expiry, matchedOrgID, matchedProjectID, matchedScopes)
	if err != nil {
		return "", fmt.Errorf("issue jwt: %w", err)
	}
	// Update last_used_at via repo
	_ = store.APIKeys().MarkUsedNow(ctx, prefix, matchedHash)
	return token, nil
}

// --- helpers ---

// randomString returns a cryptographically secure random hex string of length n.
func randomString(n int) string {
	if n <= 0 {
		n = 16
	}
	// Need n/2 bytes to produce n hex chars (round up)
	byteLen := (n + 1) / 2
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		// Extremely unlikely; fallback to time+pid entropy (still return hex)
		fallback := []byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid()))
		return strings.ToLower(hex.EncodeToString(fallback))[:n]
	}
	return strings.ToLower(hex.EncodeToString(b))[:n]
}

// parseFlexibleDuration accepts Go duration (e.g., 15m, 24h) or Nd/Nw (days/weeks).
func parseFlexibleDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "d") {
		n, err := time.ParseDuration(strings.TrimSuffix(s, "d") + "h")
		if err != nil {
			return 0, err
		}
		return n * 24, nil
	}
	if strings.HasSuffix(s, "w") {
		n, err := time.ParseDuration(strings.TrimSuffix(s, "w") + "h")
		if err != nil {
			return 0, err
		}
		return n * 24 * 7, nil
	}
	return time.ParseDuration(s)
}
