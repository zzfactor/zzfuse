package repository

import (
	"testing"
)

func TestDatabaseSignalFilter(t *testing.T) {
	t.Run("empty filter accepts all signals", func(t *testing.T) {
		config := map[string]interface{}{}
		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse empty filter: %v", err)
		}

		attrs := map[string]interface{}{
			"service.name": "test-service",
			"severity":     "ERROR",
		}

		if !filter.Matches(attrs, FilterLevelResource) {
			t.Error("Empty filter should accept all signals for resource level")
		}
		if !filter.Matches(attrs, FilterLevelScope) {
			t.Error("Empty filter should accept all signals for scope level")
		}
		if !filter.Matches(attrs, FilterLevelAttributes) {
			t.Error("Empty filter should accept all signals for attributes level")
		}
	})

	t.Run("service name DSL format in resource section", func(t *testing.T) {
		config := map[string]interface{}{
			"resource": map[string]interface{}{
				"service.name": map[string]interface{}{
					"op":    "=",
					"value": "api-service",
				},
			},
		}
		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse service name DSL filter: %v", err)
		}

		attrs1 := map[string]interface{}{
			"service.name": "api-service",
		}
		if !filter.Matches(attrs1, FilterLevelResource) {
			t.Error("Should match exact service name with DSL in resource section")
		}

		attrs2 := map[string]interface{}{
			"service.name": "other-service",
		}
		if filter.Matches(attrs2, FilterLevelResource) {
			t.Error("Should not match different service name with DSL in resource section")
		}

		// Should accept all for other levels when resource section is empty
		if !filter.Matches(attrs1, FilterLevelScope) {
			t.Error("Should accept all signals for scope level when scope section is empty")
		}
		if !filter.Matches(attrs1, FilterLevelAttributes) {
			t.Error("Should accept all signals for attributes level when attributes section is empty")
		}
	})

	t.Run("wildcard accepts all (DSL) in resource section", func(t *testing.T) {
		config := map[string]interface{}{
			"resource": map[string]interface{}{
				"service.name": map[string]interface{}{
					"op":    "=",
					"value": "*",
				},
			},
		}
		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse wildcard DSL filter: %v", err)
		}

		attrs := map[string]interface{}{
			"service.name": "any-service",
		}
		if !filter.Matches(attrs, FilterLevelResource) {
			t.Error("Wildcard DSL should accept any service name in resource section")
		}
	})

	t.Run("DSL format with IN operator in attributes section", func(t *testing.T) {
		config := map[string]interface{}{
			"attributes": map[string]interface{}{
				"severity": map[string]interface{}{
					"op":    "IN",
					"value": []interface{}{"ERROR", "WARN"},
				},
			},
		}
		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse IN filter: %v", err)
		}

		// Should match first value
		attrs1 := map[string]interface{}{
			"severity": "ERROR",
		}
		if !filter.Matches(attrs1, FilterLevelAttributes) {
			t.Error("Should match first value in IN list in attributes section")
		}

		// Should match second value
		attrs2 := map[string]interface{}{
			"severity": "WARN",
		}
		if !filter.Matches(attrs2, FilterLevelAttributes) {
			t.Error("Should match second value in IN list in attributes section")
		}

		// Should not match
		attrs3 := map[string]interface{}{
			"severity": "INFO",
		}
		if filter.Matches(attrs3, FilterLevelAttributes) {
			t.Error("Should not match value not in IN list in attributes section")
		}

		// Should accept all for other levels when those sections are empty
		if !filter.Matches(attrs1, FilterLevelResource) {
			t.Error("Should accept all signals for resource level when resource section is empty")
		}
		if !filter.Matches(attrs1, FilterLevelScope) {
			t.Error("Should accept all signals for scope level when scope section is empty")
		}
	})

	t.Run("scope section filtering", func(t *testing.T) {
		config := map[string]interface{}{
			"scope": map[string]interface{}{
				"instrumentation.name": map[string]interface{}{
					"op":    "=",
					"value": "opentelemetry-http",
				},
			},
		}
		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse scope filter: %v", err)
		}

		attrs1 := map[string]interface{}{
			"instrumentation.name": "opentelemetry-http",
		}
		if !filter.Matches(attrs1, FilterLevelScope) {
			t.Error("Should match exact instrumentation name in scope section")
		}

		attrs2 := map[string]interface{}{
			"instrumentation.name": "other-instrumentation",
		}
		if filter.Matches(attrs2, FilterLevelScope) {
			t.Error("Should not match different instrumentation name in scope section")
		}

		// Should accept all for other levels when those sections are empty
		if !filter.Matches(attrs1, FilterLevelResource) {
			t.Error("Should accept all signals for resource level when resource section is empty")
		}
		if !filter.Matches(attrs1, FilterLevelAttributes) {
			t.Error("Should accept all signals for attributes level when attributes section is empty")
		}
	})

	t.Run("multiple sections filtering", func(t *testing.T) {
		config := map[string]interface{}{
			"resource": map[string]interface{}{
				"service.name": map[string]interface{}{
					"op":    "=",
					"value": "api-service",
				},
			},
			"attributes": map[string]interface{}{
				"severity": map[string]interface{}{
					"op":    "=",
					"value": "ERROR",
				},
			},
		}
		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse multi-section filter: %v", err)
		}

		// Should match resource section
		attrs1 := map[string]interface{}{
			"service.name": "api-service",
		}
		if !filter.Matches(attrs1, FilterLevelResource) {
			t.Error("Should match resource section")
		}

		// Should match attributes section
		attrs2 := map[string]interface{}{
			"severity": "ERROR",
		}
		if !filter.Matches(attrs2, FilterLevelAttributes) {
			t.Error("Should match attributes section")
		}

		// Should accept all for scope level when scope section is empty
		if !filter.Matches(attrs1, FilterLevelScope) {
			t.Error("Should accept all signals for scope level when scope section is empty")
		}
	})

	t.Run("invalid filter level", func(t *testing.T) {
		config := map[string]interface{}{
			"resource": map[string]interface{}{
				"service.name": map[string]interface{}{
					"op":    "=",
					"value": "test-service",
				},
			},
		}
		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse filter: %v", err)
		}

		attrs := map[string]interface{}{
			"service.name": "test-service",
		}

		if filter.Matches(attrs, "invalid_level") {
			t.Error("Should not match invalid filter level")
		}
	})
}

