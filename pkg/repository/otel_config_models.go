package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// =========================
// Configuration Statistics
// =========================

// ConfigStatistics holds statistics about configuration entities
type ConfigStatistics struct {
	SchemaConfigCount           int64     `json:"schema_config_count"`
	EnabledSchemaConfigCount    int64     `json:"enabled_schema_config_count"`
	ProcessorConfigCount        int64     `json:"processor_config_count"`
	EnabledProcessorConfigCount int64     `json:"enabled_processor_config_count"`
	EmbeddingConfigCount        int64     `json:"embedding_config_count"`
	TemplateConfigCount         int64     `json:"template_config_count"`
	VectorStoreConfigCount      int64     `json:"vector_store_config_count"`
	LastUpdated                 time.Time `json:"last_updated"`
}

// =========================
// Vector Store Configuration (Reference)
// =========================

// Note: VectorStoreConfig is defined in config_vectorstore_provider.go
// This file provides additional utilities and validation for it

// =========================
// Configuration Query Models
// =========================

// ConfigQuery represents common query parameters for configuration entities
type ConfigQuery struct {
	ID        *int64  `json:"id,omitempty"`
	Name      *string `json:"name,omitempty"`
	Enabled   *bool   `json:"enabled,omitempty"`
	Provider  *string `json:"provider,omitempty"`
	Limit     int     `json:"limit"`
	Offset    int     `json:"offset"`
	OrderBy   string  `json:"order_by"`
	Direction string  `json:"direction"`
}

// SchemaConfigQuery represents query parameters specific to schema configurations
type SchemaConfigQuery struct {
	ConfigQuery
	DatabaseSchema *string `json:"database_schema,omitempty"`
}

// ProcessorConfigQuery represents query parameters specific to processor configurations
type ProcessorConfigQuery struct {
	ConfigQuery
	DatabaseID     *int64  `json:"database_id,omitempty"`
	VectorStoreID  *int64  `json:"vectorstore_id,omitempty"`
	TemplateID     *int64  `json:"template_id,omitempty"`
	ProcessingMode *string `json:"processing_mode,omitempty"`
}

// EmbeddingConfigQuery represents query parameters specific to embedding configurations
type EmbeddingConfigQuery struct {
	ConfigQuery
	Model     *string `json:"model,omitempty"`
	Dimension *int    `json:"dimension,omitempty"`
}

// TemplateConfigQuery represents query parameters specific to template configurations
type TemplateConfigQuery struct {
	ConfigQuery
	HasContent *bool `json:"has_content,omitempty"`
}

// =========================
// Configuration Validation
// =========================

