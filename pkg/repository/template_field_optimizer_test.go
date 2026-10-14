package repository

import (
	"github.com/stretchr/testify/assert"
	"testing"

	templatepkg "github.com/medxops/trazr-otel/pkg/repository/template"
)

func TestTemplateFieldOptimizer_OptimizeTemplates(t *testing.T) {
	optimizer := NewTemplateFieldOptimizer()
	helper := NewTestHelper()

	// Test data using actual database templates
	contentTemplates := GetTestContentTemplates()
	metadataTemplates := GetTestMetadataTemplates()

	// Optimize templates
	optimizedLists, err := optimizer.OptimizeTemplates(contentTemplates, metadataTemplates)
	if err != nil {
		t.Fatalf("Failed to optimize templates: %v", err)
	}

	// Test spans optimization
	spansOptimized := optimizedLists["spans"]
	if spansOptimized == nil {
		t.Fatalf("Spans optimization not found")
	}

	// Verify static fields
	expectedStaticFields := []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id"}
	helper.AssertFieldsContain(t, spansOptimized.StaticFields, expectedStaticFields, "spans static fields")

	// Full JSONB objects are preserved for nested metadata extraction.
	assert.Empty(t, spansOptimized.JSONBFields)
	assert.ElementsMatch(t, []string{"attributes", "resource_attributes", "events", "logs", "exemplars_histogram", "exemplars_summary", "exemplars_gauge", "exemplars_sum"}, spansOptimized.FullJSONBFields)
	assert.ElementsMatch(t, expectedStaticFields, spansOptimized.StaticFields)

	t.Logf("Spans optimization: %+v", spansOptimized)
}

func TestTemplateFieldOptimizer_GenerateSQLSelect(t *testing.T) {
	optimizer := NewTemplateFieldOptimizer()
	helper := NewTestHelper()

	// Create a test optimized field list
	optimizedList := &OptimizedFieldList{
		SignalType:   "spans",
		StaticFields: []string{"id", "name", "service_name", "status_code", "duration_ms", "trace_id", "span_id"},
		JSONBFields: map[string][]string{
			"attributes":          {"region"},
			"resource_attributes": {"trazr_random_seed"},
		},
		FullJSONBFields: []string{},
	}

	// Generate SQL SELECT clause
	sqlSelect := optimizer.GenerateSQLSelect(optimizedList)

	// Verify the SQL contains expected elements
	expectedElements := []string{
		"id", "name", "service_name", "status_code", "duration_ms", "trace_id", "span_id",
		"COALESCE(attributes->>'region', '') as attributes_region",
		"COALESCE(resource_attributes->>'trazr.random.seed', '') as resource_attributes_trazr_random_seed",
	}

	helper.AssertSQLContains(t, sqlSelect, expectedElements, "SQL generation")

	t.Logf("Generated SQL: %s", sqlSelect)
}

func TestTemplateFieldOptimizer_ConflictResolution(t *testing.T) {
	optimizer := NewTemplateFieldOptimizer()

	// Test case: both full field and specific keys
	contentTemplates := map[string]string{
		"test": "{{.attributes | formatMap}}", // Full field
	}

	metadataTemplates := map[string][]string{
		"test": {"attributes.region", "attributes.service"}, // Specific keys
	}

	optimizedLists, err := optimizer.OptimizeTemplates(contentTemplates, metadataTemplates)
	if err != nil {
		t.Fatalf("Failed to optimize templates: %v", err)
	}

	optimized := optimizedLists["test"]
	if optimized == nil {
		t.Fatalf("Test optimization not found")
	}

	// Should prefer full field over specific extractions
	if len(optimized.FullJSONBFields) == 0 {
		t.Errorf("Expected full JSONB field to be preferred")
	}
	if len(optimized.JSONBFields) > 0 {
		t.Errorf("Expected no specific JSONB extractions when full field is present")
	}

	t.Logf("Conflict resolution result: %+v", optimized)
}

func TestTemplateFieldOptimizer_GetRequiredFields(t *testing.T) {
	optimizer := NewTemplateFieldOptimizer()
	helper := NewTestHelper()

	optimizedList := &OptimizedFieldList{
		SignalType:   "spans",
		StaticFields: []string{"id", "name", "service_name"},
		JSONBFields: map[string][]string{
			"attributes": {"region"},
		},
		FullJSONBFields: []string{"events"},
	}

	requiredFields := optimizer.GetRequiredFields(optimizedList)

	expectedFields := []string{"id", "name", "service_name", "attributes", "events"}
	helper.AssertFieldsContain(t, requiredFields, expectedFields, "required fields")

	t.Logf("Required fields: %v", requiredFields)
}

func TestTemplateFieldOptimizer_JSONBFieldDetection(t *testing.T) {
	fieldMapping := templatepkg.NewFieldMapping()

	testCases := []struct {
		field    string
		expected bool
	}{
		{"attributes", true},
		{"resource_attributes", true},
		{"attributes.region", true},
		{"resource_attributes.trazr_random_seed", true},
		{"name", false},
		{"service_name", false},
		{"unknown.field", false},
	}

	for _, tc := range testCases {
		result := fieldMapping.IsJSONBField(tc.field)
		if result != tc.expected {
			t.Errorf("Field %q: expected %v, got %v", tc.field, tc.expected, result)
		}
	}
}

func TestTemplateFieldOptimizer_ParseJSONBField(t *testing.T) {
	fieldMapping := templatepkg.NewFieldMapping()

	testCases := []struct {
		field        string
		expectedBase string
		expectedKey  string
	}{
		{"attributes", "attributes", ""},
		{"attributes.region", "attributes", "region"},
		{"resource_attributes.trazr_random_seed", "resource_attributes", "trazr_random_seed"},
		{"attributes.service.name", "attributes", "service.name"},
	}

	for _, tc := range testCases {
		base, key := fieldMapping.ParseJSONBField(tc.field)
		if base != tc.expectedBase || key != tc.expectedKey {
			t.Errorf("Field %q: expected (%s, %s), got (%s, %s)",
				tc.field, tc.expectedBase, tc.expectedKey, base, key)
		}
	}
}
