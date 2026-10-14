package repository

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"go.uber.org/zap"
)

// Simple test to verify priority-based routing behavior
func TestDatabaseRoutingSchema_PriorityBehavior(t *testing.T) {
	// Test the priority sorting logic using sort.Slice directly
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			ID:             1,
			Name:           "low-priority",
			DatabaseSchema: "low",
			Priority:       1,
		},
		{
			ID:             2,
			Name:           "high-priority",
			DatabaseSchema: "high",
			Priority:       10,
		},
		{
			ID:             3,
			Name:           "medium-priority",
			DatabaseSchema: "medium",
			Priority:       5,
		},
	}

	// Test the sorting logic (same as used in Initialize method)
	sort.Slice(schemaConfigs, func(i, j int) bool {
		return schemaConfigs[i].Priority < schemaConfigs[j].Priority
	})

	// Verify they are sorted by priority (lowest priority number first)
	if schemaConfigs[0].Priority != 1 {
		t.Errorf("Expected first config priority=1, got %d", schemaConfigs[0].Priority)
	}
	if schemaConfigs[1].Priority != 5 {
		t.Errorf("Expected second config priority=5, got %d", schemaConfigs[1].Priority)
	}
	if schemaConfigs[2].Priority != 10 {
		t.Errorf("Expected third config priority=10, got %d", schemaConfigs[2].Priority)
	}

	// Verify names are in correct order
	if schemaConfigs[0].Name != "low-priority" {
		t.Errorf("Expected first config name=low-priority, got %s", schemaConfigs[0].Name)
	}
	if schemaConfigs[1].Name != "medium-priority" {
		t.Errorf("Expected second config name=medium-priority, got %s", schemaConfigs[1].Name)
	}
	if schemaConfigs[2].Name != "high-priority" {
		t.Errorf("Expected third config name=high-priority, got %s", schemaConfigs[2].Name)
	}
}

// Test the cleanRoutingValue function
func TestCleanRoutingValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  test-service  ", "test-service"},
		{"", ""},
		{"   ", ""},
		{"no-spaces", "no-spaces"},
		{"\t\ntest\t\n", "test"},
	}

	// Create a minimal routing schema to test the method
	drs := &DatabaseRoutingSchema{}

	for _, tt := range tests {
		result := drs.cleanRoutingValue(tt.input)
		if result != tt.expected {
			t.Errorf("cleanRoutingValue(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

// Test namespace-first routing priority
func TestNamespaceFirstPriority(t *testing.T) {
	// Test that when both service name and namespace are specified,
	// namespace takes priority over service name

	// Create schema configurations with namespace having higher priority
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       1,
			DatabaseSchema: "production_schema",
			Routing:        map[string]interface{}{"service_namespace": "production"},
		},
		{
			Priority:       2,
			DatabaseSchema: "service_schema",
			Routing:        map[string]interface{}{"service_name": "my-service"},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	// Test cases
	tests := []struct {
		name             string
		serviceName      string
		serviceNamespace string
		expectedSchema   string
		expectedFound    bool
		description      string
	}{
		{
			name:             "namespace_priority",
			serviceName:      "my-service",
			serviceNamespace: "production",
			expectedSchema:   "production_schema",
			expectedFound:    true,
			description:      "Should return namespace schema when both service name and namespace match",
		},
		{
			name:             "service_name_only",
			serviceName:      "my-service",
			serviceNamespace: "",
			expectedSchema:   "service_schema",
			expectedFound:    true,
			description:      "Should return service name schema when only service name matches",
		},
		{
			name:             "namespace_only",
			serviceName:      "",
			serviceNamespace: "production",
			expectedSchema:   "production_schema",
			expectedFound:    true,
			description:      "Should return namespace schema when only namespace matches",
		},
		{
			name:             "no_match",
			serviceName:      "other-service",
			serviceNamespace: "staging",
			expectedSchema:   "",
			expectedFound:    false,
			description:      "Should return false when neither matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, found := drs.GetSchema(tt.serviceName, tt.serviceNamespace)
			if found != tt.expectedFound {
				t.Errorf("Expected found=%v, got found=%v for %s", tt.expectedFound, found, tt.description)
			}
			if found && schema != tt.expectedSchema {
				t.Errorf("Expected schema=%s, got schema=%s for %s", tt.expectedSchema, schema, tt.description)
			}
		})
	}
}

// Test catch-all priority behavior with priority 9999
func TestCatchAllPriority(t *testing.T) {
	// Test that catch-all with priority 9999 is used as final fallback
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			ID:             1,
			Name:           "high-priority-exact",
			DatabaseSchema: "high_exact",
			Priority:       1,
			Routing: map[string]interface{}{
				"service_name": "specific-service",
			},
		},
		{
			ID:             2,
			Name:           "catch-all-early",
			DatabaseSchema: "early_catchall",
			Priority:       5,
			Routing:        map[string]interface{}{}, // Empty routing = catch-all
		},
		{
			ID:             3,
			Name:           "catch-all-final",
			DatabaseSchema: "final_catchall",
			Priority:       9999, // Final fallback
			Routing:        nil,  // Null routing = catch-all
		},
	}

	// Sort by priority (lowest number first)
	sort.Slice(schemaConfigs, func(i, j int) bool {
		return schemaConfigs[i].Priority < schemaConfigs[j].Priority
	})

	// Simulate the catch-all logic from buildRoutingIndex
	var catchAllSchema string
	hasCatchAll := false

	for _, config := range schemaConfigs {
		if config.Routing == nil || len(config.Routing) == 0 {
			// Always update catch-all - later ones (higher priority numbers) override
			catchAllSchema = config.DatabaseSchema
			hasCatchAll = true
		}
	}

	// Verify that the final catch-all (priority 9999) is used
	if !hasCatchAll {
		t.Error("Expected to have catch-all schema")
	}
	if catchAllSchema != "final_catchall" {
		t.Errorf("Expected final_catchall, got %s - catch-all with priority 9999 should be final fallback", catchAllSchema)
	}
}

