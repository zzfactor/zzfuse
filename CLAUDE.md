# zzfuse contributor guidance

Follow [AGENTS.md](AGENTS.md) and the project [zzfuse development skill](.agents/skills/zzfuse-development/SKILL.md). Those are the canonical project instructions for every agent.

zzfuse is the project name and `zzfactor/zzfuse` is its intended repository. Go import paths, Collector component IDs, executable/directory names, and `TRAZR_*` environment variables retain their legacy Trazr identity until a coordinated compatibility migration.

## Maintained documentation

- [Architecture](docs/architecture.md): Collector composition, extension ownership, data flow, and source boundaries.
- [Development](docs/development.md): module navigation, compatible dependency versions, regeneration, and checks.
- [Configuration](docs/configuration.md): runtime environment and service configuration.
- [Embedding](docs/embedding.md): worker modes, templates, metadata, and statistics.
- [Documentation index](docs/README.md): maintained guides, domain references, and historical archives.

Historical completion reports under `docs/archive/` are context, not current specifications or validation evidence.

## Common commands

There is no root Go module or workspace. Use the Makefile or enter the relevant module before running Go commands.

```sh
make build
make go-check
make frontend-install
make frontend-dev
make frontend-check
make python-test
make docs-check

# Focused unit checks
(cd trazrprocessor && go test -mod=readonly -timeout 45s ./...)
(cd pkg/repository && go test -mod=readonly -timeout 45s -run TestDatabaseRoutingSchema .)

# Format only changed files
gofmt -w path/to/changed_file.go
```

## Backend contracts

- This is an OpenTelemetry Collector application; Encore commands and annotations do not apply.
- `TrazrExtension` owns `ServiceResourceManager`. Exporters discover `TrazrServiceProvider` through the Collector host, wait for readiness, and borrow its services. Preserve this lifecycle and factory injection.
- Routing uses `DatabaseRoutingSchema` with a `sync.Map` cache invalidated on configuration changes.
- Authorize the effective schema before queries, writes, ID lookups, cursors, WebSocket upgrade/preload, and subscriptions. Populate event schema filters. Existing auth policy grants admin users and users with an empty allowed-schema list access to any schema.
- REST routes are registered under `/api/v1/` in `trazrextension/extension.go` and `internal/*_routes.go`. Typed signal GET routes exist; arbitrary SQL queries and vector search have no active backend route.
- Worker internal metadata lives under `metadata.internal`. Keep job outcomes separate from batch counts, signal totals, and token totals.
- Keep Collector/core/contrib versions, builder configuration, and generated output compatible; the current profiles boundary is documented in the development guide.

## External integration checks

Default unit suites must not contact real infrastructure. Auth database suites require `-tags integration` and a dedicated `TEST_PG_DSN`. NATS tests require `TRAZR_IT=1` and `NATS_URL`. Extension tests require `TRAZR_IT=1`, `TEST_API_BASE_URL`, and explicit `TEST_USERNAME`/`TEST_PASSWORD` or `TEST_JWT`, depending on the suite. MCP tests use `TRAZR_IT=1`, `EXTENSION_URL`, and `MCP_API_KEY`.

Do not load credentials from committed files or print tokens in test logs. Local credentials and backups belong under ignored `.local/`.
