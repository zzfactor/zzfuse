package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// EnsureSchema ensures schema is not empty, returns DefaultSchema if empty
func EnsureSchema(schema string) string {
	if schema == "" {
		return DefaultSchema
	}
	return schema
}

// CursorableEntry interface for entries that support cursor pagination
type CursorableEntry interface {
	GetTimestampNS() int64
	GetIDString() string
}

// generateCursors creates nextCursor and prevCursor from entries with consistent logic
func generateCursors[T CursorableEntry](entries []T, hasMore bool, hasCursor bool) (nextCursor, prevCursor string, hasPrev bool) {
	if len(entries) == 0 {
		return "", "", false
	}

	// Generate nextCursor only when hasMore is true
	if hasMore {
		lastEntry := entries[len(entries)-1]
		nextCursor = EncodeCursor(time.Unix(0, lastEntry.GetTimestampNS()), lastEntry.GetIDString())
	}

	// Always generate prevCursor from first entry for consistency
	firstEntry := entries[0]
	prevCursor = EncodeCursor(time.Unix(0, firstEntry.GetTimestampNS()), firstEntry.GetIDString())

	// Set hasPrev only when not on first page
	hasPrev = hasCursor

	return nextCursor, prevCursor, hasPrev
}

// MetricColumnSet defines the column sets for a metric type
type MetricColumnSet struct {
	SelectColumns []string
	InsertColumns []string
}

// Common metric column sets - can be used to reduce duplication
var CommonMetricColumns = struct {
	Base     []string
	Temporal []string
	Scope    []string
	Resource []string
}{
	Base:     []string{"id", "timestamp_ns", "timestamp", "service_name", "service_namespace", "service_instance_id", "name", "description", "unit", "attributes"},
	Temporal: []string{"flags", "exemplars"},
	Scope:    []string{"scope_name", "scope_version", "scope_attributes", "scope_dropped_attr_count"},
	Resource: []string{"resource_attributes"},
}

// buildMetricColumns creates select and insert column sets from components
func buildMetricColumns(specificCols []string) MetricColumnSet {
	selectCols := make([]string, 0, len(CommonMetricColumns.Base)+len(specificCols)+len(CommonMetricColumns.Temporal)+len(CommonMetricColumns.Scope)+len(CommonMetricColumns.Resource))
	selectCols = append(selectCols, CommonMetricColumns.Base...)
	selectCols = append(selectCols, specificCols...)
	selectCols = append(selectCols, CommonMetricColumns.Temporal...)
	selectCols = append(selectCols, CommonMetricColumns.Resource...)
	selectCols = append(selectCols, CommonMetricColumns.Scope...)
	selectCols = append(selectCols, "metadata", "resource_schema_url", "scope_schema_url")

	// Both id and the generated timestamp are excluded from writable columns.
	insertCols := append([]string{}, selectCols[1:2]...)
	insertCols = append(insertCols, selectCols[3:]...)

	return MetricColumnSet{
		SelectColumns: selectCols,
		InsertColumns: insertCols,
	}
}

// Generic query result structure for all telemetry types
type QueryResult[T any] struct {
	Entries    []T    `json:"entries"`
	NextCursor string `json:"next_cursor"`
	HasMore    bool   `json:"has_more"`
	PrevCursor string `json:"prev_cursor,omitempty"`
	HasPrev    bool   `json:"has_prev,omitempty"`
}

