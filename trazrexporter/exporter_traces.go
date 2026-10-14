// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/trazrexporter/internal"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type spansExporter struct {
	baseExporter
	traceWriter repository.TraceWriterService
}

// newTracesExporter creates a traces exporter without initializing the SRM.
// The SRM is obtained from the TrazrExtension via Host.GetExtensions() during start().
func newTracesExporter(logger *zap.Logger, cfg *Config) *spansExporter {
	return &spansExporter{
		baseExporter: baseExporter{
			logger:     logger,
			cfg:        cfg,
			signalType: "traces",
		},
	}
}

// start initializes the exporter by obtaining the ServiceResourceManager from the TrazrExtension.
// This is called by the OTel Collector after all extensions have started.
func (e *spansExporter) start(ctx context.Context, host component.Host) error {
	if err := e.initializeSRM(ctx, host); err != nil {
		return err
	}

	// Get the trace writer service
	traceWriter, err := e.srm.GetTraceWriter()
	if err != nil {
		return fmt.Errorf("failed to get trace writer: %w", err)
	}
	e.traceWriter = traceWriter

	e.logger.Info("Traces exporter started successfully")
	return nil
}

// pushTraceData consumes spans data and exports it to PostgreSQL using sequential filtering.
//
// The function implements three-phase sequential filtering:
// 1. Resource filtering with signal type (most restrictive, done once per ResourceSpans)
// 2. Scope filtering (done once per ScopeSpans, only on resource matches)
// 3. Attribute filtering (done per Span, only on scope matches)
//
// This approach maximizes performance through early filtering and supports routing to multiple schemas.
func (e *spansExporter) pushTraceData(ctx context.Context, td ptrace.Traces) error {
	tenantSchema, err := e.tenantSchema(ctx)
	if err != nil {
		return err
	}

	if span := trace.SpanFromContext(ctx); span != nil && span.IsRecording() {
		span.AddEvent("pushTraceData called", trace.WithAttributes(
			attribute.Int("num_resource_spans", td.ResourceSpans().Len()),
		))
	}

	groups := make(map[string][]repository.TraceEntry) // key: schemaName

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		resourceSpan := td.ResourceSpans().At(i)
		res := resourceSpan.Resource()

		serviceName := internal.GetServiceName(res.Attributes()) // Original service name
		schema := tenantSchema

		// Marshal resource attributes once per ResourceSpans (immutable)
		resourceAttrsJSON, _ := json.Marshal(internal.AttributesToMap(res.Attributes()))
		resourceSchemaURL := internal.PtrFromString(resourceSpan.SchemaUrl())

		for j := 0; j < resourceSpan.ScopeSpans().Len(); j++ {
			scopeSpan := resourceSpan.ScopeSpans().At(j)
			scope := scopeSpan.Scope()

			// Marshal scope attributes once per ScopeSpans
			scopeAttrsJSON, _ := json.Marshal(internal.AttributesToMap(scope.Attributes()))
			scopeName := internal.PtrFromString(scope.Name())
			scopeVersion := internal.PtrFromString(scope.Version())
			scopeDroppedAttrCount := internal.PtrFromInt32(int32(scope.DroppedAttributesCount()))
			scopeSchemaURL := internal.PtrFromString(scopeSpan.SchemaUrl())

			for k := 0; k < scopeSpan.Spans().Len(); k++ {
				span := scopeSpan.Spans().At(k)

				timestamp := internal.NormalizeTimestamp(span.StartTimestamp().AsTime())
				traceID := internal.TraceIDToHex(span.TraceID())
				spanID := internal.SpanIDToHex(span.SpanID())
				parentSpanID := internal.SpanIDToHex(span.ParentSpanID())
				spanAttrsJSON, _ := json.Marshal(internal.AttributesToMap(span.Attributes()))

				spanFlags := int32(span.Flags())
				entry := repository.TraceEntry{
					TimestampNS:  timestamp.Time.UnixNano(),
					Timestamp:    timestamp.Time,
					TraceID:      traceID,
					SpanID:       spanID,
					ParentSpanID: internal.PtrFromString(parentSpanID),
					TraceState:   internal.PtrFromString(span.TraceState().AsRaw()),
					Flags:        &spanFlags,
					Name:         span.Name(),
					SpanKind:     span.Kind().String(),
					// Service entity fields (OTel v1.39.0)
					ServiceName:           serviceName, // Original service name
					ServiceNamespace:      internal.GetServiceNamespacePtr(res.Attributes()),
					ServiceInstanceID:     internal.GetServiceInstanceIDPtr(res.Attributes()),
					ResourceAttributes:    resourceAttrsJSON,
					ResourceSchemaURL:     resourceSchemaURL,
					ScopeName:             scopeName,
					ScopeVersion:          scopeVersion,
					ScopeAttributes:       scopeAttrsJSON,
					ScopeSchemaURL:        scopeSchemaURL,
					ScopeDroppedAttrCount: scopeDroppedAttrCount,
					Attributes:            spanAttrsJSON,
					DroppedAttrCount:      internal.PtrFromInt32(int32(span.DroppedAttributesCount())),
					Events:                internal.ConvertEventsToJSONB(span.Events()),
					DroppedEventsCount:    internal.PtrFromInt32(int32(span.DroppedEventsCount())),
					Links:                 internal.ConvertLinksToJSONB(span.Links()),
					DroppedLinksCount:     internal.PtrFromInt32(int32(span.DroppedLinksCount())),
					StatusCode:            internal.PtrFromInt32(int32(span.Status().Code())),
					StatusMessage:         internal.PtrFromString(span.Status().Message()),
					EndTime:               span.EndTimestamp().AsTime(),
					DurationNS:            int64(span.EndTimestamp()) - int64(span.StartTimestamp()),
					Metadata:              []byte("{}"),
				}

				groups[schema] = append(groups[schema], entry)
			}
		}
	}

	// Insert traces to database
	for schemaName, entries := range groups {
		e.logger.Debug("Inserting traces to schema",
			zap.String("schema_name", schemaName),
			zap.Int("entry_count", len(entries)))

		_, err := e.traceWriter.InsertSpans(ctx, entries, schemaName)
		if err != nil {
			return fmt.Errorf("insert spans (schema=%s): %w", schemaName, err)
		}
	}

	e.logger.Debug("pushTraceData completed successfully",
		zap.Int("total_schemas_processed", len(groups)))

	return nil
}
