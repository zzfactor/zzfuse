-- MFA secrets
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_mfa_secrets (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    secret VARCHAR(255) NOT NULL,
    enabled BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_mfa_secrets_user ON {{.Schema}}.auth_mfa_secrets(user_id);

