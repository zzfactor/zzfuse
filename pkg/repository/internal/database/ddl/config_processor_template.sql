-- Config Embedding DDL

-- Replace all %s or hardcoded schema with {{.Schema}}
-- Embedding Configuration Table
-- This table stores LLM/embedding provider configurations
-- Schema placeholders will be replaced with the schema name (e.g., "admin", "public")

-- Main embedding table
CREATE TABLE IF NOT EXISTS {{.Schema}}.config_processor_template (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    content JSONB DEFAULT '{}',
    template_metadata JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create trigger function for updated_at
CREATE OR REPLACE FUNCTION update_embedding_template_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for updated_at
DROP TRIGGER IF EXISTS update_config_embedding_template_updated_at ON {{.Schema}}.config_processor_template;
CREATE TRIGGER update_config_embedding_template_updated_at
    BEFORE UPDATE ON {{.Schema}}.config_processor_template
    FOR EACH ROW
    EXECUTE FUNCTION update_embedding_template_updated_at_column();
