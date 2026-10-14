package trazrextension

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	auth "github.com/medxops/trazr-otel/pkg/auth"
	"go.uber.org/zap"
    "database/sql"
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
		var result []map[string]interface{}
		for _, u := range users {
			roles, _ := auth.ListRoles(r.Context(), db, u.ID)
			result = append(result, map[string]interface{}{
				"id":       u.ID,
				"email":    u.Email,
				"roles":    roles,
				"metadata": u.Metadata,
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
		// TODO: Add actorID and permission checks
		if err := auth.DeleteUser(r.Context(), db, req.UserID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
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
		if err := auth.DeleteUser(r.Context(), db, userID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error(), zap.NewNop())
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
			UserID int64    `json:"user_id"`
			Roles  []string `json:"roles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		// Normalize desired roles
		desired := map[string]struct{}{}
		for _, rname := range req.Roles {
			rname = strings.TrimSpace(rname)
			if rname != "" {
				desired[strings.ToLower(rname)] = struct{}{}
			}
		}
		// Fetch current roles
		currentList, err := auth.ListRoles(r.Context(), db, req.UserID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to list current roles", zap.NewNop())
			return
		}
		current := map[string]struct{}{}
		for _, rname := range currentList {
			current[strings.ToLower(rname)] = struct{}{}
		}
		// Add missing roles
		for rname := range desired {
			if _, ok := current[rname]; !ok {
				if err := auth.AssignRole(r.Context(), db, req.UserID, rname); err != nil {
					writeJSONError(w, http.StatusInternalServerError, "failed to assign role: "+rname, zap.NewNop())
					return
				}
			}
		}
		// Remove roles not desired
		for rname := range current {
			if _, ok := desired[rname]; !ok {
				if err := auth.RemoveRole(r.Context(), db, req.UserID, rname); err != nil {
					writeJSONError(w, http.StatusInternalServerError, "failed to remove role: "+rname, zap.NewNop())
					return
				}
			}
		}
		// Return updated roles
		updated, err := auth.ListRoles(r.Context(), db, req.UserID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to fetch updated roles", zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": req.UserID,
			"roles":   updated,
		})
	}
}
