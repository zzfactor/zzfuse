# Metrics Package

This package centralizes all Prometheus metrics for the project. It provides definitions, registration, and documentation for metrics related to Notifier, Database, and HTTP subsystems.

## How to Register and Use Metrics

### 1. Import the Metrics Package

Import the metrics package in your main application or service:

```go
import "your_project/pkg/metrics"
```

### 2. Register All Metrics at Startup

Call `metrics.Register()` once at application startup (before serving requests):

```go
func main() {
    metrics.Register()
    // ... your other initialization ...
}
```

### 3. Use Metrics in Your Code

Increment or observe metrics as needed in your business logic. For example:

```go
// Increment when a notification is delivered
metrics.NotifierDelivered.WithLabelValues("topic_name").Inc()

// Increment when a DB query is executed
metrics.DBQueryTotal.WithLabelValues("select", "success").Inc()

// Observe HTTP request duration
metrics.HTTPRequestDuration.WithLabelValues("GET", "200").Observe(durationSeconds)
```

### 4. Expose the /metrics Endpoint

Expose the `/metrics` endpoint using Prometheus' HTTP handler so Prometheus can scrape your metrics:

```go
import (
    "net/http"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
    metrics.Register()
    http.Handle("/metrics", promhttp.Handler())
    // ... your other handlers ...
    http.ListenAndServe(":8080", nil)
}
```

## API Functions for Metrics Access

The metrics package now provides comprehensive API functions for programmatic access to metrics data, making it ready for serving metrics via API endpoints.

### Core API Functions

#### GetAllMetrics()
Returns all metrics data in a unified format suitable for API responses.

```go
func GetAllMetrics() map[string]interface{}
```

**Returns:** Complete metrics data including memory, storage, database, HTTP, notifier, and embedding metrics.

#### GetMetricsJSON()
Returns all metrics as formatted JSON.

```go
func GetMetricsJSON() ([]byte, error)
```

**Returns:** JSON-formatted metrics data with proper indentation.

#### GetPrometheusMetrics()
Returns metrics in standard Prometheus text format.

```go
func GetPrometheusMetrics() (string, error)
```

**Returns:** Prometheus-formatted metrics string.

#### GetMetricsSummary()
Returns a summary of key metrics for quick overview.

```go
func GetMetricsSummary() map[string]interface{}
```

**Returns:** Summary of critical metrics including memory usage, storage status, and runtime statistics.

### Category-Specific API Functions

#### GetMemoryMetrics()
Returns memory and runtime statistics.

```go
func GetMemoryMetrics() map[string]interface{}
```

**Returns:** Memory allocation, heap statistics, garbage collection info, and runtime metrics.

#### GetStorageMetrics()
Returns storage and I/O statistics.

```go
func GetStorageMetrics() map[string]interface{}
```

**Returns:** Disk usage, I/O statistics, and storage performance metrics.

#### GetDatabaseMetrics()
Returns database operation statistics.

```go
func GetDatabaseMetrics() map[string]interface{}
```

**Returns:** Query statistics, connection pool status, and database performance metrics.

#### GetHTTPMetrics()
Returns HTTP request statistics.

```go
func GetHTTPMetrics() map[string]interface{}
```

**Returns:** Request counts, latency statistics, and HTTP performance metrics.

#### GetNotifierMetrics()
Returns notifier operation statistics.

```go
func GetNotifierMetrics() map[string]interface{}
```

**Returns:** Notification delivery statistics, topic-based metrics, and notifier performance.

#### GetEmbeddingMetrics()
Returns embedding operation statistics.

```go
func GetEmbeddingMetrics() map[string]interface{}
```

**Returns:** Job statistics, signal processing metrics, vector store operations, and embedding performance.

### Example API Usage

