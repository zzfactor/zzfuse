package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"
	"go.uber.org/zap"
)

// SchemaManagerInterface defines the interface for managing schemas, workers, and templates
type SchemaManagerInterface interface {
	// Lifecycle operations
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Schema operations
	StartSchema(ctx context.Context, schemaName string) error
	StopSchema(ctx context.Context, schemaName string) error
	DeleteSchema(ctx context.Context, schemaName string) error
	CreateSchema(ctx context.Context, schemaConfig *dbconfig.DatabaseSchemaConfig) error

	// Query operations
	GetSchema(serviceName, serviceNamespace string) (string, bool, error)
	GetVectorStore(schemaName string) (VectorStoreClient, error)
	GetRoutingSchema() (DatabaseRoutingSchemaInterface, error)
	GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error)

	// Worker operations
	StartAllWorkers() error
	StopAllWorkers() error
	ReloadSchemaWorkers(ctx context.Context, schemaName string) error
	GetWorkerStatus() map[string]map[string]interface{}

	// Resource access
	GetSchemaPools() map[string]*ServiceResources
	GetSchemaResources(schemaName string) (*ServiceResources, bool)

	// Health and status
	IsRunning() bool
	HealthCheck(ctx context.Context) []ResourceHealthStatus

	// On-demand execution (simplified)
	StartOnDemandWorkers(ctx context.Context, params OnDemandParams) error
	StopOnDemandWorkers(ctx context.Context, schema string) error
}

// SchemaManager manages schema lifecycle, workers, and templates
type SchemaManager struct {
	pools            map[string]*ServiceResources // Schema name -> resources
	routingSchema    DatabaseRoutingSchemaInterface
	templateService  *TemplateService
	workerManager    *ServiceWorkerManager
	mainDB           *sql.DB
	logger           util.Logger
	configMgrs       *ConfigManagers
	eventBusProvider EventBusProvider // Use interface instead of concrete *ServiceResourceManager
	mu               sync.RWMutex
	lifecycleMu      sync.Mutex // Serializes complete Start/Stop transitions.
	workerMu         sync.Mutex // Serializes mutable per-schema worker managers.
	running          bool
	wg               sync.WaitGroup
	closing          bool
}

// Ensure SchemaManager implements the interface
var _ SchemaManagerInterface = (*SchemaManager)(nil)
var _ WorkerManagerDependencies = (*SchemaManager)(nil)

// NewSchemaManager creates a new SchemaManager instance.
// eventBusProvider can be nil if event bus functionality is not needed.
func NewSchemaManager(mainDB *sql.DB, logger util.Logger, configMgrs *ConfigManagers, eventBusProvider EventBusProvider) *SchemaManager {
	sm := &SchemaManager{
		pools:            make(map[string]*ServiceResources),
		mainDB:           mainDB,
		logger:           logger,
		configMgrs:       configMgrs,
		eventBusProvider: eventBusProvider, // Store reference to EventBusProvider interface
	}
	// Depend on this manager's pools/database directly. Calling back through the
	// outer service manager would re-enter its lifecycle gate during shutdown.
	sm.workerManager = NewServiceWorkerManager(sm, eventBusProvider)
	return sm
}

