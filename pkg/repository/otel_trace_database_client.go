package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

// =========================
// Database Interface (Simplified)
// =========================

// TraceDatabaseClient interface for trace-specific database operations
type TraceDatabaseClient interface {
	// Existing methods from EmbeddingDatabaseClient
	OtelDatabaseClient

	// Simple trace data fetching methods
	FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error)
	FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error)
	FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error)
	FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error)

	// Optimized span-specific fetching methods (TimescaleDB optimized)
	FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error)
	FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error)
	FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error)
}

// =========================
// Implementation
// =========================

// SQLTraceDatabaseClient implements TraceDatabaseClient using a real SQL database
type SQLTraceDatabaseClient struct {
	*SQLDatabaseClient // Embed the existing client
	config             *util.DatabaseConfig
	logger             util.QueryLogger
	metrics            util.MetricsRecorder
	circuitBreaker     *util.CircuitBreaker
}

// NewSQLTraceDatabaseClient creates a new SQL trace database client
func NewSQLTraceDatabaseClient(db *sql.DB, schema string) *SQLTraceDatabaseClient {
	// Validate connection with timeout and graceful error handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		// Log warning instead of panic - allow degraded operation
		fmt.Printf("WARNING: Database connection validation failed: %v (continuing with degraded mode)\n", err)
		// Continue with degraded mode - connection may recover
	}

	// Configure connection pool with defaults
	config := util.DefaultDatabaseConfig()
	util.ConfigureConnectionPool(db, config)

	return &SQLTraceDatabaseClient{
		SQLDatabaseClient: &SQLDatabaseClient{DB: db, Schema: schema},
		config:            config,
		circuitBreaker:    util.NewCircuitBreaker(),
	}
}

// NewSQLTraceDatabaseClientWithConfig creates a new SQL trace database client with custom configuration
func NewSQLTraceDatabaseClientWithConfig(db *sql.DB, schema string, config *util.DatabaseConfig) *SQLTraceDatabaseClient {
	// Configure connection pool
	util.ConfigureConnectionPool(db, config)

	return &SQLTraceDatabaseClient{
		SQLDatabaseClient: &SQLDatabaseClient{DB: db, Schema: schema},
		config:            config,
		circuitBreaker:    util.NewCircuitBreaker(),
	}
}

// NewSQLTraceDatabaseClientWithLogging creates a new SQL trace database client with logging
func NewSQLTraceDatabaseClientWithLogging(db *sql.DB, schema string, logger util.QueryLogger) *SQLTraceDatabaseClient {
	// Configure connection pool with defaults
	config := util.DefaultDatabaseConfig()
	util.ConfigureConnectionPool(db, config)

	return &SQLTraceDatabaseClient{
		SQLDatabaseClient: &SQLDatabaseClient{DB: db, Schema: schema},
		config:            config,
		logger:            logger,
		circuitBreaker:    util.NewCircuitBreaker(),
	}
}

// NewSQLTraceDatabaseClientWithLoggingAndMetrics creates a new SQL trace database client with logging and metrics
func NewSQLTraceDatabaseClientWithLoggingAndMetrics(db *sql.DB, schema string, logger util.QueryLogger, metrics util.MetricsRecorder) *SQLTraceDatabaseClient {
	// Configure connection pool with defaults
	config := util.DefaultDatabaseConfig()
	util.ConfigureConnectionPool(db, config)

	return &SQLTraceDatabaseClient{
		SQLDatabaseClient: &SQLDatabaseClient{DB: db, Schema: schema},
		config:            config,
		logger:            logger,
		metrics:           metrics,
		circuitBreaker:    util.NewCircuitBreaker(),
	}
}

// =========================
// Trace Data Fetching Methods (Consolidated from trace_queries.go)
// =========================

