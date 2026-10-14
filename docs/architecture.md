# Architecture

zzfuse retains legacy Trazr module and runtime identifiers. Repository naming is separate from Go imports, Collector component IDs, environment variables, and persisted schema names.

## Modules

- `trazr-otel/`: generated Collector; composition comes from `builder-config.yaml`.
- `trazrprocessor/`: signal processing, correlation metadata, and sensitive-data handling.
- `trazrexporter/`: authenticated OTLP conversion and repository writes.
- `zzfuseauth/`: Collector server authentication and signed tenant receipts for queue replay.
- `trazrextension/`: HTTP/WebSocket APIs and shared service ownership.
- `pkg/repository/`: storage, query/writer services, schema routing, workers, templates, and vector clients.
- `pkg/auth/` and `pkg/auth/client/`: identity, user context, keys, and client integration.
- `pkg/config/`: defaults shared by exporter and extension, using repository signal constants.
- `pkg/event/`, `pkg/metrics/`, and `pkg/util/`: delivery, instrumentation, and utilities.
- `trazr-mcp/`: stdio tools calling the extension.
- `frontend/`: Next.js application and authenticated proxies.
- `services/`: Python embedding and Chroma API.

## Ownership and tenant boundaries

`TrazrExtension` creates and shuts down `ServiceResourceManager`. Exporters borrow it through `Host.GetExtensions()` and `TrazrServiceProvider`, waiting for readiness. Exporter shutdown must not close extension-owned resources. Schema resources own worker managers and vector destinations; preserve ownership flags when changing cleanup.

OTLP signals flow through processors/exporters into PostgreSQL. Supported ingestion binds an opaque credential to a fixed schema using `zzfuse_auth`; producer service name/namespace remain telemetry labels. Exporters verify a signed receipt before enqueue and again at database delivery. The receipt survives the file-backed queue; raw ingestion tokens do not. Legacy service routing remains an internal repository compatibility mechanism. Workers read configured fields, generate content, obtain embeddings, and write vector destinations. Short PostgreSQL transactions acquire durable, versioned leases before inference; joined heartbeats renew them and cancel work after lease loss. PGVector writes validate the fencing token and persist vector references atomically, allowing retries to finish receipts without duplicating committed vectors. Time windows are half-open, with inclusive start and exclusive stop. SQL polling is authoritative; NATS notifications are best-effort hints, not durable jobs.

API schema selection is distinct from ingestion routing. Authorize the requested schema before queries, writes, preload, or subscriptions. Administrators can access all schemas; every other account requires an explicit matching grant. Empty grants deny access. Session validation reads the current account, roles and grants from the instance-owned auth database, checks account version and JTI revocation, and validates machine-key state. Legacy versionless JWTs require a new login. WebSocket subscriptions carry the authorized schema so event delivery remains isolated, and recheck session/access state every second with a five-second database timeout. Revocation or changed permissions closes the stream. Machine-key scopes restrict signal reads/writes and cannot manage accounts or create broader credentials.

Pagination uses nanosecond timestamps and entry IDs. Query direction and presentation order are separate parameters.

Numeric signal IDs belong to individual signal tables. Metric types have independent sequences, so a typed lookup must select its own table even when another metric type has the same ID. Keep physical SQL projections aligned with scanner order and preserve each type's measurements in API and event responses. Span storage uses the physical `duration` column; its API field remains `duration_ns`.

## Deployment limits

The supported durable worker path uses PGVector in the same PostgreSQL database as source claims. External stores and `template_plus` secondary trace effects require separate fencing protocols and fail clearly for durable workers. Ingestion retries are at least once: a crash after a database commit but before queue acknowledgement can still duplicate raw telemetry. Queue storage is local to one Collector and must not be shared concurrently by replicas. File storage protects process restarts; disk loss, capacity exhaustion and configured retry expiry remain loss scenarios.

The API, ingestion and worker orchestration still share a Collector process. Functional and concurrency tests establish the tested contracts; production capacity, failover and separate worker deployment require further workload-specific validation.

## Extension points

Add APIs in `trazrextension/internal/` and register them in extension/signal route files. Keep OpenAPI and frontend contracts aligned. Storage belongs in repository services/database clients; DDL lives under `pkg/repository/internal/database/ddl/`. Templates belong in `pkg/repository/template/` and existing template/field-extraction services. Add Collector components and replacements through `builder-config.yaml` before regenerating.
