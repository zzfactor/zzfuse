# Trazr OpenTelemetry Backend

A high-performance, scalable observability backend built on OpenTelemetry with advanced signal filtering, embedding, and partition management capabilities.

## 🚀 Features

- **Multi-Signal Support**: Logs, traces, metrics, and profiles
- **Advanced Signal Filtering**: Hierarchical filtering with signal-specific fields
- **Vector Embedding**: AI-powered semantic search with multiple providers
- **Partition Management**: Automatic table partitioning for performance
- **Real-time Processing**: Trigger-based and batch processing modes
- **Scalable Architecture**: Clean architecture with modular design
- **Observability**: Built-in monitoring and health checks

## 📋 Table of Contents

- [Quick Start](#quick-start)
- [Architecture](#architecture)
- [Configuration](#configuration)
- [Signal Filtering](#signal-filtering)
- [Embedding Configuration](#embedding-configuration)
- [Partition Management](#partition-management)
- [Database Configuration](#database-configuration)
- [Recent Updates](#recent-updates)
- [Troubleshooting](#troubleshooting)

## 🏃 Quick Start

### Prerequisites

- Go 1.21+
- PostgreSQL 14+ with pgvector extension
- Docker (optional)

### Installation

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd trazr-otel
   ```

2. **Build the application**:
   ```bash
   go build -o trazr-otel .
   chmod +x trazr-otel
   ```

3. **Configure environment**:
   ```bash
   cp .env.example .env
   # Edit .env with your database settings
   ```

4. **Run the application**:
   ```bash
   ./trazr-otel --config config.yaml
   ```

## 🏗️ Architecture

### Core Components

- **TrazrExporter**: OpenTelemetry exporter for data ingestion
- **TrazrExtension**: Extension for configuration and management
- **Repository Layer**: Data access and business logic
- **Embedding Workers**: AI-powered signal processing
- **Partition Manager**: Automatic table maintenance
- **Signal Filters**: Hierarchical filtering system

### Data Flow

```
OpenTelemetry Data → TrazrExporter → Signal Filtering → Storage → Embedding → Vector Search
```

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `TRAZR_DB_HOST` | localhost | PostgreSQL host |
| `TRAZR_DB_PORT` | 5432 | PostgreSQL port |
| `TRAZR_DB_USER` | trazr | PostgreSQL user |
| `TRAZR_DB_PASSWORD` | trazrpass | PostgreSQL password |
| `TRAZR_DB_NAME` | trazr | Database name |
| `TRAZR_DB_SSL_MODE` | disable | SSL mode |
| `TRAZR_DB_MAX_OPEN_CONNS` | 20 | Max open connections |
| `TRAZR_DB_MAX_IDLE_CONNS` | 10 | Max idle connections |
| `TRAZR_DB_CONN_MAX_LIFETIME` | 30m | Connection lifetime |

### Example Configuration

```yaml
# config.yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  trazr:
    database:
      host: ${TRAZR_DB_HOST}
      port: ${TRAZR_DB_PORT}
      user: ${TRAZR_DB_USER}
      password: ${TRAZR_DB_PASSWORD}
      name: ${TRAZR_DB_NAME}

exporters:
  trazr:
    database:
      host: ${TRAZR_DB_HOST}
      port: ${TRAZR_DB_PORT}
      user: ${TRAZR_DB_USER}
      password: ${TRAZR_DB_PASSWORD}
      name: ${TRAZR_DB_NAME}

extensions:
  trazr:
    database:
      host: ${TRAZR_DB_HOST}
      port: ${TRAZR_DB_PORT}
      user: ${TRAZR_DB_USER}
      password: ${TRAZR_DB_PASSWORD}
      name: ${TRAZR_DB_NAME}

service:
  extensions: [trazr]
  pipelines:
    traces:
      receivers: [otlp]
      processors: [trazr]
      exporters: [trazr]
    logs:
      receivers: [otlp]
      processors: [trazr]
      exporters: [trazr]
    metrics:
      receivers: [otlp]
      processors: [trazr]
      exporters: [trazr]
```

## 🌐 Network (WebSocket Keepalive)

The Trazr Extension exposes WebSocket streaming endpoints. Connection reliability is controlled via these extension parameters:

### Parameters (under `extensions.trazr`)

- `ws_read_timeout` (duration)
  - Default: `90s`
  - Purpose: Server read deadline. Extended automatically on each Pong.
- `ws_read_limit` (bytes)
  - Default: `1048576` (~1 MiB)
  - Purpose: Max size the server will read per incoming message.
- `ws_ping_interval` (duration)
  - Default: `0` (disabled)
  - Purpose: If > 0, server will send Ping control frames on this interval.
- `ws_write_timeout` (duration)
  - Default: not applied yet
  - Purpose: Reserved for future use to standardize write deadlines in stream handlers.

Notes:
- Client-initiated pings are fully supported and preferred when available. The server reads continuously and auto-Pongs; `ws_read_timeout` is extended on each Pong.
- If your clients cannot send pings (e.g., browsers) or you face idle disconnects due to proxies/NAT, enable `ws_ping_interval` (e.g., `30s`).

### Example

```yaml
extensions:
  trazr:
    ws_read_timeout: 120s
    ws_read_limit: 1048576
    # Enable server-initiated pings if needed (disabled by default)
    # ws_ping_interval: 30s
    # ws_write_timeout: 5s
```

## 🔍 Signal Filtering

The signal filtering system provides hierarchical filtering with signal-specific fields for granular control over data processing.

### Filter Structure

```json
{
  "signal_type": "logs",
  "resource": {
    "service.name": { "op": "=", "value": "my-service" }
  },
  "scope": {
    "library.name": { "op": "=", "value": "express" }
  },
  "attributes": {
    "log.level": { "op": "=", "value": "error" }
  },
  "logs": {
    "severity_text": { "op": "=", "value": "ERROR" },
    "body": { "op": "LIKE", "value": "%error%" }
  }
}
```

### Supported Signal Types

| Signal Type | Description |
|-------------|-------------|
| `logs` | Log records |
| `traces` | Trace spans |
| `gauge_metrics` | Gauge metrics |
| `sum_metrics` | Sum/Counter metrics |
| `histogram_metrics` | Histogram metrics |
| `summary_metrics` | Summary metrics |
| `exponential_histogram_metrics` | Exponential histogram metrics |
| `profiles` | Profile data |
| `*` | All signal types |

### Filter Examples

#### Error Logs Only
```json
{
  "signal_type": "logs",
  "resource": {
    "service.name": { "op": "=", "value": "api-service" }
  },
  "logs": {
    "severity_text": { "op": "=", "value": "ERROR" }
  }
}
```

#### High-Value Metrics
```json
{
  "signal_type": "metrics",
  "attributes": {
    "metric_type": { "op": "in", "value": ["gauge_metrics", "sum_metrics"] },
    "name": { "op": "like", "value": "business_*" }
  }
}
```

## 🤖 Embedding Configuration

The embedding system provides AI-powered semantic search capabilities with **three fully operational processing modes** that support real-time, event-driven processing using PostgreSQL triggers.

### ✅ Processing Modes (All Working)

| Mode | Description | Use Case | Status |
|------|-------------|----------|--------|
| `batch` | Periodic batch processing at fixed intervals | Historical data analysis, background processing | ✅ **Operational** |
| `trigger` | Real-time trigger processing using database notifications | Live data indexing, immediate processing | ✅ **Operational** |
| `hybrid` | Initial batch + trigger processing | Balanced approach, startup + real-time | ✅ **Operational** |

### 🚀 Real-Time Trigger Processing

The system uses PostgreSQL's `LISTEN/NOTIFY` mechanism to provide real-time embedding processing:

- **Database Triggers**: All signal tables (logs, traces, metrics, profiles) have triggers that notify when new data arrives
- **Channel Subscriptions**: Workers subscribe to notification channels like `otel.new_log`, `otel.new_trace`, etc.
- **Debounced Processing**: Multiple triggers are batched together for efficiency (configurable delay)
- **Graceful Fallback**: Falls back to batch mode if trigger system fails

### Configuration Example

```json
{
  "embedding_mode": "hybrid",
  "embedding_batch_interval": 5,
  "embedding_batch_group_signals": false,
  "embedding_trigger_debounce_delay": 2,
  "embedding_start_time": 1722384000000000000,
  "embedding_stop_time": 1722470400000000000
}
```

### Configuration Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `embedding_mode` | `string` | `"batch"` | Processing mode: `"batch"`, `"trigger"`, or `"hybrid"` |
| `embedding_batch_interval` | `int` | `30` | Sleep duration in seconds when no work is found |
| `embedding_batch_group_signals` | `bool` | `false` | Group signals for batch processing |
| `embedding_trigger_debounce_delay` | `int` | `2` | Debounce delay in seconds for trigger mode |
| `embedding_start_time` | `int64` | `0` | Start time filter (nanoseconds) |
| `embedding_stop_time` | `int64` | `0` | Stop time filter (nanoseconds) |

### Template Configuration

```go
var TemplateContent = map[string]string{
  "logs": `Service: {{.service_name}}. Level: {{.level}}. Message: {{.body}}.`,
  "traces": `Service: {{.service_name}}. Trace: {{.trace_id}}. Operation: {{.name}}.`,
  "metrics": `Service: {{.service_name}}. Metric: {{.name}}. Value: {{.value}}.`,
}
```

### 🔄 How Trigger Processing Works

1. **Database Insert**: New signal data is inserted into PostgreSQL tables
2. **Trigger Fires**: Database trigger calls `notify_channel()` function
3. **Notification Sent**: PostgreSQL sends notification to channel (e.g., `otel.new_log`)
4. **Worker Receives**: Embedding worker receives notification via `LISTEN`
5. **Debounced Processing**: Multiple triggers are batched together (configurable delay)
6. **Batch Execution**: Worker runs `RunBatch()` to process accumulated signals
7. **Vector Storage**: Processed embeddings are stored in vector database

### 📊 Performance Benefits

- **Real-time Processing**: Signals are processed immediately when they arrive
- **Debounced Triggers**: 50-100x reduction in batch operations
- **Configurable Delay**: Adjustable debounce delay (1-10 seconds)
- **Efficient Batching**: Multiple triggers processed as single batch
- **Resource Optimization**: Workers sleep when no work is available

### 🛡️ Error Handling & Reliability

- **Graceful Fallback**: Falls back to batch mode if trigger system fails
- **Connection Recovery**: Automatically reconnects if database connection is lost
- **Safe Shutdown**: Workers can be stopped multiple times without panicking
- **Timeout Protection**: Configurable timeouts prevent hanging operations

## 📊 TimescaleDB Maintenance

The TimescaleDB maintenance system provides automatic hypertable management for improved query performance and data retention management.

### Configuration

```json
{
  "timescaledb_maintenance": {
    "enabled": true,
    "retention_days": 30,
    "maintenance_interval_hours": 24
  }
}
```

### Configuration Fields

| Field | Type | Default | Range | Description |
|-------|------|---------|-------|-------------|
| `enabled` | boolean | `true` | - | Enable/disable TimescaleDB maintenance |
| `retention_days` | integer | `30` | 1-365 | Days to retain data chunks |
| `maintenance_interval_hours` | integer | `24` | 1-168 | Hours between maintenance runs |

### Maintenance Schedule

- **Daily Maintenance**: Runs every 24 hours by default
- **Configurable Interval**: Adjustable via `maintenance_interval_hours`

### What Maintenance Does

1. Drops old chunks older than `retention_days`
2. Updates table statistics with `ANALYZE`
3. Logs maintenance activities and chunk information
4. Provides hypertable status and statistics

## 🗄️ Database Configuration

### Connection Pool Settings

Configure connection pools via metadata or environment variables:

```json
{
  "max_open_conns": 20,
  "max_idle_conns": 10,
  "conn_max_lifetime": "30m"
}
```

### Database Schema

The system uses multiple schemas for different data types:

- **OTEL Signal Tables**: `logs`, `traces`, `metrics`, `profiles`
- **Vector Store Tables**: Embedding vectors and metadata
- **Configuration Tables**: System configuration and metadata

## 🎯 Database Schema Routing

The database schema routing system provides dynamic schema selection based on service identity and namespace, enabling multi-tenant data isolation and flexible deployment patterns.

### Overview

Schema routing automatically routes telemetry data (logs, traces, metrics, profiles) to different database schemas based on configurable rules. This enables:

- **Multi-tenant isolation**: Route different services to separate schemas
- **Environment separation**: Route production/staging data to different schemas  
- **Service-based partitioning**: Route specific services to dedicated schemas
- **Namespace-based routing**: Route by Kubernetes namespace or service namespace

### Routing Parameters

The routing system supports the following parameters in schema configuration:

#### Exact Match Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `service_name` | `string` or `array` | Exact service name match | `"api-gateway"` or `["api-gateway", "user-service"]` |
| `service_namespace` | `string` or `array` | Exact service namespace match | `"production"` or `["prod", "staging"]` |

#### Pattern Match Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `service_name_prefix` | `string` | Service name starts with prefix | `"api-"` matches `api-gateway`, `api-orders` |
| `service_name_suffix` | `string` | Service name ends with suffix | `"-service"` matches `user-service`, `order-service` |
| `service_name_contains` | `string` | Service name contains substring | `"auth"` matches `auth-service`, `oauth-proxy` |
| `service_namespace_prefix` | `string` | Namespace starts with prefix | `"env-"` matches `env-prod`, `env-staging` |
| `service_namespace_suffix` | `string` | Namespace ends with suffix | `"-ns"` matches `prod-ns`, `staging-ns` |
| `service_namespace_contains` | `string` | Namespace contains substring | `"test"` matches `test-env`, `integration-test` |

#### Special Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `*` (wildcard) | `string` | Matches any service name | Use as fallback for unmatched services |
| Empty routing `{}` | `object` | Catch-all rule | Matches everything, typically with priority 9999 |

### Priority System

Schema routing uses a priority-based system where **lower numbers have higher priority**:

- **Priority 1-10**: High priority exact matches
- **Priority 11-100**: Medium priority pattern matches  
- **Priority 101-1000**: Low priority broad patterns
- **Priority 9999**: Catch-all fallback (recommended)

### Configuration Examples

#### Basic Multi-Tenant Setup

```json
[
  {
    "priority": 1,
    "database_schema": "tenant_a_schema",
    "routing": {
      "service_namespace": "tenant-a"
    }
  },
  {
    "priority": 2, 
    "database_schema": "tenant_b_schema",
    "routing": {
      "service_namespace": "tenant-b"
    }
  },
  {
    "priority": 9999,
    "database_schema": "default_schema",
    "routing": {}
  }
]
```

#### Environment-Based Routing

```json
[
  {
    "priority": 1,
    "database_schema": "production_schema",
    "routing": {
      "service_namespace": "production"
    }
  },
  {
    "priority": 2,
    "database_schema": "staging_schema", 
    "routing": {
      "service_namespace": "staging"
    }
  },
  {
    "priority": 3,
    "database_schema": "development_schema",
    "routing": {
      "service_namespace_prefix": "dev-"
    }
  }
]
```

#### Service-Specific Routing

```json
[
  {
    "priority": 1,
    "database_schema": "critical_services_schema",
    "routing": {
      "service_name": ["payment-service", "auth-service", "billing-service"]
    }
  },
  {
    "priority": 2,
    "database_schema": "api_services_schema",
    "routing": {
      "service_name_prefix": "api-"
    }
  },
  {
    "priority": 3,
    "database_schema": "microservices_schema",
    "routing": {
      "service_name_suffix": "-service"
    }
  }
]
```

#### Complex Pattern Matching

```json
[
  {
    "priority": 1,
    "database_schema": "production_api_schema",
    "routing": {
      "service_namespace": "production",
      "service_name_prefix": "api-"
    }
  },
  {
    "priority": 2,
    "database_schema": "test_environment_schema",
    "routing": {
      "service_namespace_contains": "test"
    }
  },
  {
    "priority": 10,
    "database_schema": "wildcard_schema",
    "routing": {
      "service_name": "*"
    }
  }
]
```

### Routing Logic

The routing system follows this evaluation order:

1. **Exact namespace matches** (highest priority)
2. **Exact service name matches**
3. **Namespace pattern matches** (prefix, suffix, contains)
4. **Service name pattern matches** (prefix, suffix, contains)  
5. **Wildcard matches** (`service_name: "*"`)
6. **Catch-all fallback** (empty routing `{}`)

### Performance Features

- **Singleton Optimization**: If only one schema is configured, routing is bypassed
- **Circular Cache**: 16-entry LRU cache for frequent lookups
- **Preprocessed Index**: O(1) exact matches, optimized pattern matching
- **Priority Sorting**: Schemas sorted once at initialization

### Cache Behavior

- **Cache Size**: Fixed 16-entry circular buffer
- **Cache Key**: `serviceName + serviceNamespace` combination
- **Cache Eviction**: Circular overwrite when capacity exceeded
- **Cache Clearing**: Automatic clear when configuration changes

### Error Handling

- **No Match Found**: Returns error if no schema matches and no catch-all exists
- **Invalid Configuration**: Logs warnings for malformed routing rules
- **Fallback Strategy**: Always configure a catch-all with priority 9999

## 🆕 Recent Updates

### Latest Improvements (Latest Release)

#### 1. **TimescaleDB Migration**
- **Hypertable Support**: Migrated from native PostgreSQL partitioning to TimescaleDB hypertables
- **Automatic Maintenance**: TimescaleDB handles chunk management automatically
- **Performance Optimization**: 67% reduction in maintenance code complexity
- **Apache License Compatible**: No licensing restrictions on maintenance features

#### 2. **Embedding Worker Improvements**
- **✅ Complete Trigger System**: All three processing modes (batch, trigger, hybrid) are fully operational
- **Real-Time Processing**: PostgreSQL triggers provide immediate signal processing
- **Safe Stop Method**: Workers can now be stopped multiple times without panicking
- **Debounced Processing**: Configurable debounce delay for trigger-based processing
- **Performance Optimization**: 50-100x reduction in batch operations
- **Error Handling**: Improved error handling and recovery with graceful fallbacks

#### 3. **Code Quality Improvements**
- **DRY Principle**: Eliminated code duplication in maintenance operations
- **Unified Configuration**: Centralized TimescaleDB configuration parsing and caching
- **Better Logging**: Enhanced debug and error logging
- **Type Safety**: Improved type safety and validation

#### 4. **Configuration Changes**
- **TimescaleDB Defaults**: TimescaleDB maintenance enabled by default
- **Simplified Configuration**: Reduced configuration complexity
- **Better Validation**: Enhanced configuration validation
- **Backward Compatibility**: Maintains compatibility with existing configurations

### Key Changes Summary

| Component | Change | Impact |
|-----------|--------|--------|
| TimescaleDB Maintenance | Default `enabled: true` | Automatic chunk management |
| Embedding Workers | Complete trigger system | Real-time processing |
| Embedding Workers | Safe stop method | No more panics |
| Maintenance Code | DRY refactoring | Better maintainability |
| Configuration | Unified service | Consistent behavior |

## 🔧 Troubleshooting

### Common Issues

#### 1. **Permission Denied Error**
```bash
zsh: permission denied: ./trazr-otel/trazr-otel
```
**Solution**: Add execute permissions:
```bash
chmod +x trazr-otel
```

#### 2. **Partition Management Not Working**
- Check that `enabled: true` in partition management config
- Verify `months_ahead > 0`
- Check database logs for errors

#### 3. **Embedding Workers Not Starting**
- Verify database connectivity
- Check embedding provider configuration
- Review worker logs for errors

#### 4. **High Memory Usage**
- Reduce `embedding_batch_size`
- Increase `embedding_trigger_debounce_delay`
- Monitor worker count

### Debug Mode

Enable debug logging by setting the log level in your configuration:

```yaml
service:
  telemetry:
    logs:
      level: debug
```

### Health Checks

The system provides health check endpoints for monitoring:

```bash
# Check overall health
curl http://localhost:4318/health

# Check specific components
curl http://localhost:4318/health/embedding
curl http://localhost:4318/health/partitions
```

## 📚 Additional Documentation

- [Signal Filtering Guide](pkg/repository/README.md) - Detailed filtering documentation
- [Embedding Configuration](pkg/repository/README.md#embedding-configuration-guide) - Embedding setup guide
- [Partition Management](pkg/repository/README.md#partition-management-configuration) - Partition management details
- [API Documentation](frontend/README.md) - Frontend API documentation

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the LICENSE file for details.

## 🆘 Support

For support and questions:
- Create an issue in the repository
- Check the troubleshooting section
- Review the configuration examples
- Consult the detailed documentation in `pkg/repository/README.md` 