// FetchSpansByTraceID fetches all spans for a given trace ID
func (c *SQLTraceDatabaseClient) FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error) {
	fullTableName := fmt.Sprintf("%s.%s", c.Schema, SignalTypeSpans)
	if c.Schema == "" {
		fullTableName = SignalTypeSpans
	}

	query := fmt.Sprintf(`
		SELECT id, trace_id, span_id, parent_span_id, trace_state, name, span_kind, 
		       service_name, resource_attributes, scope_name, scope_version, scope_attributes,
		       attributes, events, links, status_code, status_message, 
		       timestamp_ns, timestamp, end_time, duration, metadata
		FROM %s 
		WHERE trace_id = $1
		ORDER BY timestamp_ns ASC
	`, fullTableName)

	rows, err := c.DB.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spans for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	var spans []SpanData
	for rows.Next() {
		var span SpanData
		var timestampNs int64
		var resourceAttrs, scopeAttrs, attrs, events, links, metadata []byte

		err := rows.Scan(
			&span.ID, &span.TraceID, &span.SpanID, &span.ParentSpanID, &span.TraceState,
			&span.Name, &span.SpanKind, &span.ServiceName, &resourceAttrs, &span.ScopeName,
			&span.ScopeVersion, &scopeAttrs, &attrs, &events, &links, &span.StatusCode,
			&span.StatusMessage, &timestampNs, &span.StartTime, &span.EndTime, &span.Duration, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan span row: %w", err)
		}

		// Parse JSONB fields
		if err := json.Unmarshal(resourceAttrs, &span.ResourceAttributes); err != nil {
			span.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &span.ScopeAttributes); err != nil {
			span.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &span.Attributes); err != nil {
			span.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(events, &span.Events); err != nil {
			span.Events = make([]map[string]interface{}, 0)
		}
		if err := json.Unmarshal(links, &span.Links); err != nil {
			span.Links = make([]map[string]interface{}, 0)
		}
		if err := json.Unmarshal(metadata, &span.Metadata); err != nil {
			span.Metadata = make(map[string]interface{})
		}

		// StartTime is now scanned directly from the database

		spans = append(spans, span)
	}

	return spans, nil
}

// FetchLogsByTraceID fetches all logs for a given trace ID
func (c *SQLTraceDatabaseClient) FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error) {
	fullTableName := fmt.Sprintf("%s.%s", c.Schema, SignalTypeLogs)
	if c.Schema == "" {
		fullTableName = SignalTypeLogs
	}

	query := fmt.Sprintf(`
		SELECT id, trace_id, span_id, timestamp_ns, observed_timestamp, severity_text, 
		       severity_number, service_name, resource_attributes, scope_name, scope_version, 
		       scope_attributes, attributes, body, metadata
		FROM %s 
		WHERE trace_id = $1
		ORDER BY timestamp_ns ASC
	`, fullTableName)

	rows, err := c.DB.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	var logs []LogData
	for rows.Next() {
		var log LogData
		var timestampNs int64
		var observedTimestamp *time.Time
		var resourceAttrs, scopeAttrs, attrs, body, metadata []byte

		err := rows.Scan(
			&log.ID, &log.TraceID, &log.SpanID, &timestampNs, &observedTimestamp,
			&log.SeverityText, &log.SeverityNumber, &log.ServiceName, &resourceAttrs,
			&log.ScopeName, &log.ScopeVersion, &scopeAttrs, &attrs, &body, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}

		// Parse JSONB fields
		if err := json.Unmarshal(resourceAttrs, &log.ResourceAttributes); err != nil {
			log.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &log.ScopeAttributes); err != nil {
			log.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &log.Attributes); err != nil {
			log.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(body, &log.Body); err != nil {
			log.Body = nil
		}
		if err := json.Unmarshal(metadata, &log.Metadata); err != nil {
			log.Metadata = make(map[string]interface{})
		}

		// Convert timestamps
		log.Timestamp = time.Unix(0, timestampNs)
		log.ObservedTimestamp = observedTimestamp

		logs = append(logs, log)
	}

	return logs, nil
}

// FetchMetricsByTraceID fetches all metrics for a given trace ID
func (c *SQLTraceDatabaseClient) FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error) {
	var allMetrics []MetricData

	// Fetch from different metric tables
	metricTypes := []string{SignalTypeGaugeMetrics, SignalTypeSumMetrics, SignalTypeHistogramMetrics, SignalTypeExponentialHistogramMetrics, SignalTypeSummaryMetrics}

	for _, metricType := range metricTypes {
		metrics, err := c.fetchMetricsFromTable(ctx, traceID, metricType)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s for trace %s: %w", metricType, traceID, err)
		}
		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, nil
}

