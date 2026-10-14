-- Profile storage v1: additive and idempotent. Existing rows retain version 0
-- and their exact original bytes; there is no implicit development-wire upgrade.
ALTER TABLE {{.Schema}}.profiles
    ADD COLUMN IF NOT EXISTS service_namespace TEXT,
    ADD COLUMN IF NOT EXISTS service_instance_id TEXT,
    ADD COLUMN IF NOT EXISTS profile_storage_version SMALLINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS profile_encoding TEXT NOT NULL DEFAULT 'legacy-unversioned',
    ADD COLUMN IF NOT EXISTS profile_schema_version TEXT,
    ADD COLUMN IF NOT EXISTS otel_profile_id TEXT,
    ADD COLUMN IF NOT EXISTS profile_time_unix_nano TEXT,
    ADD COLUMN IF NOT EXISTS duration_nano TEXT;

COMMENT ON COLUMN {{.Schema}}.profiles.profile_schema_version IS 'OTLP development wire metadata version for explicitly versioned rows; legacy rows have no asserted version';
COMMENT ON COLUMN {{.Schema}}.profiles.profile_time_unix_nano IS 'Exact unsigned source nanoseconds as decimal; timestamp_ns indexes ingestion time when source time is zero or exceeds signed bigint';
COMMENT ON COLUMN {{.Schema}}.profiles.duration_nano IS 'Exact unsigned duration as decimal; legacy duration summary is zero when source duration exceeds signed bigint';
