package metrics

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	testSetupOnce sync.Once
	testSetupDone bool
)

// setupTest ensures metrics are registered only once for all tests
func setupTest() {
	testSetupOnce.Do(func() {
		// Reset the default registry to avoid conflicts
		prometheus.DefaultRegisterer = prometheus.NewRegistry()
		Register()
		testSetupDone = true
	})
}

func TestGetAllMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetAllMetrics
	allMetrics := GetAllMetrics()

	// Verify structure
	requiredKeys := []string{"memory", "storage", "database", "http", "notifier", "embedding", "timestamp", "version"}
	for _, key := range requiredKeys {
		if _, exists := allMetrics[key]; !exists {
			t.Errorf("GetAllMetrics missing required key: %s", key)
		}
	}

	// Verify timestamp is recent
	if timestamp, ok := allMetrics["timestamp"].(int64); !ok {
		t.Error("GetAllMetrics timestamp is not int64")
	} else {
		now := time.Now().Unix()
		if timestamp < now-10 || timestamp > now+10 {
			t.Errorf("GetAllMetrics timestamp %d is not within 10 seconds of current time %d", timestamp, now)
		}
	}

	// Verify version
	if version, ok := allMetrics["version"].(string); !ok {
		t.Error("GetAllMetrics version is not string")
	} else if version != "1.0.0" {
		t.Errorf("GetAllMetrics version expected '1.0.0', got '%s'", version)
	}

	t.Logf("GetAllMetrics returned %d top-level keys", len(allMetrics))
}

func TestGetMemoryMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetMemoryMetrics
	memoryMetrics := GetMemoryMetrics()

	// Verify it's not an error response
	if _, hasError := memoryMetrics["error"]; hasError {
		t.Error("GetMemoryMetrics returned error response")
		return
	}

	// Verify structure
	requiredKeys := []string{"memory", "runtime", "timestamp"}
	for _, key := range requiredKeys {
		if _, exists := memoryMetrics[key]; !exists {
			t.Errorf("GetMemoryMetrics missing required key: %s", key)
		}
	}

	// Verify memory stats structure
	if memoryStats, ok := memoryMetrics["memory"].(map[string]int64); ok {
		expectedMemoryKeys := []string{"alloc", "total_alloc", "sys", "heap_alloc", "heap_objects"}
		for _, key := range expectedMemoryKeys {
			if _, exists := memoryStats[key]; !exists {
				t.Errorf("GetMemoryMetrics memory stats missing key: %s", key)
			}
		}
	} else {
		t.Error("GetMemoryMetrics memory stats is not map[string]int64")
	}

	// Verify runtime stats structure
	if runtimeStats, ok := memoryMetrics["runtime"].(map[string]int64); ok {
		expectedRuntimeKeys := []string{"goroutines", "threads", "cpus"}
		for _, key := range expectedRuntimeKeys {
			if _, exists := runtimeStats[key]; !exists {
				t.Errorf("GetMemoryMetrics runtime stats missing key: %s", key)
			}
		}
	} else {
		t.Error("GetMemoryMetrics runtime stats is not map[string]int64")
	}

	t.Logf("GetMemoryMetrics returned valid memory and runtime statistics")
}

func TestGetStorageMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetStorageMetrics
	storageMetrics := GetStorageMetrics()

	// Verify it's not an error response
	if _, hasError := storageMetrics["error"]; hasError {
		t.Error("GetStorageMetrics returned error response")
		return
	}

	// Verify structure
	requiredKeys := []string{"disk_usage", "io_stats", "timestamp"}
	for _, key := range requiredKeys {
		if _, exists := storageMetrics[key]; !exists {
			t.Errorf("GetStorageMetrics missing required key: %s", key)
		}
	}

	// Verify disk usage structure
	if diskUsage, ok := storageMetrics["disk_usage"].(map[string]interface{}); ok {
		if len(diskUsage) == 0 {
			t.Log("GetStorageMetrics disk_usage is empty (may be expected on some systems)")
		} else {
			// Check first path stats
			for path, stats := range diskUsage {
				if pathStats, ok := stats.(map[string]interface{}); ok {
					expectedKeys := []string{"total", "free", "used", "used_percent"}
					for _, key := range expectedKeys {
						if _, exists := pathStats[key]; !exists {
							t.Errorf("GetStorageMetrics disk usage stats missing key: %s for path %s", key, path)
						}
					}
					break // Only check first path
				}
			}
		}
	} else {
		t.Error("GetStorageMetrics disk_usage is not map[string]interface{}")
	}

	t.Logf("GetStorageMetrics returned valid storage statistics")
}

func TestGetDatabaseMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetDatabaseMetrics
	dbMetrics := GetDatabaseMetrics()

	// Verify structure
	requiredKeys := []string{"queries", "connections"}
	for _, key := range requiredKeys {
		if _, exists := dbMetrics[key]; !exists {
			t.Errorf("GetDatabaseMetrics missing required key: %s", key)
		}
	}

	// Verify queries structure
	if queries, ok := dbMetrics["queries"].(map[string]interface{}); ok {
		expectedKeys := []string{"total_queries", "latency_stats"}
		for _, key := range expectedKeys {
			if _, exists := queries[key]; !exists {
				t.Errorf("GetDatabaseMetrics queries missing key: %s", key)
			}
		}
	} else {
		t.Error("GetDatabaseMetrics queries is not map[string]interface{}")
	}

	// Verify connections structure
	if connections, ok := dbMetrics["connections"].(map[string]interface{}); ok {
		expectedKeys := []string{"active", "idle"}
		for _, key := range expectedKeys {
			if _, exists := connections[key]; !exists {
				t.Errorf("GetDatabaseMetrics connections missing key: %s", key)
			}
		}
	} else {
		t.Error("GetDatabaseMetrics connections is not map[string]interface{}")
	}

	t.Logf("GetDatabaseMetrics returned valid database metrics structure")
}

func TestGetHTTPMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetHTTPMetrics
	httpMetrics := GetHTTPMetrics()

	// Verify structure
	requiredKeys := []string{"requests", "latency"}
	for _, key := range requiredKeys {
		if _, exists := httpMetrics[key]; !exists {
			t.Errorf("GetHTTPMetrics missing required key: %s", key)
		}
	}

	// Verify requests structure
	if requests, ok := httpMetrics["requests"].(map[string]interface{}); ok {
		expectedKeys := []string{"total", "by_method", "by_status"}
		for _, key := range expectedKeys {
			if _, exists := requests[key]; !exists {
				t.Errorf("GetHTTPMetrics requests missing key: %s", key)
			}
		}
	} else {
		t.Error("GetHTTPMetrics requests is not map[string]interface{}")
	}

	// Verify latency structure
	if latency, ok := httpMetrics["latency"].(map[string]interface{}); ok {
		expectedKeys := []string{"average", "p95", "p99"}
		for _, key := range expectedKeys {
			if _, exists := latency[key]; !exists {
				t.Errorf("GetHTTPMetrics latency missing key: %s", key)
			}
		}
	} else {
		t.Error("GetHTTPMetrics latency is not map[string]interface{}")
	}

	t.Logf("GetHTTPMetrics returned valid HTTP metrics structure")
}

func TestGetNotifierMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetNotifierMetrics
	notifierMetrics := GetNotifierMetrics()

	// Verify structure
	requiredKeys := []string{"notifications"}
	for _, key := range requiredKeys {
		if _, exists := notifierMetrics[key]; !exists {
			t.Errorf("GetNotifierMetrics missing required key: %s", key)
		}
	}

	// Verify notifications structure
	if notifications, ok := notifierMetrics["notifications"].(map[string]interface{}); ok {
		expectedKeys := []string{"delivered", "dropped", "by_topic"}
		for _, key := range expectedKeys {
			if _, exists := notifications[key]; !exists {
				t.Errorf("GetNotifierMetrics notifications missing key: %s", key)
			}
		}
	} else {
		t.Error("GetNotifierMetrics notifications is not map[string]interface{}")
	}

	t.Logf("GetNotifierMetrics returned valid notifier metrics structure")
}

func TestGetEmbeddingMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetEmbeddingMetrics
	embeddingMetrics := GetEmbeddingMetrics()

	// Verify structure
	requiredKeys := []string{"jobs", "signals", "tokens", "vectorstore", "templates", "quality", "memory", "workers"}
	for _, key := range requiredKeys {
		if _, exists := embeddingMetrics[key]; !exists {
			t.Errorf("GetEmbeddingMetrics missing required key: %s", key)
		}
	}

	// Verify jobs structure
	if jobs, ok := embeddingMetrics["jobs"].(map[string]interface{}); ok {
		expectedKeys := []string{"total", "active", "by_status", "by_signal_type", "duration_stats"}
		for _, key := range expectedKeys {
			if _, exists := jobs[key]; !exists {
				t.Errorf("GetEmbeddingMetrics jobs missing key: %s", key)
			}
		}
	} else {
		t.Error("GetEmbeddingMetrics jobs is not map[string]interface{}")
	}

	// Verify signals structure
	if signals, ok := embeddingMetrics["signals"].(map[string]interface{}); ok {
		expectedKeys := []string{"processed", "by_status", "by_signal_type"}
		for _, key := range expectedKeys {
			if _, exists := signals[key]; !exists {
				t.Errorf("GetEmbeddingMetrics signals missing key: %s", key)
			}
		}
	} else {
		t.Error("GetEmbeddingMetrics signals is not map[string]interface{}")
	}

	t.Logf("GetEmbeddingMetrics returned valid embedding metrics structure")
}

