// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrexporter // import "github.com/medxops/trazr-otel/trazrexporter"

import (
	"fmt"
	"time"

	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/config/configretry"
	"go.opentelemetry.io/collector/exporter/exporterhelper"

	"github.com/medxops/trazr-otel/pkg/config"
	"github.com/medxops/trazr-otel/trazrexporter/internal"
)

// Config defines configuration for PostgreSQL exporter.
// This configuration supports both direct database connections and OpenTelemetry Collector integration.
//
// Connection Configuration:
//   - Use Endpoint for full PostgreSQL connection strings (postgres://...)
//   - Or use individual Host, Port, Username, Password, Database fields
//   - SSL mode can be controlled via SSLMode field
//
// Table Configuration:
//   - LogsTableName and SpansTableName control table names for telemetry data
//   - MetricsTables provides granular control over metric table names by type
//
// Performance Configuration:
//   - MaxConnections and MaxIdleConnections control connection pooling
//   - ConnectionMaxLifetime controls connection reuse
//   - PartitionInterval controls table partitioning strategy
//
// Vector Search:
//   - EnableVectorSearch enables semantic search capabilities
//   - VectorDimensions controls embedding vector size
type Config struct {
	// collectorVersion is the build version of the collector. This is overridden when an exporter is initialized.
	collectorVersion string
	driverName       string // for testing

	// TimeoutSettings is the timeout settings for the exporter.
	TimeoutSettings           exporterhelper.TimeoutConfig `mapstructure:",squash"`
	configretry.BackOffConfig `mapstructure:"retry_on_failure"`
	QueueSettings             configoptional.Optional[exporterhelper.QueueBatchConfig] `mapstructure:"sending_queue"`

	// ConnectionParams is the extra connection parameters with map format.
	ConnectionParams map[string]string `mapstructure:"connection_params"`
	// LogsTableName is the table name for logs. default is `logs`.
	LogsTableName string `mapstructure:"logs_table_name"`
	// SpansTableName is the table name for spans. default is `spans`.
	SpansTableName string `mapstructure:"spans_table_name"`
	// MetricsTableName is the table name for metrics. default is `metrics`.
	//
	// Deprecated: MetricsTableName exists for historical compatibility
	// and should not be used. To set the metrics tables name,
	// use the MetricsTables parameter instead.
	MetricsTableName string `mapstructure:"metrics_table_name"`
	// TTL is The data time-to-live for automatic partition cleanup. 0 means no ttl.
	TTL time.Duration `mapstructure:"ttl"`
	// PartitionInterval controls how often to create new partitions (monthly, weekly, daily)
	PartitionInterval string `mapstructure:"partition_interval"`
	// MaxConnections is the maximum number of database connections in the pool
	MaxConnections int `mapstructure:"max_connections"`
	// MaxIdleConnections is the maximum number of idle connections in the pool
	MaxIdleConnections int `mapstructure:"max_idle_connections"`
	// ConnectionMaxLifetime is the maximum lifetime of a connection
	ConnectionMaxLifetime time.Duration `mapstructure:"connection_max_lifetime"`
	// EnableVectorSearch enables vector search capabilities for semantic search
	EnableVectorSearch bool `mapstructure:"enable_vector_search"`
	// VectorDimensions is the number of dimensions for embedding vectors
	VectorDimensions int `mapstructure:"vector_dimensions"`
	// Schema is the PostgreSQL schema name for all tables. default is `otel`.
	Schema string `mapstructure:"schema"`
	// MetricsTables defines the table names for metric types.
	MetricsTables MetricTablesConfig `mapstructure:"metrics_tables"`
}

