# Batch Embedding Pipeline Design Document

---

## Table of Contents
1. Problem Statement
2. Goals
3. High-Level Architecture
4. Signal Table Access and Filtering
5. Embedding Filter Configuration (`config_embedding_provider.filters`)
6. Marking Embedded Signal Data
7. Linking Vectors to Source Signal Data
8. Worker Assignment and Lifecycle
9. Data Chunking Strategy
10. Worker Notification for New Data
11. Job Control, Scheduling, and Lifecycle
12. Filtering Configuration and Standards
13. Attribute-to-Metadata Mapping for VectorStore
14. Performance Considerations and Optimizations
15. Testing and Validation
16. Metrics, Observability, and Audit
17. OpenTelemetry and VectorStore Observability Best Practices
18. Schema Evolution and Adaptation
19. Resource Limits and Configurability
20. Versioning and Audit
21. Summary

---

## 1. Problem Statement
Efficient semantic search and retrieval over OpenTelemetry signals (logs, traces, metrics) requires embedding these signals into a VectorStore. Real-time embedding is not feasible due to performance, cost, and operational complexity. A batch-oriented approach is needed to periodically process and embed signals based on configurable filters and criteria.

## 2. Goals
- Enable scalable, efficient batch embedding of OpenTelemetry signals into a VectorStore.
- Support flexible, per-signal-type filtering and chunking of signals for embedding.
- Decouple embedding from real-time ingestion/exporter logic.
- Ensure traceability, idempotency, and observability throughout the pipeline.

## 3. High-Level Architecture
- Batch worker(s) query signal tables (logs, traces, metrics) using filters from config tables.
- Embedding is performed in the background, and results are stored in the VectorStore with metadata linking back to the source.
- Each embedding configuration (row) in `config_embedding_provider` can have a dedicated worker/job.

## 4. Signal Table Access and Filtering
- Signal tables are defined by SQL DDLs (see `internal/database/ddl/`).
- Filtering must be translatable to SQL WHERE clauses for efficiency.
- The system must be extensible for new tables and filter types.

## 5. Embedding Filter Configuration (`config_embedding_provider.filters`)
- The `filters` field is a structured JSON object, grouping filter criteria by signal type.
- Example:
  ```json
  {
    "logs": "*", // embed all logs
    "traces": {
      "service_name": ["auth-service"],
      "span_name": ["Login", "Register"]
    },
    "metrics_gauge": {}
  }
  ```
- **Filter Semantics:**
  - If the value is `"*"`: Embed all data of that type.
  - If the value is an empty array (`[]`) or object (`{}`): Do not embed any data of that type.
  - If the key is not present: Do not embed any data of that type.
  - If the value is an object with filters: Embed only data matching the filters.

## 6. Marking Embedded Signal Data
- Use the `metadata` JSONB field in each signal table to record embedding status and details.
- **Multiple Embedding Receipts:**
  - The metadata field should support an array of embedding receipts, each with the embedding config ID, timestamp, and optionally embedding version/hash.
  - Example:
    ```json
    {
      "embeddings": [
        {"config_id": 1, "embedded_at": "2024-07-01T12:34:56Z", "embedding_version": "v1"},
        {"config_id": 2, "embedded_at": "2024-07-01T12:35:10Z", "embedding_version": "v2"}
      ]
    }
    ```
- This allows a signal to be embedded by multiple configs and provides full traceability.

## 7. Linking Vectors to Source Signal Data
- Store relevant metadata in the VectorStore (source table, row ID, key attributes) to enable reverse lookup and traceability.
- Attribute-to-metadata mapping is configurable (see section 13).

## 8. Worker Assignment and Lifecycle (Updated)

- **Resource-Managed Worker Pattern (2024-07):**
  - The ServiceResourceManager now manages the lifecycle of `EmbeddingWorker` instances, attaching a single worker to each `VectorStores` resource.
  - Each worker is constructed with all dependencies injected by the resource manager:
    - Database client (for fetching and marking signals)
    - Logger (using a sugared zap.Logger for compatibility)
    - Embedding provider (adapted if needed)
    - Vector store client (adapted if needed for AddVectors)
  - Adapters are used only where strictly necessary to match interfaces, keeping the code minimal and idiomatic.
  - Workers are started automatically when the resource manager loads resources, and stopped on shutdown/cleanup.
  - All configuration and resource management is centralized, making the system modular, testable, and ready for future extension (e.g., dynamic config reload, job control API, UI integration).
  - The worker is now a first-class managed resource, fully observable and integrated with OpenTelemetry best practices.

