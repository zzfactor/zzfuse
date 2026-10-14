package repository

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// API response models

type RunQueryResult struct {
	Entries    []RunEntryResponse `json:"entries"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

// Service interfaces

type RunQueryService interface {
	QueryRuns(ctx context.Context, q RunQuery) (RunQueryResult, error)
	StreamRuns(ctx context.Context, q RunQuery) (<-chan RunEntry, error)
	GetRunByID(ctx context.Context, id int64, schema string) (RunEntryResponse, error)
}

type RunWriterService interface {
	InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error)
	UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error
	EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error
}

// Service implementations

type runQueryService struct {
	repo RunRepository
}

func NewRunQueryService(repo RunRepository) RunQueryService {
	return &runQueryService{repo: repo}
}

func (s *runQueryService) QueryRuns(ctx context.Context, q RunQuery) (RunQueryResult, error) {
	entries, nextCursor, hasMore, err := s.repo.QueryRuns(ctx, q)
	if err != nil {
		return RunQueryResult{}, err
	}
	respEntries := make([]RunEntryResponse, 0, len(entries))
	for _, e := range entries {
		respEntries = append(respEntries, toRunEntryResponse(e))
	}
	return RunQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore}, nil
}

func (s *runQueryService) StreamRuns(ctx context.Context, q RunQuery) (<-chan RunEntry, error) {
	return nil, nil // Not implemented
}

func (s *runQueryService) GetRunByID(ctx context.Context, id int64, schema string) (RunEntryResponse, error) {
	entry, err := s.repo.GetRunByID(ctx, id, schema)
	if err != nil {
		return RunEntryResponse{}, err
	}
	return toRunEntryResponse(entry), nil
}

type runWriterService struct {
	repo RunRepository
}

func NewRunWriterService(repo RunRepository) RunWriterService {
	return &runWriterService{repo: repo}
}

func (s *runWriterService) InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error) {
	tracer := otel.Tracer("trazr-otel.repository.runs")
	ctx, span := tracer.Start(ctx, "InsertRuns", trace.WithAttributes(
		attribute.String("db.schema", schema),
		attribute.Int("runs.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.runs"),
		zap.String("method", "InsertRuns"),
		zap.String("schema", schema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertRuns started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	ids, err := s.repo.InsertRuns(ctx, entries, schema)
	if err != nil {
		logger.Error("InsertRuns failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}
	logger.Debug("InsertRuns completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return ids, nil
}

func (s *runWriterService) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error {
	tracer := otel.Tracer("trazr-otel.repository.runs")
	ctx, span := tracer.Start(ctx, "UpdateRunMetadata", trace.WithAttributes(
		attribute.Int64("run.id", runID),
		attribute.String("db.schema", schema),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.runs"),
		zap.String("method", "UpdateRunMetadata"),
		zap.Int64("run_id", runID),
		zap.String("schema", schema),
	)
	logger.Debug("UpdateRunMetadata started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	err := s.repo.UpdateRunMetadata(ctx, runID, metadata, schema)
	if err != nil {
		logger.Error("UpdateRunMetadata failed", zap.Error(err))
		span.RecordError(err)
		return err
	}
	logger.Debug("UpdateRunMetadata completed", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return nil
}

func (s *runWriterService) EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error {
	tracer := otel.Tracer("trazr-otel.repository.runs")
	ctx, span := tracer.Start(ctx, "EndRun", trace.WithAttributes(
		attribute.String("run.name", name),
		attribute.Int64("end_timestamp_ns", endTimestampNs),
		attribute.String("db.schema", schema),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.runs"),
		zap.String("method", "EndRun"),
		zap.String("name", name),
		zap.String("schema", schema),
	)
	logger.Debug("EndRun started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))

	err := s.repo.EndRun(ctx, name, endTimestampNs, schema)
	if err != nil {
		logger.Error("EndRun failed", zap.Error(err))
		span.RecordError(err)
		return err
	}
	logger.Debug("EndRun completed", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().SpanID().String()))
	return nil
}

// Mapping and helper functions

func toRunEntryResponse(e RunEntry) RunEntryResponse {
	return RunEntryResponse{
		ID:               e.ID,
		SessionID:        e.SessionID,
		ServiceName:      e.ServiceName,
		ScopeName:        e.ScopeName,
		Name:             e.Name,
		StartTimestampNs: e.StartTimestampNs,
		EndTimestampNs:   e.EndTimestampNs,
		TraceID:          e.TraceID,
		ProcessorID:      e.ProcessorID,
		RunType:          e.RunType,
		Metadata:         e.Metadata,
		CreatedAt:        e.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt:        e.UpdatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

// New constructors for unified client

type runQueryServiceWithUnifiedClient struct {
	repo OtelDatabaseClient
}

func NewRunQueryServiceWithUnifiedClient(db OtelDatabaseClient) RunQueryService {
	return &runQueryServiceWithUnifiedClient{repo: db}
}

func (s *runQueryServiceWithUnifiedClient) QueryRuns(ctx context.Context, q RunQuery) (RunQueryResult, error) {
	entries, nextCursor, hasMore, err := s.repo.QueryRuns(ctx, q)
	if err != nil {
		return RunQueryResult{}, err
	}
	respEntries := make([]RunEntryResponse, 0, len(entries))
	for _, e := range entries {
		respEntries = append(respEntries, toRunEntryResponse(e))
	}
	return RunQueryResult{Entries: respEntries, NextCursor: nextCursor, HasMore: hasMore}, nil
}

func (s *runQueryServiceWithUnifiedClient) StreamRuns(ctx context.Context, q RunQuery) (<-chan RunEntry, error) {
	return nil, nil // Not implemented
}

func (s *runQueryServiceWithUnifiedClient) GetRunByID(ctx context.Context, id int64, schema string) (RunEntryResponse, error) {
	entry, err := s.repo.GetRunByID(ctx, id, schema)
	if err != nil {
		return RunEntryResponse{}, err
	}
	return toRunEntryResponse(entry), nil
}

type runWriterServiceWithUnifiedClient struct {
	repo   OtelDatabaseClient
	schema string
}

func NewRunWriterServiceWithUnifiedClient(db OtelDatabaseClient) RunWriterService {
	return &runWriterServiceWithUnifiedClient{repo: db, schema: DefaultSchema}
}

func (s *runWriterServiceWithUnifiedClient) InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error) {
	tracer := otel.Tracer("trazr-otel.repository.runs")
	ctx, span := tracer.Start(ctx, "InsertRuns", trace.WithAttributes(
		attribute.String("db.schema", s.schema),
		attribute.Int("runs.count", len(entries)),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.runs"),
		zap.String("method", "InsertRuns"),
		zap.String("schema", s.schema),
		zap.Int("count", len(entries)),
	)
	logger.Debug("InsertRuns started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().TraceID().String()))

	ids, err := s.repo.InsertRuns(ctx, entries, s.schema)
	if err != nil {
		logger.Error("InsertRuns failed", zap.Error(err))
		span.RecordError(err)
		return nil, err
	}
	logger.Debug("InsertRuns completed", zap.Int("inserted", len(ids)), zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().TraceID().String()))
	return ids, nil
}

func (s *runWriterServiceWithUnifiedClient) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error {
	tracer := otel.Tracer("trazr-otel.repository.runs")
	ctx, span := tracer.Start(ctx, "UpdateRunMetadata", trace.WithAttributes(
		attribute.Int64("run.id", runID),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.runs"),
		zap.String("method", "UpdateRunMetadata"),
		zap.Int64("run_id", runID),
	)
	logger.Debug("UpdateRunMetadata started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().TraceID().String()))

	err := s.repo.UpdateRunMetadata(ctx, runID, metadata, s.schema)
	if err != nil {
		logger.Error("UpdateRunMetadata failed", zap.Error(err))
		span.RecordError(err)
		return err
	}
	logger.Debug("UpdateRunMetadata completed", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().TraceID().String()))
	return nil
}

func (s *runWriterServiceWithUnifiedClient) EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error {
	tracer := otel.Tracer("trazr-otel.repository.runs")
	ctx, span := tracer.Start(ctx, "EndRun", trace.WithAttributes(
		attribute.String("run.name", name),
		attribute.Int64("end_timestamp_ns", endTimestampNs),
	))
	defer span.End()

	logger := zap.L().With(
		zap.String("component", "repository.runs"),
		zap.String("method", "EndRun"),
		zap.String("name", name),
		zap.String("schema", s.schema),
	)
	logger.Debug("EndRun started", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().TraceID().String()))

	err := s.repo.EndRun(ctx, name, endTimestampNs, s.schema)
	if err != nil {
		logger.Error("EndRun failed", zap.Error(err))
		span.RecordError(err)
		return err
	}
	logger.Debug("EndRun completed", zap.String("trace_id", span.SpanContext().TraceID().String()), zap.String("span_id", span.SpanContext().TraceID().String()))
	return nil
}
