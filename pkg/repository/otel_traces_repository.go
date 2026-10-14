package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type TraceRepository interface {
	QuerySpans(ctx context.Context, q TraceQuery) ([]TraceEntry, string, bool, string, bool, error)
	InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error)
	GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntry, error)
	CountSpans(ctx context.Context, q TraceQuery) (int64, error)

	// Trace correlation methods
	FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error)
	FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error)
	FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error)
	FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error)
	FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error)
	FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error)
	FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error)
}

type traceRepository struct {
	db *sql.DB
}

func NewTraceRepository(db *sql.DB) TraceRepository {
	return &traceRepository{db: db}
}

// =========================
// Trace Correlation Methods
// =========================

// FetchSpansByTraceID fetches spans associated with a trace ID
// Optimized to reduce JSON unmarshaling overhead by using efficient batch processing
func (r *traceRepository) FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error) {
	query := `SELECT id, trace_id, span_id, parent_span_id, trace_state, name, span_kind,
				 service_name, resource_attributes, scope_name, scope_version, scope_attributes,
				 attributes, events, links, status_code, status_message, start_time, end_time,
				 duration_ns, metadata
			 FROM otel_traces WHERE trace_id = $1 ORDER BY start_time`

	rows, err := r.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spans for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	// Pre-allocate with reasonable capacity to reduce slice growth allocations
	spans := make([]SpanData, 0, 32)

	for rows.Next() {
		var span SpanData
		var resourceAttrs, scopeAttrs, attrs, events, links, metadata []byte

		err := rows.Scan(
			&span.ID, &span.TraceID, &span.SpanID, &span.ParentSpanID, &span.TraceState,
			&span.Name, &span.SpanKind, &span.ServiceName, &resourceAttrs, &span.ScopeName,
			&span.ScopeVersion, &scopeAttrs, &attrs, &events, &links, &span.StatusCode,
			&span.StatusMessage, &span.StartTime, &span.EndTime, &span.Duration, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan span row: %w", err)
		}

		// Parse JSONB fields using optimized helper
		span.ResourceAttributes = unmarshalJSONBMap(resourceAttrs)
		span.ScopeAttributes = unmarshalJSONBMap(scopeAttrs)
		span.Attributes = unmarshalJSONBMap(attrs)
		span.Events = unmarshalJSONBArray(events)
		span.Links = unmarshalJSONBArray(links)
		span.Metadata = unmarshalJSONBMap(metadata)

		spans = append(spans, span)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating span rows: %w", err)
	}

	return spans, nil
}

// unmarshalJSONBMap efficiently unmarshals JSONB data into a map
// Returns empty map if data is nil, empty, or invalid JSON
func unmarshalJSONBMap(data []byte) map[string]interface{} {
	if len(data) == 0 || string(data) == "null" {
		return make(map[string]interface{})
	}
	result := make(map[string]interface{})
	if err := json.Unmarshal(data, &result); err != nil {
		return make(map[string]interface{})
	}
	return result
}

// unmarshalJSONBArray efficiently unmarshals JSONB data into an array of maps
// Returns empty slice if data is nil, empty, or invalid JSON
func unmarshalJSONBArray(data []byte) []map[string]interface{} {
	if len(data) == 0 || string(data) == "null" || string(data) == "[]" {
		return make([]map[string]interface{}, 0)
	}
	result := make([]map[string]interface{}, 0)
	if err := json.Unmarshal(data, &result); err != nil {
		return make([]map[string]interface{}, 0)
	}
	return result
}

// FetchLogsByTraceID fetches logs associated with a trace ID
func (r *traceRepository) FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error) {
	query := `SELECT id, trace_id, span_id, timestamp, observed_timestamp, severity_text,
				 severity_number, service_name, resource_attributes, scope_name, scope_version,
				 scope_attributes, attributes, body, metadata
			 FROM otel_logs WHERE trace_id = $1 ORDER BY timestamp`

	rows, err := r.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	logs := make([]LogData, 0, 32)
	for rows.Next() {
		var log LogData
		var resourceAttrs, scopeAttrs, attrs, body, metadata []byte

		err := rows.Scan(
			&log.ID, &log.TraceID, &log.SpanID, &log.Timestamp, &log.ObservedTimestamp,
			&log.SeverityText, &log.SeverityNumber, &log.ServiceName, &resourceAttrs,
			&log.ScopeName, &log.ScopeVersion, &scopeAttrs, &attrs, &body, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}

		// Parse JSONB fields using optimized helpers
		log.ResourceAttributes = unmarshalJSONBMap(resourceAttrs)
		log.ScopeAttributes = unmarshalJSONBMap(scopeAttrs)
		log.Attributes = unmarshalJSONBMap(attrs)
		log.Body = unmarshalJSONBValue(body)
		log.Metadata = unmarshalJSONBMap(metadata)

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log rows: %w", err)
	}

	return logs, nil
}

