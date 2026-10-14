//go:build integration

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	pgxstdlib "github.com/jackc/pgx/v5/stdlib"
)

// Every run creates a unique database on the explicitly supplied disposable
// PostgreSQL service. It never truncates or migrates an existing user database.
func sessionIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_AUTH_SESSION_DSN")
	if os.Getenv("TRAZR_IT") != "1" || dsn == "" {
		t.Skip("requires TRAZR_IT=1 and TEST_AUTH_SESSION_DSN for a disposable PostgreSQL service with CREATE DATABASE privileges")
	}
	t.Setenv("BCRYPT_COST", "4")
	t.Setenv("TRAZR_PASSWORD_PEPPER", "")
	t.Setenv("PASSWORD_PEPPER", "")
	t.Setenv("TRAZR_JWT_SECRET", "session-integration-secret-not-a-production-key")
	t.Setenv("TRAZR_JWT_KID", "")
	t.Setenv("TRAZR_JWT_SECRETS", "")
	t.Setenv("TRAZR_API_KEY_PEPPER", "session-integration-api-pepper-not-a-production-key")
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("auth_session_%d", time.Now().UnixNano())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Database = name
	db := sql.OpenDB(pgxstdlib.GetConnector(*cfg))
	db.SetMaxOpenConns(16)
	t.Cleanup(func() {
		db.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, "DROP DATABASE "+name+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	return db
}

func integrationRegister(t *testing.T, db *sql.DB, name, password string) *User {
	t.Helper()
	user, err := RegisterWithUsername(context.Background(), db, name, name+"@example.test", password, name, "", "", "", "", "", "", "", map[string]interface{}{"roles": []string{"admin"}, "schemas": []string{"other_tenant"}})
	if err != nil {
		t.Fatal(err)
	}
	return user
}
func integrationLogin(t *testing.T, db *sql.DB, name, password string) string {
	t.Helper()
	token, err := LoginByIdentifier(context.Background(), db, name, password)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
func assertSessionSchema(t *testing.T, db *sql.DB, token, schema string, want bool) *Claims {
	t.Helper()
	claims, err := ValidateSessionJWT(context.Background(), db, token)
	if err != nil {
		t.Fatal(err)
	}
	if UserHasSchemaAccess(WithUserContext(context.Background(), ClaimsToUserContext(claims)), schema) != want {
		t.Fatal("unexpected current schema authorization")
	}
	return claims
}

func TestSessionMigrationRevocationAndAccessPostgres(t *testing.T) {
	db := sessionIntegrationDB(t)
	ctx := context.Background()
	legacyHash, err := HashPassword("LegacyPassword123")
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the real pre-migration account/role shape. No version/grants exist.
	oldDDL := `CREATE SCHEMA admin;
 CREATE TABLE admin.auth_users(id BIGSERIAL PRIMARY KEY,username VARCHAR(255),email VARCHAR(255) UNIQUE NOT NULL,password_hash TEXT NOT NULL,created_at TIMESTAMPTZ DEFAULT NOW(),email_verified BOOLEAN NOT NULL DEFAULT FALSE,name VARCHAR(255),first_name VARCHAR(255),last_name VARCHAR(255),address VARCHAR(255),city VARCHAR(255),state VARCHAR(255),country VARCHAR(255),company VARCHAR(255),metadata JSONB);
 CREATE TABLE admin.auth_user_roles(user_id BIGINT REFERENCES admin.auth_users(id) ON DELETE CASCADE,role VARCHAR(32) NOT NULL,PRIMARY KEY(user_id,role));`
	if _, err := db.ExecContext(ctx, oldDDL); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO admin.auth_users(username,email,password_hash) VALUES ('legacy','legacy@example.test',$1)`, legacyHash); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO admin.auth_user_roles(user_id,role) VALUES (1,'admin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE admin.auth_identities(id BIGSERIAL PRIMARY KEY,user_id BIGINT REFERENCES admin.auth_users(id),provider TEXT,provider_user_id TEXT,email TEXT,UNIQUE(provider,provider_user_id)); INSERT INTO admin.auth_identities(user_id,provider,provider_user_id,email) VALUES (1,'github','historical-owner','legacy@example.test')`); err != nil {
		t.Fatal(err)
	}

	legacyJWT, err := GenerateJWTWithFullContext(1, "legacy@example.test", "legacy", "", nil, []string{"admin"}, nil, time.Hour, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureAllAuthTables(db); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAllAuthTables(db); err != nil {
		t.Fatal(err)
	}
	var preservedIdentities int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM admin.auth_user_identities WHERE user_id=1 AND provider='github' AND provider_user_id='historical-owner'`).Scan(&preservedIdentities); err != nil || preservedIdentities != 1 {
		t.Fatal("historical OAuth mapping not copied idempotently", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM admin.auth_identities`).Scan(&preservedIdentities); err != nil || preservedIdentities != 1 {
		t.Fatal("historical OAuth table was rewritten", err)
	}
	state, err := loadSessionState(ctx, db, 1)
	if err != nil || state.AuthVersion != 1 || state.Disabled || !reflect.DeepEqual(state.Roles, []string{"admin"}) || len(state.Schemas) != 0 {
		t.Fatal("idempotent migration changed account/role or granted access", err)
	}
	var retainedHash string
	if err := db.QueryRowContext(ctx, "SELECT password_hash FROM admin.auth_users WHERE id=1").Scan(&retainedHash); err != nil || retainedHash != legacyHash {
		t.Fatal("migration changed credentials", err)
	}
	if _, err := ValidateSessionJWT(ctx, db, legacyJWT); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("legacy administrator JWT accepted", err)
	}
	if profile, err := GetUserByID(ctx, db, 1); err != nil || profile.ID != 1 || profile.Email != "legacy@example.test" {
		t.Fatal("legacy nullable profile prevents whoami", err)
	}
	adminToken := integrationLogin(t, db, "legacy", "LegacyPassword123")
	assertSessionSchema(t, db, adminToken, "any_tenant", true)
	user := integrationRegister(t, db, "member", "OldPassword123")
	token := integrationLogin(t, db, "member", "OldPassword123")
	claims := assertSessionSchema(t, db, token, "tenant_a", false)
	if len(claims.Roles) != 0 || len(claims.Schemas) != 0 {
		t.Fatal("registration metadata self-granted authority")
	}
	if err := SetUserSchemaGrants(ctx, db, user.ID, user.ID, []string{"tenant_a"}); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("nonadmin self grant accepted", err)
	}
	if err := SetUserRoles(ctx, db, user.ID, user.ID, []string{"admin"}); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("nonadmin self promotion accepted", err)
	}
	if err := SetUserRoles(ctx, db, 1, user.ID, []string{"viewer"}); err != nil {
		t.Fatal(err)
	}
	if err := SetUserSchemaGrants(ctx, db, 1, user.ID, []string{"tenant_a"}); err != nil {
		t.Fatal(err)
	}
	assertSessionSchema(t, db, token, "tenant_a", true)
	assertSessionSchema(t, db, token, "tenant_b", false)
	assertSessionSchema(t, db, token, "Tenant_A", false)
	if err := SetUserSchemaGrants(ctx, db, 1, user.ID, nil); err != nil {
		t.Fatal(err)
	}
	assertSessionSchema(t, db, token, "tenant_a", false)
	if err := SetUserSchemaGrants(ctx, db, 1, user.ID, []string{"tenant_a"}); err != nil {
		t.Fatal(err)
	}
	keyPlain, key, err := CreateAPIKey(ctx, db, user.ID, "read-only", nil, nil, []string{"logs:read"}, "1h")
	if err != nil {
		t.Fatal(err)
	}
	machine, err := ExchangeAPIKeyForJWT(ctx, db, keyPlain)
	if err != nil {
		t.Fatal(err)
	}
	machineClaims := assertSessionSchema(t, db, machine, "tenant_a", true)
	if machineClaims.APIKeyID != key.ID || len(machineClaims.Roles) != 0 || !reflect.DeepEqual(machineClaims.Scopes, []string{"logs:read"}) {
		t.Fatal("machine key authority changed")
	}
	if err := RevokeAPIKey(ctx, db, user.ID, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSessionJWT(ctx, db, machine); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("deleted API key JWT remained valid", err)
	}
	org := int64(9)
	if _, _, err := CreateAPIKey(ctx, db, user.ID, "escalated", &org, nil, []string{"logs:read"}, "1h"); !errors.Is(err, ErrAccessDenied) {
		t.Fatal("nonadmin selected arbitrary organization", err)
	}
	if _, _, err := CreateAPIKey(ctx, db, user.ID, "escalated", nil, nil, []string{"admin"}, "1h"); !errors.Is(err, ErrInvalidAccess) {
		t.Fatal("admin scope self-created", err)
	}
	if err := RevokeSessionJWT(ctx, db, token); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSessionJWT(ctx, db, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("logged-out JWT remained valid", err)
	}
	token = integrationLogin(t, db, "member", "OldPassword123")
	renewed, err := RenewSessionJWT(ctx, db, token, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := RevokeUserSessions(ctx, db, 1, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSessionJWT(ctx, db, renewed); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("all-session revoke did not reject renewed JWT", err)
	}
	token = integrationLogin(t, db, "member", "OldPassword123")
	if err := SetUserRoles(ctx, db, 1, user.ID, []string{"admin"}); err != nil {
		t.Fatal(err)
	}
	if !IsAdminFromClaims(assertSessionSchema(t, db, token, "other_tenant", true)) {
		t.Fatal("current administrator role missing")
	}
	if err := SetUserRoles(ctx, db, 1, user.ID, []string{"viewer"}); err != nil {
		t.Fatal(err)
	}
	if IsAdminFromClaims(assertSessionSchema(t, db, token, "other_tenant", false)) {
		t.Fatal("revoked administrator role remained in JWT authority")
	}
	if err := SetUserDisabled(ctx, db, 1, user.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSessionJWT(ctx, db, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("disabled session valid", err)
	}
	if _, err := LoginByIdentifier(ctx, db, "member", "OldPassword123"); err == nil {
		t.Fatal("disabled account logged in")
	}
	if err := SetUserDisabled(ctx, db, 1, user.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSessionJWT(ctx, db, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("old session revived when account re-enabled", err)
	}
	token = integrationLogin(t, db, "member", "OldPassword123")
	if err := SetUserSchemaGrants(ctx, db, 1, user.ID, []string{"tenant_a", "tenant_b"}); err != nil {
		t.Fatal(err)
	}
	// Readers must observe one complete grant set, never a partially replaced set.
	var writers sync.WaitGroup
	failures := make(chan error, 32)
	for i := 0; i < 16; i++ {
		writers.Add(1)
		go func(n int) {
			defer writers.Done()
			grants := []string{"tenant_a", "tenant_b"}
			if n%2 == 1 {
				grants = []string{"tenant_c"}
			}
			if err := SetUserSchemaGrants(ctx, db, 1, user.ID, grants); err != nil {
				failures <- err
			}
		}(i)
	}
	for i := 0; i < 32; i++ {
		c, err := ValidateSessionJWT(ctx, db, token)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(c.Schemas, []string{"tenant_a", "tenant_b"}) && !reflect.DeepEqual(c.Schemas, []string{"tenant_c"}) {
			t.Fatal("partial schema grants observed")
		}
	}
	writers.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	before, _ := loadSessionState(ctx, db, user.ID)
	resetToken, err := CreatePasswordResetToken(ctx, db, user.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan error, 2)
	var resetters sync.WaitGroup
	for i := 0; i < 2; i++ {
		resetters.Add(1)
		go func() {
			defer resetters.Done()
			outcomes <- ResetPasswordWithToken(ctx, db, resetToken, "NewPassword123")
		}()
	}
	resetters.Wait()
	close(outcomes)
	success, replay := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, ErrInvalidResetToken) {
			replay++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || replay != 1 {
		t.Fatal("concurrent reset did not have one winner")
	}
	after, _ := loadSessionState(ctx, db, user.ID)
	if after.AuthVersion != before.AuthVersion+1 {
		t.Fatal("reset version was not incremented atomically once")
	}
	if _, err := ValidateSessionJWT(ctx, db, token); !errors.Is(err, ErrInvalidSession) {
		t.Fatal("pre-reset session remained valid", err)
	}
	if _, err := LoginByIdentifier(ctx, db, "member", "OldPassword123"); err == nil {
		t.Fatal("old password remained valid")
	}
	token = integrationLogin(t, db, "member", "NewPassword123")
	resetToken, err = CreatePasswordResetToken(ctx, db, user.Email, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `CREATE FUNCTION admin.reject_session_password() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture password update denied'; END $$; CREATE TRIGGER reject_session_password BEFORE UPDATE OF password_hash ON admin.auth_users FOR EACH ROW EXECUTE FUNCTION admin.reject_session_password()`); err != nil {
		t.Fatal(err)
	}
	if err := ResetPasswordWithToken(ctx, db, resetToken, "NewestPassword123"); err == nil {
		t.Fatal("forced reset failure succeeded")
	}
	rollbackState, _ := loadSessionState(ctx, db, user.ID)
	if rollbackState.AuthVersion != after.AuthVersion {
		t.Fatal("failed reset changed session version")
	}
	if _, err := ValidateSessionJWT(ctx, db, token); err != nil {
		t.Fatal("failed reset revoked valid session", err)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER reject_session_password ON admin.auth_users"); err != nil {
		t.Fatal(err)
	}
	if err := ResetPasswordWithToken(ctx, db, resetToken, "NewestPassword123"); err != nil {
		t.Fatal("failed reset consumed one-time token", err)
	}
	token = integrationLogin(t, db, "member", "NewestPassword123")
	if err := DeleteUserAsAdmin(ctx, db, 1, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateSessionJWT(ctx, db, token); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("deleted account session remained valid", err)
	}
	if err := SetUserRoles(ctx, db, 1, 1, nil); !errors.Is(err, ErrLastAdmin) {
		t.Fatal("last administrator role removed", err)
	}
	if err := SetUserDisabled(ctx, db, 1, 1, true); !errors.Is(err, ErrLastAdmin) {
		t.Fatal("last administrator disabled", err)
	}
	if err := DeleteUserAsAdmin(ctx, db, 1, 1); !errors.Is(err, ErrLastAdmin) {
		t.Fatal("last administrator deleted", err)
	}
}

func TestConcurrentRegistrationAndLastAdminPostgres(t *testing.T) {
	db := sessionIntegrationDB(t)
	ctx := context.Background()
	if err := EnsureAllAuthTables(db); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan *User, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			name := fmt.Sprintf("member%d", n)
			user, err := RegisterWithUsername(ctx, db, name, name+"@example.test", "Password123", name, "", "", "", "", "", "", "", map[string]interface{}{"roles": []string{"admin"}})
			if err != nil {
				failures <- err
			} else {
				results <- user
			}
		}(i)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	var users []*User
	for user := range results {
		users = append(users, user)
	}
	if len(users) != 8 {
		t.Fatal("concurrent registrations lost")
	}
	var administrator int64
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*),min(user_id) FROM admin.auth_user_roles WHERE role='admin'").Scan(&count, &administrator); err != nil || count != 1 {
		t.Fatal("concurrent first-user bootstrap did not select exactly one administrator", err)
	}
	var other int64
	for _, user := range users {
		if user.ID != administrator {
			other = user.ID
			break
		}
	}
	if err := SetUserRoles(ctx, db, administrator, other, []string{"admin"}); err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan error, 2)
	for _, actor := range []int64{administrator, other} {
		wg.Add(1)
		go func(id int64) { defer wg.Done(); outcomes <- SetUserRoles(ctx, db, id, id, []string{"viewer"}) }(actor)
	}
	wg.Wait()
	close(outcomes)
	success, protected := 0, 0
	for err := range outcomes {
		if err == nil {
			success++
		} else if errors.Is(err, ErrLastAdmin) {
			protected++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || protected != 1 {
		t.Fatal("concurrent admin demotions did not protect last enabled administrator")
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM admin.auth_users u WHERE disabled=FALSE AND EXISTS(SELECT 1 FROM admin.auth_user_roles r WHERE r.user_id=u.id AND role='admin')").Scan(&count); err != nil || count != 1 {
		t.Fatal("last administrator lost", err)
	}
}
