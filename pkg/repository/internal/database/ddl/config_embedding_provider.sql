-- Config Embedding DDL

-- Replace all %s or hardcoded schema with {{.Schema}}
-- Embedding Configuration Table
-- This table stores LLM/embedding provider configurations
-- Schema placeholders will be replaced with the schema name (e.g., "admin", "public")

-- Main embedding table
CREATE TABLE IF NOT EXISTS {{.Schema}}.config_embedding_provider (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    provider VARCHAR(255) NOT NULL,
    endpoint VARCHAR(255),
    model TEXT,
    timeout_ms INTEGER,
    rate_limit INTEGER,
    headers JSONB DEFAULT '{}'::jsonb,
    normalize BOOLEAN DEFAULT FALSE,
    dimension INTEGER,
    max_context_length INTEGER DEFAULT 2048,
    description TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create trigger function for updated_at
CREATE OR REPLACE FUNCTION update_embedding_config_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for updated_at
DROP TRIGGER IF EXISTS update_config_embedding_updated_at ON {{.Schema}}.config_embedding_provider;
CREATE TRIGGER update_config_embedding_updated_at
    BEFORE UPDATE ON {{.Schema}}.config_embedding_provider
    FOR EACH ROW
    EXECUTE FUNCTION update_embedding_config_updated_at_column();
