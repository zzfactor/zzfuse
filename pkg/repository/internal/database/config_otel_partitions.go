package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

// TimescaleDBMaintenanceManager handles TimescaleDB maintenance for OTEL signal tables
type TimescaleDBMaintenanceManager struct {
	db            *sql.DB
	logger        util.Logger
	configService *TimescaleDBConfigService
	ticker        *time.Ticker
	stopped       chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
}

// NewTimescaleDBMaintenanceManager creates a new TimescaleDB maintenance manager
func NewTimescaleDBMaintenanceManager(db *sql.DB, logger util.Logger) *TimescaleDBMaintenanceManager {
	return &TimescaleDBMaintenanceManager{
		db:            db,
		logger:        logger,
		configService: NewTimescaleDBConfigService(db, logger),
		stopped:       make(chan struct{}),
	}
}

// Start begins TimescaleDB maintenance
func (mm *TimescaleDBMaintenanceManager) Start(ctx context.Context) {
	if mm.db == nil {
		if mm.logger != nil {
			mm.logger.Error("Cannot start TimescaleDB maintenance manager: database connection is nil")
		}
		return
	}

	if mm.logger != nil {
		mm.logger.Info("Starting TimescaleDB maintenance manager")
	}

	// Start maintenance goroutine
	mm.wg.Add(1)
	go mm.startMaintenance(ctx)
}

// Stop gracefully stops the maintenance goroutine
func (mm *TimescaleDBMaintenanceManager) Stop() {
	if mm.logger != nil {
		mm.logger.Info("Stopping TimescaleDB maintenance manager")
	}

	// Close stopped channel if not already closed
	select {
	case <-mm.stopped:
		// Already closed
	default:
		close(mm.stopped)
	}

	// Stop ticker
	mm.mu.Lock()
	if mm.ticker != nil {
		mm.ticker.Stop()
	}
	mm.mu.Unlock()

	// Wait for goroutine to finish
	mm.wg.Wait()
	if mm.logger != nil {
		mm.logger.Info("TimescaleDB maintenance manager stopped")
	}
}

// startMaintenance runs maintenance with configurable scheduling
func (mm *TimescaleDBMaintenanceManager) startMaintenance(ctx context.Context) {
	defer mm.wg.Done()

	// Run initial maintenance on startup
	mm.logger.Info("Running initial TimescaleDB maintenance")
	if err := mm.runMaintenance(ctx); err != nil {
		mm.logger.Error("Failed to run initial TimescaleDB maintenance", "error", err)
	}

	// Get configuration for maintenance interval
	config := mm.configService.GetDefaultConfig()
	interval := time.Duration(config.MaintenanceIntervalHours) * time.Hour

	// Create ticker for periodic execution
	mm.mu.Lock()
	mm.ticker = time.NewTicker(interval)
	ticker := mm.ticker
	mm.mu.Unlock()
	defer ticker.Stop()

	mm.logger.Info("TimescaleDB maintenance job started",
		"interval", interval.String())

	for {
		select {
		case <-ctx.Done():
			mm.logger.Info("TimescaleDB maintenance job stopped")
			return
		case <-mm.stopped:
			mm.logger.Info("TimescaleDB maintenance job stopped")
			return
		case <-ticker.C:
			mm.logger.Info("Running scheduled TimescaleDB maintenance")
			if err := mm.runMaintenance(ctx); err != nil {
				mm.logger.Error("Failed to run TimescaleDB maintenance", "error", err)
			}
		}
	}
}

