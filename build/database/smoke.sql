\set ON_ERROR_STOP on

-- Run only in the disposable database created by smoke-test.sh.
DO $$
BEGIN
    IF current_setting('server_version_num') <> '180006' THEN
        RAISE EXCEPTION 'Expected PostgreSQL 18.6';
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'timescaledb' AND extversion = '2.30.2')
        OR NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector' AND extversion = '0.8.7')
        OR NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vectorscale' AND extversion = '0.9.1') THEN
        RAISE EXCEPTION 'Database extension versions do not match versions.json';
    END IF;
END $$;

CREATE SCHEMA zzfuse_database_smoke;
CREATE FUNCTION zzfuse_database_smoke.integer_now() RETURNS bigint
    LANGUAGE SQL STABLE AS 'SELECT 10000::bigint';
CREATE TABLE zzfuse_database_smoke.events (
    time_ns bigint NOT NULL,
    service_name text NOT NULL,
    value integer NOT NULL
);
SELECT create_hypertable('zzfuse_database_smoke.events', 'time_ns', chunk_time_interval => 1000);
SELECT set_integer_now_func('zzfuse_database_smoke.events', 'zzfuse_database_smoke.integer_now');
INSERT INTO zzfuse_database_smoke.events
    SELECT i, 'smoke', i FROM generate_series(1, 2000) AS i;
ALTER TABLE zzfuse_database_smoke.events SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'service_name',
    timescaledb.orderby = 'time_ns DESC'
);
CALL add_columnstore_policy('zzfuse_database_smoke.events', after => 1000::bigint);
DO $$
DECLARE
    chunk regclass;
BEGIN
    SELECT show_chunks('zzfuse_database_smoke.events') INTO chunk LIMIT 1;
    CALL convert_to_columnstore(chunk);
    IF (SELECT sum(value) FROM zzfuse_database_smoke.events) <> 2001000 THEN
        RAISE EXCEPTION 'Columnstore conversion changed stored data';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM timescaledb_information.chunks
        WHERE hypertable_schema = 'zzfuse_database_smoke' AND is_compressed
    ) THEN
        RAISE EXCEPTION 'Columnstore conversion did not compress a chunk';
    END IF;
END $$;

CREATE TABLE zzfuse_database_smoke.hnsw_vectors (id integer PRIMARY KEY, embedding vector(3));
INSERT INTO zzfuse_database_smoke.hnsw_vectors
    SELECT i, ARRAY[i, i + 1, i + 2]::vector FROM generate_series(1, 100) AS i;
CREATE INDEX ON zzfuse_database_smoke.hnsw_vectors USING hnsw (embedding vector_cosine_ops);
CREATE TABLE zzfuse_database_smoke.diskann_vectors (LIKE zzfuse_database_smoke.hnsw_vectors INCLUDING ALL);
INSERT INTO zzfuse_database_smoke.diskann_vectors SELECT * FROM zzfuse_database_smoke.hnsw_vectors;
-- LIKE INCLUDING ALL copied the HNSW index, so drop it before exercising DiskANN.
DROP INDEX zzfuse_database_smoke.diskann_vectors_embedding_idx;
CREATE INDEX ON zzfuse_database_smoke.diskann_vectors USING diskann (embedding vector_cosine_ops);
SET enable_seqscan = off;
EXPLAIN (COSTS OFF) SELECT id FROM zzfuse_database_smoke.hnsw_vectors ORDER BY embedding <=> '[1,2,3]' LIMIT 1;
EXPLAIN (COSTS OFF) SELECT id FROM zzfuse_database_smoke.diskann_vectors ORDER BY embedding <=> '[1,2,3]' LIMIT 1;
DO $$
BEGIN
    IF (SELECT id FROM zzfuse_database_smoke.hnsw_vectors ORDER BY embedding <=> '[1,2,3]' LIMIT 1) <> 1 THEN
        RAISE EXCEPTION 'HNSW nearest-neighbor query returned the wrong row';
    END IF;
    IF (SELECT id FROM zzfuse_database_smoke.diskann_vectors ORDER BY embedding <=> '[1,2,3]' LIMIT 1) <> 1 THEN
        RAISE EXCEPTION 'DiskANN nearest-neighbor query returned the wrong row';
    END IF;
END $$;
DROP SCHEMA zzfuse_database_smoke CASCADE;
SELECT 'Database SQL smoke tests passed' AS result;
