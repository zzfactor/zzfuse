package trazrextension

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/golang-jwt/jwt/v5"
	"github.com/medxops/trazr-otel/pkg/auth"
)

func sessionTestToken(t *testing.T, userID int64, apiKeyID string) string {
	t.Helper()
	now := time.Now()
	claims := auth.Claims{UserID: userID, AuthVersion: 1, APIKeyID: apiKeyID,
		// These deliberately differ from persisted state: authorization must be refreshed.
		Roles: []string{"admin"}, Schemas: []string{"forged_schema"},
		RegisteredClaims: jwt.RegisteredClaims{ID: fmt.Sprintf("test-session-%d", userID),
			IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), Issuer: os.Getenv("TRAZR_JWT_ISSUER")}}
	if audience := os.Getenv("TRAZR_JWT_AUDIENCE"); audience != "" {
		claims.Audience = jwt.ClaimStrings{audience}
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(os.Getenv("TRAZR_JWT_SECRET")))
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func sessionTestDatabase(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		_ = db.Close()
	})
	return db, mock
}

func expectSessionLookup(mock sqlmock.Sqlmock, userID int64, roles, schemas []string) {
	roleJSON, _ := json.Marshal(roles)
	schemaJSON, _ := json.Marshal(schemas)
	mock.ExpectQuery(`(?s)SELECT u.id,u.email,.*FROM admin.auth_users u WHERE u.id=\$1`).WithArgs(userID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "username", "auth_version", "disabled", "tenant_id", "roles", "schemas"}).
			AddRow(userID, "user@example.test", "user", int64(1), false, "tenant-one", roleJSON, schemaJSON))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM admin.auth_token_blacklist`).WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
}