```go
// Get all metrics for a comprehensive API response
allMetrics := metrics.GetAllMetrics()

// Get metrics as JSON for API responses
jsonData, err := metrics.GetMetricsJSON()
if err != nil {
    // Handle error
}

// Get Prometheus format for compatibility
prometheusData, err := metrics.GetPrometheusMetrics()
if err != nil {
    // Handle error
}

// Get summary for dashboard widgets
summary := metrics.GetMetricsSummary()

// Get specific category metrics
memoryStats := metrics.GetMemoryMetrics()
storageStats := metrics.GetStorageMetrics()
```

### API Response Format

The API functions return structured data in the following format:

```json
{
  "memory": {
    "memory": {
      "alloc": 1234567,
      "heap_objects": 12345,
      "goroutines": 42
    },
    "runtime": {
      "goroutines": 42,
      "threads": 8,
      "cpus": 8
    },
    "timestamp": 1640995200
  },
  "storage": {
    "disk_usage": {
      "/": {
        "total": 1000000000,
        "free": 500000000,
        "used_percent": 50.0
      }
    },
    "io_stats": {
      "sda": {
        "read_bytes": 1000000,
        "write_bytes": 2000000
      }
    },
    "timestamp": 1640995200
  },
  "database": {
    "queries": {
      "total_queries": "use_prometheus_endpoint",
      "latency_stats": "use_prometheus_endpoint"
    }
  },
  "http": {
    "requests": {
      "total": "use_prometheus_endpoint",
      "by_method": "use_prometheus_endpoint"
    }
  },
  "notifier": {
    "notifications": {
      "delivered": "use_prometheus_endpoint",
      "dropped": "use_prometheus_endpoint"
    }
  },
  "embedding": {
    "jobs": {
      "total": "use_prometheus_endpoint",
      "active": "use_prometheus_endpoint"
    }
  },
  "timestamp": 1640995200,
  "version": "1.0.0"
}
```

### Thread Safety

All API functions are thread-safe and use read-write mutexes to ensure concurrent access safety. The functions can be called from multiple goroutines without additional synchronization.

### Error Handling

The API functions handle errors gracefully:
- Missing collectors return error information in the response
- JSON marshaling errors are returned as errors
- Prometheus gathering errors are returned as errors
- Invalid states are handled with appropriate error messages

## Current Metrics Coverage

### Implemented Metrics

#### Notifier
- `notifier_notifications_delivered_total{topic}`: Counter. Total notifications delivered per topic.
- `notifier_notifications_dropped_total{topic}`: Counter. Total notifications dropped per topic.

#### Database
- `db_query_total{operation, status}`: Counter. Total number of database queries by operation and status.
- `db_query_latency_seconds{operation}`: Histogram. Database query latency in seconds by operation.

#### HTTP
- `http_requests_total{method, code}`: Counter. Total number of HTTP requests by method and response code.
- `http_request_duration_seconds{method, code}`: Histogram. HTTP request duration in seconds by method and response code.

#### Embedding
- `embedding_jobs_total{status, signal_type, processing_mode}`: Counter. Total number of embedding jobs by status and signal type.
- `embedding_jobs_duration_seconds{signal_type, processing_mode}`: Histogram. Embedding job duration in seconds by signal type.
- `embedding_signals_total{status, signal_type}`: Counter. Total number of signals processed by status and signal type.
- `embedding_tokens_total{signal_type, token_type}`: Counter. Total number of tokens processed by signal type.
- `embedding_batches_total{status, signal_type}`: Counter. Total number of batches processed by status and signal type.
- `embedding_batch_size{signal_type}`: Histogram. Batch sizes by signal type.
- `embedding_throughput_signals_per_second{signal_type}`: Gauge. Signals processed per second by signal type.
- `embedding_errors_total{error_type, signal_type}`: Counter. Total number of errors by error type and signal type.
- `embedding_retries_total{signal_type}`: Counter. Total number of retries by signal type.
- `embedding_vectorstore_writes_total{status, vectorstore_provider}`: Counter. Total number of vector store writes by status.
- `embedding_vectorstore_latency_seconds{vectorstore_provider}`: Histogram. Vector store write latency in seconds.
- `embedding_template_renders_total{status, template_type}`: Counter. Total number of template renders by status.
- `embedding_template_latency_seconds{template_type}`: Histogram. Template render latency in seconds.
- `embedding_quality_score{signal_type}`: Histogram. Embedding quality scores (0.0 to 1.0).
- `embedding_memory_usage_bytes{signal_type, memory_type}`: Gauge. Embedding-specific memory usage in bytes. Includes:
  - `current`: Current memory usage for embedding operations
