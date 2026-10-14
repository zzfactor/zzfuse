-- =====================================================
-- VECTOR STORE TABLE (TimescaleDB Hypertable with Columnstore)
-- =====================================================
-- Optimized for:
--   - PostgreSQL 18+ (async I/O, skip scan, parallel GIN)
--   - TimescaleDB 2.23+ (columnstore with automatic bloom filters)
--   - pgvector/vectorscale HNSW index for semantic search
--   - LangChain Go integration
-- =====================================================

CREATE TABLE IF NOT EXISTS {{.Schema}}.{{.Table}} (
    id BIGSERIAL NOT NULL,
    timestamp_ns BIGINT NOT NULL,
    timestamp TIMESTAMPTZ GENERATED ALWAYS AS (to_timestamp(timestamp_ns / 1000000000.0)) STORED,
    service_name TEXT,
    content TEXT,                -- The raw text or document to be embedded
    content_hash TEXT,           -- SHA256 hash of content for baseline comparison
    embedding vector({{.Dimension}}) NOT NULL, -- Embedding dimension must be set at creation time
    metadata JSONB DEFAULT '{}'::jsonb,
    	PRIMARY KEY (id, timestamp_ns)
);

-- Indexes for performance (parent table, will be inherited by chunks)
CREATE INDEX IF NOT EXISTS idx_{{.Table}}_timestamp ON {{.Schema}}.{{.Table}} (timestamp);
CREATE INDEX IF NOT EXISTS idx_{{.Table}}_service_time ON {{.Schema}}.{{.Table}} (service_name, timestamp);
CREATE INDEX IF NOT EXISTS idx_{{.Table}}_content_hash ON {{.Schema}}.{{.Table}} (content_hash);

-- HNSW index for fast approximate nearest neighbor search on embedding
-- Tuned for large datasets: m=32 (connections), ef_construction=200 (build quality)
-- Set ef_search at query time for recall/speed tuning (default 40, recommend 100-200 for high recall)
CREATE INDEX IF NOT EXISTS idx_{{.Table}}_embedding_hnsw
    ON {{.Schema}}.{{.Table}}
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 32, ef_construction = 200);

-- Convert to TimescaleDB hypertable for automatic chunk management
SELECT create_hypertable('{{.Schema}}.{{.Table}}', 'timestamp_ns',
    chunk_time_interval => 86400000000000, -- 1 day in nanoseconds
    if_not_exists => TRUE
);

-- Set integer_now_func with error handling
-- Note: Using get_unix_ns_now (nanoseconds) to match timestamp_ns column
DO $$
BEGIN
    PERFORM set_integer_now_func('{{.Schema}}.{{.Table}}', 'public.get_unix_ns_now');
EXCEPTION
    WHEN OTHERS THEN
        -- Function already set, ignore the error
        NULL;
END $$;

-- =====================================================
-- COLUMNSTORE CONFIGURATION (TimescaleDB 2.23+)
-- =====================================================
ALTER TABLE {{.Schema}}.{{.Table}} SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'service_name',
    timescaledb.orderby = 'timestamp_ns DESC'
);

-- Schedule columnstore conversion for chunks older than 7 days
DO $$
BEGIN
    CALL add_columnstore_policy('{{.Schema}}.{{.Table}}', after => 604800000000000::bigint, if_not_exists => TRUE);
EXCEPTION
    WHEN undefined_function THEN
        PERFORM add_compression_policy('{{.Schema}}.{{.Table}}', 604800000000000, if_not_exists => TRUE);
END $$;

-- Retention policy: Drop chunks older than 30 days
SELECT add_retention_policy('{{.Schema}}.{{.Table}}', 2592000000000000, if_not_exists => TRUE);

-- GIN index on metadata for filtering before vector search
CREATE INDEX IF NOT EXISTS idx_{{.Table}}_metadata ON {{.Schema}}.{{.Table}} USING GIN (metadata) WHERE metadata IS NOT NULL;

-- Note: Vector indexes are created on the parent table and inherited by chunks.
-- TimescaleDB handles chunk management automatically.
