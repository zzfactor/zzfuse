# Signal Table Filtering: Current State & Gaps

## Comprehensive Field Support Table

| Field Name              | Generic | Logs | Traces | Metrics Gauge | Metrics Sum | Metrics Histogram | Metrics Summary | Metrics Exp. Histogram | Notes                                      |
|------------------------|---------|------|--------|--------------|------------|-------------------|----------------|-----------------------|--------------------------------------------|
| timestamp_ns           | ✅      | ✅   | ✅     | ✅           | ✅         | ✅                | ✅             | ✅                    | Nanosecond-precision timestamp             |
| timestamp              | ✅      | ✅   | ✅     | ✅           | ✅         | ✅                | ✅             | ✅                    | Date/time string, auto-converted to ns     |
| service_name           | ✅      | ✅   | ✅     | ✅           | ✅         | ✅                | ✅             | ✅                    | Flat or legacy service name                |
| trace_id               | ✅      | ✅   | ✅     |              |            |                   |                |                       |                                            |
| span_id                | ✅      | ✅   | ✅     |              |            |                   |                |                       |                                            |
| resource_attributes.*  | ✅      | ✅   | ✅     | ✅           | ✅         | ✅                | ✅             | ✅                    | Flattened resource attributes              |
| attributes.*           | ✅      | ✅   | ✅     | ✅           | ✅         | ✅                | ✅             | ✅                    | Flattened signal/span/metric attributes    |
| body                   |         | ✅   |        |              |            |                   |                |                       | Log body/message                           |
| severity_number        |         | ✅   |        |              |            |                   |                |                       | Log severity (numeric)                     |
| severity_text          |         | ✅   |        |              |            |                   |                |                       | Log severity (text)                        |
| log_attributes.*       |         | ✅   |        |              |            |                   |                |                       | Flattened log attributes                   |
| name                   |         |      | ✅     | ✅           | ✅         | ✅                | ✅             | ✅                    | Span/metric name                           |
| unit                   |         |      |        | ✅           | ✅         | ✅                | ✅             | ✅                    | Metric unit                                |
| value                  |         |      |        | ✅           | ✅         | ✅                |               |                       | Metric value                               |
| count                  |         |      |        |              |            | ✅                | ✅             | ✅                    | Metric sample count                        |
| sum                    |         |      |        |              |            | ✅                | ✅             | ✅                    | Metric sample sum                          |
| buckets                |         |      |        |              |            | ✅                |               |                       | Histogram buckets                          |
| quantiles              |         |      |        |              |            |                   | ✅             |                       | Metric quantile values                     |
| scale                  |         |      |        |              |            |                   |               | ✅                    | Exponential histogram scale                |
| zero_count             |         |      |        |              |            |                   |               | ✅                    | Exponential histogram zero bucket count    |
| positive_bucket_counts |         |      |        |              |            |                   |               | ✅                    | Exponential histogram positive buckets     |
| negative_bucket_counts |         |      |        |              |            |                   |               | ✅                    | Exponential histogram negative buckets     |

> **Note:** All fields marked as Supported (✅) are available for filtering. Fields not listed or marked as future are not currently filterable.

---

## Generic (Common) Filterable Fields

| Field Name              | Description                                 |
|------------------------|---------------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp (int64)      |
| timestamp              | Date/time string (e.g., "2024-07-01")      |
| service_name           | Service name (legacy/flat)                  |
| trace_id               | Trace identifier                            |
| span_id                | Span identifier                             |
| resource_attributes.*  | Any resource attribute (flattened)          |
| attributes.*           | Any signal attribute (flattened)            |

> **Note:** Use `resource_attributes.service.name` for OTel-compliant filtering if your code flattens resource attributes. Use `service_name` for legacy or flat schemas.

---

## Filtering by Timestamp or Timestamp_ns

You can filter by either:
- `timestamp_ns`: Provide nanosecond values directly (legacy, int64)
- `timestamp`: Provide date/time strings (e.g., `"2024-07-01"` or `"2024-07-01T15:00:00Z"`)

If you use `timestamp`, the system will automatically convert the value(s) to nanoseconds and apply the filter to the `timestamp_ns` column.

### Examples

**YAML:**
```yaml
# Using a date/time string (recommended for readability)
timestamp:
  op: "BETWEEN"
  value: ["2024-07-01", "2024-07-02"]

# Using a specific instant
timestamp:
  op: "="
  value: "2024-07-01T15:00:00Z"

# Using nanoseconds (legacy, still supported)
timestamp_ns:
  op: "BETWEEN"
  value: [1722384000000000000, 1722470399999999999]
```

