package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ExtractTokenFromRequest gets a bearer token from Authorization header, cookie "jwt", or token query param.
func ExtractTokenFromRequest(r *http.Request) (string, error) {
	token := ""
	if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
		token = strings.TrimPrefix(authHeader, "Bearer ")
	}
	if token == "" {
		if c, err := r.Cookie("jwt"); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		return "", errors.New("missing token")
	}
	return token, nil
}

// ValidateJWTFromRequest parses and validates JWT from the HTTP request and returns typed Claims.
func ValidateJWTFromRequest(r *http.Request) (*Claims, error) {
	token, err := ExtractTokenFromRequest(r)
	if err != nil {
		return nil, err
	}
	return ValidateSessionJWT(r.Context(), SessionDatabaseFromContext(r.Context()), token)
}

// ScopedJWTAuthMiddleware validates JWT and enforces optional org/project scoping.
// If JWT includes org_id/project_id, incoming request with headers or query params must match.
// The middleware injects X-Org-ID/X-Project-ID headers from claims to standardize downstream handling.
func ScopedJWTAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, err := ValidateJWTFromRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		// Parse requested scoping
		q := r.URL.Query()
		parseInt64 := func(s string) (int64, bool) {
			if s == "" {
				return 0, false
			}
			var out int64
			_, err := fmt.Sscan(s, &out)
			if err != nil {
				return 0, false
			}
			return out, true
		}
		var reqOrgID, reqProjectID *int64
		if v := r.Header.Get("X-Org-ID"); v != "" {
			if n, ok := parseInt64(v); ok {
				reqOrgID = &n
			}
		} else if v := q.Get("org_id"); v != "" {
			if n, ok := parseInt64(v); ok {
				reqOrgID = &n
			}
		}
		if v := r.Header.Get("X-Project-ID"); v != "" {
			if n, ok := parseInt64(v); ok {
				reqProjectID = &n
			}
		} else if v := q.Get("project_id"); v != "" {
			if n, ok := parseInt64(v); ok {
				reqProjectID = &n
			}
		}

		// Enforce claim scoping
		if claims.OrgID != nil {
			if reqOrgID != nil && *reqOrgID != *claims.OrgID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: org scope mismatch"})
				return
			}
			r.Header.Set("X-Org-ID", fmt.Sprintf("%d", *claims.OrgID))
		}
		if claims.ProjectID != nil {
			if reqProjectID != nil && *reqProjectID != *claims.ProjectID {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: project scope mismatch"})
				return
			}
			r.Header.Set("X-Project-ID", fmt.Sprintf("%d", *claims.ProjectID))
		}
		next(w, r)
	}
}

// UserContextMiddleware validates JWT and adds user context to the request context.
// This middleware extracts JWT claims and converts them to UserContext for easy access.
func UserContextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract and validate JWT
		claims, err := ValidateJWTFromRequest(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}

		// Convert claims to user context
		userCtx := ClaimsToUserContext(claims)

		// Add user context to request context
		ctx := WithUserContext(r.Context(), userCtx)

		// Continue with request
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireSchemaAccessMiddleware ensures the user has access to the requested schema.
// This middleware should be used after UserContextMiddleware.
func RequireSchemaAccessMiddleware(getSchemaFromRequest func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get requested schema
			schema := getSchemaFromRequest(r)
			if schema == "" {
				// No schema specified, continue
				next.ServeHTTP(w, r)
				return
			}

			// Check schema access
			if !UserHasSchemaAccess(r.Context(), schema) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: schema access denied"})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRoleMiddleware ensures the user has a specific role.
// This middleware should be used after UserContextMiddleware.
func RequireRoleMiddleware(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !UserHasRole(r.Context(), role) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden: insufficient privileges"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
