package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tmc/langchaingo/schema"
)

// MockEmbeddingDatabaseClient is a mock implementation for testing
type MockEmbeddingDatabaseClient struct {
	mock.Mock
}

// Rename existing methods to avoid conflicts
func (m *MockEmbeddingDatabaseClient) FetchUnembeddedSignalsAdvanced(ctx context.Context, tableName string, batchSize int, configID int64, schema string, options *FetchOptions, requiredFields []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error) {
	args := m.Called(ctx, tableName, batchSize, configID, schema, options, requiredFields)
	return args.Get(0).([]map[string]interface{}), args.Get(1).(*sql.Tx), args.Error(2)
}

func (m *MockEmbeddingDatabaseClient) FetchUnembeddedSignalsSimple(ctx context.Context, tableName string, batchSize int, filterConfig map[string]interface{}, configID int64) ([]map[string]interface{}, error) {
	args := m.Called(ctx, tableName, batchSize, filterConfig, configID)
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) MarkSignalsEmbeddedAdvanced(ctx context.Context, tableName string, ids []interface{}, configID int64, embeddedIDs []string, schema string) error {
	args := m.Called(ctx, tableName, ids, configID, embeddedIDs, schema)
	return args.Error(0)
}

// Implement OtelDatabaseClient interface methods
func (m *MockEmbeddingDatabaseClient) FetchUnembeddedSignals(ctx context.Context, configID int64, signalType string, limit int, schema string) ([]map[string]interface{}, error) {
	args := m.Called(ctx, configID, signalType, limit, schema)
	return args.Get(0).([]map[string]interface{}), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) MarkSignalsEmbedded(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	args := m.Called(ctx, configID, signalIDs, schema)
	return args.Error(0)
}

func (m *MockEmbeddingDatabaseClient) MarkSignalsEmbeddedBulk(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	args := m.Called(ctx, configID, signalIDs, schema)
	return args.Error(0)
}

func (m *MockEmbeddingDatabaseClient) FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error) {
	args := m.Called(ctx, traceID)
	return args.Get(0).([]SpanData), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error) {
	args := m.Called(ctx, traceID)
	return args.Get(0).([]LogData), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error) {
	args := m.Called(ctx, traceID)
	return args.Get(0).([]MetricData), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error) {
	args := m.Called(ctx, traceID)
	return args.Get(0).([]ProfileData), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error) {
	args := m.Called(ctx, spanID)
	return args.Get(0).([]LogData), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error) {
	args := m.Called(ctx, spanID, signalType)
	return args.Get(0).([]ExemplarData), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error) {
	args := m.Called(ctx, spanID)
	return args.Get(0).([]ProfileData), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QuerySpans(ctx context.Context, q TraceQuery) ([]TraceEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]TraceEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]LogEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) CountLogs(ctx context.Context, q LogQuery) (int64, error) {
	args := m.Called(ctx, q)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) CountMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	args := m.Called(ctx, q)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) CountMetricsGauge(ctx context.Context, q MetricQuery) (int64, error) {
	args := m.Called(ctx, q)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) CountMetricsHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	args := m.Called(ctx, q)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) CountMetricsSum(ctx context.Context, q MetricQuery) (int64, error) {
	args := m.Called(ctx, q)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) CountMetricsSummary(ctx context.Context, q MetricQuery) (int64, error) {
	args := m.Called(ctx, q)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) CountSpans(ctx context.Context, q TraceQuery) (int64, error) {
	args := m.Called(ctx, q)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) GetMetricByID(ctx context.Context, id int64, schema string) (interface{}, string, error) {
	args := m.Called(ctx, id, schema)
	return args.Get(0), args.String(1), args.Error(2)
}

func (m *MockEmbeddingDatabaseClient) GetLogByID(ctx context.Context, id int64, schema string) (LogEntry, error) {
	args := m.Called(ctx, id, schema)
	return args.Get(0).(LogEntry), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntry, error) {
	args := m.Called(ctx, id, schema)
	return args.Get(0).(TraceEntry), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QueryRuns(ctx context.Context, q RunQuery) ([]RunEntry, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]RunEntry), args.String(1), args.Bool(2), args.Error(3)
}

func (m *MockEmbeddingDatabaseClient) InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]int64), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error {
	args := m.Called(ctx, runID, metadata, schema)
	return args.Error(0)
}

func (m *MockEmbeddingDatabaseClient) EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error {
	args := m.Called(ctx, name, endTimestampNs, schema)
	return args.Error(0)
}

