// Package database provides configuration management for embedding providers, templates, and filters.
// It includes repository logic for CRUD operations and table initialization for embedding-related tables.
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

//go:embed ddl/config_embedding_provider.sql
var embeddingConfigSQL string

// Constants for Embedding configuration
const (
	// Table and indexes
	EmbeddingConfigTableName = "config_embedding_provider"
)

// Column list for SELECT queries
const embeddingConfigSelectColumns = "id, name, provider, endpoint, model, timeout_ms, rate_limit, headers, normalize, dimension, max_context_length, description, metadata, created_at, updated_at"

// scanEmbeddingConfigRow scans a row into an EmbeddingConfig struct
func scanEmbeddingConfigRow(scanner interface{ Scan(dest ...any) error }) (EmbeddingConfig, error) {
	var config EmbeddingConfig
	var timeoutMs int
	var headersJSON, metadataJSON string
	var description sql.NullString
	if err := scanner.Scan(
		&config.ID, &config.Name, &config.Provider, &config.Endpoint, &config.Model,
		&timeoutMs, &config.RateLimit, &headersJSON, &config.Normalize,
		&config.Dimension, &config.MaxContextLength,
		&description, &metadataJSON, &config.CreatedAt, &config.UpdatedAt,
	); err != nil {
		return config, err
	}
	config.Description = ""
	if description.Valid {
		config.Description = description.String
	}
	config.Timeout = time.Duration(timeoutMs) * time.Millisecond

	// Use utility functions for JSON unmarshaling
	var err error
	config.Headers, err = unmarshalJSONFieldGeneric[map[string]string](headersJSON, "headers")
	if err != nil {
		return config, err
	}

	config.Metadata, err = unmarshalJSONField(metadataJSON, "metadata")
	if err != nil {
		return config, err
	}

	return config, nil
}

