package internal

import (
	"context"
	"fmt"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository"
)

// ConfigService provides business logic for configuration management
type ConfigService struct {
	serviceManager repository.ServiceResourceManagerInterface
}

// NewConfigService creates a new ConfigService instance
func NewConfigService(serviceManager repository.ServiceResourceManagerInterface) *ConfigService {
	return &ConfigService{
		serviceManager: serviceManager,
	}
}

// ConfigServiceResponse represents a unified response from the service layer
type ConfigServiceResponse struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Enabled     bool                   `json:"enabled"`
	Description string                 `json:"description"`
	Priority    int                    `json:"priority"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
	Config      map[string]interface{} `json:"config"`
}

// ConfigCreateServiceRequest represents a request to create a configuration
type ConfigCreateServiceRequest struct {
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`
	Enabled     bool                   `json:"enabled"`
	Description string                 `json:"description"`
	Priority    int                    `json:"priority"`
	Metadata    map[string]interface{} `json:"metadata"`
	Config      map[string]interface{} `json:"config"`
}

// ConfigStatisticsServiceResponse represents statistics from the service layer
type ConfigStatisticsServiceResponse struct {
	SchemaConfigs      int64 `json:"schema_configs"`
	ProcessorConfigs   int64 `json:"processor_configs"`
	EmbeddingConfigs   int64 `json:"embedding_configs"`
	TemplateConfigs    int64 `json:"template_configs"`
	VectorStoreConfigs int64 `json:"vectorstore_configs"`
	TotalConfigs       int64 `json:"total_configs"`
}

// getConfigRepository creates a ConfigRepository instance from the service manager
func (s *ConfigService) getConfigRepository(ctx context.Context) (repository.ConfigRepository, error) {
	db, err := s.serviceManager.GetDatabase()
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	routingSchema, err := s.serviceManager.GetRoutingSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to get routing schema: %w", err)
	}

	// Create the database client first
	dbClient, err := repository.NewOtelSQLConfigDatabaseClient(ctx, db, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create database client: %w", err)
	}

	return repository.NewConfigRepositoryWithRouter(dbClient, routingSchema), nil
}

// ListConfigurations returns all configurations of a specific type
func (s *ConfigService) ListConfigurations(ctx context.Context, configType string) ([]ConfigServiceResponse, error) {
	configRepo, err := s.getConfigRepository(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config repository: %w", err)
	}

	var configs []ConfigServiceResponse

	switch configType {
	case "schemas":
		schemaConfigs, err := configRepo.GetAllSchemaConfigs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema configs: %w", err)
		}
		for _, config := range schemaConfigs {
			configs = append(configs, ConfigServiceResponse{
				ID:          config.ID,
				Name:        config.Name,
				Type:        "schemas",
				Enabled:     config.Enabled,
				Description: config.Description,
				CreatedAt:   config.CreatedAt,
				UpdatedAt:   config.UpdatedAt,
			})
		}
	case "processors":
		processorConfigs, err := configRepo.GetAllProcessorConfigs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get processor configs: %w", err)
		}
		for _, config := range processorConfigs {
			configs = append(configs, ConfigServiceResponse{
				ID:          config.ID,
				Name:        config.Name,
				Type:        "processors",
				Enabled:     config.Enabled,
				Description: "", // ProcessorMappingConfig doesn't have Description field
				CreatedAt:   config.CreatedAt,
				UpdatedAt:   config.UpdatedAt,
			})
		}
	case "embeddings":
		embeddingConfigs, err := configRepo.GetAllEmbeddingConfigs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding configs: %w", err)
		}
		for _, config := range embeddingConfigs {
			configs = append(configs, ConfigServiceResponse{
				ID:          config.ID,
				Name:        config.Name,
				Type:        "embeddings",
				Enabled:     true, // EmbeddingConfig doesn't have Enabled field, default to true
				Description: "",   // EmbeddingConfig doesn't have Description field
				CreatedAt:   config.CreatedAt,
				UpdatedAt:   config.UpdatedAt,
			})
		}
	case "templates":
		templateConfigs, err := configRepo.GetAllTemplateConfigs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get template configs: %w", err)
		}
		for _, config := range templateConfigs {
			configs = append(configs, ConfigServiceResponse{
				ID:          config.ID,
				Name:        config.Name,
				Type:        "templates",
				Enabled:     true, // EmbeddingConfigTemplate doesn't have Enabled field, default to true
				Description: config.Description,
				CreatedAt:   config.CreatedAt,
				UpdatedAt:   config.UpdatedAt,
			})
		}
	case "vectorstores":
		vectorStoreConfigs, err := configRepo.GetAllVectorStoreConfigs(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get vector store configs: %w", err)
		}
		for _, config := range vectorStoreConfigs {
			configs = append(configs, ConfigServiceResponse{
				ID:          config.ID,
				Name:        config.Name,
				Type:        "vectorstores",
				Enabled:     true, // VectorStoreConfig doesn't have Enabled field, default to true
				Description: "",   // VectorStoreConfig doesn't have Description field
				CreatedAt:   config.CreatedAt,
				UpdatedAt:   config.UpdatedAt,
			})
		}
	default:
		return nil, fmt.Errorf("unsupported config type: %s", configType)
	}

	return configs, nil
}

