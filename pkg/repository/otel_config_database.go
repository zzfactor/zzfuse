// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
)

// OtelConfigDatabaseClient defines the interface for OTel config database operations
type OtelConfigDatabaseClient interface {
	// Schema Config operations
	GetSchemaConfigByID(ctx context.Context, id int64) (*database.DatabaseSchemaConfig, error)
	GetSchemaConfigByName(ctx context.Context, name string) (*database.DatabaseSchemaConfig, error)
	GetAllSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error)
	GetAllEnabledSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error)
	UpsertSchemaConfig(ctx context.Context, config *database.DatabaseSchemaConfig) error
	DeleteSchemaConfig(ctx context.Context, id int64) error
	GetSchemaConfigsForProcessor(ctx context.Context, processorID int64) ([]*database.DatabaseSchemaConfig, error)

	// Processor Config operations
	GetProcessorConfigByID(ctx context.Context, id int64) (*database.ProcessorMappingConfig, error)
	GetProcessorConfigByName(ctx context.Context, name string) (*database.ProcessorMappingConfig, error)
	GetProcessorConfigsBySchemaID(ctx context.Context, schemaID int64) ([]*database.ProcessorMappingConfig, error)
	GetAllProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error)
	GetAllEnabledProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error)
	UpsertProcessorConfig(ctx context.Context, config *database.ProcessorMappingConfig) error
	DeleteProcessorConfig(ctx context.Context, id int64) error
	GetSchemaByProcessorID(ctx context.Context, processorID int64) (*database.DatabaseSchemaConfig, error)

	// Embedding Config operations
	GetEmbeddingConfigByID(ctx context.Context, id int64) (*database.EmbeddingConfig, error)
	GetEmbeddingConfigByName(ctx context.Context, name string) (*database.EmbeddingConfig, error)
	GetAllEmbeddingConfigs(ctx context.Context) ([]*database.EmbeddingConfig, error)
	UpsertEmbeddingConfig(ctx context.Context, config *database.EmbeddingConfig) error
	DeleteEmbeddingConfig(ctx context.Context, id int64) error

	// Template Config operations
	GetTemplateConfigByID(ctx context.Context, id int64) (*database.EmbeddingConfigTemplate, error)
	GetTemplateConfigByName(ctx context.Context, name string) (*database.EmbeddingConfigTemplate, error)
	GetAllTemplateConfigs(ctx context.Context) ([]*database.EmbeddingConfigTemplate, error)
	UpsertTemplateConfig(ctx context.Context, config *database.EmbeddingConfigTemplate) error
	DeleteTemplateConfig(ctx context.Context, id int64) error

	// Vector Store Config operations
	GetVectorStoreConfigByID(ctx context.Context, id int64) (*database.VectorStoreConfig, error)
	GetVectorStoreConfigByName(ctx context.Context, name string) (*database.VectorStoreConfig, error)
	GetAllVectorStoreConfigs(ctx context.Context) ([]*database.VectorStoreConfig, error)
	UpsertVectorStoreConfig(ctx context.Context, config *database.VectorStoreConfig) error
	DeleteVectorStoreConfig(ctx context.Context, id int64) error

	// Utility operations
	GetConfigStatistics(ctx context.Context) (*database.ConfigStatistics, error)
	EnsureTablesAndDefaults(ctx context.Context) error

}

// OtelSQLConfigDatabaseClient implements the OtelConfigDatabaseClient interface
type OtelSQLConfigDatabaseClient struct {
	db     *sql.DB
	schema string
	logger util.Logger
	
	// Individual repository managers
	schemaRepo     database.SchemaConfigInterface
	processorRepo  *database.ProcessorConfigRepository
	embeddingRepo  *database.EmbeddingConfigRepository
	templateRepo   database.EmbeddingConfigTemplateInterface
	vectorStoreRepo database.VectorStoreConfigInterface
}

// NewOtelSQLConfigDatabaseClient creates a new unified config database client
func NewOtelSQLConfigDatabaseClient(ctx context.Context, db *sql.DB, logger util.Logger) (*OtelSQLConfigDatabaseClient, error) {
	client := &OtelSQLConfigDatabaseClient{
		db:     db,
		schema: DefaultSchema,
		logger: logger,
	}

	// Initialize individual repository managers
	if err := client.initializeRepositories(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize repositories: %w", err)
	}

	return client, nil
}

