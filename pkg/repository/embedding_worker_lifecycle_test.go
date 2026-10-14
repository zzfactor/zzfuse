package repository

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/require"
)

func newLifecycleTestWorker() *BaseWorker {
	return NewBaseWorker(nil, util.NoopLogger{}, WithBaseWorkerID("lifecycle-worker"), WithBaseSignalType("logs"))
}

func TestBaseWorkerJobLifecycleMetrics(t *testing.T) {
	w := newLifecycleTestWorker()
	job, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	require.Len(t, w.GetActiveJobs(), 1)
	require.Equal(t, int64(1), w.GetStatistics().TotalJobsProcessed)
	require.Equal(t, "running", w.GetStatistics().WorkerMetrics[w.WorkerID].CurrentJob)

	w.RecordBatchStatistics(2, 100*time.Millisecond, 10, true)
	w.RecordBatchStatistics(3, 200*time.Millisecond, 20, false)
	metrics := w.GetStatistics()
	require.Zero(t, metrics.SuccessfulJobs)
	require.Zero(t, metrics.FailedJobs)
	require.Equal(t, int64(1), metrics.TotalJobsProcessed)
	require.Equal(t, int64(5), metrics.TotalSignalsProcessed)
	require.Equal(t, int64(2), metrics.TotalBatchesProcessed)
	require.Equal(t, int64(30), metrics.TotalTokensProcessed)
	require.Equal(t, 300*time.Millisecond, metrics.TotalProcessingTime)

	require.NoError(t, w.CompleteJob(job.ID, &JobStatistics{
		SignalCount: 5, BatchCount: 2, TokenCount: 30, ErrorCount: 1, TotalTime: time.Second,
	}))
	metrics = w.GetStatistics()
	require.Equal(t, int64(1), metrics.TotalJobsProcessed)
	require.Equal(t, int64(1), metrics.SuccessfulJobs)
	require.Zero(t, metrics.FailedJobs)
	require.Equal(t, int64(5), metrics.TotalSignalsProcessed)
	require.Equal(t, int64(2), metrics.TotalBatchesProcessed)
	require.Equal(t, int64(30), metrics.TotalTokensProcessed)
	require.Equal(t, int64(1), metrics.TotalErrors)
	require.Equal(t, time.Second, metrics.TotalProcessingTime)
	require.Equal(t, time.Second, metrics.WorkerMetrics[w.WorkerID].TotalTime)
	require.Equal(t, "idle", metrics.WorkerMetrics[w.WorkerID].CurrentJob)
	require.Empty(t, w.GetActiveJobs())
	completed, err := w.GetJobStatus(job.ID)
	require.NoError(t, err)
	require.Equal(t, JobStateCompleted, completed.State)
	require.NotNil(t, completed.CompletedAt)

	require.Error(t, w.CompleteJob(job.ID, nil))
	require.Error(t, w.FailJob(job.ID, errors.New("late failure")))
	require.Equal(t, int64(1), w.GetStatistics().SuccessfulJobs)
	require.Zero(t, w.GetStatistics().FailedJobs)

	failed, err := w.StartJob(context.Background(), "trigger")
	require.NoError(t, err)
	require.Error(t, w.FailJob(failed.ID, nil))
	require.Len(t, w.GetActiveJobs(), 1)
	require.NoError(t, w.FailJob(failed.ID, errors.New("embedding failed")))
	metrics = w.GetStatistics()
	require.Equal(t, int64(2), metrics.TotalJobsProcessed)
	require.Equal(t, int64(1), metrics.SuccessfulJobs)
	require.Equal(t, int64(1), metrics.FailedJobs)
	require.Equal(t, int64(2), metrics.TotalErrors)
	require.Empty(t, w.GetActiveJobs())
	failed, err = w.GetJobStatus(failed.ID)
	require.NoError(t, err)
	require.Equal(t, JobStateFailed, failed.State)
	require.Equal(t, "embedding failed", failed.Error.Details)
}

