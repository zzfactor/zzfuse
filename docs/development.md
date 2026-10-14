# Development

Use Go 1.27.2 or newer, Node.js 24, Python 3.11, and uv. There is no root `go.mod` or `go.work`; run Go checks inside the twelve modules or through the Makefile.

```sh
make go-check
make frontend-install
make frontend-check
make python-install
make python-test
make docs-check
```

For focused checks:

```sh
cd pkg/repository
go test -mod=readonly -run TestDatabaseRoutingSchema -count=1 .
go test -race -mod=readonly -run TestConcurrent -count=1 .
```

External-service suites require explicit integration tags and environment configuration. Inspect each suite before running it: auth storage uses PostgreSQL, event integration uses NATS, and extension/MCP integration calls a running backend. Default unit checks should not require private hosts or credentials.

Python API tests replace model inference and Chroma connections with fixtures. They verify auth, validation, response shape, cache separation, and lifecycle without weights or a live Chroma server.

## Collector and dependency upgrades

Collector core and contrib components use 0.163.0; stable Collector modules use 1.69.0. OpenTelemetry Go SDK and API use 1.47.0. These versions include the profile storage/API migration described in [profiles](profiles.md).

```sh
make install-ocb
make generate
make build
```

`trazr-otel/` is generated. Update builder composition, local replacements, and versioned SDK/security overrides first. Test regeneration in isolation: a manually patched generated manifest building successfully does not prove reproducibility.

`make install-ocb` installs builder 0.163.0 into the project without replacing a global builder. `make generate` uses Go 1.27.2. The SDK overrides keep regeneration from downgrading OTLP, stdout, Prometheus, and log SDK packages to the Collector's transitive defaults.

## Profile integration checks

```sh
make profile-e2e
```

This opt-in check requires Docker and a TimescaleDB PostgreSQL 18 image. It creates a unique disposable container with a random test password and no host volumes, starts the built Collector on localhost ports, sends the checked-in OTLP profile fixture, and checks authenticated queries against real storage. It verifies profile dictionaries, IDs, exact unsigned time/duration, samples, resource/scope context, malformed version rejection, legacy bytes, and restart migration. The process and owned container are stopped automatically. Set a different image or binary with `python3 scripts/test-profile-e2e.py --help`.

The E2E uses the sending queue; `--disable-queue` exercises synchronous export. Repository integration tests also exercise the additive DDL against a pre-migration table:

```sh
cd pkg/repository
TRAZR_IT=1 TEST_PROFILE_MIGRATION_DSN="$DISPOSABLE_TEST_DSN" \
  go test -mod=readonly -tags integration \
  -run '^TestProfileStorageMigrationAndRepositoryRoundTrip$' -count=1 .
```

Supply only a disposable TimescaleDB database. The test creates and removes its own schema. These checks do not cover embedding inference or the complete frontend workflow.

## Full-stack checks

```sh
make stack-e2e
```

This builds all project images, downloads the public MiniLM model and pinned Playwright Chromium, and runs a real browser against the production frontend. The harness owns a unique Docker network, randomly published localhost ports, generated test secrets, and a tmpfs database. It verifies signup/login, private reset email and token replay rejection, logs/traces/all metric types/profiles, NATS events, actual model inference, production manager-driven embedding jobs, vector persistence, and nearest-neighbor lookup. SMTP is captured locally with Mailpit; no external email is sent. It cleans up only its own resources and stores redacted diagnostics under ignored `.local/e2e-output/`.

Run `python3 scripts/test-stack-e2e.py --help` to select prebuilt images. The manually dispatched GitHub CI option `full_stack_e2e` runs this suite; ordinary CI runs unit, manifest, frontend, Python contract, and secret checks. Run `sh build/database/smoke-test.sh` for focused extension/columnstore/index validation. See [database images](../build/database/README.md) for ARM64 and AMD64 checks and PostgreSQL major-upgrade requirements.

Password reset also has an opt-in real PostgreSQL/SMTP test in `pkg/auth`:

```sh
TRAZR_IT=1 TEST_AUTH_RESET_DSN="$DISPOSABLE_TEST_DSN" \
  go test -mod=readonly -tags integration \
  -run '^TestPasswordResetSMTPAndAtomicPostgres$' -count=1 ./...
```

Supply only a disposable database. This test checks hashed storage, concurrent one-time consumption, rollback on password update failure, and failed-delivery revocation using a local SMTP capture.

The metric storage regression in `pkg/repository` uses `TRAZR_IT=1` and `TEST_METRIC_READBACK_DSN` with `-tags integration -run '^TestMetricStorageAndQueryRoundTrip$'`. It provisions current DDL and checks all five types through insert, list, typed by-ID lookup with colliding IDs, and cursor pages. Supply a disposable TimescaleDB database. These focused checks complement the complete OTLP/browser/worker flow above.

Python direct pins live in `services/requirements.in`; its header gives the universal-lock command. Frontend updates must keep the lockfile and pass `npm ci`, lint, typecheck, and build. Ignored vendor trees are local artifacts; repository checks use `-mod=readonly` so stale vendor contents cannot hide updates.

Dependabot's frontend version updates allow minor and patch releases. Major table/tooling migrations require separate compatibility checks: its initial TypeScript 7 proposal failed the current Next.js lint tooling. Node types remain aligned with the Node 24 runtime. Python version PRs are disabled because the standard updater removed the universal lock's platform and Python-version conditions; update direct pins and regenerate with the documented `uv` command instead. Security suggestions still require supported-platform validation and proper lock regeneration.

Format changed source and test changed behavior. Pull requests should report exact validation and integration limits.
