package trazrextension

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strings"
	"time"

	auth "github.com/medxops/trazr-otel/pkg/auth"
	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
	"go.uber.org/zap"
)

func NewRegisterHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Email    string `json:"email,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		// Validate registration input
		req.Username = strings.TrimSpace(req.Username)
		if len(req.Username) < 3 || len(req.Username) > 64 {
			writeJSONError(w, http.StatusBadRequest, "username must be between 3 and 64 characters", zap.NewNop())
			return
		}
		if len(req.Password) < 8 {
			writeJSONError(w, http.StatusBadRequest, "password must be at least 8 characters", zap.NewNop())
			return
		}
		req.Email = strings.TrimSpace(req.Email)
		if req.Email != "" {
			if !strings.Contains(req.Email, "@") || !strings.Contains(req.Email, ".") {
				writeJSONError(w, http.StatusBadRequest, "invalid email format", zap.NewNop())
				return
			}
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		user, err := auth.RegisterWithUsername(r.Context(), db, req.Username, req.Email, req.Password, "", "", "", "", "", "", "", "", nil)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error(), zap.NewNop())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"id": user.ID, "username": user.Username, "email": user.Email, "created_at": user.CreatedAt})
	}
}

func NewLoginHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		token, err := auth.LoginByIdentifier(r.Context(), db, req.Username, req.Password)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, err.Error(), zap.NewNop())
			return
		}
		user, err := auth.GetUserByIdentifier(r.Context(), db, req.Username)
		if err != nil || user == nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to fetch user info", zap.NewNop())
			return
		}
		roles, err := auth.ListRoles(r.Context(), db, user.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to fetch user roles", zap.NewNop())
			return
		}
		if len(roles) == 0 {
			if user.ID == 1 {
				roles = []string{"admin", "user"}
			} else {
				roles = []string{"user"}
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"token": token,
			"user":  user,
			"roles": roles,
		})
	}
}

// NewRequestPasswordResetHandler preserves the standalone factory API. Collector
// startup uses the explicitly owned singleton dispatcher constructor below.
func NewRequestPasswordResetHandler(db *sql.DB) http.HandlerFunc {
	delivery, err := auth.PasswordResetDeliveryFromEnv()
	if err != nil || db == nil {
		return newPasswordResetRequestHandler(nil)
	}
	store := pgstore.New(db)
	return newPasswordResetRequestHandler(func(email string) bool {
		return auth.SubmitPasswordResetFallback(delivery, store, email)
	})
}
func NewRequestPasswordResetHandlerWithDispatcher(dispatcher *auth.PasswordResetDispatcher) http.HandlerFunc {
	if dispatcher == nil {
		return newPasswordResetRequestHandler(nil)
	}
	return newPasswordResetRequestHandler(dispatcher.Submit)
}
func newPasswordResetRequestHandler(submit func(string) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			Email string `json:"email"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if strings.TrimSpace(req.Email) == "" {
			writeJSONError(w, http.StatusBadRequest, "email is required", zap.NewNop())
			return
		}
		mailbox, err := mail.ParseAddress(strings.TrimSpace(req.Email))
		if err != nil || strings.ContainsAny(req.Email, "\r\n") || len(mailbox.Address) > 254 {
			writeJSONError(w, http.StatusBadRequest, "invalid email address", zap.NewNop())
			return
		}
		if submit == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "password reset is unavailable until secure email delivery is configured", zap.NewNop())
			return
		}
		// Queue availability, lookup/delivery outcome, and account existence do
		// not change the public response. Accepted work outlives the request.
		_ = submit(mailbox.Address)
		writeJSON(w, http.StatusOK, map[string]string{"status": "if the account exists, a password reset email will be sent"})
	}
}

func NewResetPasswordHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		var req struct {
			Token       string `json:"token"`
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body", zap.NewNop())
			return
		}
		if strings.TrimSpace(req.Token) == "" || len(req.NewPassword) < 8 || len(req.NewPassword) > 72 {
			writeJSONError(w, http.StatusBadRequest, "a reset token and password of at least 8 characters are required", zap.NewNop())
			return
		}
		if db == nil {
			writeJSONError(w, http.StatusInternalServerError, "db connection error", zap.NewNop())
			return
		}
		if err := auth.ResetPasswordWithToken(r.Context(), db, req.Token, req.NewPassword); err != nil {
			if errors.Is(err, auth.ErrInvalidResetToken) {
				writeJSONError(w, http.StatusBadRequest, "invalid or expired reset token", zap.NewNop())
			} else if errors.Is(err, auth.ErrInvalidResetPassword) {
				writeJSONError(w, http.StatusBadRequest, "invalid new password length", zap.NewNop())
			} else {
				writeJSONError(w, http.StatusInternalServerError, "failed to reset password", zap.NewNop())
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "password reset successful"})
	}
}

// NewWhoAmIHandler returns the authenticated user's info and roles based on JWT claims.
func NewWhoAmIHandler(db *sql.DB) http.HandlerFunc {
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
		// Extract identifiers
		var email, username string
		if v, ok := claims["email"].(string); ok {
			email = v
		}
		if v, ok := claims["username"].(string); ok {
			username = v
		}
		var uid int64
		switch v := claims["user_id"].(type) {
		case float64:
			uid = int64(v)
		case int64:
			uid = v
		}
		var user *auth.User
		// Prefer user_id, then username, then email
		if uid != 0 {
			user, err = auth.GetUserByID(r.Context(), db, uid)
		} else if username != "" {
			user, err = auth.GetUserByIdentifier(r.Context(), db, username)
		} else if email != "" {
			user, err = auth.GetUserByEmail(r.Context(), db, email)
		}
		if err != nil || user == nil {
			// If not resolvable, return claims-only for backward compatibility
			writeJSON(w, http.StatusOK, map[string]any{"claims": claims})
			return
		}
		roles, err := auth.ListRoles(r.Context(), db, user.ID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to fetch user roles", zap.NewNop())
			return
		}
		if len(roles) == 0 {
			if user.ID == 1 {
				roles = []string{"admin", "user"}
			} else {
				roles = []string{"user"}
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user":  user,
			"roles": roles,
		})
	}
}

// NewRenewTokenHandler exchanges a valid token for a new token with refreshed expiry.
func NewRenewTokenHandler(db *sql.DB) http.HandlerFunc {
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
		// Extract user ID, email, and username from claims
		var email string
		var username string
		if v, ok := claims["email"].(string); ok {
			email = v
		}
		if v, ok := claims["username"].(string); ok {
			username = v
		}
		// Backward compatibility: if no email, try subject (sub)
		if email == "" {
			if v, ok := claims["sub"].(string); ok {
				email = v
			}
		}
		var uid int64
		switch v := claims["user_id"].(type) {
		case float64:
			uid = int64(v)
		case int64:
			uid = v
		}
		var u *auth.User
		if (uid == 0 || username == "") && email != "" {
			if got, err := auth.GetUserByEmail(r.Context(), db, email); err == nil && got != nil {
				u = got
				uid = got.ID
				if username == "" {
					username = got.Username
				}
			}
		}
		// If still missing username, try lookup by identifier if we have one
		if username == "" && email != "" {
			if got, err := auth.GetUserByIdentifier(r.Context(), db, email); err == nil && got != nil {
				u = got
				username = got.Username
				if uid == 0 {
					uid = got.ID
				}
			}
		}
		// If missing email but we have user (from uid), fetch
		if email == "" && u == nil && uid != 0 {
			if got, err := auth.GetUserByIdentifier(r.Context(), db, username); err == nil && got != nil {
				u = got
				email = got.Email
			}
		}
		// If still missing identifiers, cannot proceed
		if uid == 0 || username == "" {
			writeJSONError(w, http.StatusBadRequest, "invalid token claims: missing user identifiers", zap.NewNop())
			return
		}
		// Issue new token (24h) including username convenience claim
		token, err := auth.GenerateJWTWithUsername(uid, email, username, 24*time.Hour)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to generate token", zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token})
	}
}
