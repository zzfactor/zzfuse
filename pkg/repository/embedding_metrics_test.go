package repository

import (
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestEmbeddingMetricsCollector(t *testing.T) {
	// Create a new metrics collector
	collector := NewEmbeddingMetricsCollector(
		1,          // configID
		"spans",    // signalType
		"batch",    // processingMode
		"openai",   // embeddingProvider
		"pgvector", // vectorStoreProvider
		"worker-1", // workerID
	)

	// Test job start
	stats := collector.RecordJobStart("worker-1", 10, 100)
	assert.NotNil(t, stats)
	assert.Equal(t, "worker-1", stats.WorkerID)
	assert.Equal(t, 10, stats.SignalCount)
	assert.Equal(t, 100, stats.TokenCount)
	assert.False(t, stats.StartTime.IsZero())

	// Test job completion with success
	details := &EmbeddingJobDetails{
		FetchTime:          100 * time.Millisecond,
		EmbeddingTime:      500 * time.Millisecond,
		StoreTime:          200 * time.Millisecond,
		TemplateTime:       50 * time.Millisecond,
		ErrorCount:         0,
		RetryCount:         0,
		MemoryUsage:        1024 * 1024, // 1MB
		VectorStoreWrites:  10,
		VectorStoreErrors:  0,
		VectorStoreLatency: 200 * time.Millisecond,
		TemplateRenders:    10,
		TemplateErrors:     0,
		TotalTemplateTime:  50 * time.Millisecond,
		QualityScore:       0.95,
		TruncatedTokens:    0,
	}

	collector.RecordJobCompletion(stats, true, details)

	// Test job completion with failure
	stats2 := collector.RecordJobStart("worker-1", 5, 50)
	failureDetails := &EmbeddingJobDetails{
		FetchTime:          50 * time.Millisecond,
		EmbeddingTime:      0,
		StoreTime:          0,
		TemplateTime:       0,
		ErrorCount:         1,
		RetryCount:         2,
		MemoryUsage:        512 * 1024, // 512KB
		VectorStoreWrites:  0,
		VectorStoreErrors:  1,
		VectorStoreLatency: 0,
		TemplateRenders:    0,
		TemplateErrors:     1,
		TotalTemplateTime:  0,
		QualityScore:       0.0,
		TruncatedTokens:    0,
	}

	collector.RecordJobCompletion(stats2, false, failureDetails)

	// Test additional metric recording methods
	collector.RecordEmbeddingQuality(0.85)
	collector.RecordTokenTruncation(100, 80)
	collector.RecordSignalSkipped("empty_content")
	collector.RecordBatchProcessed(true, 15)
	collector.RecordThroughput(25.5)
	collector.RecordError("timeout")
	collector.RecordRetry()
	collector.RecordVectorStoreWrite(true, 150*time.Millisecond)
	collector.RecordTemplateRender(true, 25*time.Millisecond)
	collector.RecordMemoryUsage("peak", 2048*1024)

	// Test job cancellation
	collector.RecordJobCancellation("worker-1")

	// Test metrics retrieval
	metrics := collector.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(1), metrics.ConfigID)
	assert.Equal(t, "spans", metrics.SignalType)
	assert.Equal(t, "batch", metrics.ProcessingMode)
	assert.Equal(t, "openai", metrics.EmbeddingProvider)
	assert.Equal(t, "pgvector", metrics.VectorStoreProvider)

	// Test logging (should not panic)
	collector.LogMetrics(nil)

	// Test reset (should not panic)
	collector.ResetMetrics()
}

func TestMemoryTracking(t *testing.T) {
	// Create a new metrics collector
	collector := NewEmbeddingMetricsCollector(
		1,          // configID
		"spans",    // signalType
		"batch",    // processingMode
		"openai",   // embeddingProvider
		"pgvector", // vectorStoreProvider
		"worker-1", // workerID
	)

	// Test basic memory recording
	collector.RecordMemoryUsage("current", 1024*1024) // 1MB
	collector.RecordMemoryUsage("peak", 2048*1024)    // 2MB

	// Test embedding-specific memory recording
	collector.RecordEmbeddingMemoryUsage(1024 * 1024) // 1MB

	// Test metrics retrieval
	metrics := collector.GetMetrics()
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(1), metrics.ConfigID)
	assert.Equal(t, "spans", metrics.SignalType)
}

