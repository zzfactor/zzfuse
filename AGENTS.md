# zzfuse development

zzfuse is the canonical project name; the intended GitHub repository is `zzfactor/zzfuse`. Existing Trazr Go imports, component names, environment variables, and binaries are compatibility surfaces.

Use the project skill at [.agents/skills/zzfuse-development/SKILL.md](.agents/skills/zzfuse-development/SKILL.md) for development, dependency upgrades, and validation. Read only the maintained docs relevant to the task; historical reports under `docs/archive/` are not current specifications or test evidence.

## Source boundaries

- Collector composition: `builder-config.yaml`; generated output: `trazr-otel/`.
- HTTP/WebSocket handlers: `trazrextension/`; services, schema routing, workers, and DDL: `pkg/repository/`.
- OTLP writes: `trazrexporter/`; processing: `trazrprocessor/`.
- Authentication: `pkg/auth/`; shared defaults: `pkg/config/`.
- Frontend: `frontend/`; Python embedding/Chroma: `services/`; MCP: `trazr-mcp/`.

`TrazrExtension` owns the shared service manager. Exporters borrow it through the Collector host and readiness interface. Authorize a requested schema before HTTP operations, WebSocket upgrade/preload, or subscriptions; carry that schema into event filters. Empty schema grants currently mean unrestricted access.

## Checks and documentation

There is no root Go module/workspace. Use `make go-check` or module-local commands, with the Go version declared in the manifests. Frontend checks: `make frontend-check`. Python checks: `make python-test`. Link checks: `make docs-check`.

External-service integration tests require explicit configuration and opt-in. Default unit suites must not need private hosts, credentials, model downloads, or a production database. Preserve existing user edits and resource ownership when refactoring.

For public publication, retain the original private history locally and use a fresh audited source snapshot. Never push the original private history or ignored `.local/` credentials/backups to the public repository.

Current guides: [architecture](docs/architecture.md), [development](docs/development.md), [configuration](docs/configuration.md), and [embedding](docs/embedding.md). New plans belong in `docs/plans/`; completed historical notes belong in `docs/archive/`. Local credentials and backups stay under ignored `.local/`.
