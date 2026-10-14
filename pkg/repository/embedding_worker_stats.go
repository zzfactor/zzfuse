package repository

import (
	"sync"
	"time"
)

// StatisticsCollector collects and tracks performance metrics
type StatisticsCollector struct {
	metrics *WorkerMetrics
	mu      sync.RWMutex
}

// WorkerMetrics tracks comprehensive performance metrics
type WorkerMetrics struct {
	// Job metrics
	TotalJobsProcessed int64
	SuccessfulJobs     int64
	FailedJobs         int64
	CancelledJobs      int64

	// Processing metrics
	TotalSignalsProcessed int64
	TotalBatchesProcessed int64
	TotalTokensProcessed  int64

	// Timing metrics
	TotalProcessingTime   time.Duration
	AverageProcessingTime time.Duration
	TotalFetchTime        time.Duration
	TotalEmbeddingTime    time.Duration
	TotalStoreTime        time.Duration

	// Error metrics
	TotalErrors          int64
	TotalRetries         int64
	AverageRetriesPerJob float64

	// Performance metrics
	AverageSignalsPerBatch float64
	AverageTokensPerSignal float64
	AverageBatchSize       float64

	// Memory metrics
	PeakMemoryUsage    int64
	CurrentMemoryUsage int64

	// Signal type metrics
	SignalTypeMetrics map[string]*SignalTypeStats

	// Worker metrics
	WorkerMetrics map[string]*WorkerStats

	// Last updated
	LastUpdated time.Time
}

// (SignalTypeStats tracks metrics for a specific signal type
type SignalTypeStats struct {
	TotalProcessed int64
	SuccessfulJobs int64
	FailedJobs     int64
	AverageTime    time.Duration
	TotalTime      time.Duration
	AverageTokens  float64
	TotalTokens    int64
	ErrorCount     int64
	RetryCount     int64
	LastProcessed  time.Time
}

// WorkerStats tracks metrics for a specific worker
type WorkerStats struct {
	TotalJobs      int64
	SuccessfulJobs int64
	FailedJobs     int64
	AverageTime    time.Duration
	TotalTime      time.Duration
	CurrentJob     string
	LastJobTime    time.Time
	Uptime         time.Duration
	StartTime      time.Time
}

// ProcessingStats represents statistics for a single processing operation
type ProcessingStats struct {
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	SignalCount   int
	BatchCount    int
	TokenCount    int
	ErrorCount    int
	RetryCount    int
	MemoryUsage   int64
	FetchTime     time.Duration
	ProcessTime   time.Duration
	EmbeddingTime time.Duration
	StoreTime     time.Duration
}

// NewStatisticsCollector creates a new statistics collector
func NewStatisticsCollector() *StatisticsCollector {
	return &StatisticsCollector{
		metrics: &WorkerMetrics{
			SignalTypeMetrics: make(map[string]*SignalTypeStats),
			WorkerMetrics:     make(map[string]*WorkerStats),
		},
	}
}

// RecordJobStart records the start of a job
func (sc *StatisticsCollector) RecordJobStart(workerID, signalType string) *ProcessingStats {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	stats := &ProcessingStats{
		StartTime: time.Now(),
	}

	// Update worker stats
	if _, exists := sc.metrics.WorkerMetrics[workerID]; !exists {
		sc.metrics.WorkerMetrics[workerID] = &WorkerStats{
			StartTime: time.Now(),
		}
	}
	workerStats := sc.metrics.WorkerMetrics[workerID]
	workerStats.TotalJobs++
	workerStats.CurrentJob = "running"
	workerStats.LastJobTime = time.Now()

	// Update signal type stats
	if _, exists := sc.metrics.SignalTypeMetrics[signalType]; !exists {
		sc.metrics.SignalTypeMetrics[signalType] = &SignalTypeStats{}
	}

	sc.metrics.TotalJobsProcessed++
	sc.metrics.LastUpdated = time.Now()

	return stats
}

