// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"context"
	"encoding/json"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/trazrexporter/internal"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type logsExporter struct {
	baseExporter
	logWriter repository.LogWriterService
}

// newLogsExporter creates a logs exporter without initializing the SRM.
// The SRM is obtained from the TrazrExtension via Host.GetExtensions() during start().
func newLogsExporter(logger *zap.Logger, cfg *Config) *logsExporter {
	return &logsExporter{
		baseExporter: baseExporter{
			logger:     logger,
			cfg:        cfg,
			signalType: "logs",
		},
	}
}

// start initializes the exporter by obtaining the ServiceResourceManager from the TrazrExtension.
// This is called by the OTel Collector after all extensions have started.
func (e *logsExporter) start(ctx context.Context, host component.Host) error {
	if err := e.initializeSRM(ctx, host); err != nil {
		return err
	}

	// Get the log writer service
	logWriter, err := e.srm.GetLogWriter()
	if err != nil {
		return fmt.Errorf("failed to get log writer: %w", err)
	}
	e.logWriter = logWriter

	e.logger.Info("Logs exporter started successfully")
	return nil
}

// pushLogsData consumes logs data and exports it to PostgreSQL using efficient bulk inserts.
//
// The function separates concerns by:
// 1. Processing and inserting logs (primary responsibility)
// 2. Processing runs from logs (secondary responsibility)
//
// This approach ensures clean code, better error handling, and easier optimization.
func (e *logsExporter) pushLogsData(ctx context.Context, ld plog.Logs) error {

	if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
		span.AddEvent("pushLogsData called", trace.WithAttributes(
			attribute.Int("num_records", ld.LogRecordCount()),
		))
	}

	e.logger.Debug("Starting pushLogsData",
		zap.Int("total_log_records", ld.LogRecordCount()),
		zap.Int("resource_logs_count", ld.ResourceLogs().Len()))

	// Process and insert logs (run processing is now handled in repository layer)
	if err := e.processAndInsertLogs(ctx, ld); err != nil {
		return fmt.Errorf("process and insert logs: %w", err)
	}

	return nil
}

