# 📝 **Complete Template How-To Guide**

## 🎯 **Overview**

This guide provides comprehensive documentation for creating and customizing templates in the Trazr OTEL pipeline. Templates are used to generate searchable text content from OpenTelemetry signals (spans, logs, metrics, profiles) for embedding and analysis.

## 🏗️ **Template System Architecture**

The template system uses Go's `text/template` engine with extensive custom functions from:
- **Sprig**: 100+ utility functions for text manipulation, math, time, etc.
- **Custom Functions**: Specialized functions for OpenTelemetry data processing
- **Performance Optimizations**: Zero-allocation and memory-efficient functions

## 📊 **Available Signal Types**

### 1. **Spans** (`spans`)
Traces and spans with timing, status, and correlation data.

### 2. **Logs** (`logs`)
Log entries with severity, messages, and attributes.

### 3. **Metrics** (`gauge`, `counter`, `sum`, `histogram`, `summary`, `exponential_histogram`)
Various metric types with values, labels, and metadata.

### 4. **Profiles** (`profiles`)
Performance profiling data.

### 5. **Exemplars** (`exemplars`)
Trace-correlated metric data points with **metric-type specific access**.

---

## 🔧 **Template Functions Reference**

### **📈 Categorization Functions**

#### **Log Count Categorization**
```go
{{.log_count | categorizeLogCount}}
```
**Ranges:**
- 0-9: `"small"`
- 10-20: `"medium"`
- 21+: `"large"`

#### **Duration Categorization**
```go
{{.duration_ms | categorizeDuration}}
```
**Ranges:**
- 0-99ms: `"fast"`
- 100-1000ms: `"normal"`
- 1001ms+: `"slow"`

#### **Error Rate Categorization**
```go
{{.error_count | categorizeErrorRate .span_count}}
```
**Ranges:**
- 0-1%: `"low"`
- 1-5%: `"medium"`
- 5%+: `"high"`

#### **Span Count Categorization**
```go
{{.span_count | categorizeSpanCount}}
```
**Ranges:**
- 0-4: `"simple"`
- 5-20: `"medium"`
- 21+: `"complex"`

### **📊 Log and Event Counting Functions**

#### **Count Logs by Severity**
```go
{{.logs | countLogsBySeverity}}
```
Returns a map: `{"ERROR": 2, "WARN": 1, "INFO": 5, "DEBUG": 3}`

#### **Format Log Counts by Severity**
```go
{{.logs | formatLogCountsBySeverity}}
```
**Output:** `"11 logs (2 ERROR, 1 WARN, 5 INFO, 3 DEBUG)"`

#### **Count Events by Name**
```go
{{.events | countEventsByName}}
```
Returns a map: `{"db_query": 3, "cache_hit": 1, "http_request": 2}`

#### **Format Event Counts by Name**
```go
{{.events | formatEventCountsByName}}
```
**Output:** `"6 events (1 cache_hit, 3 db_query, 2 http_request)"`

### **⏰ Time Functions**

#### **Sprig Time Functions (All Available)**
```go
{{now | date "2006-01-02 15:04:05"}}                    // Current time
{{.timestamp | date "2006-01-02 15:04:05"}}            // Format timestamp
{{.start_time | ago}}                                   // Time since
{{.duration_seconds | duration}}                        // Convert to duration
{{.timestamp | dateModify "-1h" | date "15:04"}}       // Modify time
{{.timestamp | unixEpoch}}                              // Unix timestamp
{{.timestamp | htmlDate}}                               // HTML date format
```

#### **Custom Time Functions**
```go
{{.duration_ms | formatDuration}}                       // Format milliseconds
{{.duration_ms | categorizeDuration}}                   // Categorize duration
```

### **📝 Formatting Functions**

#### **Map and Array Formatting**
```go
{{.attributes | formatMap}}                             // Format map as key=value
{{.labels | formatLabels}}                              // Format labels
{{.buckets | formatArray}}                              // Format array
{{.quantiles | formatMap}}                              // Format quantiles
```

#### **Memory-Efficient Formatting**
```go
{{.bytes | formatBytes}}                                // Format bytes (B, KB, MB, etc.)
{{.percentage | formatPercentage}}                      // Format percentage
{{.duration_ms | formatDuration}}                       // Format duration
```

### **🔗 String Manipulation**

#### **Fast String Operations**
```go
{{.strings | fastJoin ", "}}                            // Join strings efficiently
{{fastConcat "prefix", .name, "suffix"}}               // Concatenate strings
{{.text | fastReplace "old" "new"}}                    // Replace text
{{.text | fastContains "substring"}}                   // Check if contains
```

### **🎛️ Conditional Functions**

#### **Conditional Logic**
```go
{{ifThenElse .condition "yes" "no"}}                    // Ternary operator
{{defaultIfEmpty .value "default"}}                     // Default if empty
{{coalesce .value1 .value2 .value3}}                    // First non-empty value
```

### **📊 Type-Specific Functions (Zero-Allocation)**

#### **Numeric Categorization**
```go
{{.count | categorizeInt}}                              // Categorize int
{{.count | categorizeInt64}}                            // Categorize int64
{{.count | categorizeFloat64}}                          // Categorize float64
```

---

## 📋 **Available Data Fields by Signal Type**

### **🔍 Spans Data Fields**

#### **Core Fields**
```go
.name                   // Span name/operation
.span_id               // Unique span identifier
.trace_id              // Trace identifier
.parent_span_id        // Parent span ID (if any)
.span_kind             // Span kind (SERVER, CLIENT, etc.)
.status_code           // Status code (0=OK, 1=ERROR)
.status_message        // Status message
.duration_ms           // Duration in milliseconds
.service_name          // Service name
```

