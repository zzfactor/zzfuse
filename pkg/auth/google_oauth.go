package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/coreos/go-oidc"
	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	googleOAuthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Endpoint:     google.Endpoint,
		RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
		Scopes:       []string{"openid", "email", "profile"},
	}
	googleVerifier *oidc.IDTokenVerifier
)

func InitGoogleOIDC() error {
	provider, err := oidc.NewProvider(context.Background(), "https://accounts.google.com")
	if err != nil {
		return err
	}
	googleVerifier = provider.Verifier(&oidc.Config{ClientID: googleOAuthConfig.ClientID})
	return nil
}

func generateGoogleState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateGooglePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// GoogleLoginHandler redirects user to Google for OAuth login.
func GoogleLoginHandler(w http.ResponseWriter, r *http.Request) {
	state, err := generateGoogleState()
	if err != nil {
		http.Error(w, "failed to init oauth", http.StatusInternalServerError)
		return
	}
	verifier, challenge, err := generateGooglePKCE()
	if err != nil {
		http.Error(w, "failed to init oauth", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: state, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.SetCookie(w, &http.Cookie{Name: "oauth_code_verifier", Value: verifier, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	authURL := googleOAuthConfig.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// GoogleCallbackHandler handles the OAuth callback, exchanges code for token, fetches user info, and logs in or registers the user.
// GoogleCallbackHandler handles the OAuth callback, exchanges code for token, fetches user info, and logs in or registers the user.
// All DB reads/writes for identity linking or user creation occur within a single transaction for atomicity.
func GoogleCallbackHandler(db *sql.DB, jwtIssuer func(userID int64, email string) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if db == nil || googleVerifier == nil {
			http.Error(w, "OAuth unavailable", http.StatusServiceUnavailable)
			return
		}
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		stateCookie, _ := r.Cookie("oauth_state")
		verCookie, _ := r.Cookie("oauth_code_verifier")
		if code == "" || state == "" || stateCookie == nil || verCookie == nil || stateCookie.Value != state {
			http.Error(w, "invalid oauth state", http.StatusBadRequest)
			return
		}
		// Clear cookies
		http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: "", Path: "/", MaxAge: -1})
		http.SetCookie(w, &http.Cookie{Name: "oauth_code_verifier", Value: "", Path: "/", MaxAge: -1})
		tok, err := googleOAuthConfig.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verCookie.Value))
		if err != nil {
			http.Error(w, "token exchange failed", http.StatusUnauthorized)
			return
		}
		rawIDToken, ok := tok.Extra("id_token").(string)
		if !ok {
			http.Error(w, "missing id_token", http.StatusUnauthorized)
			return
		}
		idToken, err := googleVerifier.Verify(ctx, rawIDToken)
		if err != nil {
			http.Error(w, "invalid id_token", http.StatusUnauthorized)
			return
		}
		var claims struct {
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
			Sub           string `json:"sub"`
		}
		if err := idToken.Claims(&claims); err != nil {
			http.Error(w, "invalid claims", http.StatusInternalServerError)
			return
		}
		if !claims.EmailVerified {
			http.Error(w, "email not verified", http.StatusUnauthorized)
			return
		}
		// Find or create/link user (transaction)
		var userID int64
		var exists bool
		err = db.QueryRowContext(ctx, fmt.Sprintf("SELECT user_id FROM %s.auth_user_identities WHERE provider=$1 AND provider_user_id=$2", pgstore.DefaultConfigSchema), "google", claims.Sub).Scan(&userID)
		if err == sql.ErrNoRows {
			tx, txErr := db.BeginTx(ctx, nil)
			if txErr != nil {
				http.Error(w, "failed to start tx", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()
			linkErr := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT id FROM %s.auth_users WHERE email=$1", pgstore.DefaultConfigSchema), claims.Email).Scan(&userID)
			if linkErr == sql.ErrNoRows {
				// create user within tx
				var newUserID int64
				if err := tx.QueryRowContext(ctx, fmt.Sprintf(`INSERT INTO %s.auth_users (email, password_hash, email_verified, name) VALUES ($1,$2,$3,$4) RETURNING id`, pgstore.DefaultConfigSchema), claims.Email, "", true, claims.Name).Scan(&newUserID); err != nil {
					http.Error(w, "failed to register user", http.StatusInternalServerError)
					return
				}
				userID = newUserID
			}
			_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s.auth_user_identities (user_id, provider, provider_user_id, email) VALUES ($1, $2, $3, $4)", pgstore.DefaultConfigSchema), userID, "google", claims.Sub, claims.Email)
			if err != nil {
				http.Error(w, "failed to link identity", http.StatusInternalServerError)
				return
			}
			if err := tx.Commit(); err != nil {
				http.Error(w, "failed to commit", http.StatusInternalServerError)
				return
			}
		} else if err == nil {
			exists = true
		} else {
			http.Error(w, "failed to query identity", http.StatusInternalServerError)
			return
		}
		// Issue JWT
		token, err := IssueSessionJWT(ctx, db, userID, 24*time.Hour, nil, nil, nil)
		if err != nil {
			http.Error(w, "failed to issue token", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":    token,
			"new_user": !exists,
		})
	}
}
