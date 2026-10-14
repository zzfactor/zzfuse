package trazrextension

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/medxops/trazr-otel/pkg/repository"
	"go.uber.org/zap"

	authpkg "github.com/medxops/trazr-otel/pkg/auth"
)

// writeJSONError writes a standardized JSON error response and logs the error.
func writeJSONError(w http.ResponseWriter, status int, message string, logger *zap.Logger) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  status,
	})
	if logger != nil {
		logger.Error(message, zap.Int("status", status))
	}
}

// writeJSON writes a standardized JSON success response.
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// ValidateJWTFromRequest extracts and validates a JWT from the Authorization header or 'token' query param.
// Returns the claims if valid, or an error.
func ValidateJWTFromRequest(r *http.Request) (jwt.MapClaims, error) {
	var tokenString string
	// 1) Authorization: Bearer
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		tokenString = strings.TrimPrefix(authHeader, "Bearer ")
	}
	// 2) Cookie "jwt"
	if tokenString == "" {
		if c, err := r.Cookie("jwt"); err == nil {
			tokenString = c.Value
		}
	}
	// 3) token query param
	if tokenString == "" {
		tokenString = r.URL.Query().Get("token")
	}
	if tokenString == "" {
		return nil, errors.New("missing token")
	}

	// Delegate verification to pkg/auth for kid/issuer/audience/leeway support
	parsed, err := authpkg.ValidateJWT(tokenString)
	if err != nil {
		return nil, errors.New("invalid token")
	}
	// Convert to MapClaims for backward compatibility with existing handlers
	mc := jwt.MapClaims{
		"user_id": parsed.UserID,
		"email":   parsed.Email,
	}
	if parsed.Username != "" {
		mc["username"] = parsed.Username
	}
	if parsed.TenantID != "" {
		mc["tenant_id"] = parsed.TenantID
	}
	if len(parsed.Schemas) > 0 {
		mc["schemas"] = append([]string(nil), parsed.Schemas...)
	}
	if len(parsed.Roles) > 0 {
		mc["roles"] = append([]string(nil), parsed.Roles...)
	}
	// Propagate optional org/project/scopes if present on the token
	if parsed.OrgID != nil {
		mc["org_id"] = *parsed.OrgID
	}
	if parsed.ProjectID != nil {
		mc["project_id"] = *parsed.ProjectID
	}
	if len(parsed.Scopes) > 0 {
		// represent scopes as []string
		scopes := make([]string, len(parsed.Scopes))
		copy(scopes, parsed.Scopes)
		mc["scopes"] = scopes
	}
	if parsed.RegisteredClaims.Issuer != "" {
		mc["iss"] = parsed.RegisteredClaims.Issuer
	}
	if aud, _ := parsed.RegisteredClaims.GetAudience(); len(aud) > 0 {
		// Represent audience as first element or full slice; use slice for fidelity
		mc["aud"] = aud
	}
	if parsed.RegisteredClaims.Subject != "" {
		mc["sub"] = parsed.RegisteredClaims.Subject
	}
	if parsed.RegisteredClaims.ID != "" {
		mc["jti"] = parsed.RegisteredClaims.ID
	}
	if parsed.RegisteredClaims.ExpiresAt != nil {
		mc["exp"] = parsed.RegisteredClaims.ExpiresAt.Time.Unix()
	}
	if parsed.RegisteredClaims.IssuedAt != nil {
		mc["iat"] = parsed.RegisteredClaims.IssuedAt.Time.Unix()
	}
	if parsed.RegisteredClaims.NotBefore != nil {
		mc["nbf"] = parsed.RegisteredClaims.NotBefore.Time.Unix()
	}
	// Optional convenience: derived boolean for expiry
	if parsed.RegisteredClaims.ExpiresAt != nil {
		mc["expired"] = time.Now().UTC().After(parsed.RegisteredClaims.ExpiresAt.Time)
	}
	return mc, nil
}

// IsAdmin returns true if the claims indicate an admin role.
func IsAdmin(claims jwt.MapClaims) bool {
	if role, ok := claims["role"].(string); ok && role == "admin" {
		return true
	}
	switch roles := claims["roles"].(type) {
	case []string:
		for _, role := range roles {
			if role == "admin" {
				return true
			}
		}
	case []interface{}:
		for _, r := range roles {
			if rs, ok := r.(string); ok && rs == "admin" {
				return true
			}
		}
	}
	return false
}

// ProvideVectorStore returns a VectorStoreClient provider closure for a given storeID.
// For now, it always returns the provided vectorStore, but can be extended for multi-store support.
func ProvideVectorStore(vectorStore repository.VectorStoreClient) func(storeID string) (repository.VectorStoreClient, error) {
	return func(storeID string) (repository.VectorStoreClient, error) {
		if vectorStore == nil {
			return nil, fmt.Errorf("VectorStore not available")
		}
		// In the future, use storeID to select among multiple stores.
		return vectorStore, nil
	}
}
