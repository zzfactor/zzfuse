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

type TraceEntryResponse struct {
	TraceID               string      `json:"trace_id"`
	SpanID                string      `json:"span_id"`
	ParentSpanID          *string     `json:"parent_span_id,omitempty"`
	Name                  string      `json:"name"`
	Kind                  *string     `json:"kind,omitempty"`
	StartTime             string      `json:"start_time"`
	EndTime               string      `json:"end_time"`
	DurationNS            int64       `json:"duration_ns"`
	ServiceName           *string     `json:"service_name,omitempty"`
	ResourceAttributes    interface{} `json:"resource_attributes,omitempty"`
	ResourceSchemaURL     *string     `json:"resource_schema_url,omitempty"`
	ScopeName             *string     `json:"scope_name,omitempty"`
	ScopeVersion          *string     `json:"scope_version,omitempty"`
	ScopeAttributes       interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount *int32      `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL        *string     `json:"scope_schema_url,omitempty"`
	Attributes            interface{} `json:"attributes,omitempty"`
	DroppedAttrCount      *int32      `json:"dropped_attr_count,omitempty"`
	Events                interface{} `json:"events,omitempty"`
	Links                 interface{} `json:"links,omitempty"`
	StatusCode            *string     `json:"status_code,omitempty"`
	StatusMessage         *string     `json:"status_message,omitempty"`
	Metadata              interface{} `json:"metadata,omitempty"`
}

type TraceQueryResult struct {
	Entries    []TraceEntryResponse `json:"entries"`
	NextCursor string               `json:"next_cursor"`
	HasMore    bool                 `json:"has_more"`
	PrevCursor string               `json:"prev_cursor,omitempty"`
	HasPrev    bool                 `json:"has_prev,omitempty"`
}

// Service interfaces

type TraceQueryService interface {
	QuerySpans(ctx context.Context, q TraceQuery) (TraceQueryResult, error)
	StreamSpans(ctx context.Context, q TraceQuery) (<-chan TraceEntry, error)
	GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntryResponse, error)
	GenerateCursor(ctx context.Context, offset int, filters TraceQuery) (TraceQueryResult, error)
}

type TraceWriterService interface {
	InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error)
}

// Service implementations

type traceQueryService struct {
	repo TraceRepository
}

func NewTraceQueryService(repo TraceRepository) TraceQueryService {
	return &traceQueryService{repo: repo}
}

func (s *traceQueryService) QuerySpans(ctx context.Context, q TraceQuery) (TraceQueryResult, error) {
	// Validate query including schema validation
	if err := ValidateTraceQuery(q); err != nil {
		return TraceQueryResult{}, err
	}

	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QuerySpans(ctx, q)
	if err != nil {
		return TraceQueryResult{}, err
	}

	needReverse := strings.ToLower(q.DirectionResults) != strings.ToLower(q.Direction)
	respEntries := make([]TraceEntryResponse, 0, len(entries))
	if needReverse {
		for i := len(entries) - 1; i >= 0; i-- {
			respEntries = append(respEntries, toTraceEntryResponse(entries[i]))
		}
	} else {
		for _, e := range entries {
			respEntries = append(respEntries, toTraceEntryResponse(e))
		}
	}

	return TraceQueryResult{
		Entries:    respEntries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		PrevCursor: prevCursor,
		HasPrev:    hasPrev,
	}, nil
}

func (s *traceQueryService) StreamSpans(ctx context.Context, q TraceQuery) (<-chan TraceEntry, error) {
	return nil, nil // Not implemented
}

func (s *traceQueryService) GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntryResponse, error) {
	e, err := s.repo.GetSpanByID(ctx, id, schema)
	if err != nil {
		return TraceEntryResponse{}, err
	}
	return toTraceEntryResponse(e), nil
}

func (s *traceQueryService) CountSpans(ctx context.Context, q TraceQuery) (int64, error) {
	return s.repo.CountSpans(ctx, q)
}

func (s *traceQueryService) GenerateCursor(ctx context.Context, offset int, filters TraceQuery) (TraceQueryResult, error) {
	// Use existing QuerySpans method with offset-based query
	query := filters
	query.Limit = 1
	query.Offset = offset
	query.Direction = "asc" // Default direction for cursor generation

	// Get total count
	totalCount, err := s.repo.CountSpans(ctx, filters)
	if err != nil {
		return TraceQueryResult{}, err
	}

	// Validate offset
	if offset < 0 || int64(offset) >= totalCount {
		return TraceQueryResult{}, fmt.Errorf("invalid offset: %d (total: %d)", offset, totalCount)
	}

	// Query spans at offset position
	entries, nextCursor, hasMore, _, _, err := s.repo.QuerySpans(ctx, query)
	if err != nil {
		return TraceQueryResult{}, err
	}

	if len(entries) == 0 {
		return TraceQueryResult{}, fmt.Errorf("no spans found at offset %d", offset)
	}

	// Convert to response format
	respEntries := make([]TraceEntryResponse, 0, len(entries))
	for _, e := range entries {
		respEntries = append(respEntries, toTraceEntryResponse(e))
	}

	return TraceQueryResult{
		Entries:    respEntries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

type traceWriterService struct {
	repo         TraceRepository
	eventBus     event.EventBusInterface
	schemaRouter DatabaseRoutingSchemaInterface
}

func NewTraceWriterService(repo TraceRepository) TraceWriterService {
	return &traceWriterService{repo: repo}
}

// NewTraceWriterServiceWithEventBus creates a TraceWriterService with event bus support
func NewTraceWriterServiceWithEventBus(repo TraceRepository, eventBus event.EventBusInterface, schemaRouter DatabaseRoutingSchemaInterface) TraceWriterService {
	return &traceWriterService{repo: repo, eventBus: eventBus, schemaRouter: schemaRouter}
}

// extractServiceNamespaceFromTrace extracts service.namespace from ResourceAttributes JSONB
func extractServiceNamespaceFromTrace(resourceAttributes []byte) string {
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

func (s *traceWriterService) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {

	// Determine target schema using schema routing if schema is not specified
	targetSchema := schema
	if schema == "" && len(entries) > 0 {
		// Use the first entry to determine schema routing
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := extractServiceNamespaceFromTrace(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		}
	}

	// Insert spans into target schema
	if targetSchema == "" {
		return nil, fmt.Errorf("no database schema found for trace entry")
	}

	tracer := otel.Tracer("trazr-otel.repository.spans")
	ctx, span := tracer.Start(ctx, "InsertSpans", trace.WithAttributes(
		attribute.String("db.schema", targetSchema),
		attribute.Int("spans.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.spans"),
		zap.String("method", "InsertSpans"),
		zap.String("schema", targetSchema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertSpans started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertSpans(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertSpans failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Publish trace.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]TraceEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, toTraceEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventTraceInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish trace.inserted event",
				zap.String("schema", targetSchema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertSpans completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

// Mapping and helper functions

func toTraceEntryResponse(e TraceEntry) TraceEntryResponse {
	metadata := UnmarshalJSONField(e.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return TraceEntryResponse{
		TraceID:               e.TraceID,
		SpanID:                e.SpanID,
		ParentSpanID:          e.ParentSpanID,
		Name:                  e.Name,
		Kind:                  &e.SpanKind,
		StartTime:             e.StartTime.Format("2006-01-02T15:04:05.999999999Z07:00"),
		EndTime:               e.EndTime.Format("2006-01-02T15:04:05.999999999Z07:00"),
		DurationNS:            e.DurationNS,
		ServiceName:           &e.ServiceName,
		ResourceAttributes:    UnmarshalJSONField(e.ResourceAttributes),
		ResourceSchemaURL:     e.ResourceSchemaURL,
		ScopeName:             e.ScopeName,
		ScopeVersion:          e.ScopeVersion,
		ScopeAttributes:       UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount: e.ScopeDroppedAttrCount,
		ScopeSchemaURL:        e.ScopeSchemaURL,
		Attributes:            UnmarshalJSONField(e.Attributes),
		DroppedAttrCount:      e.DroppedAttrCount,
		Events:                UnmarshalJSONField(e.Events),
		Links:                 UnmarshalJSONField(e.Links),
		StatusCode:            nil, // If you want to format StatusCode as string, add a helper
		StatusMessage:         e.StatusMessage,
		Metadata:              metadata,
	}
}

// New constructors for unified client

func NewTraceQueryServiceWithUnifiedClient(db OtelDatabaseClient) TraceQueryService {
	return &traceQueryService{repo: db}
}

func NewTraceWriterServiceWithUnifiedClient(db OtelDatabaseClient) TraceWriterService {
	return &traceWriterService{repo: db}
}

// NewTraceWriterServiceWithUnifiedClientAndEventBus creates a TraceWriterService with unified client and event bus
func NewTraceWriterServiceWithUnifiedClientAndEventBus(db OtelDatabaseClient, eventBus event.EventBusInterface) TraceWriterService {
	return &traceWriterService{repo: db, eventBus: eventBus}
}
