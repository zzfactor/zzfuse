package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgconn"
	"github.com/lib/pq"
)

// Constants for configuration defaults and timeouts
const (

	// Database operation timeouts
	DatabaseTimeout     = 5 * time.Second
	DefaultConfigSchema = "admin"
	DefaultOtelSchema   = "otel"
)

func isTableNotExistError(err error) bool {
	if err == nil {
		return false
	}
	// Try pgx/pgconn
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
		return true
	}
	// Try lib/pq
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && string(pqErr.Code) == "42P01" {
		return true
	}
	// Fallback: string match
	return strings.Contains(err.Error(), "SQLSTATE 42P01") || strings.Contains(err.Error(), "does not exist")
}

// EnsureSchemaExists creates the schema if it doesn't exist
func EnsureSchemaExists(ctx context.Context, db *sql.DB, schema string) error {
	createSchemaSQL := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %q", schema)
	_, err := db.ExecContext(ctx, createSchemaSQL)
	if err != nil {
		return fmt.Errorf("failed to create schema %s: %w", schema, err)
	}
	return nil
}

// SQL identifier validation regex - allows alphanumeric, underscore, and dots
var validIdentifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)

// QuoteIdentifier safely quotes a PostgreSQL identifier
func QuoteIdentifier(identifier string) string {
	// Replace double quotes with double double quotes (PostgreSQL escaping)
	escaped := strings.ReplaceAll(identifier, `"`, `""`)
	return fmt.Sprintf(`"%s"`, escaped)
}

// ValidateAndQuoteSchema validates and quotes a schema name
func ValidateAndQuoteSchema(schema string) (string, error) {
	if schema == "" {
		return "", fmt.Errorf("schema name cannot be empty")
	}

	if !validIdentifierRegex.MatchString(schema) {
		return "", fmt.Errorf("invalid schema name: %s (must be alphanumeric, underscore, or dots only)", schema)
	}

	return QuoteIdentifier(schema), nil
}

// ValidateAndQuoteTable validates and quotes a table name
func ValidateAndQuoteTable(table string) (string, error) {
	if table == "" {
		return "", fmt.Errorf("table name cannot be empty")
	}

	if !validIdentifierRegex.MatchString(table) {
		return "", fmt.Errorf("invalid table name: %s (must be alphanumeric, underscore, or dots only)", table)
	}

	return QuoteIdentifier(table), nil
}

// BuildSafeTableReference builds a safe schema.table reference
func BuildSafeTableReference(schema, table string) (string, error) {
	quotedSchema, err := ValidateAndQuoteSchema(schema)
	if err != nil {
		return "", fmt.Errorf("invalid schema: %w", err)
	}

	quotedTable, err := ValidateAndQuoteTable(table)
	if err != nil {
		return "", fmt.Errorf("invalid table: %w", err)
	}

	return fmt.Sprintf("%s.%s", quotedSchema, quotedTable), nil
}
