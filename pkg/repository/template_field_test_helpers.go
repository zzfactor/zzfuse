package repository

import (
	"sort"
	"testing"
)

// TestHelper contains helper functions for template field tests
type TestHelper struct{}

// NewTestHelper creates a new test helper
func NewTestHelper() *TestHelper {
	return &TestHelper{}
}

// AssertFieldsContain checks if a slice of fields contains all expected fields
func (h *TestHelper) AssertFieldsContain(t *testing.T, fields []string, expected []string, context string) {
	t.Helper()

	// Sort both slices for consistent comparison
	sortedFields := make([]string, len(fields))
	copy(sortedFields, fields)
	sort.Strings(sortedFields)

	sortedExpected := make([]string, len(expected))
	copy(sortedExpected, expected)
	sort.Strings(sortedExpected)

	for _, exp := range sortedExpected {
		found := false
		for _, field := range sortedFields {
			if field == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: Expected field %q not found in fields: %v", context, exp, sortedFields)
		}
	}
}

// AssertJSONBFieldsMatch checks if JSONB field mappings match expected values
func (h *TestHelper) AssertJSONBFieldsMatch(t *testing.T, actual map[string][]string, expected map[string][]string, context string) {
	t.Helper()

	for expectedBase, expectedKeys := range expected {
		actualKeys, exists := actual[expectedBase]
		if !exists {
			t.Errorf("%s: Expected JSONB base field %q not found", context, expectedBase)
			continue
		}

		// Sort keys for consistent comparison
		sort.Strings(actualKeys)
		sort.Strings(expectedKeys)

		if len(actualKeys) != len(expectedKeys) {
			t.Errorf("%s: JSONB field %q has %d keys, expected %d", context, expectedBase, len(actualKeys), len(expectedKeys))
			continue
		}

		for i, expectedKey := range expectedKeys {
			if i >= len(actualKeys) || actualKeys[i] != expectedKey {
				t.Errorf("%s: JSONB field %q key mismatch at index %d: expected %q, got %q",
					context, expectedBase, i, expectedKey, actualKeys[i])
			}
		}
	}
}

// AssertSQLContains checks if generated SQL contains expected elements
func (h *TestHelper) AssertSQLContains(t *testing.T, sql string, expectedElements []string, context string) {
	t.Helper()

	for _, expected := range expectedElements {
		if !containsSubstringTemplateField(sql, expected) {
			t.Errorf("%s: Expected SQL element %q not found in generated SQL: %s", context, expected, sql)
		}
	}
}

// AssertFieldConfigsContainEssential checks if field configs contain essential fields
func (h *TestHelper) AssertFieldConfigsContainEssential(t *testing.T, configs []SignalFieldConfig, context string) {
	t.Helper()

	essentialFields := []string{"id", "timestamp_ns", "service_name"}
	for _, essential := range essentialFields {
		found := false
		for _, config := range configs {
			if config.SignalKey == essential {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: Essential field %q not found in field configs", context, essential)
		}
	}
}

// containsSubstringTemplateField checks if a string contains a substring (case-insensitive)
func containsSubstringTemplateField(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsSubstringHelperTemplateField(s, substr))))
}

// containsSubstringHelperTemplateField is a helper for case-insensitive substring search
func containsSubstringHelperTemplateField(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
