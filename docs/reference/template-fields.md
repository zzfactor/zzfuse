# Trazr OTEL Template Field Reference

This document lists all available keys you can use in Go text templates for logs, traces, and metrics in the Trazr OTEL pipeline. It is based on the current database schema and ingestion logic.

---

## Logs Table (`logs`)

**Top-level fields:**
- `id`, `timestamp_ns`, `timestamp`, `observed_timestamp`, `trace_id`, `span_id`, `flags`, `severity_text`, `severity_number`, `service_name`, `body`, `dropped_attr_count`, `metadata`
- `resource_attributes` (map), `attributes` (map)
- `scope_name`, `scope_version`, `scope_attributes` (map), `scope_dropped_attr_count`, `scope_schema_url`, `resource_schema_url`

**Template usage:**
- Direct: `{{.service_name}}`, `{{.body}}`, `{{.severity_text}}`, etc.
- Nested: `{{.resource_attributes.key}}`, `{{.attributes.key}}`

---

## Traces Table (`traces`)

**Top-level fields:**
- `id`, `timestamp_ns`, `timestamp`, `trace_id`, `span_id`, `parent_span_id`, `trace_state`, `span_name`, `span_kind`, `service_name`, `status_code`, `status_message`, `start_time_ns`, `start_time`, `end_time_ns`, `end_time`, `duration`, `dropped_attr_count`, `dropped_events_count`, `dropped_links_count`, `metadata`
- `resource_attributes` (map), `attributes` (map), `events` (array), `links` (array)
- `scope_name`, `scope_version`, `scope_attributes` (map), `scope_dropped_attr_count`, `scope_schema_url`, `resource_schema_url`

**Template usage:**
- Direct: `{{.trace_id}}`, `{{.span_id}}`, `{{.span_name}}`, `{{.service_name}}`, etc.
- Nested: `{{.resource_attributes.key}}`, `{{.attributes.key}}`

---

## Metrics Tables (Gauge, Sum, Histogram, Summary, Exponential Histogram)

**Common top-level fields:**
- `id`, `timestamp_ns`, `timestamp`, `service_name`, `name`, `description`, `unit`, `value`, `count`, `sum`, `scale`, `zero_count`, `positive_offset`, `negative_offset`, `min`, `max`, `aggregation_temporality`, `is_monotonic`, `flags`, `start_time_ns`, `start_time`, `dropped_attr_count`, `metadata`
- `attributes` (map), `resource_attributes` (map)
- `positive_bucket_counts` (array/map), `negative_bucket_counts` (array/map), `buckets` (array/map), `quantiles` (array/map), `exemplars` (array)
- `scope_name`, `scope_version`, `scope_attributes` (map), `scope_dropped_attr_count`, `scope_schema_url`, `resource_schema_url`

**Template usage:**
- Direct: `{{.service_name}}`, `{{.name}}`, `{{.value}}`, `{{.count}}`, `{{.sum}}`, etc.
- Nested: `{{.attributes.key}}`, `{{.resource_attributes.key}}`
- Arrays/JSONB: For arrays or JSONB fields (e.g., `buckets`, `quantiles`, `exemplars`), use a custom template function to iterate or format if needed.

---

## Guidance for Template Authors

- **Direct fields:** Use `{{.field_name}}` for any top-level column.
- **Nested fields:** Use `{{.attributes.key}}`, `{{.resource_attributes.key}}`, etc., for nested maps.
- **Arrays/JSONB:** To iterate or format arrays/maps, register a custom function in Go and use it in your template.
- **Missing fields:** If a field is missing in the data, Go templates render it as `<no value>` or blank.

---

## Example Template Snippets

**Log Example:**
```
Log level: {{.level}}. Service: {{if .service_name}}{{.service_name}}{{else}}{{.attributes.service_name}}{{end}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}.
```

**Trace Example:**
```
Trace: {{.trace_id}}. Span: {{.span_id}}. Operation: {{.span_name}}.
```

**Gauge Metric Example:**
```
Gauge {{.name}}: value={{.value}} at {{.timestamp}} (labels: {{.attributes.label}})
```

**Iterating Attributes (with helper):**
```
Attributes: {{range rangeMap .attributes}}{{.}}; {{end}}
```

---

## Extending This Document
- Update this document if you add new columns or nested fields to your tables.
- For new signal types, add a section with available fields.
- For custom template helpers, document their usage here. 