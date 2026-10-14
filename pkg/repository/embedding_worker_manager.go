package repository

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"
	"go.uber.org/zap"
)

// Configuration constants
const (
	// Worker configuration
	DefaultWorkerBatchSize    = 100
	DefaultShutdownTimeout    = 30 * time.Second
	DefaultWorkerStartTimeout = 10 * time.Second

	// Validation limits
	MaxWorkerIDLength   = 255
	MaxSchemaNameLength = 64
)

// WorkerConfig holds configuration for creating an embedding worker
type WorkerConfig struct {
	VectorStoreKey   string
	SignalType       string
	SchemaName       string
	MappingConfig    dbconfig.ProcessorMappingConfig
	EmbeddingConfig  dbconfig.EmbeddingConfig
	VectorStore      *VectorStore
	TemplateEngine   *content.ContentTemplateEngine
	TemplateContent  map[string]string
	TemplateMetadata map[string][]string
	DatabaseClient   OtelDatabaseClient
	EventBus         event.EventBusInterface
}

// Validate validates the worker configuration
func (wc *WorkerConfig) Validate() error {
	if wc.VectorStoreKey == "" {
		return fmt.Errorf("vector store key cannot be empty")
	}
	if wc.SignalType == "" {
		return fmt.Errorf("signal type cannot be empty")
	}
	if wc.SchemaName == "" {
		return fmt.Errorf("schema name cannot be empty")
	}
	if len(wc.SchemaName) > MaxSchemaNameLength {
		return fmt.Errorf("schema name exceeds maximum length of %d", MaxSchemaNameLength)
	}
	if wc.VectorStore == nil {
		return fmt.Errorf("vector store cannot be nil")
	}
	// TemplateEngine can be nil during architectural transition
	// if wc.TemplateEngine == nil {
	//     return fmt.Errorf("template engine cannot be nil")
	// }
	if wc.DatabaseClient == nil {
		return fmt.Errorf("database client cannot be nil")
	}
	// EventBus can be nil during architectural transition
	// if wc.EventBus == nil {
	//     return fmt.Errorf("event bus cannot be nil")
	// }
	return nil
}

// WorkerManager manages a collection of embedding workers for a schema
type WorkerManager struct {
	workers     []*EmbeddingWorker
	configs     []*WorkerConfig
	schemaName  string
	logger      util.Logger
	mu          sync.RWMutex
	wg          sync.WaitGroup
	isRunning   bool
	shutdownCtx context.Context
	cancelFunc  context.CancelFunc
}

// NewWorkerManager creates a new worker manager
func NewWorkerManager(schemaName string, logger util.Logger) *WorkerManager {
	return &WorkerManager{
		workers:    []*EmbeddingWorker{},
		configs:    []*WorkerConfig{},
		schemaName: schemaName,
		logger:     logger,
	}
}

// StartWorkers starts all workers
func (wm *WorkerManager) StartWorkers() {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if wm.isRunning {
		return
	}

	for _, w := range wm.workers {
		wm.wg.Add(1)
		go func(worker *EmbeddingWorker) {
			defer wm.wg.Done()
			worker.Start()
		}(w)
	}
	wm.isRunning = true
	wm.logger.Info("Started workers", "schema", wm.schemaName, "count", len(wm.workers))
}

// StopWorkers stops all workers with graceful shutdown timeout
func (wm *WorkerManager) StopWorkers() {
	wm.StopWorkersWithTimeout(DefaultShutdownTimeout)
}

