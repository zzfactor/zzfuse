package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/schema"
)

// Only the batch database boundary is replaced. Other unexpected DB calls fail
// through the embedded nil interface rather than returning invented successes.
type observabilityDatabase struct {
	OtelDatabaseClient
	fetch func(context.Context) ([]map[string]interface{}, error)
	mark  func([]interface{}) error
}

func (db *observabilityDatabase) FetchUnembeddedSignalsAdvanced(ctx context.Context, _ string, _ int, _ int64, _ string, _ *FetchOptions, _ []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error) {
	signals, err := db.fetch(ctx)
	return signals, nil, err
}

func (db *observabilityDatabase) MarkSignalsEmbeddedAdvanced(_ context.Context, _ string, ids []interface{}, _ int64, _ []string, _ string) error {
	if db.mark != nil {
		return db.mark(ids)
	}
	return nil
}

type observabilityVectorStore struct {
	VectorStoreClient
	add func([]schema.Document) ([]string, error)
}

func (store *observabilityVectorStore) AddDocuments(_ context.Context, docs []schema.Document) ([]string, error) {
	return store.add(docs)
}

func observabilityWorker(db OtelDatabaseClient, store VectorStoreClient) *EmbeddingWorker {
	return NewEmbeddingWorker(db, util.NoopLogger{}, store,
		WithEmbeddingSchema("otel"), WithWorkerID("observability-worker"),
		WithConfigID(37), WithSignalType("logs"),
		WithMetadata(map[string]interface{}{"embedding_mode": "batch", "embedding_batch_interval": float64(0.01)}),
		WithVectorStoreConfigs(map[string]*VectorStoreConfig{
			"test-store": {ConfigID: 19, VectorStore: store},
		}))
}

func observabilitySignals() []map[string]interface{} {
	return []map[string]interface{}{
		{"id": "signal-one", "body": "first sample message"},
		{"id": "signal-two", "body": "second sample message"},
	}
}

func TestProductionMetricsTrackOutcomesOnceAndCopySnapshots(t *testing.T) {
	collector := NewEmbeddingMetricsCollector(37, "logs", "batch", "test", "test", "observability-worker")
	success := collector.RecordJobStart("observability-worker", 2, 4)
	failure := collector.RecordJobStart("observability-worker", 1, 2)
	cancelled := collector.RecordJobStart("observability-worker", 7, 14)
	require.Equal(t, int64(3), collector.GetMetrics().ActiveJobs)
	collector.RecordJobCompletion(success, true, nil)
	collector.RecordJobCompletion(failure, false, nil)
	collector.RecordJobCancellation("observability-worker")
	collector.RecordJobCompletion(success, true, nil)
	collector.RecordJobCompletion(cancelled, true, nil)
	collector.RecordJobCompletion(nil, false, nil)
	collector.RecordJobCompletion(&EmbeddingJobStats{}, true, nil)
	collector.RecordJobCancellation("observability-worker")
	snapshot := collector.GetMetrics()
	require.Equal(t, int64(3), snapshot.TotalJobsProcessed)
	require.Equal(t, int64(1), snapshot.SuccessfulJobs)
	require.Equal(t, int64(1), snapshot.FailedJobs)
	require.Equal(t, int64(1), snapshot.CancelledJobs)
	require.Zero(t, snapshot.ActiveJobs)
	require.Equal(t, int64(3), snapshot.TotalSignals)
	require.Equal(t, int64(6), snapshot.TotalTokens)
	require.Positive(t, snapshot.Uptime)
	require.False(t, success.EndTime.IsZero())
	snapshot.TotalSignals = 999
	snapshot.SignalType = "mutated"
	require.Equal(t, int64(3), collector.GetMetrics().TotalSignals)
	require.Equal(t, "logs", collector.GetMetrics().SignalType)
}

func TestProductionMetricsConcurrentAttemptsAndSnapshots(t *testing.T) {
	collector := NewEmbeddingMetricsCollector(37, "logs", "batch", "test", "test", "concurrent-worker")
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stats := collector.RecordJobStart("concurrent-worker", 2, 4)
			collector.GetMetrics().TotalSignals = -1
			collector.RecordJobCompletion(stats, i%2 == 0, nil)
			collector.RecordJobCompletion(stats, true, nil)
		}()
	}
	wg.Wait()
	metrics := collector.GetMetrics()
	require.Equal(t, int64(100), metrics.TotalJobsProcessed)
	require.Equal(t, int64(50), metrics.SuccessfulJobs)
	require.Equal(t, int64(50), metrics.FailedJobs)
	require.Equal(t, int64(200), metrics.TotalSignals)
	require.Equal(t, int64(400), metrics.TotalTokens)
	require.Zero(t, metrics.ActiveJobs)
}