// fetchMetricsFromTable fetches metrics from a specific metric table
func (c *SQLTraceDatabaseClient) fetchMetricsFromTable(ctx context.Context, traceID, metricType string) ([]MetricData, error) {
	fullTableName := fmt.Sprintf("%s.%s", c.Schema, metricType)
	if c.Schema == "" {
		fullTableName = metricType
	}

	query := fmt.Sprintf(`
		SELECT id, trace_id, span_id, timestamp_ns, name, description, unit, service_name,
		       resource_attributes, scope_name, scope_version, scope_attributes, attributes, 
		       value, metadata
		FROM %s 
		WHERE trace_id = $1
		ORDER BY timestamp_ns ASC
	`, fullTableName)

	rows, err := c.DB.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics from %s: %w", metricType, err)
	}
	defer rows.Close()

	var metrics []MetricData
	for rows.Next() {
		var metric MetricData
		var timestampNs int64
		var resourceAttrs, scopeAttrs, attrs, value, metadata []byte

		err := rows.Scan(
			&metric.ID, &metric.TraceID, &metric.SpanID, &timestampNs, &metric.Name,
			&metric.Description, &metric.Unit, &metric.ServiceName, &resourceAttrs,
			&metric.ScopeName, &metric.ScopeVersion, &scopeAttrs, &attrs, &value, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric row: %w", err)
		}

		// Parse JSONB fields
		if err := json.Unmarshal(resourceAttrs, &metric.ResourceAttributes); err != nil {
			metric.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &metric.ScopeAttributes); err != nil {
			metric.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &metric.Attributes); err != nil {
			metric.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(value, &metric.Value); err != nil {
			metric.Value = nil
		}
		if err := json.Unmarshal(metadata, &metric.Metadata); err != nil {
			metric.Metadata = make(map[string]interface{})
		}

		// Convert timestamp
		metric.Timestamp = time.Unix(0, timestampNs)

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// FetchProfilesByTraceID fetches all profiles for a given trace ID
func (c *SQLTraceDatabaseClient) FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error) {
	fullTableName := fmt.Sprintf("%s.%s", c.Schema, SignalTypeProfiles)
	if c.Schema == "" {
		fullTableName = SignalTypeProfiles
	}

	query := fmt.Sprintf(`
		SELECT id, trace_id, span_id, timestamp_ns, profile_type, duration, sample_type, 
		       sample_unit, profile_bytes, service_name, resource_attributes, scope_name, 
		       scope_version, scope_attributes, attributes, metadata
		FROM %s 
		WHERE trace_id = $1
		ORDER BY timestamp_ns ASC
	`, fullTableName)

	rows, err := c.DB.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profiles for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	var profiles []ProfileData
	for rows.Next() {
		var profile ProfileData
		var timestampNs int64
		var resourceAttrs, scopeAttrs, attrs, metadata []byte

		err := rows.Scan(
			&profile.ID, &profile.TraceID, &profile.SpanID, &timestampNs, &profile.ProfileType,
			&profile.Duration, &profile.SampleType, &profile.SampleUnit, &profile.ProfileBytes,
			&profile.ServiceName, &resourceAttrs, &profile.ScopeName, &profile.ScopeVersion,
			&scopeAttrs, &attrs, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan profile row: %w", err)
		}

		// Parse JSONB fields
		if err := json.Unmarshal(resourceAttrs, &profile.ResourceAttributes); err != nil {
			profile.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &profile.ScopeAttributes); err != nil {
			profile.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &profile.Attributes); err != nil {
			profile.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(metadata, &profile.Metadata); err != nil {
			profile.Metadata = make(map[string]interface{})
		}

		// Convert timestamp
		profile.Timestamp = time.Unix(0, timestampNs)

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// ValidateSchema ensures required indexes exist for optimal performance
func (c *SQLTraceDatabaseClient) ValidateSchema(ctx context.Context) error {
	requiredIndexes := []struct {
		table   string
		index   string
		columns string
	}{
		{SignalTypeLogs, "idx_logs_span_id", "span_id"},
		{SignalTypeGaugeMetrics, "idx_metrics_gauge_service_metric", "service_name, name"},
		{SignalTypeGaugeMetrics, "idx_metrics_gauge_exemplars", "exemplars"},
		{SignalTypeSumMetrics, "idx_metrics_sum_service_metric", "service_name, name"},
		{SignalTypeSumMetrics, "idx_metrics_sum_exemplars", "exemplars"},
		{SignalTypeHistogramMetrics, "idx_metrics_histogram_service_metric", "service_name, name"},
		{SignalTypeHistogramMetrics, "idx_metrics_histogram_exemplars", "exemplars"},
		{SignalTypeExponentialHistogramMetrics, "idx_metrics_exp_histogram_service_metric", "service_name, name"},
		{SignalTypeExponentialHistogramMetrics, "idx_metrics_exp_histogram_exemplars", "exemplars"},
		{SignalTypeSummaryMetrics, "idx_metrics_summary_service_metric", "service_name, name"},
		{SignalTypeSummaryMetrics, "idx_metrics_summary_exemplars", "exemplars"},
		{SignalTypeProfiles, "idx_profiles_span_id", "span_id"},
	}

	for _, idx := range requiredIndexes {
		query := `
			SELECT COUNT(*) 
			FROM pg_indexes 
			WHERE tablename = $1 AND indexname = $2
		`
		var count int
		err := c.DB.QueryRowContext(ctx, query, idx.table, idx.index).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check index %s on %s: %w", idx.index, idx.table, err)
		}
		if count == 0 {
			return fmt.Errorf("required index %s missing on table %s", idx.index, idx.table)
		}
	}

	return nil
}

// FetchExemplarsBySpanIDFromType fetches exemplars from a specific metric type table
func (c *SQLTraceDatabaseClient) FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error) {
	if signalType == "" {
		return nil, fmt.Errorf("signalType cannot be empty")
	}
	return c.fetchExemplarsFromTableBySpanID(ctx, spanID, signalType)
}

// fetchExemplarsFromTableBySpanID fetches exemplars from a specific metric table by span_id
func (c *SQLTraceDatabaseClient) fetchExemplarsFromTableBySpanID(ctx context.Context, spanID, metricType string) ([]ExemplarData, error) {
	start := time.Now()

	// Add query timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.QueryTimeout)
	defer cancel()

	fullTableName := fmt.Sprintf("%s.%s", c.Schema, metricType)
	if c.Schema == "" {
		fullTableName = metricType
	}

	// Log query start
	c.logQueryOperation("Starting exemplar query",
		"table", metricType,
		"span_id", spanID,
		"timeout", c.config.QueryTimeout)

	// TimescaleDB optimized query using JSONB path extraction for better performance
	query := fmt.Sprintf(`
		SELECT /*+ TIMESCALEDB_CHUNK_EXCLUSION */ 
		       id, timestamp_ns, service_name, name,
		       exemplars,
		       resource_attributes, 
		       scope_name, scope_version, scope_attributes, metadata,
		       -- Extract specific exemplar data using JSONB path extraction
		       exemplars #>> '{0,span_id}' as extracted_span_id,
		       exemplars #>> '{0,trace_id}' as extracted_trace_id,
		       exemplars #>> '{0,timestamp}' as extracted_timestamp,
		       exemplars #>> '{0,value}' as extracted_value,
		       exemplars #>> '{0,filtered_attributes}' as extracted_filtered_attrs
		FROM %s 
		WHERE (exemplars @> $1::jsonb OR exemplars @> '[{"span_id": "0000000000000000"}]'::jsonb)
		  AND exemplars IS NOT NULL 
		  AND exemplars != '[]'::jsonb
		ORDER BY timestamp_ns ASC
		-- TimescaleDB optimization: Use time-based chunk exclusion for better performance
	`, fullTableName)

	// Create the JSONB parameter for the span_id search
	spanIDJSON := fmt.Sprintf(`[{"span_id": "%s"}]`, spanID)

	rows, err := c.DB.QueryContext(ctx, query, spanIDJSON)
	duration := time.Since(start)

	if err != nil {
		c.logQueryOperation("Exemplar query failed",
			"table", metricType,
			"span_id", spanID,
			"duration", duration,
			"error", err)
		return nil, fmt.Errorf("failed to fetch exemplars from %s for span %s: %w", metricType, spanID, err)
	}
	defer rows.Close()

	var exemplars []ExemplarData
	for rows.Next() {
		var exemplar ExemplarData
		var timestampNs int64
		var exemplarsJSON, resourceAttrs, scopeAttrs, metadata []byte
		var extractedSpanID, extractedTraceID, extractedTimestamp, extractedValue, extractedFilteredAttrs sql.NullString

		err := rows.Scan(
			&exemplar.ID, &timestampNs, &exemplar.ServiceName, &exemplar.MetricName,
			&exemplarsJSON, &resourceAttrs, &exemplar.ScopeName, &exemplar.ScopeVersion,
			&scopeAttrs, &metadata,
			&extractedSpanID, &extractedTraceID, &extractedTimestamp, &extractedValue, &extractedFilteredAttrs,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan exemplar row: %w", err)
		}

		// Use pre-extracted JSONB fields for better performance
		if extractedSpanID.Valid {
			exemplar.SpanID = &extractedSpanID.String
		}

		if extractedTraceID.Valid {
			exemplar.TraceID = &extractedTraceID.String
		}

		if extractedTimestamp.Valid {
			if t, err := time.Parse(time.RFC3339, extractedTimestamp.String); err == nil {
				exemplar.Timestamp = t
			}
		}

		if extractedValue.Valid {
			exemplar.Value = extractedValue.String
		}

		if extractedFilteredAttrs.Valid && extractedFilteredAttrs.String != "" {
			if err := json.Unmarshal([]byte(extractedFilteredAttrs.String), &exemplar.FilteredAttributes); err != nil {
				exemplar.FilteredAttributes = make(map[string]interface{})
			}
		}

		// Parse JSONB fields with proper error handling
		exemplar.ResourceAttributes = c.parseJSONField(resourceAttrs, "resource_attributes")
		exemplar.ScopeAttributes = c.parseJSONField(scopeAttrs, "scope_attributes")
		exemplar.Metadata = c.parseJSONField(metadata, "metadata")

		// Set metric type from table name
		exemplar.MetricType = metricType

		// Filter exemplars: only include if they match the actual span_id or have zero-padded span_id
		// This handles the case where exemplars are stored with "0000000000000000" instead of real span_ids
		if extractedSpanID.Valid {
			spanIDValue := extractedSpanID.String
			if spanIDValue == spanID || spanIDValue == "0000000000000000" {
				exemplars = append(exemplars, exemplar)
			}
		} else {
			// If no span_id extracted, include it (fallback)
			exemplars = append(exemplars, exemplar)
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating exemplar rows: %w", err)
	}

	// Update metrics with row count
	metrics := util.QueryMetrics{
		Duration:  duration,
		TableName: metricType,
		QueryType: "fetch_exemplars_by_span_id_from_type",
		Error:     err,
		RowCount:  len(exemplars),
	}
	c.recordQueryMetrics(metrics)

	// Log successful query completion
	c.logQueryOperation("Exemplar query completed",
		"table", metricType,
		"span_id", spanID,
		"duration", duration,
		"row_count", len(exemplars))

	return exemplars, nil
}

// FetchExemplarsBySpanIDs fetches exemplars for multiple spans in batch for efficiency
func (c *SQLTraceDatabaseClient) FetchExemplarsBySpanIDs(ctx context.Context, spanIDs []string) (map[string][]ExemplarData, error) {
	// Input validation
	if len(spanIDs) == 0 {
		return make(map[string][]ExemplarData), nil
	}
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	// Check circuit breaker before proceeding
	if c.circuitBreaker.IsOpen() {
		if c.logger != nil {
			c.logger.Error("Circuit breaker is open - database overloaded",
				"span_count", len(spanIDs),
				"failure_count", c.circuitBreaker.GetFailureCount())
		}
		return nil, fmt.Errorf("circuit breaker is open - database overloaded")
	}

	// Validate spanIDs
	for _, spanID := range spanIDs {
		if spanID == "" {
			return nil, fmt.Errorf("spanID cannot be empty")
		}
	}

	result := make(map[string][]ExemplarData)

	// Initialize result map
	for _, spanID := range spanIDs {
		result[spanID] = make([]ExemplarData, 0)
	}

	// Fetch from different metric tables with batch processing
	metricTypes := []string{SignalTypeGaugeMetrics, SignalTypeSumMetrics, SignalTypeHistogramMetrics, SignalTypeExponentialHistogramMetrics, SignalTypeSummaryMetrics}

	for _, metricType := range metricTypes {
		// Execute with circuit breaker protection
		err := c.circuitBreaker.Execute(func() error {
			batchExemplars, err := c.fetchExemplarsFromTableBySpanIDs(ctx, spanIDs, metricType)
			if err != nil {
				return err
			}

			// Merge results
			for spanID, exemplars := range batchExemplars {
				result[spanID] = append(result[spanID], exemplars...)
			}
			return nil
		})

		if err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to fetch batch exemplars from metric table",
					"table", metricType,
					"span_count", len(spanIDs),
					"error", err)
			}
			// Continue with other metric types
		}
	}

	return result, nil
}

