package auth

import (
	"context"
	"database/sql"
	"time"

	pgstore "github.com/medxops/trazr-otel/pkg/auth/storage/pg"
)

// LogAuditEvent logs an admin or auth action for traceability.
func LogAuditEvent(ctx context.Context, db *sql.DB, actorID int64, action string, targetUserID *int64, details, ip string) error {
	store := pgstore.New(db)
	return store.AuditLogs().Insert(ctx, actorID, action, targetUserID, details, ip, time.Now().UTC())
}

// EnsureAuditLogsTable creates the auth_audit_logs table if it does not exist.
func EnsureAuditLogsTable(db *sql.DB) error {
	store := pgstore.New(db)
	return store.AuditLogs().EnsureSchema(context.Background())
}
