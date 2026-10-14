-- =====================================================
-- TimescaleDB Maintenance Functions for Trazr-Otel
-- =====================================================
-- This script provides comprehensive maintenance functions
-- for TimescaleDB hypertables (Apache license compatible)

-- =====================================================
-- 2. HYPERTABLE STATISTICS AND MONITORING
-- =====================================================

-- Function to get comprehensive hypertable statistics
CREATE OR REPLACE FUNCTION get_hypertable_statistics(p_schema TEXT DEFAULT NULL) 
RETURNS TABLE(
    table_name TEXT,
    chunk_count BIGINT,
    total_size TEXT,
    total_size_bytes BIGINT,
    oldest_chunk_start TEXT,
    newest_chunk_end TEXT,
    compression_enabled BOOLEAN
) AS $$
DECLARE
    schema_name TEXT;
BEGIN
    -- Determine schema
    schema_name := COALESCE(p_schema, current_setting('search_path', true));
    IF schema_name = 'default' THEN
        schema_name := 'public';
    END IF;
    
    RETURN QUERY
    SELECT 
        h.hypertable_name::TEXT,
        COUNT(c.chunk_name)::BIGINT as chunk_count,
        pg_size_pretty(SUM(pg_total_relation_size(c.chunk_name::regclass)))::TEXT as total_size,
        SUM(pg_total_relation_size(c.chunk_name::regclass))::BIGINT as total_size_bytes,
        MIN(c.range_start)::TEXT as oldest_chunk_start,
        MAX(c.range_end)::TEXT as newest_chunk_end,
        h.compression_enabled
    FROM timescaledb_information.hypertables h
    LEFT JOIN timescaledb_information.chunks c ON h.hypertable_name = c.hypertable_name
    WHERE h.hypertable_schema = schema_name
    GROUP BY h.hypertable_name, h.compression_enabled
    ORDER BY h.hypertable_name;
END;
$$ LANGUAGE plpgsql;

-- Function to get detailed chunk information for a specific table
CREATE OR REPLACE FUNCTION get_chunk_information(
    p_table_name TEXT,
    p_schema TEXT DEFAULT NULL
) RETURNS TABLE(
    chunk_name TEXT,
    range_start TEXT,
    range_end TEXT,
    size TEXT,
    size_bytes BIGINT,
    row_count BIGINT
) AS $$
DECLARE
    schema_name TEXT;
BEGIN
    -- Determine schema
    schema_name := COALESCE(p_schema, current_setting('search_path', true));
    IF schema_name = 'default' THEN
        schema_name := 'public';
    END IF;
    
    RETURN QUERY
    SELECT 
        c.chunk_name::TEXT,
        c.range_start::TEXT,
        c.range_end::TEXT,
        pg_size_pretty(pg_total_relation_size(c.chunk_name::regclass))::TEXT as size,
        pg_total_relation_size(c.chunk_name::regclass)::BIGINT as size_bytes,
        COALESCE(s.n_tup_ins + s.n_tup_upd - s.n_tup_del, 0)::BIGINT as row_count
    FROM timescaledb_information.chunks c
    LEFT JOIN pg_stat_user_tables s ON c.chunk_name::regclass = s.relid
    WHERE c.hypertable_name = p_table_name 
    AND c.hypertable_schema = schema_name
    ORDER BY c.range_start;
END;
$$ LANGUAGE plpgsql;

-- ===================================================== 