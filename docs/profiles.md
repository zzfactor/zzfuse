# Profiles

zzfuse uses the experimental OTLP profiles schema compiled into Collector 0.163.0. The `trazr` exporter factory exposes profiles alongside logs, metrics, and traces. See [configuration](configuration.md) for the feature gate and receiver route and [development](development.md) for validation commands.

## Storage migration

Schema initialization applies `pkg/repository/internal/database/ddl/migrations/profiles_storage_v1.sql` when ensuring each profile table. The migration adds version and exact-value columns without rewriting existing payloads. It is safe to reapply on startup. Take the usual database backup before upgrading an existing deployment.

Existing compressed chunks retain their compression layout. Existing retention policy settings and schedule are preserved. A newly installed profile retention policy keeps the 30-day default and starts its first run one day later, giving schema bootstrap time to finish without immediate historical cleanup. Once it runs, rows older than the configured retention window are eligible for deletion.

New rows have `profile_storage_version=1`, `profile_encoding=otlp-profiles-protobuf`, and `profile_schema_version` equal to the compiled OTLP development version, currently `1`. Each payload contains one profile, its resource and instrumentation scope, and the dictionary entries reachable from that profile and context. The converter copies pdata before normalization and marshaling because the current marshaler interns attributes into the dictionary and can mutate its input.

A shared dictionary can contain data belonging to multiple resources. `CompactProfiles` walks the complete reachable graph, retains required zero entries, and deterministically reindexes strings, attributes, stacks, locations, mappings, functions, and links. It preserves sample values, typed attributes, source identifiers, units, and resource/scope context while removing unused entries and unrelated resources. Dictionary indices may change; decoded meaning remains the same.

Mixed-schema requests are supported because each stored row receives an isolated dictionary. Resources with actual profiles must all resolve before any writes; an unrouted resource with profiles rejects the request. Empty resources do not contribute stored data. Entirely profile-empty packets are a no-op. REST inserts retain accepted raw bytes exactly; compaction applies to OTLP export conversion.

`otel_profile_id` retains the original 16-byte profile identifier as hex. `profile_time_unix_nano` and `duration_nano` retain unsigned values as decimal text, including values above signed 64-bit limits. The existing signed `timestamp_ns` query index uses ingestion time for source timestamps of zero or values beyond its range. The signed `duration_ns` summary is zero for durations beyond its range. Use the exact fields when interpreting original profile time and duration.

## Query and insert API

Authenticated `GET /api/v1/profiles?schema=otel` returns paginated `data`. Every row includes base64 `profile_bytes`, encoding/storage version, and `profile_decode_status`. Current rows also include structured OTLP `profile_data`, original IDs, exact time/duration, and complete dropped-attribute counts. `profile_id` remains the numeric database ID.

`POST /api/v1/profiles` accepts a profile object or array. To submit current protobuf, supply base64 `profile_bytes`, `profile_storage_version=1`, `profile_encoding=otlp-profiles-protobuf`, and `profile_schema_version="1"`. The payload must contain exactly one resource, scope, and profile, with valid dictionary references. Invalid payloads or declarations are rejected before writing. Searchable summaries derive from the protobuf; accepted raw bytes are retained exactly. Metadata omitted by older clients continues to identify an unversioned legacy payload.

## Legacy compatibility

Existing rows retain storage version `0` and encoding `legacy-unversioned`. Valid legacy JSON remains available as `profile_data`. Legacy protobuf remains downloadable through base64 `profile_bytes`; its bytes are not passed through a JSON string or decoded using the new incompatible protobuf schema.

The genuine Collector 0.135.0 fixture fails decoding under the 0.163.0 schema, and the upstream profile migration helper does not convert it. A matching development-version header does not prove old schema compatibility. Upgrade profile producers with the Collector; converting legacy protobuf requires a separate decoder for its original schema. Unknown versions and decode failures remain explicit in the query response.
