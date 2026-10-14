-- Auth users table
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255),
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
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

CREATE UNIQUE INDEX IF NOT EXISTS auth_users_username_unique ON {{.Schema}}.auth_users (LOWER(username));

UPDATE {{.Schema}}.auth_users SET username = email WHERE (username IS NULL OR username = '') AND email IS NOT NULL;

