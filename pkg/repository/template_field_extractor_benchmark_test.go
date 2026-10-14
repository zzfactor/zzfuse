package repository

import (
	"testing"
)

func BenchmarkTemplateFieldExtractor_ExtractFieldsFromTemplate(b *testing.B) {
	extractor := NewTemplateFieldExtractor()
	template := `{{.service_name}} for operation '{{.name}}'. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events}}.{{end}}{{if .logs}} Logs: {{.logs}}.{{end}}{{if .resource_attributes.trazr_random_seed}}{{.resource_attributes.trazr_random_seed}}{{end}}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.ExtractFieldsFromTemplate(template)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemplateFieldExtractor_MapTemplateFieldsToDBColumns(b *testing.B) {
	extractor := NewTemplateFieldExtractor()
	templateFields := []string{"name", "service_name", "status_code", "duration_ms", "trace_id", "events", "logs", "resource_attributes.trazr_random_seed"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		extractor.MapTemplateFieldsToDBColumns("spans", templateFields)
	}
}

func BenchmarkTemplateFieldExtractor_ExtractFieldsFromTemplates(b *testing.B) {
	extractor := NewTemplateFieldExtractor()
	templateContent := map[string]string{
		"gauge": `Metric: The gauge '{{.name}}' from service {{.service_name}} recorded a value of {{.value}}. Labels: {{.labels}}.`,
		"spans": `{{.service_name}} for operation '{{.name}}'. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events}}.{{end}}{{if .logs}} Logs: {{.logs}}.{{end}}{{if .resource_attributes.trazr_random_seed}}{{.resource_attributes.trazr_random_seed}}{{end}}`,
		"logs":  `Log Event from service {{.service_name}}. Severity: {{.level}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}. Attributes: {{.attributes}}.`,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := extractor.ExtractFieldsFromTemplates(templateContent)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Benchmark comparison: Full field selection vs Template-driven field selection
func BenchmarkFieldSelectionComparison(b *testing.B) {
	extractor := NewTemplateFieldExtractor()

	// Template for spans
	spansTemplate := `{{.service_name}} for operation '{{.name}}'. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.`

	b.Run("TemplateDriven", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			fields, _ := extractor.ExtractFieldsFromTemplate(spansTemplate)
			extractor.MapTemplateFieldsToDBColumns("spans", fields)
		}
	})

	b.Run("FullFieldSelection", func(b *testing.B) {
		// Simulate full field selection (all fields)
		allFields := []string{"id", "timestamp_ns", "timestamp", "end_time", "duration", "trace_id", "span_id", "parent_span_id", "trace_state", "name", "span_kind", "service_name", "resource_attributes", "scope_name", "scope_version", "scope_attributes", "scope_dropped_attr_count", "attributes", "dropped_attr_count", "events", "dropped_events_count", "links", "dropped_links_count", "status_code", "status_message", "metadata", "resource_schema_url", "scope_schema_url"}

		for i := 0; i < b.N; i++ {
			extractor.MapTemplateFieldsToDBColumns("spans", allFields)
		}
	})
}
