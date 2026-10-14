package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// DatabaseRoutingSchemaInterface defines the interface for schema routing
type DatabaseRoutingSchemaInterface interface {
	GetSchema(serviceName, serviceNamespace string) (string, bool)
	Reset(ctx context.Context) error
	GetAllEnabled(ctx context.Context) ([]*database.DatabaseSchemaConfig, error)
}

// API response models

type LogQueryResult struct {
	Entries    []LogEntryResponse `json:"entries"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
	PrevCursor string             `json:"prev_cursor,omitempty"`
	HasPrev    bool               `json:"has_prev,omitempty"`
}

type LogEntryResponse struct {
	ID                    int64       `json:"id"`
	TimestampNS           int64       `json:"timestamp_ns"`
	Timestamp             string      `json:"timestamp"`
	ObservedTimestamp     *string     `json:"observed_timestamp,omitempty"`
	TraceID               *string     `json:"trace_id,omitempty"`
	SpanID                *string     `json:"span_id,omitempty"`
	Flags                 *int32      `json:"flags,omitempty"`
	SeverityText          string      `json:"severity_text"`
	SeverityNumber        int32       `json:"severity_number"`
	ServiceName           string      `json:"service_name"`
	ResourceAttributes    interface{} `json:"resource_attributes,omitempty"`
	ResourceSchemaURL     *string     `json:"resource_schema_url,omitempty"`
	ScopeName             *string     `json:"scope_name,omitempty"`
	ScopeVersion          *string     `json:"scope_version,omitempty"`
	ScopeAttributes       interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount *int32      `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL        *string     `json:"scope_schema_url,omitempty"`
	Attributes            interface{} `json:"attributes,omitempty"`
	DroppedAttrCount      *int32      `json:"dropped_attr_count,omitempty"`
	Body                  string      `json:"body"`
	Metadata              interface{} `json:"metadata,omitempty"`
}

// Service interfaces

type LogQueryService interface {
	QueryLogs(ctx context.Context, q LogQuery) (LogQueryResult, error)
	CountLogs(ctx context.Context, q LogQuery) (int64, error)
	StreamLogs(ctx context.Context, q LogQuery) (<-chan LogEntry, error)
	GetLogByID(ctx context.Context, id int64, schema string) (LogEntryResponse, error)
	GenerateCursor(ctx context.Context, offset int, filters LogQuery) (LogQueryResult, error)
}

type LogWriterService interface {
	InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error)
}

// Service implementations

type logQueryService struct {
	repo LogRepository
}

func NewLogQueryService(repo LogRepository) LogQueryService {
	return &logQueryService{repo: repo}
}

func (s *logQueryService) QueryLogs(ctx context.Context, q LogQuery) (LogQueryResult, error) {
	// Validate query including schema validation
	if errors := ValidateLogQuery(q); len(errors) > 0 {
		return LogQueryResult{}, fmt.Errorf("validation errors: %v", errors)
	}

	entries, nextCursor, hasMore, prevCursor, hasPrev, err := s.repo.QueryLogs(ctx, q)
	if err != nil {
		return LogQueryResult{}, err
	}

	needReverse := strings.ToLower(q.DirectionResults) != strings.ToLower(q.Direction)
	respEntries := make([]LogEntryResponse, 0, len(entries))
	if needReverse {
		for i := len(entries) - 1; i >= 0; i-- {
			respEntries = append(respEntries, toLogEntryResponse(entries[i]))
		}
	} else {
		for _, e := range entries {
			respEntries = append(respEntries, toLogEntryResponse(e))
		}
	}

	return LogQueryResult{
		Entries:    respEntries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
		PrevCursor: prevCursor,
		HasPrev:    hasPrev,
	}, nil
}

func (s *logQueryService) CountLogs(ctx context.Context, q LogQuery) (int64, error) {
	// Validate query including schema validation
	if errors := ValidateLogQuery(q); len(errors) > 0 {
		return 0, fmt.Errorf("validation errors: %v", errors)
	}

	return s.repo.CountLogs(ctx, q)
}

func (s *logQueryService) StreamLogs(ctx context.Context, q LogQuery) (<-chan LogEntry, error) {
	return nil, nil // Not implemented
}

