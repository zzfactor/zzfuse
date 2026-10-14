package trazrextension

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/medxops/trazr-otel/pkg/auth"
	"go.uber.org/zap"
)

func writeAccessError(w http.ResponseWriter, err error) {
	status, message := http.StatusInternalServerError, "access update failed"
	switch {
	case errors.Is(err, sql.ErrNoRows):
		status, message = http.StatusNotFound, "user not found"
	case errors.Is(err, auth.ErrAccessDenied):
		status, message = http.StatusForbidden, "administrator access required"
	case errors.Is(err, auth.ErrInvalidAccess), errors.Is(err, auth.ErrLastAdmin):
		status, message = http.StatusBadRequest, err.Error()
	}
	writeJSONError(w, status, message, zap.NewNop())
}

func NewUserAccessHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		actor, err := auth.GetUserID(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		target, err := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
		if err != nil || target <= 0 {
			writeJSONError(w, http.StatusBadRequest, "a positive user_id is required", zap.NewNop())
			return
		}
		access, err := auth.GetUserAccess(r.Context(), db, actor, target)
		if err != nil {
			writeAccessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, access)
	}
}

func NewSetUserSchemasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var body struct {
			UserID  int64     `json:"user_id"`
			Schemas *[]string `json:"schemas"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID <= 0 || body.Schemas == nil {
			writeJSONError(w, http.StatusBadRequest, "user_id and schemas are required; use an empty array to remove all grants", zap.NewNop())
			return
		}
		actor, err := auth.GetUserID(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		if err := auth.SetUserSchemaGrants(r.Context(), db, actor, body.UserID, *body.Schemas); err != nil {
			writeAccessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_id": body.UserID, "schemas": *body.Schemas})
	}
}

func NewRevokeUserSessionsHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var body struct {
			UserID int64 `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "a positive user_id is required", zap.NewNop())
			return
		}
		actor, err := auth.GetUserID(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		if err := auth.RevokeUserSessions(r.Context(), db, actor, body.UserID); err != nil {
			writeAccessError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func NewSetUserDisabledHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var body struct {
			UserID   int64 `json:"user_id"`
			Disabled *bool `json:"disabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.UserID <= 0 || body.Disabled == nil {
			writeJSONError(w, http.StatusBadRequest, "user_id and disabled are required", zap.NewNop())
			return
		}
		actor, err := auth.GetUserID(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		if err := auth.SetUserDisabled(r.Context(), db, actor, body.UserID, *body.Disabled); err != nil {
			writeAccessError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
