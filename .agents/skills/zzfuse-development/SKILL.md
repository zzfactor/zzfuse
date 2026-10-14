---
name: zzfuse-development
description: Develop, maintain, or upgrade the zzfuse OpenTelemetry backend, including its legacy Trazr modules, tenant APIs, workers, frontend, and Python service. Use for this project rather than generic Collector or router work.
---

# zzfuse development

## Input

A scoped change, investigation, or dependency upgrade in the zzfuse repository. Locate the checkout from the active workspace; do not assume another Trazr or zzrouter project is the same codebase.

## Process

1. Read the checkout's `AGENTS.md`, inspect its current diff, and identify the modules and contracts affected. Preserve work already in progress. Use `docs/architecture.md` for extension points and `docs/development.md` for current tools and commands.
2. Keep the canonical zzfuse repository identity distinct from legacy Trazr imports, Collector names, environment variables, and binary paths. Change those compatibility surfaces only when the requested migration covers them.
3. For backend work, preserve extension ownership of `ServiceResourceManager`, exporter borrowing/readiness, schema-scoped API/event operations, and the separation of worker job statistics from batch totals. Keep worker creation/start explicit and drain accepted operations before provider shutdown. For schema and API changes, update DDL, ordered SQL projections/scanners, services, handlers, OpenAPI, and clients together. Numeric metric IDs are scoped to their metric table; typed lookups must retain the type. Verify full measurements/context through real readback, not only ingestion counts.
4. For upgrades, inspect every consuming module and the builder configuration. Use `docs/development.md` for supported versions and `docs/profiles.md` for profile storage/wire compatibility. Development-version metadata alone does not prove wire compatibility. Preserve required SDK/security pins during isolated Collector regeneration, then validate the generated output. Profile compaction must isolate each row's reachable dictionary without mutating input or losing semantic data; resolve all nonempty resource routes before writing. Preserve legacy protobuf bytes without decoding them using the current schema.
5. Run checks from module directories or the Makefile. Default Go vendor trees are local artifacts; use the repository's `-mod=readonly` checks. Keep external-service integration suites explicitly opt-in. `make profile-e2e` covers disposable real storage and restart migration; `make stack-e2e` covers the browser, SMTP capture, telemetry, NATS, real inference, and production workers. Both own their resources and require Docker. Use frontend lockfile installs and lint/typecheck/build; use Python direct pins plus the generated universal lock and service contract tests. Database changes require the extension/columnstore/vector SQL smoke in `build/database/`; never attach an existing PostgreSQL 17 volume to the PostgreSQL 18 image.
6. Update the maintained documentation when behavior changes. Consult `docs/reference/` for domain examples and `docs/archive/` only for historical decisions. Check current code before treating old completion reports or benchmarks as evidence.

For authentication changes, use the SMTP settings in `docs/configuration.md`; keep tokens out of responses/logs and preserve transactional token consumption/password update. Collector startup must create auth tables independently of optional account bootstrap. For open-source publication, audit the final source and preserve the original private history locally; never push that history to the fresh public snapshot repository.

## Output

State the resulting behavior, changed files or modules, exact validation run, and remaining material limits. For upgrades, include selected versions and any compatibility ceiling. For investigation, separate reproduced defects from unverified concerns. Record durable memory as pointers to maintained docs and this skill, without copying specifications or Git status.
