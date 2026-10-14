-- Email verifications
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_email_verifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_auth_emailver_user ON {{.Schema}}.auth_email_verifications(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_emailver_token ON {{.Schema}}.auth_email_verifications(token_hash);

