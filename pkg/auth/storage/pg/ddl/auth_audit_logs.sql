-- Audit logs
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_id BIGINT,
    action VARCHAR(64) NOT NULL,
    target_user_id BIGINT,
    details TEXT,
    ip_address VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