func TestParseDatabaseSignalFilter(t *testing.T) {
	t.Run("valid hierarchical config", func(t *testing.T) {
		config := map[string]interface{}{
			"resource": map[string]interface{}{
				"service.name": map[string]interface{}{
					"op":    "=",
					"value": "test-service",
				},
			},
			"scope": map[string]interface{}{
				"instrumentation.name": map[string]interface{}{
					"op":    "=",
					"value": "test-instrumentation",
				},
			},
			"attributes": map[string]interface{}{
				"severity": map[string]interface{}{
					"op":    "IN",
					"value": []interface{}{"ERROR", "WARN"},
				},
			},
		}

		filter, err := ParseDatabaseSignalFilter(config)
		if err != nil {
			t.Fatalf("Failed to parse valid config: %v", err)
		}

		if filter == nil {
			t.Error("Filter should not be nil")
		}
	})

	t.Run("invalid config structure", func(t *testing.T) {
		config := map[string]interface{}{
			"resource": "not-a-map",
		}

		_, err := ParseDatabaseSignalFilter(config)
		if err == nil {
			t.Error("Should return error for invalid config structure")
		}
	})

	t.Run("invalid condition format", func(t *testing.T) {
		config := map[string]interface{}{
			"resource": map[string]interface{}{
				"service.name": "not-a-condition",
			},
		}

		_, err := ParseDatabaseSignalFilter(config)
		if err == nil {
			t.Error("Should return error for invalid condition format")
		}
	})
}
