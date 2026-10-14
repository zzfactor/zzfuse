package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// API response models
type MetricEntryResponse struct {
	ID                     int64       `json:"id"`
	TimestampNS            int64       `json:"timestamp_ns"`
	Timestamp              string      `json:"timestamp"`
	ServiceName            string      `json:"service_name"`
	ServiceNamespace       *string     `json:"service_namespace,omitempty"`
	ServiceInstanceID      *string     `json:"service_instance_id,omitempty"`
	Name                   string      `json:"name"`
	Description            *string     `json:"description,omitempty"`
	Unit                   *string     `json:"unit,omitempty"`
	Attributes             interface{} `json:"attributes,omitempty"`
	StartTime              *string     `json:"start_time,omitempty"`
	Value                  *float64    `json:"value,omitempty"`
	Count                  *int64      `json:"count,omitempty"`
	Sum                    *float64    `json:"sum,omitempty"`
	Buckets                interface{} `json:"buckets,omitempty"`
	Quantiles              interface{} `json:"quantiles,omitempty"`
	Flags                  *int32      `json:"flags,omitempty"`
	Exemplars              interface{} `json:"exemplars,omitempty"`
	ResourceAttributes     interface{} `json:"resource_attributes,omitempty"`
	ScopeName              *string     `json:"scope_name,omitempty"`
	ScopeVersion           *string     `json:"scope_version,omitempty"`
	ScopeAttributes        interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount  *int32      `json:"scope_dropped_attr_count,omitempty"`
	Metadata               interface{} `json:"metadata,omitempty"`
	ResourceSchemaURL      *string     `json:"resource_schema_url,omitempty"`
	ScopeSchemaURL         *string     `json:"scope_schema_url,omitempty"`
	AggregationTemporality *int32      `json:"aggregation_temporality,omitempty"`
	IsMonotonic            *bool       `json:"is_monotonic,omitempty"`
	Min                    *float64    `json:"min,omitempty"`
	Max                    *float64    `json:"max,omitempty"`
	Scale                  *int32      `json:"scale,omitempty"`
	ZeroCount              *int64      `json:"zero_count,omitempty"`
	ZeroThreshold          *float64    `json:"zero_threshold,omitempty"`
	PositiveOffset         *int32      `json:"positive_offset,omitempty"`
	PositiveBucketCounts   interface{} `json:"positive_bucket_counts,omitempty"`
	NegativeOffset         *int32      `json:"negative_offset,omitempty"`
	NegativeBucketCounts   interface{} `json:"negative_bucket_counts,omitempty"`
}

type MetricQueryResult struct {
	Entries    []MetricEntryResponse `json:"entries"`
	NextCursor string                `json:"next_cursor"`
	HasMore    bool                  `json:"has_more"`
	PrevCursor string                `json:"prev_cursor,omitempty"`
	HasPrev    bool                  `json:"has_prev,omitempty"`
}

// Service interfaces
type MetricQueryService interface {
	GetMetricByID(ctx context.Context, id int64, schema string) (MetricEntryResponse, error)
	GetMetricByIDTyped(ctx context.Context, metricType string, id int64, schema string) (MetricEntryResponse, error)
	QueryMetricsGauge(ctx context.Context, q MetricQuery) (MetricQueryResult, error)
	QueryMetricsSum(ctx context.Context, q MetricQuery) (MetricQueryResult, error)
	QueryMetricsHistogram(ctx context.Context, q MetricQuery) (MetricQueryResult, error)
	QueryMetricsSummary(ctx context.Context, q MetricQuery) (MetricQueryResult, error)
	QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (MetricQueryResult, error)
	StreamMetrics(ctx context.Context, q MetricQuery) (<-chan MetricEntryResponse, error)
	GenerateCursor(ctx context.Context, offset int, filters MetricQuery) (MetricQueryResult, error)
}

type MetricWriterService interface {
	InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error)
	InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error)
	InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error)
	InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error)
	InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error)
}

// Service implementations
// metricQueryService now uses table-specific structs from the repository

type metricQueryService struct {
	repo MetricRepository
}

