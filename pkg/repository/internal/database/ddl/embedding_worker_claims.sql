-- Worker leases are ordinary rows, not telemetry hypertables. No transaction
-- holding these rows remains open during inference. Vector references survive
-- failed attempts so retries can finish receipts without duplicating vectors.
CREATE TABLE IF NOT EXISTS {{.Schema}}.embedding_worker_claims (
    signal_table TEXT NOT NULL,
    processor_id BIGINT NOT NULL,
    config_version TEXT NOT NULL,
    signal_id TEXT NOT NULL,
    lease_token TEXT NOT NULL,
    leased_until TIMESTAMPTZ NOT NULL,
    completed BOOLEAN NOT NULL DEFAULT FALSE,
    vector_schema TEXT,
    vector_table TEXT,
    vector_id BIGINT,
    PRIMARY KEY (signal_table, processor_id, config_version, signal_id)
);
CREATE INDEX IF NOT EXISTS embedding_worker_claims_expiry
    ON {{.Schema}}.embedding_worker_claims (signal_table, processor_id, config_version, leased_until);
