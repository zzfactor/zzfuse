-- =====================================================
-- PROFILES TABLE (TimescaleDB Hypertable with Columnstore)
-- =====================================================
-- Optimized for:
--   - PostgreSQL 18+ (async I/O, skip scan, parallel GIN)
--   - TimescaleDB 2.23+ (columnstore with automatic bloom filters)
--   - OpenTelemetry profiling data (pprof format)
-- =====================================================

CREATE TABLE IF NOT EXISTS {{.Schema}}.profiles (
    id BIGSERIAL NOT NULL,
    timestamp_ns BIGINT NOT NULL,
    timestamp TIMESTAMP WITH TIME ZONE GENERATED ALWAYS AS (to_timestamp(timestamp_ns / 1000000000.0)) STORED,
    -- Service entity fields (OTel v1.39.0 splits service into namespace/service/instance)
    service_name TEXT,
    service_namespace TEXT,              -- OTel v1.39.0: service.namespace
    service_instance_id TEXT,            -- OTel v1.39.0: service.instance.id
    profile_type TEXT NOT NULL,           -- e.g., cpu, heap, goroutine, etc.
    duration BIGINT,                   -- Duration of the profile window
    sample_type TEXT,                     -- e.g., cpu, alloc_objects, etc.
    sample_unit TEXT,                     -- e.g., nanoseconds, bytes, etc.
    profile_bytes BYTEA NOT NULL,         -- Raw pprof profile data (protobuf-encoded)
    attributes JSONB DEFAULT '{}'::jsonb,
    resource_attributes JSONB DEFAULT '{}'::jsonb,
    scope_name TEXT,
    scope_version TEXT,
    scope_attributes JSONB DEFAULT '{}'::jsonb,
    scope_dropped_attr_count INTEGER DEFAULT 0,
    metadata JSONB DEFAULT '{}'::jsonb,
    resource_schema_url TEXT,
    scope_schema_url TEXT,
    PRIMARY KEY (id, timestamp_ns)
);

-- Applied for fresh and existing tables before policies; no payload rewriting.
{{.ProfileStorageMigration}}

-- Convert to TimescaleDB hypertable using BIGINT timestamp_ns (nanoseconds)
SELECT create_hypertable('{{.Schema}}.profiles', 'timestamp_ns', 
    chunk_time_interval => 86400000000000, -- 1 day in nanoseconds
    if_not_exists => TRUE
);

-- Set integer_now_func with error handling
-- Note: Using get_unix_ns_now (nanoseconds) to match timestamp_ns column
DO $$
BEGIN
    PERFORM set_integer_now_func('{{.Schema}}.profiles', 'public.get_unix_ns_now');
EXCEPTION
    WHEN OTHERS THEN
        -- Function already set, ignore the error
        NULL;
END $$;

-- =====================================================
-- COLUMNSTORE CONFIGURATION (TimescaleDB 2.23+)
-- =====================================================
-- Retain the layout of existing compressed tables/chunks. Upgrading the profile
-- wire representation does not require rewriting their segment-by configuration.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables
        WHERE hypertable_schema = '{{.SchemaName}}' AND hypertable_name = 'profiles'
          AND compression_enabled
    ) THEN
        ALTER TABLE {{.Schema}}.profiles SET (
            timescaledb.enable_columnstore,
            timescaledb.segmentby = 'service_name',
            timescaledb.orderby = 'timestamp_ns DESC'
        );
    END IF;
END $$;

-- Schedule columnstore conversion for chunks older than 7 days
DO $$
BEGIN
    CALL add_columnstore_policy('{{.Schema}}.profiles', after => 604800000000000::bigint, if_not_exists => TRUE);
EXCEPTION
    WHEN undefined_function THEN
        PERFORM add_compression_policy('{{.Schema}}.profiles', 604800000000000, if_not_exists => TRUE);
END $$;

-- Retention policy: Drop chunks older than 30 days
-- Delay the first newly installed job so schema bootstrap cannot immediately
-- delete historical rows. Existing policies are left unchanged.
SELECT add_retention_policy('{{.Schema}}.profiles', 2592000000000000,
    initial_start => now() + INTERVAL '1 day', if_not_exists => TRUE);

-- 1. Time-based queries (most common for observability)
CREATE INDEX IF NOT EXISTS idx_profiles_timestamp ON {{.Schema}}.profiles (timestamp);
CREATE INDEX IF NOT EXISTS idx_profiles_service_time ON {{.Schema}}.profiles (service_name, timestamp);

-- 2. Profile identification
CREATE INDEX IF NOT EXISTS idx_profiles_service_type ON {{.Schema}}.profiles (service_name, profile_type);

-- 3. Attribute searching (simple GIN, not complex akeys/avals)
CREATE INDEX IF NOT EXISTS idx_profiles_attributes ON {{.Schema}}.profiles USING GIN (attributes);
CREATE INDEX IF NOT EXISTS idx_profiles_resource_attrs ON {{.Schema}}.profiles USING GIN (resource_attributes);

-- 4. Metadata JSONB index (PostgreSQL 18 parallel GIN build support)
CREATE INDEX IF NOT EXISTS idx_profiles_metadata ON {{.Schema}}.profiles USING GIN (metadata) WHERE metadata IS NOT NULL;

-- 5. Profile type + sample type for filtering
CREATE INDEX IF NOT EXISTS idx_profiles_type_sample ON {{.Schema}}.profiles (profile_type, sample_type, timestamp_ns);