func TestProductionRunBatchFailureIsNotSuccessful(t *testing.T) {
	for _, stage := range []string{"fetch", "vector-write", "receipt-write"} {
		t.Run(stage, func(t *testing.T) {
			failure := errors.New(stage + " failed")
			db := &observabilityDatabase{
				fetch: func(context.Context) ([]map[string]interface{}, error) {
					if stage == "fetch" {
						return nil, failure
					}
					return observabilitySignals(), nil
				},
				mark: func([]interface{}) error {
					if stage == "receipt-write" {
						return failure
					}
					return nil
				},
			}
			store := &observabilityVectorStore{add: func(docs []schema.Document) ([]string, error) {
				if stage == "vector-write" {
					return nil, failure
				}
				return make([]string, len(docs)), nil
			}}
			worker := observabilityWorker(db, store)
			require.ErrorIs(t, worker.RunBatch(context.Background()), failure)
			metrics := worker.GetMetrics()
			require.Equal(t, int64(1), metrics.TotalJobsProcessed)
			require.Equal(t, int64(1), metrics.FailedJobs)
			require.Zero(t, metrics.SuccessfulJobs)
			require.Zero(t, metrics.TotalSignals)
			require.Zero(t, metrics.TotalTokens)
			require.Zero(t, metrics.ActiveJobs)
			require.ErrorIs(t, worker.GetStatus().LastError, failure)
		})
	}
}

func TestProductionRunBatchPreservesSuccessfulChunksBeforeFailure(t *testing.T) {
	failure := errors.New("second chunk failed")
	db := &observabilityDatabase{fetch: func(context.Context) ([]map[string]interface{}, error) {
		return observabilitySignals(), nil
	}}
	writes := 0
	store := &observabilityVectorStore{add: func([]schema.Document) ([]string, error) {
		writes++
		if writes == 2 {
			return nil, failure
		}
		return []string{"vector-one"}, nil
	}}
	worker := observabilityWorker(db, store)
	worker.Metadata["embedding_batch_group_signals"] = true
	worker.MaxContextLength = 5
	worker.tokenizer = nil // Deterministic whitespace fallback makes two chunks.
	require.ErrorIs(t, worker.RunBatch(context.Background()), failure)
	metrics := worker.GetMetrics()
	require.Equal(t, int64(1), metrics.FailedJobs)
	require.Zero(t, metrics.SuccessfulJobs)
	require.Equal(t, int64(1), metrics.TotalSignals)
	require.Equal(t, int64(2), metrics.TotalTokens)
	require.Zero(t, metrics.ActiveJobs)
}

func TestProductionGroupedDocumentsPreserveMetadata(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(fmt.Sprintf("configured-store-%t", configured), func(t *testing.T) {
			db := &observabilityDatabase{fetch: func(context.Context) ([]map[string]interface{}, error) {
				signals := observabilitySignals()
				for _, signal := range signals {
					signal["target_vector_store"] = "test-store"
				}
				return signals, nil
			}}
			var document schema.Document
			store := &observabilityVectorStore{add: func(docs []schema.Document) ([]string, error) {
				require.Len(t, docs, 1)
				document = docs[0]
				return []string{"grouped-vector"}, nil
			}}
			worker := observabilityWorker(db, store)
			worker.Metadata["embedding_batch_group_signals"] = true
			worker.MaxContextLength = 100
			if !configured {
				worker.VectorStoreConfigs = nil
			}
			require.NoError(t, worker.RunBatch(context.Background()))
			require.NotNil(t, document.Metadata)
			internal, ok := document.Metadata["internal"].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, []string{"signal-one", "signal-two"}, internal[MetadataFieldGroupedIDs])
			require.Equal(t, "logs", internal[MetadataFieldSignalType])
			require.Equal(t, 2, internal[MetadataFieldSignalCount])
			if configured {
				require.Equal(t, int64(19), internal["vector_store_id"])
			}
			require.Equal(t, int64(2), worker.GetMetrics().TotalSignals)
		})
	}
}

func TestProductionRunBatchSerializesOverlappingCalls(t *testing.T) {
	entered, release := make(chan struct{}, 2), make(chan struct{}, 2)
	db := &observabilityDatabase{fetch: func(context.Context) ([]map[string]interface{}, error) {
		entered <- struct{}{}
		<-release
		return nil, nil
	}}
	worker := observabilityWorker(db, nil)
	results := make(chan error, 2)
	go func() { results <- worker.RunBatch(context.Background()) }()
	<-entered
	go func() { results <- worker.RunBatch(context.Background()) }()
	require.Equal(t, int64(1), worker.GetMetrics().ActiveJobs)
	select {
	case <-entered:
		t.Fatal("second call entered the current worker attempt")
	case <-time.After(20 * time.Millisecond):
	}
	release <- struct{}{}
	<-entered
	release <- struct{}{}
	require.NoError(t, <-results)
	require.NoError(t, <-results)
	require.Equal(t, int64(2), worker.GetMetrics().SuccessfulJobs)
	require.Zero(t, worker.GetMetrics().ActiveJobs)
}