// StopWorkersWithTimeout stops all workers with specified timeout
func (wm *WorkerManager) StopWorkersWithTimeout(timeout time.Duration) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if !wm.isRunning {
		return
	}

	// Cancel any existing shutdown context
	if wm.cancelFunc != nil {
		wm.cancelFunc()
	}

	// Create shutdown context with timeout
	wm.shutdownCtx, wm.cancelFunc = context.WithTimeout(context.Background(), timeout)

	// Stop workers with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, w := range wm.workers {
			w.Stop()
		}
	}()

	// Wait for shutdown or timeout
	select {
	case <-done:
		wm.logger.Info("Workers stopped gracefully", "schema", wm.schemaName, "count", len(wm.workers))
	case <-wm.shutdownCtx.Done():
		wm.logger.Error("Worker shutdown timeout exceeded", "schema", wm.schemaName, "timeout", timeout)
	}

	// Wait for all worker goroutines to exit
	wm.wg.Wait()

	wm.isRunning = false
	if wm.cancelFunc != nil {
		wm.cancelFunc()
		wm.cancelFunc = nil
	}
}

// ReloadWorkers stops current workers and creates new ones with updated configs
func (wm *WorkerManager) ReloadWorkers(ctx context.Context, newConfigs []*WorkerConfig) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	// Validate all configurations first
	for i, config := range newConfigs {
		if err := config.Validate(); err != nil {
			return fmt.Errorf("invalid worker config at index %d: %w", i, err)
		}
		// Additional validation for signal type
		if !isValidSignalType(config.SignalType) {
			return fmt.Errorf("invalid signal type '%s' at config index %d", config.SignalType, i)
		}
	}

	// Stop existing workers with timeout
	wasRunning := wm.isRunning
	if wasRunning {
		wm.stopWorkersInternal(DefaultShutdownTimeout)
	}

	// Clear existing workers and configs
	wm.workers = []*EmbeddingWorker{}
	wm.configs = []*WorkerConfig{}

	// Create new workers from configs
	var creationErrors []string
	for _, config := range newConfigs {
		worker := createWorkerFromConfig(config, wm.logger)
		if worker == nil {
			creationErrors = append(creationErrors, fmt.Sprintf("vectorStore=%s: worker creation failed", config.VectorStoreKey))
			wm.logger.Error("Failed to create worker from config", "vectorStoreKey", config.VectorStoreKey)
			continue
		}
		wm.workers = append(wm.workers, worker)
		wm.configs = append(wm.configs, config)
	}

	// Return error if no workers were created successfully
	if len(wm.workers) == 0 && len(newConfigs) > 0 {
		return fmt.Errorf("failed to create any workers: %v", creationErrors)
	}

	// Start new workers if manager was running
	if wasRunning {
		for _, w := range wm.workers {
			wm.wg.Add(1)
			go func(worker *EmbeddingWorker) {
				defer wm.wg.Done()
				worker.Start()
			}(w)
		}
		wm.isRunning = true
	}

	wm.logger.Info("Reloaded workers", "schema", wm.schemaName, "count", len(wm.workers), "errors", len(creationErrors))
	return nil
}

// stopWorkersInternal stops workers with timeout (internal method, assumes lock held)
func (wm *WorkerManager) stopWorkersInternal(timeout time.Duration) {
	// Cancel any existing shutdown context
	if wm.cancelFunc != nil {
		wm.cancelFunc()
	}

	// Create shutdown context with timeout
	wm.shutdownCtx, wm.cancelFunc = context.WithTimeout(context.Background(), timeout)

	// Stop workers with timeout
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, w := range wm.workers {
			w.Stop()
		}
	}()

	// Wait for shutdown or timeout
	select {
	case <-done:
		wm.logger.Info("Workers stopped gracefully", "schema", wm.schemaName, "count", len(wm.workers))
	case <-wm.shutdownCtx.Done():
		wm.logger.Error("Worker shutdown timeout exceeded", "schema", wm.schemaName, "timeout", timeout)
	}

	// Wait for all worker goroutines to exit
	wm.wg.Wait()

	wm.isRunning = false
	if wm.cancelFunc != nil {
		wm.cancelFunc()
		wm.cancelFunc = nil
	}
}

// GetWorkerCount returns the number of workers
func (wm *WorkerManager) GetWorkerCount() int {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return len(wm.workers)
}

// IsRunning returns whether workers are currently running
func (wm *WorkerManager) IsRunning() bool {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.isRunning
}