// RecordJobCompletion records the completion of a job
func (sc *StatisticsCollector) RecordJobCompletion(workerID, signalType string, stats *ProcessingStats, success bool) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	stats.EndTime = time.Now()
	if stats.Duration == 0 && !stats.StartTime.IsZero() {
		stats.Duration = stats.EndTime.Sub(stats.StartTime)
	}

	// Update overall metrics
	if success {
		sc.metrics.SuccessfulJobs++
	} else {
		sc.metrics.FailedJobs++
	}

	sc.metrics.TotalSignalsProcessed += int64(stats.SignalCount)
	sc.metrics.TotalBatchesProcessed += int64(stats.BatchCount)
	sc.metrics.TotalTokensProcessed += int64(stats.TokenCount)
	sc.metrics.TotalErrors += int64(stats.ErrorCount)
	sc.metrics.TotalRetries += int64(stats.RetryCount)

	// Update timing metrics
	sc.metrics.TotalProcessingTime += stats.Duration
	sc.metrics.TotalFetchTime += stats.FetchTime
	sc.metrics.TotalEmbeddingTime += stats.EmbeddingTime
	sc.metrics.TotalStoreTime += stats.StoreTime

	// Update memory metrics
	if stats.MemoryUsage > sc.metrics.PeakMemoryUsage {
		sc.metrics.PeakMemoryUsage = stats.MemoryUsage
	}
	sc.metrics.CurrentMemoryUsage = stats.MemoryUsage

	// Update averages
	if sc.metrics.TotalJobsProcessed > 0 {
		sc.metrics.AverageProcessingTime = time.Duration(sc.metrics.TotalProcessingTime.Nanoseconds() / sc.metrics.TotalJobsProcessed)
		sc.metrics.AverageRetriesPerJob = float64(sc.metrics.TotalRetries) / float64(sc.metrics.TotalJobsProcessed)
	}

	if sc.metrics.TotalBatchesProcessed > 0 {
		sc.metrics.AverageSignalsPerBatch = float64(sc.metrics.TotalSignalsProcessed) / float64(sc.metrics.TotalBatchesProcessed)
		sc.metrics.AverageBatchSize = float64(sc.metrics.TotalSignalsProcessed) / float64(sc.metrics.TotalBatchesProcessed)
	}

	if sc.metrics.TotalSignalsProcessed > 0 {
		sc.metrics.AverageTokensPerSignal = float64(sc.metrics.TotalTokensProcessed) / float64(sc.metrics.TotalSignalsProcessed)
	}

	// Update worker stats
	if workerStats, exists := sc.metrics.WorkerMetrics[workerID]; exists {
		if success {
			workerStats.SuccessfulJobs++
		} else {
			workerStats.FailedJobs++
		}
		workerStats.TotalTime += stats.Duration
		workerStats.CurrentJob = "idle"
		if workerStats.TotalJobs > workerStats.SuccessfulJobs+workerStats.FailedJobs {
			workerStats.CurrentJob = "running"
		}
		workerStats.Uptime = time.Since(workerStats.StartTime)

		if workerStats.TotalJobs > 0 {
			workerStats.AverageTime = time.Duration(workerStats.TotalTime.Nanoseconds() / workerStats.TotalJobs)
		}
	}

	// Update signal type stats
	if signalStats, exists := sc.metrics.SignalTypeMetrics[signalType]; exists {
		signalStats.TotalProcessed++
		if success {
			signalStats.SuccessfulJobs++
		} else {
			signalStats.FailedJobs++
		}
		signalStats.TotalTime += stats.Duration
		signalStats.TotalTokens += int64(stats.TokenCount)
		signalStats.ErrorCount += int64(stats.ErrorCount)
		signalStats.RetryCount += int64(stats.RetryCount)
		signalStats.LastProcessed = time.Now()

		if signalStats.TotalProcessed > 0 {
			signalStats.AverageTime = time.Duration(signalStats.TotalTime.Nanoseconds() / signalStats.TotalProcessed)
			signalStats.AverageTokens = float64(signalStats.TotalTokens) / float64(signalStats.TotalProcessed)
		}
	}

	sc.metrics.LastUpdated = time.Now()
}

