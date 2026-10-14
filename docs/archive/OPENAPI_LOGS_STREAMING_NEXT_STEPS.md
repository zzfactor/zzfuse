## Logs and Streaming API — Proposed Next Steps

Brief plan to evolve GET /api/v1/logs and WS /api/v1/stream/logs to support richer time, text, attribute, severity, and streaming semantics; and to keep OpenAPI/doc/UI aligned.

### Completed Tasks

- [x] Implemented run streaming WebSocket (`trazrextension/internal/otel_runs.go`)
- [x] Fixed run event handling to accept single and batch events (`pkg/repository/run_stream.go`)
- [x] Log streaming working with server-side filtering (`trazrextension/internal/otel_stream.go`)

### In Progress Tasks

- [ ] Extend OpenAPI for logs query/stream to add time, text, attributes, severity, and preload/backpressure semantics

### Future Tasks

- [ ] Backend: implement new filters for GET /api/v1/logs
- [ ] Backend: align WS /api/v1/stream/logs with query filters; add preload + filter updates + backpressure notices
- [ ] DB: add/verify indexes (btree + GIN + pg_trgm) to keep queries performant
- [ ] Tests: unit, integration, and e2e streaming coverage for new behaviors
- [ ] Observability and limits (rate-limit, regex guards, window caps)
- [ ] Docs/UI: update OpenAPI in frontend and usage guides

## Implementation Plan

### 1) OpenAPI Spec Updates

- GET `/api/v1/logs`
  - Time: `start` (RFC3339), `end` (RFC3339), `last` (shorthand, e.g., 15m, 1h, 2d). Default last=1h if none provided.
  - Text: `q` (substring search, case-insensitive), `q_mode` (simple|regex, default simple), `q_flags` (i|m) for RE2.
  - Services: `service_in[]` (repeatable, OR within list).
  - Severity: `severity_min`, `severity_max` (1–24), `severity_group_in[]` (TRACE|DEBUG|INFO|WARN|ERROR|FATAL → mapped to numeric ranges), optional `severity_text_in[]` for precise variants (case-insensitive).
  - Attributes: deepObject params supporting
    - `attr.eq[key]=value` (AND across keys),
    - `attr.in[key]=v1,v2` (OR within key),
    - `attr.exists[key]=true|false`.
  - Resource attributes: mirror above as `res_attr.eq|in|exists`.
  - Pagination/others: `limit`, `cursor`, `schema` (default `otel`).
  - Semantics doc: OR within repeated params/IN lists; AND across different keys. `q` applied after coarse filters; push to DB indexes when available.

- WS `/api/v1/stream/logs`
  - Accept the SAME query parameters as GET `/api/v1/logs`.
  - Preload: if `last` or `start` provided, send historical window first (oldest→newest), then switch to realtime.
  - In-band filter update: client → `{"type":"set_filter","filter":{...}}`; server → `{"type":"ack"}` or `{"type":"error","message":"..."}`.
  - Backpressure: server may emit `{"type":"dropped","count":N}` when messages are dropped.
  - Optionally emit `{"type":"ready"}` after preload completes to mark realtime transition.

- Mirror these patterns to runs/traces/metrics later for consistency (not required for this phase).

### 2) Backend — Query (GET /api/v1/logs)

- Parse and validate new params in `trazrextension/internal/otel_query.go` (or relevant logs query handler).
- Extend repository layer (`pkg/repository/otel_logs_repository.go` and/or service) to build SQL:
  - Time window logic: default last=1h if missing; clamp to max window (e.g., 24h) to protect DB.
  - Service OR list, severity number ranges, group mapping → numeric ranges, severity_text exact match list.
  - Attributes/resource-attributes: equals, IN (split comma-separated), exists; AND across keys, OR within same key.
  - Text search:
    - simple: use ILIKE when pg_trgm unavailable; prefer `pg_trgm` + GIN when enabled.
    - regex: apply only after time/coarse filters; use RE2-compatible filtering at application layer or DB if available; enforce size/time guardrails.

### 3) Backend — Streaming (WS /api/v1/stream/logs)

- Accept same filters on connect; reuse repository filter structs.
- Preload: query last/start-end window, send results oldest→newest, then continue live stream.
- In-band filter update handling in `trazrextension/internal/otel_stream.go`:
  - Parse `set_filter` messages, validate, rebuild filter, ack/error.
  - Apply filter changes without tearing down the socket (restart subscription if needed).
- Backpressure: keep existing consecutive write error handling; add periodic `dropped` meta events when buffers overflow.
- Consistent close control frame with context-aware shutdown.

### 4) Database & Indexing

- Ensure/introduce indexes:
  - btree: `(timestamp)`, `(service_name)`, `(severity_number)`.
  - JSONB: GIN on attributes/resource attributes maps.
  - Text: `pg_trgm` (GIN) on rendered body/message column (or a generated column aggregating body + decoded fields).
- Provide migrations under `pkg/repository/internal/database/ddl/`.

### 5) Testing

- Unit tests: parameter parsing (time `last`, regex flags, attribute deepObject), severity mapping.
- Repository tests: SQL generation + result correctness under combinations of filters.
- Integration: seeded data to validate `last`, regex, attributes, severity groups.
- Streaming e2e: preload ordering, live continuation, in-band `set_filter`, backpressure `dropped` notices.
- Performance: benchmark typical queries ("last hour + text + attrs").

