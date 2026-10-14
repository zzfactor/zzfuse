package trazrprocessor

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
)

const (
	// TypeStr is the unique identifier for this processor type.
	TypeStr = "trazr"
)

// NewFactory returns a new processor factory supporting spans, logs, and metrics.
func NewFactory() processor.Factory {
	return processor.NewFactory(
		component.MustNewType(TypeStr),
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelDevelopment),
		processor.WithLogs(createLogsProcessor, component.StabilityLevelDevelopment),
		processor.WithMetrics(createMetricsProcessor, component.StabilityLevelDevelopment),
	)
}

func createTracesProcessor(
	_ context.Context,
	set processor.Settings,
	_ component.Config,
	next consumer.Traces,
) (processor.Traces, error) {
	return newPassthroughTracesProcessor(set.TelemetrySettings, next), nil
}

func createLogsProcessor(
	_ context.Context,
	set processor.Settings,
	_ component.Config,
	next consumer.Logs,
) (processor.Logs, error) {
	return newPassthroughLogsProcessor(set.TelemetrySettings, next), nil
}

func createMetricsProcessor(
	_ context.Context,
	set processor.Settings,
	_ component.Config,
	next consumer.Metrics,
) (processor.Metrics, error) {
	return newPassthroughMetricsProcessor(set.TelemetrySettings, next), nil
}
