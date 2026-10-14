package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

const (
	ProcessorTableName = "config_otel_processor"

	processorSelectColumns = "id, name, database_id, vectorstore_id, template_id, processing_mode, enabled, routing, filter, metadata, created_at, updated_at"
)

type ProcessorMappingConfig struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	DatabaseID    int64  `json:"database_id"`
	VectorStoreID int64  `json:"vectorstore_id"`
	TemplateID    int64  `json:"template_id"`

	ProcessingMode  string                 `json:"processing_mode"`
	Enabled         bool                   `json:"enabled"`
	Routing         map[string]interface{} `json:"routing"`
	EmbeddingFilter map[string]interface{} `json:"filter"`
	Metadata        map[string]interface{} `json:"metadata"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
}

// ProcessorConfigRepository handles processor mapping configurations
type ProcessorConfigRepository struct {
	DB     *sql.DB
	schema string
	Logger util.Logger
}

// NewProcessorConfigRepo creates a new repository config repository that implements RepositoryConfigInterface
func NewProcessorConfigRepo(ctx context.Context, db *sql.DB, logger util.Logger, schema_otel string) (*ProcessorConfigRepository, error) {
	repo := &ProcessorConfigRepository{
		DB:     db,
		schema: DefaultConfigSchema,
		Logger: logger,
	}

	// Ensure tables and defaults are set
	if err := repo.EnsureTableAndDefault(ctx, schema_otel); err != nil {
		return nil, fmt.Errorf("failed to ensure repository config tables and defaults: %w", err)
	}

	return repo, nil
}

// --- ProcessorConfigRepository Interface Methods (CRUD) ---

func (r *ProcessorConfigRepository) GetByID(ctx context.Context, id int64) (*ProcessorMappingConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, processorSelectColumns, r.getTableName())
	row := r.DB.QueryRowContext(ctx, query, id)
	mapping, err := scanProcessorMappingRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &mapping, nil
}

func (r *ProcessorConfigRepository) GetByName(ctx context.Context, name string) (*ProcessorMappingConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE name = $1`, processorSelectColumns, r.getTableName())
	row := r.DB.QueryRowContext(ctx, query, name)
	mapping, err := scanProcessorMappingRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &mapping, nil
}

func (r *ProcessorConfigRepository) GetAll(ctx context.Context) ([]*ProcessorMappingConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s`, processorSelectColumns, r.getTableName())
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mappings []*ProcessorMappingConfig
	for rows.Next() {
		mapping, err := scanProcessorMappingRow(rows)
		if err != nil {
			return nil, err
		}
		mappings = append(mappings, &mapping)
	}
	return mappings, nil
}

func (r *ProcessorConfigRepository) Upsert(ctx context.Context, mapping *ProcessorMappingConfig) error {
	if mapping == nil {
		return fmt.Errorf("mapping must not be nil")
	}
	if mapping.Name == "" {
		return fmt.Errorf("mapping name is required")
	}

	query := fmt.Sprintf(`INSERT INTO %s (name, database_id, vectorstore_id, template_id, processing_mode, enabled, filter, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (name) DO UPDATE SET 
		database_id = EXCLUDED.database_id,
		vectorstore_id = EXCLUDED.vectorstore_id,
		template_id = EXCLUDED.template_id,
		processing_mode = EXCLUDED.processing_mode,
		enabled = EXCLUDED.enabled, 
		filter = EXCLUDED.filter, 
		metadata = EXCLUDED.metadata, 
		updated_at = NOW()
		RETURNING id, created_at, updated_at`, r.getTableName())

	filterJSON, err := json.Marshal(mapping.EmbeddingFilter)
	if err != nil {
		return fmt.Errorf("failed to marshal filter config: %w", err)
	}
	metadataJSON, err := json.Marshal(mapping.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	err = r.DB.QueryRowContext(ctx, query,
		mapping.Name, mapping.DatabaseID, mapping.VectorStoreID, mapping.TemplateID, mapping.ProcessingMode, mapping.Enabled,
		string(filterJSON), string(metadataJSON),
		mapping.CreatedAt, mapping.UpdatedAt).Scan(&mapping.ID, &mapping.CreatedAt, &mapping.UpdatedAt)
	return err
}

func (r *ProcessorConfigRepository) Delete(ctx context.Context, id int64) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, r.getTableName())
	_, err := r.DB.ExecContext(ctx, query, id)
	return err
}

func (r *ProcessorConfigRepository) GetAllEnabled(ctx context.Context) ([]*ProcessorMappingConfig, error) {
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE enabled = true ORDER BY processing_mode ASC`, processorSelectColumns, r.getTableName())
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		if r.Logger != nil {
			r.Logger.Error("Failed to get all enabled repository mappings", err)
		}
		return nil, fmt.Errorf("get all enabled repository mappings: %w", err)
	}
	defer rows.Close()
	var mappings []*ProcessorMappingConfig
	for rows.Next() {
		mapping, err := scanProcessorMappingRow(rows)
		if err != nil {
			if r.Logger != nil {
				r.Logger.Error("Failed to scan repository mapping row", err)
			}
			return nil, fmt.Errorf("scan repository mapping: %w", err)
		}
		if r.Logger != nil {
			r.Logger.Debug("Read processor mapping from DB",
				"id", mapping.ID,
				"name", mapping.Name,
				"template_id", mapping.TemplateID,
				"processing_mode", mapping.ProcessingMode)
		}
		mappings = append(mappings, &mapping)
	}
	return mappings, nil
}