// Start initializes all schemas, routing, templates, and workers
func (sm *SchemaManager) Start(ctx context.Context) error {
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.running {
		return nil // Already running
	}

	// Step 1: Initialize database routing schema
	sm.routingSchema = NewDatabaseRoutingSchemaWithUtilLogger(sm.configMgrs.Config, sm.logger)
	if err := sm.routingSchema.Reset(ctx); err != nil {
		return fmt.Errorf("failed to initialize database routing schema: %w", err)
	}

	// Step 2: Load all enabled schemas and create pools
	enabledSchemas, err := sm.initializeSchemaPools(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize schema pools: %w", err)
	}

	// Step 3: Initialize template service
	if err := sm.initializeTemplateService(ctx, enabledSchemas); err != nil {
		return fmt.Errorf("failed to initialize template service: %w", err)
	}

	// Step 4: Load enabled processors for vector store creation
	enabledProcessors, err := sm.configMgrs.Processor.GetAllEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to load enabled processors: %w", err)
	}

	// Step 5: Create vector stores for all schemas
	if err := sm.createAllVectorStores(ctx, enabledProcessors); err != nil {
		return fmt.Errorf("failed to create vector stores: %w", err)
	}

	// Step 6: Workers are not auto-started - they will only start on-demand
	sm.logger.Info("Workers will only start on-demand - use StartOnDemandWorkers() to start workers")

	// Step 7: Analyze templates for all vector stores
	sm.analyzeTemplatesOnStartup()

	sm.running = true
	sm.logger.Info("Schema manager started successfully",
		zap.Int("schema_count", len(sm.pools)),
		zap.Int("processor_count", len(enabledProcessors)))

	return nil
}

// Stop gracefully shuts down all schemas, workers, and resources
func (sm *SchemaManager) Stop(ctx context.Context) error {
	sm.lifecycleMu.Lock()
	defer sm.lifecycleMu.Unlock()
	sm.mu.Lock()

	if !sm.running && len(sm.pools) == 0 {
		sm.mu.Unlock()
		return nil // Already stopped
	}

	sm.closing = true
	sm.mu.Unlock()
	// Accepted operations may still need mu, including through Pools(). Close
	// the admission gate first, then drain without holding that same lock.
	sm.wg.Wait() // Wait for all operations to complete
	sm.workerMu.Lock()
	defer sm.workerMu.Unlock()

	var closeErrs []error

	// Stop all workers first
	if sm.workerManager != nil {
		sm.workerManager.StopAllWorkers(sm.logger)
	}

	// Close all schema resources
	for schemaName, res := range sm.GetSchemaPools() {
		if err := res.Close(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to close schema %s: %w", schemaName, err))
		}
	}

	sm.mu.Lock()
	sm.running = false
	sm.closing = false
	sm.pools = make(map[string]*ServiceResources)
	sm.routingSchema = nil
	sm.templateService = nil
	sm.mu.Unlock()

	if len(closeErrs) > 0 {
		return fmt.Errorf("errors occurred while stopping schema manager: %v", closeErrs)
	}

	sm.logger.Info("Schema manager stopped successfully")
	return nil
}

// StartSchema starts resources and workers for a specific schema
func (sm *SchemaManager) StartSchema(ctx context.Context, schemaName string) error {
	if !sm.startOp() {
		return fmt.Errorf("schema manager is not running")
	}
	defer sm.endOp()

	sm.workerMu.Lock()
	defer sm.workerMu.Unlock()
	return sm.startSchemaWorkers(ctx, schemaName)
}

// StopSchema stops workers and routing for a specific schema
func (sm *SchemaManager) StopSchema(ctx context.Context, schemaName string) error {
	if !sm.startOp() {
		return fmt.Errorf("schema manager is not running")
	}
	defer sm.endOp()

	sm.workerMu.Lock()
	defer sm.workerMu.Unlock()

	res, exists := sm.GetSchemaResources(schemaName)
	if !exists {
		return fmt.Errorf("schema %s not found", schemaName)
	}

	// Stop workers for this schema
	if res.WorkerManager != nil {
		res.WorkerManager.StopWorkers()
		sm.logger.Info("Stopped workers for schema", zap.String("schema", schemaName))
	}

	return nil
}

// DeleteSchema stops workers for a schema (future: will also clean up resources)
func (sm *SchemaManager) DeleteSchema(ctx context.Context, schemaName string) error {
	if !sm.startOp() {
		return fmt.Errorf("schema manager is not running")
	}
	defer sm.endOp()

	// For now, just stop the workers
	if err := sm.StopSchema(ctx, schemaName); err != nil {
		return fmt.Errorf("failed to stop schema %s: %w", schemaName, err)
	}

	sm.logger.Info("Deleted schema (workers stopped)", zap.String("schema", schemaName))
	return nil
}