// GetConfiguration returns a specific configuration by ID and type
func (s *ConfigService) GetConfiguration(ctx context.Context, configType string, id int64) (*ConfigServiceResponse, error) {
	configRepo, err := s.getConfigRepository(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config repository: %w", err)
	}

	switch configType {
	case "schemas":
		config, err := configRepo.GetSchemaConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get schema config: %w", err)
		}
		if config == nil {
			return nil, fmt.Errorf("configuration not found")
		}
		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        "schemas",
			Enabled:     config.Enabled,
			Description: config.Description,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
		}, nil
	case "processors":
		config, err := configRepo.GetProcessorConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get processor config: %w", err)
		}
		if config == nil {
			return nil, fmt.Errorf("configuration not found")
		}
		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        "processors",
			Enabled:     config.Enabled,
			Description: "",
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
		}, nil
	case "embeddings":
		config, err := configRepo.GetEmbeddingConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding config: %w", err)
		}
		if config == nil {
			return nil, fmt.Errorf("configuration not found")
		}
		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        "embeddings",
			Enabled:     true,
			Description: "",
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
		}, nil
	case "templates":
		config, err := configRepo.GetTemplateConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get template config: %w", err)
		}
		if config == nil {
			return nil, fmt.Errorf("configuration not found")
		}
		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        "templates",
			Enabled:     true,
			Description: config.Description,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
		}, nil
	case "vectorstores":
		config, err := configRepo.GetVectorStoreConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get vector store config: %w", err)
		}
		if config == nil {
			return nil, fmt.Errorf("configuration not found")
		}
		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        "vectorstores",
			Enabled:     true,
			Description: "",
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported config type: %s", configType)
	}
}

// Helper functions to extract typed values from generic config maps
func extractStringFromConfig(config map[string]interface{}, key, defaultValue string) string {
	if val, ok := config[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

func extractInt64FromConfig(config map[string]interface{}, key string, defaultValue int64) int64 {
	if val, ok := config[key]; ok {
		switch v := val.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v)
		}
	}
	return defaultValue
}

