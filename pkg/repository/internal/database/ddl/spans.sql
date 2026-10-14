-- =====================================================
-- SPANS TABLE (TimescaleDB Hypertable with Columnstore)
-- =====================================================
-- Optimized for:
--   - PostgreSQL 18+ (async I/O, skip scan, parallel GIN)
--   - TimescaleDB 2.23+ (columnstore with automatic bloom filters)
--   - OpenTelemetry trace data with high-cardinality attributes
--
-- Key Features:
--   - Nanosecond precision timestamps
--   - Deduplication constraint for retry-prone exporters
--   - Composite indexes for trace tree reconstruction
--   - Partial indexes for error tracking
-- =====================================================

CREATE TABLE IF NOT EXISTS {{.Schema}}.spans (
    id BIGSERIAL NOT NULL,
    timestamp_ns BIGINT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE GENERATED ALWAYS AS (to_timestamp(timestamp_ns / 1000000000.0)) STORED,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    duration BIGINT GENERATED ALWAYS AS (EXTRACT(EPOCH FROM (end_time - to_timestamp(timestamp_ns / 1000000000.0))) * 1e9) STORED,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT,
    trace_state TEXT,
    flags INTEGER DEFAULT 0,              -- OTel span-level flags (fixed32)
    name TEXT NOT NULL,
    span_kind TEXT,
    -- Service entity fields (OTel v1.39.0 splits service into namespace/service/instance)
    service_name TEXT,
    service_namespace TEXT,              -- OTel v1.39.0: service.namespace
    service_instance_id TEXT,            -- OTel v1.39.0: service.instance.id
    resource_attributes JSONB DEFAULT '{}'::jsonb,
    scope_name TEXT,
    scope_version TEXT,
    scope_attributes JSONB DEFAULT '{}'::jsonb,
    scope_dropped_attr_count INTEGER DEFAULT 0,
    attributes JSONB DEFAULT '{}'::jsonb,
    dropped_attr_count INTEGER DEFAULT 0,
    events JSONB DEFAULT '[]'::jsonb,
    dropped_events_count INTEGER DEFAULT 0,
    links JSONB DEFAULT '[]'::jsonb,
    dropped_links_count INTEGER DEFAULT 0,
    status_code INTEGER DEFAULT 0,
    status_message TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,

    resource_schema_url TEXT,
    scope_schema_url TEXT,
    	PRIMARY KEY (id, timestamp_ns)
);

-- Convert to TimescaleDB hypertable using BIGINT timestamp_ns (nanoseconds)
SELECT create_hypertable('{{.Schema}}.spans', 'timestamp_ns', 
    chunk_time_interval => 86400000000000, -- 1 day in nanoseconds
    if_not_exists => TRUE
);

-- Set integer_now_func with error handling
-- Note: Using get_unix_ns_now (nanoseconds) to match timestamp_ns column
DO $$
BEGIN
    PERFORM set_integer_now_func('{{.Schema}}.spans', 'public.get_unix_ns_now');
EXCEPTION
    WHEN OTHERS THEN
        -- Function already set, ignore the error
        NULL;
END $$;

-- =====================================================
-- COLUMNSTORE CONFIGURATION (TimescaleDB 2.23+)
-- =====================================================
-- Using new columnstore API which provides:
--   - Automatic bloom filters for faster filtering
--   - Dynamic compression settings per-chunk
--   - Better storage efficiency with adaptive compression
--
-- Note: segmentby should NOT include 'id' as it creates unique segmentation
-- per row, effectively disabling compression benefits.
-- =====================================================

-- Enable columnstore with optimized settings for trace data
ALTER TABLE {{.Schema}}.spans SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'service_name',
    timescaledb.orderby = 'timestamp_ns DESC'
);

-- Schedule columnstore conversion for chunks older than 7 days
-- Using CALL for TimescaleDB 2.20+ add_columnstore_policy
DO $$
BEGIN
    -- Try new columnstore API first (TimescaleDB 2.20+)
    CALL add_columnstore_policy('{{.Schema}}.spans', after => 604800000000000::bigint, if_not_exists => TRUE);
EXCEPTION
    WHEN undefined_function THEN
        -- Fallback to legacy compression API (TimescaleDB < 2.20)
        PERFORM add_compression_policy('{{.Schema}}.spans', 604800000000000, if_not_exists => TRUE);
END $$;

-- Retention policy: Drop chunks older than 30 days
SELECT add_retention_policy('{{.Schema}}.spans', 2592000000000000, if_not_exists => TRUE);

-- Span deduplication constraint to prevent duplicate spans from retry-prone exporters
-- Note: Using unique index instead of constraint for better TimescaleDB compatibility
CREATE UNIQUE INDEX IF NOT EXISTS idx_spans_trace_span_unique
ON {{.Schema}}.spans (trace_id, span_id, timestamp_ns);

-- Simple, proven indexes (PostgreSQL best practices)
-- 1. Time-based queries (most common for observability)
CREATE INDEX IF NOT EXISTS idx_spans_timestamp ON {{.Schema}}.spans (timestamp);
CREATE INDEX IF NOT EXISTS idx_spans_service_time ON {{.Schema}}.spans (service_name, timestamp);

-- 2. ID lookups (use standard B-tree, not hash)
CREATE INDEX IF NOT EXISTS idx_spans_trace_id ON {{.Schema}}.spans (trace_id);
CREATE INDEX IF NOT EXISTS idx_spans_span_id ON {{.Schema}}.spans (span_id);
CREATE INDEX IF NOT EXISTS idx_spans_parent_span_id ON {{.Schema}}.spans (parent_span_id) WHERE parent_span_id IS NOT NULL;

