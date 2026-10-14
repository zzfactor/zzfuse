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

	"github.com/dghubble/sessions"
	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
	"golang.org/x/oauth2"
	oauth2github "golang.org/x/oauth2/github"
)

var (
	githubOAuthConfig = &oauth2.Config{
		ClientID:     os.Getenv("GITHUB_CLIENT_ID"),
		ClientSecret: os.Getenv("GITHUB_CLIENT_SECRET"),
		Endpoint:     oauth2github.Endpoint,
		RedirectURL:  os.Getenv("GITHUB_REDIRECT_URL"),
		Scopes:       []string{"user:email"},
	}
	sessionName   = "github-oauth-session"
	sessionSecret = []byte(os.Getenv("SESSION_SECRET"))
	sessionStore  = sessions.NewCookieStore[string](
		sessions.DefaultCookieConfig,
		sessionSecret,
		nil, // encryption key (optional)
	)
)

func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func generateCodeVerifier() (verifier string, challenge string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

// GitHubLoginHandler redirects user to GitHub for OAuth login.
func GitHubLoginHandler(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		http.Error(w, "failed to init oauth", http.StatusInternalServerError)
		return
	}
	verifier, challenge, err := generateCodeVerifier()
	if err != nil {
		http.Error(w, "failed to init oauth", http.StatusInternalServerError)
		return
	}
	// Store state and code_verifier in secure cookies
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: state, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	http.SetCookie(w, &http.Cookie{Name: "oauth_code_verifier", Value: verifier, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	authURL := githubOAuthConfig.AuthCodeURL(state,
		oauth2.AccessTypeOnline,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// GitHubCallbackHandler handles the OAuth callback, exchanges code for token, fetches user info, and logs in or registers the user.
// Now accepts exchangeFunc and httpClient for testability. All user/link writes occur in a single transaction.
func GitHubCallbackHandler(db *sql.DB, jwtIssuer func(userID int64, email string) (string, error), exchangeFunc func(ctx context.Context, code string) (*oauth2.Token, error), httpClient *http.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
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
		// Use injected exchangeFunc or default with PKCE
		var tok *oauth2.Token
		var err error
		if exchangeFunc != nil {
			tok, err = exchangeFunc(ctx, code)
		} else {
			// include code_verifier param
			tok, err = githubOAuthConfig.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verCookie.Value))
		}
		if err != nil {
			http.Error(w, "token exchange failed", http.StatusUnauthorized)
			return
		}
		// Use injected httpClient or default
		client := httpClient
		if client == nil {
			client = githubOAuthConfig.Client(ctx, tok)
		}
		resp, err := client.Get("https://api.github.com/user")
		if err != nil || resp.StatusCode != 200 {
			http.Error(w, "failed to fetch user info", http.StatusUnauthorized)
			return
		}
		defer resp.Body.Close()
		var ghUser struct {
			ID    int64  `json:"id"`
			Login string `json:"login"`
			Email string `json:"email"`
			Name  string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
			http.Error(w, "invalid user info", http.StatusInternalServerError)
			return
		}
		// If email is empty, fetch from /emails
		if ghUser.Email == "" {
			emailResp, err := client.Get("https://api.github.com/user/emails")
			if err == nil && emailResp.StatusCode == 200 {
				var emails []struct {
					Email    string `json:"email"`
					Primary  bool   `json:"primary"`
					Verified bool   `json:"verified"`
				}
				if err := json.NewDecoder(emailResp.Body).Decode(&emails); err == nil {
					for _, e := range emails {
						if e.Primary && e.Verified {
							ghUser.Email = e.Email
							break
						}
					}
				}
				emailResp.Body.Close()
			}
		}
		if ghUser.Email == "" {
			http.Error(w, "email not available from GitHub", http.StatusUnauthorized)
			return
		}
		// Find or create/link user within a transaction
		var userID int64
		var exists bool
		err = db.QueryRowContext(ctx, fmt.Sprintf("SELECT user_id FROM %s.auth_identities WHERE provider=$1 AND provider_user_id=$2", pgstore.DefaultConfigSchema), "github", fmt.Sprint(ghUser.ID)).Scan(&userID)
		if err == sql.ErrNoRows {
			tx, txErr := db.BeginTx(ctx, nil)
			if txErr != nil {
				http.Error(w, "failed to start tx", http.StatusInternalServerError)
				return
			}
			defer tx.Rollback()
			linkErr := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT id FROM %s.auth_users WHERE email=$1", pgstore.DefaultConfigSchema), ghUser.Email).Scan(&userID)
			if linkErr == sql.ErrNoRows {
				var newUserID int64
				if err := tx.QueryRowContext(ctx, fmt.Sprintf(`INSERT INTO %s.auth_users (email, password_hash, email_verified, name) VALUES ($1,$2,$3,$4) RETURNING id`, pgstore.DefaultConfigSchema), ghUser.Email, "", true, ghUser.Name).Scan(&newUserID); err != nil {
					http.Error(w, "failed to register user", http.StatusInternalServerError)
					return
				}
				userID = newUserID
			}
			_, err = tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s.auth_identities (user_id, provider, provider_user_id, email) VALUES ($1, $2, $3, $4)", pgstore.DefaultConfigSchema), userID, "github", fmt.Sprint(ghUser.ID), ghUser.Email)
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
		token, err := jwtIssuer(userID, ghUser.Email)
		if err != nil {
			http.Error(w, "failed to issue token", http.StatusInternalServerError)
			return
		}
		// Respond with JWT (or set cookie, etc.)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"token":    token,
			"new_user": !exists,
		})
	}
}