- **Worker Model:**
  - By default, each active embedding configuration in `config_embedding_provider` has a dedicated worker/job.
  - **Scalability:** For large numbers of configs, support a worker pool model: a fixed pool of workers pulls jobs/configs from the table and processes them in a round-robin or priority order.
  - **Configurable:** The worker assignment model (one-per-config vs. pool) should be configurable based on deployment size and resource constraints.
- Advantages: isolation, parallelism, flexible scheduling.
- Workers can be started, paused, resumed, and stopped via API/UI.
- Graceful shutdown is supported via a `Close()` method.

> **Note:** The current implementation is now fully aligned with this design and with Go microservices best practices for modularity, testability, and observability. All resource and worker lifecycle management is handled in a single, idiomatic pattern.

## 9. Data Chunking Strategy
- **Chunking is important** for large signals (e.g., logs with long messages, traces with many spans).
- **Configurable Chunking:**
  - Define chunking logic per signal type (e.g., max message length, max spans per chunk).
  - Chunking parameters should be configurable in the `config_embedding_provider` table (e.g., `max_chunk_size`, `split_on_newline`).
- **Examples:**
  - Logs: split long messages into chunks of N characters or on newlines.
  - Traces: split traces with many spans into smaller sub-traces.
  - Metrics: batch metrics by time window or count.

## 10. Worker Notification for New Data
- Support for notifying workers when new data is ingested:
  - PostgreSQL NOTIFY/LISTEN
  - Message queue/event bus
  - Application-level callbacks
  - Hybrid polling (with timestamp sampler)
- Batching and rate-limiting are recommended for high-ingest environments.

## 11. Job Control, Scheduling, and Lifecycle
- Configurable interval (duration or cron-like) for each job.
- Pause/resume methods and endpoints (callable from UI).
- Max batch size and timeouts are configurable.
- **Batch Scoping:**
  - By default, workers query for all unmarked data (`metadata.embedded_at IS NULL`).
  - Optionally, support time-windowed queries (e.g., `timestamp BETWEEN X AND Y`) for more predictable batch sizes and easier recovery after downtime.
  - Batch scoping strategy should be configurable.
- No persistent job state; jobs re-query on restart.

## 12. Filtering Configuration and Standards
- Use declarative, YAML/JSON-based filter configuration.
- Only filters that can be directly translated to SQL WHERE clauses are supported.
- **Supported SQL Operators:**
  - `=` (equals)
  - `IN`, `NOT IN`
  - `LIKE` (wildcard/pattern matching)
  - `>`, `<`, `>=`, `<=` (range)
  - `BETWEEN`
  - Logical `AND` (multiple conditions)
- **Examples:**
  - `service_name = 'auth-service'`
  - `service_name LIKE 'auth-%'`
  - `severity_number IN (30, 40, 50)`
  - `timestamp BETWEEN '2024-07-01' AND '2024-07-02'`
- Provide an API endpoint to test filter expressions against sample data for validation.

## 13. Attribute-to-Metadata Mapping for VectorStore
- Mapping from signal attributes to VectorStore metadata is configurable (YAML/JSON).
- Sensible defaults for each signal type (e.g., `service_name`, `severity_number` for logs).
- Users can extend or override mappings as needed.

## 14. Performance Considerations and Optimizations
- Query optimization: use indexed columns, partitioning, and batch by time window.
- Parallelism: process different tables/shards in parallel.
- Bulk operations for marking embedded rows and writing to VectorStore.
- Resource management: stream large result sets, tune connection pools.
- Observability: track batch duration, throughput, error rate.
- **Backpressure and Throttling:**
  - Limit SELECT query size (batch size).
  - Worker pauses or reduces concurrency if VectorStore or DB is slow.
  - Use circuit breakers or rate limiters on external calls (embedding model, VectorStore).
  - All backpressure parameters should be configurable.

## 15. Testing and Validation
- Comprehensive unit and integration tests for:
  - Filter parsing and SQL translation
  - Attribute-to-metadata mapping
  - Worker lifecycle and job control
  - End-to-end embedding pipeline
