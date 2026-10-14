package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// EnsureEmbeddingWorkerClaims upgrades a tenant lazily when its first worker
// polls. Serialize concurrent first starts without adding startup side effects.
func EnsureEmbeddingWorkerClaims(ctx context.Context, db *sql.DB, schema string) error {
	ref, err := BuildSafeTableReference(schema, "embedding_worker_claims")
	if err != nil {
		return err
	}
	schemaRef := strings.TrimSuffix(ref, `."embedding_worker_claims"`)
	ddl, err := sqlFiles.ReadFile("ddl/embedding_worker_claims.sql")
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", schema+".embedding_worker_claims"); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, strings.ReplaceAll(string(ddl), "{{.Schema}}", schemaRef)); err != nil {
		return fmt.Errorf("initialize worker claims: %w", err)
	}
	return tx.Commit()
}
