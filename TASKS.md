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

## Future work

- [ ] Enforce database-backed JWT session revocation across Collector HTTP/WebSocket routes, including password reset and logout.
- [ ] Apply on-demand worker time bounds and requested batch sizes to signal fetches.

See [architecture](docs/architecture.md) and [development](docs/development.md).