- API endpoint for filter testing (dry-run) for UI validation.

## 16. Metrics, Observability, and Audit
- Use OpenTelemetry best practices for tracing, metrics, and logging.
- **Key Metrics to Track:**
  - `embedding_jobs.run.duration` (labels: config ID, status)
  - `signals.embedded.count` (labels: signal type, config ID)
  - `vectorstore.write.latency`
  - `db.query.error.count`
  - `embedding_jobs.run.errors`
  - `embedding_jobs.run.success`
- Record job status, errors, and statistics.
- Versioning and audit of config changes.

## 17. OpenTelemetry and VectorStore Observability Best Practices

### Comprehensive Instrumentation
- Use both auto-instrumentation (OpenTelemetry SDKs for HTTP, DB, etc.) and manual instrumentation for domain-specific operations.
- Manually create spans for:
  - Batch job lifecycle (start, end, error)
  - Embedding model calls
  - VectorStore operations (add/search/delete)
  - LLM or downstream AI calls (if applicable)

### OpenTelemetry Semantic Conventions
- Adhere to OTel semantic conventions for span and attribute naming:
  - `db.system`: VectorStore type (e.g., "qdrant", "milvus")
  - `db.operation`: Action (e.g., "search", "add", "delete")
  - `db.vector.query.top_k`: Number of nearest neighbors requested
  - `db.vector.search.metric`: Distance metric (e.g., "euclidean", "cosine")

### End-to-End Tracing
- Propagate context across all service boundaries (HTTP, gRPC, DB, embedding, vector store).
- Enrich spans with metadata:
  - Number of vectors returned
  - IDs of returned documents
  - Query text (with privacy in mind)
- Visualize traces to identify bottlenecks in the full embedding/search pipeline.

### Key Metrics for VectorStore Monitoring
- Track and export:
  - Query/search latency
  - Indexing (add) latency
  - Error rates (per operation)
  - Resource utilization (CPU, memory, disk) for VectorStore nodes
- Scrape VectorStore's Prometheus/OpenMetrics endpoints if available.

### Effective Logging
- Log all key events (collection creation/deletion, errors, batch job status).
- Always include `trace_id` and `span_id` in logs for correlation.
- Use structured, JSON-formatted logs.

### OpenTelemetry Collector
- Route all telemetry (traces, metrics, logs) through the OTel Collector.
- Use the Collector for enrichment, redaction, and multi-backend export.
- Centralizes telemetry management and enables vendor-agnostic observability.

### Practical Example: Tracing a Batch Embedding Job
- A typical trace for a batch embedding job might include:
  - **Root span:** Batch job execution
    - **Child span:** DB query for unembedded signals
    - **Child span:** Embedding model call
    - **Child span:** VectorStore write (add/search)
    - **Child span:** (Optional) LLM call with retrieved context
- Each span is enriched with relevant attributes (e.g., batch size, config ID, operation type, latency, error status).
- Example (pseudocode):
  ```
  [BatchJob] (trace_id=abc123)
    ├─ [DBQuery] (span_id=def456, db.operation="select", batch_size=100)
    ├─ [EmbeddingModel] (span_id=ghi789, model="openai-ada", input_count=100)
    ├─ [VectorStoreWrite] (span_id=jkl012, db.system="qdrant", db.operation="add", vector_count=100)
    └─ [LLMCall] (span_id=mno345, model="gpt-4", context_size=10KB)
  ```
- This enables full visibility into the performance and behavior of the embedding pipeline.

## 18. Schema Evolution and Adaptation
- Detect new columns/tables and log/notify for manual review.
- Design for extensibility as OpenTelemetry evolves.

## 19. Resource Limits and Configurability
- Configurable max DB connections, concurrent jobs, batch size, and timeouts.
- All resource limits should be easily adjustable.

## 20. Versioning and Audit
- Track config changes and job versions for auditability.
- Provide endpoints or logs for version history.

## 21. Summary
This design enables robust, scalable, and maintainable batch embedding of OpenTelemetry signals into a VectorStore, with strong support for filtering, traceability, observability, and operational control. The architecture is extensible for future UI, schema evolution, and advanced operational needs. 