// unmarshalJSONBValue unmarshals JSONB data into an interface{} value
func unmarshalJSONBValue(data []byte) interface{} {
	if len(data) == 0 || string(data) == "null" {
		return nil
	}
	var result interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

// FetchLogsBySpanID fetches logs associated with a span ID
func (r *traceRepository) FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error) {
	query := `SELECT id, trace_id, span_id, timestamp, observed_timestamp, severity_text,
				 severity_number, service_name, resource_attributes, scope_name, scope_version,
				 scope_attributes, attributes, body, metadata
			 FROM otel_logs WHERE span_id = $1 ORDER BY timestamp`

	rows, err := r.db.QueryContext(ctx, query, spanID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs for span %s: %w", spanID, err)
	}
	defer rows.Close()

	logs := make([]LogData, 0, 16)
	for rows.Next() {
		var log LogData
		var resourceAttrs, scopeAttrs, attrs, body, metadata []byte

		err := rows.Scan(
			&log.ID, &log.TraceID, &log.SpanID, &log.Timestamp, &log.ObservedTimestamp,
			&log.SeverityText, &log.SeverityNumber, &log.ServiceName, &resourceAttrs,
			&log.ScopeName, &log.ScopeVersion, &scopeAttrs, &attrs, &body, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}

		// Parse JSONB fields using optimized helpers
		log.ResourceAttributes = unmarshalJSONBMap(resourceAttrs)
		log.ScopeAttributes = unmarshalJSONBMap(scopeAttrs)
		log.Attributes = unmarshalJSONBMap(attrs)
		log.Body = unmarshalJSONBValue(body)
		log.Metadata = unmarshalJSONBMap(metadata)

		logs = append(logs, log)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log rows: %w", err)
	}

	return logs, nil
}

// FetchMetricsByTraceID fetches metrics associated with a trace ID
func (r *traceRepository) FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error) {
	// Note: Metrics typically don't have direct trace_id, check exemplars or attributes
	query := `SELECT id, trace_id, span_id, timestamp, name, description, unit, service_name,
				 resource_attributes, scope_name, scope_version, scope_attributes, attributes,
				 value, metadata, metric_type
			 FROM otel_metrics WHERE trace_id = $1 OR attributes->>'trace_id' = $1 ORDER BY timestamp`

	rows, err := r.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	metrics := make([]MetricData, 0, 16)
	for rows.Next() {
		var metric MetricData
		var resourceAttrs, scopeAttrs, attrs, value, metadata []byte

		err := rows.Scan(
			&metric.ID, &metric.TraceID, &metric.SpanID, &metric.Timestamp, &metric.Name,
			&metric.Description, &metric.Unit, &metric.ServiceName, &resourceAttrs,
			&metric.ScopeName, &metric.ScopeVersion, &scopeAttrs, &attrs, &value, &metadata,
			&metric.MetricType,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric row: %w", err)
		}

		// Parse JSONB fields using optimized helpers
		metric.ResourceAttributes = unmarshalJSONBMap(resourceAttrs)
		metric.ScopeAttributes = unmarshalJSONBMap(scopeAttrs)
		metric.Attributes = unmarshalJSONBMap(attrs)
		metric.Value = unmarshalJSONBValue(value)
		metric.Metadata = unmarshalJSONBMap(metadata)

		metrics = append(metrics, metric)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating metric rows: %w", err)
	}

	return metrics, nil
}