### 6) Observability, Limits, and Safety

- Add tracing spans to query and streaming paths; record filter attributes and result counts.
- Metrics: request rate, latency, error rates, dropped counts, backpressure activations.
- Guardrails:
  - Cap maximum `last` (e.g., 24h) and regex execution time/size.
  - Rate-limit in-band filter updates and max matches/sec per connection.

### 7) Docs & UI

- Update `trazrextension/openapi-merged.yaml` and mirror to `frontend/public/openapi-merged.yaml`.
- Update `STREAMING_CLIENT_GUIDE.md` with new examples (preload, set_filter, dropped notices).
- Add `README` snippets and curl examples showing common combinations.

### 8) Rollout

- Add feature flags for regex mode and preload if needed.
- Keep legacy `service`/`severity` parameters for compatibility; prefer documenting new params.
- Consider deprecation notes in a later version.

## Milestones

- M1 (1–2 days): OpenAPI updates, handler param parsing, severity mapping, service_in, last/start/end.
- M2 (2–3 days): Attributes/resource-attributes filters, text search (simple via ILIKE + optional pg_trgm), tests.
- M3 (2–4 days): Streaming preload + in-band filter updates + backpressure notices; e2e tests and docs.

### Relevant Files

- `trazrextension/openapi-merged.yaml` — API spec updates (mirror to `frontend/public/openapi-merged.yaml`)
- `trazrextension/internal/otel_query.go` — logs query handler (parameter parsing)
- `pkg/repository/otel_logs_repository.go` and `pkg/repository/otel_logs_service.go` — query building & execution
- `trazrextension/internal/otel_stream.go` — WebSocket logs streaming (preload, filter updates, backpressure)
- `pkg/repository/internal/database/ddl/` — migrations for indexes (btree/GIN/pg_trgm)
- `STREAMING_CLIENT_GUIDE.md` — client streaming guidance and examples

---

## Best Practices Compliance

- **Consistent filters**: Same model for query and stream; AND across keys, OR within repeated params or IN lists.
- **Time defaults**: Support `last` with default 1h; `start`/`end` optional.
- **Text search**: Default case-insensitive substring; optional RE2 regex with flags; applied after coarse filters.
- **Severity**: Numeric range and group mapping, with optional exact text list.
- **Attributes**: `attr.*` and `res_attr.*` with eq/in/exists; GIN indexes planned.
- **Streaming**: Preload window then realtime; in-band filter updates; backpressure meta events; graceful close.
- **Indexing**: BTREE on time/service/severity; GIN on JSONB; `pg_trgm` for text.
- **Testing/ops**: Plan includes unit/integration/E2E and observability.

## Explicit Limits & Validation

- **Time window cap**: Reject `last` exceeding max (e.g., 24h) or `end-start` over cap; 400 on violation.
- **Limit**: Enforce max `limit` (e.g., 1000) with default 100.
- **Severity**: Validate `1 ≤ severity_min ≤ severity_max ≤ 24`.
- **last format**: Regex `^[0-9]+[smhd]$`; 400 on invalid.
- **Regex mode**: Require `q` present when `q_mode=regex`; validate `q_flags ⊆ {i,m}`.
- **Param conflicts**: Define precedence if both `last` and `start` provided (prefer `start`/`end`, ignore `last`).
- **WS readiness**: Support `wait_for_established=1` query param.
- **Security**: Document and enforce bearer auth where required.

## WebSocket Control & Meta Schemas (Docs)

- **Set filter (client → server)**
  ```json
  {"type":"set_filter","filter":{"service_in":["api","worker"],"severity_min":17,"last":"1h"}}
  ```
- **Ack (server → client)**
  ```json
  {"type":"ack"}
  ```
- **Error (server → client)**
  ```json
  {"type":"error","message":"invalid last window"}
  ```
- **Dropped notice (server → client)**
  ```json
  {"type":"dropped","count":42}
  ```
- **Ready after preload (server → client) [optional]**
  ```json
  {"type":"ready"}
  ```

## Streaming Semantics

- **Preload ordering**: Historical messages sent oldest→newest, then switch to live.
- **Preload scope**: If `last` or `start` set, preload is enabled. `limit` applies to preload only (documented behavior).
- **Backpressure policy**: Drop oldest buffer entries when congested; emit periodic `dropped` notices; close on repeated write failures.
- **Consecutive write errors**: Close connection after N consecutive errors (current default: 3).

## Security & Rate Limiting

- **Auth**: Require bearer auth where needed; document scopes/roles if applicable.
- **Regex guardrails**: Set time/size limits; reject pathological patterns.
- **Update rate**: Rate-limit `set_filter` messages per connection.
- **Throughput**: Optionally cap matches/sec per connection.

## OpenAPI To-Dos

- [ ] Add new parameters and descriptions for GET `/api/v1/logs` and WS `/api/v1/stream/logs` as specified above.
- [ ] Define `WebSocketControlMessage` and `WebSocketMetaMessage` components (set_filter/ack/error/dropped/ready) for documentation.
- [ ] Mirror the spec to `frontend/public/openapi-merged.yaml`.
- [ ] Add usage examples for common queries and streaming scenarios. 