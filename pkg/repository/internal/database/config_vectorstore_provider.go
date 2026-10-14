// Package database provides configuration management for vector store backends.
//
// This file defines the VectorStoreConfig struct and CRUD operations for the config_vectorstore_provider table.
//
// Key features:
//   - All important fields (including connection pool settings) are first-class columns.
//   - Pool settings (max_open_conns, max_idle_conns, conn_max_lifetime) are settable via API, not just metadata.
//   - Secrets (e.g., passwords, API keys) should be referenced in the metadata.secrets map using secret references (env:, file:, dev:, etc.).
//   - All CRUD operations (Create, Read, Update, Delete) are supported via the VectorStoreConfigManager interface.
//   - The struct is fully serializable/deserializable to/from JSON for use in REST/gRPC APIs.
//
// Example usage:
//
//	config := VectorStoreConfig{...}
//	err := manager.Upsert(ctx, db, config)
//	config, err := manager.GetByName(ctx, db, "my_pgvector_store")
//
// See README.md for environment variable and metadata conventions.
package database

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/util"
)

//go:embed ddl/config_vectorstore_provider.sql
var vectorStoreConfigSQL string

// Constants for vector store configuration
const (
	// Table and indexes
	VectorStoreConfigTableName = "config_vectorstore_provider"

	// Default configuration values
	DefaultVectorStoreConfigName           = "pgvector"
	DefaultVectorStoreProvider             = "postgres"
	DefaultVectorStoreBatchSize            = 100
	DefaultVectorStoreFlushIntervalMs      = 200 // 200ms
	DefaultVectorStoreMaxRetries           = 3
	DefaultVectorStoreMaxOpenConns         = 25
	DefaultVectorStoreMaxIdleConns         = 5
	DefaultVectorStoreConnMaxLifetime      = 3600000 // 1 hour in milliseconds
	DefaultVectorStoreBatchErrorPolicy     = "retry"
	DefaultVectorStoreMaxConcurrentBatches = 8
	DefaultVectorStoreRetryBackoffBaseMs   = 1000  // 1s
	DefaultVectorStoreRetryBackoffMaxMs    = 30000 // 30s
	DefaultVectorStoreConfigDescription    = "Default vector store configuration"
)

// DRY: Centralized list of columns for vector store config SELECTs
const vectorStoreConfigSelectColumns = `id, name, provider, endpoint, "user", database, embedding_id, batch_size, flush_interval_ms, max_retries, max_open_conns, max_idle_conns, conn_max_lifetime_ms, batch_error_policy, max_concurrent_batches, vendor_specific_options, metadata, description, created_at, updated_at`

// scanVectorStoreConfigRow scans a row into a VectorStoreConfig struct
func scanVectorStoreConfigRow(scanner interface{ Scan(dest ...any) error }) (VectorStoreConfig, error) {
	var config VectorStoreConfig
	var flushIntervalMs, connMaxLifetimeMs int
	var vendorOptionsJSON, metadataJSON string
	if err := scanner.Scan(
		&config.ID, &config.Name, &config.Provider, &config.Endpoint, &config.User,
		&config.Database, &config.EmbeddingID, &config.BatchSize, &flushIntervalMs,
		&config.MaxRetries, &config.MaxOpenConns, &config.MaxIdleConns,
		&connMaxLifetimeMs, &config.BatchErrorPolicy, &config.MaxConcurrentBatches,
		&vendorOptionsJSON, &metadataJSON, &config.Description, &config.CreatedAt, &config.UpdatedAt,
	); err != nil {
		return config, err
	}
	config.FlushInterval = time.Duration(flushIntervalMs) * time.Millisecond
	config.ConnMaxLifetime = time.Duration(connMaxLifetimeMs) * time.Millisecond

	// Use utility functions for JSON unmarshaling
	var err error
	config.VendorSpecificOptions, err = unmarshalJSONField(vendorOptionsJSON, "vendor_specific_options")
	if err != nil {
		return config, err
	}

	config.Metadata, err = unmarshalJSONField(metadataJSON, "metadata")
	if err != nil {
		return config, err
	}

	return config, nil
}

// GetDefaultVectorVendorOptions returns the default vendor-specific options map
func GetDefaultVectorVendorOptions() map[string]interface{} {
	return map[string]interface{}{}
}