// EmbeddingConfig represents a complete LLM/embedding configuration
// Template and filter references are now managed in the vector store mapping
type EmbeddingConfig struct {
	ID               int64                  `json:"id"`
	Name             string                 `json:"name"`
	Provider         string                 `json:"provider"`
	Endpoint         string                 `json:"endpoint"`
	Model            string                 `json:"model"`
	Timeout          time.Duration          `json:"timeout"`
	RateLimit        int                    `json:"rate_limit"`
	Headers          map[string]string      `json:"headers"`
	Normalize        bool                   `json:"normalize"`
	Dimension        int                    `json:"dimension"`
	MaxContextLength int                    `json:"max_context_length"`
	Metadata         map[string]interface{} `json:"metadata"`
	Description      string                 `json:"description"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

// EmbeddingConfigRepository interface for Embedding configuration operations
type EmbeddingConfigRepository struct {
	DB     *sql.DB
	schema string
	Logger util.Logger
}

// Change the constructor to accept context.Context as the first argument
func NewEmbeddingConfigRepository(ctx context.Context, db *sql.DB, logger util.Logger) (*EmbeddingConfigRepository, error) {
	repo := &EmbeddingConfigRepository{DB: db, schema: DefaultConfigSchema, Logger: logger}
	if err := repo.EnsureTableAndDefault(ctx); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *EmbeddingConfigRepository) Create(ctx context.Context, config *EmbeddingConfig) error {
	tableName := r.getTableName()

	headersJSON, err := json.Marshal(config.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}
	metadataJSON, err := json.Marshal(config.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	query := fmt.Sprintf(`
	INSERT INTO %s (
		name, provider, endpoint, model, timeout_ms, rate_limit,
		headers, normalize, dimension, description, created_at, updated_at, metadata, max_context_length
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	RETURNING id, created_at, updated_at
	`, tableName)
	err = r.DB.QueryRowContext(ctx, query, config.Name, config.Provider, config.Endpoint, config.Model,
		int(config.Timeout.Milliseconds()), config.RateLimit,
		string(headersJSON), config.Normalize, config.Dimension, config.Description,
		config.CreatedAt, config.UpdatedAt, string(metadataJSON), config.MaxContextLength).Scan(&config.ID, &config.CreatedAt, &config.UpdatedAt)
	return err
}

func (r *EmbeddingConfigRepository) GetByID(ctx context.Context, id int64) (*EmbeddingConfig, error) {
	tableName := r.getTableName()
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1`, embeddingConfigSelectColumns, tableName)
	row := r.DB.QueryRowContext(ctx, query, id)
	config, err := scanEmbeddingConfigRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

func (r *EmbeddingConfigRepository) GetByName(ctx context.Context, name string) (*EmbeddingConfig, error) {
	tableName := r.getTableName()
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE name = $1`, embeddingConfigSelectColumns, tableName)
	row := r.DB.QueryRowContext(ctx, query, name)
	config, err := scanEmbeddingConfigRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

func (r *EmbeddingConfigRepository) GetAll(ctx context.Context) ([]*EmbeddingConfig, error) {
	tableName := r.getTableName()
	query := fmt.Sprintf(`SELECT %s FROM %s`, embeddingConfigSelectColumns, tableName)
	rows, err := r.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var configs []*EmbeddingConfig
	for rows.Next() {
		config, err := scanEmbeddingConfigRow(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, &config)
	}
	return configs, nil
}

func (r *EmbeddingConfigRepository) Update(ctx context.Context, config *EmbeddingConfig) error {
	tableName := r.getTableName()
	headersJSON, err := json.Marshal(config.Headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}
	metadataJSON, err := json.Marshal(config.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	query := fmt.Sprintf(`
	UPDATE %s SET
		provider = $1,
		endpoint = $2,
		model = $3,
		timeout_ms = $4,
		rate_limit = $5,
		headers = $6,
		normalize = $7,
		dimension = $8,
		description = $9,
		updated_at = NOW(),
		metadata = $10,
		max_context_length = $11
	WHERE id = $12
	`, tableName)
	_, err = r.DB.ExecContext(ctx, query, config.Provider, config.Endpoint, config.Model,
		int(config.Timeout.Milliseconds()), config.RateLimit,
		string(headersJSON), config.Normalize, config.Dimension, config.Description,
		string(metadataJSON), config.MaxContextLength, config.ID)
	return err
}

func (r *EmbeddingConfigRepository) Delete(ctx context.Context, id int64) error {
	tableName := r.getTableName()
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, tableName)
	_, err := r.DB.ExecContext(ctx, query, id)
	return err
}

func (r *EmbeddingConfigRepository) Upsert(ctx context.Context, config *EmbeddingConfig) error {
	if config == nil {
		return fmt.Errorf("config must not be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("embedding config name is required")
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

func (r *EmbeddingConfigRepository) EnsureTableAndDefault(ctx context.Context) error {
	// Check if table exists
	tableExists, err := r.tableExistsByName(ctx, EmbeddingConfigTableName)
	if err != nil {
		return err
	}

	if !tableExists {
		// Create table
		query := strings.ReplaceAll(embeddingConfigSQL, "{{.Schema}}", r.schema)
		if _, err := r.DB.ExecContext(ctx, query); err != nil {
			if r.Logger != nil {
				r.Logger.Error("Failed to create embedding config table", err)
			}
			return fmt.Errorf("failed to create embedding config table: %w", err)
		}
	}

	// Check if default row exists
	checkQuery := fmt.Sprintf(`SELECT 1 FROM %s WHERE id = 1`, r.getTableName())
	row := r.DB.QueryRowContext(ctx, checkQuery)
	var defaultExists int
	err = row.Scan(&defaultExists)
	if err == nil {
		return nil // default exists
	}

	if err == sql.ErrNoRows {
		// Insert each default embedding row individually using r.Create
		defaultEmbeddings := []EmbeddingConfig{
			{ID: 1, Name: "minilm", Provider: "sbert", Endpoint: "", Model: "sentence-transformers/all-MiniLM-L6-v2", Timeout: 30000 * time.Millisecond, RateLimit: 0, Headers: map[string]string{}, Normalize: false, Dimension: 384, MaxContextLength: 256, Description: "", Metadata: map[string]interface{}{}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: 2, Name: "mxbai", Provider: "sbert", Endpoint: "", Model: "mixedbread-ai/mxbai-embed-large-v1", Timeout: 30000 * time.Millisecond, RateLimit: 0, Headers: map[string]string{}, Normalize: false, Dimension: 1024, MaxContextLength: 512, Description: "", Metadata: map[string]interface{}{}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: 3, Name: "ollama", Provider: "ollama", Endpoint: "", Model: "nomic-embed-text:latest", Timeout: 30000 * time.Millisecond, RateLimit: 0, Headers: map[string]string{}, Normalize: false, Dimension: 768, MaxContextLength: 2048, Description: "", Metadata: map[string]interface{}{}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
			{ID: 4, Name: "text-embedding", Provider: "openai", Endpoint: "https://api.openai.com/v1", Model: "text-embedding-3-small", Timeout: 30000 * time.Millisecond, RateLimit: 0, Headers: map[string]string{}, Normalize: false, Dimension: 1536, MaxContextLength: 2048, Description: "", Metadata: map[string]interface{}{"secrets": map[string]interface{}{"embedding_api_key": "ENV:OPENAI_API_KEY"}}, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		}
		for _, row := range defaultEmbeddings {
			if err := r.Create(ctx, &row); err != nil {
				return fmt.Errorf("insert default embedding row (id=%d): %w", row.ID, err)
			}
		}
		return nil
	}

	return fmt.Errorf("check default embedding row: %w", err)
}

func (r *EmbeddingConfigRepository) getTableName() string {
	return fmt.Sprintf("%s.%s", r.schema, EmbeddingConfigTableName)
}

func (r *EmbeddingConfigRepository) tableExistsByName(ctx context.Context, tableName string) (bool, error) {
	schemaName := r.schema
	query := `
	SELECT EXISTS (
		SELECT FROM information_schema.tables 
		WHERE table_schema = $1 AND table_name = $2
	)`
	var exists bool
	err := r.DB.QueryRowContext(ctx, query, schemaName, tableName).Scan(&exists)
	if err != nil {
		if r.Logger != nil {
			r.Logger.Error("Failed to check table existence", err, "table", tableName)
		}
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}
	return exists, nil
}
