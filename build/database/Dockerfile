# Official multiarch binaries: PostgreSQL 18.6, TimescaleDB 2.30.2,
# pgvector 0.8.7 and pgvectorscale 0.9.1. See versions.json for provenance.
FROM timescale/timescaledb-ha:pg18.6-ts2.30.2@sha256:75b1a432956ae6efc31551413ba51b6601ec0910dbad123a64617face989dc7c

# Keep the project's data mount contract. The upstream entrypoint creates and
# owns this directory, then starts PostgreSQL as the postgres user.
USER root
ENV PGDATA=/var/lib/postgresql/data \
    NO_TS_TUNE=1

COPY init/020_zzfuse_extensions.sql /docker-entrypoint-initdb.d/

# Use ordinary PostgreSQL defaults unless a deployment explicitly enables
# timescaledb-tune with NO_TS_TUNE="" and container resource limits.
CMD ["postgres", "-c", "shared_preload_libraries=timescaledb", "-c", "timescaledb.license=timescale"]