// ValidateSchemaConfig validates a DatabaseSchemaConfig
func ValidateSchemaConfig(config *database.DatabaseSchemaConfig) error {
	if config == nil {
		return fmt.Errorf("schema config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("schema config name is required")
	}
	if config.DatabaseSchema == "" {
		return fmt.Errorf("database schema is required")
	}
	return nil
}

// ValidateProcessorConfig validates a ProcessorMappingConfig
func ValidateProcessorConfig(config *database.ProcessorMappingConfig) error {
	if config == nil {
		return fmt.Errorf("processor config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("processor config name is required")
	}
	if config.DatabaseID <= 0 {
		return fmt.Errorf("database ID must be positive")
	}
	if config.VectorStoreID <= 0 {
		return fmt.Errorf("vector store ID must be positive")
	}
	if config.TemplateID <= 0 {
		return fmt.Errorf("template ID must be positive")
	}
	if config.ProcessingMode == "" {
		return fmt.Errorf("processing mode is required")
	}

	// Validate processing mode
	validModes := map[string]bool{
		"template":    true,
		"embedding":   true,
		"hybrid":      true,
		"passthrough": true,
	}
	if !validModes[config.ProcessingMode] {
		return fmt.Errorf("invalid processing mode: %s", config.ProcessingMode)
	}

	return nil
}

// ValidateEmbeddingConfig validates an EmbeddingConfig
func ValidateEmbeddingConfig(config *database.EmbeddingConfig) error {
	if config == nil {
		return fmt.Errorf("embedding config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("embedding config name is required")
	}
	if config.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if config.Model == "" {
		return fmt.Errorf("model is required")
	}
	if config.Dimension <= 0 {
		return fmt.Errorf("dimension must be positive")
	}
	if config.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	return nil
}

// ValidateTemplateConfig validates an EmbeddingConfigTemplate
func ValidateTemplateConfig(config *database.EmbeddingConfigTemplate) error {
	if config == nil {
		return fmt.Errorf("template config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("template config name is required")
	}
	if config.Content == nil || len(config.Content) == 0 {
		return fmt.Errorf("template content is required")
	}
	return nil
}

// ValidateVectorStoreConfig validates vector store configuration
func ValidateVectorStoreConfig(config *database.VectorStoreConfig) error {
	if config == nil {
		return fmt.Errorf("vector store config cannot be nil")
	}
	if config.Name == "" {
		return fmt.Errorf("vector store config name cannot be empty")
	}
	if config.Provider == "" {
		return fmt.Errorf("vector store provider cannot be empty")
	}
	return nil
}

// =========================
// Configuration Helpers
// =========================

// GetProcessingModeFromConfig extracts processing mode from processor config with fallback
func GetProcessingModeFromConfig(config *database.ProcessorMappingConfig) string {
	if config == nil {
		return "hybrid"
	}
	if config.ProcessingMode != "" {
		return config.ProcessingMode
	}
	// Check metadata for processing mode
	if config.Metadata != nil {
		if mode, ok := config.Metadata["processing_mode"].(string); ok && mode != "" {
			return mode
		}
	}
	return "hybrid"
}

// GetEmbeddingBatchModeFromConfig extracts batch mode setting from processor config
func GetEmbeddingBatchModeFromConfig(config *database.ProcessorMappingConfig) bool {
	if config == nil || config.Metadata == nil {
		return false
	}
	if batchMode, ok := config.Metadata["embedding_batch_group_signals"].(bool); ok {
		return batchMode
	}
	return false
}

// GetRoutingRulesFromConfig extracts routing rules from config
func GetRoutingRulesFromConfig(config interface{}) map[string]interface{} {
	switch c := config.(type) {
	case *database.DatabaseSchemaConfig:
		if c.Routing != nil {
			return c.Routing
		}
	case *database.ProcessorMappingConfig:
		if c.Routing != nil {
			return c.Routing
		}
	}
	return map[string]interface{}{}
}

// GetFilterRulesFromConfig extracts filter rules from config
func GetFilterRulesFromConfig(config interface{}) map[string]interface{} {
	switch c := config.(type) {
	case *database.DatabaseSchemaConfig:
		// DatabaseSchemaConfig no longer has Filter field
		// Return empty map for schema configs
		return map[string]interface{}{}
	case *database.ProcessorMappingConfig:
		if c.EmbeddingFilter != nil {
			return c.EmbeddingFilter
		}
	}
	return map[string]interface{}{}
}

// =========================
// JSON Utilities
// =========================

// MarshalConfigToJSON marshals a config object to JSON string
func MarshalConfigToJSON(config interface{}) (string, error) {
	data, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal config to JSON: %w", err)
	}
	return string(data), nil
}

// UnmarshalConfigFromJSON unmarshals a JSON string to a config object
func UnmarshalConfigFromJSON(jsonStr string, config interface{}) error {
	if jsonStr == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(jsonStr), config); err != nil {
		return fmt.Errorf("failed to unmarshal config from JSON: %w", err)
	}
	return nil
}

// =========================
// Configuration Defaults
// =========================

// DefaultSchemaConfig returns a default schema configuration
func DefaultSchemaConfig() *database.DatabaseSchemaConfig {
	return &database.DatabaseSchemaConfig{
		Name:           "default_otel_schema",
		DatabaseSchema: DefaultSchema,
		Enabled:        true,
		Description:    "Default OpenTelemetry schema configuration",
		Routing:        map[string]interface{}{},
		Priority:       0,
		Metadata:       map[string]interface{}{},
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
}

// DefaultProcessorConfig returns a default processor configuration
func DefaultProcessorConfig() *database.ProcessorMappingConfig {
	return &database.ProcessorMappingConfig{
		Name:            "default",
		DatabaseID:      1,
		VectorStoreID:   1,
		TemplateID:      1,
		ProcessingMode:  "template",
		Enabled:         true,
		Routing:         map[string]interface{}{},
		EmbeddingFilter: map[string]interface{}{},
		Metadata: map[string]interface{}{
			"embedding_batch_group_signals": false,
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// DefaultEmbeddingConfig returns a default embedding configuration
func DefaultEmbeddingConfig() *database.EmbeddingConfig {
	return &database.EmbeddingConfig{
		Name:             "minilm",
		Provider:         "sbert",
		Endpoint:         "",
		Model:            "sentence-transformers/all-MiniLM-L6-v2",
		Timeout:          30 * time.Second,
		RateLimit:        0,
		Headers:          map[string]string{},
		Normalize:        false,
		Dimension:        384,
		MaxContextLength: 256,
		Description:      "Default MiniLM embedding configuration",
		Metadata:         map[string]interface{}{},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

// DefaultTemplateConfig returns a default template configuration
func DefaultTemplateConfig() *database.EmbeddingConfigTemplate {
	return &database.EmbeddingConfigTemplate{
		Name:             "default",
		Content:          map[string]string{"default": "default embedding template"},
		TemplateMetadata: map[string][]string{"version": {"1.0"}},
		Description:      "Default template configuration",
		Metadata:         map[string]interface{}{},
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

// DefaultVectorStoreConfig returns a default vector store configuration
func DefaultVectorStoreConfig() *database.VectorStoreConfig {
	return &database.VectorStoreConfig{
		Name:                  "default",
		Provider:              "postgres",
		Endpoint:              "",
		User:                  "",
		Database:              "",
		EmbeddingID:           1,
		BatchSize:             100,
		FlushInterval:         200 * time.Millisecond,
		MaxRetries:            3,
		MaxOpenConns:          25,
		MaxIdleConns:          5,
		ConnMaxLifetime:       3600 * time.Second,
		BatchErrorPolicy:      "retry",
		MaxConcurrentBatches:  8,
		VendorSpecificOptions: map[string]interface{}{},
		Metadata:              map[string]interface{}{},
		Description:           "Default vector store configuration",
		CreatedAt:             time.Now(),
		UpdatedAt:             time.Now(),
	}
}
