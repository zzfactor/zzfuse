# zzfuse

zzfuse is an OpenTelemetry backend for logs, spans, metrics, profiles, and correlated runs. It combines PostgreSQL storage, schema routing, embedding workers, REST and WebSocket APIs, an MCP server, and a Next.js frontend.

The repository is [zzfactor/zzfuse](https://github.com/zzfactor/zzfuse). Go module paths, Collector component names, environment variables, and binaries retain their legacy Trazr identifiers for compatibility.

## Build from source

Use Go 1.27.2 or newer, Node.js 24, Python 3.11, and uv for the embedding service. This is a multi-module Go repository; checks run through the Makefile or from each module directory.

```sh
make build
make frontend-install
make frontend-check
make python-install
make python-test
```

With PostgreSQL and the TimescaleDB extension configured, set the ingestion token/signing key and a writable queue directory as described in the [configuration guide](docs/configuration.md), then run the Collector:

```sh
./trazr-otel/trazr-otel --feature-gates=+service.profilesSupport --config config.yaml
```

Startup creates authentication tables and configured database schemas/tables. For the complete local stack, copy `.env.example` to `.env`, supply the required secrets, and run `docker compose up --build`. Follow the [configuration guide](docs/configuration.md), including the PostgreSQL major-version migration requirements.

## Development

```sh
make go-check
make docs-check
```

See [development](docs/development.md) for unit tests, race checks, integration setup, and Collector regeneration. The frontend runs with `make frontend-dev`.

`make profile-e2e` checks real profile storage and migration. `make stack-e2e` builds an isolated stack and checks browser authentication, SMTP reset, all telemetry signals, NATS events, actual MiniLM inference, production worker/vector persistence, tenant isolation, session revocation, and queue recovery after a forced crash. Both require Docker and clean up their disposable resources.

SQL and vector search require explicitly configured backend endpoints: the Collector does not register their legacy routes. Password reset requires the SMTP configuration described in the [configuration guide](docs/configuration.md).

## Reference

- [Documentation index](docs/README.md)
- [Architecture](docs/architecture.md) and [configuration](docs/configuration.md)
- [Embedding and templates](docs/embedding.md)
- [Contributing](CONTRIBUTING.md) and [security policy](SECURITY.md)
- [Changelog](CHANGELOG.md)
- [Consolidated history](docs/history.md)

Licensed under [Apache 2.0](LICENSE). Existing third-party copyright and license notices remain in their source files.
