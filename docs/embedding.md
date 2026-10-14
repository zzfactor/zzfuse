# Embedding and templates

Scheduling and content processing are separate settings. Batch, trigger, and hybrid determine when workers run; transformations select signal, enriched-span, or full-trace content.

Schema configuration connects storage to vector destinations and embedding providers. Database-backed templates feed the template service/registry. Field extraction maps referenced template fields to columns so workers select needed content.

Use the [template guide](reference/templates.md), [fields](reference/template-fields.md), and [setup](reference/template-setup.md). Verify examples against `pkg/repository/template/`, `template_service.go`, `template_field_extractor.go`, and `embedding_worker*.go`.

Python provides `/sentencer/embed` and model-information endpoints. Models are cached and concurrent requests are limited. Vector dimensions must match destination schemas; keep model changes separate from library upgrades.

Lifecycle statistics count jobs when they start/finish. Batch statistics measure processed signal batches within jobs. Overlapping jobs must report batches with `RecordJobBatchStatistics(jobID, ...)` so completion summaries can be reconciled per job. The generic batch reporter infers a job only when exactly one is active; otherwise its independent totals must not also appear in job summaries. Resetting statistics establishes a new baseline for still-active jobs. Production embedding batches also use their separate metrics collector.

`ServiceResourceManager.GetWorkerStatus` exposes typed `workers` snapshots from the production metrics collector. Counters are cumulative, completion is idempotent, and `active_jobs` counts current attempts. Batch execution is serialized per production worker; fetch, vector-write, and receipt failures propagate into failed-job metrics. Partial failures retain counts for earlier successful work. Live worker reload preserves whether the manager was running.

Schema startup creates the service worker manager while leaving jobs stopped. Schema/all-worker start APIs load configured workers lazily; on-demand execution reloads signal filters before starting. Configured templates are loaded and compiled before replacement; body and generic context fields feed the fetch/render plan. Missing or invalid templates fail reload while retaining existing workers. Worker lifecycle operations are serialized separately from schema pool access. Shutdown rejects new operations, drains accepted operations, stops workers, and then closes their providers.

Current on-demand time bounds and requested batch sizes are not applied to worker fetches. The factory uses a batch size of 100. This limitation is tracked in [maintenance tasks](../TASKS.md).

pgvector writes and exact-content lookup bind the raw UTF-8 SHA256 hash computed in Go, preserving compatibility with existing hashes while avoiding the legacy SQL helper. Bound vector parameters retain full float32 precision. The focused integration `TestPGVectorCurrentDDLWriteAndBaseline` runs with `TRAZR_IT=1`, `TEST_PGVECTOR_WRITE_DSN`, and `-tags integration` against a disposable project database.

The [full-stack check](development.md#full-stack-checks) uses real MiniLM inference and the production service manager, then verifies vectors and nearest-neighbor lookup in PostgreSQL. It does not bypass the workers with mocked model responses.

Earlier independent-template-package and full-trace proposals remain in [history](history.md).
