# Repository Configuration Guide

This guide explains how to configure **signal filtering**, **embedding**, **template content**, and **template metadata** in this repository.

---

## Signal Filtering

The signal filtering system allows you to control which telemetry signals (logs, traces, metrics) are processed by each schema. Filters are configured in the `config_otel_database` table and support hierarchical filtering by signal type, resource attributes, scope attributes, and signal-specific attributes.

### ⚠️ **Important: Filter Syntax vs Template Syntax**

**Filters** and **Templates** use **different syntax**:

| Feature | Syntax | Purpose | Example |
|---------|--------|---------|---------|
| **Filters** | JSON with operators | Control which signals are stored/processed | `{"op": "=", "value": "api-service"}` |
| **Templates** | Go template syntax | Format content for embedding | `{{.service_name}}` |

**Filter Syntax (JSON with operators):**
```json
{
  "resource": {
    "service.name": {"op": "=", "value": "api-service"}
  }
}
```

**Template Syntax (Go templates):**
```go
"Service: {{.service_name}}. Message: {{.body}}"
```

**Key Differences:**
- **Filters**: Use JSON with `op` and `value` fields for conditions
- **Templates**: Use `{{.field_name}}` for variable substitution
- **Filters**: Control data flow (what gets stored)
- **Templates**: Control data formatting (how it's formatted)

### Quick Start: Metrics Filtering

For metrics, use `signal_type: "metrics"` and filter by metric type in the `attributes` section:

```json
{
  "signal_type": "metrics",
  "attributes": {
    "metric_type": { "op": "=", "value": "gauge_metrics" },
    "name": { "op": "=", "value": "http_requests_total" }
  }
}
```

**Why `signal_type: "metrics"`?** This ensures proper filtering at the resource level, while metric type information is available at the data point level for granular filtering.

### Enhanced Filter Structure

Filters use a hierarchical structure with **five main sections**:

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
    "severity_number": { "op": ">=", "value": 16 },
    "body": { "op": "LIKE", "value": "%error%" }
  }
}
```

#### **Signal-Specific Sections**

The new filter structure includes dedicated sections for signal-specific fields:

- **`logs`**: Filter by log-specific fields (severity, body content)
- **`metrics`**: Filter by metric-specific fields (name, value, unit, type)
- **`traces`**: Filter by trace-specific fields (name, kind, status code, duration)

These sections work in addition to the standard `resource`, `scope`, and `attributes` sections, providing granular control over signal-specific data.

### Database Filter Configuration

Filters are stored in the `config_otel_database` table in the `filter` column as **JSONB**:

```sql
-- Check current filter configuration
SELECT name, filter FROM config_otel_database WHERE enabled = true;