- `embedding_active_jobs{signal_type, worker_id}`: Gauge. Currently active embedding jobs by signal type.
- `embedding_worker_uptime_seconds{worker_id, signal_type}`: Gauge. Worker uptime in seconds by worker ID.

#### Memory & Runtime
- `memory_alloc_bytes`: Gauge. Currently allocated memory in bytes.
- `memory_total_alloc_bytes`: Gauge. Total memory allocated since start in bytes.
- `memory_sys_bytes`: Gauge. Total system memory in bytes.
- `memory_heap_alloc_bytes`: Gauge. Heap allocated memory in bytes.
- `memory_heap_sys_bytes`: Gauge. Heap system memory in bytes.
- `memory_heap_idle_bytes`: Gauge. Heap idle memory in bytes.
- `memory_heap_inuse_bytes`: Gauge. Heap in-use memory in bytes.
- `memory_heap_released_bytes`: Gauge. Heap released memory in bytes.
- `memory_heap_objects`: Gauge. Number of heap objects.
- `memory_stack_inuse_bytes`: Gauge. Stack in-use memory in bytes.
- `memory_stack_sys_bytes`: Gauge. Stack system memory in bytes.
- `memory_mspan_inuse_bytes`: Gauge. MSpan in-use memory in bytes.
- `memory_mspan_sys_bytes`: Gauge. MSpan system memory in bytes.
- `memory_mcache_inuse_bytes`: Gauge. MCache in-use memory in bytes.
- `memory_mcache_sys_bytes`: Gauge. MCache system memory in bytes.
- `memory_buck_hash_sys_bytes`: Gauge. Bucket hash table memory in bytes.
- `memory_gc_sys_bytes`: Gauge. GC metadata memory in bytes.
- `memory_other_sys_bytes`: Gauge. Other system memory in bytes.
- `gc_next_target_bytes`: Gauge. Next GC target in bytes.
- `gc_last_time_seconds`: Gauge. Last GC time in seconds since epoch.
- `gc_pause_total_seconds`: Gauge. Total GC pause time in seconds.
- `gc_cycles_total`: Counter. Total number of GC cycles.
- `gc_forced_cycles_total`: Counter. Total number of forced GC cycles.
- `goroutines`: Gauge. Current number of goroutines.
- `threads`: Gauge. Current number of OS threads.
- `cpus`: Gauge. Number of logical CPUs.

#### Storage
- `storage_total_bytes{path, device, fstype}`: Gauge. Total disk space in bytes.
- `storage_free_bytes{path, device, fstype}`: Gauge. Free disk space in bytes.
- `storage_used_bytes{path, device, fstype}`: Gauge. Used disk space in bytes.
- `storage_used_percent{path, device, fstype}`: Gauge. Disk usage percentage (0-100).
- `storage_inodes_total{path, device, fstype}`: Gauge. Total number of inodes.
- `storage_inodes_free{path, device, fstype}`: Gauge. Free number of inodes.
- `storage_inodes_used{path, device, fstype}`: Gauge. Used number of inodes.
- `storage_inodes_used_percent{path, device, fstype}`: Gauge. Inode usage percentage (0-100).
- `storage_read_bytes_total{device}`: Counter. Total bytes read from disk.
- `storage_write_bytes_total{device}`: Counter. Total bytes written to disk.
- `storage_read_time_seconds{device}`: Counter. Total time spent reading from disk in seconds.
- `storage_write_time_seconds{device}`: Counter. Total time spent writing to disk in seconds.
- `storage_reads_total{device}`: Counter. Total number of read operations.
- `storage_writes_total{device}`: Counter. Total number of write operations.
- `storage_io_time_seconds{device}`: Counter. Total time spent doing I/O in seconds.
- `storage_weighted_io_time_seconds{device}`: Counter. Weighted I/O time in seconds.

