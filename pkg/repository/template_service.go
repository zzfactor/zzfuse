package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"
)

// Package-level cache for template engines
var (
	globalTemplateCache        sync.Map // Thread-safe cache for template engines
	globalLogger               util.Logger
	globalConfig               *TemplateServiceConfig
	globalTemplateService      *TemplateService // Global TemplateService instance
	globalTemplateServiceMutex sync.RWMutex
)

// TemplateServiceConfig holds configuration for the template service
type TemplateServiceConfig struct {
	MaxCacheSize    int           `json:"max_cache_size"`
	CacheTTL        time.Duration `json:"cache_ttl"`
	EnableMetrics   bool          `json:"enable_metrics"`
	ValidationLevel string        `json:"validation_level"` // "none", "basic", "strict"
}

// DefaultTemplateServiceConfig returns default configuration
func DefaultTemplateServiceConfig() *TemplateServiceConfig {
	return &TemplateServiceConfig{
		MaxCacheSize:    1000,
		CacheTTL:        1 * time.Hour,
		EnableMetrics:   true,
		ValidationLevel: "basic",
	}
}

// GetTemplateEngine returns a template engine for the given template ID
// This is the main public interface - clients can call this without storing resources
func GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error) {
	// Check cache first
	if cached, exists := globalTemplateCache.Load(templateID); exists {
		if engine, ok := cached.(*content.ContentTemplateEngine); ok {
			return engine, nil
		}
	}

	// Return a default template engine with a simple template
	return createDefaultTemplateEngine(), nil
}

// GetTemplateEngineForSignalType returns a template engine for a specific signal type
func GetTemplateEngineForSignalType(signalType string) (*content.ContentTemplateEngine, error) {
	// For now, return a default template engine
	// In the future, this could look up templates by signal type
	return createDefaultTemplateEngine(), nil
}

// createDefaultTemplateEngine creates a default template engine with simple templates
func createDefaultTemplateEngine() *content.ContentTemplateEngine {
	engine := content.NewContentTemplateEngine()

	// Register simple templates for common signal types
	templates := map[string]string{
		"logs":                          "Log Event from service {{.service_name}}. Severity: {{.severity_text}}. Message: {{.body}}. Generated at {{now}}. Status: {{if .severity_text}}{{.severity_text}}{{else}}unknown{{end}}.",
		"spans":                         "Span: {{.name}} from service {{.service_name}}. Duration: {{.duration_ms}}ms. Status: {{.status}}.",
		"metrics":                       "Metric: {{.metric_name}} from service {{.service_name}} recorded {{.value}}. Type: {{.metric_type}}.",
		"gauge_metrics":                 "Gauge metric '{{.metric_name}}' from service {{.service_name}} recorded a value of {{.value}}.",
		"sum_metrics":                   "Sum metric '{{.metric_name}}' from service {{.service_name}} recorded a value of {{.value}}.",
		"histogram_metrics":             "Histogram metric '{{.metric_name}}' from service {{.service_name}} with count {{.count}} and sum {{.sum}}.",
		"summary_metrics":               "Summary metric '{{.metric_name}}' from service {{.service_name}} with count {{.count}} and sum {{.sum}}.",
		"exponential_histogram_metrics": "Exponential histogram metric '{{.metric_name}}' from service {{.service_name}} with count {{.count}} and sum {{.sum}}.",
	}

	if globalLogger != nil {
		globalLogger.Debug("Creating default template engine with templates",
			"templates", templates)
	}

	for signalType, template := range templates {
		if globalLogger != nil {
			globalLogger.Debug("Registering default template",
				"signal_type", signalType,
				"template", template)
		}
		if err := engine.RegisterTemplate(signalType, template); err != nil {
			if globalLogger != nil {
				globalLogger.Error("Failed to register default template",
					"signal_type", signalType,
					"error", err)
			}
		} else {
			if globalLogger != nil {
				globalLogger.Debug("Successfully registered default template",
					"signal_type", signalType)
			}
		}
	}

	return engine
}

// CacheTemplateEngine caches a template engine for future use
func CacheTemplateEngine(templateID int64, engine *content.ContentTemplateEngine) {
	globalTemplateCache.Store(templateID, engine)
}

// ClearGlobalCache clears the global template cache
func ClearGlobalCache() {
	globalTemplateCache = sync.Map{}
}