func NewMetricQueryService(repo MetricRepository) MetricQueryService {
	return &metricQueryService{repo: repo}
}

func (s *metricQueryService) QueryMetricsGauge(ctx context.Context, q MetricQuery) (MetricQueryResult, error) {
	// Validate query including schema validation
	if err := ValidateMetricQuery(q); err != nil {
		return MetricQueryResult{}, err
	}

	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QueryMetricsGauge(ctx, q)
	if err != nil {
		return MetricQueryResult{}, err
	}
	needReverse := strings.ToLower(q.DirectionResults) != strings.ToLower(q.Direction)
	respEntries := make([]MetricEntryResponse, 0, len(entries))
	if needReverse {
		for i := len(entries) - 1; i >= 0; i-- {
			respEntries = append(respEntries, toGaugeMetricEntryResponse(entries[i]))
		}
	} else {
		for _, e := range entries {
			respEntries = append(respEntries, toGaugeMetricEntryResponse(e))
		}
	}
	return MetricQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore, PrevCursor: prevCursor, HasPrev: hasPrev}, nil
}

func (s *metricQueryService) CountMetricsGauge(ctx context.Context, q MetricQuery) (int64, error) {
	return s.repo.CountMetricsGauge(ctx, q)
}

func (s *metricQueryService) QueryMetricsSum(ctx context.Context, q MetricQuery) (MetricQueryResult, error) {
	// Validate query including schema validation
	if err := ValidateMetricQuery(q); err != nil {
		return MetricQueryResult{}, err
	}

	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QueryMetricsSum(ctx, q)
	if err != nil {
		return MetricQueryResult{}, err
	}
	needReverse := strings.ToLower(q.DirectionResults) != strings.ToLower(q.Direction)
	respEntries := make([]MetricEntryResponse, 0, len(entries))
	if needReverse {
		for i := len(entries) - 1; i >= 0; i-- {
			respEntries = append(respEntries, toSumMetricEntryResponse(entries[i]))
		}
	} else {
		for _, e := range entries {
			respEntries = append(respEntries, toSumMetricEntryResponse(e))
		}
	}
	return MetricQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore, PrevCursor: prevCursor, HasPrev: hasPrev}, nil
}

func (s *metricQueryService) CountMetricsSum(ctx context.Context, q MetricQuery) (int64, error) {
	return s.repo.CountMetricsSum(ctx, q)
}

func (s *metricQueryService) QueryMetricsHistogram(ctx context.Context, q MetricQuery) (MetricQueryResult, error) {
	// Validate query including schema validation
	if err := ValidateMetricQuery(q); err != nil {
		return MetricQueryResult{}, err
	}

	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QueryMetricsHistogram(ctx, q)
	if err != nil {
		return MetricQueryResult{}, err
	}
	needReverse := strings.ToLower(q.DirectionResults) != strings.ToLower(q.Direction)
	respEntries := make([]MetricEntryResponse, 0, len(entries))
	if needReverse {
		for i := len(entries) - 1; i >= 0; i-- {
			respEntries = append(respEntries, toHistogramMetricEntryResponse(entries[i]))
		}
	} else {
		for _, e := range entries {
			respEntries = append(respEntries, toHistogramMetricEntryResponse(e))
		}
	}
	return MetricQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore, PrevCursor: prevCursor, HasPrev: hasPrev}, nil
}

func (s *metricQueryService) CountMetricsHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return s.repo.CountMetricsHistogram(ctx, q)
}

func (s *metricQueryService) QueryMetricsSummary(ctx context.Context, q MetricQuery) (MetricQueryResult, error) {
	// Validate query including schema validation
	if err := ValidateMetricQuery(q); err != nil {
		return MetricQueryResult{}, err
	}

	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QueryMetricsSummary(ctx, q)
	if err != nil {
		return MetricQueryResult{}, err
	}
	needReverse := strings.ToLower(q.DirectionResults) != strings.ToLower(q.Direction)
	respEntries := make([]MetricEntryResponse, 0, len(entries))
	if needReverse {
		for i := len(entries) - 1; i >= 0; i-- {
			respEntries = append(respEntries, toSummaryMetricEntryResponse(entries[i]))
		}
	} else {
		for _, e := range entries {
			respEntries = append(respEntries, toSummaryMetricEntryResponse(e))
		}
	}
	return MetricQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore, PrevCursor: prevCursor, HasPrev: hasPrev}, nil
}