// WorkerSnapshot is a read-only value snapshot of a production embedding worker.
// Counters come from the collector used by RunBatch, not BaseWorker test jobs.
type WorkerSnapshot struct {
	ID                 string `json:"id"`
	SchemaID           string `json:"schema_id"`
	EmbeddingConfigID  int64  `json:"embedding_config_id"`
	SignalType         string `json:"signal_type"`
	Running            bool   `json:"running"`
	LastError          string `json:"last_error"`
	TotalJobsProcessed int64  `json:"total_jobs_processed"`
	SuccessfulJobs     int64  `json:"successful_jobs"`
	FailedJobs         int64  `json:"failed_jobs"`
	ProcessedSignals   int64  `json:"processed_signals"`
	TokensUsed         int64  `json:"tokens_used"`
	ActiveJobs         int64  `json:"active_jobs"`
}

// GetWorkerSnapshots copies live state without exposing mutable worker pointers.
func (wm *WorkerManager) GetWorkerSnapshots() []WorkerSnapshot {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	workers := make([]WorkerSnapshot, 0, len(wm.workers))
	for _, worker := range wm.workers {
		state := worker.GetStatus()
		snapshot := WorkerSnapshot{
			ID: worker.WorkerID, SchemaID: worker.Schema,
			EmbeddingConfigID: worker.ConfigID, SignalType: worker.SignalType,
			Running: state.Running,
		}
		if state.LastError != nil {
			snapshot.LastError = state.LastError.Error()
		}
		if counters := worker.GetMetrics(); counters != nil {
			snapshot.TotalJobsProcessed = counters.TotalJobsProcessed
			snapshot.SuccessfulJobs = counters.SuccessfulJobs
			snapshot.FailedJobs = counters.FailedJobs
			snapshot.ProcessedSignals = counters.TotalSignals
			snapshot.TokensUsed = counters.TotalTokens
			snapshot.ActiveJobs = counters.ActiveJobs
		}
		workers = append(workers, snapshot)
	}
	return workers
}

