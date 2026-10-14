package database

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/util"
)

//go:embed ddl/*.sql ddl/migrations/*.sql
var sqlFiles embed.FS

// SchemaManager handles creation of OTEL data tables and related schema
// in the configured database schema (e.g., DefaultSchema).
type SchemaManager struct {
}

const (
	SchemaConfigTableName = "config_otel_database"

	schemaConfigSelectColumns = "id, name, schema, enabled, description, routing, priority, metadata, created_at, updated_at"
)

type DatabaseSchemaConfig struct {
	ID             int64                  `json:"id"`
	Name           string                 `json:"name"`
	DatabaseSchema string                 `json:"schema"`
	Enabled        bool                   `json:"enabled"`
	Description    string                 `json:"description"`
	Routing        map[string]interface{} `json:"routing"`
	Priority       int                    `json:"priority"`
	Metadata       map[string]interface{} `json:"metadata"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

// GetProcessingMode returns the processing mode from metadata, with fallback to default
func (c *DatabaseSchemaConfig) GetProcessingMode() string {
	if c.Metadata == nil {
		return "hybrid" // Default processing mode
	}

	if mode, ok := c.Metadata["processing_mode"].(string); ok && mode != "" {
		return mode
	}

	return "hybrid" // Default processing mode
}

type SchemaConfigRepository struct {
	DB     *sql.DB
	schema string
	Logger util.Logger
}

// NewDatabaseSchemaConfig creates a new schema config repository that implements SchemaConfigInterface
func NewDatabaseSchemaConfig(ctx context.Context, db *sql.DB, logger util.Logger, defaultOtelSchema string) (SchemaConfigInterface, error) {
	repo := &SchemaConfigRepository{DB: db, schema: DefaultConfigSchema, Logger: logger}
	if err := repo.EnsureTableAndDefault(ctx, defaultOtelSchema); err != nil {
		return nil, err
	}
	return repo, nil
}

// --- SchemaConfigRepository Interface Methods (CRUD) ---

func (r *SchemaConfigRepository) GetByID(ctx context.Context, id int64) (*DatabaseSchemaConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, schemaConfigSelectColumns, r.getTableName())
	row := r.DB.QueryRowContext(ctx, query, id)
	config, err := scanSchemaConfigRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

func (r *SchemaConfigRepository) GetByName(ctx context.Context, name string) (*DatabaseSchemaConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE name = $1`, schemaConfigSelectColumns, r.getTableName())
	row := r.DB.QueryRowContext(ctx, query, name)
	config, err := scanSchemaConfigRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

func (r *SchemaConfigRepository) GetAll(ctx context.Context) ([]*DatabaseSchemaConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s`, schemaConfigSelectColumns, r.getTableName())
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []*DatabaseSchemaConfig
	for rows.Next() {
		config, err := scanSchemaConfigRow(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, &config)
	}
	return configs, nil
}

func (r *SchemaConfigRepository) Upsert(ctx context.Context, config *DatabaseSchemaConfig) error {
	if config == nil {
		return fmt.Errorf("config must not be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("schema config name is required")
	}
	existing, err := r.GetByName(ctx, config.Name)
	if err != nil {
		return err
	}
	if existing == nil {
		return r.Create(ctx, config)
	}
	config.ID = existing.ID
	return r.Update(ctx, config)
}

func (r *SchemaConfigRepository) Delete(ctx context.Context, id int64) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, r.getTableName())
	_, err := r.DB.ExecContext(ctx, query, id)
	return err
}

func (r *SchemaConfigRepository) GetAllEnabled(ctx context.Context) ([]*DatabaseSchemaConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE enabled = true`, schemaConfigSelectColumns, r.getTableName())
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []*DatabaseSchemaConfig
	for rows.Next() {
		config, err := scanSchemaConfigRow(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, &config)
	}
	return configs, nil
}

// --- End Interface Methods ---

// Non-interface methods (helpers, internal logic)

