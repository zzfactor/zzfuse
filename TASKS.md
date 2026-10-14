# Project maintenance

## Completed

- [x] Review architecture and validation commands.
- [x] Use zzrouter as repository/open-source reference.
- [x] Consolidate maintained docs and archive historical reports.
- [x] Preserve credentials/backups outside the public source tree.
- [x] Pin Python dependencies and test service contracts.
- [x] Finish upgrades and backend/frontend validation.
- [x] Complete tenant isolation and worker lifecycle regression tests.
- [x] Record project memory and reusable development skill.
- [x] Modernize Go and Collector with the profile storage/API migration.
- [x] Add a disposable-database exporter E2E using the extension-owned service manager.
- [x] Implement private SMTP password-reset delivery and atomic one-time consumption.
- [x] Add lossless profile dictionary compaction for mixed-schema ingestion.
- [x] Verify PostgreSQL 18 and current Timescale/vector extensions on ARM64 and AMD64.
- [x] Complete real browser/telemetry/NATS/model/production-worker full-stack validation.
- [x] Repair current-DDL span/metric readback, metric typed IDs, and vector content/hash/precision contracts.
- [x] Audit source/history and create the public zzfactor/zzfuse repository from a fresh snapshot.

- [x] Deny empty nonadministrator schema grants and validate current database-backed account/session/key state across HTTP and WebSockets.
- [x] Bind HTTP/gRPC ingestion credentials to fixed tenant schemas and retain authenticated identity through a persistent queue.
- [x] Add administrator grant/account/session controls, browser logout revocation, and last-administrator protection.
- [x] Coordinate durable worker claims, fenced PGVector/receipt commits, crash recovery, polling, time bounds and requested batch sizes.
- [x] Verify real two-tenant ingestion, API-key scopes, live-stream revocation and forced-crash WAL recovery.
- [x] Persist requested schema configuration fields and preserve explicit exponential-histogram destination schemas.

## Future work

- [ ] Implement separate durability protocols before enabling external vector stores or secondary `template_plus` trace effects in durable workers.
- [ ] Run workload-specific capacity and replica-failover tests; design separate worker/API deployment when resource isolation is required.
- [ ] Review the outstanding frontend dependency PR against the hardened full-stack baseline.
- [ ] Rotate any active credentials from retained legacy history in their owning services.

See [architecture](docs/architecture.md) and [development](docs/development.md).
