package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemoryMetricsCollector(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Test collecting memory metrics
	collector.CollectMemoryMetrics()

	// Test getting memory stats
	memoryStats := collector.GetMemoryStats()
	assert.NotNil(t, memoryStats)
	assert.Contains(t, memoryStats, "alloc")
	assert.Contains(t, memoryStats, "heap_alloc")
	assert.Contains(t, memoryStats, "sys")
	assert.GreaterOrEqual(t, memoryStats["alloc"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["heap_alloc"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["sys"], int64(0))

	// Test getting runtime stats
	runtimeStats := collector.GetRuntimeStats()
	assert.NotNil(t, runtimeStats)
	assert.Contains(t, runtimeStats, "goroutines")
	assert.Contains(t, runtimeStats, "threads")
	assert.Contains(t, runtimeStats, "cpus")
	assert.GreaterOrEqual(t, runtimeStats["goroutines"], int64(0))
	assert.GreaterOrEqual(t, runtimeStats["threads"], int64(0))
	assert.GreaterOrEqual(t, runtimeStats["cpus"], int64(0))

	// Test getting combined stats
	combinedStats := collector.GetCombinedStats()
	assert.NotNil(t, combinedStats)
	assert.Contains(t, combinedStats, "memory")
	assert.Contains(t, combinedStats, "runtime")
	assert.Contains(t, combinedStats, "timestamp")

	// Verify memory stats in combined stats
	memoryInCombined := combinedStats["memory"].(map[string]int64)
	assert.NotNil(t, memoryInCombined)
	assert.Contains(t, memoryInCombined, "alloc")
	assert.Contains(t, memoryInCombined, "heap_alloc")

	// Verify runtime stats in combined stats
	runtimeInCombined := combinedStats["runtime"].(map[string]int64)
	assert.NotNil(t, runtimeInCombined)
	assert.Contains(t, runtimeInCombined, "goroutines")
	assert.Contains(t, runtimeInCombined, "threads")
	assert.Contains(t, runtimeInCombined, "cpus")

	// Verify timestamp
	timestamp := combinedStats["timestamp"].(int64)
	assert.Greater(t, timestamp, int64(0))
}

func TestMemoryMetricsCollectorPeriodicCollection(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Start periodic collection with a short interval
	stopChan := make(chan struct{})
	done := make(chan bool)

	go func() {
		collector.CollectMemoryMetricsPeriodically(100*time.Millisecond, stopChan)
		done <- true
	}()

	// Let it run for a short time
	time.Sleep(200 * time.Millisecond)

	// Stop the collection
	close(stopChan)

	// Wait for the goroutine to finish
	<-done

	// Test passed if we didn't panic
	assert.True(t, true)
}

func TestMemoryMetricsCollectorMultipleCollections(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics multiple times
	for i := 0; i < 5; i++ {
		collector.CollectMemoryMetrics()

		// Get stats after each collection
		memoryStats := collector.GetMemoryStats()
		assert.NotNil(t, memoryStats)
		assert.Contains(t, memoryStats, "alloc")
		assert.GreaterOrEqual(t, memoryStats["alloc"], int64(0))

		// Small delay between collections
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMemoryMetricsCollectorStatsConsistency(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics
	collector.CollectMemoryMetrics()

	// Get stats multiple times and verify consistency
	stats1 := collector.GetMemoryStats()
	stats2 := collector.GetMemoryStats()

	// The stats should be consistent (same keys)
	assert.Equal(t, len(stats1), len(stats2))

	// Check that all keys exist in both
	for key := range stats1 {
		assert.Contains(t, stats2, key)
	}

	// Runtime stats should also be consistent
	runtime1 := collector.GetRuntimeStats()
	runtime2 := collector.GetRuntimeStats()

	assert.Equal(t, len(runtime1), len(runtime2))
	for key := range runtime1 {
		assert.Contains(t, runtime2, key)
	}
}

func TestMemoryMetricsCollectorGCStats(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics
	collector.CollectMemoryMetrics()

	// Get memory stats
	memoryStats := collector.GetMemoryStats()

	// Check that GC-related stats exist and are reasonable
	assert.Contains(t, memoryStats, "num_gc")
	assert.Contains(t, memoryStats, "num_forced_gc")
	assert.Contains(t, memoryStats, "pause_total_ns")
	assert.Contains(t, memoryStats, "last_gc")

	// GC stats should be non-negative
	assert.GreaterOrEqual(t, memoryStats["num_gc"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["num_forced_gc"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["pause_total_ns"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["last_gc"], int64(0))
}

func TestMemoryMetricsCollectorHeapStats(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics
	collector.CollectMemoryMetrics()

	// Get memory stats
	memoryStats := collector.GetMemoryStats()

	// Check heap-related stats
	heapStats := []string{
		"heap_alloc", "heap_sys", "heap_idle", "heap_inuse",
		"heap_released", "heap_objects",
	}

	for _, stat := range heapStats {
		assert.Contains(t, memoryStats, stat)
		assert.GreaterOrEqual(t, memoryStats[stat], int64(0))
	}

	// Verify heap relationships
	assert.GreaterOrEqual(t, memoryStats["heap_sys"], memoryStats["heap_alloc"])
	assert.GreaterOrEqual(t, memoryStats["heap_sys"], memoryStats["heap_idle"])
	assert.GreaterOrEqual(t, memoryStats["heap_sys"], memoryStats["heap_inuse"])
}

func TestMemoryMetricsCollectorStackStats(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics
	collector.CollectMemoryMetrics()

	// Get memory stats
	memoryStats := collector.GetMemoryStats()

	// Check stack-related stats
	assert.Contains(t, memoryStats, "stack_inuse")
	assert.Contains(t, memoryStats, "stack_sys")
	assert.GreaterOrEqual(t, memoryStats["stack_inuse"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["stack_sys"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["stack_sys"], memoryStats["stack_inuse"])
}

func TestMemoryMetricsCollectorMSpanStats(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics
	collector.CollectMemoryMetrics()

	// Get memory stats
	memoryStats := collector.GetMemoryStats()

	// Check MSpan-related stats
	assert.Contains(t, memoryStats, "mspan_inuse")
	assert.Contains(t, memoryStats, "mspan_sys")
	assert.GreaterOrEqual(t, memoryStats["mspan_inuse"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["mspan_sys"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["mspan_sys"], memoryStats["mspan_inuse"])
}

func TestMemoryMetricsCollectorMCacheStats(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics
	collector.CollectMemoryMetrics()

	// Get memory stats
	memoryStats := collector.GetMemoryStats()

	// Check MCache-related stats
	assert.Contains(t, memoryStats, "mcache_inuse")
	assert.Contains(t, memoryStats, "mcache_sys")
	assert.GreaterOrEqual(t, memoryStats["mcache_inuse"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["mcache_sys"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["mcache_sys"], memoryStats["mcache_inuse"])
}

func TestMemoryMetricsCollectorTimestampAccuracy(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Collect metrics
	collector.CollectMemoryMetrics()

	// Get combined stats
	combinedStats := collector.GetCombinedStats()
	timestamp := combinedStats["timestamp"].(int64)

	// Verify timestamp is recent (within last 5 seconds)
	now := time.Now().Unix()
	assert.GreaterOrEqual(t, now, timestamp)
	assert.LessOrEqual(t, now-timestamp, int64(5))
}

func TestMemoryMetricsCollectorConcurrentAccess(t *testing.T) {
	// Create a memory metrics collector
	collector := NewMemoryMetricsCollector()

	// Test concurrent access to the collector
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			// Collect metrics
			collector.CollectMemoryMetrics()

			// Get stats
			collector.GetMemoryStats()
			collector.GetRuntimeStats()
			collector.GetCombinedStats()

			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Final collection and verification
	collector.CollectMemoryMetrics()
	memoryStats := collector.GetMemoryStats()
	assert.NotNil(t, memoryStats)
	assert.Contains(t, memoryStats, "alloc")
}
