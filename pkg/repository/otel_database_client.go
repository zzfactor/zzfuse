package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// =========================
// Otel Database Client (DRY Implementation)
// =========================

// OnDemandParams represents parameters for on-demand worker execution
type OnDemandParams struct {
	Schema      string     `json:"schema"`
	StartTime   *time.Time `json:"start_time,omitempty"`
	StopTime    *time.Time `json:"stop_time,omitempty"`
	SignalTypes []string   `json:"signal_types,omitempty"`
	BatchSize   int        `json:"batch_size,omitempty"`
}

// ApplyDefaults sets default values for time parameters if not provided
func (p *OnDemandParams) ApplyDefaults() {
	now := time.Now()
	if p.StartTime == nil {
		p.StartTime = &now
	}
	if p.StopTime == nil {
		stopTime := now.Add(5 * time.Minute)
		p.StopTime = &stopTime
	}
}

// OtelDatabaseClient provides a single interface for all database operations
type OtelDatabaseClient interface {
	// Signal fetching methods - unified to handle both simple and complex cases
	FetchUnembeddedSignals(ctx context.Context, configID int64, signalType string, limit int, schema string) ([]map[string]interface{}, error)
	MarkSignalsEmbedded(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error
	MarkSignalsEmbeddedBulk(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error

	// Advanced embedding methods (optional - for embedding workers)
	FetchUnembeddedSignalsAdvanced(ctx context.Context, tableName string, batchSize int, configID int64, schema string, options *FetchOptions, requiredFields []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error)
	MarkSignalsEmbeddedAdvanced(ctx context.Context, tableName string, ids []interface{}, configID int64, embeddedIDs []string, schema string) error

	// Trace data fetching methods (merged from TraceDatabaseClient)
	FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error)
	FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error)
	FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error)
	FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error)

	// Optimized span-specific fetching methods (TimescaleDB optimized)
	FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error)
	FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error)
	FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error)

	// General OTEL data operations (unified from repositories)
	QuerySpans(ctx context.Context, q TraceQuery) ([]TraceEntry, string, bool, string, bool, error)
	CountSpans(ctx context.Context, q TraceQuery) (int64, error)
	InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error)
	QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, string, bool, error)
	CountLogs(ctx context.Context, q LogQuery) (int64, error)
	InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error)
	QueryProfiles(ctx context.Context, q ProfileQuery) ([]ProfileEntry, string, bool, error)
	InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error)
	QueryRuns(ctx context.Context, q RunQuery) ([]RunEntry, string, bool, error)
	InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error)
	UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error
	EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error
	GetRunByID(ctx context.Context, id int64, schema string) (RunEntry, error)

	// MetricRepository interface methods (aliases for compatibility)
	// Direct by ID fetch (type-agnostic helper)
	GetMetricByID(ctx context.Context, id int64, schema string) (interface{}, string, error)
	// Logs and Traces direct fetches
	GetLogByID(ctx context.Context, id int64, schema string) (LogEntry, error)
	GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntry, error)
	// Gauge metrics
	QueryMetricsGauge(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error)
	CountMetricsGauge(ctx context.Context, q MetricQuery) (int64, error)
	InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error)

	// Sum metrics
	QueryMetricsSum(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error)
	CountMetricsSum(ctx context.Context, q MetricQuery) (int64, error)
	InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error)

	// Histogram metrics
	QueryMetricsHistogram(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error)
	CountMetricsHistogram(ctx context.Context, q MetricQuery) (int64, error)
	InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error)

	// Summary metrics
	QueryMetricsSummary(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error)
	CountMetricsSummary(ctx context.Context, q MetricQuery) (int64, error)
	InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error)

	// Exponential histogram metrics
	QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error)
	CountMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (int64, error)
	InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error)

	// On-demand worker execution methods
	StartOnDemandWorkers(ctx context.Context, params OnDemandParams) error
	StopOnDemandWorkers(ctx context.Context, schema string) error

	// Statistics and monitoring
	GetStatistics(ctx context.Context) (*WorkerMetrics, error)
	GetJobStatus(ctx context.Context, jobID string) (*ProcessingJob, error)
	GetActiveJobs(ctx context.Context) ([]*ProcessingJob, error)
}

// OtelSQLDatabaseClient implements OtelDatabaseClient using SQL
type OtelSQLDatabaseClient struct {
	*SQLDatabaseClient
	metricRepo    MetricRepository
	traceRepo     TraceRepository
	logRepo       LogRepository
	profileRepo   ProfileRepository
	runRepo       RunRepository
	schemaManager *SchemaManager // Add schema manager for on-demand worker control
}