// Test comprehensive service name and namespace filtering permutations
func TestServiceNameNamespacePermutations(t *testing.T) {
	// Create schema configurations for testing
	schemaConfigs := []*database.DatabaseSchemaConfig{
		// Priority 1: Exact namespace matches
		{
			Priority:       1,
			DatabaseSchema: "prod_schema",
			Routing:        map[string]interface{}{"service_namespace": "production"},
		},
		{
			Priority:       2,
			DatabaseSchema: "staging_schema",
			Routing:        map[string]interface{}{"service_namespace": "staging"},
		},
		// Priority 3: Exact service name matches
		{
			Priority:       3,
			DatabaseSchema: "gateway_schema",
			Routing:        map[string]interface{}{"service_name": "api-gateway"},
		},
		{
			Priority:       4,
			DatabaseSchema: "user_schema",
			Routing:        map[string]interface{}{"service_name": "user-service"},
		},
		// Priority 5: Namespace patterns
		{
			Priority:       5,
			DatabaseSchema: "env_pattern_schema",
			Routing:        map[string]interface{}{"service_namespace_prefix": "env-"},
		},
		// Priority 6: Service name patterns
		{
			Priority:       6,
			DatabaseSchema: "api_pattern_schema",
			Routing:        map[string]interface{}{"service_name_prefix": "api-"},
		},
		{
			Priority:       7,
			DatabaseSchema: "service_pattern_schema",
			Routing:        map[string]interface{}{"service_name_suffix": "-service"},
		},
		// Priority 8: Wildcard
		{
			Priority:       8,
			DatabaseSchema: "wildcard_schema",
			Routing:        map[string]interface{}{"service_name": "*"},
		},
		// Priority 9999: Catch-all
		{
			Priority:       9999,
			DatabaseSchema: "catchall_schema",
			Routing:        map[string]interface{}{},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	tests := []struct {
		name             string
		serviceName      string
		serviceNamespace string
		expectedSchema   string
		expectedFound    bool
		description      string
	}{
		// Exact namespace matches (highest priority)
		{
			name:             "exact_namespace_production",
			serviceName:      "any-service",
			serviceNamespace: "production",
			expectedSchema:   "prod_schema",
			expectedFound:    true,
			description:      "Exact namespace match should take priority over service name",
		},
		{
			name:             "exact_namespace_staging",
			serviceName:      "api-gateway", // This has exact service match but namespace wins
			serviceNamespace: "staging",
			expectedSchema:   "staging_schema",
			expectedFound:    true,
			description:      "Exact namespace should override exact service name",
		},

		// Exact service name matches (when no namespace match)
		{
			name:             "exact_service_api_gateway",
			serviceName:      "api-gateway",
			serviceNamespace: "unknown-ns",
			expectedSchema:   "gateway_schema",
			expectedFound:    true,
			description:      "Exact service name match when namespace doesn't match",
		},
		{
			name:             "exact_service_user_service",
			serviceName:      "user-service",
			serviceNamespace: "",
			expectedSchema:   "user_schema",
			expectedFound:    true,
			description:      "Exact service name match with empty namespace",
		},

		// Namespace pattern matches
		{
			name:             "namespace_pattern_env_dev",
			serviceName:      "some-service",
			serviceNamespace: "env-dev",
			expectedSchema:   "env_pattern_schema",
			expectedFound:    true,
			description:      "Namespace pattern match should work",
		},
		{
			name:             "namespace_pattern_priority",
			serviceName:      "api-test", // This would match service pattern but namespace pattern wins
			serviceNamespace: "env-test",
			expectedSchema:   "env_pattern_schema",
			expectedFound:    true,
			description:      "Namespace pattern should take priority over service pattern",
		},

		// Service name pattern matches
		{
			name:             "service_pattern_api_prefix",
			serviceName:      "api-orders",
			serviceNamespace: "unknown-ns",
			expectedSchema:   "api_pattern_schema",
			expectedFound:    true,
			description:      "Service name prefix pattern match",
		},
		{
			name:             "service_pattern_suffix",
			serviceName:      "payment-service",
			serviceNamespace: "",
			expectedSchema:   "service_pattern_schema",
			expectedFound:    true,
			description:      "Service name suffix pattern match",
		},

		// Wildcard fallback
		{
			name:             "wildcard_fallback",
			serviceName:      "unknown-service",
			serviceNamespace: "unknown-namespace",
			expectedSchema:   "service_pattern_schema", // "-service" suffix matches
			expectedFound:    true,
			description:      "Service pattern matches before wildcard fallback",
		},

		// Catch-all fallback (when wildcard disabled)
		{
			name:             "empty_inputs",
			serviceName:      "",
			serviceNamespace: "",
			expectedSchema:   "catchall_schema",
			expectedFound:    true,
			description:      "Should fallback to catch-all for empty inputs",
		},

		// Mixed scenarios
		{
			name:             "both_exact_namespace_wins",
			serviceName:      "api-gateway", // Exact service match
			serviceNamespace: "production",  // Exact namespace match
			expectedSchema:   "prod_schema", // Namespace should win
			expectedFound:    true,
			description:      "When both service and namespace have exact matches, namespace wins",
		},
		{
			name:             "service_exact_namespace_pattern",
			serviceName:      "user-service", // Exact service match
			serviceNamespace: "env-prod",     // Namespace pattern match
			expectedSchema:   "user_schema",  // Exact service match wins (priority 4 vs 5)
			expectedFound:    true,
			description:      "Exact service match beats namespace pattern when both specified",
		},

		// True wildcard fallback test
		{
			name:             "true_wildcard_fallback",
			serviceName:      "nomatch",
			serviceNamespace: "nomatch",
			expectedSchema:   "wildcard_schema",
			expectedFound:    true,
			description:      "Should fallback to wildcard when no patterns match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, found := drs.GetSchema(tt.serviceName, tt.serviceNamespace)
			if found != tt.expectedFound {
				t.Errorf("Expected found=%v, got found=%v for %s", tt.expectedFound, found, tt.description)
			}
			if found && schema != tt.expectedSchema {
				t.Errorf("Expected schema=%s, got schema=%s for %s", tt.expectedSchema, schema, tt.description)
			}
		})
	}
}

// Test priority ordering with mixed service name and namespace rules
func TestMixedPriorityOrdering(t *testing.T) {
	// Test that priority ordering works correctly across different rule types
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			ID:             1,
			Name:           "high-priority-service",
			DatabaseSchema: "high_service_schema",
			Priority:       1, // Highest priority
			Routing: map[string]interface{}{
				"service_name": "test-service",
			},
		},
		{
			ID:             2,
			Name:           "medium-priority-namespace",
			DatabaseSchema: "medium_namespace_schema",
			Priority:       5,
			Routing:        map[string]interface{}{"service_namespace": "test-namespace"},
		},
		{
			Priority:       10,
			DatabaseSchema: "low_pattern_schema",
			Routing:        map[string]interface{}{"service_name_prefix": "test-"},
		},
		{
			Priority:       9999,
			DatabaseSchema: "catchall_schema",
			Routing:        map[string]interface{}{},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	// Test that priority 1 service name beats priority 5 namespace
	schema, found := drs.GetSchema("test-service", "test-namespace")
	if !found {
		t.Error("Expected to find schema")
	}
	if schema != "high_service_schema" {
		t.Errorf("Expected high_service_schema (priority 1 beats priority 5), got %s", schema)
	}

	// Test exact service name match only
	schema, found = drs.GetSchema("test-service", "")
	if !found {
		t.Error("Expected to find schema")
	}
	if schema != "high_service_schema" {
		t.Errorf("Expected high_service_schema (exact service match), got %s", schema)
	}

	// Test pattern match
	schema, found = drs.GetSchema("test-other", "")
	if !found {
		t.Error("Expected to find schema")
	}
	if schema != "low_pattern_schema" {
		t.Errorf("Expected low_pattern_schema (pattern match), got %s", schema)
	}

	// Test catch-all fallback
	schema, found = drs.GetSchema("nomatch", "nomatch")
	if !found {
		t.Error("Expected to find schema")
	}
	if schema != "catchall_schema" {
		t.Errorf("Expected catchall_schema (catch-all fallback), got %s", schema)
	}
}

// Test cache performance and hit/miss behavior
func TestCachePerformance(t *testing.T) {
	// Create schema configurations for testing
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       1,
			DatabaseSchema: "prod_schema",
			Routing:        map[string]interface{}{"service_namespace": "production"},
		},
		{
			Priority:       2,
			DatabaseSchema: "user_schema",
			Routing:        map[string]interface{}{"service_name": "user-service"},
		},
		{
			Priority:       9999,
			DatabaseSchema: "catchall_schema",
			Routing:        map[string]interface{}{},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	// Test cache hit
	serviceName := "user-service"
	serviceNamespace := "production"

	// First call - should miss cache and populate it
	schema1, found1 := drs.GetSchema(serviceName, serviceNamespace)
	if !found1 {
		t.Error("Expected to find schema")
	}
	if schema1 != "prod_schema" {
		t.Errorf("Expected prod_schema, got %s", schema1)
	}

	// Second call - should hit cache
	schema2, found2 := drs.GetSchema(serviceName, serviceNamespace)
	if !found2 {
		t.Error("Expected to find schema from cache")
	}
	if schema2 != schema1 {
		t.Errorf("Cache returned different result: %s vs %s", schema1, schema2)
	}

	// Test cache with different inputs
	schema3, found3 := drs.GetSchema("user-service", "")
	if !found3 {
		t.Error("Expected to find schema")
	}
	if schema3 != "user_schema" {
		t.Errorf("Expected user_schema, got %s", schema3)
	}

	// Test cache miss for non-existent service
	schema4, found4 := drs.GetSchema("nonexistent", "nonexistent")
	if !found4 {
		t.Error("Expected to find catchall schema")
	}
	if schema4 != "catchall_schema" {
		t.Errorf("Expected catchall_schema, got %s", schema4)
	}
}

// Test that the sync.Map cache retains entries until configuration invalidation.
func TestCacheRetainsEntriesUntilReset(t *testing.T) {
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       9999,
			DatabaseSchema: "catchall_schema",
			Routing:        map[string]interface{}{},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	// Populate more than the obsolete circular cache capacity of sixteen entries.
	for i := 0; i < 20; i++ {
		serviceName := "service-" + string(rune('a'+i))
		serviceNamespace := "namespace-" + string(rune('a'+i))

		schema, found := drs.GetSchema(serviceName, serviceNamespace)
		if !found {
			t.Errorf("Expected to find schema for iteration %d", i)
		}
		if schema != "catchall_schema" {
			t.Errorf("Expected catchall_schema, got %s for iteration %d", schema, i)
		}
	}

	// The sync.Map cache retains both early and recent entries.
	firstSchema, found, hit := drs.checkCache("service-a", "namespace-a")
	if !hit || !found || firstSchema != "catchall_schema" {
		t.Error("Expected the first entry to remain cached")
	}

	// Recent entries are retained too.
	schema, _, hit := drs.checkCache("service-t", "namespace-t")
	if !hit {
		t.Error("Expected cache hit for recent entry")
	}
	if schema != "catchall_schema" {
		t.Errorf("Expected catchall_schema from cache, got %s", schema)
	}
	drs.clearCache()
	for _, suffix := range []string{"a", "t"} {
		if _, _, hit := drs.checkCache("service-"+suffix, "namespace-"+suffix); hit {
			t.Error("Expected configuration invalidation to clear every cached entry")
		}
	}
}

// Test cache clearing functionality
func TestCacheClearOnReset(t *testing.T) {
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       1,
			DatabaseSchema: "test_schema",
			Routing:        map[string]interface{}{"service_name": "test-service"},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	// Populate cache
	schema, found := drs.GetSchema("test-service", "")
	if !found || schema != "test_schema" {
		t.Error("Failed to populate cache")
	}

	// Verify cache hit
	_, _, hit := drs.checkCache("test-service", "")
	if !hit {
		t.Error("Expected cache hit before reset")
	}

	// Clear cache manually (simulating reset)
	drs.clearCache()

	// Verify cache miss after clear
	_, _, hit = drs.checkCache("test-service", "")
	if hit {
		t.Error("Expected cache miss after clear")
	}
}

// =========================
// Concurrent Race Condition Tests
// =========================
// These tests validate the thread-safety of the schema routing cache.
// Run with: go test -race -run TestConcurrent

// TestConcurrentCacheAccess tests concurrent read/write access to the cache.
// This validates the sync.Map-based cache is thread-safe.
func TestConcurrentCacheAccess(t *testing.T) {
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       1,
			DatabaseSchema: "prod_schema",
			Routing:        map[string]interface{}{"service_namespace": "production"},
		},
		{
			Priority:       2,
			DatabaseSchema: "staging_schema",
			Routing:        map[string]interface{}{"service_namespace": "staging"},
		},
		{
			Priority:       9999,
			DatabaseSchema: "catchall_schema",
			Routing:        map[string]interface{}{},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	// Run concurrent goroutines accessing the cache
	const numGoroutines = 100
	const numIterations = 1000

	var wg sync.WaitGroup
	var successCount int64
	var errorCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				// Alternate between different service/namespace combinations
				var serviceName, serviceNamespace, expectedSchema string
				switch j % 4 {
				case 0:
					serviceName = "service-a"
					serviceNamespace = "production"
					expectedSchema = "prod_schema"
				case 1:
					serviceName = "service-b"
					serviceNamespace = "staging"
					expectedSchema = "staging_schema"
				case 2:
					serviceName = fmt.Sprintf("dynamic-service-%d-%d", routineID, j)
					serviceNamespace = "production"
					expectedSchema = "prod_schema"
				case 3:
					serviceName = "unknown"
					serviceNamespace = "unknown"
					expectedSchema = "catchall_schema"
				}

				schema, found := drs.GetSchema(serviceName, serviceNamespace)
				if !found {
					atomic.AddInt64(&errorCount, 1)
					continue
				}
				if schema == expectedSchema {
					atomic.AddInt64(&successCount, 1)
				} else {
					atomic.AddInt64(&errorCount, 1)
					t.Errorf("Goroutine %d: Expected %s, got %s for service=%s, namespace=%s",
						routineID, expectedSchema, schema, serviceName, serviceNamespace)
				}
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Concurrent cache test completed: %d successes, %d errors", successCount, errorCount)

	if errorCount > 0 {
		t.Errorf("Expected 0 errors, got %d", errorCount)
	}
}

// TestConcurrentCacheWriteAndClear tests concurrent writes with cache clearing.
// This validates there are no race conditions when the cache is cleared during reads.
func TestConcurrentCacheWriteAndClear(t *testing.T) {
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       1,
			DatabaseSchema: "test_schema",
			Routing:        map[string]interface{}{"service_name": "test-service"},
		},
		{
			Priority:       9999,
			DatabaseSchema: "catchall_schema",
			Routing:        map[string]interface{}{},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	const numReaders = 50
	const numClearers = 5
	const duration = 100 // iterations

	var wg sync.WaitGroup
	var readCount int64
	var clearCount int64

	// Start reader goroutines
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()
			for j := 0; j < duration; j++ {
				serviceName := fmt.Sprintf("service-%d", j%10)
				_, found := drs.GetSchema(serviceName, "")
				if found {
					atomic.AddInt64(&readCount, 1)
				}
			}
		}(i)
	}

	// Start clearer goroutines
	for i := 0; i < numClearers; i++ {
		wg.Add(1)
		go func(clearerID int) {
			defer wg.Done()
			for j := 0; j < duration/10; j++ {
				drs.clearCache()
				atomic.AddInt64(&clearCount, 1)
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Concurrent clear test completed: %d reads, %d clears", readCount, clearCount)

	// No panics or data races means success
}

// TestConcurrentSchemaLookupConsistency tests that concurrent lookups always return consistent results.
func TestConcurrentSchemaLookupConsistency(t *testing.T) {
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       1,
			DatabaseSchema: "schema_a",
			Routing:        map[string]interface{}{"service_name": "service-a"},
		},
		{
			Priority:       2,
			DatabaseSchema: "schema_b",
			Routing:        map[string]interface{}{"service_name": "service-b"},
		},
		{
			Priority:       3,
			DatabaseSchema: "schema_c",
			Routing:        map[string]interface{}{"service_name": "service-c"},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	const numGoroutines = 100
	const numIterations = 500

	// Map to track inconsistencies
	type result struct {
		schema string
		found  bool
	}
	results := make(map[string][]result)
	var mu sync.Mutex

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				for _, serviceName := range []string{"service-a", "service-b", "service-c"} {
					schema, found := drs.GetSchema(serviceName, "")

					mu.Lock()
					key := serviceName
					results[key] = append(results[key], result{schema: schema, found: found})
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	// Verify consistency: all results for the same service should be identical
	for serviceName, resultList := range results {
		if len(resultList) == 0 {
			continue
		}

		expectedSchema := resultList[0].schema
		expectedFound := resultList[0].found

		for i, r := range resultList {
			if r.schema != expectedSchema || r.found != expectedFound {
				t.Errorf("Inconsistent result for %s at index %d: expected (schema=%s, found=%v), got (schema=%s, found=%v)",
					serviceName, i, expectedSchema, expectedFound, r.schema, r.found)
				break // Only report first inconsistency
			}
		}
	}

	t.Logf("Consistency test completed: checked %d service lookups", len(results))
}

// TestConcurrentCacheKeyCollision tests cache behavior with keys that might collide.
func TestConcurrentCacheKeyCollision(t *testing.T) {
	schemaConfigs := []*database.DatabaseSchemaConfig{
		{
			Priority:       9999,
			DatabaseSchema: "catchall_schema",
			Routing:        map[string]interface{}{},
		},
	}

	logger, _ := zap.NewDevelopment()
	drs := &DatabaseRoutingSchema{
		schemaConfigs: schemaConfigs,
		logger:        logger,
	}

	const numGoroutines = 50
	const numIterations = 200

	var wg sync.WaitGroup

	// Test with similar keys that differ only in separator position
	testCases := []struct {
		serviceName      string
		serviceNamespace string
	}{
		{"service", "namespace"},
		{"service\x00", "namespace"}, // Different key due to null separator
		{"", "service\x00namespace"},
		{"service\x00namespace", ""},
	}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < numIterations; j++ {
				for _, tc := range testCases {
					schema, found := drs.GetSchema(tc.serviceName, tc.serviceNamespace)
					if !found {
						t.Errorf("Expected to find schema for service=%q, namespace=%q", tc.serviceName, tc.serviceNamespace)
					}
					if schema != "catchall_schema" {
						t.Errorf("Expected catchall_schema, got %s", schema)
					}
				}
			}
		}()
	}

	wg.Wait()
}
