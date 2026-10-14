package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/medxops/trazr-otel/pkg/util"
)

// BaseWorker provides common functionality for embedding workers
type BaseWorker struct {
	// Core configuration
	DB         OtelDatabaseClient
	Logger     util.Logger
	ConfigID   int64
	SignalType string
	WorkerID   string
	Schema     string

	// Batch processing configuration
	BatchSize            int
	FlushInterval        time.Duration
	MaxRetries           int
	BatchErrorPolicy     string
	MaxConcurrentBatches int
	RetryBackoffBase     time.Duration
	RetryBackoffMax      time.Duration

	// State management
	status   *WorkerStatus
	stopped  chan struct{}
	paused   chan struct{}
	isPaused bool
	mutex    sync.RWMutex

	// Statistics collection
	statsCollector *StatisticsCollector
	statsMutex     sync.RWMutex

	// Job tracking
	activeJobs map[string]*ProcessingJob
	jobMetrics map[string]*workerJobAccounting
	jobMutex   sync.RWMutex
}

type workerJobAccounting struct {
	startedAt time.Time
	baseline  ProcessingStats
	batches   ProcessingStats
}

// BaseWorkerOption is a functional option for configuring the base worker
type BaseWorkerOption func(*BaseWorker)

// WithBaseBatchSize sets the batch size for the base worker
func WithBaseBatchSize(size int) BaseWorkerOption {
	return func(w *BaseWorker) { w.BatchSize = size }
}

// WithBaseFlushInterval sets the flush interval for the base worker
func WithBaseFlushInterval(interval time.Duration) BaseWorkerOption {
	return func(w *BaseWorker) { w.FlushInterval = interval }
}

// WithBaseMaxRetries sets the maximum retries for the base worker
func WithBaseMaxRetries(maxRetries int) BaseWorkerOption {
	return func(w *BaseWorker) { w.MaxRetries = maxRetries }
}

// WithBaseBatchErrorPolicy sets the batch error policy for the base worker
func WithBaseBatchErrorPolicy(policy string) BaseWorkerOption {
	return func(w *BaseWorker) { w.BatchErrorPolicy = policy }
}

// WithBaseMaxConcurrentBatches sets the maximum concurrent batches for the base worker
func WithBaseMaxConcurrentBatches(maxConcurrent int) BaseWorkerOption {
	return func(w *BaseWorker) { w.MaxConcurrentBatches = maxConcurrent }
}

// WithBaseRetryBackoff sets the retry backoff configuration for the base worker
func WithBaseRetryBackoff(base, max time.Duration) BaseWorkerOption {
	return func(w *BaseWorker) {
		w.RetryBackoffBase = base
		w.RetryBackoffMax = max
	}
}

// WithBaseConfigID sets the config ID for the base worker
func WithBaseConfigID(configID int64) BaseWorkerOption {
	return func(w *BaseWorker) { w.ConfigID = configID }
}

// WithBaseSignalType sets the signal type for the base worker
func WithBaseSignalType(signalType string) BaseWorkerOption {
	return func(w *BaseWorker) { w.SignalType = signalType }
}

// WithBaseWorkerID sets the worker ID for the base worker
func WithBaseWorkerID(id string) BaseWorkerOption {
	return func(w *BaseWorker) { w.WorkerID = id }
}

// WithBaseSchema sets the schema for the base worker
func WithBaseSchema(schema string) BaseWorkerOption {
	return func(w *BaseWorker) { w.Schema = schema }
}

// NewBaseWorker creates a new base worker with the given options
func NewBaseWorker(db OtelDatabaseClient, logger util.Logger, opts ...BaseWorkerOption) *BaseWorker {
	w := &BaseWorker{
		DB:                   db,
		Logger:               logger,
		BatchSize:            100,
		FlushInterval:        200 * time.Millisecond,
		MaxRetries:           3,
		BatchErrorPolicy:     "continue",
		MaxConcurrentBatches: 1,
		RetryBackoffBase:     1 * time.Second,
		RetryBackoffMax:      30 * time.Second,
		status:               &WorkerStatus{Running: false},
		stopped:              make(chan struct{}),
		paused:               make(chan struct{}),
		activeJobs:           make(map[string]*ProcessingJob),
		jobMetrics:           make(map[string]*workerJobAccounting),
		statsCollector:       NewStatisticsCollector(),
	}

	// Apply options
	for _, opt := range opts {
		opt(w)
	}

	// Generate worker ID if not provided
	if w.WorkerID == "" {
		w.WorkerID = uuid.New().String()
	}

	return w
}

