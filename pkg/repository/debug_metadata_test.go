package repository

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

func TestDebugMetadataFields(t *testing.T) {
	// This test helps debug why certain fields are missing from metadata output

	// Sample data that might come from the database
	sampleSpanData := map[string]interface{}{
		"id":           "test-span-id",
		"name":         "GET /api/users",
		"trace_id":     "39fafb9f90c507d8916c97c19f07b126",
		"status_code":  1,
		"service_name": "trazr-gen",
		"timestamp_ns": 1753962803592545000,
		"duration":     150, // Note: database has 'duration', not 'duration_ms'
		"span_id":      "test-span-id-123",
		"attributes": map[string]interface{}{
			"region": "us-west-2",
		},
		"resource_attributes": map[string]interface{}{
			"trazrgen.random.seed": "12345", // Note: database has dots, not underscores
		},
	}

	// Get the default template metadata for spans
	defaultMetadata := database.DefaultTemplateMetadata
	spansMetadataFields, exists := defaultMetadata["spans"]
	if !exists {
		t.Fatalf("Spans metadata not found in default template metadata")
	}

	t.Logf("Default spans metadata fields: %v", spansMetadataFields)

	// Test selectMetadataFields with the sample data
	result := selectMetadataFields(sampleSpanData, spansMetadataFields)

	t.Logf("Sample data keys: %v", getMapKeysDebug(sampleSpanData))
	t.Logf("Expected metadata fields: %v", spansMetadataFields)
	t.Logf("Actual result: %v", result)

	// Check each expected field
	for _, field := range spansMetadataFields {
		if val, exists := result[field]; exists {
			t.Logf("✅ Field '%s' found: %v", field, val)
		} else {
			t.Logf("❌ Field '%s' MISSING", field)

			// Try to find it in the original data
			if val, exists := getNestedField(sampleSpanData, field); exists {
				t.Logf("   -> But found in original data: %v", val)
			} else {
				t.Logf("   -> Not found in original data either")
			}
		}
	}

	// Test with the actual data structure you're seeing
	actualData := map[string]interface{}{
		"name":         "GET /api/users",
		"trace_id":     "39fafb9f90c507d8916c97c19f07b126",
		"status_code":  1,
		"service_name": "trazr-gen",
		"timestamp_ns": 1753962803592545000,
	}

	t.Logf("\n=== Testing with actual data structure ===")
	t.Logf("Actual data keys: %v", getMapKeysDebug(actualData))

	result2 := selectMetadataFields(actualData, spansMetadataFields)
	t.Logf("Result with actual data: %v", result2)

	// Pretty print the results
	prettyResult, _ := json.MarshalIndent(result2, "", "  ")
	t.Logf("Pretty result:\n%s", string(prettyResult))
}

func TestDebugFieldMapping(t *testing.T) {
	// Test the field mapping logic

	// Test cases for different field formats
	testCases := []struct {
		field    string
		data     map[string]interface{}
		expected interface{}
		found    bool
	}{
		{
			field: "duration_ms",
			data: map[string]interface{}{
				"duration": 150, // Database has 'duration', template expects 'duration_ms'
			},
			expected: nil,
			found:    false,
		},
		{
			field: "resource_attributes.trazrgen_random_seed",
			data: map[string]interface{}{
				"resource_attributes": map[string]interface{}{
					"trazrgen.random.seed": "12345", // Database has dots, template expects underscores
				},
			},
			expected: "12345",
			found:    true,
		},
		{
			field:    "attributes.region.name",
			data:     map[string]interface{}{"attributes": map[string]interface{}{"region": "us-west-2"}},
			expected: nil,
			found:    false,
		},
		{
			field: "attributes.region",
			data: map[string]interface{}{
				"attributes": map[string]interface{}{
					"region": "us-west-2",
				},
			},
			expected: "us-west-2",
			found:    true,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("case_%d_%s", i, tc.field), func(t *testing.T) {
			val, found := getNestedField(tc.data, tc.field)
			if found != tc.found {
				t.Errorf("Expected found=%v, got found=%v", tc.found, found)
			}
			if found && val != tc.expected {
				t.Errorf("Expected value=%v, got value=%v", tc.expected, val)
			}
			t.Logf("Field '%s': found=%v, value=%v", tc.field, found, val)
		})
	}
}

// Helper function to get map keys
func getMapKeysDebug(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