// initializeRepositories creates and initializes all repository managers
func (c *OtelSQLConfigDatabaseClient) initializeRepositories(ctx context.Context) error {
	// Initialize schema repository
	schemaRepo, err := database.NewDatabaseSchemaConfig(ctx, c.db, c.logger, c.schema)
	if err != nil {
		return fmt.Errorf("failed to create schema repository: %w", err)
	}
	c.schemaRepo = schemaRepo

	// Initialize processor repository
	processorRepo, err := database.NewProcessorConfigRepo(ctx, c.db, c.logger, c.schema)
	if err != nil {
		return fmt.Errorf("failed to create processor repository: %w", err)
	}
	c.processorRepo = processorRepo

	// Initialize embedding repository
	embeddingRepo, err := database.NewEmbeddingConfigRepository(ctx, c.db, c.logger)
	if err != nil {
		return fmt.Errorf("failed to create embedding repository: %w", err)
	}
	c.embeddingRepo = embeddingRepo

	// Initialize template repository
	templateRepo, err := database.NewTemplateConfigRepo(ctx, c.db, c.logger)
	if err != nil {
		return fmt.Errorf("failed to create template repository: %w", err)
	}
	c.templateRepo = templateRepo

	// Initialize vector store repository
	vectorStoreRepo, err := database.NewVectorStoreConfigManager(ctx, c.db, c.logger)
	if err != nil {
		return fmt.Errorf("failed to create vector store repository: %w", err)
	}
	c.vectorStoreRepo = vectorStoreRepo

	return nil
}

// =========================
// Schema Config Operations
// =========================

func (c *OtelSQLConfigDatabaseClient) GetSchemaConfigByID(ctx context.Context, id int64) (*database.DatabaseSchemaConfig, error) {
	if c.schemaRepo == nil {
		return nil, fmt.Errorf("schema repository not initialized")
	}
	return c.schemaRepo.GetByID(ctx, id)
}

func (c *OtelSQLConfigDatabaseClient) GetSchemaConfigByName(ctx context.Context, name string) (*database.DatabaseSchemaConfig, error) {
	if c.schemaRepo == nil {
		return nil, fmt.Errorf("schema repository not initialized")
	}
	return c.schemaRepo.GetByName(ctx, name)
}

func (c *OtelSQLConfigDatabaseClient) GetAllSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error) {
	if c.schemaRepo == nil {
		return nil, fmt.Errorf("schema repository not initialized")
	}
	return c.schemaRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllEnabledSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error) {
	if c.schemaRepo == nil {
		return nil, fmt.Errorf("schema repository not initialized")
	}
	return c.schemaRepo.GetAllEnabled(ctx)
}

func (c *OtelSQLConfigDatabaseClient) UpsertSchemaConfig(ctx context.Context, config *database.DatabaseSchemaConfig) error {
	if c.schemaRepo == nil {
		return fmt.Errorf("schema repository not initialized")
	}
	return c.schemaRepo.Upsert(ctx, config)
}

func (c *OtelSQLConfigDatabaseClient) DeleteSchemaConfig(ctx context.Context, id int64) error {
	if c.schemaRepo == nil {
		return fmt.Errorf("schema repository not initialized")
	}
	return c.schemaRepo.Delete(ctx, id)
}

func (c *OtelSQLConfigDatabaseClient) GetSchemaConfigsForProcessor(ctx context.Context, processorID int64) ([]*database.DatabaseSchemaConfig, error) {
	if c.processorRepo == nil {
		return nil, fmt.Errorf("processor repository not initialized")
	}
	// Get the processor config first
	processorConfig, err := c.processorRepo.GetByID(ctx, processorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get processor config: %w", err)
	}
	if processorConfig == nil {
		return nil, fmt.Errorf("processor config not found")
	}
	// Get the associated schema config
	schemaConfig, err := c.processorRepo.GetSchemaById(ctx, processorConfig.DatabaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema config: %w", err)
	}
	if schemaConfig == nil {
		return []*database.DatabaseSchemaConfig{}, nil
	}
	return []*database.DatabaseSchemaConfig{schemaConfig}, nil
}

// =========================
// Processor Config Operations
// =========================

func (c *OtelSQLConfigDatabaseClient) GetProcessorConfigByID(ctx context.Context, id int64) (*database.ProcessorMappingConfig, error) {
	if c.processorRepo == nil {
		return nil, fmt.Errorf("processor repository not initialized")
	}
	return c.processorRepo.GetByID(ctx, id)
}

