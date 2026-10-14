package repository

import (
	"testing"
)

func TestTemplateFieldExtractor_ExtractFieldsFromTemplate(t *testing.T) {
	extractor := NewTemplateFieldExtractor()
	helper := NewTestHelper()

	tests := []struct {
		name     string
		template string
		expected []string
	}{
		{
			name:     "simple gauge template",
			template: `Metric: The gauge '{{.name}}' from service {{.service_name}} recorded a value of {{.value}}. Labels: {{.labels}}.`,
			expected: []string{"name", "service_name", "value", "labels"},
		},
		{
			name:     "spans template with nested fields",
			template: `{{.service_name}} for operation '{{.name}}'. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events}}.{{end}}{{if .logs}} Logs: {{.logs}}.{{end}}{{if .resource_attributes.trazr_random_seed}}{{.resource_attributes.trazr_random_seed}}{{end}}`,
			expected: []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "events", "logs", "resource_attributes.trazr_random_seed"},
		},
		{
			name:     "logs template with conditional",
			template: `Log Event from service {{.service_name}}. Severity: {{.level}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}. Attributes: {{.attributes}}.`,
			expected: []string{"service_name", "level", "body", "message", "attributes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, err := extractor.ExtractFieldsFromTemplate(tt.template)
			if err != nil {
				t.Fatalf("ExtractFieldsFromTemplate() error = %v", err)
			}

			helper.AssertFieldsContain(t, fields, tt.expected, tt.name)
		})
	}
}

func TestTemplateFieldExtractor_MapTemplateFieldsToDBColumns(t *testing.T) {
	extractor := NewTemplateFieldExtractor()
	helper := NewTestHelper()

	tests := []struct {
		name           string
		signalType     string
		templateFields []string
		expectedCount  int // We expect at least this many field configs (including essential fields)
	}{
		{
			name:           "gauge metrics with template fields",
			signalType:     "gauge_metrics",
			templateFields: []string{"name", "service_name", "value", "labels"},
			expectedCount:  4, // 3 essential + 1 template field
		},
		{
			name:           "spans with nested resource attributes",
			signalType:     "spans",
			templateFields: []string{"name", "service_name", "status_code", "resource_attributes.trazr_random_seed"},
			expectedCount:  4, // 3 essential + 1 template field
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fieldConfigs := extractor.MapTemplateFieldsToDBColumns(tt.signalType, tt.templateFields)

			if len(fieldConfigs) < tt.expectedCount {
				t.Errorf("Expected at least %d field configs, got %d", tt.expectedCount, len(fieldConfigs))
			}

			helper.AssertFieldConfigsContainEssential(t, fieldConfigs, tt.name)
		})
	}
}

func TestTemplateFieldExtractor_ExtractFieldsFromTemplates(t *testing.T) {
	extractor := NewTemplateFieldExtractor()
	helper := NewTestHelper()

	templateContent := map[string]string{
		"gauge": `Metric: The gauge '{{.name}}' from service {{.service_name}} recorded a value of {{.value}}.`,
		"spans": `{{.service_name}} for operation '{{.name}}'. Status: {{.status_code}}. Duration: {{.duration_ms}}ms.`,
	}

	requiredFields, err := extractor.ExtractFieldsFromTemplates(templateContent)
	if err != nil {
		t.Fatalf("ExtractFieldsFromTemplates() error = %v", err)
	}

	// Check that we have fields for both signal types
	if len(requiredFields) != 2 {
		t.Errorf("Expected 2 signal types, got %d", len(requiredFields))
	}

	// Check gauge fields
	if gaugeFields, exists := requiredFields["gauge"]; exists {
		expectedGaugeFields := []string{"name", "service_name", "value"}
		helper.AssertFieldsContain(t, gaugeFields, expectedGaugeFields, "gauge fields")
	} else {
		t.Error("Gauge fields not found in required fields")
	}

	// Check spans fields
	if spansFields, exists := requiredFields["spans"]; exists {
		expectedSpansFields := []string{"service_name", "name", "status_code", "duration_ms"}
		helper.AssertFieldsContain(t, spansFields, expectedSpansFields, "spans fields")
	} else {
		t.Error("Spans fields not found in required fields")
	}
}