#### **Timing Fields**
```go
.start_time            // Start timestamp
.end_time              // End timestamp
.duration              // Duration in nanoseconds
```

#### **Correlation Fields**
```go
.events                // Array of span events
.links                 // Array of span links
.logs                  // Correlated logs (from template context)
.exemplars             // Correlated exemplars (from template context)
.profiles              // Correlated profiles (from template context)
```

#### **Attribute Fields**
```go
.attributes            // Span attributes (map)
.resource_attributes   // Resource attributes (map)
.scope_attributes      // Scope attributes (map)
.scope_name            // Scope name
.scope_version         // Scope version
```

### **📝 Logs Data Fields**

#### **Core Fields**
```go
.severity_text         // Severity as text (ERROR, WARN, INFO, DEBUG)
.severity_number       // Severity as number (1-24)
.body                  // Log message body
.message               // Alternative message field
.service_name          // Service name
.timestamp             // Log timestamp
.observed_timestamp    // Observed timestamp
```

#### **Correlation Fields**
```go
.trace_id              // Associated trace ID
.span_id               // Associated span ID
```

#### **Attribute Fields**
```go
.attributes            // Log attributes (map)
.resource_attributes   // Resource attributes (map)
.scope_attributes      // Scope attributes (map)
.scope_name            // Scope name
.scope_version         // Scope version
```

### **📊 Metrics Data Fields**

#### **Core Fields**
```go
.name                  // Metric name
.description           // Metric description
.unit                  // Metric unit
.value                 // Metric value
.service_name          // Service name
```

#### **Histogram Fields**
```go
.count                 // Number of observations
.sum                   // Sum of observations
.buckets               // Histogram buckets
.min                   // Minimum value
.max                   // Maximum value
```

#### **Summary Fields**
```go
.count                 // Number of observations
.sum                   // Sum of observations
.quantiles             // Quantile values (map)
```

#### **Exponential Histogram Fields**
```go
.count                 // Number of observations
.sum                   // Sum of observations
.scale                 // Scale factor
.zero_count            // Zero bucket count
.positive_offset       // Positive bucket offset
.negative_offset       // Negative bucket offset
```

#### **Attribute Fields**
```go
.attributes            // Metric attributes (map)
.resource_attributes   // Resource attributes (map)
.scope_attributes      // Scope attributes (map)
.labels                // Metric labels (map)
```

### **📈 Exemplars Data Fields**

#### **Core Fields**
```go
.trace_id              // Associated trace ID
.span_id               // Associated span ID
.timestamp             // Exemplar timestamp
.value                 // Exemplar value
.filtered_attributes   // Filtered attributes (map)
```

#### **Attribute Fields**
```go
.attributes            // Exemplar attributes (map)
.resource_attributes   // Resource attributes (map)
.scope_attributes      // Scope attributes (map)
```

---

## 🎯 **Exemplars - Advanced Template Features**

### **📊 Metric-Type Specific Exemplars**

The system provides **1:1 table mapping** for exemplars by metric type, ensuring efficient database queries and explicit control over which exemplar tables are accessed.

#### **Available Exemplar Types**
```go
.exemplars_histogram    // Histogram exemplars only
.exemplars_summary      // Summary exemplars only  
.exemplars_gauge        // Gauge exemplars only
.exemplars_sum          // Sum exemplars only
.exemplars              // All exemplars (legacy, queries all tables)
```

#### **Exemplar Data Structure**
Each exemplar contains:
```go
.MetricName            // Name of the metric (e.g., "span.duration")
.Value                 // Exemplar value (e.g., 1049.0)
.Timestamp             // When the exemplar was recorded
.TraceID               // Associated trace ID
.SpanID                // Associated span ID
.Attributes            // Exemplar attributes
```

### **🔧 Template-Driven Exemplar Fetching**

#### **Conditional Exemplar Access**
```go
{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}
```

**Benefits:**
- ✅ **1:1 Table Mapping**: Only queries the gauge exemplars table
- ✅ **Conditional Rendering**: Only shows when exemplars exist
- ✅ **Performance**: No unnecessary database queries
- ✅ **Explicit Control**: Template determines what data to fetch

#### **Multiple Exemplar Types**
```go
{{if .exemplars_histogram}} Histogram: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}
{{if .exemplars_summary}} Summary: {{range .exemplars_summary}}{{.MetricName}}={{.Value}} {{end}}.{{end}}
{{if .exemplars_gauge}} Gauge: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}
{{if .exemplars_sum}} Sum: {{range .exemplars_sum}}{{.MetricName}}={{.Value}} {{end}}.{{end}}
```

### **⚙️ Exemplar Configuration**

#### **Template Metadata Configuration**
```go
"spans": {
    "name", "span_kind", "status_code", "events", "logs", 
    "exemplars_histogram", "exemplars_summary", "exemplars_gauge", "exemplars_sum",
    "exemplar_count_histogram", "exemplar_count_summary", "exemplar_count_gauge", "exemplar_count_sum"
}
```

#### **Worker Configuration**
The embedding worker automatically configures exemplar types based on template metadata:
```go
// Automatically extracted from template metadata
exemplar_types: ["histogram", "gauge", "summary", "sum"]
```

---

## 🎨 **Template Examples by Use Case**

