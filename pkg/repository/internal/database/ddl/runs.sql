-- PostgreSQL Runs table DDL

-- Example usage: {{.Schema}}

CREATE TABLE IF NOT EXISTS {{.Schema}}.runs (
    id BIGSERIAL PRIMARY KEY,
    session_id TEXT,
    service_name TEXT NOT NULL,
    scope_name TEXT,
    name TEXT NOT NULL, 
    start_timestamp_ns BIGINT NOT NULL,
    end_timestamp_ns BIGINT,
    trace_id TEXT,
    processor_id BIGINT,
    run_type SMALLINT, -- 1: time based, 2: trace based
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Essential indexes for the new schema
-- 1. Service name + name for GetActiveIdByName and GetCount
CREATE INDEX IF NOT EXISTS idx_runs_service_name_name ON {{.Schema}}.runs(service_name, name);

-- 2. Name for GetAllIds
CREATE INDEX IF NOT EXISTS idx_runs_name ON {{.Schema}}.runs(name);

-- 3. Start timestamp for time-based queries
CREATE INDEX IF NOT EXISTS idx_runs_start_timestamp ON {{.Schema}}.runs(start_timestamp_ns);

-- 4. Processor ID for GetByProcessorID
CREATE INDEX IF NOT EXISTS idx_runs_processor_id ON {{.Schema}}.runs(processor_id);

-- 5. Trace ID for GetByTraceID
CREATE INDEX IF NOT EXISTS idx_runs_trace_id ON {{.Schema}}.runs(trace_id) WHERE trace_id IS NOT NULL;

-- 6. Run type for filtering
CREATE INDEX IF NOT EXISTS idx_runs_run_type ON {{.Schema}}.runs(run_type);

-- 7. Session ID for session-based queries
CREATE INDEX IF NOT EXISTS idx_runs_session_id ON {{.Schema}}.runs(session_id) WHERE session_id IS NOT NULL;

-- 8. Index for retention cleanup (created_at based deletion)
CREATE INDEX IF NOT EXISTS idx_runs_created_at ON {{.Schema}}.runs(created_at);

-- =====================================================
-- RETENTION STRATEGY
-- =====================================================
-- Unlike hypertables, the runs table needs manual retention management.
-- This function deletes completed runs older than 30 days.
-- Active runs (end_timestamp_ns IS NULL) are preserved regardless of age.

CREATE OR REPLACE FUNCTION {{.Schema}}.cleanup_old_runs(retention_days INTEGER DEFAULT 30)
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    DELETE FROM {{.Schema}}.runs
    WHERE created_at < NOW() - (retention_days || ' days')::INTERVAL
      AND end_timestamp_ns IS NOT NULL;  -- Only delete completed runs

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Note: To enable automatic retention via pg_cron (requires pg_cron extension):
-- SELECT cron.schedule('cleanup-runs-{{.Schema}}', '0 3 * * *',
--   $$SELECT {{.Schema}}.cleanup_old_runs(30)$$);
--
-- Or call manually: SELECT {{.Schema}}.cleanup_old_runs(30);