-- 3. Trace tree reconstruction index for efficient trace visualization
CREATE INDEX IF NOT EXISTS idx_spans_trace_tree ON {{.Schema}}.spans (trace_id, parent_span_id, timestamp_ns);

-- 4. Common filtering patterns
CREATE INDEX IF NOT EXISTS idx_spans_service_span ON {{.Schema}}.spans (service_name, name);
CREATE INDEX IF NOT EXISTS idx_spans_status ON {{.Schema}}.spans (status_code) WHERE status_code > 0;

-- 5. Service latency analysis (for P50/P90/P99 calculations)
CREATE INDEX IF NOT EXISTS idx_spans_service_latency ON {{.Schema}}.spans (service_name, timestamp_ns, duration);

-- 6. Error tracking (optimized for error rate dashboards)
CREATE INDEX IF NOT EXISTS idx_spans_service_errors ON {{.Schema}}.spans (service_name, status_code, timestamp_ns)
WHERE status_code >= 2;

-- 7. Attribute searching (simple GIN, not complex akeys/avals)
CREATE INDEX IF NOT EXISTS idx_spans_resource_attrs ON {{.Schema}}.spans USING GIN (resource_attributes);
CREATE INDEX IF NOT EXISTS idx_spans_span_attrs ON {{.Schema}}.spans USING GIN (attributes);

-- 8. JSONB for events/links (only if actually queried)
CREATE INDEX IF NOT EXISTS idx_spans_events ON {{.Schema}}.spans USING GIN (events) WHERE jsonb_array_length(events) > 0;
CREATE INDEX IF NOT EXISTS idx_spans_links ON {{.Schema}}.spans USING GIN (links) WHERE jsonb_array_length(links) > 0;

-- 9. Metadata JSONB index (PostgreSQL 18 parallel GIN build support)
-- Enables efficient queries on any JSONB structure within metadata, including embeddings
CREATE INDEX IF NOT EXISTS idx_spans_metadata ON {{.Schema}}.spans USING GIN (metadata) WHERE metadata IS NOT NULL;

-- =====================================================
-- OPENTELEMETRY SEMANTIC CONVENTION INDEXES
-- =====================================================
-- These indexes optimize common OTel query patterns as defined in
-- the OpenTelemetry semantic conventions specification.

-- 10. Cross-signal correlation: Find spans by trace_id with time bounds
-- Optimized for: Trace timeline queries, span-to-log correlation
CREATE INDEX IF NOT EXISTS idx_spans_trace_time ON {{.Schema}}.spans (trace_id, timestamp_ns);

-- 11. Service dependency analysis: Find all spans for service pairs
-- Optimized for: Service map generation, dependency graphs
-- Uses PostgreSQL 18 skip scan for efficient prefix searches
CREATE INDEX IF NOT EXISTS idx_spans_service_kind ON {{.Schema}}.spans (service_name, span_kind, timestamp_ns);

-- 12. HTTP endpoint analysis (common OTel semantic attribute pattern)
-- Partial index for http.* attributes in the attributes JSONB
CREATE INDEX IF NOT EXISTS idx_spans_http_attrs ON {{.Schema}}.spans USING GIN ((attributes -> 'http.method'), (attributes -> 'http.status_code'))
WHERE attributes ? 'http.method';

-- 13. Database operation tracking (OTel db.* semantic attributes)
CREATE INDEX IF NOT EXISTS idx_spans_db_attrs ON {{.Schema}}.spans USING GIN ((attributes -> 'db.system'), (attributes -> 'db.operation'))
WHERE attributes ? 'db.system';

-- =====================================================
-- OTEL v1.39.0 SERVICE ENTITY INDEXES
-- =====================================================
-- Service entity is now split into namespace, service, and instance

-- 14. Service namespace composite index
CREATE INDEX IF NOT EXISTS idx_spans_service_namespace ON {{.Schema}}.spans (service_namespace, service_name, timestamp_ns)
WHERE service_namespace IS NOT NULL;

-- 15. Service instance tracking
CREATE INDEX IF NOT EXISTS idx_spans_service_instance ON {{.Schema}}.spans (service_name, service_instance_id, timestamp_ns)
WHERE service_instance_id IS NOT NULL;

-- =====================================================
-- GENAI SEMANTIC CONVENTION INDEXES (OTel v1.37+)
-- =====================================================
-- These indexes optimize GenAI/LLM observability queries

-- 16. GenAI provider and model tracking
CREATE INDEX IF NOT EXISTS idx_spans_genai_provider ON {{.Schema}}.spans USING GIN ((attributes -> 'gen_ai.provider.name'), (attributes -> 'gen_ai.request.model'))
WHERE attributes ? 'gen_ai.provider.name';

-- 17. GenAI agent tracking (for AI agent workflows)
CREATE INDEX IF NOT EXISTS idx_spans_genai_agent ON {{.Schema}}.spans USING GIN ((attributes -> 'gen_ai.agent.id'), (attributes -> 'gen_ai.agent.name'))
WHERE attributes ? 'gen_ai.agent.id';

-- 18. GenAI conversation/session tracking
CREATE INDEX IF NOT EXISTS idx_spans_genai_conversation ON {{.Schema}}.spans ((attributes ->> 'gen_ai.conversation.id'), timestamp_ns)
WHERE attributes ? 'gen_ai.conversation.id';

-- =====================================================
-- MESSAGING SEMANTIC CONVENTION INDEXES (Stabilized)
-- =====================================================

-- 19. Messaging system tracking (Kafka, RabbitMQ, etc.)
CREATE INDEX IF NOT EXISTS idx_spans_messaging ON {{.Schema}}.spans USING GIN ((attributes -> 'messaging.system'), (attributes -> 'messaging.operation.name'))
WHERE attributes ? 'messaging.system';