// NewUnifiedSQLDatabaseClient creates a new unified database client
func NewUnifiedSQLDatabaseClient(db *sql.DB) *OtelSQLDatabaseClient {
	return &OtelSQLDatabaseClient{
		SQLDatabaseClient: &SQLDatabaseClient{DB: db},
		metricRepo:        NewMetricRepository(db),
		traceRepo:         NewTraceRepository(db),
		logRepo:           NewLogRepository(db),
		profileRepo:       NewProfileRepository(db),
		runRepo:           NewRunRepository(db),
		schemaManager:     nil, // Will be set by the service layer
	}
}

// SetSchemaManager sets the schema manager for on-demand worker control
func (c *OtelSQLDatabaseClient) SetSchemaManager(sm *SchemaManager) {
	c.schemaManager = sm
}

// GetStatistics returns worker statistics
func (c *OtelSQLDatabaseClient) GetStatistics(ctx context.Context) (*WorkerMetrics, error) {
	// Implementation would aggregate statistics from both clients
	return &WorkerMetrics{}, nil
}

// GetJobStatus returns job status
func (c *OtelSQLDatabaseClient) GetJobStatus(ctx context.Context, jobID string) (*ProcessingJob, error) {
	// Implementation would query job status
	return &ProcessingJob{}, nil
}

// GetActiveJobs returns active jobs
func (c *OtelSQLDatabaseClient) GetActiveJobs(ctx context.Context) ([]*ProcessingJob, error) {
	// Implementation would query active jobs
	return []*ProcessingJob{}, nil
}

// StartOnDemandWorkers starts workers for specific signal types in a schema
func (c *OtelSQLDatabaseClient) StartOnDemandWorkers(ctx context.Context, params OnDemandParams) error {
	// Apply defaults to parameters
	params.ApplyDefaults()

	// Validate and filter signal types
	validatedTypes := validateSignalTypes(params.SignalTypes)
	if len(validatedTypes) == 0 && len(params.SignalTypes) > 0 {
		return fmt.Errorf("invalid signal types: none of %v are valid", params.SignalTypes)
	}
	params.SignalTypes = validatedTypes

	// Check if schema manager is available
	if c.schemaManager == nil {
		return fmt.Errorf("schema manager not available - cannot start on-demand workers")
	}

	// Delegate to schema manager to start workers with time bounds
	return c.schemaManager.StartOnDemandWorkers(ctx, params)
}

// StopOnDemandWorkers stops workers for a specific schema
func (c *OtelSQLDatabaseClient) StopOnDemandWorkers(ctx context.Context, schema string) error {
	if schema == "" {
		return fmt.Errorf("schema cannot be empty")
	}

	// Check if schema manager is available
	if c.schemaManager == nil {
		return fmt.Errorf("schema manager not available - cannot stop on-demand workers")
	}

	// Delegate to schema manager to stop workers
	return c.schemaManager.StopOnDemandWorkers(ctx, schema)
}

// =========================
// Basic Interface Implementation
// =========================

// FetchUnembeddedSignals implements the basic OtelDatabaseClient interface
func (c *OtelSQLDatabaseClient) FetchUnembeddedSignals(ctx context.Context, configID int64, signalType string, limit int, schema string) ([]map[string]interface{}, error) {
	return c.SQLDatabaseClient.FetchUnembeddedSignalsSimple(ctx, signalType, limit, nil, configID, schema)
}

// MarkSignalsEmbedded implements the basic OtelDatabaseClient interface
func (c *OtelSQLDatabaseClient) MarkSignalsEmbedded(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	// Create mock embedded IDs for compatibility
	embeddedIDs := make([]string, len(signalIDs))
	for i := range signalIDs {
		embeddedIDs[i] = "mock-embedded-id"
	}
	// Use a default signal type - this might need to be parameterized
	return c.SQLDatabaseClient.MarkSignalsEmbeddedAdvanced(ctx, "SignalTypeSpans", signalIDs, configID, embeddedIDs, schema)
}

// MarkSignalsEmbeddedBulk implements the basic OtelDatabaseClient interface
func (c *OtelSQLDatabaseClient) MarkSignalsEmbeddedBulk(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	// For bulk operations, we can use the same implementation as MarkSignalsEmbedded
	return c.MarkSignalsEmbedded(ctx, configID, signalIDs, schema)
}

// =========================
// Advanced Interface Implementation
// =========================