func (s *logQueryService) GetLogByID(ctx context.Context, id int64, schema string) (LogEntryResponse, error) {
	e, err := s.repo.GetLogByID(ctx, id, schema)
	if err != nil {
		return LogEntryResponse{}, err
	}
	return toLogEntryResponse(e), nil
}

func (s *logQueryService) GenerateCursor(ctx context.Context, offset int, filters LogQuery) (LogQueryResult, error) {
	// Use existing QueryLogs method with offset-based query
	query := filters
	query.Limit = 1
	query.Offset = offset
	query.Direction = "asc" // Default direction for cursor generation

	// Validate offset
	if offset < 0 {
		return LogQueryResult{}, fmt.Errorf("invalid offset: %d", offset)
	}

	// Query logs at offset position
	entries, nextCursor, hasMore, _, _, err := s.repo.QueryLogs(ctx, query)
	if err != nil {
		return LogQueryResult{}, err
	}

	if len(entries) == 0 {
		return LogQueryResult{}, fmt.Errorf("no logs found at offset %d", offset)
	}

	// Convert to response format
	respEntries := make([]LogEntryResponse, 0, len(entries))
	for _, e := range entries {
		respEntries = append(respEntries, toLogEntryResponse(e))
	}

	return LogQueryResult{
		Entries:    respEntries,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

type logWriterService struct {
	repo         LogRepository
	eventBus     event.EventBusInterface
	schemaRouter DatabaseRoutingSchemaInterface
}

func NewLogWriterService(repo LogRepository) LogWriterService {
	return &logWriterService{repo: repo}
}

// NewLogWriterServiceWithEventBus creates a LogWriterService with event bus support
func NewLogWriterServiceWithEventBus(repo LogRepository, eventBus event.EventBusInterface, schemaRouter DatabaseRoutingSchemaInterface) LogWriterService {
	return &logWriterService{repo: repo, eventBus: eventBus, schemaRouter: schemaRouter}
}

// extractServiceNamespace extracts service.namespace from ResourceAttributes JSONB
func extractServiceNamespace(resourceAttributes []byte) string {
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

func (s *logWriterService) InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error) {
	return s.InsertLogsWithRunProcessing(ctx, entries, schema, nil, nil, nil)
}

// InsertLogsWithRunProcessing inserts logs and processes run signals in a single operation
func (s *logWriterService) InsertLogsWithRunProcessing(ctx context.Context, entries []LogEntry, schema string, runSignalProcessor *RunSignalProcessor, runMetadataProcessor *RunMetadataProcessor, runService *RunService) ([]string, error) {

	// Determine target schema using schema routing schema is not specified
	targetSchema := schema
	if schema == "" && len(entries) > 0 {
		// Use the first entry to determine schema routing
		firstEntry := entries[0]
		serviceName := firstEntry.ServiceName
		serviceNamespace := extractServiceNamespace(firstEntry.ResourceAttributes)

		if routedSchema, found := s.schemaRouter.GetSchema(serviceName, serviceNamespace); found {
			targetSchema = routedSchema
		}
	}

	// Insert logs into target schema
	if targetSchema == "" {
		return nil, fmt.Errorf("no database schema found for log entry")
	}

	tracer := otel.Tracer("trazr-otel.repository.logs")
	ctx, span := tracer.Start(ctx, "InsertLogs", trace.WithAttributes(
		attribute.String("db.schema", targetSchema),
		attribute.Int("logs.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.logs"),
		zap.String("method", "InsertLogs"),
		zap.String("schema", targetSchema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertLogs started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertLogs(ctx, entries, targetSchema)
	if err != nil {
		logger.Error("InsertLogs failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}

	// Process run signals if processors are provided
	if runSignalProcessor != nil && runMetadataProcessor != nil && runService != nil {
		err := runSignalProcessor.ProcessRunSignalsFromLogEntries(ctx, entries, targetSchema, runMetadataProcessor, runService, logger)
		if err != nil {
			logger.Warn("Run processing failed", zap.Error(err))
			// Don't fail the entire operation if run processing fails
		}
	}

	// Publish log.inserted event after successful insert
	if s.eventBus != nil {
		batch := make([]LogEntryResponse, 0, len(entries))
		for _, entry := range entries {
			batch = append(batch, ToLogEntryResponse(entry))
		}
		if err := s.eventBus.Publish(ctx, event.Event{
			Type:      event.EventLogInserted,
			Timestamp: time.Now(),
			Schema:    targetSchema,
			Data:      batch,
			IDs:       nil,
		}); err != nil {
			logger.Warn("Failed to publish log.inserted event",
				zap.String("schema", schema),
				zap.Int("batch_size", len(entries)),
				zap.Error(err))
		}
	}

	logger.Debug("InsertLogs completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

// Mapping and helper functions

func toLogEntryResponse(e LogEntry) LogEntryResponse {
	return LogEntryResponse{
		ID:                    e.ID,
		TimestampNS:           e.TimestampNS,
		Timestamp:             e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ObservedTimestamp:     timePtrToStringPtr(e.ObservedTimestamp),
		TraceID:               e.TraceID,
		SpanID:                e.SpanID,
		Flags:                 e.Flags,
		SeverityText:          DerefString(e.SeverityText),
		SeverityNumber:        DerefInt32(e.SeverityNumber),
		ServiceName:           e.ServiceName,
		ResourceAttributes:    UnmarshalJSONField(e.ResourceAttributes),
		ResourceSchemaURL:     e.ResourceSchemaURL,
		ScopeName:             e.ScopeName,
		ScopeVersion:          e.ScopeVersion,
		ScopeAttributes:       UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount: e.ScopeDroppedAttrCount,
		ScopeSchemaURL:        e.ScopeSchemaURL,
		Attributes:            UnmarshalJSONField(e.Attributes),
		DroppedAttrCount:      e.DroppedAttrCount,
		Body:                  DerefString(ToStringPtr(UnmarshalJSONField(e.Body))),
		Metadata:              UnmarshalJSONField(e.Metadata),
	}
}

// Exported for use in event publishing and other packages
func ToLogEntryResponse(e LogEntry) LogEntryResponse {
	return LogEntryResponse{
		ID:                    e.ID,
		TimestampNS:           e.TimestampNS,
		Timestamp:             e.Timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"),
		ObservedTimestamp:     timePtrToStringPtr(e.ObservedTimestamp),
		TraceID:               e.TraceID,
		SpanID:                e.SpanID,
		Flags:                 e.Flags,
		SeverityText:          DerefString(e.SeverityText),
		SeverityNumber:        DerefInt32(e.SeverityNumber),
		ServiceName:           e.ServiceName,
		ResourceAttributes:    UnmarshalJSONField(e.ResourceAttributes),
		ResourceSchemaURL:     e.ResourceSchemaURL,
		ScopeName:             e.ScopeName,
		ScopeVersion:          e.ScopeVersion,
		ScopeAttributes:       UnmarshalJSONField(e.ScopeAttributes),
		ScopeDroppedAttrCount: e.ScopeDroppedAttrCount,
		ScopeSchemaURL:        e.ScopeSchemaURL,
		Attributes:            UnmarshalJSONField(e.Attributes),
		DroppedAttrCount:      e.DroppedAttrCount,
		Body:                  DerefString(ToStringPtr(UnmarshalJSONField(e.Body))),
		Metadata:              UnmarshalJSONField(e.Metadata),
	}
}

// NewLogRepository creates a new log repository - removed duplicate definition
// Use the one in otel_logs_repository.go instead

// New constructors for unified client

func NewLogQueryServiceWithUnifiedClient(db OtelDatabaseClient) LogQueryService {
	return &logQueryService{repo: db}
}

func NewLogWriterServiceWithUnifiedClient(db OtelDatabaseClient) LogWriterService {
	return &logWriterService{repo: db}
}

// NewLogWriterServiceWithUnifiedClientAndEventBus creates a LogWriterService with unified client and event bus
func NewLogWriterServiceWithUnifiedClientAndEventBus(db OtelDatabaseClient, eventBus event.EventBusInterface) LogWriterService {
	return &logWriterService{repo: db, eventBus: eventBus}
}