// RecordBatchStatistics updates processing totals without changing job outcomes.
func (sc *StatisticsCollector) RecordBatchStatistics(workerID, signalType string, stats *ProcessingStats) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics.TotalSignalsProcessed += int64(stats.SignalCount)
	sc.metrics.TotalBatchesProcessed += int64(stats.BatchCount)
	sc.metrics.TotalTokensProcessed += int64(stats.TokenCount)
	sc.metrics.TotalErrors += int64(stats.ErrorCount)
	sc.metrics.TotalProcessingTime += stats.Duration
	if sc.metrics.TotalJobsProcessed > 0 {
		sc.metrics.AverageProcessingTime = sc.metrics.TotalProcessingTime / time.Duration(sc.metrics.TotalJobsProcessed)
	}
	if sc.metrics.TotalBatchesProcessed > 0 {
		sc.metrics.AverageSignalsPerBatch = float64(sc.metrics.TotalSignalsProcessed) / float64(sc.metrics.TotalBatchesProcessed)
		sc.metrics.AverageBatchSize = sc.metrics.AverageSignalsPerBatch
	}
	if sc.metrics.TotalSignalsProcessed > 0 {
		sc.metrics.AverageTokensPerSignal = float64(sc.metrics.TotalTokensProcessed) / float64(sc.metrics.TotalSignalsProcessed)
	}

	workerStats, exists := sc.metrics.WorkerMetrics[workerID]
	if !exists {
		workerStats = &WorkerStats{StartTime: time.Now(), CurrentJob: "idle"}
		sc.metrics.WorkerMetrics[workerID] = workerStats
	}
	workerStats.TotalTime += stats.Duration
	if workerStats.TotalJobs > 0 {
		workerStats.AverageTime = workerStats.TotalTime / time.Duration(workerStats.TotalJobs)
	}

	signalStats, exists := sc.metrics.SignalTypeMetrics[signalType]
	if !exists {
		signalStats = &SignalTypeStats{}
		sc.metrics.SignalTypeMetrics[signalType] = signalStats
	}
	signalStats.TotalTime += stats.Duration
	signalStats.TotalTokens += int64(stats.TokenCount)
	signalStats.ErrorCount += int64(stats.ErrorCount)
	if signalStats.TotalProcessed > 0 {
		signalStats.AverageTime = signalStats.TotalTime / time.Duration(signalStats.TotalProcessed)
		signalStats.AverageTokens = float64(signalStats.TotalTokens) / float64(signalStats.TotalProcessed)
	}
	sc.metrics.LastUpdated = time.Now()
}

// RecordJobCancellation records the cancellation of a job
func (sc *StatisticsCollector) RecordJobCancellation(workerID, signalType string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics.CancelledJobs++
	sc.metrics.TotalJobsProcessed++

	// Update worker stats
	if workerStats, exists := sc.metrics.WorkerMetrics[workerID]; exists {
		workerStats.CurrentJob = "idle"
	}

	sc.metrics.LastUpdated = time.Now()
}

