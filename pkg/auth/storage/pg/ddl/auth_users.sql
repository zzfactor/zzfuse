-- Auth users table
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    auth_version BIGINT NOT NULL DEFAULT 1,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    tenant_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    name VARCHAR(255),
    first_name VARCHAR(255),
    last_name VARCHAR(255),
    address VARCHAR(255),
    city VARCHAR(255),
    state VARCHAR(255),
    country VARCHAR(255),
    company VARCHAR(255),
    metadata JSONB
);

ALTER TABLE {{.Schema}}.auth_users ADD COLUMN IF NOT EXISTS username VARCHAR(255);

-- Legacy JWTs carry no auth_version and are deliberately rejected. Existing
-- accounts retain their identity/roles and must log in again after this upgrade.
ALTER TABLE {{.Schema}}.auth_users ADD COLUMN IF NOT EXISTS auth_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE {{.Schema}}.auth_users ADD COLUMN IF NOT EXISTS disabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE {{.Schema}}.auth_users ADD COLUMN IF NOT EXISTS tenant_id TEXT NOT NULL DEFAULT '';

-- Empty grants deny non-admins. No implicit all-schema grant is backfilled.
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_user_schema_grants (
    user_id BIGINT NOT NULL REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    schema_name TEXT NOT NULL CHECK (schema_name ~ '^[a-z_][a-z0-9_]*$' AND length(schema_name) <= 63),
    PRIMARY KEY (user_id, schema_name)
);

CREATE UNIQUE INDEX IF NOT EXISTS auth_users_username_unique ON {{.Schema}}.auth_users (LOWER(username));

UPDATE {{.Schema}}.auth_users SET username = email WHERE (username IS NULL OR username = '') AND email IS NOT NULL;