### **🔍 Basic Span Template**
```go
`{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.`
```

**Output:**
```
user-service for operation process_request. Status: OK. Duration: 250ms. TraceID: abc123.
```

### **🔍 Enhanced Span Template with Events and Logs**
```go
`{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}`
```

**Output:**
```
user-service for operation process_request. Status: OK. Duration: 250ms. TraceID: abc123. Events: 4 events (2 db_query, 1 cache_hit, 1 http_request). Logs: 5 logs (1 ERROR, 1 WARN, 2 INFO, 1 DEBUG).
```

### **🔍 Span Template with Exemplars**
```go
`{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`
```

**Output:**
```
user-service for operation process_request. Status: OK. Duration: 250ms. TraceID: abc123. Events: 4 events (2 db_query, 1 cache_hit, 1 http_request). Logs: 5 logs (1 ERROR, 1 WARN, 2 INFO, 1 DEBUG). Gauge Exemplars: span.duration=250.0 cpu.usage=75.2 .
```

### **🔍 Comprehensive Span Template with All Exemplar Types**
```go
`{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}{{if .exemplars_histogram}} Histogram Exemplars: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_summary}} Summary Exemplars: {{range .exemplars_summary}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_sum}} Sum Exemplars: {{range .exemplars_sum}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`
```

**Output:**
```
user-service for operation process_request. Status: OK. Duration: 250ms. TraceID: abc123. Events: 4 events (2 db_query, 1 cache_hit, 1 http_request). Logs: 5 logs (1 ERROR, 1 WARN, 2 INFO, 1 DEBUG). Gauge Exemplars: span.duration=250.0 cpu.usage=75.2 . Histogram Exemplars: request.duration=245.0 .
```

### **📝 Basic Log Template**
```go
`Log Event from service {{.service_name}}. Severity: {{.severity_text | default "info"}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}. Attributes: {{.attributes | formatMap}}.`
```

**Output:**
```
Log Event from service user-service. Severity: ERROR. Message: Database connection failed. Attributes: user_id=12345, endpoint=/api/users, method=POST.
```

### **📊 Basic Metric Template**
```go
`Metric: The {{.metric_type}} "{{.name}}" from service {{.service_name}} recorded a value of {{.value}}{{if .unit}} {{.unit}}{{end}}. Labels: {{.labels | formatMap}}.`
```

**Output:**
```
Metric: The gauge "cpu_usage" from service user-service recorded a value of 75.5 %. Labels: instance=web-01, environment=production.
```

### **📈 Exemplar Template**
```go
`Exemplar from {{.service_name}} with value {{.value}} at {{.timestamp | date "15:04:05"}}. TraceID: {{.trace_id}}. Attributes: {{.filtered_attributes | formatMap}}.`
```

**Output:**
```
Exemplar from user-service with value 250ms at 14:30:25. TraceID: abc123. Attributes: endpoint=/api/users, method=POST.
```

### **🎯 Advanced Trace Template with Categorization**
```go
`Trace {{.trace_id}} from {{.service_name}}: {{.duration_ms | categorizeDuration}} performance ({{.duration_ms}}ms), {{.span_count | categorizeSpanCount}} complexity ({{.span_count}} spans), {{.log_count | categorizeLogCount}} logging ({{.log_count}} logs){{if and .error_count .span_count}}, {{.error_count | categorizeErrorRate .span_count}} error rate{{end}}`
```

**Output:**
```
Trace abc123 from user-service: normal performance (250ms), medium complexity (8 spans), medium logging (15 logs), high error rate
```

### **🔍 Conditional Logic Template**
```go
`{{if gt .log_count 10}}
High log volume detected: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{else if gt .log_count 5}}
Moderate log volume: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{else}}
Low log volume: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{end}}

{{if and (eq .status_code 1) (gt .duration_ms 1000)}}
Slow error detected: {{.duration_ms}}ms duration ({{.duration_ms | categorizeDuration}})
{{end}}`
```

**Output:**
```
High log volume detected: 15 logs (medium)

Slow error detected: 2500ms duration (slow)
```

### **⏰ Time-Aware Template**
```go
`Request processed at {{.timestamp | date "2006-01-02 15:04:05"}} with duration {{.duration_ms | formatDuration}}. Started {{.start_time | ago}} and completed {{.end_time | date "15:04:05"}}.`
```

**Output:**
```
Request processed at 2025-07-29 14:30:25 with duration 250ms. Started 1h0m0s ago and completed 14:30:25.
```

### **📊 Attribute Iteration Template**
```go
`{{.service_name}} operation {{.name}}{{range $key, $value := .attributes}} {{$key}}="{{$value}}"{{end}}{{if .resource_attributes}} resource:{{range $key, $value := .resource_attributes}} {{$key}}="{{$value}}"{{end}}{{end}}`
```

**Output:**
```
user-service operation process_request user_id="12345" endpoint="/api/users" method="POST" resource: environment="production" region="us-west-2"
```

### **📈 Exemplar-Focused Template**
```go
`{{.service_name}} operation {{.name}} with {{.duration_ms}}ms duration.{{if .exemplars_gauge}} Performance metrics: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}}{{if .Attributes}} ({{range $k, $v := .Attributes}}{{$k}}={{$v}} {{end}}){{end}} {{end}}.{{end}}{{if .exemplars_histogram}} Distribution metrics: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`
```

**Output:**
```
user-service operation process_request with 250ms duration. Performance metrics: span.duration=250.0 (endpoint=/api/users method=POST) cpu.usage=75.2 . Distribution metrics: request.duration=245.0 .
```

---

## 🗄️ **Database Configuration**