func (m *MockEmbeddingDatabaseClient) GetRunByID(ctx context.Context, id int64, schema string) (RunEntry, error) {
	args := m.Called(ctx, id, schema)
	return args.Get(0).(RunEntry), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) GetStatistics(ctx context.Context) (*WorkerMetrics, error) {
	args := m.Called(ctx)
	return args.Get(0).(*WorkerMetrics), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) GetJobStatus(ctx context.Context, jobID string) (*ProcessingJob, error) {
	args := m.Called(ctx, jobID)
	return args.Get(0).(*ProcessingJob), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) GetActiveJobs(ctx context.Context) ([]*ProcessingJob, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*ProcessingJob), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QueryGaugeMetrics(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]GaugeMetricEntry), args.String(1), args.Bool(2), args.Error(3)
}

func (m *MockEmbeddingDatabaseClient) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QuerySumMetrics(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]SumMetricEntry), args.String(1), args.Bool(2), args.Error(3)
}

func (m *MockEmbeddingDatabaseClient) InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QueryHistogramMetrics(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]HistogramMetricEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QuerySummaryMetrics(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]SummaryMetricEntry), args.String(1), args.Bool(2), args.Error(3)
}

func (m *MockEmbeddingDatabaseClient) InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QueryExponentialHistogramMetrics(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]ExponentialHistogramMetricEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QueryProfiles(ctx context.Context, q ProfileQuery) ([]ProfileEntry, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]ProfileEntry), args.String(1), args.Bool(2), args.Error(3)
}

func (m *MockEmbeddingDatabaseClient) InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error) {
	args := m.Called(ctx, entries, schema)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockEmbeddingDatabaseClient) QueryMetricsGauge(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]GaugeMetricEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) QueryMetricsSum(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]SumMetricEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) QueryMetricsHistogram(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]HistogramMetricEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) QueryMetricsSummary(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]SummaryMetricEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	args := m.Called(ctx, q)
	return args.Get(0).([]ExponentialHistogramMetricEntry), args.String(1), args.Bool(2), args.String(3), args.Bool(4), args.Error(5)
}

func (m *MockEmbeddingDatabaseClient) StartOnDemandWorkers(ctx context.Context, params OnDemandParams) error {
	args := m.Called(ctx, params)
	return args.Error(0)
}

func (m *MockEmbeddingDatabaseClient) StopOnDemandWorkers(ctx context.Context, schema string) error {
	args := m.Called(ctx, schema)
	return args.Error(0)
}

// MockVectorStoreClient is a mock implementation for testing
type MockVectorStoreClient struct {
	mock.Mock
}

func (m *MockVectorStoreClient) AddDocuments(ctx context.Context, docs []schema.Document) ([]string, error) {
	args := m.Called(ctx, docs)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockVectorStoreClient) SimilaritySearch(ctx context.Context, query string, k int, opts ...interface{}) ([]schema.Document, error) {
	args := m.Called(ctx, query, k, opts)
	return args.Get(0).([]schema.Document), args.Error(1)
}

func (m *MockVectorStoreClient) SimilaritySearchByVector(ctx context.Context, query []float32, k int, opts ...interface{}) ([]schema.Document, error) {
	args := m.Called(ctx, query, k, opts)
	return args.Get(0).([]schema.Document), args.Error(1)
}

func (m *MockVectorStoreClient) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockVectorStoreClient) Close() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockVectorStoreClient) GetVectorByID(ctx context.Context, id string) ([]float32, error) {
	args := m.Called(ctx, id)
	return args.Get(0).([]float32), args.Error(1)
}

func (m *MockVectorStoreClient) GetVectorsByIDs(ctx context.Context, ids []string) ([][]float32, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).([][]float32), args.Error(1)
}

func (m *MockVectorStoreClient) GetVectorMetadata(ctx context.Context, id string) (map[string]interface{}, error) {
	args := m.Called(ctx, id)
	return args.Get(0).(map[string]interface{}), args.Error(1)
}

func (m *MockVectorStoreClient) CombineVectors(ctx context.Context, vectors [][]float32, strategy string) ([]float32, error) {
	args := m.Called(ctx, vectors, strategy)
	return args.Get(0).([]float32), args.Error(1)
}

func (m *MockVectorStoreClient) GetVectorCombinationStrategy() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockVectorStoreClient) FindBaselineByContentHash(ctx context.Context, content string) ([]interface{}, error) {
	args := m.Called(ctx, content)
	return args.Get(0).([]interface{}), args.Error(1)
}

