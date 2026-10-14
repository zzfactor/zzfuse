package internal

import (
	"context"
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// LogsPreloadExecutor implements preload for logs streaming
type LogsPreloadExecutor struct {
	querySvc repository.LogQueryService
}

func NewLogsPreloadExecutor(querySvc repository.LogQueryService) *LogsPreloadExecutor {
	return &LogsPreloadExecutor{querySvc: querySvc}
}

func (e *LogsPreloadExecutor) ExecutePreload(ctx context.Context, commonParams *CommonQueryParams, conn *websocket.Conn) error {
	if commonParams == nil || commonParams.Schema == "" || !auth.UserHasSchemaAccess(ctx, commonParams.Schema) {
		return fmt.Errorf("access denied to preload schema")
	}
	var q repository.LogQuery

	if commonParams.Cursor != "" {
		// Cursor-based preload
		q = repository.LogQuery{}
		q.CommonBaseFilters.Cursor = commonParams.Cursor
		q.CommonBaseFilters.Schema = commonParams.Schema
		q.CommonBaseFilters.Direction = "asc" // Always ascending for streaming preload
		q.CommonBaseFilters.Limit = commonParams.Limit
	} else {
		// Time-based preload
		q = repository.LogQuery{
			TimeRange: repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
		}
		q.CommonBaseFilters.Schema = commonParams.Schema
		q.CommonBaseFilters.Direction = "asc"
		q.CommonBaseFilters.Limit = commonParams.Limit
	}

	res, err := e.querySvc.QueryLogs(ctx, q)
	if err != nil {
		return fmt.Errorf("query logs failed: %w", err)
	}

	// Send preload entries oldest->newest
	consecutiveWriteErrors := 0
	for i := len(res.Entries) - 1; i >= 0; i-- {
		entry := res.Entries[i]
		if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
			return fmt.Errorf("write preload entry failed: %w", err)
		}
	}

	return nil
}

// TracesPreloadExecutor implements preload for traces streaming
type TracesPreloadExecutor struct {
	querySvc repository.TraceQueryService
}

func NewTracesPreloadExecutor(querySvc repository.TraceQueryService) *TracesPreloadExecutor {
	return &TracesPreloadExecutor{querySvc: querySvc}
}

func (e *TracesPreloadExecutor) ExecutePreload(ctx context.Context, commonParams *CommonQueryParams, conn *websocket.Conn) error {
	if commonParams == nil || commonParams.Schema == "" || !auth.UserHasSchemaAccess(ctx, commonParams.Schema) {
		return fmt.Errorf("access denied to preload schema")
	}
	var q repository.TraceQuery

	if commonParams.Cursor != "" {
		// Cursor-based preload
		q = repository.TraceQuery{}
		// Set embedded CommonBaseFilters fields
		q.CommonBaseFilters.Cursor = commonParams.Cursor
		q.CommonBaseFilters.Schema = commonParams.Schema
		q.CommonBaseFilters.Direction = "asc" // Always ascending for streaming preload
		q.CommonBaseFilters.Limit = commonParams.Limit
	} else {
		// Time-based preload
		q = repository.TraceQuery{
			TimeRange: repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
		}
		// Set embedded CommonBaseFilters fields
		q.CommonBaseFilters.Schema = commonParams.Schema
		q.CommonBaseFilters.Direction = "asc"
		q.CommonBaseFilters.Limit = commonParams.Limit
	}

	res, err := e.querySvc.QuerySpans(ctx, q)
	if err != nil {
		return fmt.Errorf("query spans failed: %w", err)
	}

	// Send preload entries oldest->newest
	consecutiveWriteErrors := 0
	for i := len(res.Entries) - 1; i >= 0; i-- {
		entry := res.Entries[i]
		if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
			return fmt.Errorf("write preload entry failed: %w", err)
		}
	}

	return nil
}