func (r *SchemaConfigRepository) Create(ctx context.Context, config *DatabaseSchemaConfig) error {
	metadataJSON, err := json.Marshal(config.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	query := fmt.Sprintf(`INSERT INTO %s (name, schema, description, priority, metadata) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at`, r.getTableName())
	err = r.DB.QueryRowContext(ctx, query, config.Name, config.DatabaseSchema, config.Description, config.Priority, string(metadataJSON)).Scan(&config.ID, &config.CreatedAt, &config.UpdatedAt)
	return err
}

func (r *SchemaConfigRepository) Update(ctx context.Context, config *DatabaseSchemaConfig) error {
	metadataJSON, err := json.Marshal(config.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	query := fmt.Sprintf(`UPDATE %s SET schema = $1, description = $2, priority = $3, metadata = $4, updated_at = NOW() WHERE id = $5`, r.getTableName())
	_, err = r.DB.ExecContext(ctx, query, config.DatabaseSchema, config.Description, config.Priority, string(metadataJSON), config.ID)
	return err
}

func (r *SchemaConfigRepository) EnsureTableAndDefault(ctx context.Context, otel_schema string) error {
	// Check if default schema config exists
	checkQuery := fmt.Sprintf(`SELECT 1 FROM %s WHERE id = 1`, r.getTableName())
	row := r.DB.QueryRowContext(ctx, checkQuery)
	var exists int
	err := row.Scan(&exists)
	if err == nil {
		return nil // default exists
	}
	if isTableNotExistError(err) {
		// Create schema config table
		if err := r.executeSQL(ctx, "ddl/config_otel_database.sql"); err != nil {
			return fmt.Errorf("failed to execute schema SQL: %w", err)
		}
		// Try again for default row
		row = r.DB.QueryRowContext(ctx, checkQuery)
		err = row.Scan(&exists)
	}
	if err == sql.ErrNoRows {
		// Insert default schema config
		defaultConfig := &DatabaseSchemaConfig{
			ID:             1,
			Name:           otel_schema,
			DatabaseSchema: otel_schema,
			Description:    "",
			Metadata:       map[string]interface{}{},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		if err := r.Create(ctx, defaultConfig); err != nil {
			return fmt.Errorf("insert default schema config: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("check default schema config row: %w", err)
	}
	return nil
}

func (r *SchemaConfigRepository) EnsureTable(ctx context.Context) error {
	if err := EnsureSchemaExists(ctx, r.DB, r.schema); err != nil {
		return err
	}

	tableExists, err := r.TableExists(ctx, r.schema, SchemaConfigTableName)
	if err != nil {
		return err
	}
	if !tableExists {
		if err := r.executeSQL(ctx, "ddl/config_otel_database.sql"); err != nil {
			return fmt.Errorf("failed to execute schema SQL: %w", err)
		}
		r.Logger.Error("Schema configuration tables ensured", err)
	}

	tableExists, err = r.TableExists(ctx, r.schema, ProcessorTableName)
	if err != nil {
		return err
	}
	if !tableExists {
		if err := r.executeSQL(ctx, "ddl/config_otel_processor.sql"); err != nil {
			return fmt.Errorf("failed to execute schema SQL: %w", err)
		}
		r.Logger.Error("Repository configuration tables ensured", err)
	}

	return nil
}

func (r *SchemaConfigRepository) executeSQL(ctx context.Context, filename string) error {
	content, err := sqlFiles.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read embedded SQL file %s: %w", filename, err)
	}

	// Replace template variables
	sqlContent := string(content)
	sqlContent = strings.ReplaceAll(sqlContent, "{{.Schema}}", r.schema)

	if _, err := r.DB.ExecContext(ctx, sqlContent); err != nil {
		return fmt.Errorf("failed to execute SQL from %s: %w", filename, err)
	}
	return nil
}

func (r *SchemaConfigRepository) TableExists(ctx context.Context, schema, tableName string) (bool, error) {
	return tableExists(ctx, r.DB, schema, tableName)
}

func (r *SchemaConfigRepository) getTableName() string {
	return fmt.Sprintf("%s.%s", r.schema, SchemaConfigTableName)
}

// EnsureOtelDataTables creates all necessary OTEL data tables in the specified schema
func (r *SchemaConfigRepository) EnsureOtelDataTables(ctx context.Context, db *sql.DB, schema string) error {
	// List of DDL files that need to be executed to create OTEL data tables
	ddlFiles := []string{
		"integer_clock.sql",
		"logs.sql",
		"spans.sql",
		"gauge_metrics.sql",
		"sum_metrics.sql",
		"histogram_metrics.sql",
		"summary_metrics.sql",
		"exponential_histogram_metrics.sql",
		"profiles.sql",
		"runs.sql",
		"signal_hashes.sql",
	}

	for _, filename := range ddlFiles {
		if filename == "profiles.sql" {
			if err := EnsureProfilesTable(ctx, db, schema); err != nil {
				return err
			}
			continue
		}
		content, err := sqlFiles.ReadFile("ddl/" + filename)
		if err != nil {
			return fmt.Errorf("failed to read DDL file %s: %w", filename, err)
		}

		// Replace template variables
		sqlContent := string(content)
		sqlContent = strings.ReplaceAll(sqlContent, "{{.Schema}}", schema)

		if _, err := db.ExecContext(ctx, sqlContent); err != nil {
			return fmt.Errorf("failed to execute DDL from %s for schema %s: %w", filename, schema, err)
		}
	}

	return nil
}

func (r *SchemaConfigRepository) CreateSchema(ctx context.Context, schema string) error {

	createSchemaSQL := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)

	if _, err := r.DB.ExecContext(ctx, createSchemaSQL); err != nil {
		return fmt.Errorf("failed to create schema '%s': %w", schema, err)
	}
	if err := r.EnsureOtelDataTables(ctx, r.DB, schema); err != nil {
		return fmt.Errorf("failed to ensure OTEL tables for schema %s: %w", schema, err)
	}

	return nil
}

func (r *SchemaConfigRepository) DeleteSchema(ctx context.Context, schema string) error {
	// First check if schema exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`
	if err := r.DB.QueryRowContext(ctx, checkQuery, schema).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check if schema exists: %w", err)
	}

	if !exists {
		return fmt.Errorf("schema '%s' does not exist", schema)
	}

	// Drop the schema (CASCADE will drop all objects in the schema)
	dropQuery := fmt.Sprintf("DROP SCHEMA %s CASCADE", schema)
	if _, err := r.DB.ExecContext(ctx, dropQuery); err != nil {
		return fmt.Errorf("failed to drop schema '%s': %w", schema, err)
	}

	if r.Logger != nil {
		r.Logger.Info("Successfully deleted schema", "schema", schema)
	}

	return nil
}

func (r *SchemaConfigRepository) GetSchemaStats(ctx context.Context) (*SchemaStats, error) {
	// Get all enabled schemas to calculate stats for
	enabledSchemas, err := r.GetAllEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled schemas: %w", err)
	}

	if len(enabledSchemas) == 0 {
		return &SchemaStats{
			SchemaName:  "none",
			TableCount:  0,
			TotalRows:   0,
			TotalSizeMB: 0,
			LastUpdated: time.Now().Format(time.RFC3339),
			IsEnabled:   false,
		}, nil
	}

	// For simplicity, return stats for the first enabled schema
	// In a real implementation, you might want to aggregate or allow schema selection
	firstSchema := enabledSchemas[0]

	// Query table count for the schema
	var tableCount int64
	tableCountQuery := `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = $1`
	if err := r.DB.QueryRowContext(ctx, tableCountQuery, firstSchema.DatabaseSchema).Scan(&tableCount); err != nil {
		return nil, fmt.Errorf("failed to get table count: %w", err)
	}

	// Query total rows across all tables in the schema
	var totalRows int64
	rowCountQuery := `
		SELECT COALESCE(SUM(n_tup_ins + n_tup_upd), 0) 
		FROM pg_stat_user_tables 
		WHERE schemaname = $1`
	if err := r.DB.QueryRowContext(ctx, rowCountQuery, firstSchema.DatabaseSchema).Scan(&totalRows); err != nil {
		// If stats are not available, default to 0
		totalRows = 0
	}

	// Query total size in MB
	var totalSizeMB int64
	sizeQuery := `
		SELECT COALESCE(SUM(pg_total_relation_size(c.oid)) / 1024 / 1024, 0)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = $1 AND c.relkind = 'r'`
	if err := r.DB.QueryRowContext(ctx, sizeQuery, firstSchema.DatabaseSchema).Scan(&totalSizeMB); err != nil {
		// If size calculation fails, default to 0
		totalSizeMB = 0
	}

	return &SchemaStats{
		SchemaName:  firstSchema.DatabaseSchema,
		TableCount:  tableCount,
		TotalRows:   totalRows,
		TotalSizeMB: totalSizeMB,
		LastUpdated: time.Now().Format(time.RFC3339),
		IsEnabled:   firstSchema.Enabled,
	}, nil
}

// CleanNamespace returns a safe, consistent table name for a service's vector store.
func CleanNamespace(namespace string) string {
	// Convert to lowercase and replace invalid characters with underscores
	safe := strings.ToLower(namespace)
	safe = strings.ReplaceAll(safe, "-", "_")
	safe = strings.ReplaceAll(safe, ".", "_")
	safe = strings.ReplaceAll(safe, " ", "_")

	// Truncate if too long
	if len(safe) > 50 {
		safe = safe[:50]
	}
	return safe
}

func scanSchemaConfigRow(scanner interface{ Scan(dest ...any) error }) (DatabaseSchemaConfig, error) {
	var config DatabaseSchemaConfig
	var routingJSON, metadataJSON string
	if err := scanner.Scan(
		&config.ID,
		&config.Name,
		&config.DatabaseSchema,
		&config.Enabled,
		&config.Description,
		&routingJSON,
		&config.Priority,
		&metadataJSON,
		&config.CreatedAt,
		&config.UpdatedAt,
	); err != nil {
		return config, err
	}

	// Use utility functions for JSON unmarshaling
	var err error
	config.Routing, err = unmarshalJSONField(routingJSON, "routing")
	if err != nil {
		return config, err
	}

	config.Metadata, err = unmarshalJSONField(metadataJSON, "metadata")
	if err != nil {
		return config, err
	}

	// Clean the namespace of DatabaseSchema
	config.DatabaseSchema = CleanNamespace(config.DatabaseSchema)
	return config, nil
}

// EnsureProfilesTable applies fresh DDL and the additive profile storage migration.
// It is safe to repeat: existing payloads and legacy summary columns are untouched.
func EnsureProfilesTable(ctx context.Context, db *sql.DB, schema string) error {
	quoted, err := ValidateAndQuoteSchema(schema)
	if err != nil {
		return err
	}
	ddl, err := sqlFiles.ReadFile("ddl/profiles.sql")
	if err != nil {
		return err
	}
	migration, err := sqlFiles.ReadFile("ddl/migrations/profiles_storage_v1.sql")
	if err != nil {
		return err
	}
	sqlContent := strings.ReplaceAll(string(ddl), "{{.ProfileStorageMigration}}", string(migration))
	sqlContent = strings.ReplaceAll(sqlContent, "{{.Schema}}", quoted)
	sqlContent = strings.ReplaceAll(sqlContent, "{{.SchemaName}}", schema)
	if _, err := db.ExecContext(ctx, sqlContent); err != nil {
		return fmt.Errorf("ensure profiles table in %s: %w", schema, err)
	}
	return nil
}