func (r *ProcessorConfigRepository) GetSchemaById(ctx context.Context, ID int64) (*DatabaseSchemaConfig, error) {
	// Create a schema config repository to fetch the schema
	schemaRepo := &SchemaConfigRepository{
		DB:     r.DB,
		schema: r.schema,
		Logger: r.Logger,
	}
	return schemaRepo.GetByID(ctx, ID)
}

func (r *ProcessorConfigRepository) EnsureTableAndDefault(ctx context.Context, schema_otel string) error {
	createSchemaSQL := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", r.schema)
	if _, err := r.DB.ExecContext(ctx, createSchemaSQL); err != nil {
		return fmt.Errorf("failed to create database schema %s: %w", r.schema, err)
	}

	// Check if default repository mapping exists
	checkQuery := fmt.Sprintf(`SELECT 1 FROM %s LIMIT 1`, r.getTableName())
	row := r.DB.QueryRowContext(ctx, checkQuery)
	var exists int
	err := row.Scan(&exists)
	if err == nil {
		return nil // default exists
	}
	if isTableNotExistError(err) {
		// Create repository config table
		if err := r.executeSQL(ctx, "ddl/config_otel_processor.sql"); err != nil {
			return fmt.Errorf("failed to execute repository SQL: %w", err)
		}
		// Try again for default row
		row = r.DB.QueryRowContext(ctx, checkQuery)
		err = row.Scan(&exists)
	}
	if err == sql.ErrNoRows {
		// Insert default repository mapping
		defaultMapping := &ProcessorMappingConfig{
			ID:              1,
			Name:            "default",
			DatabaseID:      1, // References default schema config
			VectorStoreID:   1, // References default vector store config
			TemplateID:      1, // References default template
			ProcessingMode:  "template",
			Enabled:         false,
			EmbeddingFilter: map[string]interface{}{},
			Metadata: map[string]interface{}{
				"embedding_batch_group_signals": false, // Default to single signal processing
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := r.Upsert(ctx, defaultMapping); err != nil {
			return fmt.Errorf("insert default repository mapping: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("check default repository mapping row: %w", err)
	}
	return nil
}

// --- Helper Methods ---

func (r *ProcessorConfigRepository) executeSQL(ctx context.Context, filename string) error {
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

func (r *ProcessorConfigRepository) TableExists(ctx context.Context, schema, tableName string) (bool, error) {
	return tableExists(ctx, r.DB, schema, tableName)
}

func (r *ProcessorConfigRepository) getTableName() string {
	return fmt.Sprintf("%s.%s", r.schema, ProcessorTableName)
}

func scanProcessorMappingRow(scanner interface{ Scan(dest ...any) error }) (ProcessorMappingConfig, error) {
	var mapping ProcessorMappingConfig
	var routingJSON, filterJSON, metadataJSON string
	if err := scanner.Scan(
		&mapping.ID,
		&mapping.Name,
		&mapping.DatabaseID,
		&mapping.VectorStoreID,
		&mapping.TemplateID,
		&mapping.ProcessingMode,
		&mapping.Enabled,
		&routingJSON,
		&filterJSON,
		&metadataJSON,
		&mapping.CreatedAt,
		&mapping.UpdatedAt,
	); err != nil {
		return mapping, err
	}
	// Use utility functions for JSON unmarshaling
	var err error
	mapping.Routing, err = unmarshalJSONField(routingJSON, "routing")
	if err != nil {
		return mapping, err
	}
	mapping.EmbeddingFilter, err = unmarshalJSONField(filterJSON, "filter")
	if err != nil {
		return mapping, err
	}

	mapping.Metadata, err = unmarshalJSONField(metadataJSON, "metadata")
	if err != nil {
		return mapping, err
	}

	return mapping, nil
}
