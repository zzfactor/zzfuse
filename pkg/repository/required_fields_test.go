package repository

import (
	"testing"
)

func TestRequiredFieldsExtraction(t *testing.T) {
	extractor := NewTemplateFieldExtractor()

	// Test template with various fields
	template := `{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.`

	analysis := extractor.AnalyzeTemplate(template)

	// Check that RequiredFields contains the expected fields
	expectedFields := []string{"service_name", "name", "status_code", "duration_ms", "trace_id"}

	if len(analysis.RequiredFields) != len(expectedFields) {
		t.Errorf("Expected %d required fields, got %d", len(expectedFields), len(analysis.RequiredFields))
	}

	// Check that all expected fields are present
	fieldMap := make(map[string]bool)
	for _, field := range analysis.RequiredFields {
		fieldMap[field] = true
	}

	for _, expectedField := range expectedFields {
		if !fieldMap[expectedField] {
			t.Errorf("Missing required field: %s", expectedField)
		}
	}

	// Verify other analysis fields are correct
	if analysis.NeedsLogs {
		t.Error("Template should not need logs")
	}
	if analysis.NeedsExemplars {
		t.Error("Template should not need exemplars")
	}
	if analysis.NeedsProfiles {
		t.Error("Template should not need profiles")
	}
	if analysis.NeedsEvents {
		t.Error("Template should not need events")
	}
	if len(analysis.ExemplarTypes) != 0 {
		t.Error("Template should not need exemplar types")
	}
}