### **Default Template Installation**
The system automatically installs default templates when started:

```sql
-- Check existing templates
SELECT id, name, description FROM admin.config_processor_template;

-- View default template content
SELECT content FROM admin.config_processor_template WHERE name = 'default';
```

---

## 📋 **Template Metadata Configuration**

### **🎯 Overview**

Template metadata defines which fields are extracted from OpenTelemetry signals and made available for embedding. This controls what data gets stored in the vector database and can be used for filtering and searching.

### **🔧 Template Metadata Structure**

Template metadata is a JSON object that maps signal types to arrays of field names:

```json
{
  "spans": ["name", "service_name", "status_code", "duration", "trace_id", "span_id"],
  "logs": ["service_name", "level", "body", "message", "attributes"],
  "gauge": ["name", "service_name", "value", "attributes"],
  "sum": ["name", "service_name", "value", "unit", "attributes"],
  "histogram": ["name", "service_name", "count", "sum", "buckets", "attributes"],
  "summary": ["name", "service_name", "count", "sum", "attributes"],
  "exponential_histogram": ["name", "service_name", "count", "sum", "scale", "attributes"],
  "generic": ["service_name", "timestamp_ns"]
}
```

### **🎯 Field Access Methods**

#### **1. Direct Field Access**
Access fields directly from the signal data:
```json
{
  "spans": ["name", "service_name", "status_code", "duration", "trace_id", "span_id"]
}
```

#### **2. Dot Notation for Nested Fields**
Access specific keys within nested objects using dot notation:
```json
{
  "spans": [
    "name", 
    "service_name", 
    "status_code", 
    "duration", 
    "trace_id", 
    "span_id",
    "attributes.region",
    "attributes.http.method",
    "attributes.http.status_code",
    "attributes.user.id",
    "attributes.tenant.id"
  ]
}
```

#### **3. Generic Fields**
Fields that apply to all signal types:
```json
{
  "generic": ["service_name", "timestamp_ns"]
}
```

### **📊 Available Fields by Signal Type**

#### **🔍 Spans Fields**
```json
{
  "spans": [
    "name",                    // Span name/operation
    "service_name",            // Service name
    "status_code",             // Status code (0=OK, 1=ERROR)
    "duration",                // Duration in nanoseconds
    "trace_id",                // Trace identifier
    "span_id",                 // Unique span identifier
    "attributes",              // All span attributes
    "attributes.region",       // Specific region attribute
    "attributes.http.method",  // HTTP method
    "attributes.http.status_code", // HTTP status code
    "attributes.user.id",      // User ID
    "attributes.tenant.id",    // Tenant ID
    "attributes.environment",  // Environment
    "attributes.deployment.env", // Deployment environment
    "attributes.instance.id",  // Instance ID
    "attributes.correlation.id", // Correlation ID
    "resource_attributes",     // All resource attributes (nested)
    "resource_attributes.service_name",   // Service name (underscore format)
    "resource_attributes.service_version", // Service version (underscore format)
    "resource_attributes.deployment_environment", // Deployment environment (underscore format)
    "resource_attributes.cloud_provider", // Cloud provider (underscore format)
    "resource_attributes.cloud_region",   // Cloud region (underscore format)
    "resource_attributes.host_name",      // Host name (underscore format)
    "resource_attributes.trazr_random_seed" // Custom resource attribute (underscore format)
  ]
}
```

#### **📝 Logs Fields**
```json
{
  "logs": [
    "service_name",            // Service name
    "level",                   // Log level
    "body",                    // Log message body
    "message",                 // Alternative message field
    "attributes",              // All log attributes
    "attributes.region",       // Specific region attribute
    "attributes.environment",  // Environment
    "attributes.user.id",      // User ID
    "attributes.request.id",   // Request ID
    "attributes.session.id",   // Session ID
    "resource_attributes",     // All resource attributes
    "resource_attributes.service.name", // Service name from resource
    "resource_attributes.trazr.random.seed", // Custom resource attribute
    "resource_attributes.host.name", // Host name
    "resource_attributes.deployment.environment", // Deployment environment
    "resource_attributes.cloud.provider", // Cloud provider
    "resource_attributes.cloud.region" // Cloud region
  ]
}
```

