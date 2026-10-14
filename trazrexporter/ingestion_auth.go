package trazrexporter

import (
	"context"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/xexporter"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Verify before enqueueing as well as on delivery. A misconfigured receiver
// cannot bypass authentication or fill the durable queue with unauthenticated data.
type authenticatedLogs struct {
	exporter.Logs
	base *baseExporter
}

func (e *authenticatedLogs) ConsumeLogs(ctx context.Context, data plog.Logs) error {
	if _, err := e.base.tenantIngressSchema(ctx); err != nil {
		return err
	}
	return e.Logs.ConsumeLogs(ctx, data)
}

type authenticatedTraces struct {
	exporter.Traces
	base *baseExporter
}

func (e *authenticatedTraces) ConsumeTraces(ctx context.Context, data ptrace.Traces) error {
	if _, err := e.base.tenantIngressSchema(ctx); err != nil {
		return err
	}
	return e.Traces.ConsumeTraces(ctx, data)
}

type authenticatedMetrics struct {
	exporter.Metrics
	base *baseExporter
}

func (e *authenticatedMetrics) ConsumeMetrics(ctx context.Context, data pmetric.Metrics) error {
	if _, err := e.base.tenantIngressSchema(ctx); err != nil {
		return err
	}
	return e.Metrics.ConsumeMetrics(ctx, data)
}

type authenticatedProfiles struct {
	xexporter.Profiles
	base *baseExporter
}

func (e *authenticatedProfiles) ConsumeProfiles(ctx context.Context, data pprofile.Profiles) error {
	if _, err := e.base.tenantIngressSchema(ctx); err != nil {
		return err
	}
	return e.Profiles.ConsumeProfiles(ctx, data)
}