// FetchProfilesByTraceID fetches profiles associated with a trace ID
func (r *traceRepository) FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error) {
	query := `SELECT id, trace_id, span_id, timestamp, profile_type, duration, sample_type,
				 sample_unit, profile_bytes, service_name, resource_attributes, scope_name,
				 scope_version, scope_attributes, attributes, metadata
			 FROM otel_profiles WHERE trace_id = $1 ORDER BY timestamp`

	rows, err := r.db.QueryContext(ctx, query, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profiles for trace %s: %w", traceID, err)
	}
	defer rows.Close()

	profiles := make([]ProfileData, 0, 8)
	for rows.Next() {
		var profile ProfileData
		var resourceAttrs, scopeAttrs, attrs, metadata []byte

		err := rows.Scan(
			&profile.ID, &profile.TraceID, &profile.SpanID, &profile.Timestamp,
			&profile.ProfileType, &profile.Duration, &profile.SampleType, &profile.SampleUnit,
			&profile.ProfileBytes, &profile.ServiceName, &resourceAttrs, &profile.ScopeName,
			&profile.ScopeVersion, &scopeAttrs, &attrs, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan profile row: %w", err)
		}

		// Parse JSONB fields using optimized helpers
		profile.ResourceAttributes = unmarshalJSONBMap(resourceAttrs)
		profile.ScopeAttributes = unmarshalJSONBMap(scopeAttrs)
		profile.Attributes = unmarshalJSONBMap(attrs)
		profile.Metadata = unmarshalJSONBMap(metadata)

		profiles = append(profiles, profile)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating profile rows: %w", err)
	}

	return profiles, nil
}

// FetchProfilesBySpanID fetches profiles associated with a span ID
func (r *traceRepository) FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error) {
	query := `SELECT id, trace_id, span_id, timestamp, profile_type, duration, sample_type,
				 sample_unit, profile_bytes, service_name, resource_attributes, scope_name,
				 scope_version, scope_attributes, attributes, metadata
			 FROM otel_profiles WHERE span_id = $1 ORDER BY timestamp`

	rows, err := r.db.QueryContext(ctx, query, spanID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profiles for span %s: %w", spanID, err)
	}
	defer rows.Close()

	profiles := make([]ProfileData, 0, 8)
	for rows.Next() {
		var profile ProfileData
		var resourceAttrs, scopeAttrs, attrs, metadata []byte

		err := rows.Scan(
			&profile.ID, &profile.TraceID, &profile.SpanID, &profile.Timestamp,
			&profile.ProfileType, &profile.Duration, &profile.SampleType, &profile.SampleUnit,
			&profile.ProfileBytes, &profile.ServiceName, &resourceAttrs, &profile.ScopeName,
			&profile.ScopeVersion, &scopeAttrs, &attrs, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan profile row: %w", err)
		}

		// Parse JSONB fields using optimized helpers
		profile.ResourceAttributes = unmarshalJSONBMap(resourceAttrs)
		profile.ScopeAttributes = unmarshalJSONBMap(scopeAttrs)
		profile.Attributes = unmarshalJSONBMap(attrs)
		profile.Metadata = unmarshalJSONBMap(metadata)

		profiles = append(profiles, profile)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating profile rows: %w", err)
	}

	return profiles, nil
}

// FetchExemplarsBySpanIDFromType fetches exemplars by span ID and signal type
func (r *traceRepository) FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error) {
	query := `SELECT id, trace_id, span_id, timestamp, metric_name, metric_type, value,
				 filtered_attributes, service_name, resource_attributes, scope_name,
				 scope_version, scope_attributes, metadata
			 FROM otel_exemplars WHERE span_id = $1 AND metric_type = $2 ORDER BY timestamp`

	rows, err := r.db.QueryContext(ctx, query, spanID, signalType)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch exemplars for span %s: %w", spanID, err)
	}
	defer rows.Close()

	exemplars := make([]ExemplarData, 0, 8)
	for rows.Next() {
		var exemplar ExemplarData
		var filteredAttrs, resourceAttrs, scopeAttrs, metadata []byte

		err := rows.Scan(
			&exemplar.ID, &exemplar.TraceID, &exemplar.SpanID, &exemplar.Timestamp,
			&exemplar.MetricName, &exemplar.MetricType, &exemplar.Value, &filteredAttrs,
			&exemplar.ServiceName, &resourceAttrs, &exemplar.ScopeName, &exemplar.ScopeVersion,
			&scopeAttrs, &metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan exemplar row: %w", err)
		}

		// Parse JSONB fields using optimized helpers
		exemplar.FilteredAttributes = unmarshalJSONBMap(filteredAttrs)
		exemplar.ResourceAttributes = unmarshalJSONBMap(resourceAttrs)
		exemplar.ScopeAttributes = unmarshalJSONBMap(scopeAttrs)
		exemplar.Metadata = unmarshalJSONBMap(metadata)

		exemplars = append(exemplars, exemplar)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating exemplar rows: %w", err)
	}

	return exemplars, nil
}

