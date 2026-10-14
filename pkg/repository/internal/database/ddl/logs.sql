-- =====================================================
-- LOGS TABLE (TimescaleDB Hypertable with Columnstore)
-- =====================================================
-- Optimized for:
--   - PostgreSQL 18+ (async I/O, skip scan, parallel GIN)
--   - TimescaleDB 2.23+ (columnstore with automatic bloom filters)
--   - OpenTelemetry log data with trace correlation
--
-- Key Features:
--   - Nanosecond precision timestamps
--   - Trace/span correlation indexes
--   - Severity-based partial indexes for alerts
--   - Full-text search on log bodies
-- =====================================================

CREATE TABLE IF NOT EXISTS {{.Schema}}.logs (
    id BIGSERIAL NOT NULL,
    timestamp_ns BIGINT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE GENERATED ALWAYS AS (to_timestamp(timestamp_ns / 1000000000.0)) STORED,
    observed_timestamp TIMESTAMPTZ,
    trace_id TEXT,
    span_id TEXT,
    flags INTEGER DEFAULT 0,
    severity_text TEXT,
    severity_number INTEGER,
    -- Service entity fields (OTel v1.39.0 splits service into namespace/service/instance)
    service_name TEXT NOT NULL,
    service_namespace TEXT,              -- OTel v1.39.0: service.namespace
    service_instance_id TEXT,            -- OTel v1.39.0: service.instance.id
    -- Event name for OTel v1.39.0 otel.event.name support
    event_name TEXT,                     -- OTel v1.39.0: otel.event.name
    resource_attributes JSONB DEFAULT '{}'::jsonb,
    scope_name TEXT,
    scope_version TEXT,
    scope_attributes JSONB DEFAULT '{}'::jsonb,
    scope_dropped_attr_count INTEGER DEFAULT 0,
    attributes JSONB DEFAULT '{}'::jsonb,
    dropped_attr_count INTEGER DEFAULT 0,
    body JSONB DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    resource_schema_url TEXT,
    scope_schema_url TEXT,
    PRIMARY KEY (id, timestamp_ns)
);

-- Convert to TimescaleDB hypertable with integer_now_func for nanoseconds
SELECT create_hypertable('{{.Schema}}.logs', 'timestamp_ns', chunk_time_interval => 86400000000000, if_not_exists => TRUE);

-- Set integer_now_func with error handling
-- Note: Using get_unix_ns_now (nanoseconds) to match timestamp_ns column
DO $$
BEGIN
    PERFORM set_integer_now_func('{{.Schema}}.logs', 'public.get_unix_ns_now');
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
-- =====================================================

-- Enable columnstore with optimized settings for log data
ALTER TABLE {{.Schema}}.logs SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'service_name',
    timescaledb.orderby = 'timestamp_ns DESC'
);

-- Schedule columnstore conversion for chunks older than 7 days
DO $$
BEGIN
    -- Try new columnstore API first (TimescaleDB 2.20+)
    CALL add_columnstore_policy('{{.Schema}}.logs', after => 604800000000000::bigint, if_not_exists => TRUE);
EXCEPTION
    WHEN undefined_function THEN
        -- Fallback to legacy compression API (TimescaleDB < 2.20)
        PERFORM add_compression_policy('{{.Schema}}.logs', 604800000000000, if_not_exists => TRUE);
END $$;

-- Retention policy: Drop chunks older than 30 days
SELECT add_retention_policy('{{.Schema}}.logs', 2592000000000000, if_not_exists => TRUE);

-- Simple, proven indexes (PostgreSQL best practices)
-- 1. Time-based queries (most common for observability)
CREATE INDEX IF NOT EXISTS idx_logs_timestamp ON {{.Schema}}.logs (timestamp);
CREATE INDEX IF NOT EXISTS idx_logs_service_time ON {{.Schema}}.logs (service_name, timestamp);

-- 2. ID lookups (use standard B-tree, not hash)
CREATE INDEX IF NOT EXISTS idx_logs_trace_id ON {{.Schema}}.logs (trace_id) WHERE trace_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_logs_span_id ON {{.Schema}}.logs (span_id) WHERE span_id IS NOT NULL;

