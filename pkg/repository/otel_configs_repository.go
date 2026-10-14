// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// ConfigOperation represents an atomic configuration operation
type ConfigOperation struct {
	Type       string      // "create", "update", "delete"
	ConfigType string      // "schema", "processor", "embedding", "template", "vectorstore"
	ID         int64       // For update/delete operations
	Config     interface{} // Configuration object for create/update
}

// ConfigIntegrityReport represents the result of configuration integrity validation
type ConfigIntegrityReport struct {
	Valid               bool                 `json:"valid"`
	GeneratedAt         time.Time            `json:"generated_at"`
	Errors              []string             `json:"errors"`
	Warnings            []string             `json:"warnings"`
	OrphanedConfigs     []OrphanedConfig     `json:"orphaned_configs"`
	MissingDependencies []MissingDependency  `json:"missing_dependencies"`
}

// ConfigChangeNotification represents a configuration change event
type ConfigChangeNotification struct {
	EventType    string                 `json:"event_type"`    // "created", "updated", "deleted"
	ConfigType   string                 `json:"config_type"`   // "schema", "processor", etc.
	ConfigID     int64                  `json:"config_id"`
	ConfigName   string                 `json:"config_name"`
	OldVersion   *int64                 `json:"old_version,omitempty"`
	NewVersion   *int64                 `json:"new_version,omitempty"`
	ChangedBy    string                 `json:"changed_by"`
	Timestamp    time.Time              `json:"timestamp"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ConfigChangeHandler defines the interface for handling configuration change notifications
type ConfigChangeHandler interface {
	OnConfigChanged(notification ConfigChangeNotification) error
}

// ConfigValidationError represents a detailed validation error
type ConfigValidationError struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Message string `json:"message"`
	Code    string `json:"code"`
}

// BusinessLogicError represents a comprehensive business logic error
type BusinessLogicError struct {
	Operation        string                  `json:"operation"`
	ConfigType       string                  `json:"config_type"`
	ConfigID         *int64                  `json:"config_id,omitempty"`
	ErrorType        string                  `json:"error_type"`        // "validation", "dependency", "conflict", "not_found"
	Message          string                  `json:"message"`
	ValidationErrors []ConfigValidationError `json:"validation_errors,omitempty"`
	Cause            error                   `json:"-"`
	Timestamp        time.Time               `json:"timestamp"`
}

func (e *BusinessLogicError) Error() string {
	return fmt.Sprintf("%s operation on %s failed: %s", e.Operation, e.ConfigType, e.Message)
}

func (e *BusinessLogicError) Unwrap() error {
	return e.Cause
}

// OrphanedConfig represents a configuration that references non-existent dependencies
type OrphanedConfig struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// MissingDependency represents a missing dependency relationship
type MissingDependency struct {
	ConfigType     string `json:"config_type"`
	ConfigID       int64  `json:"config_id"`
	DependencyType string `json:"dependency_type"`
	DependencyID   int64  `json:"dependency_id"`
}

// ConfigRepository provides high-level operations for OTel configurations with business logic
type ConfigRepository interface {
	// Schema Config operations
	GetSchemaConfigByID(ctx context.Context, id int64) (*database.DatabaseSchemaConfig, error)
	GetSchemaConfigByName(ctx context.Context, name string) (*database.DatabaseSchemaConfig, error)
	GetAllSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error)
	GetAllEnabledSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error)
	UpsertSchemaConfig(ctx context.Context, config *database.DatabaseSchemaConfig) error
	DeleteSchemaConfig(ctx context.Context, id int64) error
	
	// Phase 2: Enhanced operations with business logic
	UpsertSchemaConfigWithValidation(ctx context.Context, config *database.DatabaseSchemaConfig) error
	DeleteSchemaConfigWithDependencyCheck(ctx context.Context, id int64) error

	// Processor Config operations
	GetProcessorConfigByID(ctx context.Context, id int64) (*database.ProcessorMappingConfig, error)
	GetProcessorConfigByName(ctx context.Context, name string) (*database.ProcessorMappingConfig, error)
	GetAllProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error)
	GetAllEnabledProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error)
	UpsertProcessorConfig(ctx context.Context, config *database.ProcessorMappingConfig) error
	DeleteProcessorConfig(ctx context.Context, id int64) error

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
	ValidateConfig(ctx context.Context, configType string, config interface{}) error
	EnsureTablesAndDefaults(ctx context.Context) error
	
	// Phase 2: Business logic operations
	ValidateConfigDependencies(ctx context.Context, configType string, config interface{}) error
	ExecuteAtomicConfigUpdate(ctx context.Context, operations []ConfigOperation) error
	GetConfigVersion(ctx context.Context, configType string, id int64) (int64, error)
	ValidateConfigIntegrity(ctx context.Context) (*ConfigIntegrityReport, error)
}

// configRepository implements ConfigRepository
type configRepository struct {
	client         OtelConfigDatabaseClient
	schemaRouter   DatabaseRoutingSchemaInterface
	changeHandlers []ConfigChangeHandler
}

// NewConfigRepository creates a new config repository
func NewConfigRepository(client OtelConfigDatabaseClient) ConfigRepository {
	return &configRepository{
		client: client,
	}
}

// NewConfigRepositoryWithRouter creates a new config repository with schema router
func NewConfigRepositoryWithRouter(client OtelConfigDatabaseClient, schemaRouter DatabaseRoutingSchemaInterface) ConfigRepository {
	return &configRepository{
		client:       client,
		schemaRouter: schemaRouter,
	}
}

// AddChangeHandler adds a configuration change handler
func (r *configRepository) AddChangeHandler(handler ConfigChangeHandler) {
	r.changeHandlers = append(r.changeHandlers, handler)
}

// RemoveChangeHandler removes a configuration change handler
func (r *configRepository) RemoveChangeHandler(handler ConfigChangeHandler) {
	for i, h := range r.changeHandlers {
		if h == handler {
			r.changeHandlers = append(r.changeHandlers[:i], r.changeHandlers[i+1:]...)
			break
		}
	}
}

// notifyConfigChange sends configuration change notifications to all handlers
func (r *configRepository) notifyConfigChange(notification ConfigChangeNotification) {
	for _, handler := range r.changeHandlers {
		go func(h ConfigChangeHandler) {
			if err := h.OnConfigChanged(notification); err != nil {
				// Log error but don't fail the operation
				// In production, you might want to use a proper logger
				fmt.Printf("Config change notification failed: %v\n", err)
			}
		}(handler)
	}
}

// createBusinessLogicError creates a comprehensive business logic error
func (r *configRepository) createBusinessLogicError(operation, configType string, configID *int64, errorType string, message string, cause error) *BusinessLogicError {
	return &BusinessLogicError{
		Operation:  operation,
		ConfigType: configType,
		ConfigID:   configID,
		ErrorType:  errorType,
		Message:    message,
		Cause:      cause,
		Timestamp:  time.Now(),
	}
}

// =========================
// Schema Config Operations
// =========================

func (r *configRepository) GetSchemaConfigByID(ctx context.Context, id int64) (*database.DatabaseSchemaConfig, error) {
	return r.client.GetSchemaConfigByID(ctx, id)
}

func (r *configRepository) GetSchemaConfigByName(ctx context.Context, name string) (*database.DatabaseSchemaConfig, error) {
	return r.client.GetSchemaConfigByName(ctx, name)
}

func (r *configRepository) GetAllSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error) {
	return r.client.GetAllSchemaConfigs(ctx)
}

func (r *configRepository) GetAllEnabledSchemaConfigs(ctx context.Context) ([]*database.DatabaseSchemaConfig, error) {
	return r.client.GetAllEnabledSchemaConfigs(ctx)
}

func (r *configRepository) UpsertSchemaConfig(ctx context.Context, config *database.DatabaseSchemaConfig) error {
	if err := ValidateSchemaConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if err := r.client.UpsertSchemaConfig(ctx, config); err != nil {
		return fmt.Errorf("failed to upsert schema config: %w", err)
	}

	// Reset schema router to reload configuration after schema config changes
	if r.schemaRouter != nil {
		if err := r.schemaRouter.Reset(ctx); err != nil {
			return fmt.Errorf("failed to reset schema router: %w", err)
		}
	}

	return nil
}

func (r *configRepository) DeleteSchemaConfig(ctx context.Context, id int64) error {
	if err := r.client.DeleteSchemaConfig(ctx, id); err != nil {
		return fmt.Errorf("failed to delete schema config: %w", err)
	}

	// Reset schema router to reload configuration after schema config deletion
	if r.schemaRouter != nil {
		if err := r.schemaRouter.Reset(ctx); err != nil {
			return fmt.Errorf("failed to reset schema router: %w", err)
		}
	}

	return nil
}

// =========================
// Processor Config Operations
// =========================

func (r *configRepository) GetProcessorConfigByID(ctx context.Context, id int64) (*database.ProcessorMappingConfig, error) {
	return r.client.GetProcessorConfigByID(ctx, id)
}

func (r *configRepository) GetProcessorConfigByName(ctx context.Context, name string) (*database.ProcessorMappingConfig, error) {
	return r.client.GetProcessorConfigByName(ctx, name)
}

func (r *configRepository) GetAllProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error) {
	return r.client.GetAllProcessorConfigs(ctx)
}

func (r *configRepository) GetAllEnabledProcessorConfigs(ctx context.Context) ([]*database.ProcessorMappingConfig, error) {
	return r.client.GetAllEnabledProcessorConfigs(ctx)
}

func (r *configRepository) UpsertProcessorConfig(ctx context.Context, config *database.ProcessorMappingConfig) error {
	if err := ValidateProcessorConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.client.UpsertProcessorConfig(ctx, config)
}

func (r *configRepository) DeleteProcessorConfig(ctx context.Context, id int64) error {
	return r.client.DeleteProcessorConfig(ctx, id)
}

// =========================
// Embedding Config Operations
// =========================

func (r *configRepository) GetEmbeddingConfigByID(ctx context.Context, id int64) (*database.EmbeddingConfig, error) {
	return r.client.GetEmbeddingConfigByID(ctx, id)
}

func (r *configRepository) GetEmbeddingConfigByName(ctx context.Context, name string) (*database.EmbeddingConfig, error) {
	return r.client.GetEmbeddingConfigByName(ctx, name)
}

func (r *configRepository) GetAllEmbeddingConfigs(ctx context.Context) ([]*database.EmbeddingConfig, error) {
	return r.client.GetAllEmbeddingConfigs(ctx)
}

func (r *configRepository) UpsertEmbeddingConfig(ctx context.Context, config *database.EmbeddingConfig) error {
	if err := ValidateEmbeddingConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.client.UpsertEmbeddingConfig(ctx, config)
}

func (r *configRepository) DeleteEmbeddingConfig(ctx context.Context, id int64) error {
	return r.client.DeleteEmbeddingConfig(ctx, id)
}

// =========================
// Template Config Operations
// =========================

func (r *configRepository) GetTemplateConfigByID(ctx context.Context, id int64) (*database.EmbeddingConfigTemplate, error) {
	return r.client.GetTemplateConfigByID(ctx, id)
}

func (r *configRepository) GetTemplateConfigByName(ctx context.Context, name string) (*database.EmbeddingConfigTemplate, error) {
	return r.client.GetTemplateConfigByName(ctx, name)
}

func (r *configRepository) GetAllTemplateConfigs(ctx context.Context) ([]*database.EmbeddingConfigTemplate, error) {
	return r.client.GetAllTemplateConfigs(ctx)
}

func (r *configRepository) UpsertTemplateConfig(ctx context.Context, config *database.EmbeddingConfigTemplate) error {
	if err := ValidateTemplateConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.client.UpsertTemplateConfig(ctx, config)
}

func (r *configRepository) DeleteTemplateConfig(ctx context.Context, id int64) error {
	return r.client.DeleteTemplateConfig(ctx, id)
}

// =========================
// Vector Store Config Operations
// =========================

func (r *configRepository) GetVectorStoreConfigByID(ctx context.Context, id int64) (*database.VectorStoreConfig, error) {
	return r.client.GetVectorStoreConfigByID(ctx, id)
}

func (r *configRepository) GetVectorStoreConfigByName(ctx context.Context, name string) (*database.VectorStoreConfig, error) {
	return r.client.GetVectorStoreConfigByName(ctx, name)
}

func (r *configRepository) GetAllVectorStoreConfigs(ctx context.Context) ([]*database.VectorStoreConfig, error) {
	return r.client.GetAllVectorStoreConfigs(ctx)
}

func (r *configRepository) UpsertVectorStoreConfig(ctx context.Context, config *database.VectorStoreConfig) error {
	if err := ValidateVectorStoreConfig(config); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return r.client.UpsertVectorStoreConfig(ctx, config)
}

func (r *configRepository) DeleteVectorStoreConfig(ctx context.Context, id int64) error {
	return r.client.DeleteVectorStoreConfig(ctx, id)
}

// =========================
// Utility Operations
// =========================

func (r *configRepository) GetConfigStatistics(ctx context.Context) (*database.ConfigStatistics, error) {
	return r.client.GetConfigStatistics(ctx)
}

func (r *configRepository) ValidateConfig(ctx context.Context, configType string, config interface{}) error {
	switch configType {
	case "schema":
		if schemaConfig, ok := config.(*database.DatabaseSchemaConfig); ok {
			return ValidateSchemaConfig(schemaConfig)
		}
	case "processor":
		if processorConfig, ok := config.(*database.ProcessorMappingConfig); ok {
			return ValidateProcessorConfig(processorConfig)
		}
	case "embedding":
		if embeddingConfig, ok := config.(*database.EmbeddingConfig); ok {
			return ValidateEmbeddingConfig(embeddingConfig)
		}
	case "template":
		if templateConfig, ok := config.(*database.EmbeddingConfigTemplate); ok {
			return ValidateTemplateConfig(templateConfig)
		}
	case "vectorstore":
		if vectorStoreConfig, ok := config.(*database.VectorStoreConfig); ok {
			return ValidateVectorStoreConfig(vectorStoreConfig)
		}
	}
	return fmt.Errorf("unsupported config type: %s", configType)
}

func (r *configRepository) EnsureTablesAndDefaults(ctx context.Context) error {
	return r.client.EnsureTablesAndDefaults(ctx)
}

// =========================
// Phase 2: Business Logic Operations
// =========================

// UpsertSchemaConfigWithValidation performs enhanced validation before upserting schema config
func (r *configRepository) UpsertSchemaConfigWithValidation(ctx context.Context, config *database.DatabaseSchemaConfig) error {
	// Capture old version for change notification
	var oldVersion *int64
	var eventType string = "created"
	if config.ID > 0 {
		if oldConfig, err := r.client.GetSchemaConfigByID(ctx, config.ID); err == nil {
			oldVersionVal := oldConfig.UpdatedAt.Unix()
			oldVersion = &oldVersionVal
			eventType = "updated"
		}
	}
	
	// Validate configuration structure
	if err := ValidateSchemaConfig(config); err != nil {
		bizErr := r.createBusinessLogicError("upsert", "schema", &config.ID, "validation", "Schema config validation failed", err)
		return bizErr
	}
	
	// Validate dependencies
	if err := r.ValidateConfigDependencies(ctx, "schema", config); err != nil {
		bizErr := r.createBusinessLogicError("upsert", "schema", &config.ID, "dependency", "Dependency validation failed", err)
		return bizErr
	}
	
	// Perform upsert with schema router reset
	if err := r.UpsertSchemaConfig(ctx, config); err != nil {
		bizErr := r.createBusinessLogicError("upsert", "schema", &config.ID, "persistence", "Failed to persist schema config", err)
		return bizErr
	}
	
	// Send change notification
	newVersion := time.Now().Unix()
	notification := ConfigChangeNotification{
		EventType:  eventType,
		ConfigType: "schema",
		ConfigID:   config.ID,
		ConfigName: config.Name,
		OldVersion: oldVersion,
		NewVersion: &newVersion,
		ChangedBy:  "system", // In production, get from context
		Timestamp:  time.Now(),
	}
	r.notifyConfigChange(notification)
	
	return nil
}

// UpsertProcessorConfigWithValidation performs enhanced validation before upserting processor config
func (r *configRepository) UpsertProcessorConfigWithValidation(ctx context.Context, config *database.ProcessorMappingConfig) error {
	// Validate configuration structure
	if err := ValidateProcessorConfig(config); err != nil {
		return fmt.Errorf("processor config validation failed: %w", err)
	}
	
	// Validate dependencies
	if err := r.ValidateConfigDependencies(ctx, "processor", config); err != nil {
		return fmt.Errorf("dependency validation failed: %w", err)
	}
	
	// Perform upsert
	return r.UpsertProcessorConfig(ctx, config)
}

// UpsertEmbeddingConfigWithValidation performs enhanced validation before upserting embedding config
func (r *configRepository) UpsertEmbeddingConfigWithValidation(ctx context.Context, config *database.EmbeddingConfig) error {
	// Validate configuration structure
	if err := ValidateEmbeddingConfig(config); err != nil {
		return fmt.Errorf("embedding config validation failed: %w", err)
	}
	
	// Validate dependencies
	if err := r.ValidateConfigDependencies(ctx, "embedding", config); err != nil {
		return fmt.Errorf("dependency validation failed: %w", err)
	}
	
	// Perform upsert
	return r.UpsertEmbeddingConfig(ctx, config)
}

// UpsertTemplateConfigWithValidation performs enhanced validation before upserting template config
func (r *configRepository) UpsertTemplateConfigWithValidation(ctx context.Context, config *database.EmbeddingConfigTemplate) error {
	// Validate configuration structure
	if err := ValidateTemplateConfig(config); err != nil {
		return fmt.Errorf("template config validation failed: %w", err)
	}
	
	// Validate dependencies
	if err := r.ValidateConfigDependencies(ctx, "template", config); err != nil {
		return fmt.Errorf("dependency validation failed: %w", err)
	}
	
	// Perform upsert
	return r.UpsertTemplateConfig(ctx, config)
}

// UpsertVectorStoreConfigWithValidation performs enhanced validation before upserting vector store config
func (r *configRepository) UpsertVectorStoreConfigWithValidation(ctx context.Context, config *database.VectorStoreConfig) error {
	// Validate configuration structure
	if err := ValidateVectorStoreConfig(config); err != nil {
		return fmt.Errorf("vector store config validation failed: %w", err)
	}
	
	// Validate dependencies
	if err := r.ValidateConfigDependencies(ctx, "vectorstore", config); err != nil {
		return fmt.Errorf("dependency validation failed: %w", err)
	}
	
	// Perform upsert
	return r.UpsertVectorStoreConfig(ctx, config)
}

// DeleteSchemaConfigWithDependencyCheck checks for dependencies before deleting schema config
func (r *configRepository) DeleteSchemaConfigWithDependencyCheck(ctx context.Context, id int64) error {
	// Check if any processor configs depend on this schema
	processors, err := r.client.GetProcessorConfigsBySchemaID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to check processor dependencies: %w", err)
	}
	
	if len(processors) > 0 {
		return fmt.Errorf("cannot delete schema config: %d processor configs depend on it", len(processors))
	}
	
	// Perform deletion with schema router reset
	return r.DeleteSchemaConfig(ctx, id)
}

// DeleteEmbeddingConfigWithDependencyCheck checks for dependencies before deleting embedding config
func (r *configRepository) DeleteEmbeddingConfigWithDependencyCheck(ctx context.Context, id int64) error {
	// Check if any vector store configs depend on this embedding
	vectorStores, err := r.client.GetAllVectorStoreConfigs(ctx)
	if err != nil {
		return fmt.Errorf("failed to check vector store dependencies: %w", err)
	}
	
	var dependentStores []*database.VectorStoreConfig
	for _, vs := range vectorStores {
		if vs.EmbeddingID == id {
			dependentStores = append(dependentStores, vs)
		}
	}
	
	if len(dependentStores) > 0 {
		return fmt.Errorf("cannot delete embedding config: %d vector store configs depend on it", len(dependentStores))
	}
	
	// Perform deletion
	return r.DeleteEmbeddingConfig(ctx, id)
}

// DeleteProcessorConfigWithDependencyCheck checks for dependencies before deleting processor config
func (r *configRepository) DeleteProcessorConfigWithDependencyCheck(ctx context.Context, id int64) error {
	// Processor configs typically don't have dependents, but we can add future checks here
	// For now, just perform the deletion
	return r.DeleteProcessorConfig(ctx, id)
}

// DeleteTemplateConfigWithDependencyCheck checks for dependencies before deleting template config
func (r *configRepository) DeleteTemplateConfigWithDependencyCheck(ctx context.Context, id int64) error {
	// Template configs typically don't have dependents, but we can add future checks here
	// For now, just perform the deletion
	return r.DeleteTemplateConfig(ctx, id)
}

// DeleteVectorStoreConfigWithDependencyCheck checks for dependencies before deleting vector store config
func (r *configRepository) DeleteVectorStoreConfigWithDependencyCheck(ctx context.Context, id int64) error {
	// Vector store configs typically don't have dependents, but we can add future checks here
	// For now, just perform the deletion
	return r.DeleteVectorStoreConfig(ctx, id)
}

// ValidateConfigDependencies validates that all referenced configurations exist
func (r *configRepository) ValidateConfigDependencies(ctx context.Context, configType string, config interface{}) error {
	switch configType {
	case "schema":
		// Schema configs have no dependencies
		return nil
	case "processor":
		if processorConfig, ok := config.(*database.ProcessorMappingConfig); ok {
			// Validate that referenced schema exists
			_, err := r.client.GetSchemaConfigByID(ctx, processorConfig.DatabaseID)
			if err != nil {
				return fmt.Errorf("referenced schema ID %d does not exist: %w", processorConfig.DatabaseID, err)
			}
		}
	case "embedding":
		// Embedding configs have no dependencies
		return nil
	case "template":
		// Template configs have no dependencies
		return nil
	case "vectorstore":
		if vectorStoreConfig, ok := config.(*database.VectorStoreConfig); ok {
			// Validate that referenced embedding config exists
			_, err := r.client.GetEmbeddingConfigByID(ctx, vectorStoreConfig.EmbeddingID)
			if err != nil {
				return fmt.Errorf("referenced embedding ID %d does not exist: %w", vectorStoreConfig.EmbeddingID, err)
			}
		}
	default:
		return fmt.Errorf("unsupported config type for dependency validation: %s", configType)
	}
	return nil
}

// ExecuteAtomicConfigUpdate executes multiple configuration operations atomically
func (r *configRepository) ExecuteAtomicConfigUpdate(ctx context.Context, operations []ConfigOperation) error {
	// Pre-validate all operations before executing any
	if err := r.preValidateOperations(ctx, operations); err != nil {
		return fmt.Errorf("pre-validation failed: %w", err)
	}
	
	// Store original states for rollback
	originalStates := make(map[string]interface{})
	
	// Execute operations sequentially with state tracking
	for i, op := range operations {
		// Store original state before modification
		if op.Type == "update" || op.Type == "delete" {
			originalState, err := r.captureConfigState(ctx, op.ConfigType, op.ID)
			if err != nil {
				// Rollback previous operations
				r.rollbackToOriginalStates(ctx, originalStates)
				return fmt.Errorf("failed to capture state for operation %d: %w", i, err)
			}
			stateKey := fmt.Sprintf("%s_%d", op.ConfigType, op.ID)
			originalStates[stateKey] = originalState
		}
		
		// Execute the operation
		if err := r.executeConfigOperationWithValidation(ctx, op); err != nil {
			// Rollback all operations to original states
			r.rollbackToOriginalStates(ctx, originalStates)
			return fmt.Errorf("atomic update failed at operation %d (%s/%s): %w", i, op.Type, op.ConfigType, err)
		}
	}
	
	return nil
}

// preValidateOperations validates all operations before executing any
func (r *configRepository) preValidateOperations(ctx context.Context, operations []ConfigOperation) error {
	for i, op := range operations {
		// Validate operation structure
		if err := r.validateOperationStructure(op); err != nil {
			return fmt.Errorf("operation %d structure invalid: %w", i, err)
		}
		
		// For create/update operations, validate the config
		if (op.Type == "create" || op.Type == "update") && op.Config != nil {
			if err := r.validateConfigByType(ctx, op.ConfigType, op.Config); err != nil {
				return fmt.Errorf("operation %d config validation failed: %w", i, err)
			}
			
			// Validate dependencies
			if err := r.ValidateConfigDependencies(ctx, op.ConfigType, op.Config); err != nil {
				return fmt.Errorf("operation %d dependency validation failed: %w", i, err)
			}
		}
		
		// For delete operations, check if config exists and has no dependents
		if op.Type == "delete" {
			if err := r.validateDeleteOperation(ctx, op.ConfigType, op.ID); err != nil {
				return fmt.Errorf("operation %d delete validation failed: %w", i, err)
			}
		}
	}
	return nil
}

// validateOperationStructure validates the structure of a configuration operation
func (r *configRepository) validateOperationStructure(op ConfigOperation) error {
	if op.Type == "" {
		return fmt.Errorf("operation type cannot be empty")
	}
	if op.ConfigType == "" {
		return fmt.Errorf("config type cannot be empty")
	}
	if op.Type != "create" && op.Type != "update" && op.Type != "delete" {
		return fmt.Errorf("invalid operation type: %s", op.Type)
	}
	if op.Type == "delete" && op.ID <= 0 {
		return fmt.Errorf("delete operation requires valid ID")
	}
	if (op.Type == "create" || op.Type == "update") && op.Config == nil {
		return fmt.Errorf("%s operation requires config data", op.Type)
	}
	return nil
}

// validateConfigByType validates configuration using the appropriate validation function
func (r *configRepository) validateConfigByType(ctx context.Context, configType string, config interface{}) error {
	return r.ValidateConfig(ctx, configType, config)
}

// validateDeleteOperation validates that a delete operation can be performed
func (r *configRepository) validateDeleteOperation(ctx context.Context, configType string, id int64) error {
	switch configType {
	case "schema":
		// Check if any processor configs depend on this schema
		processors, err := r.client.GetProcessorConfigsBySchemaID(ctx, id)
		if err != nil {
			return fmt.Errorf("failed to check processor dependencies: %w", err)
		}
		if len(processors) > 0 {
			return fmt.Errorf("cannot delete schema: %d processor configs depend on it", len(processors))
		}
	case "embedding":
		// Check if any vector store configs depend on this embedding
		vectorStores, err := r.client.GetAllVectorStoreConfigs(ctx)
		if err != nil {
			return fmt.Errorf("failed to check vector store dependencies: %w", err)
		}
		for _, vs := range vectorStores {
			if vs.EmbeddingID == id {
				return fmt.Errorf("cannot delete embedding: vector store config %d depends on it", vs.ID)
			}
		}
	case "processor", "template", "vectorstore":
		// These typically don't have dependents, but verify they exist
		if _, err := r.getConfigByID(ctx, configType, id); err != nil {
			return fmt.Errorf("config does not exist: %w", err)
		}
	default:
		return fmt.Errorf("unsupported config type for delete validation: %s", configType)
	}
	return nil
}

// captureConfigState captures the current state of a configuration for rollback
func (r *configRepository) captureConfigState(ctx context.Context, configType string, id int64) (interface{}, error) {
	return r.getConfigByID(ctx, configType, id)
}

// getConfigByID retrieves a configuration by type and ID
func (r *configRepository) getConfigByID(ctx context.Context, configType string, id int64) (interface{}, error) {
	switch configType {
	case "schema":
		return r.client.GetSchemaConfigByID(ctx, id)
	case "processor":
		return r.client.GetProcessorConfigByID(ctx, id)
	case "embedding":
		return r.client.GetEmbeddingConfigByID(ctx, id)
	case "template":
		return r.client.GetTemplateConfigByID(ctx, id)
	case "vectorstore":
		return r.client.GetVectorStoreConfigByID(ctx, id)
	default:
		return nil, fmt.Errorf("unsupported config type: %s", configType)
	}
}

// executeConfigOperationWithValidation executes a config operation with validation
func (r *configRepository) executeConfigOperationWithValidation(ctx context.Context, op ConfigOperation) error {
	switch op.Type {
	case "create", "update":
		return r.executeUpsertOperationWithValidation(ctx, op)
	case "delete":
		return r.executeDeleteOperation(ctx, op)
	default:
		return fmt.Errorf("unsupported operation type: %s", op.Type)
	}
}

// executeUpsertOperationWithValidation executes upsert with validation
func (r *configRepository) executeUpsertOperationWithValidation(ctx context.Context, op ConfigOperation) error {
	switch op.ConfigType {
	case "schema":
		if config, ok := op.Config.(*database.DatabaseSchemaConfig); ok {
			return r.UpsertSchemaConfigWithValidation(ctx, config)
		}
	case "processor":
		if config, ok := op.Config.(*database.ProcessorMappingConfig); ok {
			return r.UpsertProcessorConfigWithValidation(ctx, config)
		}
	case "embedding":
		if config, ok := op.Config.(*database.EmbeddingConfig); ok {
			return r.UpsertEmbeddingConfigWithValidation(ctx, config)
		}
	case "template":
		if config, ok := op.Config.(*database.EmbeddingConfigTemplate); ok {
			return r.UpsertTemplateConfigWithValidation(ctx, config)
		}
	case "vectorstore":
		if config, ok := op.Config.(*database.VectorStoreConfig); ok {
			return r.UpsertVectorStoreConfigWithValidation(ctx, config)
		}
	}
	return fmt.Errorf("invalid config type for upsert: %s", op.ConfigType)
}

func (r *configRepository) executeDeleteOperation(ctx context.Context, op ConfigOperation) error {
	switch op.ConfigType {
	case "schema":
		return r.DeleteSchemaConfig(ctx, op.ID)
	case "processor":
		return r.DeleteProcessorConfig(ctx, op.ID)
	case "embedding":
		return r.DeleteEmbeddingConfig(ctx, op.ID)
	case "template":
		return r.DeleteTemplateConfig(ctx, op.ID)
	case "vectorstore":
		return r.DeleteVectorStoreConfig(ctx, op.ID)
	}
	return fmt.Errorf("invalid config type for delete: %s", op.ConfigType)
}

// rollbackToOriginalStates restores configurations to their original states
func (r *configRepository) rollbackToOriginalStates(ctx context.Context, originalStates map[string]interface{}) {
	for stateKey, originalState := range originalStates {
		// Parse the state key to get config type and ID
		parts := strings.Split(stateKey, "_")
		if len(parts) != 2 {
			continue
		}
		configType := parts[0]
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		
		// Restore the original state
		r.restoreConfigState(ctx, configType, id, originalState)
	}
}

// restoreConfigState restores a configuration to its original state
func (r *configRepository) restoreConfigState(ctx context.Context, configType string, id int64, originalState interface{}) {
	// This is a simplified rollback - in production you'd want more sophisticated rollback logic
	switch configType {
	case "schema":
		if config, ok := originalState.(*database.DatabaseSchemaConfig); ok {
			r.UpsertSchemaConfig(ctx, config)
		}
	case "processor":
		if config, ok := originalState.(*database.ProcessorMappingConfig); ok {
			r.UpsertProcessorConfig(ctx, config)
		}
	case "embedding":
		if config, ok := originalState.(*database.EmbeddingConfig); ok {
			r.UpsertEmbeddingConfig(ctx, config)
		}
	case "template":
		if config, ok := originalState.(*database.EmbeddingConfigTemplate); ok {
			r.UpsertTemplateConfig(ctx, config)
		}
	case "vectorstore":
		if config, ok := originalState.(*database.VectorStoreConfig); ok {
			r.UpsertVectorStoreConfig(ctx, config)
		}
	}
}

// GetConfigVersion returns the version (UpdatedAt timestamp) of a configuration
func (r *configRepository) GetConfigVersion(ctx context.Context, configType string, id int64) (int64, error) {
	switch configType {
	case "schema":
		config, err := r.client.GetSchemaConfigByID(ctx, id)
		if err != nil {
			return 0, err
		}
		return config.UpdatedAt.Unix(), nil
	case "processor":
		config, err := r.client.GetProcessorConfigByID(ctx, id)
		if err != nil {
			return 0, err
		}
		return config.UpdatedAt.Unix(), nil
	case "embedding":
		config, err := r.client.GetEmbeddingConfigByID(ctx, id)
		if err != nil {
			return 0, err
		}
		return config.UpdatedAt.Unix(), nil
	case "template":
		config, err := r.client.GetTemplateConfigByID(ctx, id)
		if err != nil {
			return 0, err
		}
		return config.UpdatedAt.Unix(), nil
	case "vectorstore":
		config, err := r.client.GetVectorStoreConfigByID(ctx, id)
		if err != nil {
			return 0, err
		}
		return config.UpdatedAt.Unix(), nil
	}
	return 0, fmt.Errorf("unsupported config type: %s", configType)
}

// ValidateConfigIntegrity performs comprehensive integrity validation across all configurations
func (r *configRepository) ValidateConfigIntegrity(ctx context.Context) (*ConfigIntegrityReport, error) {
	report := &ConfigIntegrityReport{
		Valid:       true,
		GeneratedAt: time.Now(),
	}
	
	// Validate processor -> schema dependencies
	processors, err := r.client.GetAllProcessorConfigs(ctx)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Failed to get processor configs: %v", err))
		report.Valid = false
	} else {
		for _, processor := range processors {
			_, err := r.client.GetSchemaConfigByID(ctx, processor.DatabaseID)
			if err != nil {
				report.MissingDependencies = append(report.MissingDependencies, MissingDependency{
					ConfigType:     "processor",
					ConfigID:       processor.ID,
					DependencyType: "schema",
					DependencyID:   processor.DatabaseID,
				})
				report.Valid = false
			}
		}
	}
	
	// Validate vectorstore -> embedding dependencies
	vectorStores, err := r.client.GetAllVectorStoreConfigs(ctx)
	if err != nil {
		report.Errors = append(report.Errors, fmt.Sprintf("Failed to get vector store configs: %v", err))
		report.Valid = false
	} else {
		for _, vs := range vectorStores {
			_, err := r.client.GetEmbeddingConfigByID(ctx, vs.EmbeddingID)
			if err != nil {
				report.MissingDependencies = append(report.MissingDependencies, MissingDependency{
					ConfigType:     "vectorstore",
					ConfigID:       vs.ID,
					DependencyType: "embedding",
					DependencyID:   vs.EmbeddingID,
				})
				report.Valid = false
			}
		}
	}
	
	return report, nil
}