## Metrics Gap Analysis

After comprehensive review of the repository, the following areas have been identified as needing metrics implementation or enhancement:

### High Priority Metrics (Security & Core Functionality)

#### 1. Authentication & Authorization Metrics (`pkg/auth/`)
**Status:** ❌ Missing
**Impact:** Security critical - no visibility into authentication patterns, security threats, or user behavior.

**Recommended Metrics:**
```go
// Auth metrics
AuthLoginAttemptsTotal.WithLabelValues("success|failed", "method")
AuthRegistrationAttemptsTotal.WithLabelValues("success|failed")
AuthPasswordResetRequestsTotal.WithLabelValues("success|failed")
AuthJWTValidationsTotal.WithLabelValues("valid|invalid|expired")
AuthRateLimitHitsTotal.WithLabelValues("endpoint")
AuthAccountLockoutsTotal.WithLabelValues("reason")
AuthOAuthFlowsTotal.WithLabelValues("provider", "success|failed")
AuthSessionsTotal.WithLabelValues("active|expired")
```

#### 2. TrazrExtension API Metrics (`trazrextension/`)
**Status:** ❌ Missing
**Impact:** Core functionality - no visibility into API usage, performance, or errors.

**Recommended Metrics:**
```go
// Extension API metrics
ExtensionAPICallsTotal.WithLabelValues("endpoint", "method", "status")
ExtensionWebSocketConnections.WithLabelValues("active|total")
ExtensionDataInsertionsTotal.WithLabelValues("signal_type", "status")
ExtensionQueriesTotal.WithLabelValues("query_type", "status")
ExtensionAuthFailuresTotal.WithLabelValues("reason")
```

#### 3. TrazrExporter Metrics (`trazrexporter/`)
**Status:** ❌ Missing
**Impact:** Data pipeline - no visibility into export performance, failures, or data transformation.

**Recommended Metrics:**
```go
// Exporter metrics
ExporterDataExportedTotal.WithLabelValues("signal_type", "schema", "status")
ExporterTransformationDuration.WithLabelValues("signal_type")
ExporterSchemaRoutingDecisions.WithLabelValues("schema", "reason")
ExporterFailuresTotal.WithLabelValues("signal_type", "error_type")
```

#### 4. Frontend API Metrics (`frontend/src/app/api/`)
**Status:** ❌ Missing
**Impact:** User experience - no visibility into frontend-backend communication or user-facing errors.

**Recommended Metrics:**
```go
// Frontend API metrics
FrontendAPICallsTotal.WithLabelValues("endpoint", "status")
FrontendBackendFailuresTotal.WithLabelValues("endpoint", "error_type")
FrontendResponseTime.WithLabelValues("endpoint")
```

### Medium Priority Metrics (Performance & Quality)

#### 5. TrazrProcessor Metrics (`trazrprocessor/`)
**Status:** ⚠️ Basic OpenTelemetry metrics only
**Impact:** Data quality - limited visibility into processing pipeline performance.

**Recommended Metrics:**
```go
// Processor metrics
ProcessorSpansProcessedTotal.WithLabelValues("status")
ProcessorLogsProcessedTotal.WithLabelValues("status")
ProcessorMetricsProcessedTotal.WithLabelValues("status")
ProcessorValidationFailuresTotal.WithLabelValues("validation_type")
ProcessorFilteringDecisions.WithLabelValues("filter_type", "action")
```

#### 6. Enhanced Database Metrics (`pkg/repository/`)
**Status:** ⚠️ Basic query metrics only
**Impact:** Performance - limited visibility into query complexity, partition access, and connection pool usage.

**Recommended Metrics:**
```go
// Enhanced DB metrics
DBQueryComplexity.WithLabelValues("operation", "complexity_level")
DBPartitionAccess.WithLabelValues("table", "partition")
DBConnectionPool.WithLabelValues("status")
DBQueryPlanPerformance.WithLabelValues("operation", "plan_type")
```