**JSON:**
```json
{
  "timestamp": { "op": "=", "value": "2024-07-01T15:00:00Z" },
  "timestamp_ns": { "op": "BETWEEN", "value": [1722384000000000000, 1722470399999999999] }
}
```

### How It Works
- If you specify `timestamp`, the system parses the string(s) and rewrites the filter to use `timestamp_ns` internally.
- If you specify `timestamp_ns`, it is used directly.
- Both can be used in the same filter config if needed.

---

## Signal-Specific Filterable Fields

### Logs
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| trace_id               | Trace identifier                   |
| span_id                | Span identifier                    |
| body                   | Log body/message                   |
| severity_number        | Log severity (numeric)             |
| severity_text          | Log severity (text)                |
| resource_attributes.*  | Any resource attribute (flattened) |
| log_attributes.*       | Any log attribute (flattened)      |
| attributes.*           | Any log attribute (flattened)      |

### Traces
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| trace_id               | Trace identifier                   |
| span_id                | Span identifier                    |
| name                   | Span name                          |
| resource_attributes.*  | Any resource attribute (flattened) |
| attributes.*           | Any span attribute (flattened)     |

### Metrics Gauge
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| name                   | Metric name                        |
| unit                   | Metric unit                        |
| value                  | Metric value                       |
| resource_attributes.*  | Any resource attribute (flattened) |
| attributes.*           | Any metric attribute (flattened)   |

### Metrics Sum
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| name                   | Metric name                        |
| unit                   | Metric unit                        |
| value                  | Metric value                       |
| resource_attributes.*  | Any resource attribute (flattened) |
| attributes.*           | Any metric attribute (flattened)   |

### Metrics Histogram
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| name                   | Metric name                        |
| unit                   | Metric unit                        |
| count                  | Sample count                       |
| sum                    | Sample sum                         |
| buckets                | Histogram buckets                  |
| resource_attributes.*  | Any resource attribute (flattened) |
| attributes.*           | Any metric attribute (flattened)   |

### Metrics Summary
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| name                   | Metric name                        |
| unit                   | Metric unit                        |
| count                  | Sample count                       |
| sum                    | Sample sum                         |
| quantiles              | Quantile values                    |
| resource_attributes.*  | Any resource attribute (flattened) |
| attributes.*           | Any metric attribute (flattened)   |

### Metrics Exponential Histogram
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| name                   | Metric name                        |
| unit                   | Metric unit                        |
| count                  | Sample count                       |
| sum                    | Sample sum                         |
| scale                  | Histogram scale                    |
| zero_count             | Zero bucket count                  |
| positive_bucket_counts | Positive bucket counts             |
| negative_bucket_counts | Negative bucket counts             |
| resource_attributes.*  | Any resource attribute (flattened) |
| attributes.*           | Any metric attribute (flattened)   |

### Profiles
| Field Name              | Description                        |
|------------------------|------------------------------------|
| timestamp_ns           | Nanosecond-precision timestamp     |
| timestamp              | Date/time string                   |
| service_name           | Service name                       |
| profile_id             | Profile identifier                 |
| start_time             | Profile start time                 |
| end_time               | Profile end time (when available)  |
| duration               | Profile duration (when available)  |
| dropped_attributes_count | Number of dropped attributes      |
| original_payload_format | Original payload format           |
| profile_type           | Profile type (from attributes)     |
| sample_type            | Sample type (from attributes)      |
| sample_unit            | Sample unit (from attributes)      |
| resource_attributes.*  | Any resource attribute (flattened) |
| attributes.*           | Any profile attribute (flattened)  |

---

## Filtering by Date (timestamp_ns)

### Current Approach
- The `timestamp_ns` field is stored as a nanosecond-precision Unix timestamp (int64).
- To filter by date, you must convert your desired date (e.g., `2024-07-01`) to a nanosecond range covering the start and end of that day.
- Use the `BETWEEN` operator in your filter config.

**Example:**
```yaml
timestamp_ns:
  op: "BETWEEN"
  value: [1722384000000000000, 1722470399999999999]  # Start and end of 2024-07-01 in ns
```

### Limitation
- Direct date string filtering (e.g., `timestamp_ns: { op: '=', value: "2024-07-01" }`) is **not currently supported**.
- You must perform the date-to-nanoseconds conversion outside the filter config (e.g., in your application or a script).

### Future Support
- Native support for date strings could be added by extending the filter parsing logic to recognize date strings and convert them to the appropriate nanosecond range automatically.