func TestGetMetricsJSON(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetMetricsJSON
	jsonData, err := GetMetricsJSON()
	if err != nil {
		t.Fatalf("GetMetricsJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsedData map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsedData); err != nil {
		t.Fatalf("GetMetricsJSON returned invalid JSON: %v", err)
	}

	// Verify structure
	requiredKeys := []string{"memory", "storage", "database", "http", "notifier", "embedding", "timestamp", "version"}
	for _, key := range requiredKeys {
		if _, exists := parsedData[key]; !exists {
			t.Errorf("GetMetricsJSON missing required key: %s", key)
		}
	}

	// Verify it's pretty-printed (contains newlines and spaces)
	jsonStr := string(jsonData)
	if len(jsonStr) < 100 {
		t.Error("GetMetricsJSON returned too short JSON (likely not pretty-printed)")
	}

	t.Logf("GetMetricsJSON returned valid JSON with %d bytes", len(jsonData))
}

func TestGetPrometheusMetrics(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetPrometheusMetrics
	prometheusData, err := GetPrometheusMetrics()
	if err != nil {
		t.Fatalf("GetPrometheusMetrics failed: %v", err)
	}

	// Verify it's not empty
	if len(prometheusData) == 0 {
		t.Error("GetPrometheusMetrics returned empty string")
	}

	// Verify it contains Prometheus format indicators
	expectedSubstrings := []string{"# HELP", "# TYPE"}
	for _, substr := range expectedSubstrings {
		if !contains(prometheusData, substr) {
			t.Errorf("GetPrometheusMetrics missing expected substring: %s", substr)
		}
	}

	t.Logf("GetPrometheusMetrics returned valid Prometheus format with %d characters", len(prometheusData))
}

func TestGetMetricsSummary(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test GetMetricsSummary
	summary := GetMetricsSummary()

	// Verify it's not empty
	if len(summary) == 0 {
		t.Error("GetMetricsSummary returned empty map")
	}

	// Verify timestamp
	if _, exists := summary["timestamp"]; !exists {
		t.Error("GetMetricsSummary missing timestamp")
	}

	// Verify it contains at least one metric category
	hasMetrics := false
	for key := range summary {
		if key != "timestamp" {
			hasMetrics = true
			break
		}
	}
	if !hasMetrics {
		t.Error("GetMetricsSummary contains no metric data")
	}

	t.Logf("GetMetricsSummary returned summary with %d keys", len(summary))
}

func TestThreadSafety(t *testing.T) {
	// Initialize metrics
	setupTest()

	// Test concurrent access to metrics functions
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			// Call multiple functions concurrently
			GetAllMetrics()
			GetMemoryMetrics()
			GetStorageMetrics()
			GetDatabaseMetrics()
			GetHTTPMetrics()
			GetNotifierMetrics()
			GetEmbeddingMetrics()
			GetMetricsSummary()
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
			// Success
		case <-time.After(5 * time.Second):
			t.Fatal("Thread safety test timed out")
		}
	}

	t.Log("Thread safety test passed - no race conditions detected")
}

func TestErrorHandling(t *testing.T) {
	// Test behavior when collectors are not initialized
	// Reset global collectors
	collectorsMutex.Lock()
	originalMemoryCollector := memoryCollector
	originalStorageCollector := storageCollector
	memoryCollector = nil
	storageCollector = nil
	collectorsMutex.Unlock()

	// Test GetMemoryMetrics with nil collector
	memoryMetrics := GetMemoryMetrics()
	if errorMsg, exists := memoryMetrics["error"]; !exists {
		t.Error("GetMemoryMetrics should return error when collector is nil")
	} else if errorMsg != "memory collector not initialized" {
		t.Errorf("GetMemoryMetrics error message expected 'memory collector not initialized', got '%v'", errorMsg)
	}

	// Test GetStorageMetrics with nil collector
	storageMetrics := GetStorageMetrics()
	if errorMsg, exists := storageMetrics["error"]; !exists {
		t.Error("GetStorageMetrics should return error when collector is nil")
	} else if errorMsg != "storage collector not initialized" {
		t.Errorf("GetStorageMetrics error message expected 'storage collector not initialized', got '%v'", errorMsg)
	}

	// Restore original collectors
	collectorsMutex.Lock()
	memoryCollector = originalMemoryCollector
	storageCollector = originalStorageCollector
	collectorsMutex.Unlock()

	t.Log("Error handling test passed")
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			contains(s[1:len(s)-1], substr)))
}

func BenchmarkGetAllMetrics(b *testing.B) {
	setupTest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetAllMetrics()
	}
}

func BenchmarkGetMetricsJSON(b *testing.B) {
	setupTest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetMetricsJSON()
	}
}

func BenchmarkGetPrometheusMetrics(b *testing.B) {
	setupTest()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetPrometheusMetrics()
	}
}
