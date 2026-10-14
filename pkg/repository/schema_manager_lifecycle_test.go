package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/require"
)

// Configuration boundaries are replaced, but worker construction, loops,
// cancellation and status snapshots all use the production manager path.
type schemaLifecycleProcessors struct {
	dbconfig.ProcessorConfigInterface
	load func(context.Context) ([]*dbconfig.ProcessorMappingConfig, error)
}

func (p schemaLifecycleProcessors) GetAllEnabled(ctx context.Context) ([]*dbconfig.ProcessorMappingConfig, error) {
	return p.load(ctx)
}

type schemaLifecycleVectorConfig struct {
	dbconfig.VectorStoreConfigInterface
}

func (schemaLifecycleVectorConfig) GetByID(_ context.Context, id int64) (*dbconfig.VectorStoreConfig, error) {
	if id != 22 {
		return nil, fmt.Errorf("unexpected vector config %d", id)
	}
	return &dbconfig.VectorStoreConfig{ID: 22, Name: "store", EmbeddingID: 37}, nil
}

type schemaLifecycleEmbeddingConfig struct {
	dbconfig.EmbeddingConfigInterface
}

func (schemaLifecycleEmbeddingConfig) GetByID(_ context.Context, id int64) (*dbconfig.EmbeddingConfig, error) {
	if id != 37 {
		return nil, fmt.Errorf("unexpected embedding config %d", id)
	}
	return &dbconfig.EmbeddingConfig{ID: 37, Name: "embedding"}, nil
}

type schemaLifecycleSchemaConfig struct{ dbconfig.SchemaConfigInterface }

func (schemaLifecycleSchemaConfig) GetAllEnabled(context.Context) ([]*dbconfig.DatabaseSchemaConfig, error) {
	return []*dbconfig.DatabaseSchemaConfig{{ID: 1, DatabaseSchema: "otel", Enabled: true}}, nil
}
func (schemaLifecycleSchemaConfig) CreateSchema(context.Context, string) error { return nil }

type schemaLifecycleStore struct {
	VectorStoreClient
	closed  atomic.Int64
	onClose func() error
	health  func(context.Context) error
}

func (*schemaLifecycleStore) SupportsEmbeddingClaims(*sql.DB) bool { return true }

func (s *schemaLifecycleStore) Close() error {
	s.closed.Add(1)
	if s.onClose != nil {
		return s.onClose()
	}
	return nil
}
func (s *schemaLifecycleStore) HealthCheck(ctx context.Context) error {
	if s.health != nil {
		return s.health(ctx)
	}
	return nil
}

type schemaLifecycleNoQueryDriver struct{}

func (schemaLifecycleNoQueryDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("lifecycle test unexpectedly attempted a database query")
}

var schemaLifecycleDriverOnce sync.Once

func schemaLifecycleFixture(t *testing.T) (*SchemaManager, *schemaLifecycleStore, *dbconfig.ProcessorMappingConfig) {
	t.Helper()
	schemaLifecycleDriverOnce.Do(func() { sql.Register("zzfuse-schema-lifecycle", schemaLifecycleNoQueryDriver{}) })
	db, err := sql.Open("zzfuse-schema-lifecycle", "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	mapping := &dbconfig.ProcessorMappingConfig{
		ID: 19, Name: "mapping", DatabaseID: 1, VectorStoreID: 22,
		EmbeddingFilter: map[string]interface{}{"signal_type": "logs"},
		// No DB query is due during lifecycle tests. Real batch loops still start.
		Metadata: map[string]interface{}{"embedding_mode": "batch", "embedding_batch_interval": float64(60)},
	}
	config := &ConfigManagers{
		Config: schemaLifecycleSchemaConfig{},
		Processor: schemaLifecycleProcessors{load: func(context.Context) ([]*dbconfig.ProcessorMappingConfig, error) {
			return []*dbconfig.ProcessorMappingConfig{mapping}, nil
		}},
		VectorStore: schemaLifecycleVectorConfig{}, Embedding: schemaLifecycleEmbeddingConfig{},
	}
	sm := NewSchemaManager(db, util.NoopLogger{}, config, nil)
	store := &schemaLifecycleStore{}
	// Resource initialization is separate from explicit worker execution.
	sm.pools["otel"] = &ServiceResources{
		SchemaID: 1, VectorStore: map[string]*VectorStore{"mapping_store": {VectorStore: store}}, logger: util.NoopLogger{},
	}
	sm.running = true
	t.Cleanup(func() { require.NoError(t, sm.Stop(context.Background())) })
	return sm, store, mapping
}

