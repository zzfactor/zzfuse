package repository

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iancoleman/strcase"
	"github.com/jeremywohl/flatten"
	"github.com/medxops/trazr-otel/pkg/util"
	"go.uber.org/zap"
)

// FlattenKey normalizes a column name for use as a DB column.
//func FlattenKey(key string) string {
//	return strings.ReplaceAll(strings.ReplaceAll(key, ".", "_"), "-", "_")
//}

// TableExists checks if a table exists in the public schema.
func TableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	row := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1
	)`, tableName)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return false, fmt.Errorf("TableExists: scan error: %w", err)
	}
	return exists, nil
}

// FlattenAndNormalizeKeys flattens nested maps and normalizes keys to snake_case with underscores.
// For example: {"service": {"name": "foo"}} -> {"service_name": "foo"}
func FlattenAndNormalizeKeys(input map[string]interface{}) map[string]interface{} {
	// Use flatten to flatten the map with dot notation
	flat, err := flatten.Flatten(input, "", flatten.DotStyle)
	if err != nil {
		// If flatten fails, return the original map as a fallback
		return input
	}
	output := make(map[string]interface{}, len(flat))
	for k, v := range flat {
		// Convert dot notation to snake_case
		key := strcase.ToSnake(strings.ReplaceAll(k, ".", "_"))
		output[key] = v
	}
	return output
}

// OpenPostgressDatabase opens a connection to the admin database and ensures the target database exists.
// It resolves the DSN from various sources and creates the database if it doesn't exist.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - dsn: Data Source Name. If empty, will try TRAZR_DATABASE_DSN env var, then defaults
//   - logger: Zap logger for structured logging
//
// Returns:
//   - *sql.DB: Admin database connection (caller must close)
//   - error: Any error that occurred during the process
//
// The function performs these steps:
// 1. Resolves DSN from parameters, environment, or defaults
// 2. Ensures the target database exists (creates if necessary)
// 3. Opens and tests connection to the admin database
//
// Example usage:
//
//	ctx := context.Background()
//	logger := zap.NewProduction()
//	dbMain, err := util.OpenPostgressDatabase(ctx, "postgres://user:pass@localhost:5432/dbname", logger)
//	if err != nil {
//		return err
//	}
//	defer dbMain.Close()
func OpenPostgressDatabase(ctx context.Context, dsn string, logger *zap.Logger) (*sql.DB, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	logger.Debug("Opening admin database connection")

	// Step 1: Ensure database exists
	if err := ensureDatabaseExists(ctx, dsn, logger); err != nil {
		return nil, fmt.Errorf("failed to ensure database exists: %w", err)
	}

	// Step 2: Connect to database
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	maxOpen := util.GetEnvInt("TRAZR_DB_MAX_OPEN_CONNS", util.DefaultDBMaxOpenConns)
	maxIdle := util.GetEnvInt("TRAZR_DB_MAX_IDLE_CONNS", util.DefaultDBMaxIdleConns)
	maxLifetime := util.GetEnvDuration("TRAZR_DB_CONN_MAX_LIFETIME", util.DefaultDBConnMaxLifetime)
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(maxLifetime)

	// Test connection with timeout and log user-friendly error if it fails
	if err := testDatabaseConnection(ctx, db); err != nil {
		logger.Error("Database is not reachable. Please ensure the database is running and accessible.",
			zap.Error(err),
		)
		db.Close() // Clean up on error
		return nil, fmt.Errorf("database is not reachable: %w", err)
	}

	logger.Debug("Postgres database connection established successfully")
	return db, nil
}

// ensureDatabaseExists creates the database specified in the DSN if it doesn't exist
func ensureDatabaseExists(ctx context.Context, dsn string, logger *zap.Logger) error {
	// Parse the DSN to extract database name
	parsedURL, err := url.Parse(dsn)
	if err != nil {
		return fmt.Errorf("failed to parse DSN: %w", err)
	}

	// Extract database name from path (remove leading slash)
	targetDB := strings.TrimPrefix(parsedURL.Path, "/")
	if targetDB == "" {
		return fmt.Errorf("no database specified in DSN")
	}

	logger.Debug("Ensuring database exists", zap.String("database", targetDB))

	// Create admin DSN by replacing the database with postgres system DB
	adminURL := *parsedURL
	adminURL.Path = "/postgres"
	adminDSN := adminURL.String()

	// Connect to postgres system database
	dbMain, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to admin database: %w", err)
	}
	defer dbMain.Close()

	// Use context for timeout
	pingCtx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
	defer cancel()

	if err := dbMain.PingContext(pingCtx); err != nil {
		return fmt.Errorf("failed to ping admin database: %w", err)
	}

	// Check if the database already exists
	var exists bool
	checkQuery := "SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)"
	err = dbMain.QueryRowContext(ctx, checkQuery, targetDB).Scan(&exists)
	if err != nil {
		logger.Error("Failed to check if database exists", zap.String("database", targetDB), zap.Error(err))
		return fmt.Errorf("failed to check if database exists: %w", err)
	}
	if exists {
		logger.Debug("Database already exists", zap.String("database", targetDB))
		return nil
	}

	// Create target database if it doesn't exist
	createSQL := fmt.Sprintf("CREATE DATABASE %s", targetDB)
	execCtx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
	defer cancel()

	_, err = dbMain.ExecContext(execCtx, createSQL)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		logger.Error("Failed to create database",
			zap.String("database", targetDB),
			zap.Error(err))
		return fmt.Errorf("failed to create database %s: %w", targetDB, err)
	}

	if err == nil {
		logger.Info("Database created successfully", zap.String("database", targetDB))
	} else {
		logger.Debug("Database already exists", zap.String("database", targetDB))
	}

	return nil
}

// testDatabaseConnection tests the connection to the database with a timeout
func testDatabaseConnection(ctx context.Context, db *sql.DB) error {
	pingCtx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
	defer cancel()
	return db.PingContext(pingCtx)
}

// EncodeCursor encodes a timestamp and ID into a base64 cursor string.
// Always uses nanoseconds internally for consistency with database storage.
func EncodeCursor(ts time.Time, id string) string {
	ns := ts.UnixNano()
	raw := fmt.Sprintf("%d|%s", ns, id)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

// ParseCursor decodes a base64 cursor string into nanoseconds and ID.
// Always returns nanoseconds for consistency with database storage.
func ParseCursor(cursor string) (int64, string, error) {
	if cursor == "" {
		return 0, "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return 0, "", fmt.Errorf("invalid base64 cursor: %w", err)
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return 0, "", fmt.Errorf("invalid cursor format")
	}
	ns, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid cursor timestamp_ns: %w", err)
	}
	return ns, parts[1], nil
}

// Note: All cursor operations now use the unified ParseCursor and EncodeCursor functions
// for consistent nanosecond-based timestamp handling across the entire system.

func UnmarshalJSONField(b []byte) interface{} {
	if len(b) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b) // fallback: raw string
	}
	// If v is a string, strip quotes if present
	if s, ok := v.(string); ok {
		s = strings.TrimPrefix(s, "\"")
		s = strings.TrimSuffix(s, "\"")
		return s
	}
	return v
}

// ToStringPtr converts an interface{} to *string if it's a string, otherwise nil.
func ToStringPtr(v interface{}) *string {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

// DerefInt32 dereferences a *int32 pointer, returning 0 if nil.
// Delegates to util.Deref for consistency.
func DerefInt32(i *int32) int32 { return util.Deref(i) }

// DerefString dereferences a *string pointer, returning "" if nil.
// Delegates to util.Deref for consistency.
func DerefString(s *string) string { return util.Deref(s) }

// ErrInvalidDSN is returned when DSN configuration is invalid
var ErrInvalidDSN = fmt.Errorf("invalid database DSN configuration")

// GetDSN returns the DSN, building it from individual fields if needed.
// Returns an error if required fields are missing or invalid.
func GetDSN() (string, error) {
	host := util.GetEnvString("TRAZR_DB_HOST", "localhost")
	port := util.GetEnvString("TRAZR_DB_PORT", "5432")
	user := util.GetEnvString("TRAZR_DB_USER", "trazr")
	pass := util.GetEnvString("TRAZR_DB_PASSWORD", "trazrpass")
	db := util.GetEnvString("TRAZR_DB_NAME", "trazr")
	ssl := util.GetEnvString("TRAZR_DB_SSL_MODE", "prefer")

	// Validate port is numeric
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("%w: invalid port %q: %v", ErrInvalidDSN, port, err)
	}

	// Validate required fields
	var missing []string
	if host == "" {
		missing = append(missing, "TRAZR_DB_HOST")
	}
	if port == "" {
		missing = append(missing, "TRAZR_DB_PORT")
	}
	if user == "" {
		missing = append(missing, "TRAZR_DB_USER")
	}
	if pass == "" {
		missing = append(missing, "TRAZR_DB_PASSWORD")
	}
	if db == "" {
		missing = append(missing, "TRAZR_DB_NAME")
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("%w: missing required fields: %v", ErrInvalidDSN, missing)
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, host, port, db, ssl), nil
}