type observabilityManagerDependencies struct{ pools map[string]*ServiceResources }

func (deps observabilityManagerDependencies) Pools() map[string]*ServiceResources { return deps.pools }
func (deps observabilityManagerDependencies) GetDatabase() (*sql.DB, error)       { return nil, nil }

func TestProductionWorkerSnapshotsReflectLiveManagerLifecycle(t *testing.T) {
	var calls atomic.Int64
	db := &observabilityDatabase{fetch: func(context.Context) ([]map[string]interface{}, error) {
		if calls.Add(1) == 1 {
			return observabilitySignals(), nil
		}
		return nil, nil
	}}
	store := &observabilityVectorStore{add: func(docs []schema.Document) ([]string, error) {
		ids := make([]string, len(docs))
		for i := range ids {
			ids[i] = fmt.Sprintf("vector-%d", i)
		}
		return ids, nil
	}}
	worker := observabilityWorker(db, store)
	manager := NewWorkerManager("otel", util.NoopLogger{})
	manager.workers = []*EmbeddingWorker{worker}
	swm := NewServiceWorkerManager(observabilityManagerDependencies{pools: map[string]*ServiceResources{
		"otel": {WorkerManager: manager}, "empty": {},
	}}, nil)
	manager.StartWorkers()
	t.Cleanup(manager.StopWorkers)
	require.Eventually(t, func() bool {
		snapshots := manager.GetWorkerSnapshots()
		return len(snapshots) == 1 && snapshots[0].Running && snapshots[0].ProcessedSignals == 2 && snapshots[0].ActiveJobs == 0
	}, time.Second, time.Millisecond)
	status := swm.GetWorkerStatus()
	workers := status["otel"]["workers"].([]WorkerSnapshot)
	require.Equal(t, "observability-worker", workers[0].ID)
	require.Equal(t, "otel", workers[0].SchemaID)
	require.Equal(t, int64(37), workers[0].EmbeddingConfigID)
	require.Equal(t, "logs", workers[0].SignalType)
	require.GreaterOrEqual(t, workers[0].SuccessfulJobs, int64(1))
	require.Zero(t, workers[0].FailedJobs)
	require.Equal(t, int64(4), workers[0].TokensUsed)
	require.Empty(t, workers[0].LastError)
	workers[0].ProcessedSignals = 999
	require.Equal(t, int64(2), manager.GetWorkerSnapshots()[0].ProcessedSignals)
	encoded, err := json.Marshal(status["empty"]["workers"])
	require.NoError(t, err)
	require.JSONEq(t, "[]", string(encoded))
	manager.StopWorkers()
	require.False(t, manager.GetWorkerSnapshots()[0].Running)
	require.False(t, manager.IsRunning())
}

func TestProductionWorkerManagerReloadKeepsRunning(t *testing.T) {
	db := &observabilityDatabase{fetch: func(context.Context) ([]map[string]interface{}, error) { return nil, nil }}
	store := &observabilityVectorStore{}
	worker := observabilityWorker(db, store)
	manager := NewWorkerManager("otel", util.NoopLogger{})
	manager.workers = []*EmbeddingWorker{worker}
	manager.StartWorkers()
	t.Cleanup(manager.StopWorkers)
	require.Eventually(t, func() bool { return worker.GetStatus().Running }, time.Second, time.Millisecond)
	require.NoError(t, manager.ReloadWorkers(context.Background(), []*WorkerConfig{{
		VectorStoreKey: "test-store", SignalType: "logs", SchemaName: "otel",
		EmbeddingConfig: dbconfig.EmbeddingConfig{ID: 37},
		MappingConfig: dbconfig.ProcessorMappingConfig{ID: 19, Metadata: map[string]interface{}{
			"embedding_mode": "batch", "embedding_batch_interval": float64(0.01),
		}},
		VectorStore: &VectorStore{VectorStore: store}, DatabaseClient: db,
	}}))
	require.False(t, worker.GetStatus().Running)
	require.True(t, worker.IsStopped())
	require.Eventually(t, func() bool {
		snapshots := manager.GetWorkerSnapshots()
		return manager.IsRunning() && len(snapshots) == 1 && snapshots[0].Running
	}, time.Second, time.Millisecond)
}