// GetGlobalCacheStats returns statistics about the global cache
func GetGlobalCacheStats() map[string]interface{} {
	count := 0
	globalTemplateCache.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	return map[string]interface{}{
		"cache_size": count,
		"max_size":   globalConfig.MaxCacheSize,
		"ttl":        globalConfig.CacheTTL,
	}
}

// GetTemplates returns processor configs for a schema and signal type (global function)
func GetTemplates(schemaName string, signalType string, processingMode string) ([]*database.ProcessorMappingConfig, error) {
	// Try to get the global TemplateService instance
	globalTemplateServiceMutex.RLock()
	globalTS := globalTemplateService
	globalTemplateServiceMutex.RUnlock()

	if globalTS != nil {
		// Use the global TemplateService to get actual processor configs
		return globalTS.GetTemplates(schemaName, signalType, processingMode)
	}

	// No fallback - return empty slice if global TemplateService is not available
	if globalLogger != nil {
		globalLogger.Debug("Global TemplateService not available, returning empty slice",
			"schema", schemaName,
			"signal_type", signalType,
			"processing_mode", processingMode)
	}
	return []*database.ProcessorMappingConfig{}, nil
}

// GetTemplatesForSchemaAndSignalType returns templates for a schema and signal type (global function)
func GetTemplatesForSchemaAndSignalType(ctx context.Context, schemaName string, signalType string, processingMode string) ([]*TemplateInfo, error) {
	globalTemplateServiceMutex.RLock()
	globalTS := globalTemplateService
	globalTemplateServiceMutex.RUnlock()

	if globalTS != nil {
		return globalTS.GetTemplatesForSchemaAndSignalType(ctx, schemaName, signalType, processingMode)
	}
	if globalLogger != nil {
		globalLogger.Debug("Global TemplateService not available, using fallback",
			"schema", schemaName,
			"signal_type", signalType,
			"processing_mode", processingMode)
	}
	// Return empty slice if global service not available
	return []*TemplateInfo{}, nil
}

// TemplateInfo holds template data and metadata for deduplication
type TemplateInfo struct {
	Template       *database.EmbeddingConfigTemplate
	Usage          int                            // Number of vector stores using this template
	ContentEngine  *content.ContentTemplateEngine // Pre-compiled content template engine
	MetadataEngine *content.ContentTemplateEngine // Pre-compiled metadata template engine
	// Schema information for filtering
	Schemas map[string]bool // Set of schema names this template is configured for
	// Processing mode information for filtering
	ProcessingModes map[string]bool // Set of processing modes this template is configured for
}

// TemplateService provides centralized template management with caching and compilation
type TemplateService struct {
	cache  sync.Map // Thread-safe cache
	logger util.Logger
	config *TemplateServiceConfig
	mu     sync.RWMutex
}

// NewTemplateServiceWithPreload creates a TemplateService and preloads templates by ID from the repo.
func NewTemplateServiceWithPreload(ctx context.Context, logger util.Logger, templateRepo TemplateRepo, templateIDs []int64, config *TemplateServiceConfig) (*TemplateService, error) {
	if logger == nil {
		logger = util.NoopLogger{}
	}
	if config == nil {
		config = DefaultTemplateServiceConfig()
	}
	ts := &TemplateService{
		logger: logger,
		config: config,
	}
	if err := ts.PreloadTemplates(ctx, templateIDs, templateRepo); err != nil {
		return nil, err
	}
	return ts, nil
}

// GetOrCompileTemplate retrieves a template from cache or compiles it
func (s *TemplateService) GetOrCompileTemplate(templateID int64, template *database.EmbeddingConfigTemplate) (*TemplateInfo, error) {
	// Validate template first
	if err := s.ValidateTemplate(template); err != nil {
		return nil, fmt.Errorf("template validation failed: %w", err)
	}

	// Check cache first (thread-safe)
	if cached, exists := s.cache.Load(templateID); exists {
		if templateInfo, ok := cached.(*TemplateInfo); ok {
			s.mu.Lock()
			templateInfo.Usage++
			s.mu.Unlock()

			s.logger.Debug("Using cached template",
				"template_id", templateID,
				"usage_count", templateInfo.Usage)
			return templateInfo, nil
		}
	}

	// Compile new template
	contentEngine, err := s.compileContentEngine(template, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to compile content engine: %w", err)
	}

	// ✅ FIXED: Metadata is used for field extraction, not template rendering
	// No need to compile metadata engine - metadata fields are used directly

	// Cache the compiled template (thread-safe)
	templateInfo := &TemplateInfo{
		Template:        template,
		Usage:           1,
		ContentEngine:   contentEngine,
		MetadataEngine:  nil,                   // ✅ Metadata engine not needed
		Schemas:         make(map[string]bool), // Initialize empty schema set
		ProcessingModes: make(map[string]bool), // Initialize empty processing mode set
	}
	s.cache.Store(templateID, templateInfo)

	s.logger.Info("Compiled and cached new template",
		"template_id", templateID,
		"template_name", template.Name,
		"content_templates", len(template.Content),
		"metadata_templates", len(template.TemplateMetadata))

	return templateInfo, nil
}

