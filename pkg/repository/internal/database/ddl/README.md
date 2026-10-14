# SQL Schema Files

This directory contains all SQL table creation scripts for the Trazr repository package. All SQL files use schema-qualified placeholders (`{{.Schema}}`) that are replaced with the actual schema name at runtime.

## Database Architecture

The schema is divided into two categories:

### 1. OpenTelemetry Tables (TimescaleDB Hypertables)
These tables store time-series observability data and use TimescaleDB for optimal performance:

- **logs.sql** - Log data with automatic chunking and retention
- **traces.sql** - Trace/span data with automatic chunking and retention
- **gauge_metrics.sql** - Gauge metrics with automatic chunking and retention
- **histogram_metrics.sql** - Histogram metrics with automatic chunking and retention
- **sum_metrics.sql** - Sum metrics with automatic chunking and retention
- **summary_metrics.sql** - Summary metrics with automatic chunking and retention
- **exponential_histogram_metrics.sql** - Exponential histogram metrics with automatic chunking and retention
- **profiles.sql** - Profile data with automatic chunking and retention

**TimescaleDB Features:**
- Automatic time-based chunking (1-day chunks by default)
- Manual chunk cleanup (Apache license compatible)
- Continuous aggregates for pre-computed aggregations
- Time-series optimized queries
- Automatic statistics management

### 2. Configuration Tables (Regular PostgreSQL)
These tables store configuration data and remain as regular PostgreSQL tables:

- **config_otel_database.sql** - Database connection configurations
- **config_embedding_provider.sql** - Embedding provider configurations
- **config_processor_template.sql** - Embedding template configurations
- **config_vectorstore_provider.sql** - Vector store provider configurations
- **config_otel_processor.sql** - Vector store configurations

## TimescaleDB Management

### timescaledb_management.sql
**Purpose**: TimescaleDB-specific management functions  
**Description**: Provides functions for managing hypertables, retention policies, and continuous aggregates.

**Key Functions:**
- `setup_timescaledb_hypertable()` - Configure individual hypertables
- `setup_all_timescaledb_tables()` - Configure all OpenTelemetry tables
- `get_timescaledb_info()` - Get hypertable statistics
- `setup_continuous_aggregates()` - Create pre-computed aggregations
- `get_continuous_aggregate_info()` - Monitor continuous aggregates

### Continuous Aggregates
Pre-configured continuous aggregates for common queries:

1. **hourly_error_counts** - Hourly error counts per service
2. **hourly_trace_counts** - Hourly trace counts per service  
3. **hourly_metric_averages** - Hourly metric statistics

## Configuration Tables Details

### config_otel_database.sql
**Purpose**: Database connection configurations  
**Table**: `config_otel_database`  
**Description**: Stores database endpoint configurations including connection details, credentials, and default settings.

**Key Features**:
- Unique constraint on `name` field
- Support for default configuration marking
- Automatic `updated_at` timestamp triggers
- Sample default data insertion

### config_vectorstore_provider.sql
**Purpose**: Vector store configurations  
**Table**: `config_vectorstore_provider`  
**Description**: Stores vector store provider configurations including connection settings, batch processing parameters, and vendor-specific options.

**Key Features**:
- Support for multiple vector store providers (pgvector, etc.)
- JSONB fields for promoted columns and vendor options
- Configurable batch processing and connection pooling
- Performance and reliability settings

### config_embedding_provider.sql
**Purpose**: Embedding provider configurations  
**Table**: `config_embedding_provider`  
**Description**: Stores LLM/embedding provider configurations including API credentials, timeout settings, and model parameters.

**Key Features**:
- Support for multiple embedding providers (OpenAI, etc.)
- JSONB fields for headers and filters
- Rate limiting and timeout configurations
- Provider-specific index for fast lookups

## Schema Relationship Tables

### config_otel_processor.sql
**Purpose**: Schema-to-vectorstore mappings  
**Tables**: 
- `config_otel_database` - Main schema configurations
- `config_otel_processor` - 1:N mappings between schemas and vector stores

**Description**: Implements the schema→vectorstore(s)→embedding(s) relationship architecture allowing multiple vector stores per schema with processing_mode-based routing.

**Key Features**:
- Purpose-based routing (logs, traces, metrics, all)
- Filter configurations for conditional processing
- Foreign key relationships with cascade deletes

## Usage Pattern

All Go configuration managers use embedded SQL from these files:

```go
//go:embed ddl/config_otel_database.sql
var databaseConfigSQL string

// In EnsureTable method:
query := fmt.Sprintf(databaseConfigSQL, schemaName, schemaName, ...)
```

## Schema Placeholder Format

All SQL files use `{{.Schema}}` placeholders that are replaced with schema names:
- Default schema: `admin`
- Example: `{{.Schema}}.config_otel_database` becomes `admin.config_otel_database`

## Migration from Native PostgreSQL Partitioning

This represents a migration from native PostgreSQL partitioning to TimescaleDB hypertables for OpenTelemetry data:

**Benefits:**
- Automatic chunk management
- Manual chunk cleanup (Apache license compatible)
- Continuous aggregates for performance
- Time-series optimized functions
- Simplified maintenance
- 67% reduction in maintenance code complexity

**Migration Guide:** See `TIMESCALEDB_MIGRATION_CODE_REVIEW.md` for detailed migration status and remaining tasks.

## Batch Processing Controls for Embedding Workers

Embedding workers support advanced batch processing controls, which can be configured via the vector store configuration table:

- **FlushInterval**: Interval to wait after each batch (0 disables delay)
- **MaxRetries**: Maximum retry attempts for failed batch operations (0 disables retries)
- **BatchErrorPolicy**: Error policy for batch failures:
  - `continue`: Log the error and skip the failed batch, continue processing others
  - `stop`: Abort the worker immediately on error
  - `retry`: Retry retryable errors up to MaxRetries, using exponential backoff
- **MaxConcurrentBatches**: Maximum concurrent batch operations (0 disables concurrency limit)
- **RetryBackoffBase/Max**: Exponential backoff base/max for retries (defaults: 1s/30s)

### Example Usage

Set these fields in your vector store config to control worker behavior. For example:

| Field                | Value         | Description                                 |
|----------------------|--------------|---------------------------------------------|
| batch_error_policy   | retry        | Retry failed batches up to MaxRetries       |
| max_retries          | 3            | Retry up to 3 times                         |
| flush_interval_ms    | 200          | Wait 200ms after each batch                 |
| max_concurrent_batches | 2          | Allow up to 2 concurrent batches            |

### Error Policy Semantics

- **continue**: The worker logs the error and skips the failed batch, continuing with the next batch.
- **stop**: The worker aborts immediately on error, no further batches are processed.
- **retry**: The worker retries retryable errors up to MaxRetries, using exponential backoff with jitter. If all retries fail, the error is logged and the batch is skipped.

All controls are per-worker and can be set via the config_vectorstore_provider table.

## Benefits

1. **Single Source of Truth**: SQL files are the authoritative schema definition
2. **Better Tooling**: SQL syntax highlighting, formatting, and linting
3. **Version Control**: Clear diff tracking for schema changes
4. **Maintainability**: Easier to review and modify complex SQL
5. **Consistency**: Follows established patterns from trazrexporter package
6. **Performance**: TimescaleDB optimization for time-series data
7. **Automation**: Built-in retention and chunk management

See GoDoc comments in the code for further details. 