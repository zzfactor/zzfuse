package trazrprocessor

import (
	"context"
	"testing"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"
)

func TestNewFactory_Registration(t *testing.T) {
	t.Parallel()
	f := NewFactory()
	if f.Type() != component.MustNewType(TypeStr) {
		t.Errorf("unexpected factory type: got %v, want %v", f.Type(), TypeStr)
	}

	cfg := f.CreateDefaultConfig()
	if _, ok := cfg.(*Config); !ok {
		t.Errorf("CreateDefaultConfig did not return *Config, got %T", cfg)
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := createDefaultConfig()
	if cfg == nil {
		t.Fatal("createDefaultConfig returned nil")
	}
	if _, ok := cfg.(*Config); !ok {
		t.Errorf("createDefaultConfig did not return *Config, got %T", cfg)
	}
}

func testProcessorSettings() processor.Settings {
	return processor.Settings{
		ID: component.NewID(component.MustNewType(TypeStr)),
		TelemetrySettings: component.TelemetrySettings{
			Logger:         zap.NewNop(),
			MeterProvider:  metricnoop.NewMeterProvider(),
			TracerProvider: tracenoop.NewTracerProvider(),
		},
	}
}

type testTracesConsumer struct{}

func (testTracesConsumer) Capabilities() consumer.Capabilities                { return consumer.Capabilities{} }
func (testTracesConsumer) ConsumeTraces(context.Context, ptrace.Traces) error { return nil }

type testLogsConsumer struct{}

func (testLogsConsumer) Capabilities() consumer.Capabilities          { return consumer.Capabilities{} }
func (testLogsConsumer) ConsumeLogs(context.Context, plog.Logs) error { return nil }

type testMetricsConsumer struct{}

func (testMetricsConsumer) Capabilities() consumer.Capabilities                   { return consumer.Capabilities{} }
func (testMetricsConsumer) ConsumeMetrics(context.Context, pmetric.Metrics) error { return nil }

func TestCreateTracesProcessor(t *testing.T) {
	t.Parallel()
	p, err := createTracesProcessor(context.Background(), testProcessorSettings(), &Config{}, testTracesConsumer{})
	if err != nil {
		t.Fatalf("createTracesProcessor returned error: %v", err)
	}
	if p == nil {
		t.Fatal("createTracesProcessor returned nil processor")
	}
}

func TestCreateLogsProcessor(t *testing.T) {
	t.Parallel()
	p, err := createLogsProcessor(context.Background(), testProcessorSettings(), &Config{}, testLogsConsumer{})
	if err != nil {
		t.Fatalf("createLogsProcessor returned error: %v", err)
	}
	if p == nil {
		t.Fatal("createLogsProcessor returned nil processor")
	}
}

func TestCreateMetricsProcessor(t *testing.T) {
	t.Parallel()
	p, err := createMetricsProcessor(context.Background(), testProcessorSettings(), &Config{}, testMetricsConsumer{})
	if err != nil {
		t.Fatalf("createMetricsProcessor returned error: %v", err)
	}
	if p == nil {
		t.Fatal("createMetricsProcessor returned nil processor")
	}
}
