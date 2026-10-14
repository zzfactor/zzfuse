-- TimescaleDB Management Functions for trazr-otel
-- This provides TimescaleDB-specific functions for managing hypertables

-- Function to setup TimescaleDB hypertables with consistent configuration
-- Note: Retention policies require TimescaleDB Community/Enterprise license
CREATE OR REPLACE FUNCTION setup_timescaledb_hypertable(
    p_table_name TEXT,
    p_time_column TEXT DEFAULT 'timestamp_ns',
    p_chunk_interval TEXT DEFAULT '1 day',
    p_compression_interval TEXT DEFAULT '7 days'
) RETURNS void AS $$
DECLARE
    full_table_name TEXT;
    hypertable_exists BOOLEAN;
BEGIN
    -- Construct full table name with schema
    full_table_name := '{{.Schema}}.' || p_table_name;
    
    -- Check if hypertable already exists
    SELECT EXISTS (
        SELECT 1 FROM timescaledb_information.hypertables 
        WHERE hypertable_name = p_table_name 
        AND hypertable_schema = '{{.Schema}}'
    ) INTO hypertable_exists;
    
    IF NOT hypertable_exists THEN
        -- Create hypertable
        EXECUTE format('SELECT create_hypertable(%L, %L, chunk_time_interval => %L::interval, if_not_exists => TRUE)',
                      full_table_name, p_time_column, p_chunk_interval);
        
        RAISE NOTICE 'Created TimescaleDB hypertable for % with % chunks', p_table_name, p_chunk_interval;
    ELSE
        RAISE NOTICE 'Hypertable already exists for %', p_table_name;
    END IF;
    
    -- Note: Retention policies require TimescaleDB Community/Enterprise license
    -- For Apache license, manual chunk management is required
    -- To drop old chunks manually: SELECT drop_chunks(full_table_name, older_than => INTERVAL '30 days');
    
    RAISE NOTICE 'Retention policy not set (requires Community/Enterprise license) for %', p_table_name;
    
    -- Setup compression policy (if TimescaleDB Enterprise/Cloud is available)
    BEGIN
        EXECUTE format('SELECT add_compression_policy(%L, %L::interval, if_not_exists => TRUE)',
                      full_table_name, p_compression_interval);
        RAISE NOTICE 'Compression policy set for %: %', p_table_name, p_compression_interval;
    EXCEPTION
        WHEN undefined_function THEN
            RAISE NOTICE 'Compression not available (requires TimescaleDB Enterprise/Cloud) for %', p_table_name;
    END;
END;
$$ LANGUAGE plpgsql;

-- Function to setup all TimescaleDB tables with consistent configuration
CREATE OR REPLACE FUNCTION setup_all_timescaledb_tables() RETURNS void AS $$
BEGIN
    -- Setup all OpenTelemetry tables as TimescaleDB hypertables
    -- Note: Retention policies require TimescaleDB Community/Enterprise license
    PERFORM setup_timescaledb_hypertable('logs');
    PERFORM setup_timescaledb_hypertable('spans');
    PERFORM setup_timescaledb_hypertable('gauge_metrics');
    PERFORM setup_timescaledb_hypertable('histogram_metrics');
    PERFORM setup_timescaledb_hypertable('sum_metrics');
    PERFORM setup_timescaledb_hypertable('summary_metrics');
    PERFORM setup_timescaledb_hypertable('exponential_histogram_metrics');
    PERFORM setup_timescaledb_hypertable('profiles');
    PERFORM setup_timescaledb_hypertable('signal_hashes');
    
    RAISE NOTICE 'All TimescaleDB hypertables configured successfully';
END;
$$ LANGUAGE plpgsql;