// GetMetrics returns a copy of the current metrics
func (sc *StatisticsCollector) GetMetrics() *WorkerMetrics {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	// Create a deep copy to avoid race conditions
	metrics := &WorkerMetrics{
		TotalJobsProcessed:     sc.metrics.TotalJobsProcessed,
		SuccessfulJobs:         sc.metrics.SuccessfulJobs,
		FailedJobs:             sc.metrics.FailedJobs,
		CancelledJobs:          sc.metrics.CancelledJobs,
		TotalSignalsProcessed:  sc.metrics.TotalSignalsProcessed,
		TotalBatchesProcessed:  sc.metrics.TotalBatchesProcessed,
		TotalTokensProcessed:   sc.metrics.TotalTokensProcessed,
		TotalProcessingTime:    sc.metrics.TotalProcessingTime,
		AverageProcessingTime:  sc.metrics.AverageProcessingTime,
		TotalFetchTime:         sc.metrics.TotalFetchTime,
		TotalEmbeddingTime:     sc.metrics.TotalEmbeddingTime,
		TotalStoreTime:         sc.metrics.TotalStoreTime,
		TotalErrors:            sc.metrics.TotalErrors,
		TotalRetries:           sc.metrics.TotalRetries,
		AverageRetriesPerJob:   sc.metrics.AverageRetriesPerJob,
		AverageSignalsPerBatch: sc.metrics.AverageSignalsPerBatch,
		AverageTokensPerSignal: sc.metrics.AverageTokensPerSignal,
		AverageBatchSize:       sc.metrics.AverageBatchSize,
		PeakMemoryUsage:        sc.metrics.PeakMemoryUsage,
		CurrentMemoryUsage:     sc.metrics.CurrentMemoryUsage,
		LastUpdated:            sc.metrics.LastUpdated,
		SignalTypeMetrics:      make(map[string]*SignalTypeStats),
		WorkerMetrics:          make(map[string]*WorkerStats),
	}

	// Copy signal type metrics
	for signalType, stats := range sc.metrics.SignalTypeMetrics {
		metrics.SignalTypeMetrics[signalType] = &SignalTypeStats{
			TotalProcessed: stats.TotalProcessed,
			SuccessfulJobs: stats.SuccessfulJobs,
			FailedJobs:     stats.FailedJobs,
			AverageTime:    stats.AverageTime,
			TotalTime:      stats.TotalTime,
			AverageTokens:  stats.AverageTokens,
			TotalTokens:    stats.TotalTokens,
			ErrorCount:     stats.ErrorCount,
			RetryCount:     stats.RetryCount,
			LastProcessed:  stats.LastProcessed,
		}
	}

	// Copy worker metrics
	for workerID, stats := range sc.metrics.WorkerMetrics {
		metrics.WorkerMetrics[workerID] = &WorkerStats{
			TotalJobs:      stats.TotalJobs,
			SuccessfulJobs: stats.SuccessfulJobs,
			FailedJobs:     stats.FailedJobs,
			AverageTime:    stats.AverageTime,
			TotalTime:      stats.TotalTime,
			CurrentJob:     stats.CurrentJob,
			LastJobTime:    stats.LastJobTime,
			Uptime:         stats.Uptime,
			StartTime:      stats.StartTime,
		}
	}

	return metrics
}

// GetSignalTypeStats returns statistics for a specific signal type
func (sc *StatisticsCollector) GetSignalTypeStats(signalType string) *SignalTypeStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if stats, exists := sc.metrics.SignalTypeMetrics[signalType]; exists {
		return &SignalTypeStats{
			TotalProcessed: stats.TotalProcessed,
			SuccessfulJobs: stats.SuccessfulJobs,
			FailedJobs:     stats.FailedJobs,
			AverageTime:    stats.AverageTime,
			TotalTime:      stats.TotalTime,
			AverageTokens:  stats.AverageTokens,
			TotalTokens:    stats.TotalTokens,
			ErrorCount:     stats.ErrorCount,
			RetryCount:     stats.RetryCount,
			LastProcessed:  stats.LastProcessed,
		}
	}

	return &SignalTypeStats{}
}

// GetWorkerStats returns statistics for a specific worker
func (sc *StatisticsCollector) GetWorkerStats(workerID string) *WorkerStats {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if stats, exists := sc.metrics.WorkerMetrics[workerID]; exists {
		return &WorkerStats{
			TotalJobs:      stats.TotalJobs,
			SuccessfulJobs: stats.SuccessfulJobs,
			FailedJobs:     stats.FailedJobs,
			AverageTime:    stats.AverageTime,
			TotalTime:      stats.TotalTime,
			CurrentJob:     stats.CurrentJob,
			LastJobTime:    stats.LastJobTime,
			Uptime:         stats.Uptime,
			StartTime:      stats.StartTime,
		}
	}

	return &WorkerStats{}
}

// ResetMetrics resets all metrics to zero
func (sc *StatisticsCollector) ResetMetrics() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	sc.metrics = &WorkerMetrics{
		SignalTypeMetrics: make(map[string]*SignalTypeStats),
		WorkerMetrics:     make(map[string]*WorkerStats),
		LastUpdated:       time.Now(),
	}
}

// GetSuccessRate returns the overall success rate
func (sc *StatisticsCollector) GetSuccessRate() float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.metrics.TotalJobsProcessed == 0 {
		return 0.0
	}

	return float64(sc.metrics.SuccessfulJobs) / float64(sc.metrics.TotalJobsProcessed) * 100.0
}

// GetErrorRate returns the overall error rate
func (sc *StatisticsCollector) GetErrorRate() float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()

	if sc.metrics.TotalJobsProcessed == 0 {
		return 0.0
	}

	return float64(sc.metrics.FailedJobs) / float64(sc.metrics.TotalJobsProcessed) * 100.0
}