// buildWorkerConfigs creates worker configurations for all valid vector store/signal type combinations
func buildWorkerConfigs(
	ctx context.Context,
	schemaName string,
	vectorStores map[string]*VectorStore,
	embeddingConfigs map[string]dbconfig.EmbeddingConfig,
	processorMappings map[string]dbconfig.ProcessorMappingConfig,
	databaseFilter map[string]interface{},
	eventBusProvider EventBusProvider,
	dbClient OtelDatabaseClient,
	logger util.Logger,
	templateRepo TemplateRepo,
) ([]*WorkerConfig, error) {
	// Get database signal types (master filter)
	databaseTypes := getSignalTypesFromDatabaseFilter(databaseFilter)
	logger.Debug("Building worker configs",
		zap.Int("vector_stores", len(vectorStores)),
		zap.Int("processor_mappings", len(processorMappings)),
		zap.Strings("database_types", databaseTypes))

	var configs []*WorkerConfig

	// Iterate through each vector store
	for vectorStoreKey, mapping := range processorMappings {
		logger.Debug("Processing vector store", zap.String("vector_store_key", vectorStoreKey))
		vectorStore := vectorStores[vectorStoreKey]
		embeddingConfig := embeddingConfigs[vectorStoreKey]

		// This is called by explicit worker reloads outside the pool mutex.
		// Load the configured templates now: workers have no lazy initializer.
		var engine *content.ContentTemplateEngine
		var templateContent map[string]string
		var templateMetadata map[string][]string
		if mapping.TemplateID > 0 {
			if templateRepo == nil {
				return nil, fmt.Errorf("template repository not initialized for template %d", mapping.TemplateID)
			}
			tpl, err := templateRepo.GetByID(ctx, mapping.TemplateID)
			if err != nil {
				return nil, fmt.Errorf("failed to load template %d: %w", mapping.TemplateID, err)
			}
			if tpl == nil {
				return nil, fmt.Errorf("template %d not found", mapping.TemplateID)
			}
			templateContent = maps.Clone(tpl.Content)
			templateMetadata = make(map[string][]string, len(tpl.TemplateMetadata)+len(tpl.Content))
			for signal, fields := range tpl.TemplateMetadata {
				templateMetadata[signal] = append([]string(nil), fields...)
			}
			engine = content.NewContentTemplateEngine()
			for signal, text := range templateContent {
				if err := engine.RegisterTemplate(signal, text); err != nil {
					return nil, fmt.Errorf("failed to compile template %d for %s: %w", mapping.TemplateID, signal, err)
				}
				// The batch field extractor/render metadata path uses the signal's
				// list. Include generic context so it is fetched and preserved too.
				fields := append([]string(nil), templateMetadata[signal]...)
				seen := make(map[string]bool, len(fields))
				for _, field := range fields {
					seen[field] = true
				}
				for _, field := range templateMetadata["generic"] {
					if !seen[field] {
						fields = append(fields, field)
						seen[field] = true
					}
				}
				templateMetadata[signal] = fields
			}
		}

		signalTypes := getSignalTypesFromEmbeddingFilter(mapping.EmbeddingFilter)
		validSignalTypes := filterValidSignalTypes(signalTypes)

		// Create config for each valid signal type
		for _, signalType := range validSignalTypes {
			// Check if signal type is in database filter (master filter)
			if !stringSliceContains(databaseTypes, signalType) {
				logger.Debug("Skipping signal type not in database filter",
					"signal_type", signalType,
					"vector_store", vectorStoreKey,
					"database_types", databaseTypes)
				continue
			}
			if mapping.TemplateID > 0 && templateContent[signalType] == "" {
				return nil, fmt.Errorf("template %d has no content for %s", mapping.TemplateID, signalType)
			}

			// Handle EventBus - get from EventBusProvider if available, otherwise nil
			var eventBus event.EventBusInterface
			if eventBusProvider != nil {
				if eb, err := eventBusProvider.GetEventBus(); err == nil {
					eventBus = eb
					if eventBus != nil {
						logger.Debug("EventBus retrieved successfully for worker config",
							"signal_type", signalType,
							"vector_store", vectorStoreKey)
					} else {
						logger.Info("EventBus is nil from EventBusProvider",
							"signal_type", signalType,
							"vector_store", vectorStoreKey)
					}
				} else {
					logger.Error("Failed to get EventBus from EventBusProvider",
						zap.Error(err),
						"signal_type", signalType,
						"vector_store", vectorStoreKey)
				}
			} else {
				logger.Info("EventBusProvider is nil, cannot get EventBus",
					"signal_type", signalType,
					"vector_store", vectorStoreKey)
			}

			configs = append(configs, &WorkerConfig{
				VectorStoreKey:   vectorStoreKey,
				SignalType:       signalType,
				SchemaName:       schemaName,
				MappingConfig:    mapping,
				EmbeddingConfig:  embeddingConfig,
				VectorStore:      vectorStore,
				TemplateEngine:   engine,
				TemplateContent:  templateContent,
				TemplateMetadata: templateMetadata,
				DatabaseClient:   dbClient,
				EventBus:         eventBus,
			})
		}
	}

	return configs, nil
}