-- Update filter for a schema
UPDATE config_otel_database 
SET filter = '{"signal_type": "logs,traces"}'::jsonb
WHERE name = 'your-schema-name';
```

### Filter Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equals | `{"op": "=", "value": "api-service"}` |
| `!=` | Not equals | `{"op": "!=", "value": "test-service"}` |
| `IN` | In list | `{"op": "IN", "value": ["api", "web"]}` |
| `NOT_IN` | Not in list | `{"op": "NOT_IN", "value": ["test"]}` |
| `LIKE` | Pattern match | `{"op": "LIKE", "value": "%error%"}` |
| `>` | Greater than | `{"op": ">", "value": 100}` |
| `>=` | Greater than or equal | `{"op": ">=", "value": 16}` |
| `<` | Less than | `{"op": "<", "value": 1000}` |
| `<=` | Less than or equal | `{"op": "<=", "value": 500}` |
| `*` | Wildcard (accept all) | `{"op": "=", "value": "*"}` |

### Signal Types

The following signal types are supported:

| Signal Type | Constant | Description |
|-------------|----------|-------------|
| `logs` | `SignalTypeLogs` | Log records |
| `traces` | `SignalTypeTraces` | Trace spans |
| `gauge_metrics` | `SignalTypeGaugeMetrics` | Gauge metrics |
| `sum_metrics` | `SignalTypeSumMetrics` | Sum/Counter metrics |
| `histogram_metrics` | `SignalTypeHistogramMetrics` | Histogram metrics |
| `summary_metrics` | `SignalTypeSummaryMetrics` | Summary metrics |
| `exponential_histogram_metrics` | `SignalTypeExponentialHistogramMetrics` | Exponential histogram metrics |
| `profiles` | `SignalTypeProfiles` | Profile data |
| `*` | `SignalTypeAll` | All signal types (wildcard) |

### Filter Examples

#### 1. Filter for Logs Only
```json
{
  "signal_type": "logs",
  "resource": {
    "service.name": { "op": "=", "value": "user-service" },
    "deployment.environment": { "op": "=", "value": "production" }
  }
}
```

#### 2. Filter for Multiple Signal Types
```json
{
  "signal_type": ["logs", "traces"],
  "resource": {
    "service.name": { "op": "=", "value": "payment-service" }
  }
}
```

#### 3. Accept All Signal Types (Default)
```json
{
  "resource": {
    "service.name": { "op": "=", "value": "my-service" }
  }
}
```

#### 4. Complex Filter with All Sections
```json
{
  "signal_type": "logs",
  "resource": {
    "service.name": { "op": "=", "value": "api-service" },
    "deployment.environment": { "op": "=", "value": "production" }
  },
  "scope": {
    "library.name": { "op": "=", "value": "express" }
  },
  "attributes": {
    "log.level": { "op": "=", "value": "error" }
  }
}
```

#### 5. Metrics Filtering with Metric Types
```json
{
  "signal_type": "metrics",
  "resource": {
    "service.name": { "op": "=", "value": "my-service" }
  },
  "attributes": {
    "metric_type": { "op": "=", "value": "gauge_metrics" },
    "name": { "op": "=", "value": "http_requests_total" }
  }
}
```

#### 6. Filter Multiple Metric Types
```json
{
  "signal_type": "metrics",
  "resource": {
    "service.name": { "op": "=", "value": "my-service" }
  },
  "attributes": {
    "metric_type": { "op": "in", "value": ["gauge_metrics", "sum_metrics"] },
    "name": { "op": "like", "value": "http_*" }
  }
}
```

#### 7. Filter All Metrics by Name Pattern
```json
{
  "signal_type": "metrics",
  "attributes": {
    "metric_type": { "op": "in", "value": ["gauge_metrics", "sum_metrics", "histogram_metrics", "summary_metrics", "exponential_histogram_metrics"] },
    "name": { "op": "like", "value": "business_*" }
  }
}
```

#### 8. Enhanced Log Filtering with Signal-Specific Fields
```json
{
  "signal_type": "logs",
  "resource": {
    "service.name": { "op": "=", "value": "api-service" },
    "deployment.environment": { "op": "=", "value": "production" }
  },
  "logs": {
    "severity_text": { "op": "=", "value": "ERROR" },
    "severity_number": { "op": ">=", "value": 16 },
    "body": { "op": "LIKE", "value": "%database%connection%" }
  }
}
```

#### 9. Enhanced Metrics Filtering with Signal-Specific Fields
```json
{
  "signal_type": "metrics",
  "resource": {
    "service.name": { "op": "=", "value": "monitoring-service" }
  },
  "metrics": {
    "name": { "op": "LIKE", "value": "cpu_%" },
    "value": { "op": ">", "value": 80.0 },
    "unit": { "op": "=", "value": "percent" },
    "type": { "op": "=", "value": "gauge_metrics" }
  }
}
```

#### 10. Enhanced Trace Filtering with Signal-Specific Fields
```json
{
  "signal_type": "traces",
  "resource": {
    "service.name": { "op": "=", "value": "payment-service" }
  },
  "traces": {
    "name": { "op": "LIKE", "value": "process_payment%" },
    "kind": { "op": "=", "value": "SPAN_KIND_SERVER" },
    "status_code": { "op": "=", "value": "STATUS_CODE_ERROR" },
    "duration": { "op": ">", "value": 5000 }
  }
}
```

#### 11. Complex Multi-Signal Filter with Signal-Specific Fields
```json
{
  "signal_type": ["logs", "traces"],
  "resource": {
    "service.name": { "op": "=", "value": "user-service" },
    "deployment.environment": { "op": "=", "value": "staging" }
  },
  "scope": {
    "library.name": { "op": "=", "value": "express" }
  },
  "attributes": {
    "user.id": { "op": "=", "value": "premium" }
  },
  "logs": {
    "severity_text": { "op": "in", "value": ["ERROR", "WARN"] },
    "body": { "op": "LIKE", "value": "%authentication%" }
  },
  "traces": {
    "name": { "op": "LIKE", "value": "auth_%" },
    "status_code": { "op": "=", "value": "STATUS_CODE_ERROR" }
  }
}
```

### Signal-Specific Fields

Each signal type supports specific fields for granular filtering:

#### **Log Fields (`logs` section)**
| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `severity_text` | string | Log severity text (TRACE, DEBUG, INFO, WARN, ERROR, FATAL) | `{ "op": "=", "value": "ERROR" }` |
| `severity_number` | int32 | Log severity number (1-24, higher = more severe) | `{ "op": ">=", "value": 16 }` |
| `body` | string | Log message content | `{ "op": "LIKE", "value": "%error%" }` |

#### **Metric Fields (`metrics` section)**
| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Metric name | `{ "op": "LIKE", "value": "cpu_%" }` |
| `value` | float64 | Metric value | `{ "op": ">", "value": 80.0 }` |
| `unit` | string | Metric unit | `{ "op": "=", "value": "percent" }` |
| `type` | string | Metric type (gauge_metrics, sum_metrics, etc.) | `{ "op": "=", "value": "gauge_metrics" }` |

#### **Trace Fields (`traces` section)**
| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `name` | string | Span name | `{ "op": "LIKE", "value": "process_%" }` |
| `kind` | string | Span kind (SPAN_KIND_SERVER, SPAN_KIND_CLIENT, etc.) | `{ "op": "=", "value": "SPAN_KIND_SERVER" }` |
| `status_code` | string | Span status (STATUS_CODE_OK, STATUS_CODE_ERROR) | `{ "op": "=", "value": "STATUS_CODE_ERROR" }` |
| `duration` | int64 | Span duration in nanoseconds | `{ "op": ">", "value": 1000000 }` |

### Supported Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equals | `{ "op": "=", "value": "my-service" }` |
| `IN` | In array | `{ "op": "IN", "value": ["service1", "service2"] }` |
| `NOT IN` | Not in array | `{ "op": "NOT IN", "value": ["service1", "service2"] }` |
| `LIKE` | Pattern matching | `{ "op": "LIKE", "value": "web-%" }` |
| `>` | Greater than | `{ "op": ">", "value": 1000 }` |
| `<` | Less than | `{ "op": "<", "value": 1000 }` |
| `>=` | Greater than or equal | `{ "op": ">=", "value": 1000 }` |
| `<=` | Less than or equal | `{ "op": "<=", "value": 1000 }` |
| `BETWEEN` | Between two values | `{ "op": "BETWEEN", "value": [1000, 2000] }` |

### Database Filter vs Vector Store Filter

**Important**: There are **two types of filters** with different purposes:

| Filter Type | Location | Purpose | Controls |
|-------------|----------|---------|----------|
| **Database Filter** | `config_otel_database.filter` | What signals are stored in the database | Data storage |
| **Vector Store Filter** | `vector_store_mapping.filter` | What signals are embedded into vector stores | Data embedding |

#### **Filter Hierarchy**
```
Database Filter (Master) → Vector Store Filter (Consumer)
     ↓                           ↓