#### **📊 Metrics Fields**
```json
{
  "gauge": [
    "name",                    // Metric name
    "service_name",            // Service name
    "value",                   // Metric value
    "attributes",              // All metric attributes
    "attributes.region",       // Specific region attribute
    "attributes.environment",  // Environment
    "attributes.instance",     // Instance
    "attributes.host",         // Host
    "resource_attributes",     // All resource attributes
    "resource_attributes.service.name", // Service name from resource
    "resource_attributes.trazr.random.seed", // Custom resource attribute
    "resource_attributes.host.name", // Host name
    "resource_attributes.deployment.environment", // Deployment environment
    "resource_attributes.cloud.provider", // Cloud provider
    "resource_attributes.cloud.region" // Cloud region
  ],
  "sum": [
    "name",                    // Metric name
    "service_name",            // Service name
    "value",                   // Metric value
    "unit",                    // Metric unit
    "attributes",              // All metric attributes
    "attributes.region",       // Specific region attribute
    "attributes.environment",  // Environment
    "resource_attributes",     // All resource attributes
    "resource_attributes.service.name", // Service name from resource
    "resource_attributes.trazr.random.seed", // Custom resource attribute
    "resource_attributes.host.name", // Host name
    "resource_attributes.deployment.environment", // Deployment environment
    "resource_attributes.cloud.provider", // Cloud provider
    "resource_attributes.cloud.region" // Cloud region
  ],
  "histogram": [
    "name",                    // Metric name
    "service_name",            // Service name
    "count",                   // Number of observations
    "sum",                     // Sum of observations
    "buckets",                 // Histogram buckets
    "attributes",              // All metric attributes
    "attributes.region",       // Specific region attribute
    "attributes.environment",  // Environment
    "resource_attributes",     // All resource attributes
    "resource_attributes.service.name", // Service name from resource
    "resource_attributes.trazr.random.seed", // Custom resource attribute
    "resource_attributes.host.name", // Host name
    "resource_attributes.deployment.environment", // Deployment environment
    "resource_attributes.cloud.provider", // Cloud provider
    "resource_attributes.cloud.region" // Cloud region
  ],
  "summary": [
    "name",                    // Metric name
    "service_name",            // Service name
    "count",                   // Number of observations
    "sum",                     // Sum of observations
    "attributes",              // All metric attributes
    "attributes.region",       // Specific region attribute
    "attributes.environment",  // Environment
    "resource_attributes",     // All resource attributes
    "resource_attributes.service.name", // Service name from resource
    "resource_attributes.trazr.random.seed", // Custom resource attribute
    "resource_attributes.host.name", // Host name
    "resource_attributes.deployment.environment", // Deployment environment
    "resource_attributes.cloud.provider", // Cloud provider
    "resource_attributes.cloud.region" // Cloud region
  ],
  "exponential_histogram": [
    "name",                    // Metric name
    "service_name",            // Service name
    "count",                   // Number of observations
    "sum",                     // Sum of observations
    "scale",                   // Scale factor
    "attributes",              // All metric attributes
    "attributes.region",       // Specific region attribute
    "attributes.environment",  // Environment
    "resource_attributes",     // All resource attributes
    "resource_attributes.service.name", // Service name from resource
    "resource_attributes.trazr.random.seed", // Custom resource attribute
    "resource_attributes.host.name", // Host name
    "resource_attributes.deployment.environment", // Deployment environment
    "resource_attributes.cloud.provider", // Cloud provider
    "resource_attributes.cloud.region" // Cloud region
  ]
}
```

### **🔧 How Dot Notation Works**

The system uses the `getNestedField` function to access nested fields:

```go
// Example: accessing attributes.region
func getNestedField(data map[string]interface{}, field string) (interface{}, bool) {
    if val, ok := data[field]; ok {
        return val, true
    }
    parts := strings.Split(field, ".")
    var current interface{} = data
    for _, part := range parts {
        m, ok := current.(map[string]interface{})
        if !ok {
            return nil, false
        }
        current, ok = m[part]
        if !ok {
            return nil, false
        }
    }
    return current, true
}
```

### **📋 Example Results**

#### **With Full Attributes and Resource Attributes**
```json
{
  "name": "GET /api/users",
  "service_name": "trazr-gen",
  "status_code": 1,
  "duration": 150000000,
  "trace_id": "cf43e45e155d6585c8233e54820f55b9",
  "span_id": "fe3879291dac65bb",
  "attributes": {
    "region": "us-west-2",
    "user.id": "user-1000",
    "http.url": "/api/users",
    "tenant.id": "tenant-100",
    "custom.key": "v1.0.0",
    "http.route": "/api/users",
    "request.id": "req-0-9000",
    "session.id": "sess-0-0",
    "app.version": "production",
    "http.method": "GET",
    "instance.id": "instance-1",
    "correlation.id": "corr-0-0",
    "deployment.env": "us-east-1",
    "http.status_code": "200"
  },
  "resource_attributes": {
    "service.name": "trazr-gen",
    "trazr.random.seed": "12345",
    "host.name": "web-server-01",
    "deployment.environment": "production",
    "cloud.provider": "aws",
    "cloud.region": "us-west-2"
  }
}
```

#### **With Underscore Format for Resource Attributes**
```json
{
  "name": "GET /api/users",
  "service_name": "trazr-gen",
  "status_code": 1,
  "duration": 150000000,
  "trace_id": "cf43e45e155d6585c8233e54820f55b9",
  "span_id": "fe3879291dac65bb",
  "attributes.region": "us-west-2",
  "attributes.http.method": "GET",
  "attributes.http.status_code": "200",
  "attributes.user.id": "user-1000",
  "attributes.tenant.id": "tenant-100",
  "resource_attributes.service_name": "trazr-gen",
  "resource_attributes.trazr_random_seed": "12345",
  "resource_attributes.host_name": "web-server-01",
  "resource_attributes.deployment_environment": "production"
}
```

**Note**: The underscore format `resource_attributes.trazr_random_seed` automatically maps to the database key `trazr.random.seed`.

### **🏗️ Resource Attributes Overview**

Resource attributes are metadata that describe the resource (service, host, deployment) that generated the telemetry data. They are typically set once per resource and remain constant across all signals from that resource.

#### **Hybrid Approach: Nested + Flattened**

The system uses a **hybrid approach** that provides both nested and flattened access to resource attributes:

```json
// Original nested structure (preserved)
{
  "resource_attributes": {
    "service.name": "trazr-gen",
    "service.version": "1.0.0",
    "deployment.environment": "production",
    "cloud.provider": "aws",
    "cloud.region": "us-west-2",
    "trazr.random.seed": "12345"
  }
}

// Flattened fields (added for easy access)
{
  "resource_service_name": "trazr-gen",
  "resource_service_version": "1.0.0", 
  "resource_deployment_environment": "production",
  "resource_cloud_provider": "aws",
  "resource_cloud_region": "us-west-2",
  "resource_trazr_random_seed": "12345"
}
```

