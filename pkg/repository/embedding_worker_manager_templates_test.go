package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/stretchr/testify/require"
	"github.com/tmc/langchaingo/schema"
)

type managerTemplateRepository struct {
	dbconfig.EmbeddingConfigTemplateInterface
	load func(context.Context, int64) (*dbconfig.EmbeddingConfigTemplate, error)
}

func (r managerTemplateRepository) GetByID(ctx context.Context, id int64) (*dbconfig.EmbeddingConfigTemplate, error) {
	return r.load(ctx, id)
}

type managerTemplateDatabase struct {
	OtelDatabaseClient
	fetch func([]SignalFieldConfig) ([]map[string]interface{}, error)
	mark  func([]interface{}, []string) error
}

func (db managerTemplateDatabase) FetchUnembeddedSignalsAdvanced(_ context.Context, signal string, _ int, configID int64, schemaName string, _ *FetchOptions, fields []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error) {
	if signal != "logs" || configID != 19 || schemaName != "otel" {
		return nil, nil, fmt.Errorf("unexpected query scope %s/%d/%s", signal, configID, schemaName)
	}
	signals, err := db.fetch(fields)
	return signals, nil, err
}
func (db managerTemplateDatabase) MarkSignalsEmbeddedAdvanced(_ context.Context, _ string, ids []interface{}, _ int64, vectors []string, _ string) error {
	return db.mark(ids, vectors)
}

func managerLogTemplate() *dbconfig.EmbeddingConfigTemplate {
	return &dbconfig.EmbeddingConfigTemplate{
		ID: 57, Content: map[string]string{"logs": "{{.body}}"},
		TemplateMetadata: map[string][]string{
			"generic": {"service_name", "timestamp_ns", "body"},
			"logs":    {"body"},
		},
	}
}

func TestWorkerManagerConfiguredTemplateFetchesAndEmbedsOriginalBodyAndContext(t *testing.T) {
	sm, _, mapping := schemaLifecycleFixture(t)
	mapping.TemplateID = 57
	tpl := managerLogTemplate()
	sm.configMgrs.Template = managerTemplateRepository{load: func(_ context.Context, id int64) (*dbconfig.EmbeddingConfigTemplate, error) {
		require.Equal(t, int64(57), id)
		return tpl, nil
	}}
	require.NoError(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
	manager := sm.pools["otel"].WorkerManager
	require.False(t, manager.IsRunning(), "loading/compiling a template must not start workers")
	require.Len(t, manager.workers, 1)
	worker := manager.workers[0]
	plan := worker.VectorStoreConfigs["mapping_store"]
	require.NotNil(t, plan.ContentEngine)
	require.Equal(t, "{{.body}}", plan.TemplateContent["logs"])
	require.ElementsMatch(t, []string{"body", "service_name", "timestamp_ns"}, plan.TemplateMetadata["logs"])
	// Loading creates an immutable worker plan independent of repository objects.
	tpl.Content["logs"] = "mutated repository object"
	tpl.TemplateMetadata["logs"][0] = "mutated"
	tpl.TemplateMetadata["generic"][0] = "mutated"
	require.Equal(t, "{{.body}}", plan.TemplateContent["logs"])
	require.ElementsMatch(t, []string{"body", "service_name", "timestamp_ns"}, plan.TemplateMetadata["logs"])

	const timestamp int64 = 1_791_986_400_123_456_789
	var documents []schema.Document
	worker.VectorStoreConfigs["mapping_store"].VectorStore = &observabilityVectorStore{add: func(docs []schema.Document) ([]string, error) {
		documents = append(documents, docs...)
		return []string{"vector-one", "vector-two"}, nil
	}}
	marked := false
	worker.DB = managerTemplateDatabase{
		fetch: func(fields []SignalFieldConfig) ([]map[string]interface{}, error) {
			selected := make(map[string]bool, len(fields))
			for _, field := range fields {
				selected[field.SignalKey] = true
			}
			for _, key := range []string{"body", "service_name", "timestamp_ns"} {
				if !selected[key] {
					return nil, fmt.Errorf("configured template/context field %s omitted from query", key)
				}
			}
			return []map[string]interface{}{
				{"id": int64(1), "body": "first original log body", "service_name": "template-service", "timestamp_ns": timestamp},
				{"id": int64(2), "body": "second original log body", "service_name": "template-service", "timestamp_ns": timestamp + 1},
			}, nil
		},
		mark: func(ids []interface{}, vectors []string) error {
			marked = true
			require.Equal(t, []interface{}{int64(1), int64(2)}, ids)
			require.Equal(t, []string{"vector-one", "vector-two"}, vectors)
			return nil
		},
	}
	require.NoError(t, worker.RunBatch(context.Background()))
	require.True(t, marked)
	require.Len(t, documents, 2)
	for i, document := range documents {
		require.Equal(t, []string{"first original log body", "second original log body"}[i], document.PageContent)
		require.Equal(t, document.PageContent, document.Metadata["body"])
		require.Equal(t, "template-service", document.Metadata["service_name"])
		require.Equal(t, timestamp+int64(i), document.Metadata["timestamp_ns"])
	}
	snapshot := manager.GetWorkerSnapshots()[0]
	require.Equal(t, int64(1), snapshot.SuccessfulJobs)
	require.Zero(t, snapshot.FailedJobs)
	require.Equal(t, int64(2), snapshot.ProcessedSignals)
	require.Positive(t, snapshot.TokensUsed)
}

func TestWorkerManagerTemplateErrorsPreserveExistingWorkers(t *testing.T) {
	for _, failure := range []string{"repository-missing", "load-error", "not-found", "invalid-template", "signal-template-missing"} {
		t.Run(failure, func(t *testing.T) {
			sm, _, mapping := schemaLifecycleFixture(t)
			require.NoError(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
			previous := sm.pools["otel"].WorkerManager.workers[0]
			mapping.TemplateID = 57
			if failure != "repository-missing" {
				sm.configMgrs.Template = managerTemplateRepository{load: func(context.Context, int64) (*dbconfig.EmbeddingConfigTemplate, error) {
					switch failure {
					case "load-error":
						return nil, errors.New("template query failed")
					case "not-found":
						return nil, nil
					case "invalid-template":
						tpl := managerLogTemplate()
						tpl.Content["logs"] = "{{if .body}}"
						return tpl, nil
					default:
						return &dbconfig.EmbeddingConfigTemplate{ID: 57, Content: map[string]string{"profiles": "profile"}}, nil
					}
				}}
			}
			require.Error(t, sm.ReloadSchemaWorkers(context.Background(), "otel"))
			require.Same(t, previous, sm.pools["otel"].WorkerManager.workers[0])
			require.False(t, previous.IsStopped())
		})
	}
}

type managerFailingVectorConfig struct {
	dbconfig.VectorStoreConfigInterface
}

func (managerFailingVectorConfig) GetByID(context.Context, int64) (*dbconfig.VectorStoreConfig, error) {
	return nil, errors.New("vector config query failed")
}

func TestWorkerManagerVectorConfigLoadErrorDoesNotDereferenceNil(t *testing.T) {
	sm, _, _ := schemaLifecycleFixture(t)
	sm.configMgrs.VectorStore = managerFailingVectorConfig{}
	require.NotPanics(t, func() { _ = sm.ReloadSchemaWorkers(context.Background(), "otel") })
}
