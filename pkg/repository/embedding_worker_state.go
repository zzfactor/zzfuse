package repository

import (
	"fmt"
	"sync"
	"time"
)

// JobState represents the state of a processing job
type JobState string

const (
	JobStatePending   JobState = "pending"
	JobStateRunning   JobState = "running"
	JobStatePaused    JobState = "paused"
	JobStateCompleted JobState = "completed"
	JobStateFailed    JobState = "failed"
	JobStateCancelled JobState = "cancelled"
)

// JobType represents the type of job being processed
type JobType string

const (
	JobTypeBatch   JobType = "batch"
	JobTypeTrigger JobType = "trigger"
	JobTypeHybrid  JobType = "hybrid"
)

// ProcessingJob represents a job being processed by the worker
type ProcessingJob struct {
	ID          string
	Type        JobType
	State       JobState
	SignalType  string
	WorkerID    string
	ConfigID    int64
	CreatedAt   time.Time
	StartedAt   *time.Time
	PausedAt    *time.Time
	ResumedAt   *time.Time
	CompletedAt *time.Time
	Progress    *JobProgress
	Statistics  *JobStatistics
	Error       *JobError
	Metadata    map[string]interface{}
}

// JobProgress tracks the progress of a job
type JobProgress struct {
	CurrentStep            string
	StepNumber             int
	TotalSteps             int
	ProcessedSignals       int
	TotalSignals           int
	ProcessedBatches       int
	TotalBatches           int
	TokenCount             int
	EstimatedTimeRemaining time.Duration
}

// JobStatistics tracks performance metrics for a job
type JobStatistics struct {
	FetchTime     time.Duration
	ProcessTime   time.Duration
	EmbeddingTime time.Duration
	StoreTime     time.Duration
	TotalTime     time.Duration
	MemoryUsage   int64
	TokenCount    int
	SignalCount   int
	BatchCount    int
	ErrorCount    int
	RetryCount    int
}

// JobError represents an error during job processing
type JobError struct {
	Step        string
	Message     string
	Details     string
	Recoverable bool
	RetryCount  int
	Timestamp   time.Time
}

// StateManager manages job states and provides observability
type StateManager struct {
	jobs    map[string]*ProcessingJob
	jobsMu  sync.RWMutex
	metrics *StateMetrics
}

// StateMetrics tracks overall state management metrics
type StateMetrics struct {
	TotalJobsCreated    int64
	TotalJobsCompleted  int64
	TotalJobsFailed     int64
	TotalJobsCancelled  int64
	AverageJobDuration  time.Duration
	TotalProcessingTime time.Duration
	ActiveJobs          int
	mu                  sync.RWMutex
}

// NewStateManager creates a new state manager
func NewStateManager() *StateManager {
	return &StateManager{
		jobs:    make(map[string]*ProcessingJob),
		metrics: &StateMetrics{},
	}
}

// CreateJob creates a new processing job
func (sm *StateManager) CreateJob(jobType JobType, signalType, workerID string, configID int64) *ProcessingJob {
	job := &ProcessingJob{
		ID:         generateJobID(),
		Type:       jobType,
		State:      JobStatePending,
		SignalType: signalType,
		WorkerID:   workerID,
		ConfigID:   configID,
		CreatedAt:  time.Now(),
		Progress:   &JobProgress{},
		Statistics: &JobStatistics{},
		Metadata:   make(map[string]interface{}),
	}

	sm.jobsMu.Lock()
	sm.jobs[job.ID] = job
	sm.jobsMu.Unlock()

	sm.metrics.mu.Lock()
	sm.metrics.TotalJobsCreated++
	sm.metrics.ActiveJobs++
	sm.metrics.mu.Unlock()

	return job
}

// StartJob marks a job as running
func (sm *StateManager) StartJob(jobID string) error {
	sm.jobsMu.Lock()
	defer sm.jobsMu.Unlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	now := time.Now()
	job.State = JobStateRunning
	job.StartedAt = &now
	job.Progress.CurrentStep = "Starting job"
	job.Progress.StepNumber = 1

	return nil
}

// UpdateJobProgress updates the progress of a job
func (sm *StateManager) UpdateJobProgress(jobID string, progress *JobProgress) error {
	sm.jobsMu.Lock()
	defer sm.jobsMu.Unlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	job.Progress = progress
	return nil
}

// UpdateJobStatistics updates the statistics of a job
func (sm *StateManager) UpdateJobStatistics(jobID string, stats *JobStatistics) error {
	sm.jobsMu.Lock()
	defer sm.jobsMu.Unlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	job.Statistics = stats
	return nil
}