// GetTemplateEngine returns the content engine for a template ID
func (s *TemplateService) GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error) {
	if cached, exists := s.cache.Load(templateID); exists {
		if templateInfo, ok := cached.(*TemplateInfo); ok {
			return templateInfo.ContentEngine, nil
		}
	}
	return nil, fmt.Errorf("template %d not found", templateID)
}

// GetTemplatesEngine returns all content engines
func (s *TemplateService) GetTemplatesEngine() (map[int64]*content.ContentTemplateEngine, error) {
	engines := make(map[int64]*content.ContentTemplateEngine)
	s.cache.Range(func(key, value interface{}) bool {
		if templateInfo, ok := value.(*TemplateInfo); ok {
			engines[templateInfo.Template.ID] = templateInfo.ContentEngine
		}
		return true
	})
	return engines, nil
}

// GetMetadataEngine returns the metadata engine for a template ID
func (s *TemplateService) GetMetadataEngine(templateID int64) (*content.ContentTemplateEngine, error) {
	// ✅ FIXED: Metadata is used for field extraction, not template rendering
	// Return nil since metadata engine is not needed
	return nil, fmt.Errorf("metadata engine not used - metadata fields are extracted directly")
}

// ValidateTemplate validates a template before compilation
func (s *TemplateService) ValidateTemplate(template *database.EmbeddingConfigTemplate) error {
	if template == nil {
		return fmt.Errorf("template is nil")
	}

	if len(template.Content) == 0 {
		return fmt.Errorf("template has no content")
	}

	if s.config.ValidationLevel == "strict" {
		// Validate each template content
		for key, content := range template.Content {
			if err := s.validateTemplateContent(key, content); err != nil {
				return fmt.Errorf("invalid template %s: %w", key, err)
			}
		}
	}

	return nil
}

// validateTemplateContent validates individual template content
func (s *TemplateService) validateTemplateContent(key, content string) error {
	if content == "" {
		return fmt.Errorf("template content for key '%s' cannot be empty", key)
	}

	if len(content) > MaxTemplateContentSize {
		return fmt.Errorf("template content for key '%s' exceeds maximum size of %d bytes", key, MaxTemplateContentSize)
	}

	// Basic syntax validation
	if !strings.Contains(content, "{{") && !strings.Contains(content, "}}") {
		return fmt.Errorf("template content for key '%s' appears to be static text", key)
	}

	return nil
}

// compileContentEngine compiles content templates with signal type registration
func (s *TemplateService) compileContentEngine(tpl *database.EmbeddingConfigTemplate, templateID int64) (*content.ContentTemplateEngine, error) {
	engine := content.NewContentTemplateEngine()

	for signalType, templateContent := range tpl.Content {
		// ✅ SIMPLIFIED: Register template with signal type directly
		if err := engine.RegisterTemplate(signalType, templateContent); err != nil {
			s.logger.Error("Failed to register content template",
				"template_id", templateID,
				"signal_type", signalType,
				"error", err)
			return nil, fmt.Errorf("failed to register content template %s: %w", signalType, err)
		}
	}

	return engine, nil
}

// ValidateTemplateContent validates template content
func ValidateTemplateContent(templateContent map[string]string) error {
	fieldMapping := template.NewFieldMapping()
	return fieldMapping.ValidateTemplateContent(templateContent, MaxTemplateContentSize)
}

// ClearCache clears all cached templates
func (s *TemplateService) ClearCache() {
	s.cache = sync.Map{}
	s.logger.Info("Template cache cleared")
}

