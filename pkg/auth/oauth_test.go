package auth

import (
	"context"
	"encoding/json"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestEnsureOAuthTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_user_identities.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := EnsureOAuthTables(db); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

// roundTripFunc allows us to mock http.RoundTripper for custom http.Client
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGoogleCallbackHandler_UserCreationAndJWT(t *testing.T) {
	t.Skip("Implement Google OAuth callback handler tests here.")
}

func TestGitHubCallbackUsesVerifiedEmailAndDatabaseSession(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "oauth-session-unit-secret-not-a-production-key")
	for _, verified := range []bool{false, true} {
		t.Run(map[bool]string{false: "unverified-provider-email", true: "persisted-session"}[verified], func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			requests := 0
			client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				requests++
				body := `{"id":101,"login":"person","email":"unverified-admin@example.test","name":"Person"}`
				if request.URL.Path == "/user/emails" {
					body = `[{"email":"person@example.test","primary":true,"verified":false}]`
					if verified {
						body = `[{"email":"person@example.test","primary":true,"verified":true}]`
					}
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
			})}
			if verified {
				mock.ExpectQuery(`SELECT user_id FROM admin.auth_user_identities`).WithArgs("github", "101").WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(7))
				expectSessionState(mock, 7, 1, false, []string{"viewer"}, []string{"tenant_a"})
			}
			legacyIssuerCalls := 0
			handler := GitHubCallbackHandler(db, func(int64, string) (string, error) { legacyIssuerCalls++; return "unsafe-legacy-token", nil }, func(context.Context, string) (*oauth2.Token, error) {
				return &oauth2.Token{AccessToken: "local-fixture-only"}, nil
			}, client)
			request := httptest.NewRequest("GET", "https://example.test/callback?code=fixture-code&state=fixture-state", nil)
			request.AddCookie(&http.Cookie{Name: "oauth_state", Value: "fixture-state"})
			request.AddCookie(&http.Cookie{Name: "oauth_code_verifier", Value: "fixture-verifier"})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if legacyIssuerCalls != 0 || requests != 2 {
				t.Fatal("OAuth trusted legacy issuer or skipped verified-email lookup")
			}
			if !verified {
				if response.Code != http.StatusUnauthorized {
					t.Fatal("unverified provider email accepted")
				}
			} else {
				if response.Code != http.StatusOK {
					t.Fatalf("callback failed status %d", response.Code)
				}
				var body struct {
					Token string `json:"token"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				claims, err := ValidateJWT(body.Token)
				if err != nil || claims.AuthVersion != 1 || claims.Email != "person@example.test" || !reflect.DeepEqual(claims.Roles, []string{"viewer"}) || !reflect.DeepEqual(claims.Schemas, []string{"tenant_a"}) {
					t.Fatal("OAuth emitted a legacy or escalated session", err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