func (s *metricQueryService) CountMetricsSummary(ctx context.Context, q MetricQuery) (int64, error) {
	return s.repo.CountMetricsSummary(ctx, q)
}

func (s *metricQueryService) QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (MetricQueryResult, error) {
	// Validate query including schema validation
	if err := ValidateMetricQuery(q); err != nil {
		return MetricQueryResult{}, err
	}

	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QueryMetricsExponentialHistogram(ctx, q)
	if err != nil {
		return MetricQueryResult{}, err
	}
	needReverse := strings.ToLower(q.DirectionResults) != strings.ToLower(q.Direction)
	respEntries := make([]MetricEntryResponse, 0, len(entries))
	if needReverse {
		for i := len(entries) - 1; i >= 0; i-- {
			respEntries = append(respEntries, toExponentialHistogramMetricEntryResponse(entries[i]))
		}
	} else {
		for _, e := range entries {
			respEntries = append(respEntries, toExponentialHistogramMetricEntryResponse(e))
		}
	}
	return MetricQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore, PrevCursor: prevCursor, HasPrev: hasPrev}, nil
}

func (s *metricQueryService) CountMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return s.repo.CountMetricsExponentialHistogram(ctx, q)
}

func (s *metricQueryService) StreamMetrics(ctx context.Context, q MetricQuery) (<-chan MetricEntryResponse, error) {
	return nil, nil // Not implemented
}

func (s *metricQueryService) GetMetricByID(ctx context.Context, id int64, schema string) (MetricEntryResponse, error) {
	entry, typ, err := s.repo.GetMetricByID(ctx, id, schema)
	if err != nil {
		return MetricEntryResponse{}, err
	}
	switch e := entry.(type) {
	case GaugeMetricEntry:
		return toGaugeMetricEntryResponse(e), nil
	case SumMetricEntry:
		return toSumMetricEntryResponse(e), nil
	case HistogramMetricEntry:
		return toHistogramMetricEntryResponse(e), nil
	case SummaryMetricEntry:
		return toSummaryMetricEntryResponse(e), nil
	case ExponentialHistogramMetricEntry:
		return toExponentialHistogramMetricEntryResponse(e), nil
	default:
		return MetricEntryResponse{}, fmt.Errorf("unknown metric type: %s", typ)
	}
}

// Writer service uses table-specific structs for inserts

type metricWriterService struct {
	repo         MetricRepository
	eventBus     event.EventBusInterface
	schemaRouter DatabaseRoutingSchemaInterface
}

func NewMetricWriterService(repo MetricRepository) MetricWriterService {
	return &metricWriterService{repo: repo}
}

// NewMetricWriterServiceWithEventBus creates a MetricWriterService with event bus support
func NewMetricWriterServiceWithEventBus(repo MetricRepository, eventBus event.EventBusInterface, schemaRouter DatabaseRoutingSchemaInterface) MetricWriterService {
	return &metricWriterService{repo: repo, eventBus: eventBus, schemaRouter: schemaRouter}
}

// extractServiceNamespaceFromMetric extracts service.namespace from ResourceAttributes JSONB
func extractServiceNamespaceFromMetric(resourceAttributes []byte) string {
	if len(resourceAttributes) == 0 {
		return ""
	}

	var attrs map[string]interface{}
	if err := json.Unmarshal(resourceAttributes, &attrs); err != nil {
		return ""
	}

	if namespace, ok := attrs["service.namespace"].(string); ok {
		return namespace
	}
	return ""
}