// Common response field structure shared across all telemetry types
type BaseEntryResponse struct {
	ID                    int64       `json:"id"`
	TimestampNS           int64       `json:"timestamp_ns"`
	Timestamp             string      `json:"timestamp"`
	ServiceName           string      `json:"service_name"`
	ResourceAttributes    interface{} `json:"resource_attributes,omitempty"`
	ResourceSchemaURL     *string     `json:"resource_schema_url,omitempty"`
	ScopeName             *string     `json:"scope_name,omitempty"`
	ScopeVersion          *string     `json:"scope_version,omitempty"`
	ScopeAttributes       interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount *int32      `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL        *string     `json:"scope_schema_url,omitempty"`
	Metadata              interface{} `json:"metadata,omitempty"`
}

// Generic service interface pattern
type TelemetryService[TEntry any, TQuery any, TResponse any] interface {
	Insert(ctx context.Context, entries []TEntry, schema string) ([]string, error)
	Query(ctx context.Context, query TQuery) (QueryResult[TResponse], error)
	Count(ctx context.Context, query TQuery) (int64, error)
}

// shouldSkipTemplateProcessing checks if template processing should be skipped
func shouldSkipTemplateProcessing() bool {
	return os.Getenv("SKIP_TEMPLATE_PROCESSING") == "true"
}

// shouldSkipHashProcessing checks if hash processing should be skipped
func shouldSkipHashProcessing() bool {
	return os.Getenv("SKIP_HASH_PROCESSING") == "true"
}

// templateProcessingMetrics tracks template processing usage for observability
type templateProcessingMetrics struct {
	enabled    bool
	startTime  time.Time
	batchSize  int
	signalType string
	context    string
}

// newTemplateProcessingMetrics creates a new metrics tracker
func newTemplateProcessingMetrics(signalType string, batchSize int) *templateProcessingMetrics {
	return &templateProcessingMetrics{
		enabled:    !shouldSkipTemplateProcessing(),
		startTime:  time.Now(),
		batchSize:  batchSize,
		signalType: signalType,
		context:    "pre_insert",
	}
}

// recordMetrics records template processing metrics
func (m *templateProcessingMetrics) recordMetrics(err error) {
	// Currently a no-op placeholder. Keeps hook for future metrics without impacting performance.
	// Ensure method safely returns if metrics disabled or struct is nil.
	if m == nil || !m.enabled {
		return
	}
	// Suppress unused parameter warning when metrics export is not wired yet.
	_ = err
}

// insertHashesWithTx inserts signal hashes using the provided transaction
func insertHashesWithTx(ctx context.Context, tx *sql.Tx, hashes []*database.SignalHash, signalIDs []string, schema string) error {
	if len(hashes) == 0 {
		return nil
	}

	// Deduplicate hashes by (timestamp_ns, hash) to prevent ON CONFLICT issues
	// Keep the last occurrence of each unique combination
	hashMap := make(map[string]*database.SignalHash)
	for _, hash := range hashes {
		key := fmt.Sprintf("%d:%s", hash.TimestampNs, hash.Hash)
		hashMap[key] = hash
	}

	// Convert back to slice with deduplicated hashes
	deduplicatedHashes := make([]*database.SignalHash, 0, len(hashMap))
	for _, hash := range hashMap {
		deduplicatedHashes = append(deduplicatedHashes, hash)
	}

	// Build batch insert query for signal_hashes table with conflict handling
	var placeholders []string
	var args []interface{}
	idx := 1

	for _, hash := range deduplicatedHashes {
		placeholders = append(placeholders, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)",
			idx, idx+1, idx+2, idx+3, idx+4, idx+5))

		// Handle NULL for empty ID
		var idValue interface{}
		if hash.ID == 0 {
			idValue = nil
		} else {
			idValue = hash.ID
		}

		args = append(args,
			hash.TimestampNs,
			hash.Hash,
			hash.RunID,
			hash.SignalType,
			idValue,
			hash.Flags,
		)
		idx += 6
	}

	query := fmt.Sprintf(`INSERT INTO %s.signal_hashes (timestamp_ns, hash, run_id, signal_type, signal_id, flags)
		VALUES %s
		ON CONFLICT (timestamp_ns, hash) 
		DO UPDATE SET 
			run_id = EXCLUDED.run_id,
			signal_type = EXCLUDED.signal_type,
			signal_id = EXCLUDED.signal_id,
			flags = EXCLUDED.flags`, schema, strings.Join(placeholders, ","))

	_, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to insert hashes: %w", err)
	}

	return nil
}

// insertLogEntriesDirect inserts log entries without template processing to reduce memory allocation
func insertLogEntriesDirect(ctx context.Context, db *sql.DB, entries []LogEntry, schemaName string) ([]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	// Initialize metrics tracking for skipped template processing
	metrics := newTemplateProcessingMetrics("logs", len(entries))
	defer metrics.recordMetrics(nil)

	columns := []string{
		"timestamp_ns", "observed_timestamp", "trace_id", "span_id", "flags", "severity_text", "severity_number",
		"service_name", "service_namespace", "service_instance_id", "event_name",
		"resource_attributes", "scope_name", "scope_version", "scope_attributes",
		"scope_dropped_attr_count", "attributes", "dropped_attr_count", "body", "metadata",
		"resource_schema_url", "scope_schema_url",
	}

	const maxBatch = 500
	var insertedIDs []string
	for start := 0; start < len(entries); start += maxBatch {
		end := start + maxBatch
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[start:end]
		var placeholders []string
		var args []interface{}
		for i, entry := range batch {
			base := i * len(columns)
			ph := make([]string, len(columns))
			for j := 0; j < len(columns); j++ {
				ph[j] = fmt.Sprintf("$%d", base+j+1)
			}
			placeholders = append(placeholders, fmt.Sprintf("(%s)", strings.Join(ph, ",")))

			// Set observed timestamp if not set
			if entry.ObservedTimestamp == nil {
				now := time.Now().UTC()
				entry.ObservedTimestamp = &now
			}
			NormalizeLogEntryJSONBFields(&entry)

			args = append(args,
				entry.TimestampNS, entry.ObservedTimestamp, entry.TraceID, entry.SpanID, entry.Flags,
				entry.SeverityText, entry.SeverityNumber, entry.ServiceName,
				entry.ServiceNamespace, entry.ServiceInstanceID, entry.EventName,
				entry.ResourceAttributes,
				entry.ScopeName, entry.ScopeVersion, entry.ScopeAttributes, entry.ScopeDroppedAttrCount,
				entry.Attributes, entry.DroppedAttrCount, entry.Body, entry.Metadata,
				entry.ResourceSchemaURL, entry.ScopeSchemaURL,
			)
		}

		tableRef, err := database.BuildSafeTableReference(schemaName, "logs")
		if err != nil {
			return nil, fmt.Errorf("invalid schema/table reference: %w", err)
		}
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s RETURNING id", tableRef, strings.Join(columns, ", "), strings.Join(placeholders, ","))
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("begin tx: %w", err)
		}
		rows, err := tx.QueryContext(ctx, insertSQL, args...)
		if err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("bulk insert logs: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = tx.Rollback()
				return nil, fmt.Errorf("scan inserted id: %w", err)
			}
			insertedIDs = append(insertedIDs, id)
		}
		if err := rows.Err(); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("rows error: %w", err)
		}
		rows.Close()
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}
	}
	return insertedIDs, nil
}

// InsertSignalEntries is a generic helper for inserting any signal type with template processing
func InsertSignalEntries(
	ctx context.Context,
	db *sql.DB,
	hashRepo *database.SignalHashRepository,
	entries []SignalDataConverter,
	schemaName string,
	signalType string,
	tableName string,
	columns []string,
	extractArgs func(SignalDataConverter) []interface{},
) ([]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	metrics := newTemplateProcessingMetrics(signalType, len(entries))
	defer metrics.recordMetrics(nil)

	var processedEntries []SignalDataConverter
	var hashes []*database.SignalHash
	var processingError error
	if shouldSkipTemplateProcessing() {
		processedEntries = entries
	} else {
		templateProcessor := NewTemplateProcessor()
		processedEntries, hashes, processingError = templateProcessor.ProcessTemplatesAndPopulateMetadata(ctx, entries, schemaName, signalType)
		if processingError != nil {
			metrics.recordMetrics(processingError)
			return nil, fmt.Errorf("template processing failed for %s: %w", signalType, processingError)
		}
	}

	maxBatch := 500
	if !shouldSkipTemplateProcessing() {
		maxBatch = 100
	}
	var insertedIDs []string

	// Start single transaction for both signal and hash insertion
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Insert signals using transaction
	for start := 0; start < len(processedEntries); start += maxBatch {
		end := start + maxBatch
		if end > len(processedEntries) {
			end = len(processedEntries)
		}
		batch := processedEntries[start:end]
		var placeholders []string
		var args []interface{}
		for i, entry := range batch {
			base := i * len(columns)
			ph := make([]string, len(columns))
			for j := 0; j < len(columns); j++ {
				ph[j] = fmt.Sprintf("$%d", base+j+1)
			}
			placeholders = append(placeholders, fmt.Sprintf("(%s)", strings.Join(ph, ",")))
			args = append(args, extractArgs(entry)...)
		}
		tableRef, err := database.BuildSafeTableReference(schemaName, tableName)
		if err != nil {
			return nil, fmt.Errorf("invalid schema/table reference: %w", err)
		}
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s RETURNING id", tableRef, strings.Join(columns, ", "), strings.Join(placeholders, ","))
		rows, err := tx.QueryContext(ctx, insertSQL, args...)
		if err != nil {
			return nil, fmt.Errorf("bulk insert %s: %w", tableName, err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return nil, fmt.Errorf("scan inserted id: %w", err)
			}
			insertedIDs = append(insertedIDs, id)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("rows error: %w", err)
		}
		rows.Close()
	}

	// Insert hashes using the SAME transaction
	if !shouldSkipHashProcessing() && len(hashes) > 0 {
		// Update hash entries with actual signal IDs
		// All hash entries for a signal should get the same signal ID
		for _, hashEntry := range hashes {
			if len(insertedIDs) > 0 {
				// Use the first signal ID for all hash entries since they all come from the same signal
				// Convert string ID to int64
				if idNum, err := strconv.ParseInt(insertedIDs[0], 10, 64); err == nil {
					hashEntry.ID = idNum
				}
			}
		}

		if err := insertHashesWithTx(ctx, tx, hashes, insertedIDs, schemaName); err != nil {
			return nil, fmt.Errorf("failed to insert signal hashes: %w", err)
		}
	}

	// Commit both operations atomically
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	metrics.recordMetrics(nil)
	return insertedIDs, nil
}

// InsertLogEntries inserts log entries with template processing
func InsertLogEntries(ctx context.Context, db *sql.DB, entries []LogEntry, schemaName string) ([]string, error) {
	// Skip conversion if template processing is disabled to reduce memory allocation
	if shouldSkipTemplateProcessing() {
		return insertLogEntriesDirect(ctx, db, entries, schemaName)
	}

	// Convert LogEntry to SignalDataConverter only when needed
	converters := make([]SignalDataConverter, len(entries))
	for i := range entries {
		converters[i] = NewLogEntryConverter(&entries[i])
	}

	columns := []string{
		"timestamp_ns", "observed_timestamp", "trace_id", "span_id", "flags", "severity_text", "severity_number",
		"service_name", "service_namespace", "service_instance_id", "event_name",
		"resource_attributes", "scope_name", "scope_version", "scope_attributes",
		"scope_dropped_attr_count", "attributes", "dropped_attr_count", "body", "metadata",
		"resource_schema_url", "scope_schema_url",
	}

	extractArgs := func(entry SignalDataConverter) []interface{} {
		logEntry := entry.(*LogEntryConverter).entry

		// Set observed timestamp if not set
		if logEntry.ObservedTimestamp == nil {
			now := time.Now().UTC()
			logEntry.ObservedTimestamp = &now
		}

		NormalizeLogEntryJSONBFields(logEntry)

		args := []interface{}{
			logEntry.TimestampNS,
			logEntry.ObservedTimestamp,
			logEntry.TraceID,
			logEntry.SpanID,
			logEntry.Flags,
			logEntry.SeverityText,
			logEntry.SeverityNumber,
			logEntry.ServiceName,
			logEntry.ServiceNamespace,
			logEntry.ServiceInstanceID,
			logEntry.EventName,
			logEntry.ResourceAttributes,
			logEntry.ScopeName,
			logEntry.ScopeVersion,
			logEntry.ScopeAttributes,
			logEntry.ScopeDroppedAttrCount,
			logEntry.Attributes,
			logEntry.DroppedAttrCount,
			logEntry.Body,
			logEntry.Metadata,
			logEntry.ResourceSchemaURL,
			logEntry.ScopeSchemaURL,
		}

		return args
	}

	// Create hash repository for hash processing
	hashRepo, err := database.NewSignalHashRepository(ctx, db, schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to create signal hash repository: %w", err)
	}

	return InsertSignalEntries(ctx, db, hashRepo, converters, schemaName, SignalTypeLogs, SignalTypeLogs, columns, extractArgs)
}

// InsertTraceEntries inserts trace entries with template processing
func InsertTraceEntries(ctx context.Context, db *sql.DB, entries []TraceEntry, schemaName string) ([]string, error) {
	// Convert TraceEntry to SignalDataConverter
	converters := make([]SignalDataConverter, len(entries))
	for i := range entries {
		converters[i] = NewTraceEntryConverter(&entries[i])
	}

	columns := []string{
		"timestamp_ns", "trace_id", "span_id", "parent_span_id", "trace_state", "flags", "name", "span_kind",
		"service_name", "service_namespace", "service_instance_id",
		"resource_attributes", "scope_name", "scope_version", "scope_attributes",
		"scope_dropped_attr_count", "attributes", "dropped_attr_count", "events", "dropped_events_count",
		"links", "dropped_links_count", "status_code", "status_message", "end_time", "metadata",
		"resource_schema_url", "scope_schema_url",
	}

	extractArgs := func(entry SignalDataConverter) []interface{} {
		traceEntry := entry.(*TraceEntryConverter).entry

		return []interface{}{
			traceEntry.TimestampNS,
			traceEntry.TraceID,
			traceEntry.SpanID,
			traceEntry.ParentSpanID,
			traceEntry.TraceState,
			traceEntry.Flags,
			traceEntry.Name,
			traceEntry.SpanKind,
			traceEntry.ServiceName,
			traceEntry.ServiceNamespace,
			traceEntry.ServiceInstanceID,
			traceEntry.ResourceAttributes,
			traceEntry.ScopeName,
			traceEntry.ScopeVersion,
			traceEntry.ScopeAttributes,
			traceEntry.ScopeDroppedAttrCount,
			traceEntry.Attributes,
			traceEntry.DroppedAttrCount,
			traceEntry.Events,
			traceEntry.DroppedEventsCount,
			traceEntry.Links,
			traceEntry.DroppedLinksCount,
			traceEntry.StatusCode,
			traceEntry.StatusMessage,
			traceEntry.EndTime,
			traceEntry.Metadata,
			traceEntry.ResourceSchemaURL,
			traceEntry.ScopeSchemaURL,
		}
	}

	// Create hash repository for hash processing
	hashRepo, err := database.NewSignalHashRepository(ctx, db, schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to create signal hash repository: %w", err)
	}

	return InsertSignalEntries(ctx, db, hashRepo, converters, schemaName, SignalTypeSpans, SignalTypeSpans, columns, extractArgs)
}

// InsertMetricEntries inserts metric entries with template processing
func InsertMetricEntries(
	ctx context.Context,
	db *sql.DB,
	entries interface{},
	schemaName string,
	metricType string,
) ([]string, error) {
	// Convert metric entries to SignalDataConverter
	var converters []SignalDataConverter

	switch typedEntries := entries.(type) {
	case []GaugeMetricEntry:
		converters = make([]SignalDataConverter, len(typedEntries))
		for i := range typedEntries {
			converters[i] = NewMetricEntryConverter(&typedEntries[i])
		}
	case []SumMetricEntry:
		converters = make([]SignalDataConverter, len(typedEntries))
		for i := range typedEntries {
			converters[i] = NewMetricEntryConverter(&typedEntries[i])
		}
	case []HistogramMetricEntry:
		converters = make([]SignalDataConverter, len(typedEntries))
		for i := range typedEntries {
			converters[i] = NewMetricEntryConverter(&typedEntries[i])
		}
	case []SummaryMetricEntry:
		converters = make([]SignalDataConverter, len(typedEntries))
		for i := range typedEntries {
			converters[i] = NewMetricEntryConverter(&typedEntries[i])
		}
	case []ExponentialHistogramMetricEntry:
		converters = make([]SignalDataConverter, len(typedEntries))
		for i := range typedEntries {
			converters[i] = NewMetricEntryConverter(&typedEntries[i])
		}
	default:
		return nil, fmt.Errorf("unsupported metric entry type: %T", entries)
	}

	// Define columns based on metric type
	var columns []string
	var extractArgs func(SignalDataConverter) []interface{}

	switch metricType {
	case SignalTypeGaugeMetrics:
		columns = []string{
			"timestamp_ns", "service_name", "service_namespace", "service_instance_id",
			"name", "description", "unit", "attributes", "value",
			"flags", "exemplars", "resource_attributes", "scope_name", "scope_version",
			"scope_attributes", "scope_dropped_attr_count", "metadata", "resource_schema_url", "scope_schema_url",
		}
		extractArgs = func(entry SignalDataConverter) []interface{} {
			metricEntry := entry.(*MetricEntryConverter).entry.(*GaugeMetricEntry)
			return []interface{}{
				metricEntry.TimestampNS,
				metricEntry.ServiceName,
				metricEntry.ServiceNamespace,
				metricEntry.ServiceInstanceID,
				metricEntry.Name,
				metricEntry.Description,
				metricEntry.Unit,
				metricEntry.Attributes,
				metricEntry.Value,
				metricEntry.Flags,
				metricEntry.Exemplars,
				metricEntry.ResourceAttributes,
				metricEntry.ScopeName,
				metricEntry.ScopeVersion,
				metricEntry.ScopeAttributes,
				metricEntry.ScopeDroppedAttrCount,
				metricEntry.Metadata,
				metricEntry.ResourceSchemaURL,
				metricEntry.ScopeSchemaURL,
			}
		}
	case SignalTypeSumMetrics:
		columns = []string{
			"timestamp_ns", "service_name", "service_namespace", "service_instance_id",
			"resource_attributes", "scope_name", "scope_version",
			"scope_attributes", "scope_dropped_attr_count", "name", "description", "unit", "attributes",
			"start_time", "value", "flags", "exemplars", "aggregation_temporality", "is_monotonic",
			"metadata", "resource_schema_url", "scope_schema_url",
		}
		extractArgs = func(entry SignalDataConverter) []interface{} {
			metricEntry := entry.(*MetricEntryConverter).entry.(*SumMetricEntry)
			return []interface{}{
				metricEntry.TimestampNS,
				metricEntry.ServiceName,
				metricEntry.ServiceNamespace,
				metricEntry.ServiceInstanceID,
				metricEntry.ResourceAttributes,
				metricEntry.ScopeName,
				metricEntry.ScopeVersion,
				metricEntry.ScopeAttributes,
				metricEntry.ScopeDroppedAttrCount,
				metricEntry.Name,
				metricEntry.Description,
				metricEntry.Unit,
				metricEntry.Attributes,
				metricEntry.StartTime,
				metricEntry.Value,
				metricEntry.Flags,
				metricEntry.Exemplars,
				metricEntry.AggregationTemporality,
				metricEntry.IsMonotonic,
				metricEntry.Metadata,
				metricEntry.ResourceSchemaURL,
				metricEntry.ScopeSchemaURL,
			}
		}
	case SignalTypeHistogramMetrics:
		columns = []string{
			"timestamp_ns", "service_name", "service_namespace", "service_instance_id",
			"name", "description", "unit", "attributes",
			"start_time", "count", "sum", "min", "max", "buckets", "flags", "exemplars", "aggregation_temporality",
			"resource_attributes", "scope_name", "scope_version",
			"scope_attributes", "scope_dropped_attr_count", "metadata", "resource_schema_url", "scope_schema_url",
		}
		extractArgs = func(entry SignalDataConverter) []interface{} {
			metricEntry := entry.(*MetricEntryConverter).entry.(*HistogramMetricEntry)
			return []interface{}{
				metricEntry.TimestampNS,
				metricEntry.ServiceName,
				metricEntry.ServiceNamespace,
				metricEntry.ServiceInstanceID,
				metricEntry.Name,
				metricEntry.Description,
				metricEntry.Unit,
				metricEntry.Attributes,
				metricEntry.StartTime,
				metricEntry.Count,
				metricEntry.Sum,
				metricEntry.Min,
				metricEntry.Max,
				metricEntry.Buckets,
				metricEntry.Flags,
				metricEntry.Exemplars,
				metricEntry.AggregationTemporality,
				metricEntry.ResourceAttributes,
				metricEntry.ScopeName,
				metricEntry.ScopeVersion,
				metricEntry.ScopeAttributes,
				metricEntry.ScopeDroppedAttrCount,
				metricEntry.Metadata,
				metricEntry.ResourceSchemaURL,
				metricEntry.ScopeSchemaURL,
			}
		}
	case SignalTypeSummaryMetrics:
		columns = []string{
			"timestamp_ns", "service_name", "service_namespace", "service_instance_id",
			"name", "description", "unit", "attributes",
			"start_time", "count", "sum", "quantiles", "flags", "exemplars",
			"resource_attributes", "scope_name", "scope_version",
			"scope_attributes", "scope_dropped_attr_count", "metadata", "resource_schema_url", "scope_schema_url",
		}
		extractArgs = func(entry SignalDataConverter) []interface{} {
			metricEntry := entry.(*MetricEntryConverter).entry.(*SummaryMetricEntry)
			return []interface{}{
				metricEntry.TimestampNS,
				metricEntry.ServiceName,
				metricEntry.ServiceNamespace,
				metricEntry.ServiceInstanceID,
				metricEntry.Name,
				metricEntry.Description,
				metricEntry.Unit,
				metricEntry.Attributes,
				metricEntry.StartTime,
				metricEntry.Count,
				metricEntry.Sum,
				metricEntry.Quantiles,
				metricEntry.Flags,
				metricEntry.Exemplars,
				metricEntry.ResourceAttributes,
				metricEntry.ScopeName,
				metricEntry.ScopeVersion,
				metricEntry.ScopeAttributes,
				metricEntry.ScopeDroppedAttrCount,
				metricEntry.Metadata,
				metricEntry.ResourceSchemaURL,
				metricEntry.ScopeSchemaURL,
			}
		}
	case SignalTypeExponentialHistogramMetrics:
		columns = []string{
			"timestamp_ns", "service_name", "service_namespace", "service_instance_id",
			"name", "description", "unit", "attributes",
			"start_time", "count", "sum", "scale", "zero_count", "zero_threshold", "positive_offset", "positive_bucket_counts",
			"negative_offset", "negative_bucket_counts", "flags", "exemplars", "min", "max", "aggregation_temporality",
			"resource_attributes", "scope_name", "scope_version",
			"scope_attributes", "scope_dropped_attr_count", "metadata", "resource_schema_url", "scope_schema_url",
		}
		extractArgs = func(entry SignalDataConverter) []interface{} {
			metricEntry := entry.(*MetricEntryConverter).entry.(*ExponentialHistogramMetricEntry)
			return []interface{}{
				metricEntry.TimestampNS,
				metricEntry.ServiceName,
				metricEntry.ServiceNamespace,
				metricEntry.ServiceInstanceID,
				metricEntry.Name,
				metricEntry.Description,
				metricEntry.Unit,
				metricEntry.Attributes,
				metricEntry.StartTime,
				metricEntry.Count,
				metricEntry.Sum,
				metricEntry.Scale,
				metricEntry.ZeroCount,
				metricEntry.ZeroThreshold,
				metricEntry.PositiveOffset,
				metricEntry.PositiveBucketCounts,
				metricEntry.NegativeOffset,
				metricEntry.NegativeBucketCounts,
				metricEntry.Flags,
				metricEntry.Exemplars,
				metricEntry.Min,
				metricEntry.Max,
				metricEntry.AggregationTemporality,
				metricEntry.ResourceAttributes,
				metricEntry.ScopeName,
				metricEntry.ScopeVersion,
				metricEntry.ScopeAttributes,
				metricEntry.ScopeDroppedAttrCount,
				metricEntry.Metadata,
				metricEntry.ResourceSchemaURL,
				metricEntry.ScopeSchemaURL,
			}
		}
	// Add other metric types as needed...
	default:
		return nil, fmt.Errorf("unsupported metric type: %s", metricType)
	}

	// Create hash repository for hash processing
	hashRepo, err := database.NewSignalHashRepository(ctx, db, schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to create signal hash repository: %w", err)
	}

	return InsertSignalEntries(ctx, db, hashRepo, converters, schemaName, metricType, metricType, columns, extractArgs)
}

// InsertProfileEntries inserts profile entries with template processing
func InsertProfileEntries(ctx context.Context, db *sql.DB, entries []ProfileEntry, schemaName string) ([]string, error) {
	for i, e := range entries {
		if err := ValidateProfileEntry(e); err != nil {
			return nil, fmt.Errorf("profile %d: %w", i, err)
		}
	}
	// Convert ProfileEntry to SignalDataConverter
	converters := make([]SignalDataConverter, len(entries))
	for i := range entries {
		converters[i] = NewProfileEntryConverter(&entries[i])
	}

	columns := []string{
		"timestamp_ns", "service_name", "service_namespace", "service_instance_id",
		"profile_type", "duration", "sample_type", "sample_unit",
		"profile_bytes", "attributes", "resource_attributes", "scope_name", "scope_version",
		"scope_attributes", "scope_dropped_attr_count", "metadata", "resource_schema_url", "scope_schema_url",
		"profile_storage_version", "profile_encoding", "profile_schema_version", "otel_profile_id", "profile_time_unix_nano", "duration_nano",
	}

	extractArgs := func(entry SignalDataConverter) []interface{} {
		profileEntry := entry.(*ProfileEntryConverter).entry

		return []interface{}{
			profileEntry.TimestampNS,
			profileEntry.ServiceName,
			profileEntry.ServiceNamespace,
			profileEntry.ServiceInstanceID,
			profileEntry.ProfileType,
			profileEntry.DurationNS,
			profileEntry.SampleType,
			profileEntry.SampleUnit,
			profileEntry.ProfileBytes,
			profileEntry.Attributes,
			profileEntry.ResourceAttributes,
			profileEntry.ScopeName,
			profileEntry.ScopeVersion,
			profileEntry.ScopeAttributes,
			profileEntry.ScopeDroppedAttrCount,
			profileEntry.Metadata,
			profileEntry.ResourceSchemaURL,
			profileEntry.ScopeSchemaURL,
			profileEntry.ProfileStorageVersion,
			func() string {
				if profileEntry.ProfileEncoding == "" {
					return ProfileEncodingLegacy
				}
				return profileEntry.ProfileEncoding
			}(),
			profileEntry.ProfileSchemaVersion, profileEntry.OtelProfileID, profileEntry.ProfileTimeUnixNano, profileEntry.DurationNano,
		}
	}

	// Create hash repository for hash processing
	hashRepo, err := database.NewSignalHashRepository(ctx, db, schemaName)
	if err != nil {
		return nil, fmt.Errorf("failed to create signal hash repository: %w", err)
	}

	return InsertSignalEntries(ctx, db, hashRepo, converters, schemaName, SignalTypeProfiles, SignalTypeProfiles, columns, extractArgs)
}
