package trazrprocessor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// --- Traces Processor ---
type passthroughTracesProcessor struct {
	next   consumer.Traces
	logger *zap.Logger
	tracer trace.Tracer
	meter  metric.Meter

	acceptedCounter metric.Int64Counter
	refusedCounter  metric.Int64Counter
	latencyRecorder metric.Float64Histogram
}

func newPassthroughTracesProcessor(set component.TelemetrySettings, next consumer.Traces) *passthroughTracesProcessor {
	meter := set.MeterProvider.Meter("processor")
	acceptedCounter, _ := meter.Int64Counter("processor.spans_accepted")
	refusedCounter, _ := meter.Int64Counter("processor.spans_refused")
	latencyRecorder, _ := meter.Float64Histogram("processor.spans_latency_ms")
	return &passthroughTracesProcessor{
		next:            next,
		logger:          set.Logger,
		tracer:          set.TracerProvider.Tracer("processor"),
		meter:           meter,
		acceptedCounter: acceptedCounter,
		refusedCounter:  refusedCounter,
		latencyRecorder: latencyRecorder,
	}
}

func (p *passthroughTracesProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (p *passthroughTracesProcessor) ConsumeTraces(ctx context.Context, td ptrace.Traces) error {
	start := time.Now()
	ctx, span := p.tracer.Start(ctx, "ConsumeTraces")
	defer span.End()

	span.SetAttributes(attribute.Int("span_count", td.SpanCount()))
	p.logger.Info("Processing spans", zap.Int("span_count", td.SpanCount()))

	err := p.next.ConsumeTraces(ctx, td)
	latency := float64(time.Since(start).Milliseconds())
	p.latencyRecorder.Record(ctx, latency)

	if err != nil {
		p.refusedCounter.Add(ctx, int64(td.SpanCount()))
		span.RecordError(err)
		p.logger.Error("Failed to process spans", zap.Error(err))
		return fmt.Errorf("processor: %w", err)
	}

	p.acceptedCounter.Add(ctx, int64(td.SpanCount()))
	return nil
}

func (p *passthroughTracesProcessor) Start(ctx context.Context, host component.Host) error {
	return nil
}
func (p *passthroughTracesProcessor) Shutdown(ctx context.Context) error { return nil }

// --- Logs Processor ---
type passthroughLogsProcessor struct {
	next   consumer.Logs
	logger *zap.Logger
	tracer trace.Tracer
	meter  metric.Meter

	acceptedCounter metric.Int64Counter
	refusedCounter  metric.Int64Counter
	latencyRecorder metric.Float64Histogram

	validator *ValidatorService
}

func newPassthroughLogsProcessor(set component.TelemetrySettings, next consumer.Logs) *passthroughLogsProcessor {
	meter := set.MeterProvider.Meter("processor")
	acceptedCounter, _ := meter.Int64Counter("processor.logs_accepted")
	refusedCounter, _ := meter.Int64Counter("processor.logs_refused")
	latencyRecorder, _ := meter.Float64Histogram("processor.logs_latency_ms")
	validator, err := initValidatorService(set.Logger)
	if err != nil {
		set.Logger.Warn("Failed to initialize validator", zap.Error(err))
		validator = nil
	}
	return &passthroughLogsProcessor{
		next:            next,
		logger:          set.Logger,
		tracer:          set.TracerProvider.Tracer("processor"),
		meter:           meter,
		acceptedCounter: acceptedCounter,
		refusedCounter:  refusedCounter,
		latencyRecorder: latencyRecorder,
		validator:       validator,
	}
}

func (p *passthroughLogsProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: true}
}

