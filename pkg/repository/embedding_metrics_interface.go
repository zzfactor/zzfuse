package repository

import (
	"context"
	"sync"
	"time"

	"github.com/medxops/trazr-otel/pkg/metrics"
)

// EmbeddingMetricsCollector provides a simple interface for recording embedding metrics
type EmbeddingMetricsCollector struct {
	configID            int64
	signalType          string
	processingMode      string
	embeddingProvider   string
	vectorStoreProvider string
	workerID            string
	mu                  sync.RWMutex
	startedAt           time.Time
	activeJobs          map[*EmbeddingJobStats]struct{}
	totalJobsProcessed  int64
	successfulJobs      int64
	failedJobs          int64
	cancelledJobs       int64
	totalSignals        int64
	totalTokens         int64
}

// NewEmbeddingMetricsCollector creates a new embedding metrics collector
func NewEmbeddingMetricsCollector(configID int64, signalType, processingMode, embeddingProvider, vectorStoreProvider, workerID string) *EmbeddingMetricsCollector {
	return &EmbeddingMetricsCollector{
		configID:            configID,
		signalType:          signalType,
		processingMode:      processingMode,
		embeddingProvider:   embeddingProvider,
		vectorStoreProvider: vectorStoreProvider,
		workerID:            workerID,
		startedAt:           time.Now(),
		activeJobs:          make(map[*EmbeddingJobStats]struct{}),
	}
}

// RecordJobStart records the start of an embedding job
func (e *EmbeddingMetricsCollector) RecordJobStart(workerID string, signalCount, tokenCount int) *EmbeddingJobStats {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Increment active jobs
	metrics.EmbeddingActiveJobs.WithLabelValues(e.signalType, e.workerID).Inc()

	stats := &EmbeddingJobStats{
		StartTime:   time.Now(),
		SignalCount: signalCount,
		TokenCount:  tokenCount,
		WorkerID:    workerID,
	}
	e.activeJobs[stats] = struct{}{}
	return stats
}

// RecordJobCompletion records the completion of an embedding job
func (e *EmbeddingMetricsCollector) RecordJobCompletion(stats *EmbeddingJobStats, success bool, details *EmbeddingJobDetails) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, active := e.activeJobs[stats]; !active {
		return // Ignore nil, foreign, cancelled, or already completed attempts.
	}
	delete(e.activeJobs, stats)
	stats.EndTime = time.Now()
	stats.Duration = stats.EndTime.Sub(stats.StartTime)
	e.totalJobsProcessed++
	if success {
		e.successfulJobs++
	} else {
		e.failedJobs++
	}
	signalCount, tokenCount := max(stats.SignalCount, 0), max(stats.TokenCount, 0)
	e.totalSignals += int64(signalCount)
	e.totalTokens += int64(tokenCount)

	// Decrement active jobs
	metrics.EmbeddingActiveJobs.WithLabelValues(e.signalType, e.workerID).Dec()

	// Record job completion
	status := "success"
	if !success {
		status = "failed"
	}
	metrics.EmbeddingJobsTotal.WithLabelValues(status, e.signalType, e.processingMode).Inc()

	// Record job duration
	duration := stats.Duration.Seconds()
	metrics.EmbeddingJobsDuration.WithLabelValues(e.signalType, e.processingMode).Observe(duration)

	// Record signals processed
	metrics.EmbeddingSignalsTotal.WithLabelValues(status, e.signalType).Add(float64(signalCount))

	// Record tokens processed
	metrics.EmbeddingTokensTotal.WithLabelValues(e.signalType, "processed").Add(float64(tokenCount))

	// Record batch size
	metrics.EmbeddingBatchSize.WithLabelValues(e.signalType).Observe(float64(signalCount))

	// Record errors if any
	if details != nil && details.ErrorCount > 0 {
		metrics.EmbeddingErrorsTotal.WithLabelValues("processing", e.signalType).Add(float64(details.ErrorCount))
	}

	// Record retries if any
	if details != nil && details.RetryCount > 0 {
		metrics.EmbeddingRetriesTotal.WithLabelValues(e.signalType).Add(float64(details.RetryCount))
	}

	// Record vector store operations
	if details != nil {
		if details.VectorStoreWrites > 0 {
			metrics.EmbeddingVectorStoreWritesTotal.WithLabelValues("success", e.vectorStoreProvider, "default").Add(float64(details.VectorStoreWrites))
		}
		if details.VectorStoreErrors > 0 {
			metrics.EmbeddingVectorStoreWritesTotal.WithLabelValues("error", e.vectorStoreProvider, "default").Add(float64(details.VectorStoreErrors))
		}
		if details.VectorStoreLatency > 0 {
			metrics.EmbeddingVectorStoreLatency.WithLabelValues(e.vectorStoreProvider, "default").Observe(details.VectorStoreLatency.Seconds())
		}
	}

	// Record template operations
	if details != nil {
		if details.TemplateRenders > 0 {
			metrics.EmbeddingTemplateRendersTotal.WithLabelValues("success", e.signalType).Add(float64(details.TemplateRenders))
		}
		if details.TemplateErrors > 0 {
			metrics.EmbeddingTemplateRendersTotal.WithLabelValues("error", e.signalType).Add(float64(details.TemplateErrors))
		}
		if details.TotalTemplateTime > 0 {
			metrics.EmbeddingTemplateLatency.WithLabelValues(e.signalType).Observe(details.TotalTemplateTime.Seconds())
		}
	}

	// Record memory usage
	if details != nil && details.MemoryUsage > 0 {
		metrics.EmbeddingMemoryUsage.WithLabelValues(e.signalType, "current").Set(float64(details.MemoryUsage))
	}

	// Update worker uptime
	uptime := time.Since(e.startedAt).Seconds()
	metrics.EmbeddingWorkerUptime.WithLabelValues(e.workerID, e.signalType).Set(uptime)
}

