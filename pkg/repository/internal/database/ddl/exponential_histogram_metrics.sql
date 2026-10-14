-- =====================================================
-- EXPONENTIAL HISTOGRAM METRICS TABLE (TimescaleDB Hypertable with Columnstore)
-- =====================================================
-- Optimized for:
--   - PostgreSQL 18+ (async I/O, skip scan, parallel GIN)
--   - TimescaleDB 2.23+ (columnstore with automatic bloom filters)
--   - OpenTelemetry ExponentialHistogram metric type (native histograms)
-- =====================================================

CREATE TABLE IF NOT EXISTS {{.Schema}}.exponential_histogram_metrics (
    id BIGSERIAL NOT NULL,
    timestamp_ns BIGINT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE GENERATED ALWAYS AS (to_timestamp(timestamp_ns / 1000000000.0)) STORED,
    -- Service entity fields (OTel v1.39.0 splits service into namespace/service/instance)
    service_name TEXT,
    service_namespace TEXT,              -- OTel v1.39.0: service.namespace
    service_instance_id TEXT,            -- OTel v1.39.0: service.instance.id
    resource_attributes JSONB DEFAULT '{}'::jsonb,
    scope_name TEXT,
    scope_version TEXT,
    scope_attributes JSONB DEFAULT '{}'::jsonb,
    scope_dropped_attr_count INTEGER DEFAULT 0,
    name TEXT NOT NULL,
    description TEXT,
    unit TEXT,
    attributes JSONB DEFAULT '{}'::jsonb,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    count BIGINT,
    sum DOUBLE PRECISION,
    scale INTEGER,
    zero_count BIGINT,
    zero_threshold DOUBLE PRECISION,      -- OTel exponential histogram zero bucket boundary
    positive_offset INTEGER,
    positive_bucket_counts JSONB DEFAULT '[]'::jsonb,
    negative_offset INTEGER,
    negative_bucket_counts JSONB DEFAULT '[]'::jsonb,
    exemplars JSONB DEFAULT '[]'::jsonb,
    flags INTEGER DEFAULT 0,
    min DOUBLE PRECISION,
    max DOUBLE PRECISION,
    aggregation_temporality INTEGER,
    metadata JSONB DEFAULT '{}'::jsonb,
    resource_schema_url TEXT,
    scope_schema_url TEXT,
    	PRIMARY KEY (id, timestamp_ns)
);

-- Convert to TimescaleDB hypertable using BIGINT timestamp_ns (nanoseconds)
SELECT create_hypertable('{{.Schema}}.exponential_histogram_metrics', 'timestamp_ns', 
    chunk_time_interval => 86400000000000, -- 1 day in nanoseconds
    if_not_exists => TRUE
);

-- Set integer_now_func with error handling
-- Note: Using get_unix_ns_now (nanoseconds) to match timestamp_ns column
DO $$
BEGIN
    PERFORM set_integer_now_func('{{.Schema}}.exponential_histogram_metrics', 'public.get_unix_ns_now');
EXCEPTION
    WHEN OTHERS THEN
        -- Function already set, ignore the error
        NULL;
END $$;

-- =====================================================
-- COLUMNSTORE CONFIGURATION (TimescaleDB 2.23+)
-- =====================================================
ALTER TABLE {{.Schema}}.exponential_histogram_metrics SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'service_name',
    timescaledb.orderby = 'timestamp_ns DESC'
);

-- Schedule columnstore conversion for chunks older than 7 days
DO $$
BEGIN
    CALL add_columnstore_policy('{{.Schema}}.exponential_histogram_metrics', after => 604800000000000::bigint, if_not_exists => TRUE);
EXCEPTION
    WHEN undefined_function THEN
        PERFORM add_compression_policy('{{.Schema}}.exponential_histogram_metrics', 604800000000000, if_not_exists => TRUE);
END $$;

-- Retention policy: Drop chunks older than 30 days
SELECT add_retention_policy('{{.Schema}}.exponential_histogram_metrics', 2592000000000000, if_not_exists => TRUE);


-- Simple, proven indexes (PostgreSQL best practices)
-- 1. Time-based queries (most common for observability)  
CREATE INDEX IF NOT EXISTS idx_metrics_exponential_histogram_timestamp ON {{.Schema}}.exponential_histogram_metrics (timestamp);
CREATE INDEX IF NOT EXISTS idx_metrics_exponential_histogram_service_time ON {{.Schema}}.exponential_histogram_metrics (service_name, timestamp);

-- 2. Metric identification
CREATE INDEX IF NOT EXISTS idx_metrics_exponential_histogram_service_metric ON {{.Schema}}.exponential_histogram_metrics (service_name, name);

-- 3. Attribute searching (simple GIN, not complex akeys/avals)
CREATE INDEX IF NOT EXISTS idx_metrics_exp_histogram_attributes ON {{.Schema}}.exponential_histogram_metrics USING GIN (attributes);
CREATE INDEX IF NOT EXISTS idx_metrics_exp_histogram_resource_attrs ON {{.Schema}}.exponential_histogram_metrics USING GIN (resource_attributes);

-- 4. Exemplar-specific index for span_id correlation queries
-- GIN index for JSONB containment operations (@> operator) - most efficient for our query pattern
-- This is the MOST CRITICAL index for exemplar performance - DO NOT REMOVE
CREATE INDEX IF NOT EXISTS idx_metrics_exp_histogram_exemplars 
ON {{.Schema}}.exponential_histogram_metrics USING GIN (exemplars) 
WHERE exemplars IS NOT NULL AND exemplars != '[]'::jsonb;

-- 5. Metadata JSONB index (PostgreSQL 18 parallel GIN build support)
CREATE INDEX IF NOT EXISTS idx_exponential_histogram_metrics_metadata ON {{.Schema}}.exponential_histogram_metrics USING GIN (metadata) WHERE metadata IS NOT NULL;

-- 6. Metric name + time for PromQL-style queries
CREATE INDEX IF NOT EXISTS idx_exp_histogram_metrics_name_time ON {{.Schema}}.exponential_histogram_metrics (name, timestamp_ns);