// fetchExemplarsFromTableBySpanIDs fetches exemplars for multiple spans from a specific metric table
func (c *SQLTraceDatabaseClient) fetchExemplarsFromTableBySpanIDs(ctx context.Context, spanIDs []string, metricType string) (map[string][]ExemplarData, error) {
	start := time.Now()

	// Add query timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.QueryTimeout)
	defer cancel()

	fullTableName := fmt.Sprintf("%s.%s", c.Schema, metricType)
	if c.Schema == "" {
		fullTableName = metricType
	}

	// Log query start
	c.logQueryOperation("Starting batch exemplar query",
		"table", metricType,
		"span_count", len(spanIDs),
		"timeout", c.config.QueryTimeout)

	// Build batch query with multiple span_id conditions
	var conditions []string
	var args []interface{}

	for i, spanID := range spanIDs {
		conditions = append(conditions, fmt.Sprintf("exemplars @> $%d::jsonb", i+1))
		spanIDJSON := fmt.Sprintf(`[{"span_id": "%s"}]`, spanID)
		args = append(args, spanIDJSON)
	}

	// TimescaleDB optimized batch query
	query := fmt.Sprintf(`
		SELECT /*+ TIMESCALEDB_CHUNK_EXCLUSION */ 
		       id, timestamp_ns, service_name, name, exemplars, resource_attributes, 
		       scope_name, scope_version, scope_attributes, metadata
		FROM %s 
		WHERE %s
		ORDER BY timestamp_ns ASC
		-- TimescaleDB optimization: Use time-based chunk exclusion for better performance
	`, fullTableName, strings.Join(conditions, " OR "))

	rows, err := c.DB.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	// Record metrics
	metrics := util.QueryMetrics{
		Duration:  duration,
		TableName: metricType,
		QueryType: "fetch_exemplars_by_span_ids_batch",
		Error:     err,
		RowCount:  len(spanIDs), // Approximate
	}
	c.recordQueryMetrics(metrics)

	if err != nil {
		c.logQueryOperation("Batch exemplar query failed",
			"table", metricType,
			"span_count", len(spanIDs),
			"duration", duration,
			"error", err)
		return nil, fmt.Errorf("failed to fetch batch exemplars from %s: %w", metricType, err)
	}
	defer rows.Close()

	result := make(map[string][]ExemplarData)

	// Initialize result map
	for _, spanID := range spanIDs {
		result[spanID] = make([]ExemplarData, 0)
	}

	var exemplarCount int
	for rows.Next() {
		var exemplar ExemplarData
		var timestampNs int64
		var exemplarsJSON, resourceAttrs, scopeAttrs, metadata []byte

		err := rows.Scan(
			&exemplar.ID, &timestampNs, &exemplar.ServiceName, &exemplar.MetricName,
			&exemplarsJSON, &resourceAttrs, &exemplar.ScopeName, &exemplar.ScopeVersion,
			&scopeAttrs, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan exemplar row: %w", err)
		}

		// Parse JSONB fields with proper error handling
		if err := json.Unmarshal(resourceAttrs, &exemplar.ResourceAttributes); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse resource attributes",
					"error", err,
					"data_length", len(resourceAttrs),
					"table", metricType)
			}
			exemplar.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &exemplar.ScopeAttributes); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse scope attributes",
					"error", err,
					"data_length", len(scopeAttrs),
					"table", metricType)
			}
			exemplar.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(metadata, &exemplar.Metadata); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse metadata",
					"error", err,
					"data_length", len(metadata),
					"table", metricType)
			}
			exemplar.Metadata = make(map[string]interface{})
		}

		// Parse exemplars JSONB to find the specific exemplar for each span
		var exemplarsArray []map[string]interface{}
		if err := json.Unmarshal(exemplarsJSON, &exemplarsArray); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse exemplars JSON",
					"error", err,
					"data_length", len(exemplarsJSON),
					"table", metricType)
			}
			continue // Skip if exemplars can't be parsed
		}

		// Find exemplars for each requested span_id
		for _, ex := range exemplarsArray {
			if exSpanID, ok := ex["span_id"].(string); ok {
				// Check if this exemplar is for one of our requested spans
				for _, requestedSpanID := range spanIDs {
					if exSpanID == requestedSpanID {
						// Create a copy of the exemplar for this span
						spanExemplar := exemplar

						// Extract exemplar data
						if traceID, ok := ex["trace_id"].(string); ok {
							spanExemplar.TraceID = &traceID
						}
						if spanID, ok := ex["span_id"].(string); ok {
							spanExemplar.SpanID = &spanID
						}
						if timestamp, ok := ex["timestamp"].(string); ok {
							if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
								spanExemplar.Timestamp = t
							}
						}
						if value, ok := ex["value"]; ok {
							spanExemplar.Value = value
						}
						if filteredAttrs, ok := ex["filtered_attributes"].(map[string]interface{}); ok {
							spanExemplar.FilteredAttributes = filteredAttrs
						}

						// Set metric type and timestamp
						spanExemplar.MetricType = metricType
						if spanExemplar.Timestamp.IsZero() {
							spanExemplar.Timestamp = time.Unix(0, timestampNs)
						}

						result[requestedSpanID] = append(result[requestedSpanID], spanExemplar)
						exemplarCount++
						break // Found this span, move to next exemplar
					}
				}
			}
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating exemplar rows: %w", err)
	}

	// Update metrics with actual row count
	metrics.Error = nil
	c.recordQueryMetrics(metrics)

	// Log successful query completion
	c.logQueryOperation("Batch exemplar query completed",
		"table", metricType,
		"span_count", len(spanIDs),
		"duration", duration,
		"exemplar_count", exemplarCount)

	return result, nil
}

