-- API Keys table
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_apikeys (
    id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    org_id BIGINT NULL,
    project_id BIGINT NULL,
    name VARCHAR(255) NOT NULL,
    prefix VARCHAR(16) NOT NULL,
    key_hash TEXT NOT NULL,
    scopes JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL
);
ALTER TABLE {{.Schema}}.auth_apikeys ADD COLUMN IF NOT EXISTS org_id BIGINT NULL;
ALTER TABLE {{.Schema}}.auth_apikeys ADD COLUMN IF NOT EXISTS project_id BIGINT NULL;
CREATE INDEX IF NOT EXISTS idx_auth_apikeys_user ON {{.Schema}}.auth_apikeys(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_apikeys_prefix ON {{.Schema}}.auth_apikeys(prefix);
CREATE INDEX IF NOT EXISTS idx_auth_apikeys_org ON {{.Schema}}.auth_apikeys(org_id);
CREATE INDEX IF NOT EXISTS idx_auth_apikeys_project ON {{.Schema}}.auth_apikeys(project_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_auth_apikeys_prefix ON {{.Schema}}.auth_apikeys(prefix);
CREATE INDEX IF NOT EXISTS idx_auth_apikeys_prefix_hash ON {{.Schema}}.auth_apikeys(prefix, key_hash);

