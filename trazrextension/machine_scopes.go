package trazrextension

import (
	"net/http"
	"slices"
	"strings"

	"github.com/medxops/trazr-otel/pkg/auth"
)

// Machine credentials cannot manage user accounts or acquire broader credentials.
// This guard complements schema authorization: a read key cannot insert signals.
func withMachineScopes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") || publicAPIPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if _, err := auth.ExtractTokenFromRequest(r); err != nil {
			next.ServeHTTP(w, r)
			return
		}
		claims, err := auth.ValidateJWTFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", nil)
			return
		}
		if claims.APIKeyID != "" && !machineRouteAllowed(r, claims.Scopes) {
			writeJSONError(w, http.StatusForbidden, "API key scope does not permit this operation", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func publicAPIPath(path string) bool {
	return slices.Contains([]string{"/api/v1/health", "/api/v1/version", "/api/v1/auth/register", "/api/v1/auth/login",
		"/api/v1/auth/request-password-reset", "/api/v1/auth/reset-password", "/api/v1/auth/apikeys/exchange"}, path)
}

func machineRouteAllowed(r *http.Request, scopes []string) bool {
	if r.URL.Path == "/api/v1/auth/logout" {
		return r.Method == http.MethodPost
	}
	if r.URL.Path == "/api/v1/user/notifications" {
		return r.Method == http.MethodPost && slices.Contains(scopes, "notifications:write")
	}
	if r.URL.Path == "/api/v1/schemas" {
		for _, scope := range []string{"telemetry:read", "logs:read", "spans:read", "traces:read", "metrics:read", "profiles:read"} {
			if r.Method == http.MethodGet && slices.Contains(scopes, scope) {
				return true
			}
		}
		return false
	}
	for _, base := range []string{"/api/v1/logs", "/api/v1/spans", "/api/v1/metrics", "/api/v1/profiles",
		"/api/v1/stream/logs", "/api/v1/stream/spans", "/api/v1/stream/metrics"} {
		if r.URL.Path == base || strings.HasPrefix(r.URL.Path, base+"/") {
			kind := strings.TrimPrefix(base, "/api/v1/")
			kind = strings.TrimPrefix(kind, "stream/")
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				return slices.Contains(scopes, "telemetry:read") || slices.Contains(scopes, kind+":read") ||
					(kind == "spans" && slices.Contains(scopes, "traces:read"))
			}
			if r.Method != http.MethodPost || r.URL.Path != base || strings.Contains(base, "/stream/") {
				return false
			}
			return slices.Contains(scopes, "telemetry:write") || slices.Contains(scopes, kind+":write") ||
				(kind == "spans" && slices.Contains(scopes, "traces:write"))
		}
	}
	return false
}
