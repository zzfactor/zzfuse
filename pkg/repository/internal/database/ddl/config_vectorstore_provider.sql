-- Config Vectorstore DDL

-- Replace all %s or hardcoded schema with {{.Schema}}
-- Vector Store Configuration Table
-- This table stores vector store connection and behavior configurations
-- Schema placeholders will be replaced with the schema name (e.g., "admin", "public")

CREATE TABLE IF NOT EXISTS {{.Schema}}.config_vectorstore_provider (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    provider VARCHAR(255) NOT NULL,
    endpoint VARCHAR(255),
    "user" VARCHAR(255) NOT NULL,
    database VARCHAR(255) NOT NULL,
    embedding_id BIGINT NOT NULL,
    batch_size INTEGER,
    flush_interval_ms INTEGER,
    max_retries INTEGER,
    max_open_conns INTEGER,
    max_idle_conns INTEGER,
    conn_max_lifetime_ms INTEGER,
    batch_error_policy VARCHAR(50),
    max_concurrent_batches INTEGER,
    vendor_specific_options JSONB DEFAULT '{}',
    description TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Create trigger function for updated_at
CREATE OR REPLACE FUNCTION update_vectorstore_config_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create trigger for updated_at
DROP TRIGGER IF EXISTS update_config_vectorstore_updated_at ON {{.Schema}}.config_vectorstore_provider;
CREATE TRIGGER update_config_vectorstore_updated_at
    BEFORE UPDATE ON {{.Schema}}.config_vectorstore_provider
    FOR EACH ROW
    EXECUTE FUNCTION update_vectorstore_config_updated_at_column();

-- Note: Default configurations are created programmatically in Go code
-- See config_vectorstore_provider.go CreateDefaultVectorStoreConfig() method Can you check if we set the value?

-- Secret Reference Best Practice:
-- Do NOT store plaintext passwords or API keys in this table.
-- Instead, store a reference in the metadata JSONB column, e.g.:
--   { "secrets": { "api_key_env": "VECTORSTORE_API_KEY" } }
-- At runtime, resolve the reference from env vars or a secret manager.