package repository

// Test template constants to avoid duplication across test files
const (
	// Content templates
	TestSpansContentTemplate = `{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}{{if .exemplars_histogram}} Histogram Exemplars: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_summary}} Summary Exemplars: {{range .exemplars_summary}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_sum}} Sum Exemplars: {{range .exemplars_sum}}{{.MetricName}}={{.Value}} {{end}}.{{end}}  {{.resource_attributes.trazr_random_seed}}`

	TestLogsContentTemplate = `Log Event from service {{.service_name}}. Severity: {{.level | default "info"}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}. Attributes: {{.attributes | formatMap}}.`

	TestGaugeContentTemplate = `Metric: The gauge "{{.name}}" from service {{.service_name}} recorded a value of {{.value}}. Labels: {{.labels | formatMap}}.`

	// Metadata templates
	TestSpansMetadataTemplate = `{"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id", "attributes.region", "resource_attributes.trazr_random_seed"}`

	TestLogsMetadataTemplate = `{"service_name", "level", "body", "message"}`

	TestGaugeMetadataTemplate = `{"service_name", "name", "value", "labels"}`
)

// GetTestContentTemplates returns a map of test content templates
func GetTestContentTemplates() map[string]string {
	return map[string]string{
		"spans": TestSpansContentTemplate,
		"logs":  TestLogsContentTemplate,
		"gauge": TestGaugeContentTemplate,
	}
}

// GetTestMetadataTemplates returns a map of test metadata templates
func GetTestMetadataTemplates() map[string][]string {
	return map[string][]string{
		"spans": {"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id", "attributes.region", "resource_attributes.trazr_random_seed"},
		"logs":  {"service_name", "level", "body", "message"},
		"gauge": {"service_name", "name", "value", "labels"},
	}
}