// GetCacheStats returns cache statistics
func (s *TemplateService) GetCacheStats() map[string]interface{} {
	count := 0
	s.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	return map[string]interface{}{
		"cache_size": count,
		"max_size":   s.config.MaxCacheSize,
		"ttl":        s.config.CacheTTL,
	}
}

// PreloadTemplates loads and compiles all templates by ID from the database, storing them in the cache.
func (s *TemplateService) PreloadTemplates(ctx context.Context, templateIDs []int64, repo TemplateRepo) error {
	for _, templateID := range templateIDs {
		// Skip if already cached
		if _, exists := s.cache.Load(templateID); exists {
			continue
		}
		tpl, err := repo.GetByID(ctx, templateID)
		if err != nil {
			s.logger.Error("Failed to preload template", "template_id", templateID, "error", err)
			return err
		}
		_, err = s.GetOrCompileTemplate(templateID, tpl)
		if err != nil {
			s.logger.Error("Failed to compile template during preload", "template_id", templateID, "error", err)
			return err
		}
	}
	return nil
}

// NewTemplateService creates a TemplateService and preloads templates for enabled mappings.
func NewTemplateService(ctx context.Context, logger util.Logger, templateRepo TemplateRepo, enabledProcessors []*database.ProcessorMappingConfig, schemaMap map[int64]string, config *TemplateServiceConfig) (*TemplateService, error) {
	// Extract unique template IDs and build schema and processing mode mapping
	templateIDSet := make(map[int64]struct{})
	templateSchemaMap := make(map[int64]map[string]bool)         // templateID -> set of schema names
	templateProcessingModeMap := make(map[int64]map[string]bool) // templateID -> set of processing modes

	if logger != nil {
		logger.Debug("NewTemplateService - Building template mappings",
			"enabled_processors", len(enabledProcessors),
			"schema_map", schemaMap)
	}

	for _, mapping := range enabledProcessors {
		if logger != nil {
			logger.Debug("Processing mapping",
				"id", mapping.ID,
				"name", mapping.Name,
				"template_id", mapping.TemplateID,
				"processing_mode", mapping.ProcessingMode)
		}

		if mapping.TemplateID > 0 {
			templateIDSet[mapping.TemplateID] = struct{}{}

			// Get schema name for this processor using the provided schema map
			schemaName := schemaMap[mapping.DatabaseID]
			if schemaName != "" {
				if templateSchemaMap[mapping.TemplateID] == nil {
					templateSchemaMap[mapping.TemplateID] = make(map[string]bool)
				}
				templateSchemaMap[mapping.TemplateID][schemaName] = true
				if logger != nil {
					logger.Debug("Template mapped to schema",
						"template_id", mapping.TemplateID,
						"schema", schemaName,
						"database_id", mapping.DatabaseID)
				}
			}

			// Get processing mode for this processor
			processingMode := mapping.ProcessingMode
			if processingMode != "" {
				if templateProcessingModeMap[mapping.TemplateID] == nil {
					templateProcessingModeMap[mapping.TemplateID] = make(map[string]bool)
				}
				templateProcessingModeMap[mapping.TemplateID][processingMode] = true
				if logger != nil {
					logger.Debug("Template mapped to processing mode",
						"template_id", mapping.TemplateID,
						"processing_mode", processingMode)
				}
			}
		}
	}

	if logger != nil {
		logger.Debug("Final template mappings",
			"schema_mappings", templateSchemaMap,
			"processing_mode_mappings", templateProcessingModeMap)
	}

	var templateIDs []int64
	for id := range templateIDSet {
		templateIDs = append(templateIDs, id)
	}

	if logger != nil {
		logger.Debug("Template IDs to preload",
			"template_ids", templateIDs)
	}

	globalLogger = logger

	if config == nil {
		globalConfig = DefaultTemplateServiceConfig()
	} else {
		globalConfig = config
	}

	// Create the actual TemplateService with loaded templates
	ts, err := NewTemplateServiceWithPreload(ctx, logger, templateRepo, templateIDs, config)
	if err != nil {
		return nil, err
	}

	// Populate schema and processing mode information for each template
	ts.populateSchemaAndProcessingModeInformation(templateSchemaMap, templateProcessingModeMap)

	// Set the global TemplateService to the actual instance with loaded templates
	globalTemplateServiceMutex.Lock()
	globalTemplateService = ts
	globalTemplateServiceMutex.Unlock()

	return ts, nil
}