// processAndInsertLogs processes logs and inserts them into the database.
// This function focuses solely on log processing and insertion.
func (e *logsExporter) processAndInsertLogs(ctx context.Context, ld plog.Logs) error {
	tenantSchema, err := e.tenantSchema(ctx)
	if err != nil {
		return err
	}

	// Early exit for empty logs
	if ld.ResourceLogs().Len() == 0 {
		return nil
	}

	// Pre-allocate groups map with capacity hint for better performance
	groups := make(map[string][]repository.LogEntry, 10) // key: schemaName

	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		logs := ld.ResourceLogs().At(i)
		res := logs.Resource()

		e.logger.Debug("Processing ResourceLogs",
			zap.Int("resource_index", i),
			zap.String("schema_url", logs.SchemaUrl()))

		serviceName := internal.GetServiceName(res.Attributes())
		schema := tenantSchema

		// Marshal resource attributes once per ResourceLogs (immutable)
		resourceAttrsJSON, err := json.Marshal(internal.AttributesToMap(res.Attributes()))
		if err != nil {
			e.logger.Warn("Failed to marshal resource attributes", zap.Error(err))
			continue
		}
		resourceSchemaURL := internal.PtrFromString(logs.SchemaUrl())

		for j := 0; j < logs.ScopeLogs().Len(); j++ {
			rs := logs.ScopeLogs().At(j)
			scope := rs.Scope()

			// Marshal scope attributes once per ScopeLogs
			scopeAttrsJSON, err := json.Marshal(internal.AttributesToMap(scope.Attributes()))
			if err != nil {
				e.logger.Warn("Failed to marshal scope attributes", zap.Error(err))
				continue
			}
			scopeName := internal.PtrFromString(scope.Name())
			scopeVersion := internal.PtrFromString(scope.Version())
			scopeDroppedAttrCount := internal.PtrFromInt32(int32(scope.DroppedAttributesCount()))
			scopeSchemaURL := internal.PtrFromString(rs.SchemaUrl())

			for k := 0; k < rs.LogRecords().Len(); k++ {
				lr := rs.LogRecords().At(k)

				// Reduced debug logging for performance - only log at trace level if needed
				if e.logger.Core().Enabled(zap.DebugLevel) {
					e.logger.Debug("Processing LogRecord",
						zap.Int("log_index", k),
						zap.String("trace_id", internal.TraceIDToHex(lr.TraceID())),
						zap.String("span_id", internal.SpanIDToHex(lr.SpanID())))
				}

				logAttrsJSON, err := json.Marshal(internal.AttributesToMap(lr.Attributes()))
				if err != nil {
					e.logger.Warn("Failed to marshal log attributes", zap.Error(err))
					continue
				}
				traceIDHex := internal.TraceIDToHex(lr.TraceID())
				spanIDHex := internal.SpanIDToHex(lr.SpanID())
				bodyBytes, err := json.Marshal(lr.Body().AsRaw())
				if err != nil {
					// If marshaling fails, set Body to nil and log the error.
					// This will result in a log entry with an empty body.
					bodyBytes = nil
					e.logger.Warn("Failed to marshal log body", zap.Error(err))
				}
				timestamp := internal.NormalizeTimestamp(lr.Timestamp().AsTime())
				observedTimestamp := internal.NormalizeTimestamp(lr.ObservedTimestamp().AsTime())
				entry := repository.LogEntry{
					TimestampNS:       timestamp.Time.UnixNano(),
					Timestamp:         timestamp.Time,
					ObservedTimestamp: &observedTimestamp.Time,
					TraceID:           internal.PtrFromString(traceIDHex),
					SpanID:            internal.PtrFromString(spanIDHex),
					Flags:             internal.PtrFromInt32(int32(lr.Flags())),
					SeverityText:      internal.PtrFromString(lr.SeverityText()),
					SeverityNumber:    internal.PtrFromInt32(int32(lr.SeverityNumber())),
					// Service entity fields (OTel v1.39.0)
					ServiceName:           serviceName,
					ServiceNamespace:      internal.GetServiceNamespacePtr(res.Attributes()),
					ServiceInstanceID:     internal.GetServiceInstanceIDPtr(res.Attributes()),
					EventName:             internal.GetEventName(lr.Attributes()),
					ResourceAttributes:    resourceAttrsJSON,
					ResourceSchemaURL:     resourceSchemaURL,
					ScopeName:             scopeName,
					ScopeVersion:          scopeVersion,
					ScopeAttributes:       scopeAttrsJSON,
					ScopeDroppedAttrCount: scopeDroppedAttrCount,
					ScopeSchemaURL:        scopeSchemaURL,
					Attributes:            logAttrsJSON,
					DroppedAttrCount:      internal.PtrFromInt32(int32(lr.DroppedAttributesCount())),
					Body:                  bodyBytes,
					Metadata:              []byte("{}"),
				}

				// Create a copy of the entry with the correct schema name
				// Pre-allocate slice capacity for better performance
				if groups[schema] == nil {
					groups[schema] = make([]repository.LogEntry, 0, 100) // Capacity hint
				}
				groups[schema] = append(groups[schema], entry)
			}
		}
	}

	// Insert logs to database
	for schemaName, entries := range groups {
		e.logger.Debug("Inserting logs to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		// Insert logs using repository layer (run processing is integrated in the repository)
		_, err := e.logWriter.InsertLogs(ctx, entries, schemaName)
		if err != nil {
			return fmt.Errorf("insert logs (schema=%s): %w", schemaName, err)
		}
	}

	e.logger.Debug("processAndInsertLogs completed successfully",
		zap.Int("total_schemas_processed", len(groups)))

	return nil
}

// LogEventBatch is a batch of log events for efficient event bus publishing
// (You may want to move this to a shared package if used elsewhere)
type LogEventBatch struct {
	Logs []repository.LogEntryResponse `json:"logs"`
}