func (r *traceRepository) QuerySpans(ctx context.Context, q TraceQuery) ([]TraceEntry, string, bool, string, bool, error) {
	args := []interface{}{}
	where := "WHERE 1=1"
	idx := 1
	// Time range filtering (only apply if no cursor is present)
	if q.Cursor == "" {
		if q.Start != nil {
			where += fmt.Sprintf(" AND timestamp >= $%d", idx)
			args = append(args, *q.Start)
			idx++
		}
		if q.End != nil {
			where += fmt.Sprintf(" AND timestamp <= $%d", idx)
			args = append(args, *q.End)
			idx++
		}
	}

	// Service filtering using common builder
	serviceWhere, serviceArgs, newIdx := BuildServiceFiltersSQL(q.CommonServiceFilters, idx)
	where += serviceWhere
	args = append(args, serviceArgs...)
	idx = newIdx

	if q.TraceID != "" {
		where += fmt.Sprintf(" AND trace_id = $%d", idx)
		args = append(args, q.TraceID)
		idx++
	}
	if q.Cursor != "" {
		cursorNS, cursorID, err := ParseCursor(q.Cursor)
		if err != nil {
			return nil, "", false, "", false, fmt.Errorf("invalid cursor: %w", err)
		}
		// Convert string cursorID back to int64 for database comparison
		cursorIDInt, err := strconv.ParseInt(cursorID, 10, 64)
		if err != nil {
			return nil, "", false, "", false, fmt.Errorf("invalid cursor ID: %w", err)
		}
		cmp := "<"
		if strings.ToLower(q.Direction) == "asc" {
			cmp = ">"
		}
		where += fmt.Sprintf(" AND (timestamp, id) %s ($%d, $%d)", cmp, idx, idx+1)
		args = append(args, time.Unix(0, cursorNS), cursorIDInt)
		idx += 2
	}
	limit := 100
	if q.Limit > 0 {
		limit = q.Limit
	}
	orderDir := "DESC"
	if strings.ToLower(q.Direction) == "asc" {
		orderDir = "ASC"
	}
	// Build the base query
	query := fmt.Sprintf(`SELECT id, timestamp_ns, timestamp, end_time, duration AS duration_ns, trace_id, span_id, parent_span_id, trace_state, flags, name, span_kind, service_name, service_namespace, service_instance_id, resource_attributes, scope_name, scope_version, scope_attributes, scope_dropped_attr_count, attributes, dropped_attr_count, events, dropped_events_count, links, dropped_links_count, status_code, status_message, metadata, resource_schema_url, scope_schema_url FROM %s.spans %s ORDER BY (CASE WHEN events IS NOT NULL AND events != '[]' AND events != '{}' THEN 0 ELSE 1 END), timestamp %s, id %s`, q.Schema, where, orderDir, orderDir)

	// Add LIMIT+1 for hasMore detection and OFFSET if specified
	if q.Offset > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
		args = append(args, limit+1, q.Offset)
	} else {
		query += fmt.Sprintf(" LIMIT $%d", idx)
		args = append(args, limit+1)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", false, "", false, fmt.Errorf("query spans: %w", err)
	}
	defer rows.Close()
	entries := make([]TraceEntry, 0, limit)
	var hasMore bool
	var hasPrev bool

	for rows.Next() {
		var e TraceEntry
		if len(entries) == limit { // enforce limit after regex filtering
			hasMore = true
			break
		}
		err := rows.Scan(
			&e.ID,
			&e.TimestampNS,
			&e.Timestamp,
			&e.EndTime,
			&e.DurationNS,
			&e.TraceID,
			&e.SpanID,
			&e.ParentSpanID,
			&e.TraceState,
			&e.Flags,
			&e.Name,
			&e.SpanKind,
			&e.ServiceName,
			&e.ServiceNamespace,
			&e.ServiceInstanceID,
			&e.ResourceAttributes,
			&e.ScopeName,
			&e.ScopeVersion,
			&e.ScopeAttributes,
			&e.ScopeDroppedAttrCount,
			&e.Attributes,
			&e.DroppedAttrCount,
			&e.Events,
			&e.DroppedEventsCount,
			&e.Links,
			&e.DroppedLinksCount,
			&e.StatusCode,
			&e.StatusMessage,
			&e.Metadata,
			&e.ResourceSchemaURL,
			&e.ScopeSchemaURL,
		)
		if err != nil {
			return nil, "", false, "", false, fmt.Errorf("scan trace row: %w", err)
		}
		e.StartTime = e.Timestamp
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, "", false, fmt.Errorf("rows error: %w", err)
	}
	var nextCursor string
	var prevCursor string

	// Use shared cursor generation utility
	nextCursor, prevCursor, hasPrev = generateCursors(entries, hasMore, q.Cursor != "")
	return entries, nextCursor, hasMore, prevCursor, hasPrev, nil
}

