package trazrextension

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"database/sql"

	auth "github.com/medxops/trazr-otel/pkg/auth"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// extractUserID safely extracts the user_id from JWT claims,
// handling float64 (standard JSON number), int64, and json.Number types.
func extractUserID(claims jwt.MapClaims) (int64, error) {
	raw, ok := claims["user_id"]
	if !ok {
		return 0, fmt.Errorf("missing user_id claim")
	}
	switch v := raw.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid user_id claim: %w", err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unexpected user_id type %T", raw)
	}
}

// NewListAPIKeysHandler handles GET /api/v1/auth/apikeys (JWT required)
func NewListAPIKeysHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		claims, err := ValidateJWTFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized: "+err.Error(), zap.NewNop())
			return
		}
		uid, err := extractUserID(claims)
		if err != nil || uid == 0 {
			writeJSONError(w, http.StatusUnauthorized, "invalid token claims", zap.NewNop())
			return
		}
		keys, err := auth.ListAPIKeys(r.Context(), db, uid)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
	}
}

// NewCreateAPIKeyHandler handles POST /api/v1/auth/apikeys (JWT required)
func NewCreateAPIKeyHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		claims, err := ValidateJWTFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized: "+err.Error(), zap.NewNop())
			return
		}
		uid, err := extractUserID(claims)
		if err != nil || uid == 0 {
			writeJSONError(w, http.StatusUnauthorized, "invalid token claims", zap.NewNop())
			return
		}
		var req struct {
			Name      string   `json:"name"`
			OrgID     *int64   `json:"org_id,omitempty"`
			ProjectID *int64   `json:"project_id,omitempty"`
			Scopes    []string `json:"scopes"`
			ExpiresIn string   `json:"expires_in"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		plaintext, key, err := auth.CreateAPIKey(r.Context(), db, uid, req.Name, req.OrgID, req.ProjectID, req.Scopes, req.ExpiresIn)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error(), zap.NewNop())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"key": plaintext, "api_key": key})
	}
}

// NewRevokeAPIKeyHandler handles DELETE /api/v1/auth/apikeys/{id} (JWT required)
func NewRevokeAPIKeyHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		claims, err := ValidateJWTFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized: "+err.Error(), zap.NewNop())
			return
		}
		uid, err := extractUserID(claims)
		if err != nil || uid == 0 {
			writeJSONError(w, http.StatusUnauthorized, "invalid token claims", zap.NewNop())
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 5 { // api v1 auth apikeys {id}
			writeJSONError(w, http.StatusBadRequest, "missing api key id", zap.NewNop())
			return
		}
		keyID := parts[len(parts)-1]
		if err := auth.RevokeAPIKey(r.Context(), db, uid, keyID); err != nil {
			if err == sql.ErrNoRows {
				writeJSONError(w, http.StatusNotFound, "not found", zap.NewNop())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// NewExchangeAPIKeyHandler handles POST /api/v1/auth/apikeys/exchange using X-API-Key header
func NewExchangeAPIKeyHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		apiKey := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if apiKey == "" {
			// Optionally accept JSON body {"api_key":"..."}
			var body struct{ APIKey string `json:"api_key"` }
			_ = json.NewDecoder(r.Body).Decode(&body)
			apiKey = strings.TrimSpace(body.APIKey)
		}
		if apiKey == "" {
			writeJSONError(w, http.StatusUnauthorized, "missing api key", zap.NewNop())
			return
		}
		token, err := auth.ExchangeAPIKeyForJWT(r.Context(), db, apiKey)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, err.Error(), zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token})
	}
}