// AnalyzeTemplatesOnStartup analyzes templates for a set of vector stores and logs the results.
func (s *TemplateService) AnalyzeTemplatesOnStartup(vectorStores map[string]*VectorStore, logger util.Logger) map[string]*TemplateAnalysis {
	results := make(map[string]*TemplateAnalysis)
	if vectorStores == nil {
		return results
	}

	extractor := NewTemplateFieldExtractor()

	for vsKey, vs := range vectorStores {
		if vs == nil {
			continue
		}
		// If VectorStoreConfig is available, analyze it
		var vsConfig *VectorStoreConfig
		if getter, ok := interface{}(vs).(interface{ GetVectorStoreConfig() *VectorStoreConfig }); ok {
			vsConfig = getter.GetVectorStoreConfig()
		}
		if vsConfig == nil {
			continue
		}
		if vsConfig.TemplateContent == nil {
			continue
		}
		for templateKey, templateContent := range vsConfig.TemplateContent {
			if templateKey == "spans" {
				analysis := extractor.AnalyzeTemplate(templateContent)
				results[vsKey] = analysis
				if analysis.NeedsLogs || analysis.NeedsExemplars || analysis.NeedsProfiles {
					logger.Info("Spans template needs correlated signals, using trace processing",
						"needs_logs", analysis.NeedsLogs,
						"needs_exemplars", analysis.NeedsExemplars,
						"needs_profiles", analysis.NeedsProfiles,
						"needs_events", analysis.NeedsEvents,
						"exemplar_types", analysis.ExemplarTypes)
				} else {
					logger.Info("Spans template is simple, using direct rendering")
				}
			} else {
				fields, err := extractor.ExtractFieldsFromTemplate(templateContent)
				if err != nil {
					logger.Error("Failed to extract fields from template",
						"template_key", templateKey,
						"error", err)
				} else {
					logger.Debug("Extracted fields for template",
						"template_key", templateKey,
						"fields", fields)
				}
			}
		}
	}
	return results
}

// TemplateRepo is an interface for loading templates by ID (for testability)
type TemplateRepo interface {
	GetByID(ctx context.Context, templateID int64) (*database.EmbeddingConfigTemplate, error)
}

// Populate schema and processing mode information for templates
func (s *TemplateService) populateSchemaAndProcessingModeInformation(templateSchemaMap map[int64]map[string]bool, templateProcessingModeMap map[int64]map[string]bool) {
	s.cache.Range(func(key, value interface{}) bool {
		if templateInfo, ok := value.(*TemplateInfo); ok {
			templateID := templateInfo.Template.ID
			if schemas, exists := templateSchemaMap[templateID]; exists {
				templateInfo.Schemas = schemas
			} else {
				// If no schema mapping found, initialize empty map
				templateInfo.Schemas = make(map[string]bool)
			}
			if processingModes, exists := templateProcessingModeMap[templateID]; exists {
				templateInfo.ProcessingModes = processingModes
			} else {
				// If no processing mode mapping found, initialize empty map
				templateInfo.ProcessingModes = make(map[string]bool)
			}
		}
		return true
	})
}

// ✅ NEW: Get all templates for a schema and signal type
func (s *TemplateService) GetTemplatesForSchemaAndSignalType(ctx context.Context, schemaName string, signalType string, processingMode string) ([]*TemplateInfo, error) {
	// Return the actual precompiled templates from memory
	// These were loaded during TemplateService initialization
	var matchingTemplates []*TemplateInfo

	// Iterate through the cache to find matching templates
	s.cache.Range(func(key, value interface{}) bool {
		if templateInfo, ok := value.(*TemplateInfo); ok {
			// Check if this template has content for the requested signal type
			if _, hasSignalType := templateInfo.Template.Content[signalType]; hasSignalType {
				// Check if this template is configured for the requested schema
				if templateInfo.Schemas[schemaName] {
					// Check if this template is configured for the requested processing mode
					if templateInfo.ProcessingModes[processingMode] {
						// Increment usage count
						s.mu.Lock()
						templateInfo.Usage++
						s.mu.Unlock()

						matchingTemplates = append(matchingTemplates, templateInfo)
					}
				}
			}
		}
		return true
	})

	// If no matching templates found in cache, return empty slice (no fallback)
	if len(matchingTemplates) == 0 {
		s.logger.Debug("No matching templates found in cache, returning empty slice",
			"schema", schemaName,
			"signal_type", signalType,
			"processing_mode", processingMode)
	}

	s.logger.Debug("GetTemplatesForSchemaAndSignalType returning templates from cache",
		"schema", schemaName,
		"signal_type", signalType,
		"processing_mode", processingMode,
		"template_count", len(matchingTemplates))

	return matchingTemplates, nil
}