// VectorStoreConfig represents a complete vector store configuration
type VectorStoreConfig struct {
	ID                    int64                  `json:"id"`
	Name                  string                 `json:"name"`
	Provider              string                 `json:"provider"`
	Endpoint              string                 `json:"endpoint"`
	User                  string                 `json:"user"`
	Database              string                 `json:"database"`
	EmbeddingID           int64                  `json:"embedding_id"`
	BatchSize             int                    `json:"batch_size"`
	FlushInterval         time.Duration          `json:"flush_interval"`
	MaxRetries            int                    `json:"max_retries"`
	MaxOpenConns          int                    `json:"max_open_conns"`
	MaxIdleConns          int                    `json:"max_idle_conns"`
	ConnMaxLifetime       time.Duration          `json:"conn_max_lifetime"`
	BatchErrorPolicy      string                 `json:"batch_error_policy"`
	MaxConcurrentBatches  int                    `json:"max_concurrent_batches"`
	VendorSpecificOptions map[string]interface{} `json:"vendor_specific_options"`
	Metadata              map[string]interface{} `json:"metadata"`
	Description           string                 `json:"description"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

// DefaultVectorStoreConfigManager implements VectorStoreConfigManager
type DefaultVectorStoreConfigManager struct {
	logger util.Logger
	db     *sql.DB
	schema string
}

// Helper functions to eliminate code duplication

// getTableName returns the fully qualified table name
func (m *DefaultVectorStoreConfigManager) getTableName() string {
	return fmt.Sprintf("%s.%s", m.schema, VectorStoreConfigTableName)
}

// getSchemaName returns the current schema name
func (m *DefaultVectorStoreConfigManager) getSchemaName() string {
	return m.schema
}

// executeQuerySingle executes a single-row query and returns the config
func (m *DefaultVectorStoreConfigManager) executeQuerySingle(ctx context.Context, query string, args ...interface{}) (*VectorStoreConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
	defer cancel()

	row := m.db.QueryRowContext(ctx, query, args...)
	config, err := scanVectorStoreConfigRow(row)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// executeQueryMultiple executes a multi-row query and returns configs
func (m *DefaultVectorStoreConfigManager) executeQueryMultiple(ctx context.Context, query string, args ...interface{}) ([]VectorStoreConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
	defer cancel()

	rows, err := m.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var configs []VectorStoreConfig
	for rows.Next() {
		config, err := scanVectorStoreConfigRow(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, config)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return configs, nil
}

// JSON marshaling helpers to eliminate duplication

// marshalConfigFields marshals vendor options and metadata for database storage
func (m *DefaultVectorStoreConfigManager) marshalConfigFields(config VectorStoreConfig) (vendorOptionsJSON, metadataJSON []byte, err error) {
	vendorOptionsJSON, err = json.Marshal(config.VendorSpecificOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal vendor options: %w", err)
	}
	metadataJSON, err = json.Marshal(config.Metadata)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal metadata: %w", err)
	}
	return vendorOptionsJSON, metadataJSON, nil
}

// NewVectorStoreConfigManager creates a new vector store config manager
func NewVectorStoreConfigManager(ctx context.Context, db *sql.DB, logger util.Logger) (VectorStoreConfigInterface, error) {
	m := &DefaultVectorStoreConfigManager{
		logger: logger,
		db:     db,
		schema: DefaultConfigSchema,
	}

	// Ensure table exists
	if err := m.EnsureTable(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure vector store config table: %w", err)
	}

	return m, nil
}

// CreateDefaultVectorStoreConfig creates a default vector store configuration
func CreateDefaultVectorStoreConfig() VectorStoreConfig {
	return VectorStoreConfig{
		Name:                  DefaultVectorStoreConfigName,
		Provider:              DefaultVectorStoreProvider,
		Endpoint:              "", // Use default Postgress database for vector store
		User:                  "", // Use default Postgress database for vector store
		Database:              "", // Use default Postgress database for vector store
		EmbeddingID:           1,
		BatchSize:             DefaultVectorStoreBatchSize,
		FlushInterval:         DefaultVectorStoreFlushIntervalMs * time.Millisecond,
		MaxRetries:            DefaultVectorStoreMaxRetries,
		MaxOpenConns:          DefaultVectorStoreMaxOpenConns,
		MaxIdleConns:          DefaultVectorStoreMaxIdleConns,
		ConnMaxLifetime:       time.Duration(DefaultVectorStoreConnMaxLifetime) * time.Millisecond,
		BatchErrorPolicy:      DefaultVectorStoreBatchErrorPolicy,
		MaxConcurrentBatches:  DefaultVectorStoreMaxConcurrentBatches,
		VendorSpecificOptions: GetDefaultVectorVendorOptions(),
		Metadata: map[string]interface{}{
			"retry_backoff_base_ms": DefaultVectorStoreRetryBackoffBaseMs,
			"retry_backoff_max_ms":  DefaultVectorStoreRetryBackoffMaxMs,
		},
		Description: DefaultVectorStoreConfigDescription,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// EnsureTable creates the vector store config table if it doesn't exist
func (m *DefaultVectorStoreConfigManager) EnsureTable(ctx context.Context) error {
	return m.EnsureTableAndDefault(ctx)
}

// EnsureTableAndDefault creates the vector store config table and ensures default values exist
func (m *DefaultVectorStoreConfigManager) EnsureTableAndDefault(ctx context.Context) error {
	schemaName := m.getSchemaName()

	// Check if the main config_vectorstore_provider table exists
	tableExists, err := m.TableExists(ctx, m.db)
	if err != nil {
		return err
	}

	if !tableExists {
		// Replace {{.Schema}} placeholders with schema name
		query := strings.ReplaceAll(vectorStoreConfigSQL, "{{.Schema}}", schemaName)

		ctx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
		defer cancel()

		if _, err := m.db.ExecContext(ctx, query); err != nil {
			if m.logger != nil {
				m.logger.Error("create vector store config table", err,
					"table", VectorStoreConfigTableName,
					"schema", schemaName)
			}
			return fmt.Errorf("failed to create vector store config table: %w", err)
		}

		if m.logger != nil {
			m.logger.Debug("Vector store config table ensured",
				"table", VectorStoreConfigTableName,
				"schema", schemaName)
		}
	}

	// Ensure default row exists
	if _, err := m.GetByID(ctx, 1); err == sql.ErrNoRows {
		if m.logger != nil {
			m.logger.Info("Inserting default vector store config row")
		}
		defaultCfg := CreateDefaultVectorStoreConfig()
		return m.Upsert(ctx, &defaultCfg)
	}
	return nil
}

// InitializeDefault inserts default vector store configuration into the database
func (m *DefaultVectorStoreConfigManager) InitializeDefault(ctx context.Context) error {
	defaultConfig := CreateDefaultVectorStoreConfig()

	if m.logger != nil {
		m.logger.Info("Initializing default vector store configuration",
			"name", defaultConfig.Name)
	}

	return m.Upsert(ctx, &defaultConfig)
}

// Upsert inserts or updates a vector store configuration
func (m *DefaultVectorStoreConfigManager) Upsert(ctx context.Context, config *VectorStoreConfig) error {
	tableName := m.getTableName()
	vendorOptionsJSON, metadataJSON, err := m.marshalConfigFields(*config)
	if err != nil {
		return fmt.Errorf("failed to marshal config fields: %w", err)
	}
	query := fmt.Sprintf(`
	INSERT INTO %s (
		name, provider, endpoint, "user", database, embedding_id, batch_size, 
		flush_interval_ms, max_retries, max_open_conns, 
		max_idle_conns, conn_max_lifetime_ms, batch_error_policy, 
		max_concurrent_batches, vendor_specific_options, metadata, description, created_at, updated_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
	ON CONFLICT (name) 
	DO UPDATE SET 
		provider = EXCLUDED.provider,
		endpoint = EXCLUDED.endpoint,
		"user" = EXCLUDED."user",
		database = EXCLUDED.database,
		embedding_id = EXCLUDED.embedding_id,
		batch_size = EXCLUDED.batch_size,
		flush_interval_ms = EXCLUDED.flush_interval_ms,
		max_retries = EXCLUDED.max_retries,
		max_open_conns = EXCLUDED.max_open_conns,
		max_idle_conns = EXCLUDED.max_idle_conns,
		conn_max_lifetime_ms = EXCLUDED.conn_max_lifetime_ms,
		batch_error_policy = EXCLUDED.batch_error_policy,
		max_concurrent_batches = EXCLUDED.max_concurrent_batches,
		vendor_specific_options = EXCLUDED.vendor_specific_options,
		metadata = EXCLUDED.metadata,
		description = EXCLUDED.description,
		updated_at = NOW()
	RETURNING id, created_at, updated_at
	`, tableName)
	if err := m.db.QueryRowContext(ctx, query, config.Name, config.Provider, config.Endpoint, config.User,
		config.Database, config.EmbeddingID, config.BatchSize, int(config.FlushInterval.Milliseconds()),
		config.MaxRetries, config.MaxOpenConns, config.MaxIdleConns,
		int(config.ConnMaxLifetime.Milliseconds()), config.BatchErrorPolicy, config.MaxConcurrentBatches,
		string(vendorOptionsJSON), string(metadataJSON),
		config.Description, config.CreatedAt, config.UpdatedAt).Scan(&config.ID, &config.CreatedAt, &config.UpdatedAt); err != nil {
		if m.logger != nil {
			m.logger.Error("upsert vector store config", "name", config.Name, err)
		}
		return fmt.Errorf("failed to upsert vector store config: %w", err)
	}
	if m.logger != nil {
		m.logger.Debug("Vector store config upserted successfully", "name", config.Name)
	}
	return nil
}

func (m *DefaultVectorStoreConfigManager) GetByName(ctx context.Context, name string) (*VectorStoreConfig, error) {
	tableName := m.getTableName()
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE name = $1`, vectorStoreConfigSelectColumns, tableName)
	config, err := m.executeQuerySingle(ctx, query, name)
	if err != nil {
		if err == sql.ErrNoRows {
			if m.logger != nil {
				m.logger.Debug("Vector store config not found", "name", name)
			}
		} else {
			if m.logger != nil {
				m.logger.Error("get vector store config by name", "name", name, err)
			}
		}
		return nil, err
	}
	if m.logger != nil {
		m.logger.Debug("Vector store config retrieved by name", "name", name)
	}
	return config, nil
}