// CreateSchema creates a new schema with resources and workers (future implementation)
func (sm *SchemaManager) CreateSchema(ctx context.Context, schemaConfig *dbconfig.DatabaseSchemaConfig) error {
	if !sm.startOp() {
		return fmt.Errorf("schema manager is not running")
	}
	defer sm.endOp()

	// Future implementation: create new schema resources and workers
	return fmt.Errorf("CreateSchema not yet implemented")
}

// GetSchema returns the schema name for given service name and namespace
func (sm *SchemaManager) GetSchema(serviceName, serviceNamespace string) (string, bool, error) {
	var schema string
	var found bool
	err := sm.withOperation(func() error {
		if sm.routingSchema == nil {
			schema, found = "", false
			return nil
		}
		schema, found = sm.routingSchema.GetSchema(serviceName, serviceNamespace)
		return nil
	})
	return schema, found, err
}

// GetVectorStore returns the vector store client for the given schema name
func (sm *SchemaManager) GetVectorStore(schemaName string) (VectorStoreClient, error) {
	var result VectorStoreClient
	err := sm.withOperation(func() error {
		sm.mu.RLock()
		defer sm.mu.RUnlock()

		res, ok := sm.pools[schemaName]
		if !ok || res == nil {
			return fmt.Errorf("no resources found for schema: %s", schemaName)
		}

		// Return the first vector store (could be extended for multiple)
		for _, vs := range res.VectorStore {
			if vs.VectorStore != nil {
				result = vs.VectorStore
				return nil
			}
		}

		return fmt.Errorf("no vector store found for schema: %s", schemaName)
	})
	return result, err
}

// GetRoutingSchema returns the database routing schema interface
func (sm *SchemaManager) GetRoutingSchema() (DatabaseRoutingSchemaInterface, error) {
	var result DatabaseRoutingSchemaInterface
	err := sm.withOperation(func() error {
		result = sm.routingSchema
		return nil
	})
	return result, err
}

// GetTemplateEngine returns the template engine for the given template ID
func (sm *SchemaManager) GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error) {
	var result *content.ContentTemplateEngine
	err := sm.withOperation(func() error {
		if sm.templateService == nil {
			return fmt.Errorf("template service not initialized")
		}

		sm.logger.Debug("Getting template engine", zap.Int64("template_id", templateID))
		var err error
		result, err = sm.templateService.GetTemplateEngine(templateID)
		return err
	})
	return result, err
}

// StartOnDemandWorkers explicitly constructs workers with the requested filters
// and starts them. Schema initialization itself never starts background workers.
func (sm *SchemaManager) StartOnDemandWorkers(ctx context.Context, params OnDemandParams) error {
	return sm.withOperation(func() error {
		params.ApplyDefaults()
		sm.workerMu.Lock()
		defer sm.workerMu.Unlock()
		if params.BatchSize < 0 || params.BatchSize > 10000 {
			return fmt.Errorf("embedding batch size must be between 1 and 10000")
		}
		if err := sm.reloadSchemaWorkers(ctx, params.Schema, params.SignalTypes, params.StartTime, params.StopTime, params.BatchSize); err != nil {
			return fmt.Errorf("failed to reload on-demand workers: %w", err)
		}
		resources, _ := sm.GetSchemaResources(params.Schema)
		resources.WorkerManager.StartWorkers()
		sm.logger.Info("Started on-demand workers", "schema", params.Schema, "signal_types", params.SignalTypes, "start_time", params.StartTime, "stop_time", params.StopTime)
		return nil
	})
}

// StopOnDemandWorkers stops workers for a specific schema.
func (sm *SchemaManager) StopOnDemandWorkers(ctx context.Context, schema string) error {
	return sm.StopSchema(ctx, schema)
}

// StartAllWorkers explicitly starts configured workers across all schemas.
func (sm *SchemaManager) StartAllWorkers() error {
	return sm.withOperation(func() error {
		sm.workerMu.Lock()
		defer sm.workerMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), DatabaseTimeout)
		defer cancel()
		for schemaName := range sm.GetSchemaPools() {
			if err := sm.startSchemaWorkers(ctx, schemaName); err != nil {
				return err
			}
		}
		return nil
	})
}