#### 7. Vector Store Operations Metrics (`pkg/repository/`)
**Status:** ⚠️ Basic embedding vector store metrics only
**Impact:** Search performance - limited visibility into vector similarity search and index operations.

**Recommended Metrics:**
```go
// Vector store metrics
VectorSearchPerformance.WithLabelValues("provider", "query_type")
VectorIndexOperations.WithLabelValues("operation", "status")
VectorCollectionOperations.WithLabelValues("operation", "status")
VectorQueryComplexity.WithLabelValues("provider", "complexity_level")
```

### Low Priority Metrics (Operational & Advanced Features)

#### 8. Python Services Metrics (`services/`)
**Status:** ❌ Missing
**Impact:** External dependencies - no visibility into ChromaDB operations or sentence embedding performance.

**Recommended Metrics:**
```go
// Python services metrics
ChromaOperationsTotal.WithLabelValues("operation", "status")
EmbeddingOperationsTotal.WithLabelValues("model", "status")
PythonAPICallsTotal.WithLabelValues("endpoint", "status")
```

#### 9. Signal Processing Metrics (`pkg/repository/`)
**Status:** ❌ Missing
**Impact:** Advanced features - no visibility into signal filtering, template rendering, or content processing.

**Recommended Metrics:**
```go
// Signal processing metrics
SignalFilteringPerformance.WithLabelValues("filter_type", "signal_type")
TemplateRenderingPerformance.WithLabelValues("template_type", "signal_type")
ContentProcessingPipeline.WithLabelValues("stage", "signal_type")
SignalRoutingDecisions.WithLabelValues("signal_type", "destination")
```

#### 10. System Health Metrics
**Status:** ❌ Missing
**Impact:** Operational - no visibility into service discovery, configuration loading, or dependency health.

**Recommended Metrics:**
```go
// System health metrics
ServiceDiscovery.WithLabelValues("service", "status")
ConfigurationLoading.WithLabelValues("config_type", "status")
StartupPerformance.WithLabelValues("component")
DependencyHealth.WithLabelValues("dependency", "status")
```

## Implementation Roadmap

### Phase 1: High Priority (Security & Core)
1. **Authentication Metrics** - Implement in `pkg/auth/`
2. **Extension API Metrics** - Implement in `trazrextension/`
3. **Exporter Metrics** - Implement in `trazrexporter/`
4. **Frontend API Metrics** - Implement in `frontend/src/app/api/`

### Phase 2: Medium Priority (Performance)
5. **Processor Metrics** - Enhance in `trazrprocessor/`
6. **Enhanced Database Metrics** - Enhance in `pkg/repository/`
7. **Vector Store Metrics** - Enhance in `pkg/repository/`

### Phase 3: Low Priority (Operational)
8. **Python Services Metrics** - Implement in `services/`
9. **Signal Processing Metrics** - Implement in `pkg/repository/`
10. **System Health Metrics** - Implement across components

## Implementation Guidelines

### For Each New Metric Category:
1. **Create dedicated metrics file** (e.g., `auth.go`, `extension.go`)
2. **Define metrics with appropriate labels** and help text
3. **Register metrics** in the main `Register()` function
4. **Implement middleware** for automatic collection where appropriate
5. **Add metric collection** to business logic
6. **Create dashboards** for visualization
7. **Set up alerts** for critical thresholds
8. **Document metric meanings** and expected ranges

### Best Practices:
- Use consistent naming conventions (`component_metric_name_total`)
- Include appropriate labels for filtering and grouping
- Provide clear help text for each metric
- Use appropriate metric types (Counter, Gauge, Histogram)
- Implement automatic collection where possible
- Set up proper alerting thresholds
- Document metric relationships and dependencies

## Adding New Metrics
- Add new metrics to the appropriate file (notifier.go, db.go, http.go, etc.).
- Register them in the corresponding `register*Metrics()` function.
- Update this README with a description of the new metric and its labels.
- Follow the implementation roadmap for systematic coverage. 