// createWorkerFromConfig creates an embedding worker from a WorkerConfig
func createWorkerFromConfig(config *WorkerConfig, logger util.Logger) *EmbeddingWorker {
	// Create vector store config for this worker
	vectorStoreConfig := &VectorStoreConfig{
		VectorStoreKey:   config.VectorStoreKey,
		ConfigID:         config.MappingConfig.ID,
		VectorStore:      config.VectorStore.VectorStore,
		EmbeddingConfig:  config.EmbeddingConfig,
		MappingConfig:    config.MappingConfig,
		SignalTypes:      []string{config.SignalType},
		TemplateContent:  config.TemplateContent,
		TemplateMetadata: config.TemplateMetadata,
		TemplateID:       config.MappingConfig.TemplateID,
		ContentEngine:    config.TemplateEngine,
		MetadataEngine:   nil,
	}

	logger.Debug("Creating worker from config",
		zap.String("signal_type", config.SignalType),
		zap.Bool("has_event_bus", config.EventBus != nil))

	// Create worker with this specific vector store config
	worker := NewEmbeddingWorker(
		config.DatabaseClient,
		logger,
		config.VectorStore.VectorStore,
		WithBatchSize(DefaultWorkerBatchSize),
		WithConfigID(config.EmbeddingConfig.ID),
		WithSignalType(config.SignalType),
		WithEmbeddingSchema(config.SchemaName),
		WithWorkerID(fmt.Sprintf("schema-%s-vs-%s-config-%d",
			config.SignalType, config.VectorStoreKey, config.MappingConfig.ID)),
		WithFilterConfig(config.MappingConfig.EmbeddingFilter),
		WithMetadata(config.MappingConfig.Metadata),
		WithContentProcessingMode(config.MappingConfig.ProcessingMode),
		WithEventBus(config.EventBus),
		WithMaxContextLength(config.EmbeddingConfig.MaxContextLength),
		WithVectorStoreConfigs(map[string]*VectorStoreConfig{
			config.VectorStoreKey: vectorStoreConfig,
		}),
	)

	logger.Debug("Worker created",
		zap.String("signal_type", config.SignalType),
		zap.Bool("has_event_bus", worker.eventBus != nil))

	logger.Info("Created worker for vector store",
		"signal_type", config.SignalType,
		"vector_store", config.VectorStoreKey,
		"processor_id", config.EmbeddingConfig.ID,
		"worker_id", worker.WorkerID,
		"template_id", config.MappingConfig.TemplateID)

	return worker
}

// CreateSchemaEmbeddingWorkers creates workers for a schema that can handle multiple vector stores.
// NEW: Creates workers per vector store per signal type to ensure each vector store gets its own database query.
func CreateSchemaEmbeddingWorkers(
	ctx context.Context,
	mainDB *sql.DB,
	schemaName string,
	logger util.Logger,
	vectorStores map[string]*VectorStore,
	embeddingConfigs map[string]dbconfig.EmbeddingConfig,
	processorMappings map[string]dbconfig.ProcessorMappingConfig,
	databaseFilter map[string]interface{},
	serviceResources *ServiceResources, // Add service resources for template cache access
	eventBusProvider EventBusProvider, // Pass the EventBusProvider for event bus access
) ([]*EmbeddingWorker, error) {
	logger.Debug("Creating schema embedding workers",
		zap.String("schema", schemaName),
		zap.Int("vector_stores", len(vectorStores)))

	if len(vectorStores) == 0 {
		logger.Debug("No vector stores found, returning empty worker list")
		return []*EmbeddingWorker{}, nil
	}

	// Create database client for workers
	dbClient := NewUnifiedSQLDatabaseClient(mainDB)
	var templateRepo TemplateRepo
	for _, mapping := range processorMappings {
		if mapping.TemplateID > 0 {
			var err error
			templateRepo, err = dbconfig.NewTemplateConfigRepo(ctx, mainDB, logger)
			if err != nil {
				return nil, fmt.Errorf("failed to initialize template repository: %w", err)
			}
			break
		}
	}

	// Build worker configurations
	workerConfigs, err := buildWorkerConfigs(
		ctx, schemaName, vectorStores, embeddingConfigs,
		processorMappings, databaseFilter, eventBusProvider, dbClient, logger, templateRepo)
	if err != nil {
		return nil, fmt.Errorf("failed to build worker configs: %w", err)
	}
	logger.Debug("Worker configs built", zap.Int("config_count", len(workerConfigs)))

	// Create workers from configurations
	var workers []*EmbeddingWorker
	for i, config := range workerConfigs {
		logger.Debug("Creating worker",
			zap.Int("worker_index", i+1),
			zap.Int("total_workers", len(workerConfigs)),
			zap.String("signal_type", config.SignalType))
		worker := createWorkerFromConfig(config, logger)
		workers = append(workers, worker)
	}
	logger.Debug("All workers created", zap.Int("worker_count", len(workers)))

	logger.Info("Created schema-level workers per vector store",
		"count", len(workers),
		"vector_stores", len(vectorStores))
	return workers, nil
}