func (m *DefaultVectorStoreConfigManager) GetAll(ctx context.Context) ([]*VectorStoreConfig, error) {
	tableName := m.getTableName()
	query := fmt.Sprintf(`SELECT %s FROM %s`, vectorStoreConfigSelectColumns, tableName)
	configs, err := m.executeQueryMultiple(ctx, query)
	if err != nil {
		if m.logger != nil {
			m.logger.Error("get all vector store configs", err)
		}
		return nil, err
	}
	if m.logger != nil {
		m.logger.Debug("Retrieved all vector store configs", "count", len(configs))
	}
	// Convert to []*VectorStoreConfig
	result := make([]*VectorStoreConfig, 0, len(configs))
	for i := range configs {
		result = append(result, &configs[i])
	}
	return result, nil
}

func (m *DefaultVectorStoreConfigManager) Delete(ctx context.Context, id int64) error {
	tableName := m.getTableName()
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, tableName)
	ctx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
	defer cancel()
	if _, err := m.db.ExecContext(ctx, query, id); err != nil {
		if m.logger != nil {
			m.logger.Error("delete vector store config", "id", id, err)
		}
		return fmt.Errorf("failed to delete vector store config: %w", err)
	}
	if m.logger != nil {
		m.logger.Info("Vector store config deleted", "id", id)
	}
	return nil
}

