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

// NewWhoAmIHandler returns the current database-backed identity and access grants.
func NewWhoAmIHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		claims, err := auth.ValidateSessionJWTFromRequest(r, db)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		user, err := auth.GetUserByID(r.Context(), db, claims.UserID)
		if err != nil || user == nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user": user, "roles": claims.Roles, "schemas": claims.Schemas,
		})
	}
}

// Renewal checks account state and preserves the restrictions of machine tokens.
func NewRenewTokenHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		existing, err := auth.ExtractTokenFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		token, err := auth.RenewSessionJWT(r.Context(), db, existing, 24*time.Hour)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"token": token})
	}
}

func NewLogoutHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed", zap.NewNop())
			return
		}
		token, err := auth.ExtractTokenFromRequest(r)
		if err != nil || auth.RevokeSessionJWT(r.Context(), db, token) != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", zap.NewNop())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