func (c *OtelSQLConfigDatabaseClient) GetProcessorConfigByName(ctx context.Context, name string) (*database.ProcessorMappingConfig, error) {
	if c.processorRepo == nil {
		return nil, fmt.Errorf("processor config repository not initialized")
	}
	return c.processorRepo.GetByName(ctx, name)
}

func (c *OtelSQLConfigDatabaseClient) GetProcessorConfigsBySchemaID(ctx context.Context, schemaID int64) ([]*database.ProcessorMappingConfig, error) {
	if c.processorRepo == nil {
		return nil, fmt.Errorf("processor config repository not initialized")
	}
	// Get all processor configs and filter by schema ID
	allConfigs, err := c.processorRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get all processor configs: %w", err)
	}
	
	var filteredConfigs []*database.ProcessorMappingConfig
	for _, config := range allConfigs {
		if config.DatabaseID == schemaID {
			filteredConfigs = append(filteredConfigs, config)
		}
	}
	return filteredConfigs, nil
}

func (c *OtelSQLConfigDatabaseClient) GetAllProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error) {
	if c.processorRepo == nil {
		return nil, fmt.Errorf("processor repository not initialized")
	}
	return c.processorRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllEnabledProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error) {
	if c.processorRepo == nil {
		return nil, fmt.Errorf("processor repository not initialized")
	}
	return c.processorRepo.GetAllEnabled(ctx)
}

func (c *OtelSQLConfigDatabaseClient) UpsertProcessorConfig(ctx context.Context, config *database.ProcessorMappingConfig) error {
	if c.processorRepo == nil {
		return fmt.Errorf("processor repository not initialized")
	}
	return c.processorRepo.Upsert(ctx, config)
}

func (c *OtelSQLConfigDatabaseClient) DeleteProcessorConfig(ctx context.Context, id int64) error {
	if c.processorRepo == nil {
		return fmt.Errorf("processor repository not initialized")
	}
	return c.processorRepo.Delete(ctx, id)
}

func (c *OtelSQLConfigDatabaseClient) GetSchemaByProcessorID(ctx context.Context, processorID int64) (*database.DatabaseSchemaConfig, error) {
	if c.processorRepo == nil {
		return nil, fmt.Errorf("processor repository not initialized")
	}
	// Get the processor config first
	processorConfig, err := c.processorRepo.GetByID(ctx, processorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get processor config: %w", err)
	}
	if processorConfig == nil {
		return nil, fmt.Errorf("processor config not found")
	}
	// Get the associated schema config using the processor repository's method
	return c.processorRepo.GetSchemaById(ctx, processorConfig.DatabaseID)
}

// =========================
// Embedding Config Operations
// =========================

func (c *OtelSQLConfigDatabaseClient) GetEmbeddingConfigByID(ctx context.Context, id int64) (*database.EmbeddingConfig, error) {
	if c.embeddingRepo == nil {
		return nil, fmt.Errorf("embedding repository not initialized")
	}
	return c.embeddingRepo.GetByID(ctx, id)
}

func (c *OtelSQLConfigDatabaseClient) GetEmbeddingConfigByName(ctx context.Context, name string) (*database.EmbeddingConfig, error) {
	if c.embeddingRepo == nil {
		return nil, fmt.Errorf("embedding repository not initialized")
	}
	return c.embeddingRepo.GetByName(ctx, name)
}

