package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"

	"go.uber.org/zap"
)

// Service lifecycle errors
var (
	ErrServiceManagerStopped        = errors.New("service manager is stopped or shutting down")
	ErrServiceManagerNotInitialized = errors.New("service manager not properly initialized")
)

const (
	DefaultEventBusBufferSize  = 1000
	DefaultEventBusWorkerCount = 10
)

// Ensure ServiceResourceManager implements the interface
var _ ServiceResourceManagerInterface = (*ServiceResourceManager)(nil)

// ServiceResourceManager manages per-service resources: DB connections, embedding clients, and vector store clients.
// It is designed for extensibility, thread safety, and future observability/metrics integration.
// WARNING: Do not instantiate this struct directly. Use NewServiceResourceManager() instead.
// Clients should only access this through ServiceResourceManagerInterface.

type ServiceResourceManager struct {
	mainDB        *sql.DB
	logger        util.Logger
	mu            sync.RWMutex
	eventBus      event.EventBusInterface
	schemaManager SchemaManagerInterface // NEW: Dedicated schema manager
	// Resource cleanup
	wg      sync.WaitGroup
	closing bool

	// Cached services - initialized once via sync.Once for thread-safety
	unifiedDBOnce sync.Once
	unifiedDB     *OtelSQLDatabaseClient

	// Writer services
	logWriterOnce     sync.Once
	logWriter         LogWriterService
	metricWriterOnce  sync.Once
	metricWriter      MetricWriterService
	traceWriterOnce   sync.Once
	traceWriter       TraceWriterService
	profileWriterOnce sync.Once
	profileWriter     ProfileWriterService

	// Query services
	logQueryOnce     sync.Once
	logQuery         LogQueryService
	metricQueryOnce  sync.Once
	metricQuery      MetricQueryService
	traceQueryOnce   sync.Once
	traceQuery       TraceQueryService
	profileQueryOnce sync.Once
	profileQuery     ProfileQueryService
	runWriterOnce    sync.Once
	runWriter        RunWriterService
	runQueryOnce     sync.Once
	runQuery         RunQueryService
}

// ServiceResources holds all resources for a given service name, and includes the filter for routing.
//
// Ownership model:
// - VectorStore: Each VectorStore is closed via its Close method, which respects its own ownership flag
// - EmbeddingClient: Always closed if it implements io.Closer (no sharing)
// - EmbeddingWorkers: Always stopped/cleaned up if present
type ServiceResources struct {
	SchemaID      int64                   // Database schema ID from config_otel_dbconfig.id
	VectorStore   map[string]*VectorStore // e.g., "vectorstore1", etc.
	Priority      int                     // from DatabaseSchemaConfig.Priority
	WorkerManager *WorkerManager          // Lifecycle manager for workers
	logger        util.Logger             // Structured logger
}

// VectorStore represents a single vector store destination for a service.
//
// Ownership model:
// - Embedding: Always closed if it implements io.Closer (no sharing)
type VectorStore struct {
	VectorStore VectorStoreClient
	Embedding   EmbeddingClient
	logger      util.Logger
	// EmbeddingWorkers removed - now managed at schema level
}

// TemplateServiceInterface defines the interface for template services
type TemplateServiceInterface interface {
	GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error)
	AnalyzeTemplatesOnStartup(vectorStores map[string]*VectorStore, logger util.Logger) map[string]*TemplateAnalysis
}

// ServiceResourceManagerInterface defines the interface for managing service resources
type ServiceResourceManagerInterface interface {
	// Shutdown gracefully shuts down all services and cleans up resources
	Shutdown() error

	// IsRunning returns true if the service manager is currently running
	IsRunning() bool

	// GetDatabase returns the main database connection
	GetDatabase() (*sql.DB, error)

	// GetEventBus returns the event bus interface for event-driven architecture
	GetEventBus() (event.EventBusInterface, error)

	// GetVectorStore returns the vector store client for the given service name.
	GetVectorStore(serviceName string) (VectorStoreClient, error)

	// GetRoutingSchema returns the database routing schema interface
	GetRoutingSchema() (DatabaseRoutingSchemaInterface, error)

	// GetSchema returns the schema name for given service name and namespace
	GetSchema(serviceName, serviceNamespace string) (string, bool, error)

	// GetTemplateEngine returns the template engine for the given template ID
	GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error)

	// Repository service methods - ready-to-use services
	GetLogWriter() (LogWriterService, error)
	GetMetricWriter() (MetricWriterService, error)
	GetTraceWriter() (TraceWriterService, error)
	GetProfileWriter() (ProfileWriterService, error)

	// Query service methods - ready-to-use query services
	GetLogQuery() (LogQueryService, error)
	GetMetricQuery() (MetricQueryService, error)
	GetTraceQuery() (TraceQueryService, error)
	GetProfileQuery() (ProfileQueryService, error)
	GetRunWriter() (RunWriterService, error)
	GetRunQuery() (RunQueryService, error)
}

