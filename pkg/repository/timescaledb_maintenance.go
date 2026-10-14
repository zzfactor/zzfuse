package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
)

// TimescaleDBMaintenanceManager handles automated maintenance for TimescaleDB hypertables
type TimescaleDBMaintenanceManager struct {
	DB     *SQLDatabaseClient
	Logger util.Logger

	// Configuration
	Schema           string
	RetentionDays    int
	MaintenanceDelay time.Duration

	// Runtime
	stopped chan struct{}
}

// TimescaleDBMaintenanceOption is a functional option for configuring the maintenance manager
type TimescaleDBMaintenanceOption func(*TimescaleDBMaintenanceManager)

// WithTimescaleDBSchema sets the schema for maintenance
func WithTimescaleDBSchema(schema string) TimescaleDBMaintenanceOption {
	return func(tm *TimescaleDBMaintenanceManager) { tm.Schema = schema }
}

// WithRetentionDays sets how many days to retain data
func WithRetentionDays(days int) TimescaleDBMaintenanceOption {
	return func(tm *TimescaleDBMaintenanceManager) { tm.RetentionDays = days }
}

// NewTimescaleDBMaintenanceManager creates a new TimescaleDB maintenance manager
func NewTimescaleDBMaintenanceManager(
	db OtelDatabaseClient,
	logger util.Logger,
	opts ...TimescaleDBMaintenanceOption,
) *TimescaleDBMaintenanceManager {
	// Type assert to get the underlying SQLDatabaseClient
	typedDB, ok := db.(*OtelSQLDatabaseClient)
	var sqlDB *SQLDatabaseClient
	if ok {
		sqlDB = typedDB.SQLDatabaseClient
	} else {
		// Fallback: create a new SQLDatabaseClient if needed
		sqlDB = &SQLDatabaseClient{}
	}

	tm := &TimescaleDBMaintenanceManager{
		DB:               sqlDB,
		Logger:           logger,
		Schema:           DefaultSchema,  // Default schema
		RetentionDays:    30,             // Keep 30 days by default
		MaintenanceDelay: 24 * time.Hour, // Daily maintenance
		stopped:          make(chan struct{}),
	}

	for _, opt := range opts {
		opt(tm)
	}

	if tm.Logger == nil {
		tm.Logger = util.NoopLogger{}
	}

	return tm
}

// NewTimescaleDBMaintenanceManagerFromConfig creates a new maintenance manager with configuration from metadata
func NewTimescaleDBMaintenanceManagerFromConfig(
	db OtelDatabaseClient,
	logger util.Logger,
	schema string,
) *TimescaleDBMaintenanceManager {
	// Type assert to get the underlying SQLDatabaseClient
	typedDB, ok := db.(*OtelSQLDatabaseClient)
	var sqlDB *SQLDatabaseClient
	if ok {
		sqlDB = typedDB.SQLDatabaseClient
	} else {
		// Fallback: create a new SQLDatabaseClient if needed
		sqlDB = &SQLDatabaseClient{}
	}

	// Get maintenance configuration from database
	config := getTimescaleDBMaintenanceConfigFromDB(sqlDB, schema, logger)

	tm := &TimescaleDBMaintenanceManager{
		DB:               sqlDB,
		Logger:           logger,
		Schema:           schema,
		RetentionDays:    config.RetentionDays,
		MaintenanceDelay: time.Duration(config.MaintenanceIntervalHours) * time.Hour,
		stopped:          make(chan struct{}),
	}

	if tm.Logger == nil {
		tm.Logger = util.NoopLogger{}
	}

	return tm
}

// TimescaleDBMaintenanceConfig represents configuration for TimescaleDB maintenance
type TimescaleDBMaintenanceConfig struct {
	Enabled                  bool `json:"enabled"`
	RetentionDays            int  `json:"retention_days"`
	MaintenanceIntervalHours int  `json:"maintenance_interval_hours"`
}

// DefaultTimescaleDBMaintenanceConfig returns default configuration
func DefaultTimescaleDBMaintenanceConfig() TimescaleDBMaintenanceConfig {
	return TimescaleDBMaintenanceConfig{
		Enabled:                  true,
		RetentionDays:            30,
		MaintenanceIntervalHours: 24,
	}
}