What's stored              What's embedded
```

**Example:**
- **Database Filter**: `{"signal_type": "logs,traces,metrics"}` (stores logs, traces, metrics)
- **Vector Store A Filter**: `{"signal_type": "logs"}` (embeds only logs)
- **Vector Store B Filter**: `{"signal_type": "traces"}` (embeds only traces)
- **Result**: Vector Store A gets logs, Vector Store B gets traces (both from what's stored)

#### **Key Rules**
1. **Database filter is the master** - vector stores can only embed what's stored
2. **No intersection = use database filter** - if vector store wants spans but database only stores logs, only logs get embedded
3. **Empty database filter = accept all** - `{}` means store everything
4. **Empty vector store filter = accept all stored** - `{}` means embed everything that's stored

### Testing Your Filters

#### **1. Check Current Filter Configuration**
```sql
-- View all database schemas and their filters
SELECT name, schema, filter, enabled 
FROM config_otel_database 
WHERE enabled = true;

-- View specific schema filter
SELECT name, filter 
FROM config_otel_database 
WHERE name = 'your-schema-name';
```

#### **2. Run Filter Tests**
```bash
# Test the filter parsing and matching logic
go test -v -run "TestDatabaseSignalFilter"

# Test smart worker creation with filters
go test -v -run "TestSmartWorkerCreation"
```

#### **3. Monitor Application Logs**
Look for these log messages to verify filter behavior:
```
"Created schema-level workers" "count"=2 "types"=[logs traces]
"Database signal types" "types"=[logs traces metrics]
"Vector store embedding signal types" "types"=[logs]
"Intersection signal types" "types"=[logs]
```

#### **4. Test Filter Updates**
```sql
-- Update filter and restart application
UPDATE config_otel_database 
SET filter = '{"signal_type": "logs,traces"}'::jsonb
WHERE name = 'your-schema-name';

