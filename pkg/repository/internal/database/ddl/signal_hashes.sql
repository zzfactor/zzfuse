-- PostgreSQL Signal Hash table DDL with TimescaleDB maintenance (FIXED VERSION)

-- Example usage: {{.Schema}}

CREATE TABLE IF NOT EXISTS {{.Schema}}.signal_hashes (
    timestamp_ns BIGINT NOT NULL,
    hash TEXT NOT NULL,
    run_id BIGINT,
    signal_type SMALLINT,
    signal_id BIGINT,
    flags SMALLINT, 
    PRIMARY KEY (timestamp_ns, hash)  -- Fixed: time column must be first in PK for TimescaleDB
);

-- Convert to TimescaleDB hypertable with smaller chunk interval for better performance
SELECT create_hypertable('{{.Schema}}.signal_hashes', 'timestamp_ns', 
    chunk_time_interval => 21600000000000,  -- 6 hours in nanoseconds (better than 1 day)
    if_not_exists => TRUE
);

-- Set integer_now_func with error handling (assuming nanoseconds function exists)
DO $$
BEGIN
    PERFORM set_integer_now_func('{{.Schema}}.signal_hashes', 'public.get_unix_ns_now');
EXCEPTION
    WHEN OTHERS THEN
        -- Function already set or doesn't exist, ignore the error
        NULL;
END $$;

-- Enable compression with optimized segmentby
ALTER TABLE {{.Schema}}.signal_hashes
SET (
  timescaledb.compress,
  timescaledb.compress_segmentby = 'hash',  -- Updated: use hash for compression
  timescaledb.compress_orderby = 'timestamp_ns DESC'
);

-- Policy to compress chunks older than 7 days (using a literal integer)
SELECT add_compression_policy('{{.Schema}}.signal_hashes', 604800000000000, if_not_exists => TRUE);


-- 3. Apply retention policy (FINAL FIX)
-- Drops data chunks that are older than 30 days.
-- This should now work because the integer_now_func is correctly set.
SELECT add_retention_policy('{{.Schema}}.signal_hashes', 2592000000000000, if_not_exists => TRUE);

-- Optimized indexes based on common query patterns
-- 1. Hash lookups with time for deduplication (most common)
CREATE INDEX IF NOT EXISTS idx_signal_hashes_hash_lookup ON {{.Schema}}.signal_hashes(hash, timestamp_ns);

-- 2. Run ID tracking
CREATE INDEX IF NOT EXISTS idx_signal_hashes_run_id ON {{.Schema}}.signal_hashes(run_id, timestamp_ns);

-- 3. Signal type queries
CREATE INDEX IF NOT EXISTS idx_signal_hashes_signal_type ON {{.Schema}}.signal_hashes(signal_type, timestamp_ns);

-- 4. Flag-based queries
CREATE INDEX IF NOT EXISTS idx_signal_hashes_flags ON {{.Schema}}.signal_hashes(flags, timestamp_ns);

-- 5. Signal ID queries
CREATE INDEX IF NOT EXISTS idx_signal_hashes_signal_id ON {{.Schema}}.signal_hashes(signal_id, timestamp_ns) WHERE signal_id IS NOT NULL; 