#### **Benefits of Hybrid Approach**
- **✅ Performance**: Flattened fields enable fast direct access
- **✅ Compatibility**: Preserves original OpenTelemetry structure
- **✅ Simplicity**: No complex dot notation needed
- **✅ Flexibility**: Can use either approach as needed
- **✅ Indexing**: Can create B-tree indexes on flattened fields

#### **Common Resource Attributes**
```json
{
  "service.name": "trazr-gen",              // Service name
  "service.version": "1.0.0",               // Service version
  "service.namespace": "production",        // Service namespace
  "host.name": "web-server-01",             // Host name
  "host.type": "aws.ec2.instance",          // Host type
  "deployment.environment": "production",   // Deployment environment
  "cloud.provider": "aws",                  // Cloud provider
  "cloud.region": "us-west-2",              // Cloud region
  "cloud.availability_zone": "us-west-2a",  // Availability zone
  "trazr.random.seed": "12345",             // Custom resource attribute
  "custom.resource.id": "resource-001"      // Custom resource ID
}
```

#### **Accessing Resource Attributes with Dot Notation**
When accessing resource attributes in template metadata, you can use underscore format for keys that contain dots:

```json
{
  "spans": [
    "name", "service_name", "status_code", "duration", "trace_id", "span_id",
    "resource_attributes",                    // All resource attributes
    "resource_attributes.service_name",       // Service name from resource
    "resource_attributes.trazr_random_seed",  // Custom resource attribute (uses underscores)
    "resource_attributes.host_name",          // Host name
    "resource_attributes.deployment_environment", // Deployment environment
    "resource_attributes.cloud_provider",     // Cloud provider
    "resource_attributes.cloud_region"        // Cloud region
  ]
}
```

**Important**: The system automatically maps underscores to dots for resource attributes:

1. **Template Format**: Use `resource_attributes.trazr_random_seed` (underscores)
2. **Database Lookup**: System finds `trazr.random.seed` (dots) in JSONB
3. **Automatic Mapping**: Underscores in template → dots in database keys

This works with JSONB structures like:
- `{"resource_attributes": {"service.name": "trazr-gen"}}` (database)
- `{"resource_attributes": {"trazr.random.seed": "12345"}}` (database)

#### **Benefits of Resource Attributes**
- **Resource Identification**: Identify which service/host generated the data
- **Environment Filtering**: Filter by deployment environment or cloud region
- **Infrastructure Monitoring**: Track resource-level metrics and health
- **Custom Metadata**: Store resource-specific configuration or identifiers

### **⚙️ Configuration Examples**

#### **1. Minimal Configuration**
Extract only essential fields:
```json
{
  "spans": ["name", "service_name", "status_code", "duration"],
  "logs": ["service_name", "level", "body"],
  "gauge": ["name", "service_name", "value"],
  "generic": ["timestamp_ns"]
}
```

#### **2. Standard Configuration**
Include common fields and attributes:
```json
{
  "spans": ["name", "service_name", "status_code", "duration", "trace_id", "span_id", "attributes"],
  "logs": ["service_name", "level", "body", "message", "attributes"],
  "gauge": ["name", "service_name", "value", "attributes"],
  "sum": ["name", "service_name", "value", "unit", "attributes"],
  "generic": ["service_name", "timestamp_ns"]
}
```

#### **3. Advanced Configuration**
Include specific attribute keys for filtering:
```json
{
  "spans": [
    "name", "service_name", "status_code", "duration", "trace_id", "span_id",
    "attributes.region", "attributes.http.method", "attributes.http.status_code",
    "attributes.user.id", "attributes.tenant.id", "attributes.environment"
  ],
  "logs": [
    "service_name", "level", "body", "message",
    "attributes.region", "attributes.environment", "attributes.user.id"
  ],
  "gauge": [
    "name", "service_name", "value",
    "attributes.region", "attributes.environment", "attributes.instance"
  ],
  "generic": ["service_name", "timestamp_ns"]
}
```

### **🔍 Benefits of Selective Field Extraction**

#### **1. Performance Optimization**
- **Reduced Storage**: Only store fields you need
- **Faster Queries**: Smaller metadata objects
- **Lower Memory Usage**: Less data in memory

#### **2. Focused Search**
- **Specific Filtering**: Filter by `attributes.region = "us-west-2"`
- **User Tracking**: Search by `attributes.user.id`
- **Environment Isolation**: Group by `attributes.environment`

#### **3. Template Flexibility**
- **Consistent Field Names**: `attributes.region` always available
- **Predictable Structure**: Same fields across all signals
- **Easy Maintenance**: Clear field mapping

### **🛠️ Database Configuration**

#### **View Current Template Metadata**
```sql
SELECT id, name, template_metadata, description 
FROM admin.config_processor_template;
```

#### **Update Template Metadata**
```sql
UPDATE admin.config_processor_template 
SET template_metadata = '{
  "spans": ["name", "service_name", "status_code", "duration", "trace_id", "span_id", "attributes.region", "attributes.http.method"],
  "logs": ["service_name", "level", "body", "attributes.region"],
  "gauge": ["name", "service_name", "value", "attributes.region"],
  "generic": ["service_name", "timestamp_ns"]
}'
WHERE name = 'default';
```

