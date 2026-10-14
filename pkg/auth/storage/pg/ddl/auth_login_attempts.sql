-- Login attempts
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_login_attempts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    ip_address VARCHAR(64),
    success BOOLEAN NOT NULL,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_auth_login_attempts_user ON {{.Schema}}.auth_login_attempts(user_id);
CREATE INDEX IF NOT EXISTS idx_auth_login_attempts_time ON {{.Schema}}.auth_login_attempts(attempted_at);