### Summary Table

| Approach                | Supported? | Notes                                                      |
|-------------------------|------------|------------------------------------------------------------|
| Direct date string      | ❌         | Not supported in current filter config                     |
| Numeric timestamp range | ✅         | Supported; convert date to ns range externally             |
| SQL date casting        | ❌         | Not implemented, but possible with code changes            |

---

## Using the Unified Filter System

Filters are now managed using the unified filter system stored in the `config_otel_processor` table's `filter` field. This system provides a consistent approach for both signal routing and embedding queries.

### Example Filter Configurations

The unified filter system supports hierarchical filtering with the following structure:

```json
{
  "signal_type": "logs,traces",
  "resource": {
    "service.name": {
      "op": "=",
      "value": "api-gateway"
    }
  },
  "scope": {
    "name": {
      "op": "LIKE",
      "value": "opentelemetry%"
    }
  },
  "attributes": {
    "severity": {
      "op": "IN",
      "value": ["ERROR", "WARN"]
    }
  },
  "logs": {
    "severity_text": {
      "op": "=",
      "value": "ERROR"
    }
  },
  "traces": {
    "status_code": {
      "op": "=",
      "value": "ERROR"
    }
  },
  "metrics": {
    "name": {
      "op": "=",
      "value": "http_requests_total"
    }
  },
  "profiles": {
    "duration": {
      "op": ">",
      "value": 1000
    }
  }
}
```

### How to Use the Unified Filter System

The unified filter system is used for both signal routing and embedding queries:

#### For Signal Routing (Exporters)
```go
// Parse filter config
filterConfig, err := ParseFilterConfig(config)
if err != nil {
    return err
}

// Use for signal routing
matches := serviceResourceManager.MatchByLogRecord(logRecord, scopeMatches)
```

#### For Embedding Queries
```go
// Parse filter config
filterConfig, err := ParseFilterConfig(config)
if err != nil {
    return err
}

// Create embedding query service
queryService := NewEmbeddingQueryService(db, schema, logger)

// Query signals for embedding
signals, err := queryService.QuerySignalsForEmbedding(ctx, filterConfig, batchSize)
if err != nil {
    return err
}
```

#### Filter Configuration Examples

**Simple Resource Filter:**
```json
{
  "resource": {
    "service.name": {
      "op": "=",
      "value": "my-service"
    }
  }
}
```

**Signal-Specific Filter:**
```json
{
  "signal_type": "logs",
  "logs": {
    "severity_text": {
      "op": "=",
      "value": "ERROR"
    }
  }
}
```

**Complex Hierarchical Filter:**
```json
{
  "signal_type": "logs,traces",
  "resource": {
    "service.name": {
      "op": "=",
      "value": "api-gateway"
    }
  },
  "attributes": {
    "environment": {
      "op": "=",
      "value": "production"
    }
  },
  "logs": {
    "severity_text": {
      "op": "IN",
      "value": ["ERROR", "WARN"]
    }
  },
  "traces": {
    "status_code": {
      "op": "=",
      "value": "ERROR"
    }
  }
}
```

---

## Filtering for Metrics and Traces

### Available Fields for Filtering

#### Metrics Tables
- Table names: `metrics_gauge`, `sum_metrics`, `metrics_histogram`, `metrics_summary`, `exponential_histogram_metrics`
- **Common filterable fields:**
  - `timestamp_ns`
  - `service_name`
  - `name`
  - `unit`
  - `value`, `count`, `sum`, `quantiles`, `buckets`, `scale`, `zero_count`, `positive_bucket_counts`, `negative_bucket_counts` (depending on metric type)
  - `attributes` (can be flattened for key-based filtering)
  - `resource_attributes` (can be flattened for key-based filtering)

#### Traces Table
- Table name: `traces`
- **Common filterable fields:**
  - `timestamp_ns`
  - `trace_id`
  - `span_id`
  - `name`
  - `service_name`
  - `attributes` (can be flattened for key-based filtering)
  - `resource_attributes` (can be flattened for key-based filtering)

### Example Filter Configurations for Metrics and Traces

**Metrics Filter:**
```json
{
  "signal_type": "metrics",
  "metrics": {
    "name": {
      "op": "=",
      "value": "cpu_usage"
    },
    "unit": {
      "op": "=",
      "value": "percent"
    }
  },
  "resource": {
    "service.name": {
      "op": "=",
      "value": "monitoring-service"
    }
  }
}
```