// getTimescaleDBMaintenanceConfigFromDB retrieves maintenance configuration from database
func getTimescaleDBMaintenanceConfigFromDB(db *SQLDatabaseClient, schema string, logger util.Logger) TimescaleDBMaintenanceConfig {
	ctx := context.Background()

	// Try to get the schema configuration from config_otel_database table
	query := fmt.Sprintf(`
		SELECT metadata 
		FROM %s.config_otel_database 
		WHERE schema = $1 AND enabled = true 
		ORDER BY id ASC 
		LIMIT 1
	`, database.DefaultConfigSchema)

	var metadataJSON string
	err := db.DB.QueryRowContext(ctx, query, schema).Scan(&metadataJSON)
	if err != nil {
		// If no configuration found, return defaults
		logger.Debug("No TimescaleDB maintenance configuration found, using defaults",
			"schema", schema, "error", err)
		return DefaultTimescaleDBMaintenanceConfig()
	}

	// Parse metadata JSON
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		logger.Debug("Failed to parse metadata JSON, using defaults",
			"schema", schema, "error", err)
		return DefaultTimescaleDBMaintenanceConfig()
	}

	// Parse TimescaleDB maintenance configuration
	config := parseTimescaleDBMaintenanceFromMetadata(metadata)

	// Validate configuration
	if err := validateTimescaleDBMaintenanceConfig(config); err != nil {
		logger.Error("Invalid TimescaleDB maintenance configuration, using defaults",
			"schema", schema, "error", err)
		return DefaultTimescaleDBMaintenanceConfig()
	}

	logger.Debug("Using TimescaleDB maintenance configuration from database",
		"schema", schema,
		"enabled", config.Enabled,
		"retention_days", config.RetentionDays)

	return config
}

// parseTimescaleDBMaintenanceFromMetadata parses maintenance configuration from metadata
func parseTimescaleDBMaintenanceFromMetadata(metadata map[string]interface{}) TimescaleDBMaintenanceConfig {
	config := DefaultTimescaleDBMaintenanceConfig()

	if timescaledbConfig, ok := metadata["timescaledb_maintenance"].(map[string]interface{}); ok {
		if enabled, ok := timescaledbConfig["enabled"].(bool); ok {
			config.Enabled = enabled
		}
		if retentionDays, ok := timescaledbConfig["retention_days"].(float64); ok {
			config.RetentionDays = int(retentionDays)
		}
		if intervalHours, ok := timescaledbConfig["maintenance_interval_hours"].(float64); ok {
			config.MaintenanceIntervalHours = int(intervalHours)
		}
	}

	return config
}

// validateTimescaleDBMaintenanceConfig validates the maintenance configuration
func validateTimescaleDBMaintenanceConfig(config TimescaleDBMaintenanceConfig) error {
	if config.RetentionDays < 1 || config.RetentionDays > 365 {
		return fmt.Errorf("retention_days must be between 1 and 365, got %d", config.RetentionDays)
	}
	if config.MaintenanceIntervalHours < 1 || config.MaintenanceIntervalHours > 168 {
		return fmt.Errorf("maintenance_interval_hours must be between 1 and 168, got %d", config.MaintenanceIntervalHours)
	}
	return nil
}

// Start begins automated TimescaleDB maintenance
func (tm *TimescaleDBMaintenanceManager) Start() {
	tm.Logger.Info("Starting TimescaleDB maintenance manager",
		"schema", tm.Schema,
		"retention_days", tm.RetentionDays,
		"maintenance_delay", tm.MaintenanceDelay)

	// Run initial maintenance
	tm.runMaintenance()

	// Schedule periodic maintenance
	ticker := time.NewTicker(tm.MaintenanceDelay)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopped:
			tm.Logger.Info("TimescaleDB maintenance manager stopped")
			return
		case <-ticker.C:
			tm.runMaintenance()
		}
	}
}

// Stop signals the maintenance manager to stop gracefully
func (tm *TimescaleDBMaintenanceManager) Stop() {
	close(tm.stopped)
	tm.Logger.Debug("TimescaleDB maintenance manager stop signal sent")
}

// runMaintenance executes the complete TimescaleDB maintenance routine
func (tm *TimescaleDBMaintenanceManager) runMaintenance() {
	tm.Logger.Info("Starting TimescaleDB maintenance")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// 1. Update statistics
	if err := tm.updateStatistics(ctx); err != nil {
		tm.Logger.Error("Failed to update statistics", "error", err)
	}

	// 2. Log current status
	tm.logHypertableStatus(ctx)

	tm.Logger.Info("TimescaleDB maintenance completed")
}

