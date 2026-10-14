package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// RunMetadata represents the metadata for a run with optimized performance.
// This struct accumulates log counts by severity level for a single run.
//
// Severity Mapping (based on plog.SeverityNumber):
// - ERROR and FATAL (17+): LogErrorCount
// - WARN (13-16): LogWarnCount
// - INFO (9-12): LogInfoCount
// - DEBUG and TRACE (1-8): LogDebugCount
//
// Memory Management:
// - Uses sync.Pool for efficient allocation/deallocation
// - Reset on return to pool to avoid data leakage
// - Prevents excessive memory allocation during high-volume processing
type RunMetadata struct {
	LogCount      int64 // Total number of logs
	LogInfoCount  int64 // Number of INFO level logs
	LogWarnCount  int64 // Number of WARN level logs
	LogDebugCount int64 // Number of DEBUG/TRACE level logs
	LogErrorCount int64 // Number of ERROR/FATAL level logs
}

// Increment updates the metadata based on log severity.
// This method increments the appropriate counter based on the severity level.
//
// Severity Classification:
// - 17+: ERROR and FATAL levels -> LogErrorCount
// - 13-16: WARN level -> LogWarnCount
// - 9-12: INFO level -> LogInfoCount
// - 1-8: DEBUG and TRACE levels -> LogDebugCount
//
// Always increments LogCount regardless of severity.
func (m *RunMetadata) Increment(severity plog.SeverityNumber) {
	m.LogCount++
	switch {
	case severity >= 17: // ERROR and FATAL (17+)
		m.LogErrorCount++
	case severity >= 13: // WARN (13-16)
		m.LogWarnCount++
	case severity >= 9: // INFO (9-12)
		m.LogInfoCount++
	default: // DEBUG and TRACE (1-8)
		m.LogDebugCount++
	}
}

// ToMap converts the struct to a map for database storage.
// This method creates a map representation suitable for JSON serialization
// and database storage in the runs.metadata JSONB column.
//
// Database Schema Mapping:
// - LOG_COUNT: Total number of logs
// - LOG_INFO_COUNT: Number of INFO level logs
// - LOG_WARNING_COUNT: Number of WARN level logs
// - LOG_DEBUG_COUNT: Number of DEBUG/TRACE level logs
// - LOG_ERROR_COUNT: Number of ERROR/FATAL level logs
func (m *RunMetadata) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"LOG_COUNT":         m.LogCount,
		"LOG_INFO_COUNT":    m.LogInfoCount,
		"LOG_WARNING_COUNT": m.LogWarnCount,
		"LOG_DEBUG_COUNT":   m.LogDebugCount,
		"LOG_ERROR_COUNT":   m.LogErrorCount,
	}
}

// Reset clears all values for pool reuse
func (m *RunMetadata) Reset() {
	m.LogCount = 0
	m.LogInfoCount = 0
	m.LogWarnCount = 0
	m.LogDebugCount = 0
	m.LogErrorCount = 0
}

// RunMetadataBatchUpdate represents a batch update for run metadata
type RunMetadataBatchUpdate struct {
	RunID    int64
	Metadata map[string]interface{}
}

// RunMetadataProcessor handles run metadata operations with memory pooling and batch updates
type RunMetadataProcessor struct {
	client   *sql.DB
	logger   *zap.Logger
	eventBus event.EventBusInterface
	pool     sync.Pool
}

// NewRunMetadataProcessor creates a new run metadata processor
func NewRunMetadataProcessor(client *sql.DB, logger *zap.Logger, eventBus event.EventBusInterface) *RunMetadataProcessor {
	processor := &RunMetadataProcessor{
		client:   client,
		logger:   logger,
		eventBus: eventBus,
	}

	// Initialize memory pool for RunMetadata structs
	processor.pool = sync.Pool{
		New: func() interface{} {
			return &RunMetadata{}
		},
	}

	return processor
}

// GetMetadata gets a RunMetadata from the pool
func (p *RunMetadataProcessor) GetMetadata() *RunMetadata {
	return p.pool.Get().(*RunMetadata)
}

// PutMetadata returns a RunMetadata to the pool after resetting
func (p *RunMetadataProcessor) PutMetadata(m *RunMetadata) {
	m.Reset()
	p.pool.Put(m)
}

