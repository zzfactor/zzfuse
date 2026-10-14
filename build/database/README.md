# zzfuse database image

Build from the repository root:

```sh
docker build -t zzfuse-database:local build/database
sh build/database/smoke-test.sh
```

The Dockerfile uses the official `timescale/timescaledb-ha:pg18.6-ts2.30.2`
image pinned by its multiarch manifest digest. It supplies PostgreSQL 18.6,
TimescaleDB 2.30.2, pgvector 0.8.7, and pgvectorscale 0.9.1 on Linux arm64
and amd64. `versions.json` records the selected versions and platform digests.
The image also includes upstream TimescaleDB Toolkit 1.26.0 and the other
extensions and administration tools shipped by the official HA distribution.
The project build performs no unpinned package downloads or source compilation.

The upstream entrypoint accepts `POSTGRES_USER`, `POSTGRES_PASSWORD`, and
`POSTGRES_DB`, including its supported `_FILE` secret variants. `PGDATA` remains
`/var/lib/postgresql/data` for the project's mount contract. The entrypoint starts
as root to create the data directory and assign ownership, then runs PostgreSQL
as `postgres`. On an empty data directory, upstream initialization enables
TimescaleDB and its toolkit, and `init/020_zzfuse_extensions.sql` enables `vector`
and `vectorscale` in `POSTGRES_DB`. The Collector creates application schemas and
tenant tables. Initialization scripts do not run for an existing database, and
they do not upgrade installed extension versions automatically.

The server preloads TimescaleDB and retains its Timescale license setting for
columnstore support. See [TimescaleDB licensing](https://github.com/timescale/timescaledb/blob/main/LICENSE).
`NO_TS_TUNE=1` keeps ordinary PostgreSQL memory defaults during initialization.
To opt into upstream tuning, set `NO_TS_TUNE` to an empty value and provide
container CPU/memory limits or the upstream `TS_TUNE_MEMORY`, `TS_TUNE_NUM_CPUS`,
and `TS_TUNE_MAX_CONNS` settings. Resource changes after initialization need
corresponding PostgreSQL configuration changes.

The SQL smoke test uses a unique container with a tmpfs database, no published
ports, no host mounts, and no persistent volumes. It checks exact runtime
versions, BIGINT hypertables and integer clocks, columnstore policy and chunk
conversion without data loss, actual HNSW and DiskANN searches, idempotent
extension initialization, and the unprivileged server process. It removes only
its own container. Pass an alternative image tag as its first argument.

## Compatibility and upgrades

PostgreSQL 18.6 is the latest stable release verified on October 14, 2026.
PostgreSQL 19 remains beta and is outside this image's extension support.
TimescaleDB 2.30.2 supports PostgreSQL 16 through 18, and pgvectorscale 0.9.1
supports PostgreSQL 14 through 18. The common current stable target is PostgreSQL
18. pgvector 0.8.7 supports this target.

An existing PostgreSQL 17 data directory cannot be opened by PostgreSQL 18.
Before using this image with existing data, migrate to a fresh PostgreSQL 18
data directory using a tested backup/restore or PostgreSQL major-upgrade
procedure, with the required TimescaleDB extension steps. Keep the original
database and backup until the migration is verified. This image does not perform
major-version migration, delete existing data, or change user volumes.

pgvectorscale 0.9.1 rejects DiskANN indexes built without a valid `vector(N)`
dimension. Follow its release notes when upgrading existing vector indexes.
The latest pgrx is 0.19.3, but pgvectorscale 0.9.1's build dependencies require
exact pgrx 0.16.1. The official binary image removes the need to install either
Rust or cargo-pgrx in this project's build.

For a version refresh, check the primary sources, select a newly published
official multiarch image, update the Dockerfile digest and `versions.json`,
update smoke test version assertions, and run both SQL smoke and full-stack E2E
validation. A mutable `latest` tag cannot replace the digest pin.

Primary sources:

- [PostgreSQL releases](https://www.postgresql.org/)
- [TimescaleDB 2.30.2](https://github.com/timescale/timescaledb/releases/tag/2.30.2)
- [Official image versions and compatibility](https://github.com/timescale/timescaledb-docker-ha/blob/master/build_scripts/versions.yaml)
- [Official HA image distribution](https://github.com/timescale/timescaledb-docker-ha)
- [pgvector 0.8.7](https://github.com/pgvector/pgvector/releases/tag/v0.8.7)
- [pgvectorscale 0.9.1 upgrade notes](https://github.com/timescale/pgvectorscale/releases/tag/0.9.1)
- [pgvectorscale pinned pgrx dependency](https://github.com/timescale/pgvectorscale/blob/0.9.1/pgvectorscale/Cargo.toml)
- [pgrx 0.19.3](https://github.com/pgcentralfoundation/pgrx/releases/tag/v0.19.3)
