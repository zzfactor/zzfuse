package auth

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func expectAdminMutation(mock sqlmock.Sqlmock, actorID, targetID int64, actorRoles, targetRoles []string) {
	mock.ExpectBegin()
	mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WithArgs(authMutationLock).WillReturnResult(sqlmock.NewResult(0, 0))
	expectSessionState(mock, actorID, 1, false, actorRoles, nil)
	mock.ExpectQuery(`SELECT id FROM admin.auth_users WHERE id=\$1 FOR UPDATE`).WithArgs(targetID).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(targetID))
	expectSessionState(mock, targetID, 1, false, targetRoles, nil)
}

func TestAccessMutationCannotSelfGrantOrPromote(t *testing.T) {
	for _, change := range []func(context.Context, *sql.DB, int64, int64) error{
		func(ctx context.Context, db *sql.DB, actor, target int64) error {
			return SetUserSchemaGrants(ctx, db, actor, target, []string{"tenant_a"})
		},
		func(ctx context.Context, db *sql.DB, actor, target int64) error {
			return SetUserRoles(ctx, db, actor, target, []string{"admin"})
		},
		func(ctx context.Context, db *sql.DB, actor, target int64) error {
			return RevokeUserSessions(ctx, db, actor, target)
		},
		func(ctx context.Context, db *sql.DB, actor, target int64) error {
			return SetUserDisabled(ctx, db, actor, target, true)
		},
		func(ctx context.Context, db *sql.DB, actor, target int64) error {
			return DeleteUserAsAdmin(ctx, db, actor, target)
		},
	} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		mock.ExpectBegin()
		mock.ExpectExec(`SELECT pg_advisory_xact_lock`).WillReturnResult(sqlmock.NewResult(0, 0))
		expectSessionState(mock, 7, 1, false, []string{"viewer"}, nil)
		mock.ExpectRollback()
		if err := change(context.Background(), db, 7, 7); !errors.Is(err, ErrAccessDenied) {
			t.Fatal("nonadmin self-escalation accepted", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

func TestAccessLastEnabledAdministratorProtected(t *testing.T) {
	for _, change := range []func(context.Context, *sql.DB, int64, int64) error{
		func(ctx context.Context, db *sql.DB, actor, target int64) error {
			return SetUserRoles(ctx, db, actor, target, []string{"viewer"})
		},
		func(ctx context.Context, db *sql.DB, actor, target int64) error {
			return SetUserDisabled(ctx, db, actor, target, true)
		},
		DeleteUserAsAdmin,
	} {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		expectAdminMutation(mock, 7, 7, []string{"admin"}, []string{"admin"})
		mock.ExpectQuery(`SELECT count\(\*\) FROM admin.auth_users u WHERE u.disabled=FALSE`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectRollback()
		if err := change(context.Background(), db, 7, 7); !errors.Is(err, ErrLastAdmin) {
			t.Fatal("last administrator removed", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

func TestSchemaGrantReplacementAtomicAndExact(t *testing.T) {
	for _, bad := range []string{"*", "Tenant_A", "tenant.a", "tenant_a;DROP TABLE auth_users", "", " tenant_a"} {
		if err := SetUserSchemaGrants(context.Background(), nil, 1, 2, []string{bad}); !errors.Is(err, ErrInvalidAccess) {
			t.Fatal("invalid grant accepted", err)
		}
	}
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expectAdminMutation(mock, 1, 2, []string{"admin"}, []string{"viewer"})
	mock.ExpectExec(`DELETE FROM admin.auth_user_schema_grants`).WithArgs(int64(2)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin.auth_user_schema_grants`).WithArgs(int64(2), "tenant_a").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO admin.auth_user_schema_grants`).WithArgs(int64(2), "tenant_b").WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()
	if err := SetUserSchemaGrants(context.Background(), db, 1, 2, []string{"tenant_b", "tenant_a", "tenant_a"}); err == nil {
		t.Fatal("failed replacement committed partial grants")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAccessReadUsesSnapshotWithoutMutationLock(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	expectSessionState(mock, 1, 1, false, []string{"admin"}, nil)
	expectSessionState(mock, 2, 3, false, []string{"viewer"}, []string{"tenant_a"})
	mock.ExpectCommit()
	access, err := GetUserAccess(context.Background(), db, 1, 2)
	if err != nil || access.AuthVersion != 3 || len(access.Schemas) != 1 || access.Schemas[0] != "tenant_a" {
		t.Fatal("current account access not returned", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