// StopAllWorkers stops all workers without closing schema resources.
func (sm *SchemaManager) StopAllWorkers() error {
	return sm.withOperation(func() error {
		sm.workerMu.Lock()
		defer sm.workerMu.Unlock()
		sm.workerManager.StopAllWorkers(sm.logger)
		return nil
	})
}

// ReloadSchemaWorkers constructs workers, retaining an existing running state.
// Creating workers for a schema that has never been started does not start them.
func (sm *SchemaManager) ReloadSchemaWorkers(ctx context.Context, schemaName string) error {
	return sm.withOperation(func() error {
		sm.workerMu.Lock()
		defer sm.workerMu.Unlock()
		return sm.reloadSchemaWorkers(ctx, schemaName, nil, nil, nil)
	})
}

// reloadSchemaWorkers requires workerMu. Dependencies may acquire mu, so callers
// must never hold mu across these configuration and worker operations.
func (sm *SchemaManager) reloadSchemaWorkers(ctx context.Context, schemaName string, signalTypes []string, startTime, stopTime *time.Time, batchSize ...int) error {
	resources, exists := sm.GetSchemaResources(schemaName)
	if !exists {
		return fmt.Errorf("schema %s not found", schemaName)
	}
	if sm.configMgrs == nil || sm.configMgrs.Processor == nil || sm.configMgrs.VectorStore == nil || sm.configMgrs.Embedding == nil {
		return fmt.Errorf("config managers not initialized")
	}
	enabledProcessors, err := sm.configMgrs.Processor.GetAllEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to load enabled processors: %w", err)
	}
	// Vector-store keys can repeat across tenants. Only this schema's mappings
	// may participate in its worker construction.
	processors := make([]*dbconfig.ProcessorMappingConfig, 0, len(enabledProcessors))
	for _, processor := range enabledProcessors {
		if processor.DatabaseID == resources.SchemaID {
			processors = append(processors, processor)
		}
	}
	return sm.workerManager.ReloadSchemaWorkers(ctx, schemaName, sm.configMgrs, processors, sm.logger, signalTypes, startTime, stopTime, batchSize...)
}

// startSchemaWorkers requires workerMu. A stopped worker's cancellation channel
// cannot reopen, so explicit restarts reconstruct stopped workers first.
func (sm *SchemaManager) startSchemaWorkers(ctx context.Context, schemaName string) error {
	resources, exists := sm.GetSchemaResources(schemaName)
	if !exists {
		return fmt.Errorf("schema %s not found", schemaName)
	}
	needsReload := resources.WorkerManager == nil
	if manager := resources.WorkerManager; manager != nil {
		manager.mu.RLock()
		for _, worker := range manager.workers {
			if worker.IsStopped() {
				needsReload = true
				break
			}
		}
		manager.mu.RUnlock()
	}
	if needsReload {
		if err := sm.reloadSchemaWorkers(ctx, schemaName, nil, nil, nil); err != nil {
			return err
		}
	}
	resources.WorkerManager.StartWorkers()
	sm.logger.Info("Started workers for schema", zap.String("schema", schemaName))
	return nil
}

// GetWorkerStatus returns copied production worker snapshots for each schema.
func (sm *SchemaManager) GetWorkerStatus() map[string]map[string]interface{} {
	if !sm.startOp() {
		return make(map[string]map[string]interface{})
	}
	defer sm.endOp()
	sm.workerMu.Lock()
	defer sm.workerMu.Unlock()
	return sm.workerManager.GetWorkerStatus()
}

// Pools and GetDatabase satisfy WorkerManagerDependencies without entering the
// outer service manager's lifecycle gate. Accepted operations own the lifetime.
func (sm *SchemaManager) Pools() map[string]*ServiceResources { return sm.GetSchemaPools() }
func (sm *SchemaManager) GetDatabase() (*sql.DB, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.mainDB == nil {
		return nil, fmt.Errorf("no database found")
	}
	return sm.mainDB, nil
}