-- Verify the change
SELECT name, filter FROM config_otel_database WHERE name = 'your-schema-name';
```

### Database Schema

The filter configuration is stored in the `config_otel_database` table:

```sql
CREATE TABLE config_otel_database (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    schema VARCHAR(255) UNIQUE NOT NULL DEFAULT 'otel',
    enabled BOOLEAN DEFAULT TRUE,
    routing JSONB DEFAULT '{}'::jsonb,
    filter JSONB NOT NULL DEFAULT '{}',
    description TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### SQL Insert Examples

```sql
-- Insert a filter for production logs
INSERT INTO config_otel_database (name, schema, filter, description) 
VALUES (
  'production-logs',
  'otel',
  '{
    "signal_type": "logs",
    "resource": {
      "service.name": {"op": "=", "value": "user-service"},
      "deployment.environment": {"op": "=", "value": "production"}
    }
  }'::jsonb,
  'Production logs for user service'
);

-- Insert a filter for all metrics
INSERT INTO config_otel_database (name, schema, filter, description) 
VALUES (
  'all-metrics',
  'otel',
  '{
    "signal_type": "metrics",
    "resource": {
      "service.name": {"op": "=", "value": "monitoring-service"}
    },
    "attributes": {
      "metric_type": {"op": "in", "value": ["gauge_metrics", "sum_metrics", "histogram_metrics", "summary_metrics", "exponential_histogram_metrics"]}
    }
  }'::jsonb,
  'All metrics for monitoring service'
);

-- Insert a filter for specific metric types
INSERT INTO config_otel_database (name, schema, filter, description) 
VALUES (
  'http-metrics',
  'otel',
  '{
    "signal_type": "metrics",
    "resource": {
      "service.name": {"op": "=", "value": "web-service"}
    },
    "attributes": {
      "metric_type": {"op": "in", "value": ["gauge_metrics", "sum_metrics"]},
      "name": {"op": "like", "value": "http_*"}
    }
  }'::jsonb,
  'HTTP-related gauge and sum metrics for web service'
);

-- Insert a filter for business metrics
INSERT INTO config_otel_database (name, schema, filter, description) 
VALUES (
  'business-metrics',
  'otel',
  '{
    "signal_type": "metrics",
    "attributes": {
      "metric_type": {"op": "in", "value": ["gauge_metrics", "sum_metrics", "histogram_metrics"]},
      "name": {"op": "like", "value": "business_*"}
    }
  }'::jsonb,
  'All business-related metrics across all services'
);

-- Insert enhanced log filter with signal-specific fields
INSERT INTO config_otel_database (name, schema, filter, description) 
VALUES (
  'error-logs',
  'otel',
  '{
    "signal_type": "logs",
    "resource": {
      "service.name": {"op": "=", "value": "api-service"},
      "deployment.environment": {"op": "=", "value": "production"}
    },
    "logs": {
      "severity_text": {"op": "=", "value": "ERROR"},
      "severity_number": {"op": ">=", "value": 16},
      "body": {"op": "like", "value": "%database%connection%"}
    }
  }'::jsonb,
  'Production error logs with database connection issues'
);

-- Insert enhanced metrics filter with signal-specific fields
INSERT INTO config_otel_database (name, schema, filter, description) 
VALUES (
  'high-cpu-metrics',
  'otel',
  '{
    "signal_type": "metrics",
    "resource": {
      "service.name": {"op": "=", "value": "monitoring-service"}
    },
    "metrics": {
      "name": {"op": "like", "value": "cpu_%"},
      "value": {"op": ">", "value": 80.0},
      "unit": {"op": "=", "value": "percent"},
      "type": {"op": "=", "value": "gauge_metrics"}
    }
  }'::jsonb,
  'High CPU usage metrics for monitoring service'
);

-- Insert enhanced trace filter with signal-specific fields
INSERT INTO config_otel_database (name, schema, filter, description) 
VALUES (
  'slow-payment-traces',
  'otel',
  '{
    "signal_type": "traces",
    "resource": {
      "service.name": {"op": "=", "value": "payment-service"}
    },
    "traces": {
      "name": {"op": "like", "value": "process_payment%"},
      "kind": {"op": "=", "value": "SPAN_KIND_SERVER"},
      "status_code": {"op": "=", "value": "STATUS_CODE_ERROR"},
      "duration": {"op": ">", "value": 5000}
    }
  }'::jsonb,
  'Slow and failed payment processing traces'
);
```

### Enhanced Filtering Process

The filtering system uses a **four-phase sequential approach** for optimal performance:

1. **Signal Type Filtering**: First, the system checks if the signal type matches the configured `signal_type`
2. **Resource Filtering**: Then, it filters based on resource attributes (service name, environment, etc.)
3. **Scope Filtering**: Next, it filters based on instrumentation scope attributes
4. **Attribute Filtering**: Then, it filters based on signal-specific attributes
5. **Signal-Specific Field Filtering**: Finally, it filters based on signal-specific fields (logs, metrics, traces)

#### **Performance Optimizations**

- **Early Exit**: If any phase fails, the entire filter fails immediately
- **Fast Paths**: Empty sections are skipped for maximum performance
- **Lazy Evaluation**: Signal-specific fields are only extracted when needed
- **Combined Filtering**: Attributes and signal-specific fields are evaluated together for efficiency

If any section is not specified or is empty, it accepts all signals for that level.

### Implementation Details

#### **Signal-Specific Field Processing**

The enhanced filtering system processes signal-specific fields efficiently:

- **Log Records**: Direct access to `severity_text`, `severity_number`, and `body` fields
- **Metrics**: Access to `name`, `value`, `unit`, and `type` at the data point level
- **Traces**: Access to `name`, `kind`, `status_code`, and `duration` at the span level

#### **Performance Benefits**

1. **Lazy Field Extraction**: Signal-specific fields are only extracted when filters are configured
2. **Early Exit**: Filtering stops as soon as any condition fails
3. **Combined Evaluation**: Attributes and signal-specific fields are evaluated together
4. **Memory Efficiency**: No unnecessary data copying or conversion

#### **Backward Compatibility**

The enhanced filter structure is **fully backward compatible**:
- Existing filters without signal-specific sections continue to work
- New signal-specific sections are optional
- All existing operators and syntax remain unchanged

#### **Use Cases**

**Log Filtering Use Cases:**
- Filter error logs by severity level
- Search for specific error patterns in log messages
- Route high-severity logs to dedicated schemas

**Metrics Filtering Use Cases:**
- Filter high-value metrics for alerting
- Route specific metric types to optimized storage
- Filter metrics by performance thresholds

**Trace Filtering Use Cases:**
- Filter slow spans for performance analysis
- Route error traces to debugging schemas
- Filter traces by business operation type

### Metrics Filtering Architecture

Metrics filtering uses a **three-phase sequential filtering approach** that works within the constraints of OpenTelemetry's data structure:

#### **Phase 1: Resource Filtering (Signal Type: Metrics)**
```go
// At the ResourceMetrics level, we use SignalTypeMetrics for initial filtering
// because a single ResourceMetrics can contain multiple metric types
resourceMatches := MatchByResourceAttributes(resourceAttrs, SignalTypeMetrics)
```

#### **Phase 2: Scope Filtering (Signal Type: Metrics)**
```go
// At the ScopeMetrics level, we still don't know specific metric types
scopeMatches := MatchByScopeAttributes(scopeAttrs, resourceMatches)
```

#### **Phase 3: Attribute Filtering (Metric Type: Specific)**
```go
// At the DataPoint level, we now know the specific metric type
dataPointAttrs["metric_type"] = "gauge_metrics"  // or "sum_metrics", etc.
dataPointAttrs["name"] = "http_requests_total"
finalMatches := MatchByAttributes(dataPointAttrs, scopeMatches)
```

#### **Why This Approach?**

1. **OpenTelemetry Structure**: ResourceMetrics can contain multiple metric types (gauge, sum, histogram, etc.)
2. **Performance**: Early filtering at resource/scope level reduces processing overhead
3. **Flexibility**: Granular control over specific metric types and names
4. **Consistency**: Works with the existing signal type filtering system

#### **Available Metric Types for Filtering**

| Metric Type | Value | Description |
|-------------|-------|-------------|
| Gauge Metrics | `gauge_metrics` | Current value metrics |
| Sum Metrics | `sum_metrics` | Cumulative/counter metrics |
| Histogram Metrics | `histogram_metrics` | Distribution metrics |
| Summary Metrics | `summary_metrics` | Quantile-based metrics |
| Exponential Histogram Metrics | `exponential_histogram_metrics` | Exponential distribution metrics |

#### **Metrics Filtering Examples**

**Filter by Specific Metric Type and Name:**
```json
{
  "signal_type": "metrics",
  "attributes": {
    "metric_type": { "op": "=", "value": "gauge_metrics" },
    "name": { "op": "=", "value": "cpu_usage_percent" }
  }
}
```

**Filter by Multiple Metric Types:**
```json
{
  "signal_type": "metrics",
  "attributes": {
    "metric_type": { "op": "in", "value": ["gauge_metrics", "sum_metrics"] },
    "name": { "op": "like", "value": "http_*" }
  }
}
```

**Filter by Metric Name Only (All Types):**
```json
{
  "signal_type": "metrics",
  "attributes": {
    "name": { "op": "like", "value": "business_*" }
  }
}
```

**Filter by Metric Type Only:**
```json
{
  "signal_type": "metrics",
  "attributes": {
    "metric_type": { "op": "=", "value": "histogram_metrics" }
  }
}
```

#### **Implementation Details**

The metrics filtering system automatically adds the following attributes to each data point for filtering:

- **`metric_type`**: The specific metric type (`gauge_metrics`, `sum_metrics`, etc.)
- **`name`**: The metric name (e.g., `http_requests_total`, `cpu_usage_percent`)

These attributes are available for filtering in the `attributes` section of your filter configuration.

#### **Best Practices**

1. **Use `signal_type: "metrics"` for metrics**: Use the specific metrics signal type for proper filtering
2. **Combine metric_type with name**: For precise filtering, combine both metric type and name
3. **Use pattern matching**: Leverage `LIKE` and `IN` operators for flexible filtering
4. **Consider performance**: More specific filters (resource + scope + attributes) perform better than broad filters
5. **Group related metrics**: Use consistent naming patterns (e.g., `http_*`, `business_*`) for easier filtering

#### **Performance Considerations**

- **Resource filtering**: Happens once per ResourceMetrics (most efficient)
- **Scope filtering**: Happens once per ScopeMetrics (efficient)
- **Attribute filtering**: Happens per data point (most granular)

For high-volume metrics, consider using resource and scope filters to reduce the number of data points that reach the attribute filtering phase.

---

## Embedding Configuration Guide

This guide explains how to specify **filters**, **template content**, and **template metadata** for embedding in this repository.

---

## 1. Filter Configuration

Filters control which signals are selected for embedding. You can specify generic filters (applied to all signal types) and per-signal filters.

**Format:**
- Generic fields are at the top level.
- Per-signal fields are nested under their signal type key.

**Example (JSON):**
```json
{
  "service_name": { "op": "=", "value": "my-service" },
  "timestamp_ns": { "op": ">", "value": 1722384000000000000 },
  "logs": {
    "level": { "op": "=", "value": "ERROR" }
  },
  "traces": {
    "span_kind": { "op": "=", "value": "server" }
  },
  "exclude_signals": ["metrics_gauge"]
}
```

---

## 2. Template Content

Template content defines how the text for each embedded document is rendered. You can specify a template string for each signal type using Go template syntax.

**Format:**
- A map from signal type to template string.
- Use `{{.field_name}}` to reference fields from the signal.

**Example (Go map):**
```go
var DefaultTemplateContent = map[string]string{
  "logs":                  `Service: {{.service_name}}. Log level: {{.level}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}.`,
  "traces":                `Service: {{.service_name}}. Trace: {{.trace_id}}. Span: {{.span_id}}. Operation: {{.name}}.`,
  "gauge":                 `Service: {{.service_name}}. Gauge {{.name}}: value={{.value}} at {{.timestamp}} (labels: {{.labels}})`,
  // ... other types ...
}
```

---

## 3. Template Metadata

Template metadata specifies which fields from each signal should be included in the metadata for the embedded document. You can specify generic fields (included for all types) and per-signal fields.

**Format:**
- Use a `"generic"` key for fields to include for all signal types.
- Specify additional fields for each signal type as needed.

**Example (Go map):**
```go
var DefaultTemplateMetadata = map[string][]string{
  "generic":               {"timestamp_ns"},
  "logs":                  {"service_name", "level", "event_name"},
  "traces":                {"service_name", "name", "span_kind", "status_code"},
  "gauge":                 {"service_name", "name", "unit", "attributes"},
  // ... other types ...
}
```

---

## 4. Specifying API Keys Securely with Metadata Secrets

For security, **do not store plaintext API keys** in the database. Instead, use the `metadata` field to reference secrets, such as environment variables or secret manager paths.

**Example (JSON):**
```json
{
  "provider": "openai",
  "model": "text-embedding-ada-002",
  "metadata": {
    "secret_api_key": "env:OPENAI_API_KEY"
  }
}
```

- `secret_api_key`: The value should be in the format `env:ENV_VAR_NAME` to indicate the API key should be loaded from the environment variable `ENV_VAR_NAME` at runtime.
- You can also use other schemes (e.g., `vault:...`) if your application supports secret managers.

**How it works:**
- At runtime, your application should resolve the secret reference (e.g., read the API key from the specified environment variable) and inject it into the provider config before instantiating the embedding provider.

**Benefits:**
- Keeps sensitive credentials out of the database and codebase.
- Supports flexible secret management strategies.

---

## 4. Group Signals Embedding

The `embedding_batch_group_signals` flag controls how signals are processed during embedding. When enabled, multiple signals are grouped together for batch processing, which can improve efficiency for certain use cases.

**Configuration:**
Set this flag in the `metadata` field of your `ProcessorMappingConfig`. The parameter is automatically set to `false` by default when creating new vector store mappings.

```json
{
  "name": "my-mapping",
  "database_id": 1,
  "vectorstore_id": 1,
  "processing_mode": "search",
  "metadata": {
    "embedding_batch_group_signals": true
  }
}
```

**Behavior:**
- **`true`**: Uses grouped signal processing (`processGroupedSignals`)
  - Multiple signals are processed together in batches
  - Useful for scenarios where signals have related context
  - May improve embedding quality for correlated data
  
- **`false`**: Uses single signal processing (`processSingleSignal`) - **default behavior**
  - Each signal is processed individually
  - Standard processing mode for most use cases

**Example Usage:**
```go
mapping := &ProcessorMappingConfig{
    Name: "default",
    DatabaseID: 1,
    VectorStoreID: 1,
    Purpose: "search",
    Metadata: map[string]interface{}{
        "embedding_batch_group_signals": true,  // Enable grouped processing
    },
}
```

---

## 5. Debounced Trigger Processing

The `embedding_trigger_debounce_delay` configuration optimizes trigger-based embedding processing by batching multiple database triggers together, significantly reducing the number of batch operations and improving performance.

### **The Problem**

Without debouncing, every database insert triggers an immediate batch processing operation:

```
[Notifier] Fanning out notification on channel 'otel.new_log' to 1 subscribers
[Notifier] Delivered notification to a subscriber on channel 'otel.new_log'
[Notifier] Fanning out notification on channel 'otel.new_log' to 1 subscribers
[Notifier] Delivered notification to a subscriber on channel 'otel.new_log'
[Notifier] Fanning out notification on channel 'otel.new_log' to 1 subscribers
[Notifier] Delivered notification to a subscriber on channel 'otel.new_log'
...
[Worker] Processing debounced triggers: signal_type=logs, trigger_count=3
```

This results in:
- **1000 inserts = ~1-5 batch operations** ✅
- **Reduced CPU usage** ✅
- **Better database efficiency** ✅
- **Configurable delay** ✅

### **Configuration**

Set the debounce delay in the `metadata` field of your `ProcessorMappingConfig`:

```json
{
  "name": "my-mapping",
  "database_id": 1,
  "vectorstore_id": 1,
  "processing_mode": "search",
  "metadata": {
    "embedding_mode": "hybrid",
    "embedding_batch_interval": 5,
    "embedding_batch_group_signals": false,
    "embedding_trigger_debounce_delay": 2
  }
}
```

### **Debounce Delay Options**

| Value | Delay | Use Case | Performance Impact |
|-------|-------|----------|-------------------|
| `1` | 1 second | **Aggressive** - Low latency requirements | Moderate reduction |
| `2` | 2 seconds | **Default** - Balanced approach | Good reduction |
| `5` | 5 seconds | **Conservative** - High efficiency | High reduction |
| `10` | 10 seconds | **Very Conservative** - Maximum efficiency | Maximum reduction |

### **How It Works**

1. **Trigger Received**: Increment counter, reset timer
2. **Timer Expires**: Process all accumulated triggers as one batch
3. **Batch Processing**: Run `RunBatch()` once for multiple triggers
4. **Reset**: Clear counter, wait for next trigger

### **Performance Impact**

| Scenario | Before | After | Improvement |
|----------|--------|-------|-------------|
| 100 inserts/second | 100 batch ops/sec | ~1-2 batch ops/sec | **50-100x reduction** |
| 1000 inserts/burst | 1000 batch ops | ~1 batch op | **1000x reduction** |
| CPU Usage | High | Low | **Significant reduction** |
| Database Load | High | Low | **Significant reduction** |

### **Trade-offs**

**Pros:**
- ✅ **Massive performance improvement**
- ✅ **Configurable delay**
- ✅ **Maintains real-time processing**
- ✅ **Reduces resource usage**

**Cons:**
- ⚠️ **Small latency increase** (1-10 seconds)
- ⚠️ **Burst processing** instead of continuous

### **Example Usage**

```go
mapping := &ProcessorMappingConfig{
    Name: "default",
    DatabaseID: 1,
    VectorStoreID: 1,
    Purpose: "search",
    Metadata: map[string]interface{}{
        "embedding_mode": "hybrid",
        "embedding_batch_interval": 5,
        "embedding_batch_group_signals": false,
        "embedding_trigger_debounce_delay": 2,  // 2 second debounce
    },
}
```

The debounced trigger mode gives you the best of both worlds: **real-time responsiveness** with **efficient batch processing**. You can tune the delay based on your latency requirements vs. performance needs.

---

## 6. Notes
- When building metadata, the code merges the per-signal fields with the `"generic"` fields for each document.
- Template content and metadata fields must match the actual fields present in your signals.
- Filters and metadata fields are case-sensitive and must match the schema.
- The `embedding_batch_group_signals` flag only accepts boolean values (`true`/`false`).
- The `embedding_trigger_debounce_delay` field accepts numeric values (in seconds) and defaults to 2 seconds if not specified.
- Supported metadata fields for embedding configuration:
  - `embedding_mode`: Processing mode (`"batch"`, `"trigger"`, `"hybrid"`)
  - `embedding_batch_interval`: Batch processing interval in seconds
  - `embedding_batch_group_signals`: Enable grouped signal processing (`true`/`false`)
  - `embedding_trigger_debounce_delay`: Debounce delay for triggers in seconds
  - `embedding_start_time`: Start time filter (Unix timestamp)
  - `embedding_stop_time`: Stop time filter (Unix timestamp)

---

For more details, see the code in `config_embedding_provider.go` and `embedding_worker.go`.

---

## TimescaleDB Maintenance Configuration

The TimescaleDB maintenance system allows you to configure automatic hypertable management for both OTEL signal tables and vector store tables. This feature helps improve query performance and manage data retention automatically.

### Overview

TimescaleDB maintenance is configured through the `metadata` field in both `config_otel_database` and `config_otel_processor` tables. The system supports:

- **Automatic chunk cleanup** for old data
- **Configurable retention periods**
- **Daily maintenance schedules**
- **Conditional maintenance** (can be disabled per schema/mapping)
- **Apache license compatible** (no licensing restrictions)

### Configuration Structure

Add a `timescaledb_maintenance` object to the `metadata` field:

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
| `retention_days` | integer | `30` | 1-365 | Number of days to retain data chunks |
| `maintenance_interval_hours` | integer | `24` | 1-168 | Hours between maintenance runs |

### Default Values

```json
{
  "enabled": true,
  "retention_days": 30,
  "maintenance_interval_hours": 24
}
```

### Configuration Examples

#### Basic Configuration (Defaults)
```json
{
  "timescaledb_maintenance": {
    "enabled": true,
    "retention_days": 30,
    "maintenance_interval_hours": 24
  }
}
```

#### High-Frequency Maintenance (Every 6 Hours)
```json
{
  "timescaledb_maintenance": {
    "enabled": true,
    "retention_days": 7,
    "maintenance_interval_hours": 6
  }
}
```

#### Long Retention (1 Year)
```json
{
  "timescaledb_maintenance": {
    "enabled": true,
    "retention_days": 365,
    "maintenance_interval_hours": 48
  }
}
```

#### Disabled Maintenance
```json
{
  "timescaledb_maintenance": {
    "enabled": false,
    "retention_days": 30,
    "maintenance_interval_hours": 24
  }
}
```

### Database Configuration

#### For OTEL Signal Tables (`config_otel_database`)

```sql
-- Insert configuration for OTEL signal tables
INSERT INTO config_otel_database (name, schema, filter, metadata, description) 
VALUES (
  'production-otel',
  'otel',
  '{"signal_type": "*"}'::jsonb,
  '{
    "timescaledb_maintenance": {
      "enabled": true,
      "retention_days": 30,
      "maintenance_interval_hours": 24
    }
  }'::jsonb,
  'Production OTEL database with TimescaleDB maintenance'
);
```

#### For Vector Store Tables (`config_otel_processor`)

```sql
-- Insert configuration for vector store tables
INSERT INTO config_otel_processor (name, database_id, vectorstore_id, template_id, processing_mode, metadata) 
VALUES (
  'production-vectors',
  1,
  1,
  1,
  'search',
  '{
    "timescaledb_maintenance": {
      "enabled": true,
      "retention_days": 30,
      "maintenance_interval_hours": 24
    }
  }'::jsonb
);
```

### Maintenance Schedule

The TimescaleDB maintenance system runs on a unified schedule:

- **Daily Maintenance**: Runs every 24 hours by default
- **Configurable Interval**: Adjustable via `maintenance_interval_hours`
- **Unified Operations**: Both OTEL and vector store tables maintained together

This unified approach ensures:
- Consistent maintenance across all table types
- Simplified scheduling and monitoring
- Efficient resource utilization

### What Maintenance Does

1. **Cleans Old Chunks**: Drops chunks older than `retention_days`
2. **Updates Statistics**: Refreshes table statistics with `ANALYZE`
3. **Logs Status**: Records maintenance activities and chunk information
4. **Provides Statistics**: Reports hypertable status and chunk information

### Conditional Maintenance

When `enabled: false`, the system:
- Skips chunk cleanup during maintenance runs
- Skips maintenance operations for that schema/mapping
- Continues to function normally without maintenance

### Validation Rules

The system validates configuration values:

- **`retention_days`**: Must be between 1 and 365 (1 day to 1 year)
- **`maintenance_interval_hours`**: Must be between 1 and 168 (1 hour to 1 week)

Invalid values will cause the system to fall back to defaults.

### Monitoring and Logging

The TimescaleDB maintenance system provides comprehensive logging:

```
INFO    Starting TimescaleDB maintenance manager
INFO    Running TimescaleDB maintenance for schemas schema_count=1
INFO    Running TimescaleDB maintenance for schema schema=otel retention_days=30
INFO    TimescaleDB maintenance completed for schema schema=otel
INFO    Cleaned up old chunks chunk_count=5
INFO    Updated statistics for hypertables hypertable_count=8
```

### Best Practices

1. **Start with Defaults**: Use the default configuration for most use cases
2. **Monitor Performance**: Adjust `retention_days` based on storage requirements
3. **Plan Retention**: Set `retention_days` based on compliance and storage requirements
4. **Consider Maintenance**: Higher `maintenance_interval_hours` reduces overhead but increases risk
5. **Test Configuration**: Validate TimescaleDB maintenance in non-production environments first

### Troubleshooting

#### Common Issues

1. **Chunks Not Cleaned**: Check that `enabled: true` and `retention_days > 0`
2. **Old Data Not Cleaned**: Verify `retention_days` is set appropriately
3. **Maintenance Not Running**: Check logs for errors and verify database connectivity
4. **Performance Issues**: Consider adjusting `maintenance_interval_hours`

#### Manual Maintenance

You can trigger maintenance manually:

```go
// Trigger TimescaleDB maintenance
err := maintenanceManager.TriggerTimescaleDBMaintenance(ctx)

// Run manual maintenance for specific schema
err := maintenanceManager.RunManualMaintenance(ctx, "otel")
```

### Integration with Existing Configuration

TimescaleDB maintenance works alongside other metadata configurations:

```json
{
  "description": "Production configuration with TimescaleDB maintenance",
  "timescaledb_maintenance": {
    "enabled": true,
    "retention_days": 30,
    "maintenance_interval_hours": 24
  },
  "other_config": "value"
}
```

The system automatically parses and validates the `timescaledb_maintenance` section while preserving other metadata fields.

--- 