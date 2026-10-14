-- =====================================================
-- TRAZR-OTEL DATABASE INITIALIZATION
-- =====================================================
-- Optimized for:
--   - PostgreSQL 18+ (async I/O, skip scan, parallel GIN)
--   - TimescaleDB 2.23+ (columnstore, dynamic compression, bloom filters)
--   - OpenTelemetry observability workloads
--
-- Version Requirements:
--   - PostgreSQL: 16, 17, or 18 (18 recommended for best performance)
--   - TimescaleDB: 2.20+ (2.23+ recommended for columnstore features)
--   - pgvector/vectorscale: Latest for embedding support
-- =====================================================

-- Create database (run this separately as superuser if needed)
-- CREATE DATABASE trazr_otel;

-- Connect to trazr_otel database and enable required extensions
-- Note: Extension order matters - TimescaleDB should be loaded early

-- =====================================================
-- REQUIRED EXTENSIONS
-- =====================================================

-- Core PostgreSQL extensions
CREATE EXTENSION IF NOT EXISTS hstore;           -- Key-value storage for JSON-like data
CREATE EXTENSION IF NOT EXISTS btree_gin;        -- GIN index support for B-tree operations
CREATE EXTENSION IF NOT EXISTS pg_trgm;          -- Trigram matching for text search

-- Vector extensions for embeddings (CASCADE ensures vector is installed first)
CREATE EXTENSION IF NOT EXISTS vectorscale CASCADE;

-- Time-series database extension (must be loaded early)
-- TimescaleDB 2.23+ provides: columnstore, automatic bloom filters, dynamic compression
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Cryptographic functions for hashing
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Query performance monitoring (essential for observability)
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

-- =====================================================
-- HELPER FUNCTIONS
-- =====================================================

--------------------------------------------------------------------------------
-- Function to get current time in nanoseconds (for integer hypertables)
-- Used by retention and compression policies for nanosecond-precision tables.
-- PARALLEL SAFE for PostgreSQL 18 parallel query support.
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.get_unix_ns_now() RETURNS BIGINT
    LANGUAGE SQL IMMUTABLE PARALLEL SAFE AS
$$
    SELECT (EXTRACT(EPOCH FROM clock_timestamp()) * 1000000000)::BIGINT;
$$;

--------------------------------------------------------------------------------
-- SHA256 hash function for content deduplication
-- PARALLEL SAFE for PostgreSQL 18 parallel query support.
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.hash_content_sha256(content text) RETURNS text AS $$
BEGIN
    RETURN encode(digest(content, 'sha256'), 'hex');
END;
$$ LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE;

--------------------------------------------------------------------------------
-- UUIDv7 generation function (PostgreSQL 18 native, fallback for older versions)
-- UUIDv7 provides time-ordered UUIDs for better B-tree index performance.
-- TimescaleDB 2.23+ has native UUIDv7 compression optimization.
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION public.generate_uuidv7() RETURNS uuid AS $$
DECLARE
    unix_ts_ms BIGINT;
    uuid_bytes BYTEA;
BEGIN
    -- Get current Unix timestamp in milliseconds
    unix_ts_ms := (EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::BIGINT;

    -- Build UUIDv7: 48-bit timestamp + 4-bit version (7) + 12-bit random + 2-bit variant + 62-bit random
    uuid_bytes :=
        substring(int8send(unix_ts_ms) FROM 3 FOR 6) ||  -- 48-bit timestamp
        set_bit(gen_random_bytes(2), 15, 0) ||           -- version 7 (0111)
        set_bit(set_bit(gen_random_bytes(8), 63, 1), 62, 0);  -- variant 10

    -- Set version bits (0111xxxx for version 7)
    uuid_bytes := set_byte(uuid_bytes, 6, (get_byte(uuid_bytes, 6) & x'0f'::int) | x'70'::int);

    RETURN encode(uuid_bytes, 'hex')::uuid;
END;
$$ LANGUAGE plpgsql VOLATILE PARALLEL SAFE;