// ✅ NEW: Get processor configs for a schema and signal type
func (s *TemplateService) GetTemplates(schemaName string, signalType string, processingMode string) ([]*database.ProcessorMappingConfig, error) {
	s.logger.Debug("GetTemplates called",
		"schema", schemaName,
		"signal_type", signalType,
		"processing_mode", processingMode)

	// Return the actual precompiled processor configs from memory
	// These were loaded during TemplateService initialization
	var matchingConfigs []*database.ProcessorMappingConfig

	// Iterate through the cache to find matching processor configs
	s.cache.Range(func(key, value interface{}) bool {
		if templateInfo, ok := value.(*TemplateInfo); ok {
			s.logger.Debug("Checking template",
				"template_id", templateInfo.Template.ID,
				"schemas", templateInfo.Schemas,
				"processing_modes", templateInfo.ProcessingModes)

			// Check if this template has content for the requested signal type
			if _, hasSignalType := templateInfo.Template.Content[signalType]; hasSignalType {
				s.logger.Debug("Template has signal type",
					"template_id", templateInfo.Template.ID,
					"signal_type", signalType)

				// Check if this template is configured for the requested schema
				if templateInfo.Schemas[schemaName] {
					s.logger.Debug("Template configured for schema",
						"template_id", templateInfo.Template.ID,
						"schema", schemaName)

					// Check if this template is configured for the requested processing mode
					if templateInfo.ProcessingModes[processingMode] {
						s.logger.Debug("Template configured for processing mode - including",
							"template_id", templateInfo.Template.ID,
							"processing_mode", processingMode)

						// Create a processor config for this template
						config := &database.ProcessorMappingConfig{
							ID:             templateInfo.Template.ID,
							Name:           fmt.Sprintf("%s-%s-processor", schemaName, signalType),
							DatabaseID:     1, // Default schema ID
							VectorStoreID:  1, // Default vector store ID
							TemplateID:     templateInfo.Template.ID,
							Enabled:        true,
							ProcessingMode: processingMode, // Use the actual processing mode
							Routing: map[string]interface{}{
								"schema":      schemaName,
								"signal_type": signalType,
							},
							EmbeddingFilter: map[string]interface{}{
								"signal_types": []string{signalType},
							},
							Metadata: map[string]interface{}{
								"mode":        processingMode,
								"schema":      schemaName,
								"signal_type": signalType,
							},
						}
						matchingConfigs = append(matchingConfigs, config)
					} else {
						s.logger.Debug("Template not configured for processing mode - excluding",
							"template_id", templateInfo.Template.ID,
							"processing_mode", processingMode)
					}
				} else {
					s.logger.Debug("Template not configured for schema - excluding",
						"template_id", templateInfo.Template.ID,
						"schema", schemaName)
				}
			} else {
				s.logger.Debug("Template missing signal type - excluding",
					"template_id", templateInfo.Template.ID,
					"signal_type", signalType)
			}
		}
		return true
	})

	// If no matching configs found for specific metric types, try fallback to generic metrics
	if len(matchingConfigs) == 0 && (signalType == "gauge_metrics" || signalType == "sum_metrics" || signalType == "histogram_metrics" || signalType == "summary_metrics" || signalType == "exponential_histogram_metrics") {
		s.logger.Debug("No templates for specific metric type, trying fallback",
			"signal_type", signalType)
		return s.GetTemplates(schemaName, "metrics", processingMode)
	}

	// If no matching configs found, return empty slice (no fallback)
	if len(matchingConfigs) == 0 {
		s.logger.Debug("No matching processor configs found in cache, returning empty slice",
			"schema", schemaName,
			"signal_type", signalType,
			"processing_mode", processingMode)
	}

	s.logger.Debug("GetTemplates returning configs from cache",
		"schema", schemaName,
		"signal_type", signalType,
		"processing_mode", processingMode,
		"config_count", len(matchingConfigs))

	return matchingConfigs, nil
}
