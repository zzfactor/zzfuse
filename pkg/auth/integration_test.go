//go:build integration

package auth

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// getTestDSN requires an explicit dedicated Postgres DSN for integration tests.
func getTestDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Fatal("set TEST_PG_DSN to a dedicated database for integration tests")
	}
	return dsn
}

func TestAuthIntegration_NegativePaths(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := EnsureAllAuthTables(db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	ctx := context.Background()
	base := time.Now().UnixNano()
	email := fmt.Sprintf("neg.user+%d@trazr.local", base)
	password := "P@ssw0rd!123"

	// Register a user
	u, err := Register(ctx, db, email, password, "Neg User", "Neg", "User", "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// 1) Wrong password attempts should fail and accumulate
	for i := 0; i < MaxFailedAttempts; i++ {
		if _, err := Login(ctx, db, email, "wrong-pass"); err == nil {
			t.Fatalf("expected login failure on wrong password, attempt %d", i+1)
		}
	}

	// 2) Subsequent login should be blocked due to lockout
	if _, err := Login(ctx, db, email, password); err == nil {
		t.Fatalf("expected lockout error after failed attempts")
	}

	// 3) Login with non-existent user should fail
	if _, err := Login(ctx, db, "doesnotexist@trazr.local", "whatever"); err == nil {
		t.Fatalf("expected error for non-existent user login")
	}

	// 4) Tampered JWT should fail validation
	token, err := GenerateJWT(u.ID, strings.ToLower(email), 5*time.Minute)
	if err != nil {
		t.Fatalf("generate jwt: %v", err)
	}
	tampered := token
	if len(tampered) > 10 {
		tampered = tampered[:10] + "x" + tampered[11:]
	} else {
		tampered = tampered + "x"
	}
	if _, err := ValidateJWT(tampered); err == nil {
		t.Fatalf("expected tampered token to fail validation")
	}

	// 5) Query missing user should return error
	if _, err := GetUserByEmail(ctx, db, "missing.user@trazr.local"); err == nil {
		t.Fatalf("expected error for missing user")
	}
}

func TestAuthIntegration_Roles(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := EnsureAllAuthTables(db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	ctx := context.Background()
	base := time.Now().UnixNano()
	email := fmt.Sprintf("rbac.user+%d@trazr.local", base)
	password := "P@ssw0rd!123"

	// Register a fresh user to work with
	u, err := Register(ctx, db, email, password, "RBAC User", "RBAC", "User", "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Assign roles
	if err := AssignRole(ctx, db, u.ID, "editor"); err != nil {
		t.Fatalf("assign role editor: %v", err)
	}
	if err := AssignRole(ctx, db, u.ID, "viewer"); err != nil {
		t.Fatalf("assign role viewer: %v", err)
	}
	// Idempotent assignment should not error
	if err := AssignRole(ctx, db, u.ID, "viewer"); err != nil {
		t.Fatalf("assign role viewer again: %v", err)
	}

	// HasRole checks
	has, err := HasRole(ctx, db, u.ID, "editor")
	if err != nil || !has {
		t.Fatalf("expected has editor role, got has=%v err=%v", has, err)
	}
	has, err = HasRole(ctx, db, u.ID, "admin")
	if err != nil || has {
		t.Fatalf("expected no admin role, got has=%v err=%v", has, err)
	}

	// ListRoles should contain both editor and viewer
	roles, err := ListRoles(ctx, db, u.ID)
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	seen := map[string]bool{}
	for _, r := range roles {
		seen[r] = true
	}
	if !(seen["editor"] && seen["viewer"]) {
		t.Fatalf("expected roles [editor viewer], got %v", roles)
	}

	// Remove a role and verify
	if err := RemoveRole(ctx, db, u.ID, "viewer"); err != nil {
		t.Fatalf("remove role viewer: %v", err)
	}
	has, err = HasRole(ctx, db, u.ID, "viewer")
	if err != nil || has {
		t.Fatalf("expected viewer removed, got has=%v err=%v", has, err)
	}

	// Cleanup: delete user
	if err := DeleteUser(ctx, db, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
}

func TestAuthIntegration_UserCRUD(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := EnsureAllAuthTables(db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	ctx := context.Background()
	base := time.Now().UnixNano()
	email := fmt.Sprintf("crud.user+%d@trazr.local", base)
	password := "P@ssw0rd!123"

	// Register user
	u, err := Register(ctx, db, email, password, "Crud User", "Crud", "User", "", "", "", "", "", nil)
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	// Update profile (email + name)
	newEmail := fmt.Sprintf("crud.user.updated+%d@trazr.local", base)
	if err := UpdateUserProfile(ctx, db, u.ID, newEmail, "Updated Name"); err != nil {
		t.Fatalf("update profile: %v", err)
	}

	// Verify update via GetUserByEmail
	got, err := GetUserByEmail(ctx, db, newEmail)
	if err != nil {
		t.Fatalf("get by email after update: %v", err)
	}
	if got.ID != u.ID || got.Email != strings.ToLower(newEmail) || got.Name != "Updated Name" {
		t.Fatalf("unexpected updated user: %+v", got)
	}

	// List users and ensure our user is present
	users, err := ListUsers(ctx, db)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	found := false
	for _, uu := range users {
		if uu.ID == u.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("user not found in list")
	}

	// Delete user
	if err := DeleteUser(ctx, db, u.ID); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	// Verify deletion
	if _, err := GetUserByEmail(ctx, db, newEmail); err == nil {
		t.Fatalf("expected error for deleted user, got nil")
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := getTestDSN(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	// Wait briefly for readiness in case the service just started
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("failed to ping db: %v", err)
	}
	return db
}

func TestAuthIntegration_RegisterLoginValidate(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	if err := EnsureAllAuthTables(db); err != nil {
		t.Fatalf("ensure tables: %v", err)
	}

	ctx := context.Background()
	email := "test.user+integration@trazr.local"
	password := "P@ssw0rd!123"

	// Clean up any previous test user
	_, _ = db.ExecContext(ctx, `DELETE FROM admin.auth_users WHERE email=$1`, strings.ToLower(email))

	// Register
	u, err := Register(ctx, db, email, password, "Test User", "Test", "User", "", "", "", "", "", map[string]any{"source": "integration"})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if u.ID == 0 || u.Email != strings.ToLower(email) {
		t.Fatalf("unexpected user returned: %+v", u)
	}

	// Register again should fail with conflict
	if _, err := Register(ctx, db, email, password, "", "", "", "", "", "", "", "", nil); err == nil {
		t.Fatalf("expected duplicate register error, got nil")
	}

	// Login
	token, err := Login(ctx, db, email, password)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if token == "" {
		t.Fatalf("empty token returned")
	}

	// Validate JWT
	claims, err := ValidateJWT(token)
	if err != nil {
		t.Fatalf("validate jwt: %v", err)
	}
	if claims.UserID != u.ID || claims.Email != strings.ToLower(email) {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}
