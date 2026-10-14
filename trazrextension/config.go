// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package trazrextension // import "github.com/medxops/trazr-otel/trazrextension"

import (
	"fmt"
	"net"
	"strconv"
	"time"
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
	// Endpoint is the HTTP/WebSocket listen address; empty uses :18888.
	Endpoint string `mapstructure:"endpoint"`
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

	// WebSocket settings (optional). Zero values apply sensible defaults.
	// WSReadTimeout sets the maximum duration for reading the next message.
	WSReadTimeout time.Duration `mapstructure:"ws_read_timeout"`
	// WSWriteTimeout sets the maximum duration for writing a message (used by handlers where applicable).
	WSWriteTimeout time.Duration `mapstructure:"ws_write_timeout"`
	// WSPingInterval, if > 0, enables periodic server-initiated pings at the given interval.
	WSPingInterval time.Duration `mapstructure:"ws_ping_interval"`
	// WSReadLimit caps the maximum size of a single incoming message. Defaults to ~1MB if zero.
	WSReadLimit int64 `mapstructure:"ws_read_limit"`
	// WSDebug enables verbose debug logging for WS control frames and errors.
	WSDebug bool `mapstructure:"ws_debug"`

	// AllowedOrigins restricts CORS and WebSocket origins to the specified list.
	// If empty or contains "*", all origins are allowed (development mode).
	AllowedOrigins []string `mapstructure:"allowed_origins"`
}

// Validate checks the extension configuration for invalid values.
func (cfg *Config) Validate() error {
	if cfg.Endpoint != "" {
		_, port, err := net.SplitHostPort(cfg.Endpoint)
		if err != nil {
			return fmt.Errorf("invalid endpoint %q: %w", cfg.Endpoint, err)
		}
		n, err := strconv.Atoi(port)
		if err != nil || n < 0 || n > 65535 {
			return fmt.Errorf("endpoint port must be between 0 and 65535, got %q", port)
		}
	}
	if cfg.WSReadTimeout < 0 {
		return fmt.Errorf("ws_read_timeout must be >= 0, got %s", cfg.WSReadTimeout)
	}
	if cfg.WSReadLimit < 0 {
		return fmt.Errorf("ws_read_limit must be >= 0, got %d", cfg.WSReadLimit)
	}
	if cfg.MaxConnections < 0 {
		return fmt.Errorf("max_connections must be >= 0, got %d", cfg.MaxConnections)
	}
	return nil
}
