// JWT handling for Trazr.
//
// Environment variables:
//   - TRAZR_JWT_SECRET: primary HS256 signing/validation secret (required; returns error if missing).
//   - TRAZR_JWT_SECRETS: optional rotation map in kid=secret[,kid=secret] format.
//   - TRAZR_JWT_KID: when set, tokens will be signed with the secret mapped by this kid in TRAZR_JWT_SECRETS.
//     If set but missing from TRAZR_JWT_SECRETS, issuance fails.
//   - TRAZR_JWT_ISSUER: optional iss claim to set and enforce during validation when provided.
//   - TRAZR_JWT_AUDIENCE: optional aud claim to set and enforce during validation when provided.
//
// Rules:
//   - HS256 only (by design). If kid is set, it must exist in TRAZR_JWT_SECRETS.
//   - All time claims validated with a small leeway.
//   - Use GenerateJWTWith (org/project/scopes) or GenerateJWTWithUsername.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// parseSecretsMap parses TRAZR_JWT_SECRETS env as comma-separated kid=secret entries.
func parseSecretsMap() map[string][]byte {
	m := make(map[string][]byte)
	raw := os.Getenv("TRAZR_JWT_SECRETS")
	if raw == "" {
		return m
	}
	pairs := strings.Split(raw, ",")
	for _, p := range pairs {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			m[kv[0]] = []byte(kv[1])
		}
	}
	return m
}

// getJWTConfig reads JWT settings from environment variables (TRAZR-specific namespace).
// TRAZR_JWT_SECRET (fallback), TRAZR_JWT_SECRETS (kid=secret[,kid=secret]), TRAZR_JWT_ISSUER, TRAZR_JWT_AUDIENCE, TRAZR_JWT_KID
func getJWTConfig() (secret []byte, issuer string, audience string, kid string, secrets map[string][]byte, err error) {
	secretStr := strings.TrimSpace(os.Getenv("TRAZR_JWT_SECRET"))
	if secretStr == "" {
		return nil, "", "", "", nil, errors.New("TRAZR_JWT_SECRET must be set")
	}
	return []byte(secretStr), os.Getenv("TRAZR_JWT_ISSUER"), os.Getenv("TRAZR_JWT_AUDIENCE"), os.Getenv("TRAZR_JWT_KID"), parseSecretsMap(), nil
}

// Claims defines the JWT claims structure with full user context.
// RegisteredClaims includes standard fields: Issuer, Subject, Audience, ExpiresAt, NotBefore, IssuedAt, ID (jti).
type Claims struct {
	AuthVersion int64  `json:"auth_version,omitempty"`
	APIKeyID    string `json:"api_key_id,omitempty"`
	UserID      int64  `json:"user_id"`
	Email       string `json:"email"`
	Username    string `json:"username,omitempty"`
	// Multi-tenancy and user context
	TenantID string   `json:"tenant_id,omitempty"`
	Schemas  []string `json:"schemas,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	// Optional resource scoping for machine tokens
	OrgID     *int64   `json:"org_id,omitempty"`
	ProjectID *int64   `json:"project_id,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	jwt.RegisteredClaims
}

// generateJTI produces a cryptographically secure random identifier for jti.
func generateJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// GenerateJWT creates a signed JWT for the given user using HS256.
// It sets iss/aud/iat/nbf/exp and a random jti. Optional iss/aud/kid are sourced from env.
func GenerateJWT(userID int64, email string, expiry time.Duration) (string, error) {
	return GenerateJWTWith(userID, email, expiry, nil, nil, nil)
}

// GenerateJWTWith allows including optional org/project scoping and scopes in the token.
func GenerateJWTWith(userID int64, email string, expiry time.Duration, orgID, projectID *int64, scopes []string) (string, error) {
	fallback, issuer, audience, kid, secrets, err := getJWTConfig()
	if err != nil {
		return "", fmt.Errorf("jwt config: %w", err)
	}
	jti, err := generateJTI()
	if err != nil {
		return "", err
	}
	claims := Claims{
		UserID:    userID,
		Email:     email,
		Username:  "",
		OrgID:     orgID,
		ProjectID: projectID,
		Scopes:    scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			NotBefore: jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiry)),
		},
	}
	// Determine signing key and kid behavior
	key := fallback
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if kid != "" {
		if k, ok := secrets[kid]; ok {
			key = k
			token.Header["kid"] = kid
		} else {
			// If kid is configured but not present in secrets, refuse to issue
			return "", errors.New("jwt signing kid configured but not found in TRAZR_JWT_SECRETS")
		}
	}
	return token.SignedString(key)
}