func (c *OtelSQLConfigDatabaseClient) GetEmbeddingConfigsBySchemaID(ctx context.Context, schemaID int64) ([]*database.EmbeddingConfig, error) {
	if c.embeddingRepo == nil {
		return nil, fmt.Errorf("embedding repository not initialized")
	}
	// Since EmbeddingConfig doesn't have schema association, return all configs
	// This may need to be updated when schema association is added to embedding configs
	return c.embeddingRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllEmbeddingConfigs(ctx context.Context) ([]*database.EmbeddingConfig, error) {
	if c.embeddingRepo == nil {
		return nil, fmt.Errorf("embedding repository not initialized")
	}
	return c.embeddingRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllEnabledEmbeddingConfigs(ctx context.Context) ([]*database.EmbeddingConfig, error) {
	if c.embeddingRepo == nil {
		return nil, fmt.Errorf("embedding repository not initialized")
	}
	// EmbeddingConfigRepository doesn't have GetAllEnabled method, return all configs
	// This may need to be updated when enabled filtering is added
	return c.embeddingRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) UpsertEmbeddingConfig(ctx context.Context, config *database.EmbeddingConfig) error {
	if c.embeddingRepo == nil {
		return fmt.Errorf("embedding repository not initialized")
	}
	return c.embeddingRepo.Upsert(ctx, config)
}

func (c *OtelSQLConfigDatabaseClient) DeleteEmbeddingConfig(ctx context.Context, id int64) error {
	if c.embeddingRepo == nil {
		return fmt.Errorf("embedding repository not initialized")
	}
	return c.embeddingRepo.Delete(ctx, id)
}

// =========================
// Template Config Operations
// =========================

func (c *OtelSQLConfigDatabaseClient) GetTemplateConfigByID(ctx context.Context, id int64) (*database.EmbeddingConfigTemplate, error) {
	if c.templateRepo == nil {
		return nil, fmt.Errorf("template repository not initialized")
	}
	return c.templateRepo.GetByID(ctx, id)
}

func (c *OtelSQLConfigDatabaseClient) GetTemplateConfigByName(ctx context.Context, name string) (*database.EmbeddingConfigTemplate, error) {
	if c.templateRepo == nil {
		return nil, fmt.Errorf("template repository not initialized")
	}
	return c.templateRepo.GetByName(ctx, name)
}

func (c *OtelSQLConfigDatabaseClient) GetTemplateConfigsBySchemaID(ctx context.Context, schemaID int64) ([]*database.EmbeddingConfigTemplate, error) {
	if c.templateRepo == nil {
		return nil, fmt.Errorf("template repository not initialized")
	}
	// Since EmbeddingConfigTemplate doesn't have schema association, return all configs
	// This may need to be updated when schema association is added to template configs
	return c.templateRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllTemplateConfigs(ctx context.Context) ([]*database.EmbeddingConfigTemplate, error) {
	if c.templateRepo == nil {
		return nil, fmt.Errorf("template repository not initialized")
	}
	return c.templateRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllEnabledTemplateConfigs(ctx context.Context) ([]*database.EmbeddingConfigTemplate, error) {
	if c.templateRepo == nil {
		return nil, fmt.Errorf("template repository not initialized")
	}
	// EmbeddingConfigTemplate doesn't have enabled filtering, return all configs
	// This may need to be updated when enabled filtering is added
	return c.templateRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) UpsertTemplateConfig(ctx context.Context, config *database.EmbeddingConfigTemplate) error {
	if c.templateRepo == nil {
		return fmt.Errorf("template repository not initialized")
	}
	return c.templateRepo.Upsert(ctx, config)
}

func (c *OtelSQLConfigDatabaseClient) DeleteTemplateConfig(ctx context.Context, id int64) error {
	if c.templateRepo == nil {
		return fmt.Errorf("template repository not initialized")
	}
	return c.templateRepo.Delete(ctx, id)
}

// =========================
// Vector Store Config Operations
// =========================

func (c *OtelSQLConfigDatabaseClient) GetVectorStoreConfigByID(ctx context.Context, id int64) (*database.VectorStoreConfig, error) {
	if c.vectorStoreRepo == nil {
		return nil, fmt.Errorf("vector store repository not initialized")
	}
	return c.vectorStoreRepo.GetByID(ctx, id)
}

func (c *OtelSQLConfigDatabaseClient) GetVectorStoreConfigByName(ctx context.Context, name string) (*database.VectorStoreConfig, error) {
	if c.vectorStoreRepo == nil {
		return nil, fmt.Errorf("vector store repository not initialized")
	}
	return c.vectorStoreRepo.GetByName(ctx, name)
}

func (c *OtelSQLConfigDatabaseClient) GetVectorStoreConfigsBySchemaID(ctx context.Context, schemaID int64) ([]*database.VectorStoreConfig, error) {
	if c.vectorStoreRepo == nil {
		return nil, fmt.Errorf("vector store repository not initialized")
	}
	// Since VectorStoreConfig doesn't have schema association, return all configs
	// This may need to be updated when schema association is added to vector store configs
	return c.vectorStoreRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllVectorStoreConfigs(ctx context.Context) ([]*database.VectorStoreConfig, error) {
	if c.vectorStoreRepo == nil {
		return nil, fmt.Errorf("vector store repository not initialized")
	}
	return c.vectorStoreRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) GetAllEnabledVectorStoreConfigs(ctx context.Context) ([]*database.VectorStoreConfig, error) {
	if c.vectorStoreRepo == nil {
		return nil, fmt.Errorf("vector store repository not initialized")
	}
	// VectorStoreConfig doesn't have enabled filtering, return all configs
	// This may need to be updated when enabled filtering is added
	return c.vectorStoreRepo.GetAll(ctx)
}

func (c *OtelSQLConfigDatabaseClient) UpsertVectorStoreConfig(ctx context.Context, config *database.VectorStoreConfig) error {
	if c.vectorStoreRepo == nil {
		return fmt.Errorf("vector store repository not initialized")
	}
	return c.vectorStoreRepo.Upsert(ctx, config)
}

func (c *OtelSQLConfigDatabaseClient) DeleteVectorStoreConfig(ctx context.Context, id int64) error {
	if c.vectorStoreRepo == nil {
		return fmt.Errorf("vector store repository not initialized")
	}
	return c.vectorStoreRepo.Delete(ctx, id)
}

// =========================
// Utility Operations
// =========================

func (c *OtelSQLConfigDatabaseClient) GetConfigStatistics(ctx context.Context) (*database.ConfigStatistics, error) {
	stats := &database.ConfigStatistics{}
	
	// Get schema config count
	if c.schemaRepo != nil {
		schemaConfigs, err := c.schemaRepo.GetAll(ctx)
		if err == nil {
			stats.SchemaConfigCount = int64(len(schemaConfigs))
		}
	}
	
	// Get processor config count
	if c.processorRepo != nil {
		processorConfigs, err := c.processorRepo.GetAll(ctx)
		if err == nil {
			stats.ProcessorConfigCount = int64(len(processorConfigs))
		}
	}
	
	// Get embedding config count
	if c.embeddingRepo != nil {
		embeddingConfigs, err := c.embeddingRepo.GetAll(ctx)
		if err == nil {
			stats.EmbeddingConfigCount = int64(len(embeddingConfigs))
		}
	}
	
	// Get template config count
	if c.templateRepo != nil {
		templateConfigs, err := c.templateRepo.GetAll(ctx)
		if err == nil {
			stats.TemplateConfigCount = int64(len(templateConfigs))
		}
	}
	
	// Get vector store config count
	if c.vectorStoreRepo != nil {
		vectorStoreConfigs, err := c.vectorStoreRepo.GetAll(ctx)
		if err == nil {
			stats.VectorStoreConfigCount = int64(len(vectorStoreConfigs))
		}
	}
	
	return stats, nil
}

func (c *OtelSQLConfigDatabaseClient) EnsureTablesAndDefaults(ctx context.Context) error {
	// Ensure all repository tables and defaults are created
	var errors []error
	
	// Schema repository table and defaults
	if c.schemaRepo != nil {
		if schemaRepoImpl, ok := c.schemaRepo.(*database.SchemaConfigRepository); ok {
			if err := schemaRepoImpl.EnsureTableAndDefault(ctx, c.schema); err != nil {
				errors = append(errors, fmt.Errorf("failed to ensure schema tables: %w", err))
			}
		}
	}
	
	// Processor repository table and defaults
	if c.processorRepo != nil {
		if err := c.processorRepo.EnsureTableAndDefault(ctx, c.schema); err != nil {
			errors = append(errors, fmt.Errorf("failed to ensure processor tables: %w", err))
		}
	}
	
	// Embedding repository table and defaults
	if c.embeddingRepo != nil {
		if err := c.embeddingRepo.EnsureTableAndDefault(ctx); err != nil {
			errors = append(errors, fmt.Errorf("failed to ensure embedding tables: %w", err))
		}
	}
	
	// Template repository table and defaults
	if c.templateRepo != nil {
		if templateRepoImpl, ok := c.templateRepo.(interface{ EnsureTableAndDefault(context.Context) error }); ok {
			if err := templateRepoImpl.EnsureTableAndDefault(ctx); err != nil {
				errors = append(errors, fmt.Errorf("failed to ensure template tables: %w", err))
			}
		}
	}
	
	// Vector store repository table and defaults
	if c.vectorStoreRepo != nil {
		if vectorStoreRepoImpl, ok := c.vectorStoreRepo.(interface{ EnsureTableAndDefault(context.Context) error }); ok {
			if err := vectorStoreRepoImpl.EnsureTableAndDefault(ctx); err != nil {
				errors = append(errors, fmt.Errorf("failed to ensure vector store tables: %w", err))
			}
		}
	}
	
	// Return combined errors if any
	if len(errors) > 0 {
		errorMsgs := make([]string, len(errors))
		for i, err := range errors {
			errorMsgs[i] = err.Error()
		}
		return fmt.Errorf("failed to ensure tables and defaults: %s", strings.Join(errorMsgs, "; "))
	}
	
	return nil
}
