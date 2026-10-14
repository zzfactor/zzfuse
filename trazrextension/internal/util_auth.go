package internal

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/medxops/trazr-otel/pkg/auth"
)

// requireSchemaAccess enforces the same tenant policy for every signal operation.
func requireSchemaAccess(w http.ResponseWriter, r *http.Request, schema string) bool {
	if _, err := auth.GetUserContext(r.Context()); err != nil {
		WriteErrorResponse(w, http.StatusUnauthorized, "user context not found")
		return false
	}
	if !auth.UserHasSchemaAccess(r.Context(), schema) {
		WriteErrorResponse(w, http.StatusForbidden, "access denied to schema: "+schema)
		return false
	}
	return true
}

// SchemaAuthMiddleware authorizes the effective schema before a signal stream upgrades.
// UserContextMiddleware must run first so JWT claims are available to the guard.
func SchemaAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireSchemaAccess(w, r, getSchemaParam(r)) {
			return
		}
		next(w, r)
	}
}

func authorizedStreamSchema(r *http.Request) (string, bool) {
	schema := getSchemaParam(r)
	return schema, auth.UserHasSchemaAccess(r.Context(), schema)
}

// validateJWTFromRequest extracts and validates a JWT from HTTP request headers/cookies/query params.
// Returns the parsed Claims converted to MapClaims for backward compatibility.
func validateJWTFromRequest(r *http.Request) (jwt.MapClaims, error) {
	// Use the centralized JWT validation from auth package
	claims, err := auth.ValidateJWTFromRequest(r)
	if err != nil {
		return nil, err
	}

	// Convert to MapClaims for backward compatibility with existing code
	mc := jwt.MapClaims{
		"user_id":    claims.UserID,
		"email":      claims.Email,
		"username":   claims.Username,
		"tenant_id":  claims.TenantID,
		"schemas":    claims.Schemas,
		"scopes":     claims.Scopes,
		"org_id":     claims.OrgID,
		"project_id": claims.ProjectID,
	}

	// Convert roles from []string to []interface{} for MapClaims compatibility
	if len(claims.Roles) > 0 {
		roleInterfaces := make([]interface{}, len(claims.Roles))
		for i, role := range claims.Roles {
			roleInterfaces[i] = role
		}
		mc["roles"] = roleInterfaces
	}

	return mc, nil
}

// hasRole checks if the claims contain a specific role in either "role" or "roles" fields.
func hasRole(claims jwt.MapClaims, targetRole string) bool {
	// Check single role field
	if role, ok := claims["role"].(string); ok && role == targetRole {
		return true
	}
	// Check roles array field
	if roles, ok := claims["roles"].([]interface{}); ok {
		for _, r := range roles {
			if roleStr, ok := r.(string); ok && roleStr == targetRole {
				return true
			}
		}
	}
	return false
}

// isAdmin returns true if the claims indicate an admin role.
func isAdmin(claims jwt.MapClaims) bool {
	return hasRole(claims, "admin")
}

// requireAdminAuth validates JWT and enforces admin role. Returns 401 for invalid/missing token,
// and 403 Forbidden for valid non-admin tokens.
func requireAdminAuth(w http.ResponseWriter, r *http.Request) (jwt.MapClaims, bool) {
	claims, err := validateJWTFromRequest(r)
	if err != nil {
		WriteJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized: " + err.Error()})
		return nil, false
	}
	if !isAdmin(claims) {
		WriteJSONResponse(w, http.StatusForbidden, map[string]string{"error": "forbidden: admin required"})
		return nil, false
	}
	return claims, true
}