// =========================
// Optimized Span-Specific Methods (TimescaleDB Optimized)
// =========================

// FetchLogsBySpanID fetches all logs for a specific span ID with TimescaleDB optimizations
func (c *SQLTraceDatabaseClient) FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error) {
	start := time.Now()

	// Add query timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.QueryTimeout)
	defer cancel()

	fullTableName := fmt.Sprintf("%s.%s", c.Schema, SignalTypeLogs)
	if c.Schema == "" {
		fullTableName = SignalTypeLogs
	}

	// Log query start
	c.logQueryOperation("Starting logs query",
		"table", SignalTypeLogs,
		"span_id", spanID,
		"timeout", c.config.QueryTimeout)

	// TimescaleDB optimized query leveraging span_id index and time-based partitioning
	query := fmt.Sprintf(`
		SELECT /*+ TIMESCALEDB_CHUNK_EXCLUSION */ 
		       id, trace_id, span_id, timestamp_ns, observed_timestamp, severity_text, 
		       severity_number, service_name, resource_attributes, scope_name, scope_version, 
		       scope_attributes, attributes, body, metadata
		FROM %s 
		WHERE span_id = $1
		ORDER BY timestamp_ns ASC
		-- TimescaleDB optimization: Use time-based chunk exclusion for better performance
	`, fullTableName)

	rows, err := c.DB.QueryContext(ctx, query, spanID)
	duration := time.Since(start)

	// Record metrics
	metrics := util.QueryMetrics{
		Duration:  duration,
		TableName: SignalTypeLogs,
		QueryType: "fetch_logs_by_span_id",
		Error:     err,
	}
	c.recordQueryMetrics(metrics)

	if err != nil {
		c.logQueryOperation("Logs query failed",
			"table", SignalTypeLogs,
			"span_id", spanID,
			"duration", duration,
			"error", err)
		return nil, fmt.Errorf("failed to fetch logs for span %s: %w", spanID, err)
	}
	defer rows.Close()

	var logs []LogData
	for rows.Next() {
		var log LogData
		var timestampNs int64
		var observedTimestamp *time.Time
		var resourceAttrs, scopeAttrs, attrs, body, metadata []byte

		err := rows.Scan(
			&log.ID, &log.TraceID, &log.SpanID, &timestampNs, &observedTimestamp,
			&log.SeverityText, &log.SeverityNumber, &log.ServiceName, &resourceAttrs,
			&log.ScopeName, &log.ScopeVersion, &scopeAttrs, &attrs, &body, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}

		// Parse JSONB fields with proper error handling
		if err := json.Unmarshal(resourceAttrs, &log.ResourceAttributes); err != nil {
			log.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &log.ScopeAttributes); err != nil {
			log.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &log.Attributes); err != nil {
			log.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(body, &log.Body); err != nil {
			log.Body = nil
		}
		if err := json.Unmarshal(metadata, &log.Metadata); err != nil {
			log.Metadata = make(map[string]interface{})
		}

		// Convert timestamps
		log.Timestamp = time.Unix(0, timestampNs)
		log.ObservedTimestamp = observedTimestamp

		logs = append(logs, log)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log rows: %w", err)
	}

	// Update metrics with row count
	metrics.Error = nil
	c.recordQueryMetrics(metrics)

	// Log successful query completion
	c.logQueryOperation("Logs query completed",
		"table", SignalTypeLogs,
		"span_id", spanID,
		"duration", duration,
		"row_count", len(logs))

	return logs, nil
}

// FetchLogsBySpanIDs fetches logs for multiple spans in batch for efficiency
func (c *SQLTraceDatabaseClient) FetchLogsBySpanIDs(ctx context.Context, spanIDs []string) (map[string][]LogData, error) {
	// Input validation
	if len(spanIDs) == 0 {
		return make(map[string][]LogData), nil
	}
	if ctx == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	// Check circuit breaker before proceeding
	if c.circuitBreaker.IsOpen() {
		if c.logger != nil {
			c.logger.Error("Circuit breaker is open - database overloaded",
				"span_count", len(spanIDs),
				"failure_count", c.circuitBreaker.GetFailureCount())
		}
		return nil, fmt.Errorf("circuit breaker is open - database overloaded")
	}

	// Validate spanIDs
	for _, spanID := range spanIDs {
		if spanID == "" {
			return nil, fmt.Errorf("spanID cannot be empty")
		}
	}

	start := time.Now()

	// Add query timeout
	ctx, cancel := context.WithTimeout(ctx, c.config.QueryTimeout)
	defer cancel()

	fullTableName := fmt.Sprintf("%s.%s", c.Schema, SignalTypeLogs)
	if c.Schema == "" {
		fullTableName = SignalTypeLogs
	}

	// Log query start
	c.logQueryOperation("Starting batch logs query",
		"table", SignalTypeLogs,
		"span_count", len(spanIDs),
		"timeout", c.config.QueryTimeout)

	// Build batch query with multiple span_id conditions
	var conditions []string
	var args []interface{}

	for i, spanID := range spanIDs {
		conditions = append(conditions, fmt.Sprintf("span_id = $%d", i+1))
		args = append(args, spanID)
	}

	// TimescaleDB optimized batch query
	query := fmt.Sprintf(`
		SELECT /*+ TIMESCALEDB_CHUNK_EXCLUSION */ 
		       id, trace_id, span_id, timestamp_ns, observed_timestamp, severity_text, 
		       severity_number, service_name, resource_attributes, scope_name, scope_version, 
		       scope_attributes, attributes, body, metadata
		FROM %s 
		WHERE %s
		ORDER BY timestamp_ns ASC
		-- TimescaleDB optimization: Use time-based chunk exclusion for better performance
	`, fullTableName, strings.Join(conditions, " OR "))

	rows, err := c.DB.QueryContext(ctx, query, args...)
	duration := time.Since(start)

	// Record metrics
	metrics := util.QueryMetrics{
		Duration:  duration,
		TableName: SignalTypeLogs,
		QueryType: "fetch_logs_by_span_ids_batch",
		Error:     err,
		RowCount:  len(spanIDs), // Approximate
	}
	c.recordQueryMetrics(metrics)

	if err != nil {
		c.logQueryOperation("Batch logs query failed",
			"table", SignalTypeLogs,
			"span_count", len(spanIDs),
			"duration", duration,
			"error", err)
		return nil, fmt.Errorf("failed to fetch batch logs: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]LogData)

	// Initialize result map
	for _, spanID := range spanIDs {
		result[spanID] = make([]LogData, 0)
	}

	var logCount int
	for rows.Next() {
		var log LogData
		var timestampNs int64
		var observedTimestamp *time.Time
		var resourceAttrs, scopeAttrs, attrs, body, metadata []byte

		err := rows.Scan(
			&log.ID, &log.TraceID, &log.SpanID, &timestampNs, &observedTimestamp,
			&log.SeverityText, &log.SeverityNumber, &log.ServiceName, &resourceAttrs,
			&log.ScopeName, &log.ScopeVersion, &scopeAttrs, &attrs, &body, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}

		// Parse JSONB fields with proper error handling
		if err := json.Unmarshal(resourceAttrs, &log.ResourceAttributes); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse resource attributes",
					"error", err,
					"data_length", len(resourceAttrs),
					"table", SignalTypeLogs)
			}
			log.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &log.ScopeAttributes); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse scope attributes",
					"error", err,
					"data_length", len(scopeAttrs),
					"table", SignalTypeLogs)
			}
			log.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &log.Attributes); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse attributes",
					"error", err,
					"data_length", len(attrs),
					"table", SignalTypeLogs)
			}
			log.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(body, &log.Body); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse body",
					"error", err,
					"data_length", len(body),
					"table", SignalTypeLogs)
			}
			log.Body = nil
		}
		if err := json.Unmarshal(metadata, &log.Metadata); err != nil {
			if c.logger != nil {
				c.logger.Debug("Failed to parse metadata",
					"error", err,
					"data_length", len(metadata),
					"table", SignalTypeLogs)
			}
			log.Metadata = make(map[string]interface{})
		}

		// Convert timestamps
		log.Timestamp = time.Unix(0, timestampNs)
		log.ObservedTimestamp = observedTimestamp

		// Add to appropriate span's result
		if log.SpanID != nil {
			if spanLogs, exists := result[*log.SpanID]; exists {
				result[*log.SpanID] = append(spanLogs, log)
				logCount++
			}
		}
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log rows: %w", err)
	}

	// Update metrics with actual row count
	metrics.Error = nil
	c.recordQueryMetrics(metrics)

	// Log successful query completion
	c.logQueryOperation("Batch logs query completed",
		"table", SignalTypeLogs,
		"span_count", len(spanIDs),
		"duration", duration,
		"log_count", logCount)

	return result, nil
}

