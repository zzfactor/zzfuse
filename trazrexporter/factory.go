// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:generate mdatagen metadata.yaml

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exporterhelper/xexporterhelper"
	"go.opentelemetry.io/collector/exporter/xexporter"

	"github.com/medxops/trazr-otel/pkg/config"
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/trazrexporter/internal"
	"github.com/medxops/trazr-otel/trazrexporter/internal/metadata"
	"github.com/medxops/trazr-otel/zzfuseauth"
)

const postgresqlDriverName = "pgx"

// ErrTrazrExtensionNotFound is returned when the trazr extension is not found in Host.GetExtensions()
var ErrTrazrExtensionNotFound = errors.New("trazr extension not found: ensure trazrextension is enabled in your collector config")

// getTrazrServiceProvider finds the TrazrServiceProvider from the collector's extensions.
// In OTel Collector, extensions are started before exporters, so the trazr extension
// should already be running and ready to provide the ServiceResourceManager.
func getTrazrServiceProvider(host component.Host) (repository.TrazrServiceProvider, error) {
	for _, ext := range host.GetExtensions() {
		if provider, ok := ext.(repository.TrazrServiceProvider); ok {
			return provider, nil
		}
	}
	return nil, ErrTrazrExtensionNotFound
}

// NewFactory supports all stable signals and the development profiles signal.
func NewFactory() exporter.Factory {
	return xexporter.NewFactory(metadata.Type, createDefaultConfig,
		xexporter.WithLogs(createLogsExporter, metadata.LogsStability),
		xexporter.WithTraces(createTracesExporter, metadata.SpansStability),
		xexporter.WithMetrics(createMetricsExporter, metadata.MetricsStability),
		xexporter.WithProfiles(createProfilesExporter, metadata.ProfilesStability),
	)
}

func createProfilesExporter(ctx context.Context, set exporter.Settings, cfg component.Config) (xexporter.Profiles, error) {
	c := cfg.(*Config)
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	c.collectorVersion = set.BuildInfo.Version
	exp := newProfilesExporter(set.Logger, c)
	wrapped, err := xexporterhelper.NewProfiles(ctx, set, cfg, exp.pushProfileData,
		exporterhelper.WithStart(exp.start), exporterhelper.WithShutdown(exp.shutdown),
		exporterhelper.WithTimeout(c.TimeoutSettings), exporterhelper.WithQueue(c.QueueSettings), exporterhelper.WithRetry(c.BackOffConfig))
	if err != nil {
		return nil, err
	}
	return &authenticatedProfiles{wrapped, &exp.baseExporter}, nil
}

func createDefaultConfig() component.Config {
	return &Config{
		Authenticator:    component.NewID(zzfuseauth.Type),
		collectorVersion: "unknown",
		driverName:       postgresqlDriverName,

		TimeoutSettings:  exporterhelper.NewDefaultTimeoutConfig(),
		QueueSettings:    configoptional.Some(exporterhelper.NewDefaultQueueConfig()),
		BackOffConfig:    configretry.NewDefaultBackOffConfig(),
		ConnectionParams: map[string]string{},

		// Table configuration defaults
		LogsTableName:  repository.SignalTypeLogs,
		SpansTableName: repository.SignalTypeSpans,

		// Schema and performance defaults
		TTL:                   0,
		PartitionInterval:     config.DefaultPartitionInterval,
		MaxConnections:        20,
		MaxIdleConnections:    10,
		ConnectionMaxLifetime: 30 * time.Minute,
		EnableVectorSearch:    false,
		VectorDimensions:      config.DefaultVectorDimensions,

		// Metrics table defaults - use helper function to generate names
		MetricsTables: func() MetricTablesConfig {
			gauge, sum, summary, histogram, expHistogram := config.MetricTableNames("")
			return MetricTablesConfig{
				Gauge:                internal.MetricTypeConfig{Name: gauge},
				Sum:                  internal.MetricTypeConfig{Name: sum},
				Summary:              internal.MetricTypeConfig{Name: summary},
				Histogram:            internal.MetricTypeConfig{Name: histogram},
				ExponentialHistogram: internal.MetricTypeConfig{Name: expHistogram},
			}
		}(),
	}
}

// createLogsExporter creates a new exporter for logs.
// Logs are directly inserted into PostgreSQL.
func createLogsExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Logs, error) {
	c := cfg.(*Config)

	// Validate configuration
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	c.collectorVersion = set.BuildInfo.Version

	exporter := newLogsExporter(set.Logger, c)

	wrapped, err := exporterhelper.NewLogs(
		ctx,
		set,
		cfg,
		exporter.pushLogsData,
		exporterhelper.WithStart(exporter.start),
		exporterhelper.WithShutdown(exporter.shutdown),
		exporterhelper.WithTimeout(c.TimeoutSettings),
		exporterhelper.WithQueue(c.QueueSettings),
		exporterhelper.WithRetry(c.BackOffConfig),
	)
	if err != nil {
		return nil, err
	}
	return &authenticatedLogs{wrapped, &exporter.baseExporter}, nil
}

// createTracesExporter creates a new exporter for traces.
// Traces are directly inserted into PostgreSQL.
func createTracesExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Traces, error) {
	c := cfg.(*Config)

	// Validate configuration
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	c.collectorVersion = set.BuildInfo.Version
	exporter := newTracesExporter(set.Logger, c)

	wrapped, err := exporterhelper.NewTraces(
		ctx,
		set,
		cfg,
		exporter.pushTraceData,
		exporterhelper.WithStart(exporter.start),
		exporterhelper.WithShutdown(exporter.shutdown),
		exporterhelper.WithTimeout(c.TimeoutSettings),
		exporterhelper.WithQueue(c.QueueSettings),
		exporterhelper.WithRetry(c.BackOffConfig),
	)
	if err != nil {
		return nil, err
	}
	return &authenticatedTraces{wrapped, &exporter.baseExporter}, nil
}

// createMetricsExporter creates a new exporter for metrics.
// Metrics are directly inserted into PostgreSQL.
func createMetricsExporter(
	ctx context.Context,
	set exporter.Settings,
	cfg component.Config,
) (exporter.Metrics, error) {
	c := cfg.(*Config)

	// Validate configuration
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	c.collectorVersion = set.BuildInfo.Version
	exporter := newMetricsExporter(set.Logger, c)

	wrapped, err := exporterhelper.NewMetrics(
		ctx,
		set,
		cfg,
		exporter.pushMetricsData,
		exporterhelper.WithStart(exporter.start),
		exporterhelper.WithShutdown(exporter.shutdown),
		exporterhelper.WithTimeout(c.TimeoutSettings),
		exporterhelper.WithQueue(c.QueueSettings),
		exporterhelper.WithRetry(c.BackOffConfig),
	)
	if err != nil {
		return nil, err
	}
	return &authenticatedMetrics{wrapped, &exporter.baseExporter}, nil
}
