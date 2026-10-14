//go:build ignore

package trazrextension

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth"
)

// NewRegisterHandler handles POST /api/v1/auth/register
// Request JSON: {"email":"...","password":"...","name":"...", optional profile fields}
// Response: {"user": {...}}
func NewRegisterHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dbMain == nil {
			http.Error(w, "database not initialized", http.StatusServiceUnavailable)
			return
		}
		type req struct {
			Email     string                 `json:"email"`
			Password  string                 `json:"password"`
			Name      string                 `json:"name"`
			FirstName string                 `json:"first_name"`
			LastName  string                 `json:"last_name"`
			Address   string                 `json:"address"`
			City      string                 `json:"city"`
			State     string                 `json:"state"`
			Country   string                 `json:"country"`
			Company   string                 `json:"company"`
			Metadata  map[string]interface{} `json:"metadata"`
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		u, err := auth.Register(r.Context(), dbMain, body.Email, body.Password, body.Name, body.FirstName, body.LastName, body.Address, body.City, body.State, body.Country, body.Company, body.Metadata)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"user": u})
	}
}

// NewLoginHandler handles POST /api/v1/auth/login
// Request JSON: {"email":"...","password":"..."}
// Response: {"token":"..."}
func NewLoginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dbMain == nil {
			http.Error(w, "database not initialized", http.StatusServiceUnavailable)
			return
		}
		type req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		var body req
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		token, err := auth.Login(r.Context(), dbMain, body.Email, body.Password)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"token": token})
	}
}

// Helper used by potential session-based flows if needed.
func issueDevJWT(ctx context.Context, email string, userID int64) (string, error) {
	return auth.GenerateJWT(userID, strings.ToLower(strings.TrimSpace(email)), 24*time.Hour)
}

// The following handlers are stubs to satisfy references in extension.go.
// They currently return 501 Not Implemented to avoid build failures.

func NewRequestPasswordResetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}
}

func NewResetPasswordHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not implemented", http.StatusNotImplemented)
	}
}

func NewListUsersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dbMain == nil {
			http.Error(w, "database not initialized", http.StatusServiceUnavailable)
			return
		}
		users, err := auth.ListUsers(r.Context(), dbMain)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"users": users})
	}
}

func NewDeleteUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dbMain == nil {
			http.Error(w, "database not initialized", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			UserID int64  `json:"user_id"`
			Email  string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		var id int64
		if body.UserID != 0 {
			id = body.UserID
		} else if body.Email != "" {
			u, err := auth.GetUserByEmail(r.Context(), dbMain, body.Email)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			id = u.ID
		} else {
			http.Error(w, "user_id or email required", http.StatusBadRequest)
			return
		}
		if err := auth.DeleteUser(r.Context(), dbMain, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": id})
	}
}

func NewEditUserHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dbMain == nil {
			http.Error(w, "database not initialized", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			UserID int64  `json:"user_id"`
			Email  string `json:"email"`
			Name   string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.UserID == 0 {
			http.Error(w, "user_id required", http.StatusBadRequest)
			return
		}
		if err := auth.UpdateUserProfile(r.Context(), dbMain, body.UserID, strings.ToLower(strings.TrimSpace(body.Email)), body.Name); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		profile, _ := auth.GetUserProfile(r.Context(), dbMain, body.UserID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"user": profile})
	}
}

func NewResetUserPasswordHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dbMain == nil {
			http.Error(w, "database not initialized", http.StatusServiceUnavailable)
			return
		}
		var body struct {
			UserID     int64  `json:"user_id"`
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if body.UserID == 0 || body.NewPassword == "" {
			http.Error(w, "user_id and new_password required", http.StatusBadRequest)
			return
		}
		if err := auth.ResetPassword(r.Context(), dbMain, body.UserID, body.NewPassword); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"reset": body.UserID})
	}
}

func NewDeleteUserByIDHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if dbMain == nil {
			http.Error(w, "database not initialized", http.StatusServiceUnavailable)
			return
		}
		// Expect path: /api/v1/admin/users/{id}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) < 5 { // api v1 admin users {id}
			http.Error(w, "missing user id", http.StatusBadRequest)
			return
		}
		idStr := parts[len(parts)-1]
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			http.Error(w, "invalid user id", http.StatusBadRequest)
			return
		}
		if err := auth.DeleteUser(r.Context(), dbMain, id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"deleted": id})
	}
}