func (r *traceRepository) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {
	return InsertTraceEntries(ctx, r.db, entries, schema)
}

// GetSpanByID returns a single span by its ID
func (r *traceRepository) GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntry, error) {
	// Reuse the same column order as in QuerySpans
	query := fmt.Sprintf(`SELECT id, timestamp_ns, timestamp, end_time, duration AS duration_ns, trace_id, span_id, parent_span_id, trace_state, flags, name, span_kind, service_name, service_namespace, service_instance_id, resource_attributes, scope_name, scope_version, scope_attributes, scope_dropped_attr_count, attributes, dropped_attr_count, events, dropped_events_count, links, dropped_links_count, status_code, status_message, metadata, resource_schema_url, scope_schema_url FROM %s.spans WHERE id = $1 LIMIT 1`, schema)
	var e TraceEntry
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&e.ID,
		&e.TimestampNS,
		&e.Timestamp,
		&e.EndTime,
		&e.DurationNS,
		&e.TraceID,
		&e.SpanID,
		&e.ParentSpanID,
		&e.TraceState,
		&e.Flags,
		&e.Name,
		&e.SpanKind,
		&e.ServiceName,
		&e.ServiceNamespace,
		&e.ServiceInstanceID,
		&e.ResourceAttributes,
		&e.ScopeName,
		&e.ScopeVersion,
		&e.ScopeAttributes,
		&e.ScopeDroppedAttrCount,
		&e.Attributes,
		&e.DroppedAttrCount,
		&e.Events,
		&e.DroppedEventsCount,
		&e.Links,
		&e.DroppedLinksCount,
		&e.StatusCode,
		&e.StatusMessage,
		&e.Metadata,
		&e.ResourceSchemaURL,
		&e.ScopeSchemaURL,
	)
	if err != nil {
		return TraceEntry{}, err
	}
	e.StartTime = e.Timestamp
	return e, nil
}

// CountSpans returns the total number of spans matching the filters in TraceQuery.
// Note: cursor/pagination parameters are ignored for counts.
func (r *traceRepository) CountSpans(ctx context.Context, q TraceQuery) (int64, error) {
	args := []interface{}{}
	where := "WHERE 1=1"
	idx := 1
	if q.Start != nil {
		where += fmt.Sprintf(" AND timestamp >= $%d", idx)
		args = append(args, *q.Start)
		idx++
	}
	if q.End != nil {
		where += fmt.Sprintf(" AND timestamp <= $%d", idx)
		args = append(args, *q.End)
		idx++
	}
	// Service filtering using common builder
	serviceWhere, serviceArgs, newIdx := BuildServiceFiltersSQL(q.CommonServiceFilters, idx)
	where += serviceWhere
	args = append(args, serviceArgs...)
	idx = newIdx
	if q.TraceID != "" {
		where += fmt.Sprintf(" AND trace_id = $%d", idx)
		args = append(args, q.TraceID)
		idx++
	}
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s.spans %s`, q.Schema, where)
	var count int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count spans: %w", err)
	}
	return count, nil
}