// NewServiceResourceManager creates a new fully initialized ServiceResourceManager.
// Each call creates a fresh instance - there is no singleton pattern.
// The caller (typically TrazrExtension) is responsible for managing the instance lifecycle.
func NewServiceResourceManager(ctx context.Context, logger util.Logger) (*ServiceResourceManager, error) {
	srm := &ServiceResourceManager{}
	return initializeServiceResourceManager(ctx, logger, srm)
}

// initializeServiceResourceManager initializes the provided ServiceResourceManager instance.
func initializeServiceResourceManager(ctx context.Context, logger util.Logger, srm *ServiceResourceManager) (*ServiceResourceManager, error) {
	// Step 1: Initialize database connection
	dsnMain, err := GetDSN()
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration: %w", err)
	}

	// Convert util.Logger to *zap.Logger if needed
	var zapLogger *zap.Logger
	if adapter, ok := logger.(*util.ZapLoggerAdapter); ok {
		zapLogger = adapter.Zap
	} else {
		// Create a default zap logger if conversion fails
		zapLogger, _ = zap.NewProduction()
	}

	db, err := OpenPostgressDatabase(ctx, dsnMain, zapLogger)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Step 2: Initialize event bus
	eventBus, err := initializeEventBus(ctx, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize event bus: %w", err)
	}

	// Step 3: Set basic fields
	srm.mainDB = db
	srm.logger = logger
	srm.eventBus = eventBus

	// Step 4: Initialize schema manager
	configMgrs, err := LoadConfig(ctx, db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	srm.schemaManager = NewSchemaManager(db, logger, configMgrs, srm)
	if err := srm.schemaManager.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start schema manager: %w", err)
	}

	return srm, nil
}

// Legacy Start(), Stop(), and IsRunning() functions removed.
// Use NewServiceResourceManager() instead for creating service managers.

// initializeEventBus initializes the EventBus for the ServiceResourceManager if not already set
func initializeEventBus(ctx context.Context, logger util.Logger) (event.EventBusInterface, error) {
	// Get configuration from environment
	config := event.GetEventBusConfig()

	// Use configuration-based event bus creation with proper parameters
	factory := event.NewEventBusFactory(event.EventBusType(config.Type), config.NATS.URL)
	eventBus, err := factory.CreateEventBus(ctx)
	if err != nil {
		logger.Error("Failed to create event bus from configuration", zap.Error(err), zap.String("type", config.Type))
		// Fallback to default in-memory event bus using factory
		fallbackFactory := event.NewEventBusFactory(event.EventBusTypeInMemory, "")
		fallbackBus, fallbackErr := fallbackFactory.CreateEventBus(ctx)
		if fallbackErr != nil {
			logger.Error("Failed to create fallback event bus", zap.Error(fallbackErr))
			return nil, fmt.Errorf("failed to create event bus: %w", err)
		}
		eventBus = fallbackBus
		logger.Info("Fallback in-memory event bus created successfully", zap.String("type", "in-memory"))
	} else {
		// Use the event bus interface directly - supports both in-memory and NATS
		logger.Info("Event bus created successfully from configuration",
			zap.String("type", config.Type),
			zap.String("nats_url", config.NATS.URL))
	}

	return eventBus, nil
}

// GetSchema returns the schema name for given service name and namespace
func (srm *ServiceResourceManager) GetSchema(serviceName, serviceNamespace string) (string, bool, error) {
	if !srm.startOp() {
		return "", false, ErrServiceManagerStopped
	}
	defer srm.endOp()

	// Delegate to schema manager
	if srm.schemaManager == nil {
		return "", false, fmt.Errorf("schema manager not initialized")
	}

	return srm.schemaManager.GetSchema(serviceName, serviceNamespace)
}

