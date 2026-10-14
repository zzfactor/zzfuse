package database

import (
	"context"
)

// ConfigStatistics holds statistics about configuration entities
type ConfigStatistics struct {
	SchemaConfigCount           int64 `json:"schema_config_count"`
	EnabledSchemaConfigCount    int64 `json:"enabled_schema_config_count"`
	ProcessorConfigCount        int64 `json:"processor_config_count"`
	EnabledProcessorConfigCount int64 `json:"enabled_processor_config_count"`
	EmbeddingConfigCount        int64 `json:"embedding_config_count"`
	EnabledEmbeddingConfigCount int64 `json:"enabled_embedding_config_count"`
	TemplateConfigCount         int64 `json:"template_config_count"`
	EnabledTemplateConfigCount  int64 `json:"enabled_template_config_count"`
	VectorStoreConfigCount      int64 `json:"vector_store_config_count"`
}

// SchemaStats holds statistics about a specific schema
type SchemaStats struct {
	SchemaName  string `json:"schema_name"`
	TableCount  int64  `json:"table_count"`
	TotalRows   int64  `json:"total_rows"`
	TotalSizeMB int64  `json:"total_size_mb"`
	LastUpdated string `json:"last_updated"`
	IsEnabled   bool   `json:"is_enabled"`
}

// EmbeddingConfigRepositoryInterface defines the methods for embedding config repository (for mocking/testing)
type EmbeddingConfigInterface interface {
	GetByID(ctx context.Context, id int64) (*EmbeddingConfig, error)
	GetByName(ctx context.Context, name string) (*EmbeddingConfig, error)
	GetAll(ctx context.Context) ([]*EmbeddingConfig, error)
	Upsert(ctx context.Context, config *EmbeddingConfig) error
	Delete(ctx context.Context, id int64) error
}

// EmbeddingConfigRepositoryInterface defines the methods for embedding config repository (for mocking/testing)
type VectorStoreConfigInterface interface {
	GetByID(ctx context.Context, id int64) (*VectorStoreConfig, error)
	GetByName(ctx context.Context, name string) (*VectorStoreConfig, error)
	GetAll(ctx context.Context) ([]*VectorStoreConfig, error)
	Upsert(ctx context.Context, config *VectorStoreConfig) error
	Delete(ctx context.Context, id int64) error
}

// EmbeddingConfigTemplateInterface defines the methods for embedding config template repository
type EmbeddingConfigTemplateInterface interface {
	GetByID(ctx context.Context, id int64) (*EmbeddingConfigTemplate, error)
	GetByName(ctx context.Context, name string) (*EmbeddingConfigTemplate, error)
	GetAll(ctx context.Context) ([]*EmbeddingConfigTemplate, error)
	Upsert(ctx context.Context, config *EmbeddingConfigTemplate) error
	Delete(ctx context.Context, id int64) error
}

// SchemaConfigInterface defines the methods for schema config repository (for mocking/testing)
type SchemaConfigInterface interface {
	GetByID(ctx context.Context, id int64) (*DatabaseSchemaConfig, error)
	GetByName(ctx context.Context, name string) (*DatabaseSchemaConfig, error)
	GetAll(ctx context.Context) ([]*DatabaseSchemaConfig, error)
	GetAllEnabled(ctx context.Context) ([]*DatabaseSchemaConfig, error)
	Upsert(ctx context.Context, config *DatabaseSchemaConfig) error
	Delete(ctx context.Context, id int64) error
	CreateSchema(ctx context.Context, schema string) error
	DeleteSchema(ctx context.Context, schema string) error    //todo implement
	GetSchemaStats(ctx context.Context) (*SchemaStats, error) //todo implement
}

// ProcessorConfigInterface defines the methods for processor config repository
type ProcessorConfigInterface interface {
	GetByID(ctx context.Context, id int64) (*ProcessorMappingConfig, error)
	GetByName(ctx context.Context, name string) (*ProcessorMappingConfig, error)
	GetAll(ctx context.Context) ([]*ProcessorMappingConfig, error)
	GetAllEnabled(ctx context.Context) ([]*ProcessorMappingConfig, error)
	Upsert(ctx context.Context, config *ProcessorMappingConfig) error
	Delete(ctx context.Context, id int64) error
	GetSchemaById(ctx context.Context, ID int64) (*DatabaseSchemaConfig, error)
}