func (s *metricWriterService) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {

	// Determine target schema using schema routing if schema is not specified
	targetSchema := schema
	if schema == "" && len(entries) > 0 && s.schemaRouter != nil {
		// Use the first entry to determine schema routing
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := extractServiceNamespaceFromMetric(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		}
	}

	// Insert metrics into target schema
	if targetSchema == "" {
		return nil, fmt.Errorf("no database schema found for gauge metric entry")
	}

	tracer := otel.Tracer("trazr-otel.repository.metrics")
	ctx, span := tracer.Start(ctx, "InsertGaugeMetrics", trace.WithAttributes(
		attribute.String("db.schema", targetSchema),
		attribute.Int("metrics.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.metrics"),
		zap.String("method", "InsertGaugeMetrics"),
		zap.String("schema", targetSchema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertGaugeMetrics started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertGaugeMetrics(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertGaugeMetrics failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Publish metric.gauge.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]MetricEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, toGaugeMetricEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventMetricGaugeInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish metric.gauge.inserted event",
				zap.String("schema", targetSchema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertGaugeMetrics completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

func (s *metricQueryService) GetMetricByIDTyped(ctx context.Context, metricType string, id int64, schema string) (MetricEntryResponse, error) {
	metricType = strings.ToLower(metricType)
	switch metricType {
	case "gauge", "sum", "histogram", "summary", "exponential_histogram":
	default:
		return MetricEntryResponse{}, fmt.Errorf("unsupported metric type: %q", metricType)
	}
	var entry interface{}
	var typ string
	var err error
	if typed, ok := s.repo.(TypedMetricRepository); ok {
		entry, typ, err = typed.GetMetricByIDTyped(ctx, metricType, id, schema)
	} else {
		entry, typ, err = s.repo.GetMetricByID(ctx, id, schema)
	}
	if err != nil {
		return MetricEntryResponse{}, err
	}
	if typ != "metrics_"+metricType {
		return MetricEntryResponse{}, fmt.Errorf("not found")
	}
	switch e := entry.(type) {
	case GaugeMetricEntry:
		return toGaugeMetricEntryResponse(e), nil
	case SumMetricEntry:
		return toSumMetricEntryResponse(e), nil
	case HistogramMetricEntry:
		return toHistogramMetricEntryResponse(e), nil
	case SummaryMetricEntry:
		return toSummaryMetricEntryResponse(e), nil
	case ExponentialHistogramMetricEntry:
		return toExponentialHistogramMetricEntryResponse(e), nil
	default:
		return MetricEntryResponse{}, fmt.Errorf("unknown metric type")
	}
}

func (s *metricWriterService) InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error) {

	// Determine target schema using schema routing if schema is not specified
	targetSchema := schema
	if schema == "" && len(entries) > 0 && s.schemaRouter != nil {
		// Use the first entry to determine schema routing
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := extractServiceNamespaceFromMetric(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		}
	}

	// Insert metrics into target schema
	if targetSchema == "" {
		return nil, fmt.Errorf("no database schema found for sum metric entry")
	}

	tracer := otel.Tracer("trazr-otel.repository.metrics")
	ctx, span := tracer.Start(ctx, "InsertSumMetrics", trace.WithAttributes(
		attribute.String("db.schema", targetSchema),
		attribute.Int("metrics.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.metrics"),
		zap.String("method", "InsertSumMetrics"),
		zap.String("schema", targetSchema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertSumMetrics started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertSumMetrics(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertSumMetrics failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Publish metric.sum.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]MetricEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, toSumMetricEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventMetricSumInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish metric.sum.inserted event",
				zap.String("schema", targetSchema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertSumMetrics completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

func (s *metricWriterService) InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error) {

	// Determine target schema using schema routing if schema is not specified
	targetSchema := schema
	if schema == "" && len(entries) > 0 && s.schemaRouter != nil {
		// Use the first entry to determine schema routing
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := extractServiceNamespaceFromMetric(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		}
	}

	// Insert metrics into target schema
	if targetSchema == "" {
		return nil, fmt.Errorf("no database schema found for histogram metric entry")
	}

	tracer := otel.Tracer("trazr-otel.repository.metrics")
	ctx, span := tracer.Start(ctx, "InsertHistogramMetrics", trace.WithAttributes(
		attribute.String("db.schema", targetSchema),
		attribute.Int("metrics.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.metrics"),
		zap.String("method", "InsertHistogramMetrics"),
		zap.String("schema", targetSchema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertHistogramMetrics started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertHistogramMetrics(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertHistogramMetrics failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Publish metric.histogram.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]MetricEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, toHistogramMetricEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventMetricHistogramInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish metric.histogram.inserted event",
				zap.String("schema", targetSchema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertHistogramMetrics completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

func (s *metricWriterService) InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error) {

	// Determine target schema using schema routing if schema is not specified
	targetSchema := schema
	if schema == "" && len(entries) > 0 && s.schemaRouter != nil {
		// Use the first entry to determine schema routing
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := extractServiceNamespaceFromMetric(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		}
	}

	// Insert metrics into target schema
	if targetSchema == "" {
		return nil, fmt.Errorf("no database schema found for summary metric entry")
	}

	tracer := otel.Tracer("trazr-otel.repository.metrics")
	ctx, span := tracer.Start(ctx, "InsertSummaryMetrics", trace.WithAttributes(
		attribute.String("db.schema", targetSchema),
		attribute.Int("metrics.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.metrics"),
		zap.String("method", "InsertSummaryMetrics"),
		zap.String("schema", targetSchema),
		zap.String("original_schema", schema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertSummaryMetrics started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertSummaryMetrics(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertSummaryMetrics failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Publish metric.summary.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]MetricEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, toSummaryMetricEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventMetricSummaryInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish metric.summary.inserted event",
				zap.String("schema", targetSchema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertSummaryMetrics completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

func (s *metricWriterService) InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error) {

	// Determine target schema using schema routing if schema is not specified
	targetSchema := schema
	if schema != "" && len(entries) > 0 && s.schemaRouter != nil {
		// Use the first entry to determine schema routing
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := extractServiceNamespaceFromMetric(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		}
	}

	// Insert metrics into target schema
	if targetSchema == "" {
		return nil, fmt.Errorf("no database schema found for exponential histogram metric entry")
	}

	tracer := otel.Tracer("trazr-otel.repository.metrics")
	ctx, span := tracer.Start(ctx, "InsertExponentialHistogramMetrics", trace.WithAttributes(
		attribute.String("db.schema", targetSchema),
		attribute.Int("metrics.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.metrics"),
		zap.String("method", "InsertExponentialHistogramMetrics"),
		zap.String("schema", targetSchema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertExponentialHistogramMetrics started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertExponentialHistogramMetrics(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertExponentialHistogramMetrics failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Publish metric.exponential_histogram.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]MetricEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, toExponentialHistogramMetricEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventMetricExponentialHistogramInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish metric.exponential_histogram.inserted event",
				zap.String("schema", targetSchema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertExponentialHistogramMetrics completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

// Mapping functions for each metric type
func toGaugeMetricEntryResponse(e GaugeMetricEntry) MetricEntryResponse {
	metadata := UnmarshalJSONField(e.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return MetricEntryResponse{
		ID:                    e.ID,
		TimestampNS:           e.TimestampNS,
		Timestamp:             e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ServiceName:           e.ServiceName,
		ServiceNamespace:      e.ServiceNamespace,
		ServiceInstanceID:     e.ServiceInstanceID,
		Name:                  DerefString(e.Name),
		Description:           e.Description,
		Unit:                  e.Unit,
		Attributes:            UnmarshalJSONField(e.Attributes),
		ResourceAttributes:    UnmarshalJSONField(e.ResourceAttributes),
		ScopeName:             e.ScopeName,
		ScopeVersion:          e.ScopeVersion,
		ScopeAttributes:       UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount: e.ScopeDroppedAttrCount,
		Metadata:              metadata,
		Flags:                 e.Flags,
		Exemplars:             UnmarshalJSONField(e.Exemplars),
		ResourceSchemaURL:     e.ResourceSchemaURL,
		ScopeSchemaURL:        e.ScopeSchemaURL,
		Value:                 e.Value,
	}
}

func toSumMetricEntryResponse(e SumMetricEntry) MetricEntryResponse {
	metadata := UnmarshalJSONField(e.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return MetricEntryResponse{
		ID:                     e.ID,
		TimestampNS:            e.TimestampNS,
		Timestamp:              e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ServiceName:            e.ServiceName,
		ServiceNamespace:       e.ServiceNamespace,
		ServiceInstanceID:      e.ServiceInstanceID,
		Name:                   DerefString(e.Name),
		Description:            e.Description,
		Unit:                   e.Unit,
		Attributes:             UnmarshalJSONField(e.Attributes),
		ResourceAttributes:     UnmarshalJSONField(e.ResourceAttributes),
		ScopeName:              e.ScopeName,
		ScopeVersion:           e.ScopeVersion,
		ScopeAttributes:        UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount:  e.ScopeDroppedAttrCount,
		Metadata:               metadata,
		Flags:                  e.Flags,
		Exemplars:              UnmarshalJSONField(e.Exemplars),
		ResourceSchemaURL:      e.ResourceSchemaURL,
		ScopeSchemaURL:         e.ScopeSchemaURL,
		StartTime:              timePtrToStringPtr(e.StartTime),
		Value:                  e.Value,
		AggregationTemporality: e.AggregationTemporality,
		IsMonotonic:            e.IsMonotonic,
	}
}

func toHistogramMetricEntryResponse(e HistogramMetricEntry) MetricEntryResponse {
	metadata := UnmarshalJSONField(e.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return MetricEntryResponse{
		ID:                     e.ID,
		TimestampNS:            e.TimestampNS,
		Timestamp:              e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ServiceName:            e.ServiceName,
		ServiceNamespace:       e.ServiceNamespace,
		ServiceInstanceID:      e.ServiceInstanceID,
		Name:                   DerefString(e.Name),
		Description:            e.Description,
		Unit:                   e.Unit,
		Attributes:             UnmarshalJSONField(e.Attributes),
		ResourceAttributes:     UnmarshalJSONField(e.ResourceAttributes),
		ScopeName:              e.ScopeName,
		ScopeVersion:           e.ScopeVersion,
		ScopeAttributes:        UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount:  e.ScopeDroppedAttrCount,
		Metadata:               metadata,
		Flags:                  e.Flags,
		Exemplars:              UnmarshalJSONField(e.Exemplars),
		ResourceSchemaURL:      e.ResourceSchemaURL,
		ScopeSchemaURL:         e.ScopeSchemaURL,
		StartTime:              timePtrToStringPtr(e.StartTime),
		Count:                  e.Count,
		Sum:                    e.Sum,
		Min:                    e.Min,
		Max:                    e.Max,
		Buckets:                UnmarshalJSONField(e.Buckets),
		AggregationTemporality: e.AggregationTemporality,
	}
}

func toSummaryMetricEntryResponse(e SummaryMetricEntry) MetricEntryResponse {
	metadata := UnmarshalJSONField(e.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return MetricEntryResponse{
		ID:                    e.ID,
		TimestampNS:           e.TimestampNS,
		Timestamp:             e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ServiceName:           e.ServiceName,
		ServiceNamespace:      e.ServiceNamespace,
		ServiceInstanceID:     e.ServiceInstanceID,
		Name:                  DerefString(e.Name),
		Description:           e.Description,
		Unit:                  e.Unit,
		Attributes:            UnmarshalJSONField(e.Attributes),
		ResourceAttributes:    UnmarshalJSONField(e.ResourceAttributes),
		ScopeName:             e.ScopeName,
		ScopeVersion:          e.ScopeVersion,
		ScopeAttributes:       UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount: e.ScopeDroppedAttrCount,
		Metadata:              metadata,
		Flags:                 e.Flags,
		Exemplars:             UnmarshalJSONField(e.Exemplars),
		ResourceSchemaURL:     e.ResourceSchemaURL,
		ScopeSchemaURL:        e.ScopeSchemaURL,
		StartTime:             timePtrToStringPtr(e.StartTime),
		Count:                 e.Count,
		Sum:                   e.Sum,
		Quantiles:             UnmarshalJSONField(e.Quantiles),
	}
}

func toExponentialHistogramMetricEntryResponse(e ExponentialHistogramMetricEntry) MetricEntryResponse {
	metadata := UnmarshalJSONField(e.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return MetricEntryResponse{
		ID:                     e.ID,
		TimestampNS:            e.TimestampNS,
		Timestamp:              e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ServiceName:            e.ServiceName,
		ServiceNamespace:       e.ServiceNamespace,
		ServiceInstanceID:      e.ServiceInstanceID,
		Name:                   DerefString(e.Name),
		Description:            e.Description,
		Unit:                   e.Unit,
		Attributes:             UnmarshalJSONField(e.Attributes),
		ResourceAttributes:     UnmarshalJSONField(e.ResourceAttributes),
		ScopeName:              e.ScopeName,
		ScopeVersion:           e.ScopeVersion,
		ScopeAttributes:        UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount:  e.ScopeDroppedAttrCount,
		Metadata:               metadata,
		Flags:                  e.Flags,
		Exemplars:              UnmarshalJSONField(e.Exemplars),
		ResourceSchemaURL:      e.ResourceSchemaURL,
		ScopeSchemaURL:         e.ScopeSchemaURL,
		StartTime:              timePtrToStringPtr(e.StartTime),
		Count:                  e.Count,
		Sum:                    e.Sum,
		Min:                    e.Min,
		Max:                    e.Max,
		Scale:                  e.Scale,
		ZeroCount:              e.ZeroCount,
		ZeroThreshold:          e.ZeroThreshold,
		PositiveOffset:         e.PositiveOffset,
		PositiveBucketCounts:   UnmarshalJSONField(e.PositiveBucketCounts),
		NegativeOffset:         e.NegativeOffset,
		NegativeBucketCounts:   UnmarshalJSONField(e.NegativeBucketCounts),
		AggregationTemporality: e.AggregationTemporality,
	}
}

func (s *metricQueryService) GenerateCursor(ctx context.Context, offset int, filters MetricQuery) (MetricQueryResult, error) {
	// Use existing QueryMetricsGauge method with offset-based query (default to gauge)
	query := filters
	query.Limit = 1
	query.Offset = offset
	query.Direction = "asc" // Default direction for cursor generation

	// Get total count (use gauge as default)
	totalCount, err := s.repo.CountMetricsGauge(ctx, filters)
	if err != nil {
		return MetricQueryResult{}, err
	}

	// Validate offset
	if offset < 0 || int64(offset) >= totalCount {
		return MetricQueryResult{}, fmt.Errorf("invalid offset: %d (total: %d)", offset, totalCount)
	}

	// Query metrics at offset position
	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QueryMetricsGauge(ctx, query)
	if err != nil {
		return MetricQueryResult{}, err
	}

	if len(entries) == 0 {
		return MetricQueryResult{}, fmt.Errorf("no metrics found at offset %d", offset)
	}

	// Convert to response format
	respEntries := make([]MetricEntryResponse, 0, len(entries))
	for _, e := range entries {
		respEntries = append(respEntries, toGaugeMetricEntryResponse(e))
	}

	return MetricQueryResult{
		Entries:    respEntries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		PrevCursor: prevCursor,
		HasPrev:    hasPrev,
	}, nil
}

// Helper for *time.Time to *string
func timePtrToStringPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format("2006-01-02T15:04:05.999999999Z07:00")
	return &s
}

func NewMetricQueryServiceWithUnifiedClient(db OtelDatabaseClient) MetricQueryService {
	return &metricQueryService{repo: db}
}

func NewMetricWriterServiceWithUnifiedClient(db OtelDatabaseClient) MetricWriterService {
	return &metricWriterService{repo: db}
}

// NewMetricWriterServiceWithUnifiedClientAndEventBus creates a MetricWriterService with unified client and event bus
func NewMetricWriterServiceWithUnifiedClientAndEventBus(db OtelDatabaseClient, eventBus event.EventBusInterface) MetricWriterService {
	return &metricWriterService{repo: db, eventBus: eventBus}
}