// runMaintenance runs maintenance for all enabled OTEL schemas
func (mm *TimescaleDBMaintenanceManager) runMaintenance(ctx context.Context) error {
	// Get all enabled schemas
	schemaRepo, err := NewDatabaseSchemaConfig(ctx, mm.db, mm.logger, DefaultOtelSchema)

	if err != nil {
		return fmt.Errorf("failed to get enabled schemas: %w", err)
	}
	enabledSchemas, err := schemaRepo.GetAllEnabled(ctx)
	if err != nil {
		return fmt.Errorf("failed to get enabled schemas: %w", err)
	}

	if len(enabledSchemas) == 0 {
		mm.logger.Info("No enabled schemas found for TimescaleDB maintenance")
		return nil
	}

	mm.logger.Info("Running TimescaleDB maintenance for schemas",
		"schema_count", len(enabledSchemas))

	// Run maintenance for each schema
	for _, schema := range enabledSchemas {
		config := mm.configService.GetConfig(schema.DatabaseSchema)

		if !config.Enabled {
			mm.logger.Debug("TimescaleDB maintenance disabled, skipping schema",
				"schema", schema.DatabaseSchema)
			continue
		}

		mm.logger.Info("Running TimescaleDB maintenance for schema",
			"schema", schema.DatabaseSchema,
			"retention_days", config.RetentionDays)

		if err := mm.executeMaintenance(ctx, schema.DatabaseSchema, config); err != nil {
			mm.logger.Error("Failed to run TimescaleDB maintenance for schema",
				"schema", schema.DatabaseSchema,
				"error", err)
			// Continue with other schemas even if one fails
		} else {
			mm.logger.Info("TimescaleDB maintenance completed for schema",
				"schema", schema.DatabaseSchema)
		}
	}

	return nil
}

// TriggerMaintenance manually triggers TimescaleDB maintenance
func (mm *TimescaleDBMaintenanceManager) TriggerMaintenance(ctx context.Context) error {
	if mm.db == nil {
		return fmt.Errorf("cannot trigger TimescaleDB maintenance: database connection is nil")
	}
	if mm.logger != nil {
		mm.logger.Info("Manually triggering TimescaleDB maintenance")
	}
	return mm.runMaintenance(ctx)
}

// TimescaleDBMaintenanceConfig represents TimescaleDB maintenance settings
type TimescaleDBMaintenanceConfig struct {
	Enabled                  bool `json:"enabled"`
	RetentionDays            int  `json:"retention_days"`
	MaintenanceIntervalHours int  `json:"maintenance_interval_hours"`
}

// DefaultTimescaleDBMaintenanceConfig returns default TimescaleDB maintenance settings
func DefaultTimescaleDBMaintenanceConfig() TimescaleDBMaintenanceConfig {
	return TimescaleDBMaintenanceConfig{
		Enabled:                  true,
		RetentionDays:            30,
		MaintenanceIntervalHours: 24,
	}
}

// ParseTimescaleDBMaintenanceFromMetadata extracts TimescaleDB maintenance configuration from metadata
func ParseTimescaleDBMaintenanceFromMetadata(metadata map[string]interface{}) TimescaleDBMaintenanceConfig {
	config := DefaultTimescaleDBMaintenanceConfig()

	if metadata == nil {
		return config
	}

	// Check if timescaledb_maintenance exists in metadata
	if tsData, exists := metadata["timescaledb_maintenance"]; exists {
		if tsMap, ok := tsData.(map[string]interface{}); ok {
			// Parse enabled flag
			if enabled, exists := tsMap["enabled"]; exists {
				if enabledBool, ok := enabled.(bool); ok {
					config.Enabled = enabledBool
				}
			}

			// Parse retention_days
			if retentionDays, exists := tsMap["retention_days"]; exists {
				switch v := retentionDays.(type) {
				case float64:
					config.RetentionDays = int(v)
				case int:
					config.RetentionDays = v
				case int64:
					config.RetentionDays = int(v)
				}
			}

			// Parse maintenance_interval_hours
			if maintenanceInterval, exists := tsMap["maintenance_interval_hours"]; exists {
				switch v := maintenanceInterval.(type) {
				case float64:
					config.MaintenanceIntervalHours = int(v)
				case int:
					config.MaintenanceIntervalHours = v
				case int64:
					config.MaintenanceIntervalHours = int(v)
				}
			}
		}
	}

	return config
}

// ValidateTimescaleDBMaintenanceConfig validates TimescaleDB maintenance configuration
func ValidateTimescaleDBMaintenanceConfig(config TimescaleDBMaintenanceConfig) error {
	if config.RetentionDays < 1 || config.RetentionDays > 365 {
		return fmt.Errorf("retention_days must be between 1 and 365, got %d", config.RetentionDays)
	}

	if config.MaintenanceIntervalHours < 1 || config.MaintenanceIntervalHours > 168 {
		return fmt.Errorf("maintenance_interval_hours must be between 1 and 168, got %d", config.MaintenanceIntervalHours)
	}

	return nil
}