// RecordJobCancellation cancels the oldest active attempt for this worker.
// Completion of that same attempt is ignored, so terminal outcomes are recorded once.
func (e *EmbeddingMetricsCollector) RecordJobCancellation(workerID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var oldest *EmbeddingJobStats
	for stats := range e.activeJobs {
		if stats.WorkerID == workerID && (oldest == nil || stats.StartTime.Before(oldest.StartTime)) {
			oldest = stats
		}
	}
	if oldest == nil {
		return
	}
	delete(e.activeJobs, oldest)
	oldest.EndTime = time.Now()
	oldest.Duration = oldest.EndTime.Sub(oldest.StartTime)
	e.totalJobsProcessed++
	e.cancelledJobs++

	// Decrement active jobs
	metrics.EmbeddingActiveJobs.WithLabelValues(e.signalType, e.workerID).Dec()

	// Record cancelled job
	metrics.EmbeddingJobsTotal.WithLabelValues("cancelled", e.signalType, e.processingMode).Inc()
}

// RecordEmbeddingQuality records the quality of an embedding
func (e *EmbeddingMetricsCollector) RecordEmbeddingQuality(quality float64) {
	metrics.EmbeddingQualityScore.WithLabelValues(e.signalType).Observe(quality)
}

// RecordTokenTruncation records when tokens are truncated
func (e *EmbeddingMetricsCollector) RecordTokenTruncation(originalCount, truncatedCount int) {
	truncated := originalCount - truncatedCount
	metrics.EmbeddingTokensTotal.WithLabelValues(e.signalType, "truncated").Add(float64(truncated))
}

// RecordSignalSkipped records when a signal is skipped
func (e *EmbeddingMetricsCollector) RecordSignalSkipped(reason string) {
	metrics.EmbeddingSignalsTotal.WithLabelValues("skipped", e.signalType).Inc()
}

// RecordBatchProcessed records a batch being processed
func (e *EmbeddingMetricsCollector) RecordBatchProcessed(success bool, batchSize int) {
	status := "success"
	if !success {
		status = "failed"
	}
	metrics.EmbeddingBatchesTotal.WithLabelValues(status, e.signalType).Inc()
	metrics.EmbeddingBatchSize.WithLabelValues(e.signalType).Observe(float64(batchSize))
}

// RecordThroughput records the current throughput
func (e *EmbeddingMetricsCollector) RecordThroughput(signalsPerSecond float64) {
	metrics.EmbeddingThroughput.WithLabelValues(e.signalType).Set(signalsPerSecond)
}

// RecordError records a specific error
func (e *EmbeddingMetricsCollector) RecordError(errorType string) {
	metrics.EmbeddingErrorsTotal.WithLabelValues(errorType, e.signalType).Inc()
}

// RecordRetry records a retry attempt
func (e *EmbeddingMetricsCollector) RecordRetry() {
	metrics.EmbeddingRetriesTotal.WithLabelValues(e.signalType).Inc()
}