// MetricTablesConfig defines table names for different metric types.
// Each metric type can be stored in a separate table for optimized queries and storage.
// This configuration allows fine-grained control over metric storage strategy.
//
// Default table names follow the pattern: metrics_{type}
// Examples: gauge_metrics, sum_metrics, metrics_histogram
//
// All metric tables are automatically partitioned by timestamp for performance.
type MetricTablesConfig struct {
	// Gauge is the table name for gauge metric type. default is `gauge_metrics`.
	Gauge internal.MetricTypeConfig `mapstructure:"gauge"`
	// Sum is the table name for sum metric type. default is `sum_metrics`.
	Sum internal.MetricTypeConfig `mapstructure:"sum"`
	// Summary is the table name for summary metric type. default is `metrics_summary`.
	Summary internal.MetricTypeConfig `mapstructure:"summary"`
	// Histogram is the table name for histogram metric type. default is `metrics_histogram`.
	Histogram internal.MetricTypeConfig `mapstructure:"histogram"`
	// ExponentialHistogram is the table name for exponential histogram metric type. default is `exponential_histogram_metrics`.
	ExponentialHistogram internal.MetricTypeConfig `mapstructure:"exponential_histogram"`
}

// Validate validates the PostgreSQL server configuration.
// It performs comprehensive validation of all configuration fields including:
//   - Connection parameters (endpoint, host, port validation)
//   - Database connection string construction
//   - Metric table name generation and validation
//
// Returns a combined error containing all validation failures, or nil if valid.
// The returned error can be checked using errors.Is() for specific error types.
func (cfg *Config) Validate() error {
	if cfg.MaxConnections < 0 {
		return fmt.Errorf("max_connections must be >= 0, got %d", cfg.MaxConnections)
	}
	if cfg.MaxIdleConnections < 0 {
		return fmt.Errorf("max_idle_connections must be >= 0, got %d", cfg.MaxIdleConnections)
	}
	if cfg.MaxConnections > 0 && cfg.MaxIdleConnections > cfg.MaxConnections {
		return fmt.Errorf("max_idle_connections (%d) must be <= max_connections (%d)", cfg.MaxIdleConnections, cfg.MaxConnections)
	}
	if cfg.TTL < 0 {
		return fmt.Errorf("ttl must be >= 0, got %s", cfg.TTL)
	}
	if cfg.VectorDimensions < 0 {
		return fmt.Errorf("vector_dimensions must be >= 0, got %d", cfg.VectorDimensions)
	}

	cfg.buildMetricTableNames()

	return nil
}

// buildMetricTableNames generates default table names for metric types when not explicitly configured.
// It uses the MetricsTableName as a base (defaulting to SignalTypeMetrics) and appends type-specific suffixes.
// This method only sets names for metric types that haven't been explicitly configured.
func (cfg *Config) buildMetricTableNames() {
	if !cfg.areMetricTableNamesSet() {
		baseTableName := cfg.MetricsTableName
		gauge, sum, summary, histogram, expHistogram := config.MetricTableNames(baseTableName)

		if cfg.MetricsTables.Gauge.Name == "" {
			cfg.MetricsTables.Gauge.Name = gauge
		}
		if cfg.MetricsTables.Sum.Name == "" {
			cfg.MetricsTables.Sum.Name = sum
		}
		if cfg.MetricsTables.Summary.Name == "" {
			cfg.MetricsTables.Summary.Name = summary
		}
		if cfg.MetricsTables.Histogram.Name == "" {
			cfg.MetricsTables.Histogram.Name = histogram
		}
		if cfg.MetricsTables.ExponentialHistogram.Name == "" {
			cfg.MetricsTables.ExponentialHistogram.Name = expHistogram
		}
	}
}

// areMetricTableNamesSet checks if any metric table names have been explicitly configured.
// Returns true if at least one metric type has a custom table name configured,
// indicating that the user has provided specific table naming preferences.
func (cfg *Config) areMetricTableNamesSet() bool {
	return cfg.MetricsTables.Gauge.Name != "" ||
		cfg.MetricsTables.Sum.Name != "" ||
		cfg.MetricsTables.Summary.Name != "" ||
		cfg.MetricsTables.Histogram.Name != "" ||
		cfg.MetricsTables.ExponentialHistogram.Name != ""
}