// InitTimescaleDBMaintenance initializes and starts the TimescaleDB maintenance manager
func InitTimescaleDBMaintenance(ctx context.Context, db *sql.DB, logger util.Logger) (*TimescaleDBMaintenanceManager, error) {
	// Create maintenance manager
	maintenanceManager := NewTimescaleDBMaintenanceManager(db, logger)

	// Start maintenance goroutine
	maintenanceManager.Start(ctx)

	if logger != nil {
		logger.Info("TimescaleDB maintenance manager initialized and started")
	}

	return maintenanceManager, nil
}

// TimescaleDBConfigService provides centralized configuration management
type TimescaleDBConfigService struct {
	db     *sql.DB
	logger util.Logger
	cache  map[string]TimescaleDBMaintenanceConfig
	mu     sync.RWMutex
}

// NewTimescaleDBConfigService creates a new configuration service
func NewTimescaleDBConfigService(db *sql.DB, logger util.Logger) *TimescaleDBConfigService {
	return &TimescaleDBConfigService{
		db:     db,
		logger: logger,
		cache:  make(map[string]TimescaleDBMaintenanceConfig),
	}
}

// GetConfig retrieves TimescaleDB maintenance configuration with caching
func (s *TimescaleDBConfigService) GetConfig(schema string) TimescaleDBMaintenanceConfig {
	// Check cache first
	s.mu.RLock()
	if config, exists := s.cache[schema]; exists {
		s.mu.RUnlock()
		return config
	}
	s.mu.RUnlock()

	// Load from database
	config := s.loadFromDatabase(schema)

	// Cache the result
	s.mu.Lock()
	s.cache[schema] = config
	s.mu.Unlock()

	return config
}

// GetDefaultConfig returns the default configuration
func (s *TimescaleDBConfigService) GetDefaultConfig() TimescaleDBMaintenanceConfig {
	return DefaultTimescaleDBMaintenanceConfig()
}

// loadFromDatabase loads configuration from the database
func (s *TimescaleDBConfigService) loadFromDatabase(schema string) TimescaleDBMaintenanceConfig {
	ctx := context.Background()
	query := fmt.Sprintf(`
		SELECT metadata 
		FROM %s.config_otel_database 
		WHERE schema = $1 AND enabled = true 
		ORDER BY id ASC 
		LIMIT 1
	`, DefaultConfigSchema)

	var metadataJSON string
	err := s.db.QueryRowContext(ctx, query, schema).Scan(&metadataJSON)
	if err != nil {
		s.logger.Debug("No TimescaleDB maintenance configuration found, using defaults",
			"schema", schema, "error", err)
		return DefaultTimescaleDBMaintenanceConfig()
	}

	// Parse metadata JSON
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		s.logger.Debug("Failed to parse metadata JSON, using defaults",
			"schema", schema, "error", err)
		return DefaultTimescaleDBMaintenanceConfig()
	}

	// Parse TimescaleDB maintenance configuration
	config := ParseTimescaleDBMaintenanceFromMetadata(metadata)

	// Validate configuration
	if err := ValidateTimescaleDBMaintenanceConfig(config); err != nil {
		s.logger.Error("Invalid TimescaleDB maintenance configuration, using defaults",
			"schema", schema, "error", err)
		return DefaultTimescaleDBMaintenanceConfig()
	}

	s.logger.Debug("Using TimescaleDB maintenance configuration",
		"schema", schema,
		"enabled", config.Enabled,
		"retention_days", config.RetentionDays)

	return config
}

// ClearCache clears the configuration cache
func (s *TimescaleDBConfigService) ClearCache() {
	s.mu.Lock()
	s.cache = make(map[string]TimescaleDBMaintenanceConfig)
	s.mu.Unlock()
}

// executeMaintenance runs maintenance for a specific schema and configuration
func (mm *TimescaleDBMaintenanceManager) executeMaintenance(ctx context.Context, schema string, config TimescaleDBMaintenanceConfig) error {
	// 2. Update statistics
	_, err := mm.db.ExecContext(ctx, "ANALYZE")
	if err != nil {
		return fmt.Errorf("failed to update statistics: %w", err)
	}

	return nil
}
