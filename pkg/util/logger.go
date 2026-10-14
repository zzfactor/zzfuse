package util

import (
	"context"
	"log/slog"

	"go.uber.org/zap"
)

// Logger is the project-wide logging interface.
type Logger interface {
	Info(args ...interface{})
	Error(args ...interface{})
	Debug(args ...interface{})
}

// ZapLoggerAdapter implements Logger using a zap.Logger.
type ZapLoggerAdapter struct {
	Zap *zap.Logger
}

func NewZapLoggerAdapter(z *zap.Logger) *ZapLoggerAdapter {
	return &ZapLoggerAdapter{Zap: z}
}

func (l *ZapLoggerAdapter) Info(args ...interface{})  { l.Zap.Sugar().Info(args...) }
func (l *ZapLoggerAdapter) Error(args ...interface{}) { l.Zap.Sugar().Error(args...) }
func (l *ZapLoggerAdapter) Debug(args ...interface{}) { l.Zap.Sugar().Debug(args...) }

// ZapSlogAdapter wraps a zap.Logger to provide a slog.Logger interface.
//
// Usage in a custom OpenTelemetry Collector component:
//   - Use the zap.Logger provided by the Collector (set.Logger) for all logging.
//   - If you need a slog.Logger (for slog-based APIs), wrap it:
//     slogLogger := util.NewZapSlogAdapter(set.Logger)
//   - Do NOT initialize your own OpenTelemetry SDK for logs, spans, or metrics.
//   - For spans and metrics, use the Collector's context, obsreport, and exporterhelper packages.
//   - All self-telemetry should go through the Collector's pipeline, not a custom OTel SDK.
//
// This adapter is only for bridging zap to slog for code that expects slog.Logger.
type ZapSlogAdapter struct {
	zap *zap.Logger
}

func NewZapSlogAdapter(z *zap.Logger) *slog.Logger {
	return slog.New(&ZapSlogAdapter{zap: z})
}

func (a *ZapSlogAdapter) Enabled(_ context.Context, level slog.Level) bool {
	// Map slog levels to zap levels (simplified: always enabled)
	return true
}

func (a *ZapSlogAdapter) Handle(ctx context.Context, r slog.Record) error {
	msg := r.Message
	fields := make([]zap.Field, 0, r.NumAttrs())
	r.Attrs(func(attr slog.Attr) bool {
		fields = append(fields, zap.Any(attr.Key, attr.Value.Any()))
		return true
	})
	// Optionally add trace/span IDs from context if present
	// (Collector's zap logger may already do this)
	a.zap.Info(msg, fields...)
	return nil
}

func (a *ZapSlogAdapter) WithAttrs(attrs []slog.Attr) slog.Handler {
	// Not implemented for simplicity
	return a
}

func (a *ZapSlogAdapter) WithGroup(name string) slog.Handler {
	// Not implemented for simplicity
	return a
}

// NOTE:
// - Do NOT use otel.Tracer(), otel.Meter(), or initialize your own OTel SDK in a Collector component.
// - For custom exporter metrics or spans, use the Collector's obsreport or exporterhelper packages.
// - All logs, spans, and metrics should flow through the Collector's configured pipeline.

// NoopLogger implements Logger as a no-op for testing.
type NoopLogger struct{}

func (NoopLogger) Info(args ...interface{})  {}
func (NoopLogger) Error(args ...interface{}) {}
func (NoopLogger) Debug(args ...interface{}) {}