func TestWorkerMemoryTracking(t *testing.T) {
	// Create a mock embedding worker
	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			WorkerID: "test-worker",
			Logger:   util.NoopLogger{},
		},
	}

	// Test basic memory usage
	memoryUsage := worker.getCurrentMemoryUsage()
	assert.GreaterOrEqual(t, memoryUsage, int64(0), "Memory usage should be non-negative")

	// Test comprehensive memory stats
	memoryStats := worker.GetMemoryStats()
	assert.NotNil(t, memoryStats)
	assert.Contains(t, memoryStats, "alloc")
	assert.Contains(t, memoryStats, "heap_alloc")
	assert.Contains(t, memoryStats, "sys")
	assert.GreaterOrEqual(t, memoryStats["alloc"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["heap_alloc"], int64(0))
	assert.GreaterOrEqual(t, memoryStats["sys"], int64(0))

	// Test human-readable memory summary
	memorySummary := worker.GetMemoryUsageSummary()
	assert.NotNil(t, memorySummary)
	assert.Contains(t, memorySummary, "alloc")
	assert.Contains(t, memorySummary, "heap_alloc")
	assert.Contains(t, memorySummary, "heap_objects")
	assert.Contains(t, memorySummary, "num_gc")

	// Verify that summary values are human-readable strings
	assert.NotEmpty(t, memorySummary["alloc"])
	assert.NotEmpty(t, memorySummary["heap_alloc"])
	assert.NotEmpty(t, memorySummary["heap_objects"])
	assert.NotEmpty(t, memorySummary["num_gc"])
}

func TestEmbeddingJobStats(t *testing.T) {
	stats := &EmbeddingJobStats{
		StartTime:   time.Now(),
		SignalCount: 10,
		TokenCount:  100,
		WorkerID:    "test-worker",
	}

	assert.NotNil(t, stats)
	assert.Equal(t, 10, stats.SignalCount)
	assert.Equal(t, 100, stats.TokenCount)
	assert.Equal(t, "test-worker", stats.WorkerID)
}

func TestEmbeddingJobDetails(t *testing.T) {
	details := &EmbeddingJobDetails{
		FetchTime:          100 * time.Millisecond,
		EmbeddingTime:      500 * time.Millisecond,
		StoreTime:          200 * time.Millisecond,
		TemplateTime:       50 * time.Millisecond,
		ErrorCount:         0,
		RetryCount:         0,
		Timeout:            false,
		RateLimited:        false,
		MemoryUsage:        1024 * 1024,
		VectorStoreWrites:  10,
		VectorStoreErrors:  0,
		VectorStoreLatency: 200 * time.Millisecond,
		TemplateRenders:    10,
		TemplateErrors:     0,
		TotalTemplateTime:  50 * time.Millisecond,
		QualityScore:       0.95,
		TruncatedTokens:    0,
	}

	assert.NotNil(t, details)
	assert.Equal(t, 100*time.Millisecond, details.FetchTime)
	assert.Equal(t, 500*time.Millisecond, details.EmbeddingTime)
	assert.Equal(t, 200*time.Millisecond, details.StoreTime)
	assert.Equal(t, 50*time.Millisecond, details.TemplateTime)
	assert.Equal(t, 0, details.ErrorCount)
	assert.Equal(t, 0, details.RetryCount)
	assert.False(t, details.Timeout)
	assert.False(t, details.RateLimited)
	assert.Equal(t, int64(1024*1024), details.MemoryUsage)
	assert.Equal(t, 10, details.VectorStoreWrites)
	assert.Equal(t, 0, details.VectorStoreErrors)
	assert.Equal(t, 200*time.Millisecond, details.VectorStoreLatency)
	assert.Equal(t, 10, details.TemplateRenders)
	assert.Equal(t, 0, details.TemplateErrors)
	assert.Equal(t, 50*time.Millisecond, details.TotalTemplateTime)
	assert.Equal(t, 0.95, details.QualityScore)
	assert.Equal(t, 0, details.TruncatedTokens)
}