// GetSchemaPools returns a copy of the pools map for read-only access
func (sm *SchemaManager) GetSchemaPools() map[string]*ServiceResources {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	copy := make(map[string]*ServiceResources, len(sm.pools))
	for k, v := range sm.pools {
		copy[k] = v
	}
	return copy
}

// GetSchemaResources returns the resources for a specific schema
func (sm *SchemaManager) GetSchemaResources(schemaName string) (*ServiceResources, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	res, exists := sm.pools[schemaName]
	return res, exists
}

// IsRunning returns true if the schema manager is currently running
func (sm *SchemaManager) IsRunning() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.running && !sm.closing
}

// HealthCheck checks the health of all managed schemas and returns their status
func (sm *SchemaManager) HealthCheck(ctx context.Context) []ResourceHealthStatus {
	if !sm.startOp() {
		return []ResourceHealthStatus{}
	}
	defer sm.endOp()

	var statuses []ResourceHealthStatus
	now := time.Now()

	// Provider callbacks may read this manager. The accepted-operation gate
	// protects resource lifetime without holding mu across external calls.
	for schemaName, res := range sm.GetSchemaPools() {
		// Check vector stores and embeddings for each schema
		for vsName, vs := range res.VectorStore {
			if vs.VectorStore != nil {
				err := vs.VectorStore.HealthCheck(ctx)
				statuses = append(statuses, ResourceHealthStatus{
					Name:    schemaName + ":vectorstore:" + vsName,
					Type:    "vectorstore",
					Healthy: err == nil,
					LastError: func() string {
						if err != nil {
							return err.Error()
						}
						return ""
					}(),
					LastCheck: now,
				})
			}
		}
	}

	return statuses
}

// Private helper methods

// startOp increments the WaitGroup if not closing, returns false if closing
func (sm *SchemaManager) startOp() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	if sm.closing || !sm.running {
		return false
	}
	sm.wg.Add(1)
	return true
}

// endOp decrements the WaitGroup
func (sm *SchemaManager) endOp() {
	sm.wg.Done()
}

// withOperation executes a function within the operation lifecycle pattern
func (sm *SchemaManager) withOperation(fn func() error) error {
	if !sm.startOp() {
		return fmt.Errorf("schema manager is not running")
	}
	defer sm.endOp()
	return fn()
}

// initializeSchemaPools loads all enabled schemas and creates service resource pools
func (sm *SchemaManager) initializeSchemaPools(ctx context.Context) ([]*dbconfig.DatabaseSchemaConfig, error) {
	// Load all enabled schemas from config_otel_database
	enabledSchemas, err := sm.routingSchema.GetAllEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load enabled schemas: %w", err)
	}

	// Process each enabled schema to create service resource pools
	for _, schemaConfig := range enabledSchemas {
		// Register resources by schema name
		_, found := sm.pools[schemaConfig.DatabaseSchema]
		if !found {
			resources := &ServiceResources{
				SchemaID:    schemaConfig.ID,
				VectorStore: make(map[string]*VectorStore),
				Priority:    schemaConfig.Priority,
				logger:      sm.logger,
			}
			sm.pools[schemaConfig.DatabaseSchema] = resources
		}
	}

	return enabledSchemas, nil
}

// initializeTemplateService initializes the template service with schema mappings
func (sm *SchemaManager) initializeTemplateService(ctx context.Context, enabledSchemas []*dbconfig.DatabaseSchemaConfig) error {
	// Load enabled processors for template service
	enabledProcessors, err := sm.configMgrs.Processor.GetAllEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to load enabled processors: %w", err)
	}

	// Build schema map
	schemaMap := make(map[int64]string)
	for _, schema := range enabledSchemas {
		schemaMap[schema.ID] = schema.DatabaseSchema
	}

	sm.templateService, err = NewTemplateService(ctx, sm.logger, sm.configMgrs.Template, enabledProcessors, schemaMap, nil)
	if err != nil {
		return fmt.Errorf("failed to create template service: %w", err)
	}

	return nil
}

