-- OAuth identities
CREATE TABLE IF NOT EXISTS {{.Schema}}.auth_user_identities (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES {{.Schema}}.auth_users(id) ON DELETE CASCADE,
    provider VARCHAR(32) NOT NULL,
    provider_user_id VARCHAR(128) NOT NULL,
    email VARCHAR(255),
    UNIQUE(provider, provider_user_id)
);

-- Some historical consumers used the auth_identities spelling. Preserve those
-- mappings without overwriting an already linked identity or dropping the old
-- table. A conflicting owner fails closed for operator resolution.
DO $$
BEGIN
    IF to_regclass('{{.Schema}}.auth_identities') IS NOT NULL THEN
        IF EXISTS (
            SELECT 1 FROM {{.Schema}}.auth_identities old
            JOIN {{.Schema}}.auth_user_identities current
              ON current.provider=old.provider AND current.provider_user_id=old.provider_user_id
            WHERE current.user_id IS DISTINCT FROM old.user_id
        ) THEN
            RAISE EXCEPTION 'conflicting historical OAuth identity ownership';
        END IF;
        INSERT INTO {{.Schema}}.auth_user_identities(user_id,provider,provider_user_id,email)
        SELECT user_id,provider,provider_user_id,email FROM {{.Schema}}.auth_identities
        ON CONFLICT(provider,provider_user_id) DO NOTHING;
    END IF;
END $$;