-- 3. Common filtering patterns
CREATE INDEX IF NOT EXISTS idx_logs_service_name ON {{.Schema}}.logs (service_name);
CREATE INDEX IF NOT EXISTS idx_logs_severity ON {{.Schema}}.logs (severity_number, severity_text);

-- 4. Log severity filtering (optimized for severity-based dashboards and alerts)
CREATE INDEX IF NOT EXISTS idx_logs_service_severity_time ON {{.Schema}}.logs (service_name, severity_number, timestamp_ns);

-- 5. Error log tracking (partial index for error/critical logs only)
CREATE INDEX IF NOT EXISTS idx_logs_errors ON {{.Schema}}.logs (service_name, timestamp_ns)
WHERE severity_number >= 17;  -- ERROR (17) and above (FATAL=21, etc.)

-- 6. Attribute searching (simple GIN, not complex akeys/avals)
CREATE INDEX IF NOT EXISTS idx_logs_resource_attrs ON {{.Schema}}.logs USING GIN (resource_attributes);
CREATE INDEX IF NOT EXISTS idx_logs_log_attrs ON {{.Schema}}.logs USING GIN (attributes);

-- 7. Full-text search (standard approach)
CREATE INDEX IF NOT EXISTS idx_logs_body ON {{.Schema}}.logs USING GIN (to_tsvector('english', body::text)) WHERE body IS NOT NULL AND body != '{}'::jsonb;

-- 8. Metadata JSONB index (PostgreSQL 18 parallel GIN build support)
CREATE INDEX IF NOT EXISTS idx_logs_metadata ON {{.Schema}}.logs USING GIN (metadata) WHERE metadata IS NOT NULL;

-- =====================================================
-- OPENTELEMETRY SEMANTIC CONVENTION INDEXES
-- =====================================================

-- 9. Cross-signal correlation: Find logs by trace_id with time bounds
-- Optimized for: Log-to-trace correlation, distributed debugging
CREATE INDEX IF NOT EXISTS idx_logs_trace_time ON {{.Schema}}.logs (trace_id, timestamp_ns)
WHERE trace_id IS NOT NULL;

-- 10. Span-correlated logs: Find logs within a specific span
CREATE INDEX IF NOT EXISTS idx_logs_span_time ON {{.Schema}}.logs (span_id, timestamp_ns)
WHERE span_id IS NOT NULL;

-- 11. Exception tracking (OTel exception.* semantic attributes)
CREATE INDEX IF NOT EXISTS idx_logs_exceptions ON {{.Schema}}.logs USING GIN ((attributes -> 'exception.type'), (attributes -> 'exception.message'))
WHERE attributes ? 'exception.type';

-- =====================================================
-- OTEL v1.39.0 SERVICE ENTITY INDEXES
-- =====================================================

-- 12. Service namespace composite index
CREATE INDEX IF NOT EXISTS idx_logs_service_namespace ON {{.Schema}}.logs (service_namespace, service_name, timestamp_ns)
WHERE service_namespace IS NOT NULL;

-- 13. Service instance tracking
CREATE INDEX IF NOT EXISTS idx_logs_service_instance ON {{.Schema}}.logs (service_name, service_instance_id, timestamp_ns)
WHERE service_instance_id IS NOT NULL;

-- 14. Event name index (OTel v1.39.0 otel.event.name)
CREATE INDEX IF NOT EXISTS idx_logs_event_name ON {{.Schema}}.logs (event_name, timestamp_ns)
WHERE event_name IS NOT NULL;

-- =====================================================
-- GENAI SEMANTIC CONVENTION INDEXES (OTel v1.37+)
-- =====================================================

-- 15. GenAI provider tracking in logs
CREATE INDEX IF NOT EXISTS idx_logs_genai_provider ON {{.Schema}}.logs USING GIN ((attributes -> 'gen_ai.provider.name'))
WHERE attributes ? 'gen_ai.provider.name';

-- 16. GenAI conversation/session tracking
CREATE INDEX IF NOT EXISTS idx_logs_genai_conversation ON {{.Schema}}.logs ((attributes ->> 'gen_ai.conversation.id'), timestamp_ns)
WHERE attributes ? 'gen_ai.conversation.id';

-- Note: Retention policies work with TimescaleDB Community/Enterprise
-- For Apache license, use: SELECT drop_chunks('{{.Schema}}.logs', older_than => INTERVAL '30 days');
