package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// MetricRepository defines DB access methods for each metric table/type

type MetricRepository interface {
	// Direct by ID fetch (type-agnostic helper)
	GetMetricByID(ctx context.Context, id int64, schema string) (interface{}, string, error)
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
}

// countMetrics is a generic helper for counting metric entries (ignores cursor)
func countMetrics(
	ctx context.Context,
	db *sql.DB,
	schema, table string,
	q MetricQuery,
) (int64, error) {
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
	if q.MetricName != "" {
		where += fmt.Sprintf(" AND name = $%d", idx)
		args = append(args, q.MetricName)
		idx++
	}
	tableRef, err := database.BuildSafeTableReference(schema, table)
	if err != nil {
		return 0, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s %s", tableRef, where)
	row := db.QueryRowContext(ctx, query, args...)
	var cnt int64
	if err := row.Scan(&cnt); err != nil {
		return 0, fmt.Errorf("count %s: %w", table, err)
	}
	return cnt, nil
}

type metricRepository struct {
	db *sql.DB
}

// NewMetricRepository returns a MetricRepository backed by the given *sql.DB
func NewMetricRepository(db *sql.DB) MetricRepository {
	return &metricRepository{db: db}
}

// TypedMetricRepository selects the requested table because metric IDs are
// generated independently in each table and may overlap.
type TypedMetricRepository interface {
	GetMetricByIDTyped(context.Context, string, int64, string) (interface{}, string, error)
}

func (r *metricRepository) GetMetricByIDTyped(ctx context.Context, metricType string, id int64, schema string) (interface{}, string, error) {
	metricType = strings.ToLower(metricType)
	var entry interface{}
	var found bool
	var err error
	switch metricType {
	case "gauge":
		entry, found, err = r.getGaugeByID(ctx, id, schema)
	case "sum":
		entry, found, err = r.getSumByID(ctx, id, schema)
	case "histogram":
		entry, found, err = r.getHistogramByID(ctx, id, schema)
	case "summary":
		entry, found, err = r.getSummaryByID(ctx, id, schema)
	case "exponential_histogram":
		entry, found, err = r.getExponentialHistogramByID(ctx, id, schema)
	default:
		return nil, "", fmt.Errorf("unsupported metric type: %q", metricType)
	}
	if err != nil {
		return nil, "", err
	}
	if !found {
		return nil, "", sql.ErrNoRows
	}
	return entry, "metrics_" + metricType, nil
}

// GetMetricByID fetches a metric entry by ID across all metric tables.
// It returns the concrete entry (one of the *MetricEntry types), a normalized
// type string (e.g., "metrics_gauge", "metrics_sum", etc.), and an error.
func (r *metricRepository) GetMetricByID(ctx context.Context, id int64, schema string) (interface{}, string, error) {
	// Probe each metric table in a deterministic order
	// and return as soon as we find a match.

	// Gauge
	if e, ok, err := r.getGaugeByID(ctx, id, schema); err != nil {
		return nil, "", err
	} else if ok {
		return e, "metrics_gauge", nil
	}

	// Sum
	if e, ok, err := r.getSumByID(ctx, id, schema); err != nil {
		return nil, "", err
	} else if ok {
		return e, "metrics_sum", nil
	}

	// Histogram
	if e, ok, err := r.getHistogramByID(ctx, id, schema); err != nil {
		return nil, "", err
	} else if ok {
		return e, "metrics_histogram", nil
	}

	// Summary
	if e, ok, err := r.getSummaryByID(ctx, id, schema); err != nil {
		return nil, "", err
	} else if ok {
		return e, "metrics_summary", nil
	}

	// Exponential Histogram
	if e, ok, err := r.getExponentialHistogramByID(ctx, id, schema); err != nil {
		return nil, "", err
	} else if ok {
		return e, "metrics_exponential_histogram", nil
	}

	return nil, "", sql.ErrNoRows
}

// --- helpers: by-id fetch for each table ---

func (r *metricRepository) getGaugeByID(ctx context.Context, id int64, schema string) (GaugeMetricEntry, bool, error) {
	var e GaugeMetricEntry
	// Build safe table ref
	tableRef, err := database.BuildSafeTableReference(schema, SignalTypeGaugeMetrics)
	if err != nil {
		return e, false, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1 LIMIT 1`, strings.Join(gaugeColumnSet.SelectColumns, ", "), tableRef)
	row := r.db.QueryRowContext(ctx, query, id)
	e, err = scanGaugeMetric(row)
	if err == sql.ErrNoRows {
		return e, false, nil
	}
	if err != nil {
		return e, false, fmt.Errorf("get gauge by id: %w", err)
	}
	return e, true, nil
}

func (r *metricRepository) getSumByID(ctx context.Context, id int64, schema string) (SumMetricEntry, bool, error) {
	var e SumMetricEntry
	tableRef, err := database.BuildSafeTableReference(schema, SignalTypeSumMetrics)
	if err != nil {
		return e, false, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1 LIMIT 1`, strings.Join(sumColumnSet.SelectColumns, ", "), tableRef)
	row := r.db.QueryRowContext(ctx, query, id)
	e, err = scanSumMetric(row)
	if err == sql.ErrNoRows {
		return e, false, nil
	}
	if err != nil {
		return e, false, fmt.Errorf("get sum by id: %w", err)
	}
	return e, true, nil
}

func (r *metricRepository) getHistogramByID(ctx context.Context, id int64, schema string) (HistogramMetricEntry, bool, error) {
	var e HistogramMetricEntry
	tableRef, err := database.BuildSafeTableReference(schema, SignalTypeHistogramMetrics)
	if err != nil {
		return e, false, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1 LIMIT 1`, strings.Join(histogramColumnSet.SelectColumns, ", "), tableRef)
	row := r.db.QueryRowContext(ctx, query, id)
	e, err = scanHistogramMetric(row)
	if err == sql.ErrNoRows {
		return e, false, nil
	}
	if err != nil {
		return e, false, fmt.Errorf("get histogram by id: %w", err)
	}
	return e, true, nil
}

func (r *metricRepository) getSummaryByID(ctx context.Context, id int64, schema string) (SummaryMetricEntry, bool, error) {
	var e SummaryMetricEntry
	tableRef, err := database.BuildSafeTableReference(schema, SignalTypeSummaryMetrics)
	if err != nil {
		return e, false, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1 LIMIT 1`, strings.Join(summaryColumnSet.SelectColumns, ", "), tableRef)
	row := r.db.QueryRowContext(ctx, query, id)
	e, err = scanSummaryMetric(row)
	if err == sql.ErrNoRows {
		return e, false, nil
	}
	if err != nil {
		return e, false, fmt.Errorf("get summary by id: %w", err)
	}
	return e, true, nil
}

func (r *metricRepository) getExponentialHistogramByID(ctx context.Context, id int64, schema string) (ExponentialHistogramMetricEntry, bool, error) {
	var e ExponentialHistogramMetricEntry
	tableRef, err := database.BuildSafeTableReference(schema, SignalTypeExponentialHistogramMetrics)
	if err != nil {
		return e, false, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf(`SELECT %s FROM %s WHERE id = $1 LIMIT 1`, strings.Join(expHistogramColumnSet.SelectColumns, ", "), tableRef)
	row := r.db.QueryRowContext(ctx, query, id)
	e, err = scanExponentialHistogramMetric(row)
	if err == sql.ErrNoRows {
		return e, false, nil
	}
	if err != nil {
		return e, false, fmt.Errorf("get exponential histogram by id: %w", err)
	}
	return e, true, nil
}

// List and by-ID reads share scanners so their column contracts stay identical.
type metricRowScanner interface {
	Scan(...any) error
}

func scanGaugeMetric(row metricRowScanner) (GaugeMetricEntry, error) {
	var e GaugeMetricEntry
	err := row.Scan(
		&e.ID,
		&e.TimestampNS,
		&e.Timestamp,
		&e.ServiceName,
		&e.ServiceNamespace,
		&e.ServiceInstanceID,
		&e.Name,
		&e.Description,
		&e.Unit,
		&e.Attributes,
		&e.Value,
		&e.Flags,
		&e.Exemplars,
		&e.ResourceAttributes,
		&e.ScopeName,
		&e.ScopeVersion,
		&e.ScopeAttributes,
		&e.ScopeDroppedAttrCount,
		&e.Metadata,
		&e.ResourceSchemaURL,
		&e.ScopeSchemaURL,
	)
	return e, err
}

func scanSumMetric(row metricRowScanner) (SumMetricEntry, error) {
	var e SumMetricEntry
	err := row.Scan(
		&e.ID,
		&e.TimestampNS,
		&e.Timestamp,
		&e.ServiceName,
		&e.ServiceNamespace,
		&e.ServiceInstanceID,
		&e.Name,
		&e.Description,
		&e.Unit,
		&e.Attributes,
		&e.StartTime,
		&e.Value,
		&e.AggregationTemporality,
		&e.IsMonotonic,
		&e.Flags,
		&e.Exemplars,
		&e.ResourceAttributes,
		&e.ScopeName,
		&e.ScopeVersion,
		&e.ScopeAttributes,
		&e.ScopeDroppedAttrCount,
		&e.Metadata,
		&e.ResourceSchemaURL,
		&e.ScopeSchemaURL,
	)
	return e, err
}

func scanHistogramMetric(row metricRowScanner) (HistogramMetricEntry, error) {
	var e HistogramMetricEntry
	err := row.Scan(
		&e.ID,
		&e.TimestampNS,
		&e.Timestamp,
		&e.ServiceName,
		&e.ServiceNamespace,
		&e.ServiceInstanceID,
		&e.Name,
		&e.Description,
		&e.Unit,
		&e.Attributes,
		&e.StartTime,
		&e.Count,
		&e.Sum,
		&e.Min,
		&e.Max,
		&e.Buckets,
		&e.AggregationTemporality,
		&e.Flags,
		&e.Exemplars,
		&e.ResourceAttributes,
		&e.ScopeName,
		&e.ScopeVersion,
		&e.ScopeAttributes,
		&e.ScopeDroppedAttrCount,
		&e.Metadata,
		&e.ResourceSchemaURL,
		&e.ScopeSchemaURL,
	)
	return e, err
}

func scanSummaryMetric(row metricRowScanner) (SummaryMetricEntry, error) {
	var e SummaryMetricEntry
	err := row.Scan(
		&e.ID,
		&e.TimestampNS,
		&e.Timestamp,
		&e.ServiceName,
		&e.ServiceNamespace,
		&e.ServiceInstanceID,
		&e.Name,
		&e.Description,
		&e.Unit,
		&e.Attributes,
		&e.StartTime,
		&e.Count,
		&e.Sum,
		&e.Quantiles,
		&e.Flags,
		&e.Exemplars,
		&e.ResourceAttributes,
		&e.ScopeName,
		&e.ScopeVersion,
		&e.ScopeAttributes,
		&e.ScopeDroppedAttrCount,
		&e.Metadata,
		&e.ResourceSchemaURL,
		&e.ScopeSchemaURL,
	)
	return e, err
}

func scanExponentialHistogramMetric(row metricRowScanner) (ExponentialHistogramMetricEntry, error) {
	var e ExponentialHistogramMetricEntry
	err := row.Scan(
		&e.ID,
		&e.TimestampNS,
		&e.Timestamp,
		&e.ServiceName,
		&e.ServiceNamespace,
		&e.ServiceInstanceID,
		&e.Name,
		&e.Description,
		&e.Unit,
		&e.Attributes,
		&e.StartTime,
		&e.Count,
		&e.Sum,
		&e.Scale,
		&e.ZeroCount,
		&e.ZeroThreshold,
		&e.PositiveOffset,
		&e.PositiveBucketCounts,
		&e.NegativeOffset,
		&e.NegativeBucketCounts,
		&e.Min,
		&e.Max,
		&e.AggregationTemporality,
		&e.Flags,
		&e.Exemplars,
		&e.ResourceAttributes,
		&e.ScopeName,
		&e.ScopeVersion,
		&e.ScopeAttributes,
		&e.ScopeDroppedAttrCount,
		&e.Metadata,
		&e.ResourceSchemaURL,
		&e.ScopeSchemaURL,
	)
	return e, err
}

// --- DRY Helpers ---

// queryMetrics is a generic helper for querying metric entries.
func queryMetrics[T any](
	ctx context.Context,
	db *sql.DB,
	schema, table string,
	columns []string,
	q MetricQuery,
	scanRow func(metricRowScanner) (T, error),
) ([]T, string, bool, string, bool, error) {
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
	if q.MetricName != "" {
		where += fmt.Sprintf(" AND name = $%d", idx)
		args = append(args, q.MetricName)
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
	tableRef, err := database.BuildSafeTableReference(schema, table)
	if err != nil {
		return nil, "", false, "", false, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf(`SELECT %s FROM %s %s ORDER BY timestamp %s, id %s`, strings.Join(columns, ", "), tableRef, where, orderDir, orderDir)

	// Add LIMIT+1 for hasMore detection and OFFSET if specified
	if q.Offset > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
		args = append(args, limit+1, q.Offset)
	} else {
		query += fmt.Sprintf(" LIMIT $%d", idx)
		args = append(args, limit+1)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", false, "", false, fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()
	entries := make([]T, 0, limit)
	var hasMore bool
	var hasPrev bool

	for rows.Next() {
		if len(entries) == limit { // enforce limit after regex filtering
			hasMore = true
			break
		}
		e, err := scanRow(rows)
		if err != nil {
			return nil, "", false, "", false, fmt.Errorf("scan %s row: %w", table, err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, "", false, fmt.Errorf("rows error: %w", err)
	}
	var nextCursor string
	var prevCursor string

	// Generate cursors using CursorableEntry interface
	if len(entries) > 0 {
		if hasMore {
			// Generate nextCursor from last entry
			if lastEntry, ok := any(entries[len(entries)-1]).(CursorableEntry); ok {
				nextCursor = EncodeCursor(time.Unix(0, lastEntry.GetTimestampNS()), lastEntry.GetIDString())
			}
		}

		// Always generate prevCursor from first entry
		if firstEntry, ok := any(entries[0]).(CursorableEntry); ok {
			prevCursor = EncodeCursor(time.Unix(0, firstEntry.GetTimestampNS()), firstEntry.GetIDString())
		}

		// Set hasPrev if not on first page
		hasPrev = q.Cursor != ""
	}
	return entries, nextCursor, hasMore, prevCursor, hasPrev, nil
}

// --- DRY Example: Gauge Metrics (using shared utilities) ---
var gaugeColumnSet = buildMetricColumns([]string{"value"})

func (e GaugeMetricEntry) GetTimestamp() time.Time { return e.Timestamp }
func (e GaugeMetricEntry) GetID() string           { return strconv.FormatInt(e.ID, 10) }

func (r *metricRepository) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {
	return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeGaugeMetrics)
}

func (r *metricRepository) QueryMetricsGauge(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error) {
	return queryMetrics(ctx, r.db, q.Schema, SignalTypeGaugeMetrics, gaugeColumnSet.SelectColumns, q, scanGaugeMetric)
}

func (r *metricRepository) CountMetricsGauge(ctx context.Context, q MetricQuery) (int64, error) {
	return countMetrics(ctx, r.db, q.Schema, SignalTypeGaugeMetrics, q)
}

// --- DRY: Sum Metrics (using shared utilities) ---
var sumColumnSet = buildMetricColumns([]string{"start_time", "value", "aggregation_temporality", "is_monotonic"})

func (e SumMetricEntry) GetTimestamp() time.Time { return e.Timestamp }
func (e SumMetricEntry) GetID() string           { return strconv.FormatInt(e.ID, 10) }

func (r *metricRepository) InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error) {
	return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeSumMetrics)
}

func (r *metricRepository) QueryMetricsSum(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error) {
	return queryMetrics(ctx, r.db, q.Schema, SignalTypeSumMetrics, sumColumnSet.SelectColumns, q, scanSumMetric)
}

func (r *metricRepository) CountMetricsSum(ctx context.Context, q MetricQuery) (int64, error) {
	return countMetrics(ctx, r.db, q.Schema, SignalTypeSumMetrics, q)
}

// --- DRY: Histogram Metrics (using shared utilities) ---
var histogramColumnSet = buildMetricColumns([]string{"start_time", "count", "sum", "min", "max", "buckets", "aggregation_temporality"})

func (e HistogramMetricEntry) GetTimestamp() time.Time { return e.Timestamp }
func (e HistogramMetricEntry) GetID() string           { return strconv.FormatInt(e.ID, 10) }

func (r *metricRepository) InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error) {
	return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeHistogramMetrics)
}

func (r *metricRepository) QueryMetricsHistogram(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	return queryMetrics(ctx, r.db, q.Schema, SignalTypeHistogramMetrics, histogramColumnSet.SelectColumns, q, scanHistogramMetric)
}

func (r *metricRepository) CountMetricsHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return countMetrics(ctx, r.db, q.Schema, SignalTypeHistogramMetrics, q)
}

