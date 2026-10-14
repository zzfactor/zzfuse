// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package config provides shared configuration constants and types
// used across trazrexporter and trazrextension modules.
package config

import (
	"time"

	"github.com/medxops/trazr-otel/pkg/repository"
)

// Default table name constants for telemetry data storage
const (
	// DefaultMetricTableName is the base table name for metrics
	DefaultMetricTableName = repository.SignalTypeMetrics

	// Metric type suffixes appended to the base table name
	DefaultGaugeSuffix        = "_gauge"
	DefaultSumSuffix          = "_sum"
	DefaultSummarySuffix      = "_summary"
	DefaultHistogramSuffix    = "_histogram"
	DefaultExpHistogramSuffix = "_exponential_histogram"
)

// Default partition and vector configuration
const (
	// DefaultPartitionInterval controls table partitioning frequency
	DefaultPartitionInterval = "monthly"

	// DefaultVectorDimensions is the default embedding vector size (OpenAI ada-002 compatible)
	DefaultVectorDimensions = 1536
)

// Default connection pool configuration
const (
	// DefaultMaxConnections is the maximum number of database connections
	DefaultMaxConnections = 10

	// DefaultMaxIdleConnections is the maximum idle connections to keep open
	DefaultMaxIdleConnections = 5

	// DefaultConnectionMaxLifetime is the default maximum lifetime of a connection
	DefaultConnectionMaxLifetime = time.Hour
)

// Default schema configuration
const (
	// DefaultSchema is the PostgreSQL schema name for all tables
	DefaultSchema = "otel"
)

// DatabaseConfig contains common database-related configuration fields
// that are shared between exporter and extension modules.
type DatabaseConfig struct {
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
}

// ApplyDefaults sets default values for any unset fields in DatabaseConfig.
func (c *DatabaseConfig) ApplyDefaults() {
	if c.PartitionInterval == "" {
		c.PartitionInterval = DefaultPartitionInterval
	}
	if c.MaxConnections == 0 {
		c.MaxConnections = DefaultMaxConnections
	}
	if c.MaxIdleConnections == 0 {
		c.MaxIdleConnections = DefaultMaxIdleConnections
	}
	if c.VectorDimensions == 0 {
		c.VectorDimensions = DefaultVectorDimensions
	}
	if c.ConnectionMaxLifetime == 0 {
		c.ConnectionMaxLifetime = DefaultConnectionMaxLifetime
	}
}

// MetricTableNames generates the full table names for each metric type.
// It uses baseTableName (defaulting to DefaultMetricTableName) with type-specific suffixes.
func MetricTableNames(baseTableName string) (gauge, sum, summary, histogram, expHistogram string) {
	if baseTableName == "" {
		baseTableName = DefaultMetricTableName
	}
	return baseTableName + DefaultGaugeSuffix,
		baseTableName + DefaultSumSuffix,
		baseTableName + DefaultSummarySuffix,
		baseTableName + DefaultHistogramSuffix,
		baseTableName + DefaultExpHistogramSuffix
}