// Helper functions for slice operations and signal type filtering

// stringSliceContains checks if a slice contains a string
func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// isValidSignalType checks if a signal type is valid
func isValidSignalType(signalType string) bool {
	validTypes := []string{"logs", "traces", "metrics", "profiles"}
	return stringSliceContains(validTypes, signalType)
}

// filterValidSignalTypes returns only valid signal types from a slice
func filterValidSignalTypes(signalTypes []string) []string {
	var validSignals []string
	for _, signalType := range signalTypes {
		if isValidSignalType(signalType) {
			validSignals = append(validSignals, signalType)
		}
	}
	return validSignals
}

// WorkerManagerDependencies defines the interface required by ServiceWorkerManager
// to access schema resources and database. This breaks the hard dependency on
// *ServiceResourceManager.
type WorkerManagerDependencies interface {
	// Pools returns the schema resource pools map
	Pools() map[string]*ServiceResources
	// GetDatabase returns the main database connection
	GetDatabase() (*sql.DB, error)
}

// ServiceWorkerManager handles worker operations across all schemas
type ServiceWorkerManager struct {
	deps             WorkerManagerDependencies
	eventBusProvider EventBusProvider
}

// NewServiceWorkerManager creates a new service-level worker manager
func NewServiceWorkerManager(deps WorkerManagerDependencies, eventBusProvider EventBusProvider) *ServiceWorkerManager {
	return &ServiceWorkerManager{
		deps:             deps,
		eventBusProvider: eventBusProvider,
	}
}