func schemaLifecycleSnapshots(t *testing.T, sm *SchemaManager) []WorkerSnapshot {
	t.Helper()
	workers, ok := sm.GetWorkerStatus()["otel"]["workers"].([]WorkerSnapshot)
	require.True(t, ok, "worker status must expose typed production snapshots")
	return workers
}

func schemaLifecycleWaitRunning(t *testing.T, sm *SchemaManager) {
	t.Helper()
	require.Eventually(t, func() bool {
		workers := schemaLifecycleSnapshots(t, sm)
		return len(workers) == 1 && workers[0].Running
	}, 2*time.Second, time.Millisecond)
}

func TestSchemaManagerInitializationAndReloadDoNotAutoStart(t *testing.T) {
	sm, _, _ := schemaLifecycleFixture(t)
	require.NotNil(t, sm.workerManager)
	require.Same(t, sm, sm.workerManager.deps)
	require.Empty(t, schemaLifecycleSnapshots(t, sm))
	require.NoError(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
	workers := schemaLifecycleSnapshots(t, sm)
	require.Len(t, workers, 1)
	require.False(t, workers[0].Running)
	require.Equal(t, "otel", workers[0].SchemaID)
	require.Equal(t, "logs", workers[0].SignalType)
	require.Zero(t, workers[0].TotalJobsProcessed)
	require.NoError(t, sm.StartAllWorkers())
	schemaLifecycleWaitRunning(t, sm)
	require.NoError(t, sm.StopAllWorkers())
	require.False(t, schemaLifecycleSnapshots(t, sm)[0].Running)
	old := sm.pools["otel"].WorkerManager.workers[0]
	require.True(t, old.IsStopped())
	require.NoError(t, sm.StartAllWorkers())
	schemaLifecycleWaitRunning(t, sm)
	require.NotSame(t, old, sm.pools["otel"].WorkerManager.workers[0], "explicit restart must rebuild cancelled workers")
}

func TestSchemaManagerStartInitializesPoolsWithoutStartingWorkers(t *testing.T) {
	sm, _, _ := schemaLifecycleFixture(t)
	require.NoError(t, sm.Stop(context.Background()))
	sm.configMgrs.Processor = schemaLifecycleProcessors{load: func(context.Context) ([]*dbconfig.ProcessorMappingConfig, error) {
		return nil, nil
	}}
	require.NoError(t, sm.Start(context.Background()))
	require.True(t, sm.IsRunning())
	resources, exists := sm.GetSchemaResources("otel")
	require.True(t, exists)
	require.Nil(t, resources.WorkerManager)
	require.Empty(t, schemaLifecycleSnapshots(t, sm))
}

func TestSchemaManagerExplicitStartsConstructWorkersLazily(t *testing.T) {
	for _, method := range []string{"schema", "on-demand-default-signals"} {
		t.Run(method, func(t *testing.T) {
			sm, _, _ := schemaLifecycleFixture(t)
			if method == "schema" {
				require.NoError(t, sm.StartSchema(context.Background(), "otel"))
			} else {
				require.NoError(t, sm.StartOnDemandWorkers(context.Background(), OnDemandParams{Schema: "otel"}))
			}
			schemaLifecycleWaitRunning(t, sm)
			require.NoError(t, sm.StopSchema(context.Background(), "otel"))
			require.False(t, schemaLifecycleSnapshots(t, sm)[0].Running)
		})
	}
}

func TestSchemaManagerReloadIsolatesTenantMappingsWithRepeatedKeys(t *testing.T) {
	sm, _, mapping := schemaLifecycleFixture(t)
	foreign := *mapping
	foreign.DatabaseID, foreign.ID = 2, 99
	sm.configMgrs.Processor = schemaLifecycleProcessors{load: func(context.Context) ([]*dbconfig.ProcessorMappingConfig, error) {
		return []*dbconfig.ProcessorMappingConfig{&foreign, mapping}, nil
	}}
	require.NoError(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
	workers := schemaLifecycleSnapshots(t, sm)
	require.Len(t, workers, 1)
	require.Equal(t, "otel", workers[0].SchemaID)
	require.Equal(t, int64(19), sm.pools["otel"].WorkerManager.configs[0].MappingConfig.ID)
}

func TestSchemaManagerStatusAndWorkerTransitionsAreConcurrentSafe(t *testing.T) {
	sm, _, _ := schemaLifecycleFixture(t)
	var wg sync.WaitGroup
	errors := make(chan error, 3)
	for _, operation := range []func() error{
		func() error { return sm.ReloadSchemaWorkers(context.Background(), "otel") },
		sm.StartAllWorkers, sm.StopAllWorkers,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 10 {
				if err := operation(); err != nil {
					errors <- err
					return
				}
			}
		}()
	}
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				_ = sm.GetWorkerStatus()
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	require.NoError(t, sm.StopAllWorkers())
	for _, worker := range schemaLifecycleSnapshots(t, sm) {
		require.False(t, worker.Running)
	}
}

func TestSchemaManagerStopDrainsAcceptedOperationsBeforeClosingProviders(t *testing.T) {
	sm, store, mapping := schemaLifecycleFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var finished atomic.Bool
	sm.configMgrs.Processor = schemaLifecycleProcessors{load: func(context.Context) ([]*dbconfig.ProcessorMappingConfig, error) {
		close(entered)
		<-release
		// Both dependencies must remain accessible while Stop drains this call.
		_ = sm.Pools()
		_, err := sm.GetDatabase()
		finished.Store(true)
		return []*dbconfig.ProcessorMappingConfig{mapping}, err
	}}
	store.onClose = func() error {
		if !finished.Load() {
			return errors.New("provider closed before accepted operation finished")
		}
		_ = sm.Pools()
		_, err := sm.GetDatabase()
		return err
	}
	loaded, stopped := make(chan error, 1), make(chan error, 1)
	go func() { loaded <- sm.ReloadSchemaWorkers(context.Background(), "otel") }()
	<-entered
	go func() { stopped <- sm.Stop(context.Background()) }()
	require.Eventually(t, func() bool { return !sm.IsRunning() }, time.Second, time.Millisecond)
	require.Error(t, sm.StartSchema(context.Background(), "otel"), "closing gate must reject new operations")
	require.Zero(t, store.closed.Load())
	close(release)
	for _, completion := range []<-chan error{loaded, stopped} {
		select {
		case err := <-completion:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("shutdown deadlocked while draining pool dependencies")
		}
	}
	require.Equal(t, int64(1), store.closed.Load())
	require.Empty(t, sm.Pools())
	require.NoError(t, sm.Stop(context.Background()))
	require.Equal(t, int64(1), store.closed.Load())
}

func TestSchemaManagerStopDrainsHealthCallbackWithoutHoldingPoolLock(t *testing.T) {
	sm, store, _ := schemaLifecycleFixture(t)
	entered, release, checked := make(chan struct{}), make(chan struct{}), make(chan struct{})
	store.health = func(context.Context) error {
		close(entered)
		<-release
		_ = sm.Pools()
		_, err := sm.GetDatabase()
		return err
	}
	go func() { sm.HealthCheck(context.Background()); close(checked) }()
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- sm.Stop(context.Background()) }()
	require.Eventually(t, func() bool { return !sm.IsRunning() }, time.Second, time.Millisecond)
	require.Zero(t, store.closed.Load())
	close(release)
	select {
	case <-checked:
	case <-time.After(2 * time.Second):
		t.Fatal("health callback deadlocked while reading pool dependencies")
	}
	select {
	case err := <-stopped:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown failed to drain health callback")
	}
	require.Equal(t, int64(1), store.closed.Load())
}

func TestSchemaManagerStopClosesPartialInitialization(t *testing.T) {
	sm, store, _ := schemaLifecycleFixture(t)
	sm.running = false // Startup failed after a provider was created.
	require.NoError(t, sm.Stop(context.Background()))
	require.Equal(t, int64(1), store.closed.Load())
	require.Empty(t, sm.Pools())
}