// CompleteJob marks a job as completed
func (sm *StateManager) CompleteJob(jobID string) error {
	sm.jobsMu.Lock()
	defer sm.jobsMu.Unlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	now := time.Now()
	job.State = JobStateCompleted
	job.CompletedAt = &now
	job.Progress.CurrentStep = "Completed"
	job.Progress.StepNumber = job.Progress.TotalSteps

	// Update metrics
	sm.metrics.mu.Lock()
	sm.metrics.TotalJobsCompleted++
	sm.metrics.ActiveJobs--
	if job.StartedAt != nil {
		duration := now.Sub(*job.StartedAt)
		sm.metrics.TotalProcessingTime += duration
		// Update average (simplified calculation)
		totalCompleted := sm.metrics.TotalJobsCompleted
		sm.metrics.AverageJobDuration = time.Duration(sm.metrics.TotalProcessingTime.Nanoseconds() / totalCompleted)
	}
	sm.metrics.mu.Unlock()

	return nil
}

// FailJob marks a job as failed
func (sm *StateManager) FailJob(jobID string, err *JobError) error {
	sm.jobsMu.Lock()
	defer sm.jobsMu.Unlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	now := time.Now()
	job.State = JobStateFailed
	job.CompletedAt = &now
	job.Error = err

	// Update metrics
	sm.metrics.mu.Lock()
	sm.metrics.TotalJobsFailed++
	sm.metrics.ActiveJobs--
	sm.metrics.mu.Unlock()

	return nil
}

// CancelJob marks a job as cancelled
func (sm *StateManager) CancelJob(jobID string) error {
	sm.jobsMu.Lock()
	defer sm.jobsMu.Unlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	now := time.Now()
	job.State = JobStateCancelled
	job.CompletedAt = &now

	// Update metrics
	sm.metrics.mu.Lock()
	sm.metrics.TotalJobsCancelled++
	sm.metrics.ActiveJobs--
	sm.metrics.mu.Unlock()

	return nil
}

// GetJob retrieves a job by ID
func (sm *StateManager) GetJob(jobID string) (*ProcessingJob, error) {
	sm.jobsMu.RLock()
	defer sm.jobsMu.RUnlock()

	job, exists := sm.jobs[jobID]
	if !exists {
		return nil, fmt.Errorf("job %s not found", jobID)
	}

	return job, nil
}

// GetActiveJobs returns all active jobs
func (sm *StateManager) GetActiveJobs() []*ProcessingJob {
	sm.jobsMu.RLock()
	defer sm.jobsMu.RUnlock()

	var activeJobs []*ProcessingJob
	for _, job := range sm.jobs {
		if job.State == JobStateRunning || job.State == JobStatePending {
			activeJobs = append(activeJobs, job)
		}
	}

	return activeJobs
}

// GetJobsByWorker returns all jobs for a specific worker
func (sm *StateManager) GetJobsByWorker(workerID string) []*ProcessingJob {
	sm.jobsMu.RLock()
	defer sm.jobsMu.RUnlock()

	var workerJobs []*ProcessingJob
	for _, job := range sm.jobs {
		if job.WorkerID == workerID {
			workerJobs = append(workerJobs, job)
		}
	}

	return workerJobs
}

// GetJobsBySignalType returns all jobs for a specific signal type
func (sm *StateManager) GetJobsBySignalType(signalType string) []*ProcessingJob {
	sm.jobsMu.RLock()
	defer sm.jobsMu.RUnlock()

	var signalJobs []*ProcessingJob
	for _, job := range sm.jobs {
		if job.SignalType == signalType {
			signalJobs = append(signalJobs, job)
		}
	}

	return signalJobs
}

// GetMetrics returns the current state management metrics
func (sm *StateManager) GetMetrics() *StateMetrics {
	sm.metrics.mu.RLock()
	defer sm.metrics.mu.RUnlock()

	// Return a copy to avoid race conditions
	return &StateMetrics{
		TotalJobsCreated:    sm.metrics.TotalJobsCreated,
		TotalJobsCompleted:  sm.metrics.TotalJobsCompleted,
		TotalJobsFailed:     sm.metrics.TotalJobsFailed,
		TotalJobsCancelled:  sm.metrics.TotalJobsCancelled,
		AverageJobDuration:  sm.metrics.AverageJobDuration,
		TotalProcessingTime: sm.metrics.TotalProcessingTime,
		ActiveJobs:          sm.metrics.ActiveJobs,
	}
}

// CleanupOldJobs removes completed jobs older than the specified duration
func (sm *StateManager) CleanupOldJobs(olderThan time.Duration) int {
	sm.jobsMu.Lock()
	defer sm.jobsMu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	removed := 0

	for jobID, job := range sm.jobs {
		if job.CompletedAt != nil && job.CompletedAt.Before(cutoff) {
			delete(sm.jobs, jobID)
			removed++
		}
	}

	return removed
}

// generateJobID generates a unique job ID
func generateJobID() string {
	return fmt.Sprintf("job_%d", time.Now().UnixNano())
}
