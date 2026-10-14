package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang-jwt/jwt/v5"
)

func expectSessionState(mock sqlmock.Sqlmock, userID, version int64, disabled bool, roles, grants []string) {
	roleJSON, _ := json.Marshal(roles)
	grantJSON, _ := json.Marshal(grants)
	if roles == nil {
		roleJSON = []byte(`[]`)
	}
	if grants == nil {
		grantJSON = []byte(`[]`)
	}
	mock.ExpectQuery(regexp.QuoteMeta(sessionStateSQL)).WithArgs(userID).WillReturnRows(sqlmock.NewRows([]string{"id", "email", "username", "auth_version", "disabled", "tenant_id", "roles", "schemas"}).AddRow(userID, "person@example.test", "current-name", version, disabled, "current-tenant", roleJSON, grantJSON))
}
func sessionTestToken(t *testing.T, mutate func(*Claims)) string {
	t.Helper()
	claims := &Claims{UserID: 7, AuthVersion: 1, Email: "old@example.test", Username: "old-name", TenantID: "old-tenant", Roles: []string{"admin"}, Schemas: []string{"other_tenant"}, Scopes: []string{"admin"}}
	if mutate != nil {
		mutate(claims)
	}
	token, err := signSessionJWT(claims, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func expectNotRevoked(mock sqlmock.Sqlmock, token string) {
	claims, _ := ValidateJWT(token)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM admin.auth_token_blacklist`).WithArgs(claims.ID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}

func TestSessionValidationRefreshesAuthorityAndFailsClosed(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "session-unit-secret-not-a-production-key")
	cases := []struct {
		name          string
		version       int64
		disabled      bool
		roles, grants []string
		revoke        bool
		wantErr       bool
	}{
		{"explicit-grant", 1, false, []string{"viewer"}, []string{"tenant_a"}, false, false},
		{"empty-grants", 1, false, []string{"viewer"}, nil, false, false},
		{"admin-current-only", 1, false, []string{"admin"}, nil, false, false},
		{"stale-password-version", 2, false, nil, nil, false, true},
		{"disabled-account", 1, true, nil, nil, false, true},
		{"revoked-jti", 1, false, nil, nil, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			token := sessionTestToken(t, nil)
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			expectSessionState(mock, 7, tc.version, tc.disabled, tc.roles, tc.grants)
			if !tc.disabled && tc.version == 1 {
				claims, _ := ValidateJWT(token)
				count := 0
				if tc.revoke {
					count = 1
				}
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM admin.auth_token_blacklist`).WithArgs(claims.ID).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(count))
			}
			claims, err := ValidateSessionJWT(context.Background(), db, token)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v expectedFailure=%v", err, tc.wantErr)
			}
			if err == nil {
				if claims.Email != "person@example.test" || claims.Username != "current-name" || claims.TenantID != "current-tenant" || len(claims.Scopes) != 0 || claims.OrgID != nil || claims.ProjectID != nil {
					t.Fatal("stale or caller-selected identity/scope retained")
				}
				ctx := WithUserContext(context.Background(), ClaimsToUserContext(claims))
				wantTenantA := contains(tc.roles, "admin") || len(tc.grants) > 0
				if UserHasSchemaAccess(ctx, "tenant_a") != wantTenantA {
					t.Fatal("database grants not enforced")
				}
				if !contains(tc.roles, "admin") && UserHasSchemaAccess(ctx, "other_tenant") {
					t.Fatal("signed stale grant trusted")
				}
				if !contains(tc.roles, "admin") && UserHasSchemaAccess(ctx, "Tenant_A") {
					t.Fatal("schema grant incorrectly case-folded")
				}
				if UserHasRole(ctx, "admin") != contains(tc.roles, "admin") {
					t.Fatal("signed stale role trusted")
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionLegacyMissingDatabaseAndDeletedAccounts(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "session-unit-secret-not-a-production-key")
	token := sessionTestToken(t, nil)
	if _, err := ValidateSessionJWT(context.Background(), nil, token); !errors.Is(err, ErrSessionDatabaseRequired) {
		t.Fatal("missing DB did not fail closed")
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy, err := GenerateJWTWithFullContext(7, "person@example.test", "person", "", nil, []string{"admin"}, nil, time.Hour, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSessionJWT(context.Background(), db, legacy); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("legacy admin JWT accepted", err)
	}
	mock.ExpectQuery(regexp.QuoteMeta(sessionStateSQL)).WithArgs(int64(7)).WillReturnError(sql.ErrNoRows)
	if _, err := ValidateSessionJWT(context.Background(), db, token); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("deleted account accepted", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSessionMiddlewareNeverUsesCryptoOnlyFallback(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "session-unit-secret-not-a-production-key")
	token := sessionTestToken(t, nil)
	nextCalls := 0
	handler := UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalls++
		if !UserHasSchemaAccess(r.Context(), "tenant_a") {
			t.Fatal("refreshed grant missing")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest("GET", "http://example.test/api", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != http.StatusUnauthorized || nextCalls != 0 {
		t.Fatal("middleware authorized without a database")
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectSessionState(mock, 7, 1, false, []string{"viewer"}, []string{"tenant_a"})
	expectNotRevoked(mock, token)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req.WithContext(WithSessionDatabase(req.Context(), db)))
	if response.Code != http.StatusNoContent || nextCalls != 1 {
		t.Fatal("database-backed middleware failed")
	}
	mock.ExpectQuery(regexp.QuoteMeta(sessionStateSQL)).WillReturnError(errors.New("sensitive database failure"))
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, req.WithContext(WithSessionDatabase(req.Context(), db)))
	if response.Code != http.StatusUnauthorized || nextCalls != 1 || strings.Contains(response.Body.String(), "sensitive") {
		t.Fatal("database error authorized or leaked")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAPIKeySessionRefreshAndRevocation(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "session-unit-secret-not-a-production-key")
	for _, deleted := range []bool{false, true} {
		t.Run(map[bool]string{true: "deleted-key", false: "current-key"}[deleted], func(t *testing.T) {
			token := sessionTestToken(t, func(c *Claims) { c.APIKeyID = "key-fixture" })
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			expectSessionState(mock, 7, 1, false, []string{"admin"}, []string{"tenant_a"})
			expectNotRevoked(mock, token)
			query := mock.ExpectQuery(`SELECT org_id,project_id,COALESCE\(scopes,'\[\]'::jsonb\),expires_at FROM admin.auth_apikeys`).WithArgs("key-fixture", int64(7))
			if deleted {
				query.WillReturnError(sql.ErrNoRows)
			} else {
				query.WillReturnRows(sqlmock.NewRows([]string{"org_id", "project_id", "scopes", "expires_at"}).AddRow(3, 4, []byte(`["logs:read"]`), time.Now().Add(time.Minute)))
			}
			claims, err := ValidateSessionJWT(context.Background(), db, token)
			if deleted {
				if !errors.Is(err, ErrInvalidSession) {
					t.Fatal("revoked API key JWT accepted", err)
				}
			} else {
				if err != nil || len(claims.Roles) != 0 || len(claims.Scopes) != 1 || claims.Scopes[0] != "logs:read" || *claims.OrgID != 3 || *claims.ProjectID != 4 {
					t.Fatal("API key escalation or current scope lost", err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSessionRevokeRenewAndIssuanceConstraints(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "session-unit-secret-not-a-production-key")
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	token := sessionTestToken(t, nil)
	expectSessionState(mock, 7, 1, false, nil, []string{"tenant_a"})
	expectNotRevoked(mock, token)
	claims, _ := ValidateJWT(token)
	mock.ExpectExec(`INSERT INTO admin.auth_token_blacklist.*ON CONFLICT`).WithArgs(claims.ID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := RevokeSessionJWT(context.Background(), db, token); err != nil {
		t.Fatal(err)
	}
	expectSessionState(mock, 7, 1, false, nil, []string{"tenant_a"})
	expectNotRevoked(mock, token)
	expectSessionState(mock, 7, 2, false, nil, []string{"tenant_a"})
	if _, err := RenewSessionJWT(context.Background(), db, token, time.Hour); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("renewal crossed a password/version change", err)
	}
	org := int64(4)
	if _, err := IssueSessionJWT(context.Background(), db, 7, time.Hour, &org, nil, []string{"admin"}); err == nil {
		t.Fatal("user selected arbitrary machine scope")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestJWTUnknownSigningKeyFailsClosed(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "session-unit-secret-not-a-production-key")
	claims := Claims{UserID: 7, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = "unconfigured"
	signed, err := token.SignedString([]byte("session-unit-secret-not-a-production-key"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateJWT(signed); err == nil {
		t.Fatal("unknown signing key fell back to unrelated key")
	}
}