// createAllVectorStores creates vector stores for all enabled processors
func (sm *SchemaManager) createAllVectorStores(ctx context.Context, enabledProcessors []*dbconfig.ProcessorMappingConfig) error {
	sm.logger.Info("Creating vector stores for all schemas",
		zap.Int("processor_count", len(enabledProcessors)),
		zap.Int("schema_count", len(sm.pools)))

	for _, vectorstore := range enabledProcessors {
		if vectorstore.VectorStoreID == 0 {
			continue
		}

		// Get the schema config for this mapping
		schema, err := sm.configMgrs.Processor.GetSchemaById(ctx, vectorstore.DatabaseID)
		if err != nil || schema == nil {
			sm.logger.Error("Failed to get schema config", zap.Int64("database_id", vectorstore.DatabaseID), zap.Error(err))
			continue
		}

		// Get the resources for this schema
		res, found := sm.pools[schema.DatabaseSchema]
		if !found {
			sm.logger.Error("Schema not found in pools", zap.String("schema", schema.DatabaseSchema))
			continue
		}

		if err := sm.createVectorStoreForSchema(ctx, vectorstore, schema, res); err != nil {
			sm.logger.Error("Failed to create vector store for schema",
				zap.String("schema", schema.DatabaseSchema),
				zap.String("mapping", vectorstore.Name),
				zap.Error(err))
			continue
		}
	}

	return nil
}

// createVectorStoreForSchema creates a vector store for a specific schema
func (sm *SchemaManager) createVectorStoreForSchema(ctx context.Context, vectorstore *dbconfig.ProcessorMappingConfig, schema *dbconfig.DatabaseSchemaConfig, res *ServiceResources) error {
	// Get the vector store config by ID
	vsConfig, err := sm.configMgrs.VectorStore.GetByID(ctx, vectorstore.VectorStoreID)
	if err != nil {
		return fmt.Errorf("failed to get vector store config: %w", err)
	}

	// Get the embedding config by ID
	embConfig, err := sm.configMgrs.Embedding.GetByID(ctx, vsConfig.EmbeddingID)
	if err != nil {
		return fmt.Errorf("failed to get embedding config: %w", err)
	}

	// Parse TimescaleDB maintenance configuration
	timescaleConfig := dbconfig.ParseTimescaleDBMaintenanceFromMetadata(vectorstore.Metadata)

	// Create the embedding processor
	embProcessor, err := NewEmbeddingProcessor(ctx, embConfig)
	if err != nil {
		return fmt.Errorf("failed to create embedding provider: %w", err)
	}

	// Create the vector store
	vsRepository, err := NewVectorStore(ctx, schema.DatabaseSchema, vectorstore.Name, *vsConfig, embProcessor, embConfig, sm.mainDB, sm.logger, &timescaleConfig)
	if err != nil {
		return fmt.Errorf("failed to create vector store client: %w", err)
	}

	vs := &VectorStore{
		VectorStore: vsRepository,
		Embedding:   embProcessor,
		logger:      sm.logger,
	}

	// Use a unique key that combines mapping name and vector store name
	vectorStoreKey := fmt.Sprintf("%s_%s", vectorstore.Name, vsConfig.Name)
	res.VectorStore[vectorStoreKey] = vs

	sm.logger.Info("Created vector store",
		zap.String("mapping_name", vectorstore.Name),
		zap.String("vectorstore_name", vsConfig.Name),
		zap.String("vectorstore_key", vectorStoreKey),
		zap.String("schema", schema.DatabaseSchema))

	return nil
}

// analyzeTemplatesOnStartup analyzes templates for all vector stores
func (sm *SchemaManager) analyzeTemplatesOnStartup() {
	for _, res := range sm.pools {
		_ = sm.templateService.AnalyzeTemplatesOnStartup(res.VectorStore, sm.logger)
	}
}