**Traces Filter:**
```json
{
  "signal_type": "traces",
  "traces": {
    "status_code": {
      "op": "=",
      "value": "ERROR"
    }
  },
  "resource": {
    "service.name": {
      "op": "=",
      "value": "api-gateway"
    }
  }
}
```

**Profiles Filter:**
```json
{
  "signal_type": "profiles",
  "profiles": {
    "duration": {
      "op": ">",
      "value": 1000
    },
    "profile_type": {
      "op": "=",
      "value": "cpu"
    }
  }
}
```

### How to Use with EmbeddingQueryService

```go
// Parse filter config
filterConfig, err := ParseFilterConfig(config)
if err != nil {
    return err
}

// Create embedding query service
queryService := NewEmbeddingQueryService(db, schema, logger)

// Query specific signal types
logs, err := queryService.QueryLogsForEmbedding(ctx, filterConfig, batchSize)
traces, err := queryService.QueryTracesForEmbedding(ctx, filterConfig, batchSize)
metrics, err := queryService.QueryMetricsForEmbedding(ctx, filterConfig, batchSize)
profiles, err := queryService.QueryProfilesForEmbedding(ctx, filterConfig, batchSize)

// Or query all signal types
allSignals, err := queryService.QuerySignalsForEmbedding(ctx, filterConfig, batchSize)
```

### Note on Attributes and Resource Attributes

- If you want to filter on a specific attribute or resource attribute, ensure your code flattens these fields (as your codebase does with `flattenAndMerge`).
- Example: To filter on a resource attribute `host.name`, your filter key would be `resource_attributes.host.name`.

---

## Current Implementation Status

### ✅ **Completed Features**

1. **Unified Filter System**
   - Hierarchical filtering with `resource`, `scope`, `attributes`, and signal-specific sections
   - Support for all OpenTelemetry signal types (logs, traces, metrics, profiles)
   - Parameterized SQL queries for security and performance
   - Consistent filter format for both signal routing and embedding queries

2. **EmbeddingQueryService**
   - Complete implementation for querying signals for embedding processing
   - Support for individual signal type queries and combined queries
   - Integration with the unified filter system
   - Proper error handling and logging

3. **Signal-Specific Filtering**
   - Log-specific fields: `severity_text`, `severity_number`, `body`
   - Trace-specific fields: `name`, `kind`, `status_code`, `duration`
   - Metric-specific fields: `name`, `value`, `unit`, `type`
   - Profile-specific fields: `profile_id`, `start_time`, `end_time`, `duration`

4. **Database Integration**
   - Removed old `config_embedding_filter` table
   - Updated `config_otel_processor` table to use unified filter system
   - Backward compatibility maintained for existing code

### 🔄 **Future Enhancements**

1. **Advanced Filtering**
   - Nested AND/OR logic for complex filter combinations
   - JSONB field queries for nested attribute filtering
   - Custom filter functions and expressions

2. **Performance Optimization**
   - Query plan analysis and optimization
   - Index recommendations for filter fields
   - Caching of frequently used filter configurations

3. **Dynamic Schema Support**
   - Dynamic field discovery from database schema
   - Support for custom signal types and tables
   - Plugin-based filter field definitions

4. **Enhanced Testing**
   - Integration tests with real database scenarios
   - Performance benchmarks for filter queries
   - Stress testing with large datasets

---

## Summary Table

| Feature                        | Status         | Notes / TODOs                                      |
|--------------------------------|---------------|------------------------------------------------------------|
| Basic SQL filtering            | ✅ Available  | See `embedding_filter.go` and `embedding_db_client.go` |
| Supported operators            | ✅ Available  | =, IN, NOT IN, LIKE, >, <, >=, <=, BETWEEN         |
| Nested/complex filters         | ❌ Missing    | No AND/OR/grouping, no deep JSONB field support    |
| Error handling/logging         | ⚠️ Partial   | Needs improvement                                  |
| Flexible schema/table support  | ⚠️ Partial   | Only known tables/fields supported                 |
| Dynamic field discovery        | ❌ Missing    | Hardcoded field lists                              |
| User documentation/examples    | ⚠️ Partial   | Needs more coverage                                |

---

**References:**
- `pkg/repository/embedding_filter.go` (filter parsing and SQL generation)
- `pkg/repository/embedding_db_client.go` (filter application in queries)

---

## Usage Patterns & Example Code

> **Note:** All filter config examples use the recommended merged format, where each signal type (e.g., `logs`, `traces`, `metrics_gauge`) is a top-level key. You can also include generic fields at the top level to apply to all signal types. By default, all signals are included unless listed in `exclude_signals`.