// GenerateJWTWithFullContext creates a JWT with complete user context including schemas and roles.
func GenerateJWTWithFullContext(userID int64, email, username, tenantID string, schemas, roles, scopes []string, expiry time.Duration, orgID, projectID *int64) (string, error) {
	fallback, issuer, audience, kid, secrets, err := getJWTConfig()
	if err != nil {
		return "", fmt.Errorf("jwt config: %w", err)
	}
	jti, err := generateJTI()
	if err != nil {
		return "", err
	}
	claims := Claims{
		UserID:    userID,
		Email:     email,
		Username:  username,
		TenantID:  tenantID,
		Schemas:   schemas,
		Roles:     roles,
		OrgID:     orgID,
		ProjectID: projectID,
		Scopes:    scopes,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			NotBefore: jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiry)),
		},
	}
	// Determine signing key and kid behavior
	key := fallback
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if kid != "" {
		if k, ok := secrets[kid]; ok {
			key = k
			token.Header["kid"] = kid
		} else {
			return "", errors.New("jwt signing kid configured but not found in TRAZR_JWT_SECRETS")
		}
	}
	return token.SignedString(key)
}

// GenerateJWTWithUsername creates a signed JWT and includes the username convenience claim.
func GenerateJWTWithUsername(userID int64, email, username string, expiry time.Duration) (string, error) {
	fallback, issuer, audience, kid, secrets, err := getJWTConfig()
	if err != nil {
		return "", fmt.Errorf("jwt config: %w", err)
	}
	jti, err := generateJTI()
	if err != nil {
		return "", err
	}
	claims := Claims{
		UserID:   userID,
		Email:    email,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Issuer:    issuer,
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
			NotBefore: jwt.NewNumericDate(time.Now().UTC()),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(expiry)),
		},
	}
	key := fallback
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	if kid != "" {
		if k, ok := secrets[kid]; ok {
			key = k
			token.Header["kid"] = kid
		} else {
			return "", errors.New("jwt signing kid configured but not found in TRAZR_JWT_SECRETS")
		}
	}
	return token.SignedString(key)
}

// ValidateJWT parses and validates a JWT, returning the claims if valid.
// It enforces HS256, validates temporal claims with a small leeway, and checks issuer/audience when provided.
func ValidateJWT(tokenString string) (*Claims, error) {
	fallback, issuer, audience, _, secrets, err := getJWTConfig()
	if err != nil {
		return nil, fmt.Errorf("jwt config: %w", err)
	}
	parser := jwt.NewParser(jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuedAt())
	claims := &Claims{}
	token, err := parser.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if rawKid, present := token.Header["kid"]; present {
			kidVal, ok := rawKid.(string)
			if !ok || kidVal == "" {
				return nil, errors.New("invalid jwt signing key")
			}
			if key, found := secrets[kidVal]; found {
				return key, nil
			}
			return nil, errors.New("unknown jwt signing key")
		}
		return fallback, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	// Validate standard time-based claims with leeway
	validator := jwt.NewValidator(jwt.WithLeeway(5 * time.Second))
	if err := validator.Validate(claims); err != nil {
		return nil, err
	}
	// Optional issuer/audience checks if configured
	if issuer != "" && claims.Issuer != issuer {
		return nil, errors.New("invalid issuer")
	}
	if audience != "" {
		aud, _ := claims.GetAudience()
		if len(aud) == 0 || aud[0] != audience {
			return nil, errors.New("invalid audience")
		}
	}
	return claims, nil
}

// ExtractJTI returns the jti (token ID) from a token without verifying the signature.
// Use this only for non-security logic like deny-list storage; do not use it to authorize requests.
func ExtractJTI(tokenString string) (string, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) < 2 {
		return "", errors.New("invalid token format")
	}
	payloadB64 := parts[1]
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return "", err
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return "", err
	}
	if id, ok := raw["jti"].(string); ok && id != "" {
		return id, nil
	}
	return "", errors.New("jti not found")
}

// HasRoleInClaims checks if the JWT claims contain a specific role.
func HasRoleInClaims(claims *Claims, targetRole string) bool {
	if claims == nil {
		return false
	}

	for _, role := range claims.Roles {
		if role == targetRole {
			return true
		}
	}
	return false
}

// IsAdminFromClaims returns true if the claims indicate an admin role.
func IsAdminFromClaims(claims *Claims) bool {
	return HasRoleInClaims(claims, "admin")
}
