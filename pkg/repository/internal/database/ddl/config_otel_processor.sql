-- Config Repository Table DDL

-- Schema to vector store mapping table (1:N relationship)
CREATE TABLE IF NOT EXISTS {{.Schema}}.config_otel_processor (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    database_id BIGINT NOT NULL,
    vectorstore_id BIGINT,
    template_id BIGINT,
    processing_mode VARCHAR(50) NOT NULL DEFAULT 'all', -- 'logs', 'spans', 'metrics', 'all'
    enabled BOOLEAN DEFAULT TRUE,
    routing JSONB DEFAULT '{}'::jsonb,
    filter JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

