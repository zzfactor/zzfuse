package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestHashAndCheckPassword(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		plain string
		wrong string
	}{
		{"simple", "password123", "wrongpass"},
		{"symbols", "P@ssw0rd!", "P@ssw0rd!!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := HashPassword(tc.plain)
			if err != nil {
				t.Fatalf("failed to hash password: %v", err)
			}
			if err := CheckPassword(hash, tc.plain); err != nil {
				t.Errorf("expected password to match hash")
			}
			if err := CheckPassword(hash, tc.wrong); err == nil {
				t.Errorf("expected wrong password to not match hash")
			}
		})
	}
}

func TestJWTCreateAndValidate(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "test-secret-for-jwt-unit-tests")
	expiry := 1 * time.Hour
	userID := int64(42)
	email := "test@example.com"
	token, err := GenerateJWT(userID, email, expiry)
	if err != nil {
		t.Fatalf("failed to create JWT: %v", err)
	}
	parsed, err := ValidateJWT(token)
	if err != nil {
		t.Fatalf("failed to validate JWT: %v", err)
	}
	if parsed.UserID != userID || parsed.Email != email {
		t.Errorf("claims mismatch: got %+v, want userID=%d, email=%s", parsed, userID, email)
	}
}

func TestRegisterUser(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	cases := []struct {
		name      string
		email     string
		password  string
		mockSetup func()
		wantErr   bool
	}{
		{
			name:     "success",
			email:    "user@example.com",
			password: "password123",
			mockSetup: func() {
				mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM admin.auth_users WHERE LOWER\\(username\\)=LOWER\\(\\$1\\) OR LOWER\\(email\\)=LOWER\\(\\$2\\)\\)").
					WithArgs("user@example.com", "user@example.com").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
				mock.ExpectBegin()
				mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(authMutationLock).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectQuery("INSERT INTO admin.auth_users").
					WithArgs(
						"user@example.com", // username
						"user@example.com", // email
						sqlmock.AnyArg(),   // password hash
						sqlmock.AnyArg(),   // name
						sqlmock.AnyArg(),   // firstName
						sqlmock.AnyArg(),   // lastName
						sqlmock.AnyArg(),   // address
						sqlmock.AnyArg(),   // city
						sqlmock.AnyArg(),   // state
						sqlmock.AnyArg(),   // country
						sqlmock.AnyArg(),   // company
						sqlmock.AnyArg(),   // metadata
					).
					WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, time.Now()))
				mock.ExpectQuery(`SELECT COUNT\(\*\) FROM admin.auth_users`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
				mock.ExpectExec(`INSERT INTO admin.auth_user_roles`).WithArgs(int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectCommit()

			},
			wantErr: false,
		},
		{
			name:     "duplicate email",
			email:    "user@example.com",
			password: "password123",
			mockSetup: func() {
				mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM admin.auth_users WHERE LOWER\\(username\\)=LOWER\\(\\$1\\) OR LOWER\\(email\\)=LOWER\\(\\$2\\)\\)").
					WithArgs("user@example.com", "user@example.com").
					WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup()
			_, err := Register(context.Background(), db, tc.email, tc.password, "", "", "", "", "", "", "", "", nil)
			if (err != nil) != tc.wantErr {
				t.Errorf("got error %v, wantErr %v", err, tc.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	t.Setenv("TRAZR_JWT_SECRET", "test-secret-for-jwt-unit-tests")
	hash, _ := HashPassword("password123")

	type caseDef struct {
		name     string
		email    string
		password string
		setup    func(sqlmock.Sqlmock)
		wantErr  bool
	}
	cases := []caseDef{
		{
			name:     "success",
			email:    "user@example.com",
			password: "password123",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT id, password_hash, email, COALESCE\(username,''\), auth_version, disabled FROM admin.auth_users WHERE LOWER\(username\)=\$1 OR LOWER\(email\)=\$1`).
					WithArgs("user@example.com").
					WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash", "email", "username", "auth_version", "disabled"}).AddRow(1, hash, "user@example.com", "user@example.com", 1, false))
				mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempted_at\), 'epoch'::timestamptz\), COUNT\(\*\) FROM admin.auth_login_attempts`).
					WithArgs(int64(1), sqlmock.AnyArg()).
					WillReturnRows(sqlmock.NewRows([]string{"last_failed", "count"}).AddRow(time.Now().Add(-time.Hour), 0))
				mock.ExpectExec(`(?s)DELETE FROM\s+admin.auth_login_attempts\s+WHERE user_id=\$1`).
					WithArgs(int64(1)).
					WillReturnResult(sqlmock.NewResult(0, 1))
				mock.ExpectExec(`INSERT INTO admin.auth_login_attempts`).
					WithArgs(int64(1), sqlmock.AnyArg(), true).
					WillReturnResult(sqlmock.NewResult(1, 1))
				mock.ExpectExec(`INSERT INTO admin.auth_audit_logs`).WillReturnResult(sqlmock.NewResult(0, 1))
				expectSessionState(mock, 1, 1, false, []string{"user"}, []string{"tenant_a"})

			},
			wantErr: false,
		},
		{
			name:     "wrong password",
			email:    "user@example.com",
			password: "wrongpass",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT id, password_hash, email, COALESCE\(username,''\), auth_version, disabled FROM admin.auth_users WHERE LOWER\(username\)=\$1 OR LOWER\(email\)=\$1`).
					WithArgs("user@example.com").
					WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash", "email", "username", "auth_version", "disabled"}).AddRow(1, hash, "user@example.com", "user@example.com", 1, false))
				mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempted_at\), 'epoch'::timestamptz\), COUNT\(\*\) FROM admin.auth_login_attempts`).
					WithArgs(int64(1), sqlmock.AnyArg()).
					WillReturnRows(sqlmock.NewRows([]string{"last_failed", "count"}).AddRow(time.Now().Add(-time.Hour), 0))
				mock.ExpectExec(`INSERT INTO admin.auth_login_attempts`).
					WithArgs(int64(1), sqlmock.AnyArg(), false).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			wantErr: true,
		},
		{
			name:     "wrong email",
			email:    "notfound@example.com",
			password: "password123",
			setup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT id, password_hash, email, COALESCE\(username,''\), auth_version, disabled FROM admin.auth_users WHERE LOWER\(username\)=\$1 OR LOWER\(email\)=\$1`).
					WithArgs("notfound@example.com").
					WillReturnError(sql.ErrNoRows)
			},
			wantErr: true,
		},
		{
			name:     "empty input",
			email:    "",
			password: "",
			setup:    func(mock sqlmock.Sqlmock) {},
			wantErr:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to open sqlmock: %v", err)
			}
			defer db.Close()
			tc.setup(mock)
			_, err = Login(context.Background(), db, tc.email, tc.password)
			if (err != nil) != tc.wantErr {
				t.Errorf("got error %v, wantErr %v", err, tc.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unfulfilled expectations: %v", err)
			}
		})
	}
}

func TestLogin_Errors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name    string
		email   string
		pw      string
		mock    func()
		wantErr string
	}{
		{"empty email", "", "pw", func() {}, "username/email and password required"},
		{"empty password", "user@example.com", "", func() {}, "username/email and password required"},
		{"user not found", "notfound@example.com", "pw", func() {
			mock.ExpectQuery("SELECT id, password_hash, email, COALESCE\\(username,''\\), auth_version, disabled FROM admin.auth_users WHERE LOWER\\(username\\)=\\$1 OR LOWER\\(email\\)=\\$1").WithArgs("notfound@example.com").WillReturnError(sql.ErrNoRows)
		}, "invalid username/email or password"},
		{"db error", "dberr@example.com", "pw", func() {
			mock.ExpectQuery("SELECT id, password_hash, email, COALESCE\\(username,''\\), auth_version, disabled FROM admin.auth_users WHERE LOWER\\(username\\)=\\$1 OR LOWER\\(email\\)=\\$1").WithArgs("dberr@example.com").WillReturnError(errors.New("db fail"))
		}, "query user: db fail"},
		{"wrong password", "user@example.com", "wrongpw", func() {
			mock.ExpectQuery("SELECT id, password_hash, email, COALESCE\\(username,''\\), auth_version, disabled FROM admin.auth_users WHERE LOWER\\(username\\)=\\$1 OR LOWER\\(email\\)=\\$1").WithArgs("user@example.com").WillReturnRows(sqlmock.NewRows([]string{"id", "password_hash", "email", "username", "auth_version", "disabled"}).AddRow(1, "$2a$10$invalidhash", "user@example.com", "user@example.com", 1, false))
			// lockout ok
			mock.ExpectQuery(`SELECT COALESCE\(MAX\(attempted_at\), 'epoch'::timestamptz\), COUNT\(\*\) FROM admin.auth_login_attempts`).
				WithArgs(int64(1), sqlmock.AnyArg()).
				WillReturnRows(sqlmock.NewRows([]string{"last_failed", "count"}).AddRow(time.Now().Add(-time.Hour), 0))
			// record failed
			mock.ExpectExec(`INSERT INTO auth_login_attempts`).WithArgs(int64(1), sqlmock.AnyArg(), false).WillReturnResult(sqlmock.NewResult(1, 1))
		}, "invalid username/email or password"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mock()
			_, err := Login(context.Background(), db, tt.email, tt.pw)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("got err %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestEnsureAllAuthTables(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectExec(`CREATE SCHEMA IF NOT EXISTS admin`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_users.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_password_resets.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_email_verifications.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_login_attempts.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_token_blacklist.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_user_roles.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_mfa_secrets.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_audit_logs.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_user_identities.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`(?s)CREATE TABLE IF NOT EXISTS admin.auth_apikeys.*`).WillReturnResult(sqlmock.NewResult(0, 0))
	if err := EnsureAllAuthTables(db); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestRegister_Errors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock: %v", err)
	}
	defer db.Close()

	tests := []struct {
		name    string
		email   string
		pw      string
		mock    func()
		wantErr string
	}{
		{"empty email", "", "pw", func() {}, "username and password required"},
		{"empty password", "user@example.com", "", func() {}, "username and password required"},
		{"user exists", "exists@example.com", "pw", func() {
			mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM admin.auth_users WHERE LOWER\\(username\\)=LOWER\\(\\$1\\) OR LOWER\\(email\\)=LOWER\\(\\$2\\)\\)").WithArgs("exists@example.com", "exists@example.com").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
		}, "user already exists"},
		{"db error", "dberr2@example.com", "pw", func() {
			mock.ExpectQuery("SELECT EXISTS\\(SELECT 1 FROM admin.auth_users WHERE LOWER\\(username\\)=LOWER\\(\\$1\\) OR LOWER\\(email\\)=LOWER\\(\\$2\\)\\)").WithArgs("dberr2@example.com", "dberr2@example.com").WillReturnError(errors.New("db fail"))
		}, "check existing user: db fail"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt := tt // capture range var
			tt.mock()
			_, err := Register(context.Background(), db, tt.email, tt.pw, "", "", "", "", "", "", "", "", nil)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("got err %v, want %q", err, tt.wantErr)
			}
		})
	}
}
