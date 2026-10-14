# Changelog

## Unreleased

- Require credential-bound HTTP/gRPC ingestion and preserve signed tenant identity in a persistent, fsynced exporter queue.
- Deny empty nonadministrator schema grants, refresh database-backed authorization, and revoke sessions on logout, password reset, account changes and API-key removal.
- Add administrator grant/account/session controls and close existing WebSockets after access changes.
- Coordinate workers with durable leases, fenced PGVector/receipt commits, crash recovery, SQL polling, time bounds and requested batch sizes.
- Reject durable external-vector and secondary `template_plus` effects until their protocols are implemented.

- Prepare zzfuse naming, documentation, contributor guidance, Apache 2.0 licensing, and CI for the intended zzfactor/zzfuse repository.
- Upgrade compatible Go, frontend, and Python dependencies with reproducible manifests and lockfiles.
- Modernize all Go modules to Go 1.27.2 and Collector 0.163.0 / stable modules 1.69.0, retaining OpenTelemetry SDK 1.47.0.
- Enable profile ingestion with versioned protobuf storage, exact unsigned time/duration, lossless dictionary compaction, credential-bound tenant routing, and byte-preserving legacy queries.
- Correct schema authorization and isolation across HTTP and WebSocket operations.
- Preserve verified roles, schema grants, and tenant identity when adapting JWT claims for admin and legacy handlers.
- Repair worker accounting, lifecycle behavior, metadata extraction, and template rendering.
- Initialize on-demand worker managers and drain accepted operations before closing schema providers.
- Load configured templates before worker replacement, preserving original signal content and context; bind compatible SHA256 hashes and full-precision vector parameters.
- Align span/metric readback with provisioned SQL, preserve metric measurements in API/events, and resolve by-ID lookups within the requested metric type.
- Remove built-in Python API-key access and public password-reset token issuance.
- Add SMTP password reset with verified TLS, private links, hashed token storage, and atomic one-time consumption.
- Pin multiarch database images to PostgreSQL 18.6, TimescaleDB 2.30.2, pgvector 0.8.7, and pgvectorscale 0.9.1; retain existing database volumes for explicit migration.
- Add source-built Collector, standalone frontend, nonroot Python images, and a complete local Compose stack.
- Add isolated browser/SMTP/telemetry/NATS/real-model/production-worker E2E and fresh-history secret checks in CI.
- Consolidate older reports and preserve local credentials/backups outside the public source tree.
