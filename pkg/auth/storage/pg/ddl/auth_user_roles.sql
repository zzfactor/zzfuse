-- User roles
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_user_roles (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    role VARCHAR(64) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS auth_user_roles_user_id_role_idx ON {{.Schema}}.auth_user_roles(user_id, role);