func TestBaseWorkerBatchMetricsWithoutJob(t *testing.T) {
	w := newLifecycleTestWorker()
	w.RecordBatchStatistics(4, 250*time.Millisecond, 8, false)
	metrics := w.GetStatistics()
	require.Zero(t, metrics.TotalJobsProcessed)
	require.Zero(t, metrics.SuccessfulJobs)
	require.Zero(t, metrics.FailedJobs)
	require.Equal(t, int64(4), metrics.TotalSignalsProcessed)
	require.Equal(t, int64(8), metrics.TotalTokensProcessed)
	require.Equal(t, int64(1), metrics.TotalErrors)
	require.Equal(t, 250*time.Millisecond, metrics.TotalProcessingTime)
}

func TestBaseWorkerStopBeforeStart(t *testing.T) {
	w := newLifecycleTestWorker()
	require.False(t, w.IsStopped())
	w.Stop()
	require.True(t, w.IsStopped())
	require.False(t, w.GetStatus().Running)
	require.NotPanics(t, w.Stop)
}

func TestBaseWorkerConcurrentJobsReconcileTheirOwnBatches(t *testing.T) {
	w := newLifecycleTestWorker()
	first, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	second, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	require.NoError(t, w.RecordJobBatchStatistics(first.ID, 5, 300*time.Millisecond, 10, true))
	require.NoError(t, w.RecordJobBatchStatistics(second.ID, 7, 400*time.Millisecond, 14, true))
	require.NoError(t, w.CompleteJob(second.ID, &JobStatistics{
		SignalCount: 7, BatchCount: 1, TokenCount: 14, TotalTime: time.Second,
	}))
	require.Equal(t, int64(12), w.GetStatistics().TotalSignalsProcessed)
	require.Equal(t, "running", w.GetStatistics().WorkerMetrics[w.WorkerID].CurrentJob)
	require.Len(t, w.GetActiveJobs(), 1)
	require.NoError(t, w.CompleteJob(first.ID, &JobStatistics{
		SignalCount: 5, BatchCount: 1, TokenCount: 10, TotalTime: 500 * time.Millisecond,
	}))
	metrics := w.GetStatistics()
	require.Equal(t, int64(2), metrics.TotalJobsProcessed)
	require.Equal(t, int64(2), metrics.SuccessfulJobs)
	require.Equal(t, int64(12), metrics.TotalSignalsProcessed)
	require.Equal(t, int64(2), metrics.TotalBatchesProcessed)
	require.Equal(t, int64(24), metrics.TotalTokensProcessed)
	require.Equal(t, 1500*time.Millisecond, metrics.TotalProcessingTime)
}

func TestBaseWorkerFailedOverlappingJobKeepsItsBatchTotals(t *testing.T) {
	for _, attribute := range []bool{false, true} {
		name := "unattributed batch"
		if attribute {
			name = "explicit job batch"
		}
		t.Run(name, func(t *testing.T) {
			w := newLifecycleTestWorker()
			first, err := w.StartJob(context.Background(), "batch")
			require.NoError(t, err)
			second, err := w.StartJob(context.Background(), "batch")
			require.NoError(t, err)
			if attribute {
				require.NoError(t, w.RecordJobBatchStatistics(first.ID, 5, time.Millisecond, 10, true))
			} else {
				w.RecordBatchStatistics(5, time.Millisecond, 10, true)
			}
			require.NoError(t, w.CompleteJob(second.ID, &JobStatistics{
				SignalCount: 7, BatchCount: 1, TokenCount: 14,
			}))
			require.NoError(t, w.FailJob(first.ID, errors.New("failed after its batch")))
			metrics := w.GetStatistics()
			require.Equal(t, int64(12), metrics.TotalSignalsProcessed)
			require.Equal(t, int64(2), metrics.TotalBatchesProcessed)
			require.Equal(t, int64(24), metrics.TotalTokensProcessed)
			require.Equal(t, int64(2), metrics.TotalJobsProcessed)
			require.Equal(t, int64(1), metrics.SuccessfulJobs)
			require.Equal(t, int64(1), metrics.FailedJobs)
		})
	}
}