// FetchUnembeddedSignalsAdvanced implements the advanced OtelDatabaseClient interface
func (c *OtelSQLDatabaseClient) FetchUnembeddedSignalsAdvanced(ctx context.Context, tableName string, batchSize int, configID int64, schema string, options *FetchOptions, requiredFields []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error) {
	// Delegate to the embedded SQLDatabaseClient
	return c.SQLDatabaseClient.FetchUnembeddedSignalsAdvanced(ctx, tableName, batchSize, configID, schema, options, requiredFields)
}

// MarkSignalsEmbeddedAdvanced implements the advanced OtelDatabaseClient interface
func (c *OtelSQLDatabaseClient) MarkSignalsEmbeddedAdvanced(ctx context.Context, tableName string, ids []interface{}, configID int64, embeddedIDs []string, schema string) error {
	// Delegate to the embedded SQLDatabaseClient
	return c.SQLDatabaseClient.MarkSignalsEmbeddedAdvanced(ctx, tableName, ids, configID, embeddedIDs, schema)
}

// TraceDatabaseClient method implementations - delegate to traceRepo

// FetchLogsByTraceID delegates to the trace repository
func (c *OtelSQLDatabaseClient) FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error) {
	return c.traceRepo.FetchLogsByTraceID(ctx, traceID)
}

// FetchLogsBySpanID delegates to the trace repository
func (c *OtelSQLDatabaseClient) FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error) {
	return c.traceRepo.FetchLogsBySpanID(ctx, spanID)
}

// FetchSpansByTraceID delegates to the trace repository
func (c *OtelSQLDatabaseClient) FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error) {
	return c.traceRepo.FetchSpansByTraceID(ctx, traceID)
}

// FetchMetricsByTraceID delegates to the trace repository
func (c *OtelSQLDatabaseClient) FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error) {
	return c.traceRepo.FetchMetricsByTraceID(ctx, traceID)
}

// FetchProfilesByTraceID delegates to the trace repository
func (c *OtelSQLDatabaseClient) FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error) {
	return c.traceRepo.FetchProfilesByTraceID(ctx, traceID)
}

// FetchExemplarsBySpanIDFromType delegates to the trace repository
func (c *OtelSQLDatabaseClient) FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error) {
	return c.traceRepo.FetchExemplarsBySpanIDFromType(ctx, spanID, signalType)
}

// FetchProfilesBySpanID delegates to the trace repository
func (c *OtelSQLDatabaseClient) FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error) {
	return c.traceRepo.FetchProfilesBySpanID(ctx, spanID)
}

// StoreProcessedContent stores processed content for a specific span

// =========================
// OTEL Operations Delegation
// =========================

// QuerySpans delegates to trace repository
func (c *OtelSQLDatabaseClient) QuerySpans(ctx context.Context, q TraceQuery) ([]TraceEntry, string, bool, string, bool, error) {
	return c.traceRepo.QuerySpans(ctx, q)
}

// CountSpans delegates to trace repository
func (c *OtelSQLDatabaseClient) CountSpans(ctx context.Context, q TraceQuery) (int64, error) {
	return c.traceRepo.CountSpans(ctx, q)
}

// InsertSpans delegates to trace repository
func (c *OtelSQLDatabaseClient) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {
	return c.traceRepo.InsertSpans(ctx, entries, schema)
}

// QueryLogs delegates to log repository
func (c *OtelSQLDatabaseClient) QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, string, bool, error) {
	return c.logRepo.QueryLogs(ctx, q)
}

// CountLogs delegates to log repository
func (c *OtelSQLDatabaseClient) CountLogs(ctx context.Context, q LogQuery) (int64, error) {
	return c.logRepo.CountLogs(ctx, q)
}

// InsertLogs delegates to log repository
func (c *OtelSQLDatabaseClient) InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error) {
	return c.logRepo.InsertLogs(ctx, entries, schema)
}

// QueryGaugeMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) QueryGaugeMetrics(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsGauge(ctx, q)
}

// CountGaugeMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) CountGaugeMetrics(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsGauge(ctx, q)
}

// InsertGaugeMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {
	return c.metricRepo.InsertGaugeMetrics(ctx, entries, schema)
}

// QuerySumMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) QuerySumMetrics(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsSum(ctx, q)
}

// CountSumMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) CountSumMetrics(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsSum(ctx, q)
}

// InsertSumMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error) {
	return c.metricRepo.InsertSumMetrics(ctx, entries, schema)
}

// QueryHistogramMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) QueryHistogramMetrics(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsHistogram(ctx, q)
}

// CountHistogramMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) CountHistogramMetrics(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsHistogram(ctx, q)
}

// InsertHistogramMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error) {
	return c.metricRepo.InsertHistogramMetrics(ctx, entries, schema)
}

// QuerySummaryMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) QuerySummaryMetrics(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsSummary(ctx, q)
}

// CountSummaryMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) CountSummaryMetrics(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsSummary(ctx, q)
}

// InsertSummaryMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error) {
	return c.metricRepo.InsertSummaryMetrics(ctx, entries, schema)
}

// QueryExponentialHistogramMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) QueryExponentialHistogramMetrics(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsExponentialHistogram(ctx, q)
}

// CountExponentialHistogramMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) CountExponentialHistogramMetrics(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsExponentialHistogram(ctx, q)
}

// InsertExponentialHistogramMetrics delegates to metric repository
func (c *OtelSQLDatabaseClient) InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error) {
	return c.metricRepo.InsertExponentialHistogramMetrics(ctx, entries, schema)
}

// GetMetricByID delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) GetMetricByID(ctx context.Context, id int64, schema string) (interface{}, string, error) {
	return c.metricRepo.GetMetricByID(ctx, id, schema)
}

func (c *OtelSQLDatabaseClient) GetMetricByIDTyped(ctx context.Context, metricType string, id int64, schema string) (interface{}, string, error) {
	if typed, ok := c.metricRepo.(TypedMetricRepository); ok {
		return typed.GetMetricByIDTyped(ctx, metricType, id, schema)
	}
	return c.metricRepo.GetMetricByID(ctx, id, schema)
}

// GetLogByID delegates to log repository
func (c *OtelSQLDatabaseClient) GetLogByID(ctx context.Context, id int64, schema string) (LogEntry, error) {
	return c.logRepo.GetLogByID(ctx, id, schema)
}

// GetSpanByID delegates to trace repository
func (c *OtelSQLDatabaseClient) GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntry, error) {
	return c.traceRepo.GetSpanByID(ctx, id, schema)
}

// QueryProfiles delegates to profile repository
func (c *OtelSQLDatabaseClient) QueryProfiles(ctx context.Context, q ProfileQuery) ([]ProfileEntry, string, bool, error) {
	return c.profileRepo.QueryProfiles(ctx, q)
}

// InsertProfiles delegates to profile repository
func (c *OtelSQLDatabaseClient) InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error) {
	return c.profileRepo.InsertProfiles(ctx, entries, schema)
}

// QueryRuns delegates to run repository
func (c *OtelSQLDatabaseClient) QueryRuns(ctx context.Context, q RunQuery) ([]RunEntry, string, bool, error) {
	return c.runRepo.QueryRuns(ctx, q)
}

// InsertRuns delegates to run repository
func (c *OtelSQLDatabaseClient) InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error) {
	return c.runRepo.InsertRuns(ctx, entries, schema)
}

// UpdateRunMetadata delegates to run repository
func (c *OtelSQLDatabaseClient) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error {
	return c.runRepo.UpdateRunMetadata(ctx, runID, metadata, schema)
}

// EndRun delegates to run repository
func (c *OtelSQLDatabaseClient) EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error {
	return c.runRepo.EndRun(ctx, name, endTimestampNs, schema)
}

// GetRunByID delegates to run repository
func (c *OtelSQLDatabaseClient) GetRunByID(ctx context.Context, id int64, schema string) (RunEntry, error) {
	return c.runRepo.GetRunByID(ctx, id, schema)
}

// =========================
// MetricRepository Interface Compatibility
// =========================

// QueryMetricsGauge delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) QueryMetricsGauge(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsGauge(ctx, q)
}

// QueryMetricsSum delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) QueryMetricsSum(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsSum(ctx, q)
}

// QueryMetricsHistogram delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) QueryMetricsHistogram(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsHistogram(ctx, q)
}

// QueryMetricsSummary delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) QueryMetricsSummary(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsSummary(ctx, q)
}

// QueryMetricsExponentialHistogram delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	return c.metricRepo.QueryMetricsExponentialHistogram(ctx, q)
}

// CountMetricsGauge delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) CountMetricsGauge(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsGauge(ctx, q)
}

// CountMetricsSum delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) CountMetricsSum(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsSum(ctx, q)
}

// CountMetricsHistogram delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) CountMetricsHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsHistogram(ctx, q)
}

// CountMetricsSummary delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) CountMetricsSummary(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsSummary(ctx, q)
}

// CountMetricsExponentialHistogram delegates to metric repository (MetricRepository interface)
func (c *OtelSQLDatabaseClient) CountMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return c.metricRepo.CountMetricsExponentialHistogram(ctx, q)
}

// =========================
// Run Operations
// =========================
