package auth

import (
	"net/http"
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
