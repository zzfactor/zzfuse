-- The upstream entrypoint runs this only for an empty PGDATA, in POSTGRES_DB.
-- Application schemas and tenant provisioning belong to the Collector.
CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS vectorscale;