// RecordVectorStoreWrite records a vector store write operation
func (e *EmbeddingMetricsCollector) RecordVectorStoreWrite(success bool, latency time.Duration) {
	status := "success"
	if !success {
		status = "error"
	}
	metrics.EmbeddingVectorStoreWritesTotal.WithLabelValues(status, e.vectorStoreProvider, "default").Inc()
	if latency > 0 {
		metrics.EmbeddingVectorStoreLatency.WithLabelValues(e.vectorStoreProvider, "default").Observe(latency.Seconds())
	}
}

// RecordTemplateRender records a template render operation
func (e *EmbeddingMetricsCollector) RecordTemplateRender(success bool, latency time.Duration) {
	status := "success"
	if !success {
		status = "error"
	}
	metrics.EmbeddingTemplateRendersTotal.WithLabelValues(status, e.signalType).Inc()
	if latency > 0 {
		metrics.EmbeddingTemplateLatency.WithLabelValues(e.signalType).Observe(latency.Seconds())
	}
}

// RecordMemoryUsage records memory usage
func (e *EmbeddingMetricsCollector) RecordMemoryUsage(memoryType string, bytes int64) {
	metrics.EmbeddingMemoryUsage.WithLabelValues(e.signalType, memoryType).Set(float64(bytes))
}

// RecordEmbeddingMemoryUsage records embedding-specific memory usage
func (e *EmbeddingMetricsCollector) RecordEmbeddingMemoryUsage(memoryUsage int64) {
	// Only record embedding-specific memory metrics
	// System-level memory metrics are handled by SystemMetricsCollector
	metrics.EmbeddingMemoryUsage.WithLabelValues(e.signalType, "current").Set(float64(memoryUsage))
}

// EmbeddingJobStats tracks statistics for a single embedding job
type EmbeddingJobStats struct {
	StartTime   time.Time
	EndTime     time.Time
	Duration    time.Duration
	SignalCount int
	TokenCount  int
	WorkerID    string
}

// EmbeddingJobDetails provides detailed information about a job
type EmbeddingJobDetails struct {
	FetchTime          time.Duration
	EmbeddingTime      time.Duration
	StoreTime          time.Duration
	TemplateTime       time.Duration
	ErrorCount         int
	RetryCount         int
	Timeout            bool
	RateLimited        bool
	MemoryUsage        int64
	VectorStoreWrites  int
	VectorStoreErrors  int
	VectorStoreLatency time.Duration
	TemplateRenders    int
	TemplateErrors     int
	TotalTemplateTime  time.Duration
	QualityScore       float64
	TruncatedTokens    int
}

// EmbeddingMetrics is a copied cumulative snapshot for one production worker.
// TotalJobsProcessed counts terminal attempts, including cancelled attempts.
// ActiveJobs counts attempts which have not yet reached a terminal state.
type EmbeddingMetrics struct {
	ConfigID            int64
	SignalType          string
	ProcessingMode      string
	EmbeddingProvider   string
	VectorStoreProvider string
	TotalJobsProcessed  int64
	SuccessfulJobs      int64
	FailedJobs          int64
	CancelledJobs       int64
	ActiveJobs          int64
	TotalSignals        int64
	TotalTokens         int64
	Uptime              time.Duration
}

// GetMetrics returns a read-only copy of this collector's actual counters.
func (e *EmbeddingMetricsCollector) GetMetrics() *EmbeddingMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return &EmbeddingMetrics{
		ConfigID:            e.configID,
		SignalType:          e.signalType,
		ProcessingMode:      e.processingMode,
		EmbeddingProvider:   e.embeddingProvider,
		VectorStoreProvider: e.vectorStoreProvider,
		TotalJobsProcessed:  e.totalJobsProcessed,
		SuccessfulJobs:      e.successfulJobs,
		FailedJobs:          e.failedJobs,
		CancelledJobs:       e.cancelledJobs,
		ActiveJobs:          int64(len(e.activeJobs)),
		TotalSignals:        e.totalSignals,
		TotalTokens:         e.totalTokens,
		Uptime:              time.Since(e.startedAt),
	}
}

// LogMetrics logs a summary of current metrics (placeholder for Prometheus)
func (e *EmbeddingMetricsCollector) LogMetrics(ctx context.Context) {
	// Prometheus metrics are automatically exposed via /metrics endpoint
	// This method is kept for interface compatibility
}

// ResetMetrics resets all metrics (not applicable for Prometheus)
func (e *EmbeddingMetricsCollector) ResetMetrics() {
	// Prometheus metrics are cumulative and don't support reset
	// This method is kept for interface compatibility
}
