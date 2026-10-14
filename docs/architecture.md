# Architecture

zzfuse retains legacy Trazr module and runtime identifiers. Repository naming is separate from Go imports, Collector component IDs, environment variables, and persisted schema names.

## Modules

- `trazr-otel/`: generated Collector; composition comes from `builder-config.yaml`.
- `trazrprocessor/`: signal processing, correlation metadata, and sensitive-data handling.
- `trazrexporter/`: OTLP conversion and repository writes.
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

OTLP signals flow through processors/exporters into PostgreSQL. Ingestion routing uses service name/namespace, priority-ordered configuration, and a `sync.Map` lookup cache. Workers read configured fields, generate content, obtain embeddings, and write vector destinations.

API schema selection is distinct from ingestion routing. Authorize the requested schema before queries, writes, preload, or subscriptions. Current policy gives administrators and users with an empty schema list unrestricted access; others require a matching schema. WebSocket subscriptions must carry the authorized schema so event delivery remains isolated.

Pagination uses nanosecond timestamps and entry IDs. Query direction and presentation order are separate parameters.

Numeric signal IDs belong to individual signal tables. Metric types have independent sequences, so a typed lookup must select its own table even when another metric type has the same ID. Keep physical SQL projections aligned with scanner order and preserve each type's measurements in API and event responses. Span storage uses the physical `duration` column; its API field remains `duration_ns`.

## Extension points

Add APIs in `trazrextension/internal/` and register them in extension/signal route files. Keep OpenAPI and frontend contracts aligned. Storage belongs in repository services/database clients; DDL lives under `pkg/repository/internal/database/ddl/`. Templates belong in `pkg/repository/template/` and existing template/field-extraction services. Add Collector components and replacements through `builder-config.yaml` before regenerating.