-- Function to get TimescaleDB hypertable information
CREATE OR REPLACE FUNCTION get_timescaledb_info(p_table_name TEXT DEFAULT NULL) 
RETURNS TABLE(
    hypertable_name TEXT,
    chunk_count BIGINT,
    total_size TEXT,
    compression_enabled BOOLEAN,
    retention_policy TEXT,
    compression_policy TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        h.hypertable_name::TEXT,
        COUNT(c.chunk_name)::BIGINT as chunk_count,
        pg_size_pretty(SUM(pg_total_relation_size(c.chunk_name::regclass)))::TEXT as total_size,
        h.compression_enabled,
        rp.retention_period::TEXT as retention_policy,
        cp.compress_after::TEXT as compression_policy
    FROM timescaledb_information.hypertables h
    LEFT JOIN timescaledb_information.chunks c ON h.hypertable_name = c.hypertable_name
    LEFT JOIN timescaledb_information.jobs rp ON h.hypertable_name = rp.hypertable_name AND rp.proc_name = 'policy_retention'
    LEFT JOIN timescaledb_information.jobs cp ON h.hypertable_name = cp.hypertable_name AND cp.proc_name = 'policy_compression'
    WHERE (p_table_name IS NULL OR h.hypertable_name = p_table_name)
    AND h.hypertable_schema = '{{.Schema}}'
    GROUP BY h.hypertable_name, h.compression_enabled, rp.retention_period, cp.compress_after
    ORDER BY h.hypertable_name;
END;
$$ LANGUAGE plpgsql;

-- Function to create continuous aggregates for common queries
CREATE OR REPLACE FUNCTION setup_continuous_aggregates() RETURNS void AS $$
BEGIN
    -- Continuous aggregate for hourly error counts
    BEGIN
        EXECUTE format('
            CREATE MATERIALIZED VIEW IF NOT EXISTS {{.Schema}}.hourly_error_counts
            WITH (timescaledb.continuous) AS
            SELECT
                time_bucket(''1 hour'', timestamp) AS bucket,
                service_name,
                COUNT(*) AS error_count
            FROM {{.Schema}}.logs
            WHERE severity_number >= 17
            GROUP BY 1, 2
        ');
        
        -- Add refresh policy
        EXECUTE format('
            SELECT add_continuous_aggregate_policy(''{{.Schema}}.hourly_error_counts'',
                start_offset => INTERVAL ''1 week'',
                end_offset => INTERVAL ''1 hour'',
                schedule_interval => INTERVAL ''1 hour'')
        ');
        
        RAISE NOTICE 'Created continuous aggregate: hourly_error_counts';
    EXCEPTION
        WHEN duplicate_object THEN
            RAISE NOTICE 'Continuous aggregate hourly_error_counts already exists';
    END;
    
    -- Continuous aggregate for hourly trace counts
    BEGIN
        EXECUTE format('
            CREATE MATERIALIZED VIEW IF NOT EXISTS {{.Schema}}.hourly_trace_counts
            WITH (timescaledb.continuous) AS
            SELECT
                time_bucket(''1 hour'', timestamp) AS bucket,
                service_name,
                COUNT(*) AS trace_count
            FROM {{.Schema}}.spans
            GROUP BY 1, 2
        ');
        
        -- Add refresh policy
        EXECUTE format('
            SELECT add_continuous_aggregate_policy(''{{.Schema}}.hourly_trace_counts'',
                start_offset => INTERVAL ''1 week'',
                end_offset => INTERVAL ''1 hour'',
                schedule_interval => INTERVAL ''1 hour'')
        ');
        
        RAISE NOTICE 'Created continuous aggregate: hourly_trace_counts';
    EXCEPTION
        WHEN duplicate_object THEN
            RAISE NOTICE 'Continuous aggregate hourly_trace_counts already exists';
    END;
    
    -- Continuous aggregate for hourly metric averages
    BEGIN
        EXECUTE format('
            CREATE MATERIALIZED VIEW IF NOT EXISTS {{.Schema}}.hourly_metric_averages
            WITH (timescaledb.continuous) AS
            SELECT
                time_bucket(''1 hour'', timestamp) AS bucket,
                service_name,
                name as metric_name,
                AVG(value) AS avg_value,
                MIN(value) AS min_value,
                MAX(value) AS max_value,
                COUNT(*) AS sample_count
            FROM {{.Schema}}.gauge_metrics
            WHERE value IS NOT NULL
            GROUP BY 1, 2, 3
        ');
        
        -- Add refresh policy
        EXECUTE format('
            SELECT add_continuous_aggregate_policy(''{{.Schema}}.hourly_metric_averages'',
                start_offset => INTERVAL ''1 week'',
                end_offset => INTERVAL ''1 hour'',
                schedule_interval => INTERVAL ''1 hour'')
        ');
        
        RAISE NOTICE 'Created continuous aggregate: hourly_metric_averages';
    EXCEPTION
        WHEN duplicate_object THEN
            RAISE NOTICE 'Continuous aggregate hourly_metric_averages already exists';
    END;
    
    RAISE NOTICE 'All continuous aggregates configured successfully';
END;
$$ LANGUAGE plpgsql;

-- Function to get continuous aggregate information
CREATE OR REPLACE FUNCTION get_continuous_aggregate_info() 
RETURNS TABLE(
    view_name TEXT,
    materialized_only BOOLEAN,
    compression_enabled BOOLEAN,
    refresh_policy TEXT
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        c.view_name::TEXT,
        c.materialized_only,
        c.compression_enabled,
        j.schedule_interval::TEXT as refresh_policy
    FROM timescaledb_information.continuous_aggregates c
    LEFT JOIN timescaledb_information.jobs j ON c.view_name = j.hypertable_name AND j.proc_name = 'policy_refresh_continuous_aggregate'
    WHERE c.view_schema = '{{.Schema}}'
    ORDER BY c.view_name;
END;
$$ LANGUAGE plpgsql; 