### 1. Signal Selection with `exclude_signals`

- If `exclude_signals` is present, any signal type listed will be ignored (not processed or queried).
- If `exclude_signals` is omitted or empty, all signals are included by default.
- An empty config `{}` means all signals are included and unfiltered.

**Example: Exclude traces and metrics**
```json
{
  "exclude_signals": ["traces", "metrics_gauge", "sum_metrics", "metrics_histogram", "metrics_summary", "exponential_histogram_metrics"],
  "logs": {
    "severity_number": { "op": ">", "value": 10 }
  }
}
```
- Only logs will be processed; all other signals are ignored.

**Example: Exclude nothing (default, all signals included)**
```json
{}
```

**Example: Exclude only metrics**
```json
{
  "exclude_signals": ["metrics_gauge", "sum_metrics"]
}
```

### 2. Validating a Filter Config in Go

```go
// Example: Validate a merged filter config using the real schema
import (
    "fmt"
    "os"
    "github.com/medxops/trazr-otel/pkg/repository"
)

validColumns := repository.ExtractColumnsFromSQL(logsSQL)
config := map[string]interface{}{
    "exclude_signals": []interface{}{ "traces" },
    "service_name": map[string]interface{}{"op": "=", "value": "my-service"},
    "logs": map[string]interface{}{
        "severity_number": map[string]interface{}{"op": "IN", "value": []interface{}{10, 20}},
    },
}
if repository.ShouldProcessSignal("logs", config) {
    err := repository.ValidateEmbeddingFilterQuery("logs", config, validColumns)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Invalid filter: %v\n", err)
        // Handle error
    }
}
```

### 3. JSON Filter Config Examples (Merged Format)

> **Note:** All filter config examples below use the merged format. Top-level keys are signal types (e.g., `logs`, `traces`, `metrics_gauge`). You can also include generic fields at the top level. Use `exclude_signals` to block signals.

**Logs and Traces (JSON):**
```json
{
  "exclude_signals": ["metrics_gauge", "sum_metrics"],
  "service_name": { "op": "=", "value": "my-service" },
  "logs": {
    "severity_number": { "op": "IN", "value": [10, 20] }
  },
  "traces": {
    "trace_id": { "op": "=", "value": "abc123" },
    "span_id": { "op": "LIKE", "value": "%def%" }
  }
}
```

**Metrics Gauge (JSON):**
```json
{
  "exclude_signals": ["logs", "traces"],
  "metrics_gauge": {
    "name": { "op": "=", "value": "cpu.usage" }
  }
}
```

### 4. Filtering with JSONB/Blob Columns

If your logs or traces are stored as a single JSONB/blob column (e.g., `data JSONB`), you must use PostgreSQL JSONB operators to filter on fields inside the blob. The current Go filter logic does **not** support this automatically.

**Example SQL:**
```sql
SELECT * FROM logs WHERE data->>'service_name' = 'my-service' AND data->'attributes'->>'severity_number' IN ('10', '20');
```

**Note:**
- You must write custom SQL or extend the filter logic to generate these JSONB queries.
- The JSON filter config examples above will **not** work out-of-the-box for blob-based schemas.

### 5. Extracting Valid Columns from Embedded SQL

```go
// For any signal type, use the corresponding embedded SQL
validColumns := repository.ExtractColumnsFromSQL(metricsGaugeSQL)
if repository.ShouldProcessSignal("metrics_gauge", config) {
    err := repository.ValidateEmbeddingFilterQuery("metrics_gauge", config, validColumns)
}
```

### 6. Troubleshooting & FAQ

- **Q: Why do I get 'invalid field: ...'?**
  - A: The filter config references a field not present in the table schema. Double-check the schema or use ExtractColumnsFromSQL to get the valid list. For blob schemas, you must use JSONB operators.
- **Q: Why do I get 'unsupported operator: ...'?**
  - A: The filter config uses an operator not in the supported set (=, IN, NOT IN, LIKE, >, <, >=, <=, BETWEEN).
- **Q: Why do I get 'IN value for ... must be an array'?**
  - A: The value for an IN/NOT IN operator must be a list/array, not a single value.
- **Q: How do I filter by date?**
  - A: Use the 'timestamp' field with a date string, or convert your date to a nanosecond range for 'timestamp_ns'.
- **Q: Can I use the Go filter logic with JSONB/blob columns?**
  - A: Not directly. You must extend the logic to generate JSONB queries or write custom SQL.
- **Q: What does an empty config mean?**
  - A: All signals are included and unfiltered by default.

--- 