// StartJob starts a new processing job
func (w *BaseWorker) StartJob(ctx context.Context, jobType string) (*ProcessingJob, error) {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()

	jobID := uuid.New().String()
	now := time.Now()
	job := &ProcessingJob{
		ID:         jobID,
		Type:       JobType(jobType),
		State:      JobStateRunning,
		SignalType: w.SignalType,
		WorkerID:   w.WorkerID,
		ConfigID:   w.ConfigID,
		CreatedAt:  now,
		StartedAt:  &now,
		Progress:   &JobProgress{},
		Statistics: &JobStatistics{},
		Metadata:   make(map[string]interface{}),
	}

	w.activeJobs[jobID] = job
	w.jobMetrics[jobID] = &workerJobAccounting{startedAt: now}
	w.statsCollector.RecordJobStart(w.WorkerID, w.SignalType)
	w.Logger.Debug("Started processing job", "job_id", jobID, "type", jobType)
	return job, nil
}

// CompleteJob marks a job as completed
func (w *BaseWorker) CompleteJob(jobID string, stats *JobStatistics) error {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()

	job, exists := w.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}
	if job.State != JobStateRunning && job.State != JobStatePaused {
		return fmt.Errorf("job is not active: %s", jobID)
	}

	now := time.Now()
	job.State = JobStateCompleted
	job.CompletedAt = &now
	if stats != nil {
		job.Statistics = stats
	}
	w.recordJobCompletion(job, true)

	w.Logger.Debug("Completed processing job", "job_id", jobID)
	return nil
}

// FailJob marks a job as failed
func (w *BaseWorker) FailJob(jobID string, err error) error {
	if err == nil {
		return fmt.Errorf("job failure requires an error")
	}
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()

	job, exists := w.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}
	if job.State != JobStateRunning && job.State != JobStatePaused {
		return fmt.Errorf("job is not active: %s", jobID)
	}

	now := time.Now()
	job.State = JobStateFailed
	job.CompletedAt = &now
	job.Error = &JobError{
		Step:        "processing",
		Message:     "Job failed",
		Details:     err.Error(),
		Recoverable: false,
		RetryCount:  0,
		Timestamp:   now,
	}
	w.recordJobCompletion(job, false)

	w.Logger.Error("Failed processing job", "job_id", jobID, "error", err)
	return nil
}

// UpdateJobProgress updates the progress of a job
func (w *BaseWorker) UpdateJobProgress(jobID string, progress *JobProgress) error {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()

	job, exists := w.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}

	job.Progress = progress
	return nil
}

// GetJobStatus returns the status of a job
func (w *BaseWorker) GetJobStatus(jobID string) (*ProcessingJob, error) {
	w.jobMutex.RLock()
	defer w.jobMutex.RUnlock()

	job, exists := w.activeJobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", jobID)
	}

	return job, nil
}

