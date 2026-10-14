# Configuration

The Collector reads `config.yaml` or the file passed with `--config`. Build composition is separate in `builder-config.yaml`. Runtime identifiers retain their legacy `trazr` names.

## Local services

Compose provides PostgreSQL, the Python service, NATS, the Collector, and the frontend. Copy [.env.example](../.env.example) to `.env` and supply separate random database, Python API, JWT, and frontend secrets before running `docker compose up --build`. Database startup uses a persistent development volume; Collector startup creates authentication tables and configured signal schemas/tables. Optional user bootstrap is disabled by default. Use disposable databases for integration tests.

The first registered account receives the admin role. Set up the initial account before exposing registration to users.

The project database image pins PostgreSQL 18.6, TimescaleDB 2.30.2, pgvector 0.8.7, and pgvectorscale 0.9.1. Signal tables require an enabled TimescaleDB extension. Schema initialization creates the nanosecond clock used by integer-indexed retention and columnstore policies before creating tables. See the [database image guide](../build/database/README.md) for versions and SQL checks.

Compose uses a new `db_pg18` volume. Existing PostgreSQL 17 data requires a tested backup/restore or major-upgrade procedure into a new PostgreSQL 18 data directory, including the extension upgrade steps. Keep the original database and backup until verification; switching image tags does not migrate data.

Repository settings include `TRAZR_DB_HOST`, `TRAZR_DB_PORT`, `TRAZR_DB_USER`, `TRAZR_DB_PASSWORD`, `TRAZR_DB_NAME`, and `TRAZR_DB_SSL_MODE`. Check `pkg/repository/database_util.go` for the selected DSN path and pooling. Authentication and integration helpers may use different DSN variables.

Typical ports are OTLP gRPC 4317, OTLP HTTP 4318, extension 18888, Python service 8000, and frontend 3000. Compose publishes service ports only on localhost. Set `extensions.trazr.endpoint`, for example `127.0.0.1:18888`, to configure the HTTP/WebSocket listener. OTLP endpoints are configured under `receivers.otlp.protocols`.

## Profiles

The maintained Collector configs include a profiles pipeline. Enable the experimental Collector gate with `--feature-gates=+service.profilesSupport`; the run script and Docker entrypoints already supply it. Profiles use the OTLP HTTP route `/v1development/profiles`, with `Content-Type: application/x-protobuf` and `OTLP-Profiles-Development-Version: 1`, or the experimental OTLP profiles gRPC service. Producers must use the profile schema supported by Collector 0.163.0.

The profiles pipeline goes directly from `otlp` to `trazr`. The batch processor and custom `trazr` processor do not support profiles. Collector telemetry metrics use `service.telemetry.metrics.readers`; the removed `address` setting is no longer supported.

Event bus settings come from environment variables. To use NATS, set `EVENT_BUS_TYPE=nats` and `NATS_URL`; `extensions.trazr.event_bus` is not a supported YAML field.

## Credentials and clients

Use the [auth guide](../pkg/auth/README.md) for JWT/auth settings. The Python service requires configured `USER_API_KEY` and `ADMIN_API_KEY`; no built-in key grants access. Compose forwards those variables.

### Password reset delivery

Set `TRAZR_SMTP_HOST`, `TRAZR_SMTP_FROM`, and `TRAZR_PASSWORD_RESET_URL` to enable reset email. The reset URL is a configured absolute HTTPS frontend URL such as `https://app.example.com/reset-password`; localhost HTTP is allowed for development. Request-supplied redirects are ignored. Configure `TRAZR_SMTP_USERNAME` and `TRAZR_SMTP_PASSWORD` when the SMTP server requires authentication.

`TRAZR_SMTP_PORT` defaults to 587, `TRAZR_SMTP_TIMEOUT` to `10s`, and `TRAZR_SMTP_TLS_MODE` to `starttls`. `starttls` requires a successful TLS upgrade; `tls` uses implicit TLS. Both verify server certificates. Set `TRAZR_SMTP_CA_FILE` to a mounted PEM CA file for a private certificate authority. `plain` is allowed only for loopback development capture servers. Missing or invalid delivery configuration leaves reset requests unavailable with HTTP 503.

Configured reset requests return the same generic response for existing and missing accounts and for delivery failures. A bounded background dispatcher performs account lookup and SMTP delivery after the response. Reset links carry the token in a URL fragment consumed by the frontend, keeping it out of HTTP request URLs. Reset pages use `no-referrer` and `no-store`; tokens are never returned by the request API or logged. Database storage contains only a token hash, expiring after one hour. Failed delivery revokes the issued token. Password update and one-time token consumption occur in one transaction, so concurrent replay cannot succeed twice and an unsuccessful update rolls back consumption.

Successful reset signs out the current frontend session. Previously issued backend JWTs remain valid until their configured expiry. The auth library exposes optional JTI denylist helpers; Collector route middleware currently validates JWT signatures, claims, and expiry without consulting that denylist. Database-enforced session revocation is tracked in [maintenance tasks](../TASKS.md).

The full-stack test uses a local Mailpit capture server and verifies the actual reset link in Chromium. Production SMTP settings remain deployment-specific; tests do not contact external email recipients.

Local credentials and the original private Git history are retained under ignored `.local/` paths. Public releases use a fresh audited source snapshot. Do not publish the original history or include local credentials in examples.

Use [frontend/.env.example](../frontend/.env.example) for frontend settings. Proxies forward Bearer authentication. SQL/vector endpoint paths are opt-in because the old `/api/signals/query` and `/search` handlers are absent.

MCP uses `EXTENSION_URL`, `AUTH_BASE_URL`, and `MCP_TRANSPORT`. Adapt the [sanitized client example](../trazr-mcp/claude-desktop-config.example.json). Python settings are in [services/.env.example](../services/.env.example). Changing embedding models or dimensions may require vector-store migration and re-embedding.