#### **Create Custom Template with Metadata**
```sql
INSERT INTO admin.config_processor_template (name, content, template_metadata, description) VALUES (
    'region-focused-template',
    '{
        "spans": "{{.service_name}} operation {{.name}} in region {{.attributes.region}} took {{.duration}}ns",
        "logs": "{{.level}} log from {{.service_name}} in {{.attributes.region}}: {{.body}}",
        "gauge": "Gauge {{.name}} in {{.attributes.region}} = {{.value}}"
    }',
    '{
        "spans": ["name", "service_name", "duration", "attributes.region"],
        "logs": ["level", "service_name", "body", "attributes.region"],
        "gauge": ["name", "value", "attributes.region"],
        "generic": ["service_name"]
    }',
    'Template focused on regional data with specific attribute extraction'
);
```

### **🎯 Best Practices**

#### **1. Start with Essential Fields**
```json
{
  "spans": ["name", "service_name", "status_code", "duration"],
  "logs": ["service_name", "level", "body"],
  "gauge": ["name", "service_name", "value"]
}
```

#### **2. Add Common Attributes**
```json
{
  "spans": ["name", "service_name", "status_code", "duration", "attributes.region", "attributes.environment"],
  "logs": ["service_name", "level", "body", "attributes.region"],
  "gauge": ["name", "service_name", "value", "attributes.region"]
}
```

#### **3. Include Correlation Fields**
```json
{
  "spans": ["name", "service_name", "status_code", "duration", "trace_id", "span_id", "attributes.region"],
  "logs": ["service_name", "level", "body", "trace_id", "span_id", "attributes.region"],
  "gauge": ["name", "service_name", "value", "attributes.region"]
}
```

#### **4. Optimize for Your Use Case**
- **Monitoring**: Focus on metrics and status codes
- **Debugging**: Include trace IDs and correlation IDs
- **Business Analytics**: Include user IDs and tenant IDs
- **Infrastructure**: Include region, environment, and instance IDs

### **🔧 Troubleshooting**

#### **Common Issues**

1. **Field Not Found**
   - Check field name spelling
   - Verify field exists in signal data
   - Use dot notation for nested fields

2. **Empty Values**
   - Field may not exist in all signals
   - Use conditional rendering in templates
   - Provide default values

3. **Performance Issues**
   - Reduce number of extracted fields
   - Use specific attribute keys instead of full attributes
   - Remove unused fields

#### **Debugging Tips**

1. **Check Available Fields**
   ```sql
   SELECT DISTINCT jsonb_object_keys(attributes) 
   FROM otel.spans 
   LIMIT 10;
   ```

2. **Verify Field Values**
   ```sql
   SELECT name, attributes->>'region' as region
   FROM otel.spans 
   WHERE attributes ? 'region'
   LIMIT 5;
   ```

3. **Check Resource Attributes**
   ```sql
   -- Check what resource attributes are available
   SELECT DISTINCT jsonb_object_keys(resource_attributes) 
   FROM otel.spans 
   WHERE resource_attributes IS NOT NULL
   LIMIT 10;
   
   -- Check specific resource attribute values
   SELECT name, resource_attributes->>'service.name' as service_name,
          resource_attributes->>'trazr.random.seed' as random_seed
   FROM otel.spans 
   WHERE resource_attributes ? 'service.name'
   LIMIT 5;
   ```

4. **Test Template Metadata**
   ```bash
   # Test with sample data
   go test -v ./pkg/repository -run TestTemplateMetadata
   ```

#### **Common Resource Attribute Issues**

1. **Wrong Key Format**
   ```json
   // ❌ Wrong - using underscores instead of dots
   "resource_attributes.trazr_random_seed"
   
   // ✅ Correct - using dots as they appear in JSONB
   "resource_attributes.trazr.random.seed"
   ```

2. **Understanding Key Structure**
   ```json
   // Database structure:
   {
     "resource_attributes": {
       "service.name": "trazr-gen",
       "trazr.random.seed": "12345"
     }
   }
   
   // Template metadata should use:
   "resource_attributes.service.name"      // Finds "service.name" key
   "resource_attributes.trazr.random.seed" // Finds "trazr.random.seed" key
   ```

2. **Missing Resource Attributes**
   ```sql
   -- Check if resource_attributes field exists and has data
   SELECT COUNT(*) as total_spans,
          COUNT(CASE WHEN resource_attributes IS NOT NULL THEN 1 END) as with_resource_attrs
   FROM otel.spans;
   ```

3. **Nested Object Access**
   ```json
   // For deeply nested objects like:
   // {"resource_attributes": {"custom": {"config": {"version": "1.0"}}}}
   
   // Use the full path:
   "resource_attributes.custom.config.version"
   ```

### **Custom Template Creation**
```sql
INSERT INTO admin.config_processor_template (name, content, description) VALUES (
    'my-custom-template',
    '{
        "spans": "{{.service_name}} operation {{.name}} took {{.duration_ms}}ms with status {{.status_code}}{{if .exemplars_gauge}}{{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}{{end}}",
        "logs": "{{.severity_text}} log from {{.service_name}}: {{.body}}",
        "gauge": "Gauge {{.name}} = {{.value}}{{if .unit}} {{.unit}}{{end}}"
    }',
    'My custom template with exemplar support'
);
```

### **Template Update**
```sql
UPDATE admin.config_processor_template 
SET content = '{"spans": "Updated template with exemplars: {{if .exemplars_gauge}}{{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}{{end}}"}'
WHERE name = 'my-custom-template';
```

---

## 🚀 **Performance Best Practices**

### **1. Use Type-Specific Functions**
```go
// ✅ Good - Zero allocation
{{.count | categorizeInt}}

// ❌ Avoid - Interface boxing
{{.count | categorizeLogCount}}
```

