package repository

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/event"
	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/require"
)

type workerClaimsExternalStore struct{ VectorStoreClient }

func TestWorkerClaimsUnsupportedEffectsFailBeforeWorkerReplacement(t *testing.T) {
	for _, reason := range []string{"external-provider", "separate-db-pool", "template-plus"} {
		t.Run(reason, func(t *testing.T) {
			sm, _, mapping := schemaLifecycleFixture(t)
			require.NoError(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
			previous := sm.pools["otel"].WorkerManager.workers[0]
			switch reason {
			case "external-provider":
				sm.pools["otel"].VectorStore["mapping_store"].VectorStore = &workerClaimsExternalStore{}
			case "separate-db-pool":
				db, err := sql.Open("zzfuse-schema-lifecycle", "")
				require.NoError(t, err)
				defer db.Close()
				sm.pools["otel"].VectorStore["mapping_store"].VectorStore = &PGVectorCustomStore{db: db}
			case "template-plus":
				mapping.EmbeddingFilter["signal_type"] = SignalTypeSpans
				mapping.ProcessingMode = "template_plus"
			}
			require.Error(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
			require.Same(t, previous, sm.pools["otel"].WorkerManager.workers[0])
			require.False(t, previous.IsStopped())
			// Restore fixture provider so teardown never calls an absent external API.
			sm.pools["otel"].VectorStore["mapping_store"].VectorStore = &schemaLifecycleStore{}
		})
	}
}

func TestWorkerClaimsOnDemandBoundsAndRequestedBatchSizeReachWorker(t *testing.T) {
	sm, _, _ := schemaLifecycleFixture(t)
	start, stop := time.Now().Add(-time.Hour), time.Now()
	require.NoError(t, sm.StartOnDemandWorkers(context.Background(), OnDemandParams{Schema: "otel", StartTime: &start, StopTime: &stop, BatchSize: 2}))
	worker := sm.pools["otel"].WorkerManager.workers[0]
	require.Equal(t, 2, worker.BatchSize)
	require.Equal(t, start, *worker.FetchOptions.StartTime)
	require.Equal(t, stop, *worker.FetchOptions.StopTime)
	require.Len(t, worker.FetchOptions.ConfigurationVersion, 64)
	require.NoError(t, sm.StopAllWorkers())
	for _, params := range []OnDemandParams{
		{Schema: "otel", StartTime: &stop, StopTime: &start},
		{Schema: "otel", BatchSize: -1}, {Schema: "otel", BatchSize: 10001},
	} {
		require.Error(t, sm.StartOnDemandWorkers(context.Background(), params))
	}
}

func TestWorkerClaimsConfigurationFingerprintChangesWithTemplateAndMapping(t *testing.T) {
	sm, _, mapping := schemaLifecycleFixture(t)
	mapping.TemplateID = 57
	tpl := managerLogTemplate()
	sm.configMgrs.Template = managerTemplateRepository{load: func(context.Context, int64) (*dbconfig.EmbeddingConfigTemplate, error) { return tpl, nil }}
	version := func() string {
		require.NoError(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
		return sm.pools["otel"].WorkerManager.workers[0].FetchOptions.ConfigurationVersion
	}
	first := version()
	require.Equal(t, first, version())
	tpl.Content["logs"] = "prefix {{.body}}"
	second := version()
	require.NotEqual(t, first, second)
	mapping.Metadata["embedding_batch_group_signals"] = true
	require.NotEqual(t, second, version())
}

func TestWorkerClaimsMetricAliasCreatesPhysicalTableWorkers(t *testing.T) {
	sm, _, mapping := schemaLifecycleFixture(t)
	mapping.EmbeddingFilter["signal_type"] = "metrics"
	require.NoError(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
	workers := sm.pools["otel"].WorkerManager.workers
	require.Len(t, workers, 5)
	for _, worker := range workers {
		require.Contains(t, AllSupportedSignalTypes(), worker.SignalType)
	}
	require.NoError(t, sm.StartOnDemandWorkers(context.Background(), OnDemandParams{Schema: "otel", SignalTypes: []string{SignalTypeGaugeMetrics}}))
	workers = sm.pools["otel"].WorkerManager.workers
	require.Len(t, workers, 1)
	require.Equal(t, SignalTypeGaugeMetrics, workers[0].SignalType)
}

func TestWorkerClaimsTriggerPollsDatabaseWithoutNotifications(t *testing.T) {
	bus := event.NewInMemoryEventBus(4, 1)
	defer bus.Close()
	var polls atomic.Int64
	db := &observabilityDatabase{fetch: func(context.Context) ([]map[string]interface{}, error) { polls.Add(1); return nil, nil }}
	worker := NewEmbeddingWorker(db, util.NoopLogger{}, &schemaLifecycleStore{}, WithSignalType("logs"), WithWorkerID("notification-free-poll"),
		WithEventBus(bus), WithMetadata(map[string]interface{}{"embedding_mode": "trigger", "embedding_batch_interval": float64(.01)}))
	done := make(chan struct{})
	go func() { worker.Start(); close(done) }()
	require.Eventually(t, func() bool { return polls.Load() > 1 }, time.Second, time.Millisecond)
	worker.Stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("trigger poll did not drain on stop")
	}
}