// SignalHashRepositoryInterface defines the methods for signal hashes repository
type SignalHashRepositoryInterface interface {
	Create(ctx context.Context, sh *SignalHash) error
	GetByHash(ctx context.Context, hash string) (*SignalHash, error)
	HashExists(ctx context.Context, hash string) (bool, error)
	UpdateFlags(ctx context.Context, hash string, flags int16) error
	GetByTimeRange(ctx context.Context, startTime, endTime int64) ([]*SignalHash, error)
	GetBySignalType(ctx context.Context, signalType int16) ([]*SignalHash, error)
	GetByFlags(ctx context.Context, flags int16) ([]*SignalHash, error)
	CountByHash(ctx context.Context, hash string) (int64, error)
	DeleteByTimeRange(ctx context.Context, olderThan int64) error
	EnsureTable(ctx context.Context, schema string) error

	// Batch operations
	BatchCreate(ctx context.Context, hashes []*SignalHash) error
	BatchCreateOrUpdate(ctx context.Context, hashes []*SignalHash) error
	BatchCheckExists(ctx context.Context, hashes []string) (map[string]bool, error)
	GetDuplicateHashes(ctx context.Context, startTime, endTime int64) ([]*SignalHash, error)
}

// RunRepositoryInterface defines the methods for runs repository
type RunRepositoryInterface interface {
	EnsureTable(ctx context.Context, schema string) error
	GetByID(ctx context.Context, id int64) (*Run, error)
	GetActiveIdByName(ctx context.Context, name string) (int64, error)
	GetCount(ctx context.Context, name string) (int64, error)
	GetAllIds(ctx context.Context, name string) ([]int64, error)
	Create(ctx context.Context, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) (int64, error)
	CreateWithSessionID(ctx context.Context, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) (int64, error)
	Update(ctx context.Context, id int64, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) error
	UpdateWithSessionID(ctx context.Context, id int64, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) error
	Delete(ctx context.Context, id int64) error

	// Additional utility methods
	GetByProcessorID(ctx context.Context, processorID int64) ([]*Run, error)
	GetByTraceID(ctx context.Context, traceID string) (*Run, error)
	GetBySessionID(ctx context.Context, sessionID string) ([]*Run, error)
	EndRun(ctx context.Context, id int64, endTimestampNs int64) error
	GetActiveRunsByServiceName(ctx context.Context, serviceName string) ([]*Run, error)
}

// OtelConfigDatabaseInterface defines the unified interface for config database operations
type OtelConfigDatabaseInterface interface {
	// Schema Config operations
	GetSchemaConfigByID(ctx context.Context, id int64) (*DatabaseSchemaConfig, error)
	GetSchemaConfigByName(ctx context.Context, name string) (*DatabaseSchemaConfig, error)
	GetAllSchemaConfigs(ctx context.Context) ([]*DatabaseSchemaConfig, error)
	GetAllEnabledSchemaConfigs(ctx context.Context) ([]*DatabaseSchemaConfig, error)
	UpsertSchemaConfig(ctx context.Context, config *DatabaseSchemaConfig) error
	DeleteSchemaConfig(ctx context.Context, id int64) error

	// Processor Config operations
	GetProcessorConfigByID(ctx context.Context, id int64) (*ProcessorMappingConfig, error)
	GetProcessorConfigByName(ctx context.Context, name string) (*ProcessorMappingConfig, error)
	GetAllProcessorConfigs(ctx context.Context) ([]*ProcessorMappingConfig, error)
	GetAllEnabledProcessorConfigs(ctx context.Context) ([]*ProcessorMappingConfig, error)
	UpsertProcessorConfig(ctx context.Context, config *ProcessorMappingConfig) error
	DeleteProcessorConfig(ctx context.Context, id int64) error
	GetSchemaByProcessorID(ctx context.Context, processorID int64) (*DatabaseSchemaConfig, error)

	// Embedding Config operations
	GetEmbeddingConfigByID(ctx context.Context, id int64) (*EmbeddingConfig, error)
	GetEmbeddingConfigByName(ctx context.Context, name string) (*EmbeddingConfig, error)
	GetAllEmbeddingConfigs(ctx context.Context) ([]*EmbeddingConfig, error)
	UpsertEmbeddingConfig(ctx context.Context, config *EmbeddingConfig) error
	DeleteEmbeddingConfig(ctx context.Context, id int64) error

	// Template Config operations
	GetTemplateConfigByID(ctx context.Context, id int64) (*EmbeddingConfigTemplate, error)
	GetTemplateConfigByName(ctx context.Context, name string) (*EmbeddingConfigTemplate, error)
	GetAllTemplateConfigs(ctx context.Context) ([]*EmbeddingConfigTemplate, error)
	UpsertTemplateConfig(ctx context.Context, config *EmbeddingConfigTemplate) error
	DeleteTemplateConfig(ctx context.Context, id int64) error

	// Vector Store Config operations
	GetVectorStoreConfigByID(ctx context.Context, id int64) (*VectorStoreConfig, error)
	GetVectorStoreConfigByName(ctx context.Context, name string) (*VectorStoreConfig, error)
	GetAllVectorStoreConfigs(ctx context.Context) ([]*VectorStoreConfig, error)
	UpsertVectorStoreConfig(ctx context.Context, config *VectorStoreConfig) error
	DeleteVectorStoreConfig(ctx context.Context, id int64) error

	// Statistics and utilities
	GetStatistics(ctx context.Context) (*ConfigStatistics, error)
	EnsureTablesAndDefaults(ctx context.Context) error
}