// --- DRY: Summary Metrics (using shared utilities) ---
var summaryColumnSet = buildMetricColumns([]string{"start_time", "count", "sum", "quantiles"})

func (e SummaryMetricEntry) GetTimestamp() time.Time { return e.Timestamp }
func (e SummaryMetricEntry) GetID() string           { return strconv.FormatInt(e.ID, 10) }

func (r *metricRepository) InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error) {
	return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeSummaryMetrics)
}

func (r *metricRepository) QueryMetricsSummary(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error) {
	return queryMetrics(ctx, r.db, q.Schema, SignalTypeSummaryMetrics, summaryColumnSet.SelectColumns, q, scanSummaryMetric)
}

func (r *metricRepository) CountMetricsSummary(ctx context.Context, q MetricQuery) (int64, error) {
	return countMetrics(ctx, r.db, q.Schema, SignalTypeSummaryMetrics, q)
}

// --- DRY: Exponential Histogram Metrics (using shared utilities) ---
var expHistogramColumnSet = buildMetricColumns([]string{"start_time", "count", "sum", "scale", "zero_count", "zero_threshold", "positive_offset", "positive_bucket_counts", "negative_offset", "negative_bucket_counts", "min", "max", "aggregation_temporality"})

func (e ExponentialHistogramMetricEntry) GetTimestamp() time.Time { return e.Timestamp }
func (e ExponentialHistogramMetricEntry) GetID() string           { return strconv.FormatInt(e.ID, 10) }

func (r *metricRepository) InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error) {
	return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeExponentialHistogramMetrics)
}

func (r *metricRepository) QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	return queryMetrics(ctx, r.db, q.Schema, SignalTypeExponentialHistogramMetrics, expHistogramColumnSet.SelectColumns, q, scanExponentialHistogramMetric)
}

func (r *metricRepository) CountMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return countMetrics(ctx, r.db, q.Schema, SignalTypeExponentialHistogramMetrics, q)
}