func TestBaseWorkerBatchRejectsTerminalOrUnknownJob(t *testing.T) {
	w := newLifecycleTestWorker()
	require.Error(t, w.RecordJobBatchStatistics("unknown", 5, time.Second, 10, true))
	job, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	require.NoError(t, w.CompleteJob(job.ID, nil))
	require.Error(t, w.RecordJobBatchStatistics(job.ID, 5, time.Second, 10, true))
	require.Zero(t, w.GetStatistics().TotalSignalsProcessed)
}

func TestBaseWorkerResetKeepsActiveJobBaseline(t *testing.T) {
	w := newLifecycleTestWorker()
	first, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	second, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	w.ResetStatistics()
	metrics := w.GetStatistics()
	require.Equal(t, int64(2), metrics.TotalJobsProcessed)
	require.Zero(t, metrics.SuccessfulJobs)
	require.Zero(t, metrics.FailedJobs)
	require.Equal(t, int64(2), metrics.WorkerMetrics[w.WorkerID].TotalJobs)
	require.Equal(t, "running", metrics.WorkerMetrics[w.WorkerID].CurrentJob)
	require.Len(t, w.GetActiveJobs(), 2)
	require.NoError(t, w.CompleteJob(first.ID, &JobStatistics{SignalCount: 1}))
	require.NoError(t, w.FailJob(second.ID, errors.New("failed after reset")))
	metrics = w.GetStatistics()
	require.Equal(t, int64(2), metrics.TotalJobsProcessed)
	require.Equal(t, int64(1), metrics.SuccessfulJobs)
	require.Equal(t, int64(1), metrics.FailedJobs)
	require.Equal(t, "idle", metrics.WorkerMetrics[w.WorkerID].CurrentJob)
}

func TestBaseWorkerResetExcludesEarlierAttributedWork(t *testing.T) {
	w := newLifecycleTestWorker()
	job, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	started := *job.StartedAt
	require.NoError(t, w.RecordJobBatchStatistics(job.ID, 5, time.Second, 10, false))
	w.ResetStatistics()
	require.NoError(t, w.RecordJobBatchStatistics(job.ID, 3, 2*time.Second, 6, true))
	w.ResetStatistics()
	require.NoError(t, w.RecordJobBatchStatistics(job.ID, 2, time.Second, 4, true))
	require.NoError(t, w.CompleteJob(job.ID, &JobStatistics{
		SignalCount: 10, BatchCount: 3, TokenCount: 20, ErrorCount: 1, TotalTime: 5 * time.Second,
	}))
	metrics := w.GetStatistics()
	require.Equal(t, int64(1), metrics.TotalJobsProcessed)
	require.Equal(t, int64(1), metrics.SuccessfulJobs)
	require.Equal(t, int64(2), metrics.TotalSignalsProcessed)
	require.Equal(t, int64(1), metrics.TotalBatchesProcessed)
	require.Equal(t, int64(4), metrics.TotalTokensProcessed)
	require.Zero(t, metrics.TotalErrors)
	require.Equal(t, 2*time.Second, metrics.TotalProcessingTime)
	status, err := w.GetJobStatus(job.ID)
	require.NoError(t, err)
	require.Equal(t, started, *status.StartedAt)
	require.Equal(t, 10, status.Statistics.SignalCount)
}

func TestBaseWorkerConcurrentCompletionRecordsOneOutcome(t *testing.T) {
	w := newLifecycleTestWorker()
	job, err := w.StartJob(context.Background(), "batch")
	require.NoError(t, err)
	var completed atomic.Int32
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			var err error
			if i%2 == 0 {
				err = w.CompleteJob(job.ID, nil)
			} else {
				err = w.FailJob(job.ID, errors.New("failed"))
			}
			if err == nil {
				completed.Add(1)
			}
		})
	}
	wg.Wait()
	require.Equal(t, int32(1), completed.Load())
	metrics := w.GetStatistics()
	require.Equal(t, int64(1), metrics.TotalJobsProcessed)
	require.Equal(t, int64(1), metrics.SuccessfulJobs+metrics.FailedJobs)
	require.Empty(t, w.GetActiveJobs())
}