func TestTemplateFieldExtractor_AnalyzeTemplate(t *testing.T) {
	extractor := NewTemplateFieldExtractor()

	tests := []struct {
		name     string
		template string
		expected *TemplateAnalysis
	}{
		{
			name:     "simple template - no correlated signals",
			template: `{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.`,
			expected: &TemplateAnalysis{
				NeedsLogs:      false,
				NeedsExemplars: false,
				NeedsProfiles:  false,
				NeedsEvents:    false,
				ExemplarTypes:  []string{},
				RequiredFields: []string{"service_name", "name", "status_code", "duration_ms", "trace_id"},
			},
		},
		{
			name:     "complex template - needs logs",
			template: `{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}`,
			expected: &TemplateAnalysis{
				NeedsLogs:      true,
				NeedsExemplars: false,
				NeedsProfiles:  false,
				NeedsEvents:    false,
				ExemplarTypes:  []string{},
				RequiredFields: []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "logs"},
			},
		},
		{
			name:     "complex template - needs exemplars",
			template: `{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .exemplars_histogram}} Histogram Exemplars: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`,
			expected: &TemplateAnalysis{
				NeedsLogs:      false,
				NeedsExemplars: true,
				NeedsProfiles:  false,
				NeedsEvents:    false,
				ExemplarTypes:  []string{SignalTypeHistogramMetrics},
				RequiredFields: []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "exemplars_histogram"},
			},
		},
		{
			name:     "complex template - needs profiles",
			template: `{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .profiles}} Profiles: {{.profile_count}}.{{end}}`,
			expected: &TemplateAnalysis{
				NeedsLogs:      false,
				NeedsExemplars: false,
				NeedsProfiles:  true,
				NeedsEvents:    false,
				ExemplarTypes:  []string{},
				RequiredFields: []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "profiles", "profile_count"},
			},
		},
		{
			name:     "complex template - needs events",
			template: `{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}`,
			expected: &TemplateAnalysis{
				NeedsLogs:      false,
				NeedsExemplars: false,
				NeedsProfiles:  false,
				NeedsEvents:    true,
				ExemplarTypes:  []string{},
				RequiredFields: []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "events"},
			},
		},
		{
			name:     "complex template - needs multiple correlated signals",
			template: `{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .profiles}} Profiles: {{.profile_count}}.{{end}}`,
			expected: &TemplateAnalysis{
				NeedsLogs:      true,
				NeedsExemplars: true,
				NeedsProfiles:  true,
				NeedsEvents:    false,
				ExemplarTypes:  []string{SignalTypeGaugeMetrics},
				RequiredFields: []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "logs", "exemplars_gauge", "profiles", "profile_count"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.AnalyzeTemplate(tt.template)

			// Check basic fields
			if result.NeedsLogs != tt.expected.NeedsLogs {
				t.Errorf("NeedsLogs = %v, want %v", result.NeedsLogs, tt.expected.NeedsLogs)
			}
			if result.NeedsExemplars != tt.expected.NeedsExemplars {
				t.Errorf("NeedsExemplars = %v, want %v", result.NeedsExemplars, tt.expected.NeedsExemplars)
			}
			if result.NeedsProfiles != tt.expected.NeedsProfiles {
				t.Errorf("NeedsProfiles = %v, want %v", result.NeedsProfiles, tt.expected.NeedsProfiles)
			}
			if result.NeedsEvents != tt.expected.NeedsEvents {
				t.Errorf("NeedsEvents = %v, want %v", result.NeedsEvents, tt.expected.NeedsEvents)
			}

			// Check exemplar types
			if len(result.ExemplarTypes) != len(tt.expected.ExemplarTypes) {
				t.Errorf("ExemplarTypes length = %d, want %d", len(result.ExemplarTypes), len(tt.expected.ExemplarTypes))
			} else {
				for i, expectedType := range tt.expected.ExemplarTypes {
					if i >= len(result.ExemplarTypes) || result.ExemplarTypes[i] != expectedType {
						t.Errorf("ExemplarTypes[%d] = %v, want %v", i, result.ExemplarTypes[i], expectedType)
					}
				}
			}

			// Check required fields (order doesn't matter)
			if len(result.RequiredFields) != len(tt.expected.RequiredFields) {
				t.Errorf("RequiredFields length = %d, want %d", len(result.RequiredFields), len(tt.expected.RequiredFields))
			} else {
				// Create maps for comparison
				resultMap := make(map[string]bool)
				expectedMap := make(map[string]bool)
				for _, field := range result.RequiredFields {
					resultMap[field] = true
				}
				for _, field := range tt.expected.RequiredFields {
					expectedMap[field] = true
				}
				for field := range expectedMap {
					if !resultMap[field] {
						t.Errorf("RequiredFields missing: %s", field)
					}
				}
			}
		})
	}
}

func TestTemplateFieldExtractorPreservesRootScope(t *testing.T) {
	extractor := NewTemplateFieldExtractor()
	for _, templateText := range []string{
		`{{range .events}}{{.Name}} {{$.service_name}}{{else}}{{.name}}{{end}}`,
		`{{with .attributes}}{{.region}} {{$.service_name}}{{else}}{{.name}}{{end}}`,
	} {
		fields, err := extractor.ExtractFieldsFromTemplate(templateText)
		if err != nil {
			t.Fatal(err)
		}
		found := make(map[string]bool)
		for _, field := range fields {
			found[field] = true
		}
		if !found["service_name"] || !found["name"] || len(found) != 3 {
			t.Fatalf("unexpected root fields: %v", fields)
		}
		if found["Name"] || found["region"] {
			t.Fatalf("nested fields became root SQL columns: %v", fields)
		}
	}
}