// updateStatistics updates table statistics for better query planning
func (tm *TimescaleDBMaintenanceManager) updateStatistics(ctx context.Context) error {
	tm.Logger.Debug("Updating table statistics")

	query := "ANALYZE"
	_, err := tm.DB.DB.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update statistics: %w", err)
	}

	tm.Logger.Debug("Table statistics updated successfully")
	return nil
}

// logHypertableStatus logs current hypertable statistics
func (tm *TimescaleDBMaintenanceManager) logHypertableStatus(ctx context.Context) {
	query := `
		SELECT 
			h.hypertable_name,
			COUNT(c.chunk_name) as chunk_count,
			pg_size_pretty(SUM(pg_total_relation_size(c.chunk_name::regclass))) as total_size,
			SUM(pg_total_relation_size(c.chunk_name::regclass)) as total_size_bytes
		FROM timescaledb_information.hypertables h
		LEFT JOIN timescaledb_information.chunks c ON h.hypertable_name = c.hypertable_name
		WHERE h.hypertable_schema = $1
		GROUP BY h.hypertable_name
		ORDER BY h.hypertable_name
	`

	rows, err := tm.DB.DB.QueryContext(ctx, query, tm.Schema)
	if err != nil {
		tm.Logger.Error("Failed to get hypertable statistics", "error", err)
		return
	}
	defer rows.Close()

	var totalChunks int
	var totalSizeBytes int64

	for rows.Next() {
		var tableName, totalSize string
		var chunkCount int
		var sizeBytes int64

		if err := rows.Scan(&tableName, &chunkCount, &totalSize, &sizeBytes); err != nil {
			tm.Logger.Error("Failed to scan hypertable statistics", "error", err)
			continue
		}

		totalChunks += chunkCount
		totalSizeBytes += sizeBytes

		tm.Logger.Info("Hypertable statistics",
			"table_name", tableName,
			"chunk_count", chunkCount,
			"total_size", totalSize)
	}

	tm.Logger.Info("Overall hypertable statistics",
		"schema", tm.Schema,
		"total_chunks", totalChunks,
		"total_size", util.FormatBytes(totalSizeBytes))
}

// GetHypertableStatistics returns current hypertable statistics
func (tm *TimescaleDBMaintenanceManager) GetHypertableStatistics(ctx context.Context) ([]map[string]interface{}, error) {
	query := `
		SELECT 
			h.hypertable_name,
			COUNT(c.chunk_name) as chunk_count,
			pg_size_pretty(SUM(pg_total_relation_size(c.chunk_name::regclass))) as total_size,
			SUM(pg_total_relation_size(c.chunk_name::regclass)) as total_size_bytes,
			MIN(c.range_start) as oldest_chunk_start,
			MAX(c.range_end) as newest_chunk_end
		FROM timescaledb_information.hypertables h
		LEFT JOIN timescaledb_information.chunks c ON h.hypertable_name = c.hypertable_name
		WHERE h.hypertable_schema = $1
		GROUP BY h.hypertable_name
		ORDER BY h.hypertable_name
	`

	rows, err := tm.DB.DB.QueryContext(ctx, query, tm.Schema)
	if err != nil {
		return nil, fmt.Errorf("failed to get hypertable statistics: %w", err)
	}
	defer rows.Close()

	var stats []map[string]interface{}
	for rows.Next() {
		var tableName, totalSize string
		var chunkCount int
		var totalSizeBytes int64
		var oldestChunkStart, newestChunkEnd interface{}

		if err := rows.Scan(&tableName, &chunkCount, &totalSize, &totalSizeBytes, &oldestChunkStart, &newestChunkEnd); err != nil {
			return nil, fmt.Errorf("failed to scan hypertable statistics: %w", err)
		}

		stats = append(stats, map[string]interface{}{
			"table_name":         tableName,
			"chunk_count":        chunkCount,
			"total_size":         totalSize,
			"total_size_bytes":   totalSizeBytes,
			"oldest_chunk_start": oldestChunkStart,
			"newest_chunk_end":   newestChunkEnd,
		})
	}

	return stats, nil
}