// GetTemplateEngine returns the template engine for the given template ID
func (srm *ServiceResourceManager) GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	if srm.schemaManager == nil {
		return nil, fmt.Errorf("schema manager not initialized")
	}

	srm.logger.Debug("Getting template engine", zap.Int64("template_id", templateID))
	return srm.schemaManager.GetTemplateEngine(templateID)
}

// startOp increments the WaitGroup if not closing, returns false if closing
func (srm *ServiceResourceManager) startOp() bool {
	srm.mu.RLock()
	defer srm.mu.RUnlock()
	if srm.closing {
		return false
	}
	srm.wg.Add(1)
	return true
}

// endOp decrements the WaitGroup
func (srm *ServiceResourceManager) endOp() {
	srm.wg.Done()
}

// close waits for all in-flight operations to finish, then closes all resources (internal method)
func (srm *ServiceResourceManager) close() error {
	srm.mu.Lock()
	srm.closing = true
	srm.mu.Unlock()
	srm.wg.Wait()
	// Now safe to close resources
	srm.mu.Lock()
	defer srm.mu.Unlock()
	var closeErrs []error

	// Close event bus first to prevent new operations
	if srm.eventBus != nil {
		if closer, ok := srm.eventBus.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				closeErrs = append(closeErrs, fmt.Errorf("failed to close event bus: %w", err))
			}
		}
	}

	// Close schema manager
	if srm.schemaManager != nil {
		if err := srm.schemaManager.Stop(context.Background()); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to close schema manager: %w", err))
		}
	}

	// Close main database connection last (reverse order of initialization)
	if srm.mainDB != nil {
		if err := srm.mainDB.Close(); err != nil {
			closeErrs = append(closeErrs, fmt.Errorf("failed to close main database: %w", err))
		}
		srm.mainDB = nil
	}

	if len(closeErrs) > 0 {
		return fmt.Errorf("errors occurred while closing resources: %v", closeErrs)
	}
	return nil
}