// MockLogger is a mock implementation for testing
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Info(args ...interface{}) {
	m.Called(args...)
}

func (m *MockLogger) Error(args ...interface{}) {
	m.Called(args...)
}

func (m *MockLogger) Debug(args ...interface{}) {
	m.Called(args...)
}

func TestEnhancedEmbeddingWorker(t *testing.T) {
	// Setup
	mockDB := &MockEmbeddingDatabaseClient{}
	mockVectorStore := &MockVectorStoreClient{}
	mockLogger := util.NoopLogger{}

	// Test 1: Create worker with enhanced features
	t.Run("CreateEnhancedWorker", func(t *testing.T) {
		worker := NewEmbeddingWorker(
			mockDB,
			mockLogger,
			mockVectorStore,
			WithBatchSize(50),
			WithSignalType("logs"),
			WithWorkerID("test-worker-1"),
			WithConfigID(123),
			WithMaxRetries(3),
			WithBatchErrorPolicy("continue"),
			WithFlushInterval(100*time.Millisecond),
			WithMaxConcurrentBatches(2),
			WithRetryBackoff(1*time.Second, 10*time.Second),
			WithMaxContextLength(1000),
			WithMetadata(map[string]interface{}{
				"embedding_mode":           "batch",
				"embedding_batch_interval": 5.0,
			}),
		)

		// Verify base worker configuration
		assert.NotNil(t, worker.BaseWorker)
		assert.Equal(t, 50, worker.BaseWorker.BatchSize)
		assert.Equal(t, "logs", worker.BaseWorker.SignalType)
		assert.Equal(t, "test-worker-1", worker.BaseWorker.WorkerID)
		assert.Equal(t, int64(123), worker.BaseWorker.ConfigID)
		assert.Equal(t, 3, worker.BaseWorker.MaxRetries)
		assert.Equal(t, "continue", worker.BaseWorker.BatchErrorPolicy)
		assert.Equal(t, 100*time.Millisecond, worker.BaseWorker.FlushInterval)
		assert.Equal(t, 2, worker.BaseWorker.MaxConcurrentBatches)
		assert.Equal(t, 1*time.Second, worker.BaseWorker.RetryBackoffBase)
		assert.Equal(t, 10*time.Second, worker.BaseWorker.RetryBackoffMax)

		// Verify embedding worker configuration
		assert.Equal(t, 1000, worker.MaxContextLength)
		assert.Equal(t, "batch", worker.ProcessingMode())
		assert.Equal(t, 5*time.Second, worker.BatchInterval())

		// Verify statistics collector is initialized
		assert.NotNil(t, worker.BaseWorker.statsCollector)
	})

	// Test 2: Job management functionality
	t.Run("JobManagement", func(t *testing.T) {
		worker := NewEmbeddingWorker(
			mockDB,
			mockLogger,
			mockVectorStore,
			WithSignalType("spans"),
			WithWorkerID("test-worker-2"),
		)

		ctx := context.Background()

		// Start a job
		job, err := worker.BaseWorker.StartJob(ctx, "batch")
		assert.NoError(t, err)
		assert.NotNil(t, job)
		assert.Equal(t, "batch", string(job.Type))
		assert.Equal(t, JobStateRunning, job.State)
		assert.Equal(t, "spans", job.SignalType)
		assert.Equal(t, "test-worker-2", job.WorkerID)

		// Update job progress
		progress := &JobProgress{
			CurrentStep:      "processing",
			StepNumber:       1,
			TotalSteps:       3,
			ProcessedSignals: 10,
			TotalSignals:     100,
		}
		err = worker.BaseWorker.UpdateJobProgress(job.ID, progress)
		assert.NoError(t, err)

		// Get job status
		retrievedJob, err := worker.BaseWorker.GetJobStatus(job.ID)
		assert.NoError(t, err)
		assert.Equal(t, job.ID, retrievedJob.ID)
		assert.Equal(t, progress.CurrentStep, retrievedJob.Progress.CurrentStep)

		// Complete job
		stats := &JobStatistics{
			TotalTime:   5 * time.Second,
			TokenCount:  500,
			SignalCount: 100,
			BatchCount:  5,
		}
		err = worker.BaseWorker.CompleteJob(job.ID, stats)
		assert.NoError(t, err)

		// Verify job is completed
		completedJob, err := worker.BaseWorker.GetJobStatus(job.ID)
		assert.NoError(t, err)
		assert.Equal(t, JobStateCompleted, completedJob.State)
		assert.NotNil(t, completedJob.CompletedAt)
		assert.Equal(t, stats.TokenCount, completedJob.Statistics.TokenCount)
	})

	// Test 3: Statistics collection
	t.Run("StatisticsCollection", func(t *testing.T) {
		worker := NewEmbeddingWorker(
			mockDB,
			mockLogger,
			mockVectorStore,
			WithSignalType("metrics"),
			WithWorkerID("test-worker-3"),
		)

		ctx := context.Background()

		// Start and complete jobs to track job-level statistics
		job1, _ := worker.BaseWorker.StartJob(ctx, "batch")
		worker.BaseWorker.CompleteJob(job1.ID, &JobStatistics{})

		job2, _ := worker.BaseWorker.StartJob(ctx, "batch")
		worker.BaseWorker.CompleteJob(job2.ID, &JobStatistics{})

		job3, _ := worker.BaseWorker.StartJob(ctx, "batch")
		worker.BaseWorker.FailJob(job3.ID, errors.New("test error"))

		// Record batch statistics for batch-level metrics
		worker.BaseWorker.RecordBatchStatistics(25, 2*time.Second, 300, true)
		worker.BaseWorker.RecordBatchStatistics(30, 3*time.Second, 400, true)
		worker.BaseWorker.RecordBatchStatistics(20, 1*time.Second, 200, false)

		// Get statistics
		stats := worker.BaseWorker.GetStatistics()
		assert.NotNil(t, stats)
		assert.Equal(t, int64(3), stats.TotalJobsProcessed)
		assert.Equal(t, int64(2), stats.SuccessfulJobs)
		assert.Equal(t, int64(1), stats.FailedJobs)
		assert.Equal(t, int64(75), stats.TotalSignalsProcessed) // 25+30+20
		assert.Equal(t, int64(900), stats.TotalTokensProcessed) // 300+400+200

		// Verify worker-specific stats
		workerStats := stats.WorkerMetrics["test-worker-3"]
		assert.NotNil(t, workerStats)
		assert.Equal(t, int64(3), workerStats.TotalJobs)

		// Verify signal type stats
		signalStats := stats.SignalTypeMetrics["metrics"]
		assert.NotNil(t, signalStats)
		assert.Equal(t, int64(3), signalStats.TotalProcessed)
	})

	// Test 4: State management and lifecycle
	t.Run("StateManagement", func(t *testing.T) {
		worker := NewEmbeddingWorker(
			mockDB,
			mockLogger,
			mockVectorStore,
			WithSignalType("logs"),
			WithWorkerID("test-worker-4"),
		)

		// Check initial state
		status := worker.BaseWorker.GetStatus()
		assert.NotNil(t, status)
		assert.False(t, status.Running)
		assert.Nil(t, status.LastError)

		// Test pause/resume
		worker.BaseWorker.Pause()
		assert.True(t, worker.BaseWorker.IsPaused())

		worker.BaseWorker.Resume()
		assert.False(t, worker.BaseWorker.IsPaused())

		// Test error reporting
		testError := assert.AnError
		worker.BaseWorker.ReportError(testError)

		status = worker.BaseWorker.GetStatus()
		assert.Equal(t, testError, status.LastError)
		assert.NotZero(t, status.LastRunTime)

		// Test stop
		worker.BaseWorker.Stop()
		assert.True(t, worker.BaseWorker.IsStopped())
	})

	// Test 5: Active jobs tracking
	t.Run("ActiveJobsTracking", func(t *testing.T) {
		worker := NewEmbeddingWorker(
			mockDB,
			mockLogger,
			mockVectorStore,
			WithSignalType("spans"),
			WithWorkerID("test-worker-5"),
		)

		ctx := context.Background()

		// Start multiple jobs
		job1, _ := worker.BaseWorker.StartJob(ctx, "batch")
		job2, _ := worker.BaseWorker.StartJob(ctx, "trigger")
		job3, _ := worker.BaseWorker.StartJob(ctx, "hybrid")

		// Get active jobs
		activeJobs := worker.BaseWorker.GetActiveJobs()
		assert.Len(t, activeJobs, 3)

		// Complete one job
		worker.BaseWorker.CompleteJob(job1.ID, &JobStatistics{})

		// Verify active jobs count decreased
		activeJobs = worker.BaseWorker.GetActiveJobs()
		assert.Len(t, activeJobs, 2)

		// Fail another job
		worker.BaseWorker.FailJob(job2.ID, assert.AnError)

		// Verify active jobs count decreased again
		activeJobs = worker.BaseWorker.GetActiveJobs()
		assert.Len(t, activeJobs, 1)
		assert.Equal(t, job3.ID, activeJobs[0].ID)
	})

	// Test 6: Statistics reset
	t.Run("StatisticsReset", func(t *testing.T) {
		worker := NewEmbeddingWorker(
			mockDB,
			mockLogger,
			mockVectorStore,
			WithSignalType("logs"),
			WithWorkerID("test-worker-6"),
		)

		// Record one completed job containing a batch.
		job, err := worker.BaseWorker.StartJob(context.Background(), "batch")
		assert.NoError(t, err)
		worker.BaseWorker.RecordBatchStatistics(10, time.Second, 100, true)
		assert.NoError(t, worker.BaseWorker.CompleteJob(job.ID, &JobStatistics{SignalCount: 10, TokenCount: 100}))

		// Verify statistics exist
		stats := worker.BaseWorker.GetStatistics()
		assert.Equal(t, int64(1), stats.TotalJobsProcessed)

		// Reset statistics
		worker.BaseWorker.ResetStatistics()

		// Verify statistics are reset
		stats = worker.BaseWorker.GetStatistics()
		assert.Equal(t, int64(0), stats.TotalJobsProcessed)
		assert.Equal(t, int64(0), stats.SuccessfulJobs)
		assert.Equal(t, int64(0), stats.FailedJobs)
	})

}