func extractBoolFromConfig(config map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := config[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return defaultValue
}

func extractMapFromConfig(config map[string]interface{}, key string) map[string]interface{} {
	if val, ok := config[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return make(map[string]interface{})
}

func extractMapStringFromConfig(config map[string]interface{}, key string) map[string]string {
	if val, ok := config[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			result := make(map[string]string)
			for k, v := range m {
				if str, ok := v.(string); ok {
					result[k] = str
				}
			}
			return result
		}
	}
	return make(map[string]string)
}

func extractMapStringSliceFromConfig(config map[string]interface{}, key string) map[string][]string {
	if val, ok := config[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			result := make(map[string][]string)
			for k, v := range m {
				if slice, ok := v.([]interface{}); ok {
					strSlice := make([]string, 0, len(slice))
					for _, item := range slice {
						if str, ok := item.(string); ok {
							strSlice = append(strSlice, str)
						}
					}
					result[k] = strSlice
				}
			}
			return result
		}
	}
	return make(map[string][]string)
}

// CreateConfiguration creates a new configuration
func (s *ConfigService) CreateConfiguration(ctx context.Context, req *ConfigCreateServiceRequest) (*ConfigServiceResponse, error) {
	configRepo, err := s.getConfigRepository(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config repository: %w", err)
	}

	switch req.Type {
	case "schemas":
		// Check if config with this name already exists
		existing, err := configRepo.GetSchemaConfigByName(ctx, req.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing schema config: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("schema config with name '%s' already exists", req.Name)
		}

		// Create schema config using repository's default function
		config := repository.DefaultSchemaConfig()
		config.Name = req.Name
		config.Enabled = req.Enabled
		config.Priority = req.Priority
		config.Description = req.Description
		config.DatabaseSchema = extractStringFromConfig(req.Config, "database_schema", "otel")
		config.Routing = extractMapFromConfig(req.Config, "routing")
		config.Metadata = req.Metadata

		// Validate and create
		if err := repository.ValidateSchemaConfig(config); err != nil {
			return nil, fmt.Errorf("schema config validation failed: %w", err)
		}

		if err := configRepo.UpsertSchemaConfig(ctx, config); err != nil {
			return nil, fmt.Errorf("failed to create schema config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        req.Type,
			Enabled:     config.Enabled,
			Description: config.Description,
			Priority:    config.Priority,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
			Metadata:    config.Metadata,
			Config:      req.Config,
		}, nil

	case "processors":
		existing, err := configRepo.GetProcessorConfigByName(ctx, req.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing processor config: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("processor config with name '%s' already exists", req.Name)
		}

		// Create processor config using repository's default function
		config := repository.DefaultProcessorConfig()
		config.Name = req.Name
		config.Enabled = req.Enabled
		config.DatabaseID = extractInt64FromConfig(req.Config, "database_id", 1)
		config.VectorStoreID = extractInt64FromConfig(req.Config, "vectorstore_id", 1)
		config.TemplateID = extractInt64FromConfig(req.Config, "template_id", 1)
		config.ProcessingMode = extractStringFromConfig(req.Config, "processing_mode", "template")
		config.Routing = extractMapFromConfig(req.Config, "routing")
		config.EmbeddingFilter = extractMapFromConfig(req.Config, "filter")
		config.Metadata = req.Metadata

		// Validate and create
		if err := repository.ValidateProcessorConfig(config); err != nil {
			return nil, fmt.Errorf("processor config validation failed: %w", err)
		}

		if err := configRepo.UpsertProcessorConfig(ctx, config); err != nil {
			return nil, fmt.Errorf("failed to create processor config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        req.Type,
			Enabled:     config.Enabled,
			Description: req.Description, // Use description from request since processor config doesn't have this field
			Priority:    req.Priority,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
			Metadata:    config.Metadata,
			Config:      req.Config,
		}, nil

	case "embeddings":
		existing, err := configRepo.GetEmbeddingConfigByName(ctx, req.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing embedding config: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("embedding config with name '%s' already exists", req.Name)
		}

		// Create embedding config using repository's default function
		config := repository.DefaultEmbeddingConfig()
		config.Name = req.Name
		config.Description = req.Description
		config.Provider = extractStringFromConfig(req.Config, "provider", "openai")
		config.Model = extractStringFromConfig(req.Config, "model", "text-embedding-ada-002")
		config.Endpoint = extractStringFromConfig(req.Config, "endpoint", "")
		config.Dimension = int(extractInt64FromConfig(req.Config, "dimension", 1536))
		config.MaxContextLength = int(extractInt64FromConfig(req.Config, "max_context_length", 8191))
		config.RateLimit = int(extractInt64FromConfig(req.Config, "rate_limit", 1000))
		config.Timeout = time.Duration(extractInt64FromConfig(req.Config, "timeout", 30)) * time.Second
		config.Normalize = extractBoolFromConfig(req.Config, "normalize", false)
		config.Headers = extractMapStringFromConfig(req.Config, "headers")
		config.Metadata = req.Metadata

		// Validate and create
		if err := repository.ValidateEmbeddingConfig(config); err != nil {
			return nil, fmt.Errorf("embedding config validation failed: %w", err)
		}

		if err := configRepo.UpsertEmbeddingConfig(ctx, config); err != nil {
			return nil, fmt.Errorf("failed to create embedding config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        req.Type,
			Enabled:     req.Enabled,
			Description: config.Description,
			Priority:    req.Priority,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
			Metadata:    config.Metadata,
			Config:      req.Config,
		}, nil

	case "templates":
		existing, err := configRepo.GetTemplateConfigByName(ctx, req.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing template config: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("template config with name '%s' already exists", req.Name)
		}

		// Create template config using repository's default function
		config := repository.DefaultTemplateConfig()
		config.Name = req.Name
		config.Description = req.Description
		config.Content = extractMapStringFromConfig(req.Config, "content")
		config.TemplateMetadata = extractMapStringSliceFromConfig(req.Config, "template_metadata")
		config.Metadata = req.Metadata

		// Validate and create
		if err := repository.ValidateTemplateConfig(config); err != nil {
			return nil, fmt.Errorf("template config validation failed: %w", err)
		}

		if err := configRepo.UpsertTemplateConfig(ctx, config); err != nil {
			return nil, fmt.Errorf("failed to create template config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        req.Type,
			Enabled:     req.Enabled,
			Description: config.Description,
			Priority:    req.Priority,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
			Metadata:    config.Metadata,
			Config:      req.Config,
		}, nil

	case "vectorstores":
		existing, err := configRepo.GetVectorStoreConfigByName(ctx, req.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing vectorstore config: %w", err)
		}
		if existing != nil {
			return nil, fmt.Errorf("vectorstore config with name '%s' already exists", req.Name)
		}

		// Create vectorstore config using repository's default function
		config := repository.DefaultVectorStoreConfig()
		config.Name = req.Name
		config.Description = req.Description
		config.Provider = extractStringFromConfig(req.Config, "provider", "postgres")
		config.Endpoint = extractStringFromConfig(req.Config, "endpoint", "")
		config.User = extractStringFromConfig(req.Config, "user", "")
		config.Database = extractStringFromConfig(req.Config, "database", "")
		config.EmbeddingID = extractInt64FromConfig(req.Config, "embedding_id", 1)
		config.BatchSize = int(extractInt64FromConfig(req.Config, "batch_size", 100))
		config.FlushInterval = time.Duration(extractInt64FromConfig(req.Config, "flush_interval_ms", 200)) * time.Millisecond
		config.MaxRetries = int(extractInt64FromConfig(req.Config, "max_retries", 3))
		config.MaxOpenConns = int(extractInt64FromConfig(req.Config, "max_open_conns", 25))
		config.MaxIdleConns = int(extractInt64FromConfig(req.Config, "max_idle_conns", 5))
		config.ConnMaxLifetime = time.Duration(extractInt64FromConfig(req.Config, "conn_max_lifetime_ms", 3600000)) * time.Millisecond
		config.BatchErrorPolicy = extractStringFromConfig(req.Config, "batch_error_policy", "retry")
		config.MaxConcurrentBatches = int(extractInt64FromConfig(req.Config, "max_concurrent_batches", 8))
		config.VendorSpecificOptions = extractMapFromConfig(req.Config, "vendor_specific_options")
		config.Metadata = req.Metadata

		// Validate and create
		if err := repository.ValidateVectorStoreConfig(config); err != nil {
			return nil, fmt.Errorf("vectorstore config validation failed: %w", err)
		}

		if err := configRepo.UpsertVectorStoreConfig(ctx, config); err != nil {
			return nil, fmt.Errorf("failed to create vectorstore config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          config.ID,
			Name:        config.Name,
			Type:        req.Type,
			Enabled:     req.Enabled,
			Description: config.Description,
			Priority:    req.Priority,
			CreatedAt:   config.CreatedAt,
			UpdatedAt:   config.UpdatedAt,
			Metadata:    config.Metadata,
			Config:      req.Config,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported config type: %s", req.Type)
	}
}

// UpdateConfiguration updates an existing configuration
func (s *ConfigService) UpdateConfiguration(ctx context.Context, configType string, id int64, req *ConfigCreateServiceRequest) (*ConfigServiceResponse, error) {
	configRepo, err := s.getConfigRepository(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config repository: %w", err)
	}

	switch configType {
	case "schemas":
		// First, get the existing config to ensure it exists
		existing, err := configRepo.GetSchemaConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing schema config: %w", err)
		}
		if existing == nil {
			return nil, fmt.Errorf("schema config with ID %d not found", id)
		}

		// Update schema config using existing config as base
		existing.Name = req.Name
		existing.Enabled = req.Enabled
		existing.Description = req.Description
		existing.DatabaseSchema = extractStringFromConfig(req.Config, "database_schema", existing.DatabaseSchema)
		existing.Priority = req.Priority
		existing.Routing = extractMapFromConfig(req.Config, "routing")
		existing.Metadata = req.Metadata

		// Validate and update
		if err := repository.ValidateSchemaConfig(existing); err != nil {
			return nil, fmt.Errorf("schema config validation failed: %w", err)
		}

		if err := configRepo.UpsertSchemaConfig(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update schema config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          existing.ID,
			Name:        existing.Name,
			Type:        configType,
			Enabled:     req.Enabled,
			Description: existing.Description,
			Priority:    req.Priority,
			CreatedAt:   existing.CreatedAt,
			UpdatedAt:   existing.UpdatedAt,
			Metadata:    existing.Metadata,
			Config:      req.Config,
		}, nil

	case "processors":
		existing, err := configRepo.GetProcessorConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing processor config: %w", err)
		}
		if existing == nil {
			return nil, fmt.Errorf("processor config with ID %d not found", id)
		}

		// Update processor config using existing config as base
		existing.Name = req.Name
		existing.DatabaseID = extractInt64FromConfig(req.Config, "database_id", existing.DatabaseID)
		existing.VectorStoreID = extractInt64FromConfig(req.Config, "vectorstore_id", existing.VectorStoreID)
		existing.TemplateID = extractInt64FromConfig(req.Config, "template_id", existing.TemplateID)
		existing.ProcessingMode = extractStringFromConfig(req.Config, "processing_mode", existing.ProcessingMode)
		existing.Routing = extractMapFromConfig(req.Config, "routing")
		existing.EmbeddingFilter = extractMapFromConfig(req.Config, "filter")
		existing.Metadata = req.Metadata

		// Validate and update
		if err := repository.ValidateProcessorConfig(existing); err != nil {
			return nil, fmt.Errorf("processor config validation failed: %w", err)
		}

		if err := configRepo.UpsertProcessorConfig(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update processor config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          existing.ID,
			Name:        existing.Name,
			Type:        configType,
			Enabled:     req.Enabled,
			Description: req.Description,
			Priority:    req.Priority,
			CreatedAt:   existing.CreatedAt,
			UpdatedAt:   existing.UpdatedAt,
			Metadata:    existing.Metadata,
			Config:      req.Config,
		}, nil

	case "embeddings":
		existing, err := configRepo.GetEmbeddingConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing embedding config: %w", err)
		}
		if existing == nil {
			return nil, fmt.Errorf("embedding config with ID %d not found", id)
		}

		// Update embedding config using existing config as base
		existing.Name = req.Name
		existing.Description = req.Description
		existing.Provider = extractStringFromConfig(req.Config, "provider", existing.Provider)
		existing.Model = extractStringFromConfig(req.Config, "model", existing.Model)
		existing.Endpoint = extractStringFromConfig(req.Config, "endpoint", existing.Endpoint)
		existing.Dimension = int(extractInt64FromConfig(req.Config, "dimension", int64(existing.Dimension)))
		existing.MaxContextLength = int(extractInt64FromConfig(req.Config, "max_context_length", int64(existing.MaxContextLength)))
		existing.RateLimit = int(extractInt64FromConfig(req.Config, "rate_limit", int64(existing.RateLimit)))
		existing.Timeout = time.Duration(extractInt64FromConfig(req.Config, "timeout", int64(existing.Timeout.Seconds()))) * time.Second
		existing.Normalize = extractBoolFromConfig(req.Config, "normalize", existing.Normalize)
		existing.Headers = extractMapStringFromConfig(req.Config, "headers")
		existing.Metadata = req.Metadata

		// Validate and update
		if err := repository.ValidateEmbeddingConfig(existing); err != nil {
			return nil, fmt.Errorf("embedding config validation failed: %w", err)
		}

		if err := configRepo.UpsertEmbeddingConfig(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update embedding config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          existing.ID,
			Name:        existing.Name,
			Type:        configType,
			Enabled:     req.Enabled,
			Description: existing.Description,
			Priority:    req.Priority,
			CreatedAt:   existing.CreatedAt,
			UpdatedAt:   existing.UpdatedAt,
			Metadata:    existing.Metadata,
			Config:      req.Config,
		}, nil

	case "templates":
		existing, err := configRepo.GetTemplateConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing template config: %w", err)
		}
		if existing == nil {
			return nil, fmt.Errorf("template config with ID %d not found", id)
		}

		// Update template config using existing config as base
		existing.Name = req.Name
		existing.Description = req.Description
		existing.Content = extractMapStringFromConfig(req.Config, "content")
		existing.TemplateMetadata = extractMapStringSliceFromConfig(req.Config, "template_metadata")
		existing.Metadata = req.Metadata

		// Validate and update
		if err := repository.ValidateTemplateConfig(existing); err != nil {
			return nil, fmt.Errorf("template config validation failed: %w", err)
		}

		if err := configRepo.UpsertTemplateConfig(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update template config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          existing.ID,
			Name:        existing.Name,
			Type:        configType,
			Enabled:     req.Enabled,
			Description: existing.Description,
			Priority:    req.Priority,
			CreatedAt:   existing.CreatedAt,
			UpdatedAt:   existing.UpdatedAt,
			Metadata:    existing.Metadata,
			Config:      req.Config,
		}, nil

	case "vectorstores":
		existing, err := configRepo.GetVectorStoreConfigByID(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing vector store config: %w", err)
		}
		if existing == nil {
			return nil, fmt.Errorf("vector store config with ID %d not found", id)
		}

		// Update vectorstore config using existing config as base
		existing.Name = req.Name
		existing.Description = req.Description
		existing.Provider = extractStringFromConfig(req.Config, "provider", existing.Provider)
		existing.Endpoint = extractStringFromConfig(req.Config, "endpoint", existing.Endpoint)
		existing.User = extractStringFromConfig(req.Config, "user", existing.User)
		existing.Database = extractStringFromConfig(req.Config, "database", existing.Database)
		existing.EmbeddingID = extractInt64FromConfig(req.Config, "embedding_id", existing.EmbeddingID)
		existing.BatchSize = int(extractInt64FromConfig(req.Config, "batch_size", int64(existing.BatchSize)))
		existing.FlushInterval = time.Duration(extractInt64FromConfig(req.Config, "flush_interval_ms", int64(existing.FlushInterval.Milliseconds()))) * time.Millisecond
		existing.MaxRetries = int(extractInt64FromConfig(req.Config, "max_retries", int64(existing.MaxRetries)))
		existing.MaxOpenConns = int(extractInt64FromConfig(req.Config, "max_open_conns", int64(existing.MaxOpenConns)))
		existing.MaxIdleConns = int(extractInt64FromConfig(req.Config, "max_idle_conns", int64(existing.MaxIdleConns)))
		existing.ConnMaxLifetime = time.Duration(extractInt64FromConfig(req.Config, "conn_max_lifetime_ms", int64(existing.ConnMaxLifetime.Milliseconds()))) * time.Millisecond
		existing.BatchErrorPolicy = extractStringFromConfig(req.Config, "batch_error_policy", existing.BatchErrorPolicy)
		existing.MaxConcurrentBatches = int(extractInt64FromConfig(req.Config, "max_concurrent_batches", int64(existing.MaxConcurrentBatches)))
		existing.VendorSpecificOptions = extractMapFromConfig(req.Config, "vendor_specific_options")
		existing.Metadata = req.Metadata

		// Validate and update
		if err := repository.ValidateVectorStoreConfig(existing); err != nil {
			return nil, fmt.Errorf("vectorstore config validation failed: %w", err)
		}

		if err := configRepo.UpsertVectorStoreConfig(ctx, existing); err != nil {
			return nil, fmt.Errorf("failed to update vectorstore config: %w", err)
		}

		return &ConfigServiceResponse{
			ID:          existing.ID,
			Name:        existing.Name,
			Type:        configType,
			Enabled:     req.Enabled,
			Description: existing.Description,
			Priority:    req.Priority,
			CreatedAt:   existing.CreatedAt,
			UpdatedAt:   existing.UpdatedAt,
			Metadata:    existing.Metadata,
			Config:      req.Config,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported config type: %s", configType)
	}
}

// DeleteConfiguration deletes a configuration
func (s *ConfigService) DeleteConfiguration(ctx context.Context, configType string, id int64) error {
	configRepo, err := s.getConfigRepository(ctx)
	if err != nil {
		return fmt.Errorf("failed to get config repository: %w", err)
	}

	switch configType {
	case "schemas":
		return configRepo.DeleteSchemaConfigWithDependencyCheck(ctx, id)
	case "processors":
		return configRepo.DeleteProcessorConfig(ctx, id)
	case "embeddings":
		return configRepo.DeleteEmbeddingConfig(ctx, id)
	case "templates":
		return configRepo.DeleteTemplateConfig(ctx, id)
	case "vectorstores":
		return configRepo.DeleteVectorStoreConfig(ctx, id)
	default:
		return fmt.Errorf("unsupported config type: %s", configType)
	}
}

// GetConfigurationStatistics returns configuration statistics
func (s *ConfigService) GetConfigurationStatistics(ctx context.Context) (*ConfigStatisticsServiceResponse, error) {
	configRepo, err := s.getConfigRepository(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get config repository: %w", err)
	}

	stats, err := configRepo.GetConfigStatistics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration statistics: %w", err)
	}

	return &ConfigStatisticsServiceResponse{
		SchemaConfigs:      stats.SchemaConfigCount,
		ProcessorConfigs:   stats.ProcessorConfigCount,
		EmbeddingConfigs:   stats.EmbeddingConfigCount,
		TemplateConfigs:    stats.TemplateConfigCount,
		VectorStoreConfigs: stats.VectorStoreConfigCount,
		TotalConfigs:       stats.SchemaConfigCount + stats.ProcessorConfigCount + stats.EmbeddingConfigCount + stats.TemplateConfigCount + stats.VectorStoreConfigCount,
	}, nil
}
