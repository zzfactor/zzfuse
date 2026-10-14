# Schema workers

## Current durable processing contract

Production SQL workers acquire per-tenant leases in `embedding_worker_claims`,
keyed by physical signal table, processor mapping, immutable configuration
fingerprint, and signal ID. The fingerprint includes mapping, embedding and
vector provider configuration, and configured content/metadata templates.
Receipts include this version, so changed configuration can be processed again.
Pre-version receipts cause one new embedding pass for the current configuration.

Acquisition commits before inference. A heartbeat renews live claims, cancels
inference on lease loss, and drains before worker shutdown. Expired leases can
be acquired by another process; stale attempts cannot renew, complete receipts,
or write fenced vectors. Trigger notifications are best-effort latency hints:
periodic SQL polling also recovers missed events and expired work.

Local PGVector inserts and their recoverable claim references commit together.
Retries after a crash between vector insertion and receipt completion reuse
that vector, including grouped document members, without repeating inference.
Receipt completion is fenced and transactional. A retained vector reference is
verified before recovery; if vector retention removed it, the worker embeds
again. Polling removes up to 512 expired orphan mappings per pass after source
retention. Completed mappings otherwise remain with their source signals.

On-demand bounds apply to physical `timestamp_ns` columns as `[start, stop)`.
The requested positive batch size overrides the vector configuration/default
of 100, with a maximum of 10,000. Trace/metric aliases map to `spans` and the
five physical metric tables. Invalid bounds or batch sizes fail before reload.

Durable workers currently require local PGVector using the same SQL database
pool as telemetry. Remote PGVector and external vector providers are rejected
at worker configuration validation. `template_plus` span workers are also
rejected because their additional full-trace effects need a separate durable
protocol. Batch/trigger/hybrid scheduling, ordinary configured templates,
grouping, and logs/spans/metric/profile signal workers remain supported. Direct
vector APIs retain their existing provider behavior. This protocol does not
promise exactly-once external inference or nontransactional remote effects.

## Lifecycle and validation

Schema startup initializes resources without starting jobs. Explicit schema,
all-worker, and on-demand start APIs build configured workers; reload retains
a running manager state. Configured templates compile before replacement, and
generic metadata joins per-signal metadata so original content and context are
fetched and retained. Unsupported providers/modes and invalid templates fail
configuration validation before workers are replaced.

Status snapshots report actual process-local job outcomes and signal/token
counters. They are copied values, not durable global processing counts. Stop
rejects new operations, drains accepted calls and heartbeat sessions, stops
workers, and then closes vector/embedding providers.

Run unit/race tests from `pkg/repository` with `-mod=readonly`. Durable claim
integration tests are opt-in: set `TRAZR_IT=1` and `TEST_WORKER_CLAIMS_DSN` to an
owned disposable PostgreSQL database, then run `go test -race -tags=integration
-mod=readonly -run '^TestWorkerClaims' .`. They cover parallel acquisition, all
signal-table bounds, expiry/fencing, grouped receipt retry, cancellation,
source-row writability during inference, and orphan cleanup. Never use a host
or production database for these tests.

See [embedding](../../docs/embedding.md), [architecture](../../docs/architecture.md),
and [development checks](../../docs/development.md) for project-wide contracts.
