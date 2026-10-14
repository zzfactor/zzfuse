package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStorageMetricsCollector(t *testing.T) {
	// Create a storage metrics collector
	collector := NewStorageMetricsCollector([]string{"/"})

	// Test collecting disk usage metrics
	collector.CollectStorageMetrics()

	// Test collecting I/O stats
	collector.CollectIoStats()

	// Test collecting all metrics
	collector.CollectAllStorageMetrics()

	// Test getting disk usage stats
	diskStats := collector.GetDiskUsageStats()
	assert.NotNil(t, diskStats)

	// Check if we have stats for the root path
	if rootStats, exists := diskStats["/"]; exists {
		stats := rootStats.(map[string]interface{})
		assert.Contains(t, stats, "total")
		assert.Contains(t, stats, "free")
		assert.Contains(t, stats, "used")
		assert.Contains(t, stats, "used_percent")
	}

	// Test getting I/O stats
	ioStats := collector.GetIoStats()
	assert.NotNil(t, ioStats)

	// Test getting combined stats
	combinedStats := collector.GetStorageStats()
	assert.NotNil(t, combinedStats)
	assert.Contains(t, combinedStats, "disk_usage")
	assert.Contains(t, combinedStats, "io_stats")
	assert.Contains(t, combinedStats, "timestamp")
}

func TestStorageMetricsCollectorWithMultiplePaths(t *testing.T) {
	// Test with multiple paths
	paths := []string{"/", "/tmp", "/var"}
	collector := NewStorageMetricsCollector(paths)

	// Test collecting metrics
	collector.CollectStorageMetrics()

	// Test getting stats
	diskStats := collector.GetDiskUsageStats()
	assert.NotNil(t, diskStats)

	// Check that we have stats for at least one path
	assert.Greater(t, len(diskStats), 0)
}

func TestStorageMetricsCollectorDefaultPath(t *testing.T) {
	// Test with no paths (should default to "/")
	collector := NewStorageMetricsCollector([]string{})

	// Test collecting metrics
	collector.CollectStorageMetrics()

	// Test getting stats
	diskStats := collector.GetDiskUsageStats()
	assert.NotNil(t, diskStats)

	// Should have stats for root path
	assert.Contains(t, diskStats, "/")
}

func TestStorageMetricsPeriodicCollection(t *testing.T) {
	// Create a storage metrics collector
	collector := NewStorageMetricsCollector([]string{"/"})

	// Start periodic collection with a short interval
	stopChan := make(chan struct{})
	done := make(chan bool)

	go func() {
		collector.CollectStorageMetricsPeriodically(100*time.Millisecond, stopChan)
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