// SetDefault sets a vector store configuration as the default (no-op, uses name = 'default')
func (m *DefaultVectorStoreConfigManager) SetDefault(ctx context.Context, db *sql.DB, name string) error {
	if m.logger != nil {
		m.logger.Info("SetDefault called, but default is determined by name = 'default'. No action taken.", "name", name)
	}
	return nil
}

// TableExists checks if the vector store config table exists
func (m *DefaultVectorStoreConfigManager) TableExists(ctx context.Context, db *sql.DB) (bool, error) {
	schemaName := m.getSchemaName()
	query := `
	SELECT EXISTS (
		SELECT FROM information_schema.tables 
		WHERE table_schema = $1 AND table_name = $2
	)`

	ctx, cancel := context.WithTimeout(ctx, DatabaseTimeout)
	defer cancel()

	var exists bool
	err := db.QueryRowContext(ctx, query, schemaName, VectorStoreConfigTableName).Scan(&exists)
	if err != nil {
		if m.logger != nil {
			m.logger.Error("check if vector store config table exists", "table", VectorStoreConfigTableName, err)
		}
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}

	return exists, nil
}

func (m *DefaultVectorStoreConfigManager) GetByID(ctx context.Context, id int64) (*VectorStoreConfig, error) {
	tableName := m.getTableName()
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, vectorStoreConfigSelectColumns, tableName)
	config, err := m.executeQuerySingle(ctx, query, id)
	if err != nil {
		if err == sql.ErrNoRows {
			if m.logger != nil {
				m.logger.Debug("Vector store config not found by ID", "id", id)
			}
		} else {
			if m.logger != nil {
				m.logger.Error("get vector store config by ID", "id", id, err)
			}
		}
		return nil, err
	}
	if m.logger != nil {
		m.logger.Debug("Vector store config retrieved by ID", "id", id)
	}
	return config, nil
}