// BatchUpdateRunMetadata performs batch updates to reduce database round trips.
// This function accumulates log counts incrementally in the database.
//
// Update Strategy:
// - Uses PostgreSQL's COALESCE to handle NULL values (first update)
// - Adds incremental counts to existing metadata values
// - Updates all severity-based counts (LOG_COUNT, LOG_INFO_COUNT, etc.)
// - Uses a single transaction for all updates
//
// SQL Logic:
// - metadata = jsonb_build_object(...) creates new metadata object
// - COALESCE((metadata->>'LOG_COUNT')::int, 0) gets existing count or 0
// - + $2 adds the incremental count from this batch
// - This ensures counts accumulate across multiple batches
//
// Performance Benefits:
// - Single transaction for all updates
// - Prepared statement reuse
// - Incremental updates avoid overwriting previous counts
// - Batch processing reduces database round trips
func (p *RunMetadataProcessor) BatchUpdateRunMetadata(ctx context.Context, schemaName string, updates []RunMetadataBatchUpdate, runService *RunService) error {
	if len(updates) == 0 {
		return nil
	}

	// Use a transaction for batch updates
	tx, err := p.client.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Will be ignored if tx.Commit() succeeds

	// Prepare the update statement - use incremental updates to accumulate counts
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		UPDATE %s.runs 
		SET metadata = jsonb_build_object(
			'LOG_COUNT', COALESCE((metadata->>'LOG_COUNT')::int, 0) + $2,
			'LOG_INFO_COUNT', COALESCE((metadata->>'LOG_INFO_COUNT')::int, 0) + $3,
			'LOG_WARNING_COUNT', COALESCE((metadata->>'LOG_WARNING_COUNT')::int, 0) + $4,
			'LOG_DEBUG_COUNT', COALESCE((metadata->>'LOG_DEBUG_COUNT')::int, 0) + $5,
			'LOG_ERROR_COUNT', COALESCE((metadata->>'LOG_ERROR_COUNT')::int, 0) + $6
		), updated_at = NOW() 
		WHERE id = $1
	`, schemaName))
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Execute batch updates
	for _, update := range updates {
		// Extract individual count values for incremental update
		logCount := int64(0)
		logInfoCount := int64(0)
		logWarningCount := int64(0)
		logDebugCount := int64(0)
		logErrorCount := int64(0)

		if count, ok := update.Metadata["LOG_COUNT"].(int64); ok {
			logCount = count
		}
		if count, ok := update.Metadata["LOG_INFO_COUNT"].(int64); ok {
			logInfoCount = count
		}
		if count, ok := update.Metadata["LOG_WARNING_COUNT"].(int64); ok {
			logWarningCount = count
		}
		if count, ok := update.Metadata["LOG_DEBUG_COUNT"].(int64); ok {
			logDebugCount = count
		}
		if count, ok := update.Metadata["LOG_ERROR_COUNT"].(int64); ok {
			logErrorCount = count
		}

		_, err = stmt.ExecContext(ctx, update.RunID, logCount, logInfoCount, logWarningCount, logDebugCount, logErrorCount)
		if err != nil {
			return fmt.Errorf("failed to update run %d: %w", update.RunID, err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// After successful commit, publish run.updated events for each updated run, if an event bus is available
	if p.eventBus != nil && runService != nil {
		for _, update := range updates {
			// Fetch the latest run state and publish
			if run, err := runService.GetRunByID(ctx, update.RunID); err == nil && run != nil {
				runResponse := ToRunEntryResponse(run)
				_ = p.eventBus.Publish(ctx, event.Event{
					Type:      EventTypeRunUpdated.ToEventType(),
					Timestamp: time.Now(),
					Schema:    "", // runs are schema-agnostic in bus routing
					Data:      runResponse,
					IDs:       []string{fmt.Sprintf("%d", update.RunID)},
				})
			}
		}
	}

	p.logger.Debug("Batch updated run metadata", zap.Int("update_count", len(updates)))
	return nil
}

// ProcessLogCounts updates metadata for all unique runs with the given severity
func (p *RunMetadataProcessor) ProcessLogCounts(runMetadata map[int64]*RunMetadata, runIDs []int64, severity plog.SeverityNumber) {
	for _, runID := range runIDs {
		if metadata := runMetadata[runID]; metadata != nil {
			metadata.Increment(severity)
		}
	}
}

// CreateBatchUpdates creates batch update entries from run metadata map
func (p *RunMetadataProcessor) CreateBatchUpdates(runMetadata map[int64]*RunMetadata) []RunMetadataBatchUpdate {
	updates := make([]RunMetadataBatchUpdate, 0, len(runMetadata))

	for runID, metadata := range runMetadata {
		if metadata != nil { // Safety check
			updates = append(updates, RunMetadataBatchUpdate{
				RunID:    runID,
				Metadata: metadata.ToMap(),
			})
		}
	}

	return updates
}