// MetricsPreloadExecutor implements preload for metrics streaming
type MetricsPreloadExecutor struct {
	querySvc repository.MetricQueryService
}

func NewMetricsPreloadExecutor(querySvc repository.MetricQueryService) *MetricsPreloadExecutor {
	return &MetricsPreloadExecutor{querySvc: querySvc}
}

func (e *MetricsPreloadExecutor) ExecutePreload(ctx context.Context, commonParams *CommonQueryParams, conn *websocket.Conn) error {
	if commonParams == nil || commonParams.Schema == "" || !auth.UserHasSchemaAccess(ctx, commonParams.Schema) {
		return fmt.Errorf("access denied to preload schema")
	}
	var mq repository.MetricQuery

	if commonParams.Cursor != "" {
		// Cursor-based preload
		mq = repository.MetricQuery{}
		// Set embedded CommonBaseFilters fields
		mq.CommonBaseFilters.Cursor = commonParams.Cursor
		mq.CommonBaseFilters.Schema = commonParams.Schema
		mq.CommonBaseFilters.Direction = "asc" // Always ascending for streaming preload
		mq.CommonBaseFilters.Limit = commonParams.Limit
	} else {
		// Time-based preload
		mq = repository.MetricQuery{
			TimeRange: repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
		}
		// Set embedded CommonBaseFilters fields
		mq.CommonBaseFilters.Schema = commonParams.Schema
		mq.CommonBaseFilters.Direction = "asc"
		mq.CommonBaseFilters.Limit = commonParams.Limit
	}

	// Note: This is a simplified implementation. In practice, you'd need to handle
	// different metric types (gauge, sum, histogram, etc.) based on the request
	res, err := e.querySvc.QueryMetricsGauge(ctx, mq)
	if err != nil {
		return fmt.Errorf("query metrics failed: %w", err)
	}

	// Send preload entries oldest->newest
	consecutiveWriteErrors := 0
	for i := len(res.Entries) - 1; i >= 0; i-- {
		entry := res.Entries[i]
		if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
			return fmt.Errorf("write preload entry failed: %w", err)
		}
	}

	return nil
}

// RunsPreloadExecutor implements preload for runs streaming
type RunsPreloadExecutor struct {
	querySvc repository.RunQueryService
}

func NewRunsPreloadExecutor(querySvc repository.RunQueryService) *RunsPreloadExecutor {
	return &RunsPreloadExecutor{querySvc: querySvc}
}

func (e *RunsPreloadExecutor) ExecutePreload(ctx context.Context, commonParams *CommonQueryParams, conn *websocket.Conn) error {
	if commonParams == nil || commonParams.Schema == "" || !auth.UserHasSchemaAccess(ctx, commonParams.Schema) {
		return fmt.Errorf("access denied to preload schema")
	}
	var q repository.RunQuery

	if commonParams.Cursor != "" {
		// Cursor-based preload
		q = repository.RunQuery{
			Cursor:    commonParams.Cursor,
			Schema:    commonParams.Schema,
			Direction: "asc", // Always ascending for streaming preload
			Limit:     commonParams.Limit,
		}
	} else {
		// Time-based preload
		q = repository.RunQuery{
			TimeRange: repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
			Schema:    commonParams.Schema,
			Direction: "asc",
			Limit:     commonParams.Limit,
		}
	}

	res, err := e.querySvc.QueryRuns(ctx, q)
	if err != nil {
		return fmt.Errorf("query runs failed: %w", err)
	}

	// Send preload entries oldest->newest
	consecutiveWriteErrors := 0
	for i := len(res.Entries) - 1; i >= 0; i-- {
		entry := res.Entries[i]
		msg := map[string]interface{}{"type": "run_upsert", "run": entry}
		if err := wsWriteWithTimeout(conn, msg, &consecutiveWriteErrors); err != nil {
			return fmt.Errorf("write preload entry failed: %w", err)
		}
	}

	return nil
}
