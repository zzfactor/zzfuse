package auth

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// Registration and administrative authorization changes share this transaction
// lock so first-admin selection and last-admin protection cannot race each other.
const authMutationLock int64 = 0x7a7a667573656175

func withAdminTarget(ctx context.Context, db *sql.DB, actorID, targetID int64, fn func(*sql.Tx, *UserAccess) error) error {
	if db == nil {
		return ErrSessionDatabaseRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", authMutationLock); err != nil {
		return err
	}
	actor, err := loadSessionState(ctx, tx, actorID)
	if err != nil {
		return err
	}
	if actor.Disabled || !contains(actor.Roles, "admin") {
		return ErrAccessDenied
	}
	var id int64
	if err := tx.QueryRowContext(ctx, "SELECT id FROM admin.auth_users WHERE id=$1 FOR UPDATE", targetID).Scan(&id); err != nil {
		return err
	}
	target, err := loadSessionState(ctx, tx, targetID)
	if err != nil {
		return err
	}
	if err := fn(tx, target); err != nil {
		return err
	}
	return tx.Commit()
}

func GetUserAccess(ctx context.Context, db *sql.DB, actorID, targetID int64) (*UserAccess, error) {
	if db == nil {
		return nil, ErrSessionDatabaseRequired
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// A consistent read snapshot avoids blocking administrative mutations.
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	actor, err := loadSessionState(ctx, tx, actorID)
	if err != nil {
		return nil, err
	}
	if actor.Disabled || !contains(actor.Roles, "admin") {
		return nil, ErrAccessDenied
	}
	target, err := loadSessionState(ctx, tx, targetID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return target, nil
}

func SetUserSchemaGrants(ctx context.Context, db *sql.DB, actorID, targetID int64, schemas []string) error {
	if len(schemas) > 256 {
		return ErrInvalidGrant
	}
	grants := make([]string, 0, len(schemas))
	seen := map[string]bool{}
	for _, schema := range schemas {
		// Grants refer to exact schema identities, not wildcard/case-folded aliases.
		if strings.TrimSpace(schema) != schema || pgstore.ValidateSQLSchema(schema) != nil {
			return ErrInvalidGrant
		}
		if !seen[schema] {
			seen[schema] = true
			grants = append(grants, schema)
		}
	}
	sort.Strings(grants)
	return withAdminTarget(ctx, db, actorID, targetID, func(tx *sql.Tx, _ *UserAccess) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM admin.auth_user_schema_grants WHERE user_id=$1", targetID); err != nil {
			return err
		}
		for _, grant := range grants {
			if _, err := tx.ExecContext(ctx, "INSERT INTO admin.auth_user_schema_grants(user_id,schema_name) VALUES ($1,$2)", targetID, grant); err != nil {
				return err
			}
		}
		return nil
	})
}

func protectLastAdmin(ctx context.Context, tx *sql.Tx, target *UserAccess) error {
	if target.Disabled || !contains(target.Roles, "admin") {
		return nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM admin.auth_users u WHERE u.disabled=FALSE AND EXISTS(SELECT 1 FROM admin.auth_user_roles r WHERE r.user_id=u.id AND r.role='admin')`).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrLastAdmin
	}
	return nil
}

func SetUserRoles(ctx context.Context, db *sql.DB, actorID, targetID int64, roles []string) error {
	if len(roles) > 8 {
		return ErrInvalidRole
	}
	cleaned := []string{}
	seen := map[string]bool{}
	for _, role := range roles {
		switch role {
		case "admin", "user", "viewer", "editor":
		default:
			return ErrInvalidRole
		}
		if !seen[role] {
			seen[role] = true
			cleaned = append(cleaned, role)
		}
	}
	sort.Strings(cleaned)
	return withAdminTarget(ctx, db, actorID, targetID, func(tx *sql.Tx, target *UserAccess) error {
		if !contains(cleaned, "admin") {
			if err := protectLastAdmin(ctx, tx, target); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM admin.auth_user_roles WHERE user_id=$1", targetID); err != nil {
			return err
		}
		for _, role := range cleaned {
			if _, err := tx.ExecContext(ctx, "INSERT INTO admin.auth_user_roles(user_id,role) VALUES ($1,$2)", targetID, role); err != nil {
				return err
			}
		}
		return nil
	})
}

func RevokeUserSessions(ctx context.Context, db *sql.DB, actorID, targetID int64) error {
	return withAdminTarget(ctx, db, actorID, targetID, func(tx *sql.Tx, _ *UserAccess) error {
		_, err := tx.ExecContext(ctx, "UPDATE admin.auth_users SET auth_version=auth_version+1 WHERE id=$1", targetID)
		return err
	})
}

func SetUserDisabled(ctx context.Context, db *sql.DB, actorID, targetID int64, disabled bool) error {
	return withAdminTarget(ctx, db, actorID, targetID, func(tx *sql.Tx, target *UserAccess) error {
		if disabled {
			if err := protectLastAdmin(ctx, tx, target); err != nil {
				return err
			}
		}
		if target.Disabled == disabled {
			return nil
		}
		_, err := tx.ExecContext(ctx, "UPDATE admin.auth_users SET disabled=$1,auth_version=auth_version+1 WHERE id=$2", disabled, targetID)
		return err
	})
}

func DeleteUserAsAdmin(ctx context.Context, db *sql.DB, actorID, targetID int64) error {
	return withAdminTarget(ctx, db, actorID, targetID, func(tx *sql.Tx, target *UserAccess) error {
		if err := protectLastAdmin(ctx, tx, target); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM admin.auth_users WHERE id=$1", targetID)
		return err
	})
}
