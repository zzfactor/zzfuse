-- OAuth identities
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_user_identities (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    provider_user_id VARCHAR(128) NOT NULL,
    email VARCHAR(255),
    UNIQUE(provider, provider_user_id)
);