### **2. Use Fast String Operations**
```go
// ✅ Good - Memory efficient
{{.strings | fastJoin ", "}}

// ❌ Avoid - Multiple allocations
{{range $i, $s := .strings}}{{if $i}}, {{end}}{{$s}}{{end}}
```

### **3. Minimize Conditional Logic**
```go
// ✅ Good - Simple conditionals
{{if .value}}{{.value}}{{else}}default{{end}}

// ❌ Avoid - Complex nested logic
{{if .condition1}}{{if .condition2}}{{if .condition3}}...{{end}}{{end}}{{end}}
```

### **4. Use Appropriate Data Types**
```go
// ✅ Good - Direct field access
{{.duration_ms}}

// ❌ Avoid - Complex calculations in templates
{{div .duration 1000000}}
```

### **5. Exemplar-Specific Best Practices**
```go
// ✅ Good - Type-specific exemplar access
{{if .exemplars_gauge}}{{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}{{end}}

// ❌ Avoid - Generic exemplar access (queries all tables)
{{if .exemplars}}{{range .exemplars}}{{.MetricName}}={{.Value}} {{end}}{{end}}
```

---

## 🧪 **Testing Templates**

### **Template Validation**
```bash
# Test template syntax
go test -v ./pkg/repository/template/content

# Test specific template functions
go test -v ./pkg/util -run TestTemplateFunctions
```

### **Performance Benchmarking**
```bash
# Benchmark template functions
go test -v ./pkg/util -bench=BenchmarkCountLogsBySeverity -benchmem

# Benchmark full template rendering
go test -v ./pkg/repository -bench=BenchmarkTemplateRendering
```

### **Exemplar Template Testing**
```bash
# Test exemplar-specific functions
go test -v ./pkg/repository -run TestExemplarTemplateRendering

# Test template analysis
go test -v ./pkg/repository -run TestTemplateAnalysis
```

---

## 🔧 **Troubleshooting**

### **Common Issues**

#### **1. Template Not Rendering**
- Check template syntax with `go test`
- Verify all referenced fields exist in data
- Use `default` function for optional fields

#### **2. Performance Issues**
- Use type-specific functions when possible
- Avoid complex calculations in templates
- Use fast string operations for concatenation

#### **3. Missing Data**
- Check field names match data structure
- Use conditional rendering with `{{if .field}}`
- Provide defaults with `{{.field | default "value"}}`

#### **4. Exemplar Issues**
- Ensure exemplar types are configured in template metadata
- Use metric-type specific variables (`.exemplars_gauge`, not `.exemplars`)
- Check that exemplar tables exist and contain data

### **Debugging Tips**

#### **1. Enable Template Debugging**
```go
// Add debug logging to see template data
{{printf "Debug: %+v" .}}
```

#### **2. Check Available Fields**
```go
// List all available fields
{{range $key, $value := .}}{{printf "%s: %v\n" $key $value}}{{end}}
```

#### **3. Validate Data Types**
```go
// Check data type
{{printf "Type: %T, Value: %v" .field .field}}
```

#### **4. Debug Exemplar Data**
```go
// Check exemplar availability
{{if .exemplars_gauge}}Gauge exemplars: {{len .exemplars_gauge}}{{else}}No gauge exemplars{{end}}
{{if .exemplars_histogram}}Histogram exemplars: {{len .exemplars_histogram}}{{else}}No histogram exemplars{{end}}
```

---

## 📚 **Additional Resources**

- **Sprig Documentation**: https://masterminds.github.io/sprig/
- **Go Template Documentation**: https://golang.org/pkg/text/template/
- **OpenTelemetry Specification**: https://opentelemetry.io/docs/specs/
- **Exemplar Documentation**: https://opentelemetry.io/docs/specs/otel/metrics/sdk/#exemplar

---

## 🎯 **Quick Reference**

### **Most Common Template Patterns**

```go
// Basic span
`{{.service_name}} {{.name}} {{.duration_ms}}ms {{.status_code}}`

// With correlation
`{{.service_name}} {{.name}} {{.duration_ms}}ms {{.status_code}}{{if .logs}} {{.logs | formatLogCountsBySeverity}}{{end}}`

// With categorization
`{{.service_name}} {{.name}} {{.duration_ms | categorizeDuration}} {{.log_count | categorizeLogCount}}`

// With attributes
`{{.service_name}} {{.name}}{{range $k, $v := .attributes}} {{$k}}="{{$v}}"{{end}}`

// With exemplars
`{{.service_name}} {{.name}} {{.duration_ms}}ms{{if .exemplars_gauge}}{{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}{{end}}`
```

### **Exemplar Quick Reference**

```go
// Gauge exemplars only
{{if .exemplars_gauge}}{{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}{{end}}

// Histogram exemplars only  
{{if .exemplars_histogram}}{{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}{{end}}

// Summary exemplars only
{{if .exemplars_summary}}{{range .exemplars_summary}}{{.MetricName}}={{.Value}} {{end}}{{end}}

// Sum exemplars only
{{if .exemplars_sum}}{{range .exemplars_sum}}{{.MetricName}}={{.Value}} {{end}}{{end}}

// All exemplar types
{{if .exemplars_histogram}}H:{{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}{{end}}{{if .exemplars_gauge}}G:{{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}{{end}}
```

This guide covers all available template functions, data fields, and use cases for creating effective templates in the Trazr OTEL pipeline, including comprehensive exemplar support with metric-type specific access. 