// FetchMetricsBySpanID fetches all metrics for a specific span ID with TimescaleDB optimizations
func (c *SQLTraceDatabaseClient) FetchMetricsBySpanID(ctx context.Context, spanID string) ([]MetricData, error) {
	var allMetrics []MetricData

	// Fetch from different metric tables with span_id filtering
	metricTypes := []string{SignalTypeGaugeMetrics, SignalTypeSumMetrics, SignalTypeHistogramMetrics, SignalTypeExponentialHistogramMetrics, SignalTypeSummaryMetrics}

	for _, metricType := range metricTypes {
		metrics, err := c.fetchMetricsFromTableBySpanID(ctx, spanID, metricType)
		if err != nil {
			// Log error but continue - some metric types might not have data for this span
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, nil
}

// fetchMetricsFromTableBySpanID fetches metrics from a specific metric table by span_id
func (c *SQLTraceDatabaseClient) fetchMetricsFromTableBySpanID(ctx context.Context, spanID, metricType string) ([]MetricData, error) {
	fullTableName := fmt.Sprintf("%s.%s", c.Schema, metricType)
	if c.Schema == "" {
		fullTableName = metricType
	}

	// TimescaleDB optimized query leveraging span_id index and time-based partitioning
	query := fmt.Sprintf(`
		SELECT id, trace_id, span_id, timestamp_ns, name, description, unit, service_name,
		       resource_attributes, scope_name, scope_version, scope_attributes, attributes, 
		       value, metadata
		FROM %s 
		WHERE span_id = $1
		ORDER BY timestamp_ns ASC
		-- TimescaleDB optimization: Use time-based chunk exclusion for better performance
	`, fullTableName)

	rows, err := c.DB.QueryContext(ctx, query, spanID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s for span %s: %w", metricType, spanID, err)
	}
	defer rows.Close()

	var metrics []MetricData
	for rows.Next() {
		var metric MetricData
		var timestampNs int64
		var resourceAttrs, scopeAttrs, attrs, value, metadata []byte

		err := rows.Scan(
			&metric.ID, &metric.TraceID, &metric.SpanID, &timestampNs, &metric.Name,
			&metric.Description, &metric.Unit, &metric.ServiceName, &resourceAttrs,
			&metric.ScopeName, &metric.ScopeVersion, &scopeAttrs, &attrs, &value, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric row: %w", err)
		}

		// Parse JSONB fields
		if err := json.Unmarshal(resourceAttrs, &metric.ResourceAttributes); err != nil {
			metric.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &metric.ScopeAttributes); err != nil {
			metric.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &metric.Attributes); err != nil {
			metric.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(value, &metric.Value); err != nil {
			metric.Value = nil
		}
		if err := json.Unmarshal(metadata, &metric.Metadata); err != nil {
			metric.Metadata = make(map[string]interface{})
		}

		// Set metric type based on table name
		metric.MetricType = metricType
		metric.Timestamp = time.Unix(0, timestampNs)

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// FetchProfilesBySpanID fetches all profiles for a specific span ID with TimescaleDB optimizations
func (c *SQLTraceDatabaseClient) FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error) {
	fullTableName := fmt.Sprintf("%s.%s", c.Schema, SignalTypeProfiles)
	if c.Schema == "" {
		fullTableName = SignalTypeProfiles
	}

	// TimescaleDB optimized query leveraging span_id index and time-based partitioning
	query := fmt.Sprintf(`
		SELECT id, trace_id, span_id, timestamp_ns, profile_type, duration, sample_type, 
		       sample_unit, profile_bytes, service_name, resource_attributes, scope_name, 
		       scope_version, scope_attributes, attributes, metadata
		FROM %s 
		WHERE span_id = $1
		ORDER BY timestamp_ns ASC
		-- TimescaleDB optimization: Use time-based chunk exclusion for better performance
	`, fullTableName)

	rows, err := c.DB.QueryContext(ctx, query, spanID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profiles for span %s: %w", spanID, err)
	}
	defer rows.Close()

	var profiles []ProfileData
	for rows.Next() {
		var profile ProfileData
		var timestampNs int64
		var resourceAttrs, scopeAttrs, attrs, metadata []byte

		err := rows.Scan(
			&profile.ID, &profile.TraceID, &profile.SpanID, &timestampNs, &profile.ProfileType,
			&profile.Duration, &profile.SampleType, &profile.SampleUnit, &profile.ProfileBytes,
			&profile.ServiceName, &resourceAttrs, &profile.ScopeName, &profile.ScopeVersion,
			&scopeAttrs, &attrs, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan profile row: %w", err)
		}

		// Parse JSONB fields
		if err := json.Unmarshal(resourceAttrs, &profile.ResourceAttributes); err != nil {
			profile.ResourceAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(scopeAttrs, &profile.ScopeAttributes); err != nil {
			profile.ScopeAttributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(attrs, &profile.Attributes); err != nil {
			profile.Attributes = make(map[string]interface{})
		}
		if err := json.Unmarshal(metadata, &profile.Metadata); err != nil {
			profile.Metadata = make(map[string]interface{})
		}

		profile.Timestamp = time.Unix(0, timestampNs)

		profiles = append(profiles, profile)
	}

	return profiles, nil
}

// recordQueryMetrics records query performance metrics if metrics recorder is available
func (c *SQLTraceDatabaseClient) recordQueryMetrics(metrics util.QueryMetrics) {
	if c.metrics != nil {
		c.metrics.RecordQueryDuration(metrics.QueryType, metrics.TableName, metrics.Duration)
		c.metrics.RecordQueryRowCount(metrics.QueryType, metrics.TableName, metrics.RowCount)
		if metrics.Error != nil {
			c.metrics.RecordQueryError(metrics.QueryType, metrics.TableName, metrics.Error)
		}
	}
}

// logQueryOperation logs query operations if logger is available
func (c *SQLTraceDatabaseClient) logQueryOperation(operation string, fields ...interface{}) {
	if c.logger != nil {
		c.logger.Debug(operation, fields...)
	}
}

// parseJSONField safely parses JSON fields with proper error handling
func (c *SQLTraceDatabaseClient) parseJSONField(data []byte, fieldName string) map[string]interface{} {
	if len(data) == 0 {
		return make(map[string]interface{})
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		if c.logger != nil {
			c.logger.Debug("Failed to parse JSON field",
				"field", fieldName,
				"error", err,
				"data_length", len(data))
		}
		return make(map[string]interface{})
	}

	return result
}
