# Consolidated project history

Earlier plans, completion reports, reviews, and reference guides were mixed at the root. Maintained setup/architecture now have separate entry points. Originals remain in the [archive](archive/README.md), preserving earlier decisions and existing checkout work.

## Workers and content

Work compared a single all-signals worker with enhanced workers, shared lifecycle/statistics, and enriched-span/full-trace processing. Current implementations live in `embedding_worker*.go` and `processing_transformers.go`.

See [architecture comparison](archive/ARCHITECTURE_COMPARISON.md), [enhanced workers](archive/ENHANCED_ARCHITECTURE_SUMMARY.md), [dual-mode processing](archive/DUAL_MODE_IMPLEMENTATION_SUMMARY.md), and [trace design](archive/TRACE.md).

## Templates

Earlier plans proposed `pkg/template/`; implementation subsequently accumulated inside `pkg/repository/`. Database configuration, field extraction, categorization, and exemplars generated overlapping reviews and reports. Current code/tests take precedence.

See [original design](archive/template.md), [database migration](archive/DATABASE_DRIVEN_TEMPLATE_MIGRATION.md), [processing approach](archive/TEMPLATE_PROCESSING_DRY_APPROACH.md), and [organization proposal](archive/TEMPLATE_SYSTEM_ORGANIZATION.md).

## Storage, filtering, and streaming

Partition maintenance, routing/filtering, notifications, runs, and sessions were documented in separate iterations. Active contracts are DDL, repository interfaces, handlers, and OpenAPI.

See [batch design](archive/BATCH_EMBEDDING_DESIGN.md), [trigger proposal](archive/TRIGGER_ARCHITECTURE.md), [run events](archive/RUN_EVENTS_IMPLEMENTATION.md), [sessions](archive/SESSION_ID_IMPLEMENTATION.md), and [filter proposal](archive/FILTER_DSL_EXTENSION.md).

## Validation history

Reviews repeatedly revisited template functions, cleanup, performance, and mocks. Old benchmark percentages and production-readiness statements are historical observations, not current acceptance criteria.

See [refactoring](archive/REFACTORING_SUMMARY.md), [final review](archive/COMPREHENSIVE_FINAL_CODE_REVIEW.md), [benchmarks](archive/PERFORMANCE_BENCHMARK_RESULTS.md), and [old tasks](archive/TASKS.md). Current checks are in [development](development.md).

The orphaned [configuration API test](archive/config-api-test.go.txt) and disabled [legacy extension suite](archive/legacy-extension-integration-test.go.txt) are preserved as inactive fixtures. Supported integration suites remain with their modules and require explicit opt-in. One-off SQL helpers are under `scripts/sql/legacy/`; they are not automatically applied.
