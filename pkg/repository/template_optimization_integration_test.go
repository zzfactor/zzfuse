package repository

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTemplateOptimization_CompleteWorkflow(t *testing.T) {
	// Test the complete workflow: template optimization -> field selection -> SQL generation
	helper := NewTestHelper()

	// Use actual database templates
	contentTemplates := GetTestContentTemplates()
	metadataTemplates := GetTestMetadataTemplates()

	// Step 1: Optimize templates
	optimizer := NewTemplateFieldOptimizer()
	optimizedLists, err := optimizer.OptimizeTemplates(contentTemplates, metadataTemplates)
	if err != nil {
		t.Fatalf("Failed to optimize templates: %v", err)
	}

	// Step 2: Test each signal type
	testCases := []struct {
		name             string
		signalType       string
		workerSignalType string
		expectedStatic   []string
		expectedJSONB    map[string][]string
	}{
		{
			name:             "spans optimization",
			signalType:       "spans",
			workerSignalType: "spans",
			expectedStatic:   []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id"},
			expectedJSONB:    map[string][]string{},
		},
		{
			name:             "logs optimization",
			signalType:       "logs",
			workerSignalType: "logs",
			expectedStatic:   []string{"service_name", "level", "body", "message"},
			expectedJSONB:    map[string][]string{},
		},
		{
			name:             "gauge optimization",
			signalType:       "gauge",
			workerSignalType: "gauge_metrics",
			expectedStatic:   []string{"name", "service_name", "value"},
			expectedJSONB:    map[string][]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			optimizedList := optimizedLists[tc.signalType]
			if optimizedList == nil {
				t.Fatalf("Optimized list not found for signal type: %s", tc.signalType)
			}

			// Verify static fields
			helper.AssertFieldsContain(t, optimizedList.StaticFields, tc.expectedStatic, tc.name+" static fields")

			// Verify JSONB fields
			helper.AssertJSONBFieldsMatch(t, optimizedList.JSONBFields, tc.expectedJSONB, tc.name+" JSONB fields")
			assert.Empty(t, optimizedList.JSONBFields)
			if tc.signalType == "spans" {
				assert.ElementsMatch(t, tc.expectedStatic, optimizedList.StaticFields)
				assert.ElementsMatch(t, []string{"attributes", "resource_attributes", "events", "logs", "exemplars_histogram", "exemplars_summary", "exemplars_gauge", "exemplars_sum"}, optimizedList.FullJSONBFields)
			}

			// Step 3: Generate SQL SELECT clause
			sqlSelect := optimizer.GenerateSQLSelect(optimizedList)
			t.Logf("Generated SQL for %s: %s", tc.signalType, sqlSelect)

			// Step 4: Get required fields for database query
			requiredFields := optimizer.GetRequiredFields(optimizedList)
			t.Logf("Required fields for %s: %v", tc.signalType, requiredFields)

			// Step 5: Convert to SignalFieldConfig (simulate worker integration)
			extractor := NewTemplateFieldExtractor()
			fieldConfigs := extractor.MapTemplateFieldsToDBColumns(tc.workerSignalType, optimizedList.StaticFields)

			// Add JSONB field configs
			for baseField := range optimizedList.JSONBFields {
				if config := extractor.getFieldConfig(tc.workerSignalType, baseField); config != nil {
					fieldConfigs = append(fieldConfigs, *config)
				}
			}

			t.Logf("Field configs for %s: %d configs", tc.signalType, len(fieldConfigs))
		})
	}
}

func TestTemplateOptimization_SpansSpecificAnalysis(t *testing.T) {
	// Detailed analysis of spans template optimization
	helper := NewTestHelper()

	contentTemplate := TestSpansContentTemplate
	metadataTemplate := []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id", "attributes.region", "resource_attributes.trazr_random_seed"}

	optimizer := NewTemplateFieldOptimizer()

	contentTemplates := map[string]string{"spans": contentTemplate}
	metadataTemplates := map[string][]string{"spans": metadataTemplate}

	optimizedLists, err := optimizer.OptimizeTemplates(contentTemplates, metadataTemplates)
	if err != nil {
		t.Fatalf("Failed to optimize templates: %v", err)
	}

	optimizedList := optimizedLists["spans"]
	if optimizedList == nil {
		t.Fatalf("Spans optimization not found")
	}

	t.Logf("Spans optimization details:")
	t.Logf("  Static fields: %v", optimizedList.StaticFields)
	t.Logf("  JSONB fields: %v", optimizedList.JSONBFields)
	t.Logf("  Full JSONB fields: %v", optimizedList.FullJSONBFields)

	// Verify the optimization logic
	expectedStaticFields := []string{"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id"}
	helper.AssertFieldsContain(t, optimizedList.StaticFields, expectedStaticFields, "spans optimization static fields")

	assert.Empty(t, optimizedList.JSONBFields)
	assert.ElementsMatch(t, expectedStaticFields, optimizedList.StaticFields)
	assert.ElementsMatch(t, []string{"attributes", "resource_attributes", "events", "logs", "exemplars_histogram", "exemplars_summary", "exemplars_gauge", "exemplars_sum"}, optimizedList.FullJSONBFields)

	// Generate SQL and verify
	sqlSelect := optimizer.GenerateSQLSelect(optimizedList)
	t.Logf("Generated SQL: %s", sqlSelect)

	// Verify SQL contains expected elements
	expectedSQLElements := []string{
		"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id",
		"attributes", "resource_attributes",
	}

	helper.AssertSQLContains(t, sqlSelect, expectedSQLElements, "spans SQL generation")
	assert.NotContains(t, sqlSelect, "COALESCE", "full objects must not be replaced with individual aliases")
}

func TestTemplateOptimization_PerformanceComparison(t *testing.T) {
	// Compare performance between old and new approaches

	contentTemplates := map[string]string{
		"spans": TestSpansContentTemplate,
	}

	metadataTemplates := map[string][]string{
		"spans": {"service_name", "name", "status_code", "duration_ms", "trace_id", "span_id", "attributes.region", "resource_attributes.trazr_random_seed"},
	}

	optimizer := NewTemplateFieldOptimizer()

	// Benchmark the optimization process
	optimizedLists, err := optimizer.OptimizeTemplates(contentTemplates, metadataTemplates)
	if err != nil {
		t.Fatalf("Failed to optimize templates: %v", err)
	}

	optimizedList := optimizedLists["spans"]
	if optimizedList == nil {
		t.Fatalf("Spans optimization not found")
	}

	// Count fields for comparison
	totalFields := len(optimizedList.StaticFields) + len(optimizedList.FullJSONBFields)
	for _, keys := range optimizedList.JSONBFields {
		totalFields += len(keys)
	}

	// Typical full field count for spans (all columns)
	fullFieldCount := 25 // Approximate number of columns in spans table

	t.Logf("Performance comparison:")
	t.Logf("  Optimized field count: %d", totalFields)
	t.Logf("  Full field count: %d", fullFieldCount)
	t.Logf("  Reduction: %.1f%%", float64(fullFieldCount-totalFields)/float64(fullFieldCount)*100)

	// Verify we have a reasonable reduction
	if totalFields >= fullFieldCount {
		t.Errorf("Optimization should reduce field count, but got %d >= %d", totalFields, fullFieldCount)
	}
}