// HealthCheck checks the health of all managed resources and returns their status.
func (srm *ServiceResourceManager) HealthCheck(ctx context.Context) []ResourceHealthStatus {
	srm.mu.RLock()
	defer srm.mu.RUnlock()
	var statuses []ResourceHealthStatus
	now := time.Now()

	// Check DB health
	if srm.mainDB != nil {
		err := srm.mainDB.PingContext(ctx)
		statuses = append(statuses, ResourceHealthStatus{
			Type:    "db",
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

	// Delegate schema health checks to schema manager
	if srm.schemaManager != nil {
		schemaStatuses := srm.schemaManager.HealthCheck(ctx)
		statuses = append(statuses, schemaStatuses...)
	}

	return statuses
}

func (sr *ServiceResources) Close() error {
	var closeErrs []error

	// Stop schema-level workers first
	if sr.WorkerManager != nil {
		sr.WorkerManager.StopWorkers()
	}

	// Then close vector stores
	for name, vs := range sr.VectorStore {
		if err := vs.Close(); err != nil {
			closeErrs = append(closeErrs, err)
			if sr.logger != nil {
				sr.logger.Error("Error closing VectorStore", zap.String("name", name), zap.Error(err))
			}
		}
	}
	if len(closeErrs) > 0 {
		return errors.Join(closeErrs...)
	}
	return nil
}

func (vs *VectorStore) Close() error {
	var closeErrs []error
	if c, ok := vs.VectorStore.(io.Closer); ok {
		if err := c.Close(); err != nil {
			closeErrs = append(closeErrs, err)
			if vs.logger != nil {
				vs.logger.Error("Error closing VectorStore", zap.Error(err))
			}
		}
		if vs.logger != nil {
			vs.logger.Debug("Closed owned VectorStore")
		}
	}
	if c, ok := vs.Embedding.(io.Closer); ok {
		if err := c.Close(); err != nil {
			closeErrs = append(closeErrs, err)
			if vs.logger != nil {
				vs.logger.Error("Error closing Embedding", zap.Error(err))
			}
		}
		if vs.logger != nil {
			vs.logger.Debug("Closed Embedding")
		}
	}
	// Workers are now managed at schema level, not vector store level
	if len(closeErrs) > 0 {
		return errors.Join(closeErrs...)
	}
	return nil
}

// GetVectorStore returns the vector store client for the given service name.
func (srm *ServiceResourceManager) GetVectorStore(serviceName string) (VectorStoreClient, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	// Delegate to schema manager
	if srm.schemaManager == nil {
		return nil, fmt.Errorf("schema manager not initialized")
	}

	return srm.schemaManager.GetVectorStore(serviceName)
}

// GetDatabase returns the *sql.DB for the given service name, or mainDB as fallback.
func (srm *ServiceResourceManager) GetDatabase() (*sql.DB, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	if srm.mainDB != nil {
		return srm.mainDB, nil
	}
	return nil, fmt.Errorf("no database found")
}

// Pools returns a copy of the pools map for read-only access (for listing vector stores, etc.).
func (srm *ServiceResourceManager) Pools() map[string]*ServiceResources {
	if !srm.startOp() {
		return make(map[string]*ServiceResources)
	}
	defer srm.endOp()

	// Delegate to schema manager
	if srm.schemaManager == nil {
		return make(map[string]*ServiceResources)
	}

	return srm.schemaManager.GetSchemaPools()
}

// getUnifiedDB returns a cached unified database client, creating it once if needed.
// This avoids creating a new client on every Get*() call.
func (srm *ServiceResourceManager) getUnifiedDB() (*OtelSQLDatabaseClient, error) {
	var initErr error
	srm.unifiedDBOnce.Do(func() {
		if srm.mainDB == nil {
			initErr = fmt.Errorf("database not initialized")
			return
		}
		srm.unifiedDB = NewUnifiedSQLDatabaseClient(srm.mainDB)
	})
	if initErr != nil {
		return nil, initErr
	}
	if srm.unifiedDB == nil {
		return nil, fmt.Errorf("unified database client not initialized")
	}
	return srm.unifiedDB, nil
}

// GetLogWriter returns a ready-to-use LogWriterService (cached)
func (srm *ServiceResourceManager) GetLogWriter() (LogWriterService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.logWriterOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		// Create log writer with event bus support if available
		if eb := srm.eventBus; eb != nil {
			srm.logWriter = NewLogWriterServiceWithUnifiedClientAndEventBus(unifiedDB, eb)
		} else {
			srm.logWriter = NewLogWriterServiceWithUnifiedClient(unifiedDB)
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.logWriter, nil
}

// GetMetricWriter returns a ready-to-use MetricWriterService (cached)
func (srm *ServiceResourceManager) GetMetricWriter() (MetricWriterService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.metricWriterOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		if eb := srm.eventBus; eb != nil {
			srm.metricWriter = NewMetricWriterServiceWithUnifiedClientAndEventBus(unifiedDB, eb)
		} else {
			srm.metricWriter = NewMetricWriterServiceWithUnifiedClient(unifiedDB)
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.metricWriter, nil
}

// GetTraceWriter returns a ready-to-use TraceWriterService (cached)
func (srm *ServiceResourceManager) GetTraceWriter() (TraceWriterService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.traceWriterOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		if eb := srm.eventBus; eb != nil {
			srm.traceWriter = NewTraceWriterServiceWithUnifiedClientAndEventBus(unifiedDB, eb)
		} else {
			srm.traceWriter = NewTraceWriterServiceWithUnifiedClient(unifiedDB)
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.traceWriter, nil
}

// GetProfileWriter returns a ready-to-use ProfileWriterService (cached)
func (srm *ServiceResourceManager) GetProfileWriter() (ProfileWriterService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.profileWriterOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		routingSchema, _ := srm.GetRoutingSchema()
		if eb := srm.eventBus; eb != nil {
			srm.profileWriter = NewProfileWriterServiceWithUnifiedClientAndEventBus(unifiedDB, eb, routingSchema)
		} else {
			srm.profileWriter = NewProfileWriterServiceWithUnifiedClient(unifiedDB)
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.profileWriter, nil
}

// GetLogQuery returns a ready-to-use LogQueryService (cached)
func (srm *ServiceResourceManager) GetLogQuery() (LogQueryService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.logQueryOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		srm.logQuery = NewLogQueryServiceWithUnifiedClient(unifiedDB)
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.logQuery, nil
}

// GetMetricQuery returns a ready-to-use MetricQueryService (cached)
func (srm *ServiceResourceManager) GetMetricQuery() (MetricQueryService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.metricQueryOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		srm.metricQuery = NewMetricQueryServiceWithUnifiedClient(unifiedDB)
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.metricQuery, nil
}

// GetTraceQuery returns a ready-to-use TraceQueryService (cached)
func (srm *ServiceResourceManager) GetTraceQuery() (TraceQueryService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.traceQueryOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		srm.traceQuery = NewTraceQueryServiceWithUnifiedClient(unifiedDB)
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.traceQuery, nil
}

// GetProfileQuery returns a ready-to-use ProfileQueryService (cached)
func (srm *ServiceResourceManager) GetProfileQuery() (ProfileQueryService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.profileQueryOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		srm.profileQuery = NewProfileQueryServiceWithUnifiedClient(unifiedDB)
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.profileQuery, nil
}

// GetRunWriter returns a ready-to-use RunWriterService (cached)
func (srm *ServiceResourceManager) GetRunWriter() (RunWriterService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.runWriterOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		srm.runWriter = NewRunWriterServiceWithUnifiedClient(unifiedDB)
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.runWriter, nil
}

// GetRunQuery returns a ready-to-use RunQueryService (cached)
func (srm *ServiceResourceManager) GetRunQuery() (RunQueryService, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	var initErr error
	srm.runQueryOnce.Do(func() {
		unifiedDB, err := srm.getUnifiedDB()
		if err != nil {
			initErr = fmt.Errorf("failed to get database client: %w", err)
			return
		}
		srm.runQuery = NewRunQueryServiceWithUnifiedClient(unifiedDB)
	})
	if initErr != nil {
		return nil, initErr
	}
	return srm.runQuery, nil
}

// GetEventBus returns the event bus interface for event-driven architecture
func (srm *ServiceResourceManager) GetEventBus() (event.EventBusInterface, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	return srm.eventBus, nil
}

// GetRoutingSchema returns the database routing schema interface
func (srm *ServiceResourceManager) GetRoutingSchema() (DatabaseRoutingSchemaInterface, error) {
	if !srm.startOp() {
		return nil, ErrServiceManagerStopped
	}
	defer srm.endOp()

	// Delegate to schema manager
	if srm.schemaManager == nil {
		return nil, fmt.Errorf("schema manager not initialized")
	}

	return srm.schemaManager.GetRoutingSchema()
}

// ReloadSchemaWorkers reloads workers for a specific schema
func (srm *ServiceResourceManager) ReloadSchemaWorkers(ctx context.Context, schemaName string) error {
	if srm.schemaManager == nil {
		return fmt.Errorf("schema manager not initialized")
	}
	return srm.schemaManager.ReloadSchemaWorkers(ctx, schemaName)
}

// StopAllWorkers stops all workers across all schemas
func (srm *ServiceResourceManager) StopAllWorkers() {
	if srm.schemaManager != nil {
		srm.schemaManager.StopAllWorkers()
	}
}

// StartAllWorkers starts all workers across all schemas
func (srm *ServiceResourceManager) StartAllWorkers() {
	if srm.schemaManager != nil {
		srm.schemaManager.StartAllWorkers()
	}
}

// GetWorkerStatus returns the status of workers across all schemas
func (srm *ServiceResourceManager) GetWorkerStatus() map[string]map[string]interface{} {
	if srm.schemaManager != nil {
		return srm.schemaManager.GetWorkerStatus()
	}
	return make(map[string]map[string]interface{})
}

// Shutdown gracefully shuts down all services and cleans up resources.
// This is an instance-level method that shuts down this specific ServiceResourceManager.
func (srm *ServiceResourceManager) Shutdown() error {
	return srm.close()
}

// IsRunning returns true if this service manager instance is currently running and initialized.
// This is an instance-level method that checks if the current ServiceResourceManager is operational.
func (srm *ServiceResourceManager) IsRunning() bool {
	srm.mu.RLock()
	defer srm.mu.RUnlock()
	return !srm.closing && srm.mainDB != nil && srm.eventBus != nil && srm.schemaManager != nil && srm.schemaManager.IsRunning()
}