// ReloadSchemaWorkers reloads workers for a specific schema with new configurations
func (swm *ServiceWorkerManager) ReloadSchemaWorkers(ctx context.Context, schemaName string, configMgrs *ConfigManagers, enabledProcessors []*dbconfig.ProcessorMappingConfig, logger util.Logger, signalTypes []string, startTime, stopTime *time.Time) error {
	// Get pools through interface method
	pools := swm.deps.Pools()
	resources, exists := pools[schemaName]
	if !exists {
		return fmt.Errorf("schema not found: %s", schemaName)
	}

	// Build new worker configurations
	mainDB, err := swm.deps.GetDatabase()
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}
	dbClient := NewUnifiedSQLDatabaseClient(mainDB)
	embeddingConfigs := make(map[string]dbconfig.EmbeddingConfig)
	processorMappings := make(map[string]dbconfig.ProcessorMappingConfig)

	// Collect configurations for this schema
	var configErrors []string
	for vectorStoreKey := range resources.VectorStore {
		for _, mapping := range enabledProcessors {
			if mapping.DatabaseID <= 0 {
				configErrors = append(configErrors, fmt.Sprintf("Invalid DatabaseID for processor mapping %d", mapping.DatabaseID))
				continue
			}

			if mapping.VectorStoreID <= 0 {
				configErrors = append(configErrors, fmt.Sprintf("Invalid VectorStoreID for processor mapping %d", mapping.VectorStoreID))
				continue
			}

			// Apply signal type filters if provided
			if len(signalTypes) > 0 {
				// Check if this mapping's signal type is in the filter list
				found := false
				for _, signalType := range signalTypes {
					if mapping.EmbeddingFilter != nil {
						if filterSignalType, ok := mapping.EmbeddingFilter["signal_type"].(string); ok && filterSignalType == signalType {
							found = true
							break
						}
					}
				}
				if !found {
					continue // Skip this mapping if signal type not in filter
				}
			}

			// Get vector store config
			vsConfig, err := configMgrs.VectorStore.GetByID(ctx, mapping.VectorStoreID)
			if err != nil {
				configErrors = append(configErrors, fmt.Sprintf("Failed to get vector store config %d: %v", mapping.VectorStoreID, err))
				continue
			}

			if vsConfig != nil {
				expectedKey := fmt.Sprintf("%s_%s", mapping.Name, vsConfig.Name)
				if expectedKey == vectorStoreKey {
					processorMappings[vectorStoreKey] = *mapping
					embConfig, err := configMgrs.Embedding.GetByID(ctx, vsConfig.EmbeddingID)
					if err != nil {
						configErrors = append(configErrors, fmt.Sprintf("failed to get embedding config %d: %v", vsConfig.EmbeddingID, err))
						continue
					}
					embeddingConfigs[vectorStoreKey] = *embConfig
					break
				}
			}
		}
	}

	// Log configuration collection errors
	if len(configErrors) > 0 {
		logger.Error("Configuration collection errors", "schema", schemaName, "errors", configErrors)
	}

	// Build time-based database filter for worker queries
	databaseFilter := map[string]interface{}{}
	if startTime != nil {
		databaseFilter["start_time"] = *startTime
	}
	if stopTime != nil {
		databaseFilter["stop_time"] = *stopTime
	}

	// Build new worker configs with time filtering
	newConfigs, err := buildWorkerConfigs(
		ctx, schemaName, resources.VectorStore, embeddingConfigs,
		processorMappings, databaseFilter, swm.eventBusProvider, dbClient, logger, configMgrs.Template)
	if err != nil {
		return fmt.Errorf("failed to build worker configs: %w", err)
	}

	// Initialize worker manager if not exists
	if resources.WorkerManager == nil {
		resources.WorkerManager = NewWorkerManager(schemaName, logger)
	}

	// Reload workers with new configurations
	return resources.WorkerManager.ReloadWorkers(ctx, newConfigs)
}

// StopAllWorkers stops all workers across all schemas
func (swm *ServiceWorkerManager) StopAllWorkers(logger util.Logger) {
	pools := swm.deps.Pools()
	for schemaName, resources := range pools {
		if resources.WorkerManager != nil {
			resources.WorkerManager.StopWorkers()
			logger.Debug("Stopped workers for schema", zap.String("schema", schemaName))
		} else {
			logger.Debug("No worker manager found for schema", zap.String("schema", schemaName))
		}
	}
}

// StartAllWorkers starts all workers across all schemas
func (swm *ServiceWorkerManager) StartAllWorkers(logger util.Logger) {
	pools := swm.deps.Pools()
	for schemaName, resources := range pools {
		if resources.WorkerManager != nil {
			resources.WorkerManager.StartWorkers()
			logger.Debug("Started workers for schema", zap.String("schema", schemaName))
		} else {
			logger.Debug("No worker manager found for schema", zap.String("schema", schemaName))
		}
	}
}

// GetWorkerStatus returns the status of workers across all schemas
func (swm *ServiceWorkerManager) GetWorkerStatus() map[string]map[string]interface{} {
	status := make(map[string]map[string]interface{})
	pools := swm.deps.Pools()
	for schemaName, resources := range pools {
		schemaStatus := make(map[string]interface{})
		if resources.WorkerManager != nil {
			schemaStatus["worker_count"] = resources.WorkerManager.GetWorkerCount()
			schemaStatus["is_running"] = resources.WorkerManager.IsRunning()
			schemaStatus["manager_type"] = "new"
			schemaStatus["workers"] = resources.WorkerManager.GetWorkerSnapshots()
		} else {
			schemaStatus["worker_count"] = 0
			schemaStatus["is_running"] = false
			schemaStatus["manager_type"] = "uninitialized"
			schemaStatus["workers"] = []WorkerSnapshot{}
		}
		status[schemaName] = schemaStatus
	}
	return status
}
