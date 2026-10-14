package trazrextension

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"database/sql"
	auth "github.com/medxops/trazr-otel/pkg/auth"
	"go.uber.org/zap"
)

func NewListUsersHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		users, err := auth.ListUsers(r.Context(), db)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
			return
		}
		actorID, err := auth.GetUserID(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		result := make([]map[string]any, 0, len(users))
		for _, u := range users {
			access, err := auth.GetUserAccess(r.Context(), db, actorID, u.ID)
			if err != nil {
				writeAccessError(w, err)
				return
			}
			result = append(result, map[string]any{
				"id": u.ID, "email": u.Email, "username": u.Username,
				"roles": access.Roles, "schemas": access.Schemas,
				"disabled": access.Disabled, "metadata": u.Metadata,
			})
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func NewDeleteUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			UserID int64 `json:"user_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		actorID, err := auth.GetUserID(r.Context())
		if err != nil {
			writeAccessError(w, err)
			return
		}
		if err := auth.DeleteUserAsAdmin(r.Context(), db, actorID, req.UserID); err != nil {
			writeAccessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "user deleted"})
	}
}

func NewEditUserHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			UserID int64  `json:"user_id"`
			Email  string `json:"email"`
			Name   string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		if err := auth.UpdateUserProfile(r.Context(), db, req.UserID, req.Email, req.Name); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "user updated"})
	}
}

func NewResetUserPasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			UserID      int64  `json:"user_id"`
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		if err := auth.ResetPassword(r.Context(), db, req.UserID, req.NewPassword); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "password reset"})
	}
}

func NewDeleteUserByIDHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 4 {
			writeJSONError(w, http.StatusBadRequest, "missing user id in path", zap.NewNop())
			return
		}
		idStr := parts[len(parts)-1]
		userID, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || userID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid user id", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		actorID, err := auth.GetUserID(r.Context())
		if err != nil {
			writeAccessError(w, err)
			return
		}
		if err := auth.DeleteUserAsAdmin(r.Context(), db, actorID, userID); err != nil {
			writeAccessError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "user deleted"})
	}
}

// NewSetUserRolesHandler replaces a user's roles with the provided set.
// Body: {"user_id": number, "roles": ["role1", "role2", ...]}
func NewSetUserRolesHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			UserID int64     `json:"user_id"`
			Roles  *[]string `json:"roles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID <= 0 || req.Roles == nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		actorID, err := auth.GetUserID(r.Context())
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		if err := auth.SetUserRoles(r.Context(), db, actorID, req.UserID, *req.Roles); err != nil {
			writeAccessError(w, err)
			return
		}
		updated, err := auth.ListRoles(r.Context(), db, req.UserID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to fetch updated roles", zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"user_id": req.UserID, "roles": updated})
	}
}