// GetActiveJobs returns all active jobs
func (w *BaseWorker) GetActiveJobs() []*ProcessingJob {
	w.jobMutex.RLock()
	defer w.jobMutex.RUnlock()

	jobs := make([]*ProcessingJob, 0, len(w.activeJobs))
	for _, job := range w.activeJobs {
		if job.State == JobStatePending || job.State == JobStateRunning || job.State == JobStatePaused {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

// RecordBatchStatistics records a standalone batch, or associates it with the
// single active job. With overlapping jobs, batches remain unattributed and are
// independent of final job summaries. Use RecordJobBatchStatistics for those jobs.
func (w *BaseWorker) RecordBatchStatistics(batchSize int, processingTime time.Duration, tokens int, success bool) {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()
	w.statsMutex.Lock()
	defer w.statsMutex.Unlock()

	jobID := ""
	for _, job := range w.activeJobs {
		if job.State == JobStateRunning || job.State == JobStatePaused {
			if jobID != "" {
				jobID = ""
				break
			}
			jobID = job.ID
		}
	}
	w.recordBatchStatistics(jobID, batchSize, processingTime, tokens, success)
}

// RecordJobBatchStatistics attributes a batch to an active job so overlapping
// jobs can reconcile their summaries without subtracting another job's work.
func (w *BaseWorker) RecordJobBatchStatistics(jobID string, batchSize int, processingTime time.Duration, tokens int, success bool) error {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()
	job, exists := w.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}
	if job.State != JobStateRunning && job.State != JobStatePaused {
		return fmt.Errorf("job is not active: %s", jobID)
	}
	w.statsMutex.Lock()
	defer w.statsMutex.Unlock()
	w.recordBatchStatistics(jobID, batchSize, processingTime, tokens, success)
	return nil
}

// recordBatchStatistics requires jobMutex and statsMutex.
func (w *BaseWorker) recordBatchStatistics(jobID string, batchSize int, processingTime time.Duration, tokens int, success bool) {
	stats := &ProcessingStats{
		SignalCount: batchSize,
		BatchCount:  1,
		TokenCount:  tokens,
		ProcessTime: processingTime,
		Duration:    processingTime,
	}
	if !success {
		stats.ErrorCount = 1
	}
	if account := w.jobMetrics[jobID]; account != nil {
		account.batches.SignalCount += batchSize
		account.batches.BatchCount++
		account.batches.TokenCount += tokens
		account.batches.ErrorCount += stats.ErrorCount
		account.batches.Duration += processingTime
	}
	w.statsCollector.RecordBatchStatistics(w.WorkerID, w.SignalType, stats)
}

// recordJobCompletion requires jobMutex and records only metrics not already
// included in the worker's batch totals.
func (w *BaseWorker) recordJobCompletion(job *ProcessingJob, success bool) {
	stats := &ProcessingStats{}
	account := w.jobMetrics[job.ID]
	if account != nil {
		stats.Duration = job.CompletedAt.Sub(account.startedAt)
	}
	if summary := job.Statistics; summary != nil {
		stats.SignalCount = summary.SignalCount
		stats.BatchCount = summary.BatchCount
		stats.TokenCount = summary.TokenCount
		stats.ErrorCount = summary.ErrorCount
		stats.RetryCount = summary.RetryCount
		stats.MemoryUsage = summary.MemoryUsage
		stats.FetchTime = summary.FetchTime
		stats.EmbeddingTime = summary.EmbeddingTime
		stats.StoreTime = summary.StoreTime
		if summary.TotalTime > 0 {
			stats.Duration = summary.TotalTime
			if account != nil {
				stats.Duration = max(0, stats.Duration-account.baseline.Duration)
			}
		}
	}
	if account != nil {
		stats.SignalCount = max(0, stats.SignalCount-account.baseline.SignalCount)
		stats.BatchCount = max(0, stats.BatchCount-account.baseline.BatchCount)
		stats.TokenCount = max(0, stats.TokenCount-account.baseline.TokenCount)
		stats.ErrorCount = max(0, stats.ErrorCount-account.baseline.ErrorCount)
		stats.RetryCount = max(0, stats.RetryCount-account.baseline.RetryCount)
		stats.FetchTime = max(0, stats.FetchTime-account.baseline.FetchTime)
		stats.EmbeddingTime = max(0, stats.EmbeddingTime-account.baseline.EmbeddingTime)
		stats.StoreTime = max(0, stats.StoreTime-account.baseline.StoreTime)
	}
	if !success && stats.ErrorCount == 0 {
		stats.ErrorCount = 1
	}
	if account != nil {
		stats.SignalCount = max(0, stats.SignalCount-account.batches.SignalCount)
		stats.BatchCount = max(0, stats.BatchCount-account.batches.BatchCount)
		stats.TokenCount = max(0, stats.TokenCount-account.batches.TokenCount)
		stats.ErrorCount = max(0, stats.ErrorCount-account.batches.ErrorCount)
		stats.Duration = max(0, stats.Duration-account.batches.Duration)
	}
	w.statsCollector.RecordJobCompletion(w.WorkerID, w.SignalType, stats, success)
	delete(w.jobMetrics, job.ID)
}

// GetStatistics returns the current statistics
func (w *BaseWorker) GetStatistics() *WorkerMetrics {
	w.statsMutex.RLock()
	defer w.statsMutex.RUnlock()

	return w.statsCollector.GetMetrics()
}

// ResetStatistics starts a new metrics interval. Active jobs remain registered;
// their known earlier work is excluded from later lifetime summaries.
func (w *BaseWorker) ResetStatistics() {
	w.jobMutex.Lock()
	defer w.jobMutex.Unlock()
	w.statsMutex.Lock()
	defer w.statsMutex.Unlock()

	w.statsCollector.ResetMetrics()
	now := time.Now()
	for _, job := range w.activeJobs {
		if job.State != JobStateRunning && job.State != JobStatePaused {
			continue
		}
		account := w.jobMetrics[job.ID]
		account.baseline.SignalCount += account.batches.SignalCount
		account.baseline.BatchCount += account.batches.BatchCount
		account.baseline.TokenCount += account.batches.TokenCount
		account.baseline.ErrorCount += account.batches.ErrorCount
		account.baseline.Duration += account.batches.Duration
		if summary := job.Statistics; summary != nil {
			account.baseline.SignalCount = max(account.baseline.SignalCount, summary.SignalCount)
			account.baseline.BatchCount = max(account.baseline.BatchCount, summary.BatchCount)
			account.baseline.TokenCount = max(account.baseline.TokenCount, summary.TokenCount)
			account.baseline.ErrorCount = max(account.baseline.ErrorCount, summary.ErrorCount)
			account.baseline.RetryCount = max(account.baseline.RetryCount, summary.RetryCount)
			account.baseline.FetchTime = max(account.baseline.FetchTime, summary.FetchTime)
			account.baseline.EmbeddingTime = max(account.baseline.EmbeddingTime, summary.EmbeddingTime)
			account.baseline.StoreTime = max(account.baseline.StoreTime, summary.StoreTime)
			account.baseline.Duration = max(account.baseline.Duration, summary.TotalTime)
		}
		if job.StartedAt != nil {
			account.baseline.Duration = max(account.baseline.Duration, now.Sub(*job.StartedAt))
		}
		account.batches = ProcessingStats{}
		account.startedAt = now
		w.statsCollector.RecordJobStart(w.WorkerID, w.SignalType)
	}
}

// Stop stops the base worker
func (w *BaseWorker) Stop() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	select {
	case <-w.stopped:
	default:
		close(w.stopped)
		w.Logger.Debug("Base worker stopped")
	}
	w.status.Running = false
}

// Pause pauses the base worker
func (w *BaseWorker) Pause() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if !w.isPaused {
		w.isPaused = true
		w.Logger.Debug("Base worker paused")
	}
}

// Resume resumes the base worker
func (w *BaseWorker) Resume() {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	if w.isPaused {
		w.isPaused = false
		w.Logger.Debug("Base worker resumed")
	}
}

// IsPaused returns whether the worker is paused
func (w *BaseWorker) IsPaused() bool {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	return w.isPaused
}

// IsStopped returns whether the worker is stopped
func (w *BaseWorker) IsStopped() bool {
	select {
	case <-w.stopped:
		return true
	default:
		return false
	}
}

// ReportError reports an error to the worker status
func (w *BaseWorker) ReportError(err error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()

	w.status.LastError = err
	w.status.LastRunTime = time.Now().Unix()
	w.Logger.Error("Worker error reported", "error", err)
}

// GetStatus returns the current worker status
func (w *BaseWorker) GetStatus() *WorkerStatus {
	w.mutex.RLock()
	defer w.mutex.RUnlock()

	return &WorkerStatus{
		Running:     w.status.Running,
		LastError:   w.status.LastError,
		LastRunTime: w.status.LastRunTime,
	}
}
