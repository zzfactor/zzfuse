-- Token blacklist
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_token_blacklist (
    jti TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_auth_token_blacklist_expires ON {{.Schema}}.auth_token_blacklist(expires_at);

