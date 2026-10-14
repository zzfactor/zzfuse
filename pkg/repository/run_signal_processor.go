package repository

import (
	"context"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

// RunSignalProcessor handles run lifecycle management from log attributes
type RunSignalProcessor struct {
	logger     *zap.Logger
	runService *RunService
}

// Helper functions for extracting run signals from attributes

// splitStringArray splits a string into an array, handling both single values and comma-separated values
func (p *RunSignalProcessor) splitStringArray(s string) []string {
	if s == "" {
		return []string{}
	}
	// Split by comma and trim whitespace
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// contains checks if a string slice contains the given string
func (p *RunSignalProcessor) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// getTrazrRunFlags extracts the run flags from attributes
func (p *RunSignalProcessor) getTrazrRunFlags(attrs pcommon.Map) []string {
	trazrRunFlags, exists := attrs.Get("trazr.run.flags")
	if exists {
		return p.splitStringArray(trazrRunFlags.AsString())
	}
	return []string{}
}

// getTrazrRunStart extracts the run start signal from log attributes
func (p *RunSignalProcessor) getTrazrRunStart(attrs pcommon.Map) string {
	// Try trazr.run.start first (primary convention)
	if trazrRunStart, exists := attrs.Get("trazr.run.start"); exists {
		return trazrRunStart.AsString()
	}
	// Try trazr.time.bound.start (alternative convention)
	if trazrTimeBoundStart, exists := attrs.Get("trazr.time.bound.start"); exists {
		return trazrTimeBoundStart.AsString()
	}
	return ""
}

// getTrazrRunStop extracts the run stop signal from log attributes
func (p *RunSignalProcessor) getTrazrRunStop(attrs pcommon.Map) string {
	trazrRunStop, exists := attrs.Get("trazr.run.stop")
	if exists {
		return trazrRunStop.AsString()
	}
	return ""
}

// NewRunSignalProcessor creates a new run signal processor
func NewRunSignalProcessor(logger *zap.Logger, runService *RunService) *RunSignalProcessor {
	return &RunSignalProcessor{
		logger:     logger,
		runService: runService,
	}
}

// ActiveRunTracker tracks active runs and run names for overlap detection
type ActiveRunTracker struct {
	// Track active runs by service:scope
	ActiveRuns map[string][]int64 // key: "service:scope"
	// Track runs by name to handle overlaps
	RunNameToID map[string]int64 // key: "service:scope:name"
}

// NewActiveRunTracker creates a new active run tracker
func NewActiveRunTracker() *ActiveRunTracker {
	return &ActiveRunTracker{
		ActiveRuns:  make(map[string][]int64, 50),
		RunNameToID: make(map[string]int64, 50),
	}
}

// LoadActiveRunsForService loads active runs for a service and updates the tracker
func (p *RunSignalProcessor) LoadActiveRunsForService(ctx context.Context, serviceName string, tracker *ActiveRunTracker, runMetadata map[int64]*RunMetadata, metadataProcessor *RunMetadataProcessor) error {
	runs, err := p.runService.GetActiveRunsByServiceName(ctx, serviceName)
	if err != nil {
		p.logger.Warn("Failed to load active runs", zap.String("service", serviceName), zap.Error(err))
		return err
	}

	// Initialize metadata and track by scope
	for _, run := range runs {
		runMetadata[run.ID] = metadataProcessor.GetMetadata()
		scopeName := ""
		if run.ScopeName != nil {
			scopeName = *run.ScopeName
		}
		key := serviceName + ":" + scopeName
		tracker.ActiveRuns[key] = append(tracker.ActiveRuns[key], run.ID)
	}

	p.logger.Debug("Loaded active runs", zap.String("service", serviceName), zap.Int("run_count", len(runs)))
	return nil
}

// ProcessRunFromLogRecord processes a single log record for run management.
// This function handles run start/stop signals and manages run lifecycle tracking.
//
// Parameters:
// - serviceName: The service name for run tracking
// - scopeName: The scope name for run tracking (empty string for resource-level)
// - lr: The log record containing potential run signals
// - runTimeBound: Whether timebound processing is enabled (resource or scope level)
// - runTraceBound: Whether tracebound processing is enabled (resource or scope level)
// - tracker: Active run tracker for managing run state
// - runMetadata: Map tracking metadata for each run
// - metadataProcessor: Processor for managing run metadata
//
// Return Values:
// - int64: Run ID if a new run was created (>0), 0 if no run was created/ended
// - error: Any error that occurred during processing
//
// Run Signal Processing:
// 1. Start Signal (trazr.run.start):
//   - Check for existing run with same name (overlap detection)
//   - If overlap found: End previous run, remove from tracking
//   - Create new run in database
//   - Add to activeRuns and runNameToID tracking
//   - Initialize metadata for the new run
//   - Return the new run ID
//
// 2. Stop Signal (trazr.run.stop):
//   - End the run in database
//   - Remove from activeRuns and runNameToID tracking
//   - Keep metadata for final batch update
//   - Return 0 (no new run created)
//
// 3. No Signal:
//   - Return 0 (no run processing needed)
//
// Overlap Handling:
// - Detects when same run name appears again
// - Automatically ends previous run before creating new one
// - Maintains accumulated log counts across both runs
// - Ensures clean run lifecycle management
func (p *RunSignalProcessor) ProcessRunFromLogRecord(ctx context.Context, serviceName string, scopeName string, lr plog.LogRecord, runTimeBound, runTraceBound bool, tracker *ActiveRunTracker, runMetadata map[int64]*RunMetadata, metadataProcessor *RunMetadataProcessor) (int64, error) {
	runSignalFlags := p.getTrazrRunFlags(lr.Attributes())
	p.logger.Debug("LogRecord run flags",
		zap.Strings("run_signal_flags", runSignalFlags),
		zap.Bool("contains_ignore", p.contains(runSignalFlags, "ignore")))

	if p.contains(runSignalFlags, "ignore") {
		p.logger.Debug("Skipping run processing - ignore flag present")
		return 0, nil
	}

	if runTimeBound {
		runSignalStart := p.getTrazrRunStart(lr.Attributes())
		runSignalStop := p.getTrazrRunStop(lr.Attributes())

		p.logger.Debug("Timebound run signals",
			zap.String("run_signal_start", runSignalStart),
			zap.String("run_signal_stop", runSignalStop))

		if runSignalStart != "" {
			return p.processRunStart(ctx, serviceName, scopeName, runSignalStart, lr, tracker, runMetadata, metadataProcessor)
		} else if runSignalStop != "" {
			return p.processRunStop(ctx, serviceName, scopeName, runSignalStop, lr, tracker)
		} else {
			p.logger.Debug("No run start/stop signals found in timebound log")
		}
	}

	return 0, nil
}

// processRunStart handles run start signals
func (p *RunSignalProcessor) processRunStart(ctx context.Context, serviceName, scopeName, runSignalStart string, lr plog.LogRecord, tracker *ActiveRunTracker, runMetadata map[int64]*RunMetadata, metadataProcessor *RunMetadataProcessor) (int64, error) {
	// Check for same-name overlap
	runKey := serviceName + ":" + scopeName + ":" + runSignalStart
	if existingRunID, exists := tracker.RunNameToID[runKey]; exists {
		// End the previous run with the same name
		p.logger.Debug("Same-name overlap detected; ending previous run",
			zap.Int64("run_id", existingRunID),
			zap.String("name", runSignalStart))
		err := p.runService.EndRun(ctx, runSignalStart, lr.Timestamp().AsTime().UnixNano())
		if err != nil {
			p.logger.Warn("Failed to end overlapping run",
				zap.String("name", runSignalStart),
				zap.Int64("run_id", existingRunID),
				zap.Error(err))
		}

		// Remove the old run from tracking
		p.removeRunFromTracking(serviceName, scopeName, existingRunID, tracker)
		// Keep the metadata for the old run (it will be updated in batch)
	}

	// Create new run with start time using RunService
	runType := int16(1) // 1 for time-based runs
	runID, err := p.runService.CreateRun(ctx, serviceName, runSignalStart, lr.Timestamp().AsTime().UnixNano(), &runType)
	if err != nil {
		p.logger.Warn("Failed to create run",
			zap.String("service", serviceName),
			zap.String("name", runSignalStart),
			zap.Error(err))
		return 0, err
	}

	// Track the new run
	tracker.RunNameToID[runKey] = runID
	key := serviceName + ":" + scopeName
	tracker.ActiveRuns[key] = append(tracker.ActiveRuns[key], runID)
	runMetadata[runID] = metadataProcessor.GetMetadata()

	p.logger.Info("Run created successfully",
		zap.Int64("run_id", runID),
		zap.String("service", serviceName),
		zap.String("name", runSignalStart),
		zap.Int64("timestamp_ns", lr.Timestamp().AsTime().UnixNano()))
	return runID, nil
}

// GetUniqueRunIDs combines run IDs from multiple sources, avoiding duplicates
func (p *RunSignalProcessor) GetUniqueRunIDs(resourceRunIDs, scopeRunIDs []int64) []int64 {
	runIDs := make(map[int64]bool)
	for _, runID := range resourceRunIDs {
		runIDs[runID] = true
	}
	for _, runID := range scopeRunIDs {
		runIDs[runID] = true
	}

	var uniqueRunIDs []int64
	for runID := range runIDs {
		uniqueRunIDs = append(uniqueRunIDs, runID)
	}
	return uniqueRunIDs
}

// processRunStop handles run stop signals
func (p *RunSignalProcessor) processRunStop(ctx context.Context, serviceName, scopeName, runSignalStop string, lr plog.LogRecord, tracker *ActiveRunTracker) (int64, error) {
	// End run using RunService
	err := p.runService.EndRun(ctx, runSignalStop, lr.Timestamp().AsTime().UnixNano())
	if err != nil {
		p.logger.Warn("Failed to end run",
			zap.String("name", runSignalStop),
			zap.Error(err))
		return 0, err
	}

	// Remove the run from tracking
	runKey := serviceName + ":" + scopeName + ":" + runSignalStop
	if runID, exists := tracker.RunNameToID[runKey]; exists {
		p.removeRunFromTracking(serviceName, scopeName, runID, tracker)
		// Remove from name tracking
		delete(tracker.RunNameToID, runKey)
		p.logger.Debug("Run ended and removed from tracking",
			zap.Int64("run_id", runID),
			zap.String("name", runSignalStop))
	}

	p.logger.Info("Run ended successfully",
		zap.String("service", serviceName),
		zap.String("name", runSignalStop),
		zap.Int64("timestamp_ns", lr.Timestamp().AsTime().UnixNano()))
	return 0, nil
}

// removeRunFromTracking removes a run from active tracking
func (p *RunSignalProcessor) removeRunFromTracking(serviceName, scopeName string, runID int64, tracker *ActiveRunTracker) {
	key := serviceName + ":" + scopeName
	if runs, ok := tracker.ActiveRuns[key]; ok {
		for i, rid := range runs {
			if rid == runID {
				// Remove from activeRuns
				tracker.ActiveRuns[key] = append(runs[:i], runs[i+1:]...)
				break
			}
		}
	}
}

// CheckRunFlags checks if run processing is enabled at resource or scope level
// This is a helper function that can be used when you have pcommon.Map attributes
func (p *RunSignalProcessor) CheckRunFlags(resourceAttrs, scopeAttrs interface{}) (bool, bool) {
	var resourceTimeBound, resourceTraceBound bool
	var scopeTimeBound, scopeTraceBound bool

	// Handle resource attributes
	if resourceMap, ok := resourceAttrs.(interface {
		Get(string) (interface{}, bool)
	}); ok {
		if flagsVal, exists := resourceMap.Get("trazr.run.flags"); exists {
			if flagsStr, ok := flagsVal.(string); ok {
				resourceFlags := p.splitStringArray(flagsStr)
				resourceTimeBound = p.contains(resourceFlags, "timebound")
				resourceTraceBound = p.contains(resourceFlags, "tracebound")
			}
		}
	}

	// Handle scope attributes
	if scopeMap, ok := scopeAttrs.(interface {
		Get(string) (interface{}, bool)
	}); ok {
		if flagsVal, exists := scopeMap.Get("trazr.run.flags"); exists {
			if flagsStr, ok := flagsVal.(string); ok {
				scopeFlags := p.splitStringArray(flagsStr)
				scopeTimeBound = p.contains(scopeFlags, "timebound")
				scopeTraceBound = p.contains(scopeFlags, "tracebound")
			}
		}
	}

	// Combine resource and scope flags
	runTimeBound := resourceTimeBound || scopeTimeBound
	runTraceBound := resourceTraceBound || scopeTraceBound

	return runTimeBound, runTraceBound
}

// ProcessRunSignalsFromLogEntries processes run signals from log entries
// This method was moved from otel_logs_service.go to centralize run processing logic
func (p *RunSignalProcessor) ProcessRunSignalsFromLogEntries(ctx context.Context, entries []LogEntry, schema string, runMetadataProcessor *RunMetadataProcessor, runService *RunService, logger *zap.Logger) error {
	// Group entries by service for efficient processing
	serviceGroups := make(map[string][]LogEntry)
	for _, entry := range entries {
		serviceName := entry.ServiceName
		serviceGroups[serviceName] = append(serviceGroups[serviceName], entry)
	}

	// Process each service group
	for serviceName, serviceEntries := range serviceGroups {
		// Initialize tracking structures for this service
		tracker := NewActiveRunTracker()
		runMetadata := make(map[int64]*RunMetadata)
		processedServices := make(map[string]bool)

		// Load active runs for this service if not done yet
		if !processedServices[serviceName] {
			processedServices[serviceName] = true
			err := p.LoadActiveRunsForService(ctx, serviceName, tracker, runMetadata, runMetadataProcessor)
			if err != nil {
				logger.Warn("Failed to load active runs", zap.String("service", serviceName), zap.Error(err))
				continue
			}
		}

		// Process run signals and metadata for each entry
		for _, entry := range serviceEntries {
			// Convert LogEntry back to the format needed for run processing
			// This is a simplified approach - in practice you might want to store the original plog.LogRecord

			// Process run metadata (log counting)
			key := serviceName + ":" // service:scope format
			if activeRuns, exists := tracker.ActiveRuns[key]; exists && len(activeRuns) > 0 {
				// Convert severity number to plog.SeverityNumber for processing
				var severityNumber plog.SeverityNumber
				if entry.SeverityNumber != nil {
					severityNumber = plog.SeverityNumber(*entry.SeverityNumber)
				}

				// Process log counts for all active runs
				runMetadataProcessor.ProcessLogCounts(runMetadata, activeRuns, severityNumber)
			}
		}

		// Create and execute batch updates for this service
		if len(runMetadata) > 0 {
			batchUpdates := runMetadataProcessor.CreateBatchUpdates(runMetadata)
			if len(batchUpdates) > 0 {
				err := runMetadataProcessor.BatchUpdateRunMetadata(ctx, schema, batchUpdates, runService)
				if err != nil {
					logger.Warn("Failed to batch update run metadata",
						zap.String("service", serviceName),
						zap.Error(err))
				}
			}
		}
	}

	return nil
}