// Example: Obfuscate sensitive data in log entries (e.g., email addresses)
// This demonstrates how to scan and replace sensitive fields in each log record.
// In production, use a robust regex or PII/PHI detection library.
func (p *passthroughLogsProcessor) ConsumeLogs(ctx context.Context, ld plog.Logs) error {
	start := time.Now()
	ctx, span := p.tracer.Start(ctx, "ConsumeLogs")
	defer span.End()
	logCount := ld.LogRecordCount()
	span.SetAttributes(attribute.Int("log_count", logCount))
	p.logger.Info("Processing logs", zap.Int("log_count", logCount))
	// Obfuscate emails and redact if '1' is found
	for i := 0; i < ld.ResourceLogs().Len(); i++ {
		rl := ld.ResourceLogs().At(i)
		for j := 0; j < rl.ScopeLogs().Len(); j++ {
			sl := rl.ScopeLogs().At(j)
			for k := 0; k < sl.LogRecords().Len(); k++ {
				rec := sl.LogRecords().At(k)
				if rec.Body().Type() == pcommon.ValueTypeStr {
					body := rec.Body().Str()
					// Validate for sensitive data
					if p.validator != nil {
						req := &SensitiveDataValidationRequest{
							Payload: []byte(body),
							Format:  "json", // or "proto" if you expect proto
						}
						resp, err := p.validator.Validate(ctx, req)
						if err != nil {
							p.logger.Warn("Validator error", zap.Error(err))
						} else if !resp.Valid {
							rec.Body().SetStr("***REDACTED***")
							rec.Attributes().PutStr("validator_reason", resp.Reason)
							p.logger.Debug("Sensitive data redacted by validator", zap.String("reason", resp.Reason))
							continue
						}
					}
					if strings.Contains(body, "1") {
						rec.Body().SetStr("*****************")
						p.logger.Debug("Redacted log body", zap.String("body", "*****************"))
						rec.Attributes().PutInt("audit_it", 123)
						continue
					}
					obfuscated := obfuscateEmail(body)
					rec.Body().SetStr(obfuscated)
					p.logger.Debug("Obfuscated log body", zap.String("body", obfuscated))
				}
			}
		}
	}
	err := p.next.ConsumeLogs(ctx, ld)
	latency := float64(time.Since(start).Milliseconds())
	p.latencyRecorder.Record(ctx, latency)

	if err != nil {
		p.refusedCounter.Add(ctx, int64(logCount))
		span.RecordError(err)
		p.logger.Error("Processing logs failed", zap.Error(err))
		return fmt.Errorf("processor: %w", err)
	}

	p.acceptedCounter.Add(ctx, int64(logCount))
	return nil
}

// obfuscateEmail is a simple example that replaces '@' with '[at]' in strings.
// Replace with a real PII/PHI detection/obfuscation in production.
func obfuscateEmail(s string) string {
	// Very naive: replace all '@' with '[at]'
	return strings.ReplaceAll(s, "@", "[at]")
}

func (p *passthroughLogsProcessor) Start(ctx context.Context, host component.Host) error { return nil }
func (p *passthroughLogsProcessor) Shutdown(ctx context.Context) error                   { return nil }

// --- Metrics Processor ---
type passthroughMetricsProcessor struct {
	next   consumer.Metrics
	logger *zap.Logger
	tracer trace.Tracer
	meter  metric.Meter

	acceptedCounter metric.Int64Counter
	refusedCounter  metric.Int64Counter
	latencyRecorder metric.Float64Histogram
}

func newPassthroughMetricsProcessor(set component.TelemetrySettings, next consumer.Metrics) *passthroughMetricsProcessor {
	meter := set.MeterProvider.Meter("processor")
	acceptedCounter, _ := meter.Int64Counter("processor.metrics_accepted")
	refusedCounter, _ := meter.Int64Counter("processor.metrics_refused")
	latencyRecorder, _ := meter.Float64Histogram("processor.metrics_latency_ms")
	return &passthroughMetricsProcessor{
		next:            next,
		logger:          set.Logger,
		tracer:          set.TracerProvider.Tracer("processor"),
		meter:           meter,
		acceptedCounter: acceptedCounter,
		refusedCounter:  refusedCounter,
		latencyRecorder: latencyRecorder,
	}
}

func (p *passthroughMetricsProcessor) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

func (p *passthroughMetricsProcessor) ConsumeMetrics(ctx context.Context, md pmetric.Metrics) error {
	start := time.Now()
	ctx, span := p.tracer.Start(ctx, "ConsumeMetrics")
	defer span.End()
	metricCount := md.MetricCount()
	span.SetAttributes(attribute.Int("metric_count", metricCount))
	p.logger.Info("Processing metrics", zap.Int("metric_count", metricCount))

	err := p.next.ConsumeMetrics(ctx, md)
	latency := float64(time.Since(start).Milliseconds())
	p.latencyRecorder.Record(ctx, latency)

	if err != nil {
		p.refusedCounter.Add(ctx, int64(metricCount))
		span.RecordError(err)
		p.logger.Error("Processing metrics failed", zap.Error(err))
		return fmt.Errorf("processor: %w", err)
	}

	p.acceptedCounter.Add(ctx, int64(metricCount))
	return nil
}

func (p *passthroughMetricsProcessor) Start(ctx context.Context, host component.Host) error {
	return nil
}
func (p *passthroughMetricsProcessor) Shutdown(ctx context.Context) error { return nil }