func TestEnhancedEmbeddingWorkerIntegration(t *testing.T) {
	// Test integration between all enhanced features
	t.Run("FullIntegration", func(t *testing.T) {
		mockDB := &MockEmbeddingDatabaseClient{}
		mockVectorStore := &MockVectorStoreClient{}
		mockLogger := util.NoopLogger{}

		// Create worker with all enhancements
		worker := NewEmbeddingWorker(
			mockDB,
			mockLogger,
			mockVectorStore,
			WithBatchSize(100),
			WithSignalType("logs"),
			WithWorkerID("integration-worker"),
			WithConfigID(456),
			WithMaxRetries(5),
			WithBatchErrorPolicy("retry"),
			WithFlushInterval(500*time.Millisecond),
			WithMaxConcurrentBatches(3),
			WithRetryBackoff(2*time.Second, 60*time.Second),
			WithMaxContextLength(2000),
			WithMetadata(map[string]interface{}{
				"embedding_mode":                "hybrid",
				"embedding_batch_interval":      float64(10),
				"embedding_batch_group_signals": true,
			}),
		)

		// Verify all enhancements are properly integrated
		assert.NotNil(t, worker.BaseWorker)
		assert.NotNil(t, worker.BaseWorker.statsCollector)
		assert.Equal(t, "hybrid", worker.ProcessingMode())
		assert.Equal(t, 10*time.Second, worker.BatchInterval())
		assert.True(t, worker.GroupSignals())

		// Test complete workflow
		ctx := context.Background()

		// Start a job
		job, err := worker.BaseWorker.StartJob(ctx, "hybrid")
		assert.NoError(t, err)

		// Record batch processing
		worker.BaseWorker.RecordBatchStatistics(50, 3*time.Second, 600, true)
		worker.BaseWorker.RecordBatchStatistics(75, 4*time.Second, 900, true)

		// Update job progress
		progress := &JobProgress{
			CurrentStep:      "embedding",
			StepNumber:       2,
			TotalSteps:       4,
			ProcessedSignals: 125,
			TotalSignals:     200,
		}
		worker.BaseWorker.UpdateJobProgress(job.ID, progress)

		// Complete job
		stats := &JobStatistics{
			TotalTime:   10 * time.Second,
			TokenCount:  1500,
			SignalCount: 125,
			BatchCount:  2,
		}
		worker.BaseWorker.CompleteJob(job.ID, stats)

		// Verify final state
		finalStats := worker.BaseWorker.GetStatistics()
		assert.Equal(t, int64(1), finalStats.TotalJobsProcessed) // One job contains two batches
		assert.Equal(t, int64(1), finalStats.SuccessfulJobs)
		assert.Equal(t, int64(0), finalStats.FailedJobs)

		completedJob, err := worker.BaseWorker.GetJobStatus(job.ID)
		assert.NoError(t, err)
		assert.Equal(t, JobStateCompleted, completedJob.State)
		assert.Equal(t, 125, completedJob.Progress.ProcessedSignals)
		assert.Equal(t, 1500, completedJob.Statistics.TokenCount)

	})
}
