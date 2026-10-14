-- Config Schema Table DDL

-- Main schema configuration table
CREATE TABLE IF NOT EXISTS {{.Schema}}.config_otel_database (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255),
    schema VARCHAR(255) UNIQUE NOT NULL DEFAULT 'otel',
    enabled BOOLEAN DEFAULT TRUE,
    routing JSONB DEFAULT '{}'::jsonb,
    priority INT UNIQUE NOT NULL DEFAULT 9999,
    description TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);