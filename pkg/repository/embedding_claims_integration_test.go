//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/require"
)

func workerClaimFixture(t *testing.T) (*sql.DB, *OtelSQLDatabaseClient, *PGVectorCustomStore, string) {
	t.Helper()
	dsn := os.Getenv("TEST_WORKER_CLAIMS_DSN")
	if os.Getenv("TRAZR_IT") != "1" || dsn == "" {
		t.Skip("requires TRAZR_IT=1 and TEST_WORKER_CLAIMS_DSN for a disposable database")
	}
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(16)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	schema := fmt.Sprintf("worker_claim_%d", time.Now().UnixNano())
	_, err = db.ExecContext(context.Background(), "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	})
	initializer := &database.SchemaConfigRepository{DB: db}
	require.NoError(t, initializer.EnsureOtelDataTables(context.Background(), db, schema))
	store := &PGVectorCustomStore{db: db, schema: schema, table: "claim_vectors", embeddingDimension: 2, logger: util.NoopLogger{}}
	require.NoError(t, store.EnsureTable(context.Background()))
	return db, NewUnifiedSQLDatabaseClient(db), store, schema
}

func workerClaimInsert(t *testing.T, db *sql.DB, schema, table string, timestamp int64) string {
	t.Helper()
	var id string
	query := fmt.Sprintf(`INSERT INTO %s.%s(timestamp_ns,service_name,metadata) VALUES($1,'claim-service','{}') RETURNING id::text`, schema, table)
	if table == "profiles" {
		query = fmt.Sprintf(`INSERT INTO %s.profiles(timestamp_ns,service_name,profile_type,profile_bytes,metadata) VALUES($1,'claim-service','cpu','\x','{}') RETURNING id::text`, schema)
	}
	if table == "spans" {
		query = fmt.Sprintf(`INSERT INTO %s.spans(timestamp_ns,end_time,trace_id,span_id,name,service_name,metadata) VALUES($1,to_timestamp($1::bigint/1000000000.0),'trace','span','claim-span','claim-service','{}') RETURNING id::text`, schema)
	}
	if table == "sum_metrics" || table == "exponential_histogram_metrics" {
		query = fmt.Sprintf(`INSERT INTO %s.%s(timestamp_ns,name,service_name,metadata) VALUES($1,'claim.metric','claim-service','{}') RETURNING id::text`, schema, table)
	}
	if table == "exponential_histogram_metrics" {
		query = fmt.Sprintf(`INSERT INTO %s.exponential_histogram_metrics(timestamp_ns,start_time,name,service_name,metadata) VALUES($1,to_timestamp($1::bigint/1000000000.0),'claim.metric','claim-service','{}') RETURNING id::text`, schema)
	}
	err := db.QueryRowContext(context.Background(), query, timestamp).Scan(&id)
	require.NoError(t, err)
	return id
}

func claimOptions(version string) *FetchOptions {
	return &FetchOptions{OrderBy: "ASC", ConfigurationVersion: version, LeaseDuration: time.Minute}
}

func TestWorkerClaimsConcurrentProcessesAndTimeBounds(t *testing.T) {
	db, _, _, schema := workerClaimFixture(t)
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	for i := range 42 {
		workerClaimInsert(t, db, schema, "logs", base.Add(time.Duration(i)*time.Second).UnixNano())
	}
	start, stop := base.Add(time.Second), base.Add(41*time.Second)
	var seen sync.Map
	var count atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Independent clients model separate worker processes. No shared lock
			// or session-local claims participate in selection correctness.
			client := NewUnifiedSQLDatabaseClient(db)
			for range 12 {
				options := claimOptions("same-config")
				options.StartTime = &start
				options.StopTime = &stop
				signals, claims, err := client.ClaimEmbeddingSignals(context.Background(), "logs", 3, 19, schema, options, []SignalFieldConfig{{FieldName: "id", SignalKey: "id"}, {FieldName: "timestamp_ns", SignalKey: "timestamp_ns"}})
				if err != nil {
					errs <- err
					return
				}
				if len(signals) > 3 {
					errs <- errors.New("requested batch size was exceeded")
					return
				}
				for i, claim := range claims {
					if _, duplicate := seen.LoadOrStore(claim.SignalID, true); duplicate {
						errs <- errors.New("parallel workers acquired the same in-flight signal")
						return
					}
					ts := signals[i]["timestamp_ns"].(int64)
					if ts < start.UnixNano() || ts >= stop.UnixNano() {
						errs <- errors.New("worker selected outside requested time bounds")
						return
					}
					count.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int64(40), count.Load())
}

func TestWorkerClaimsAllPhysicalSignalTableBounds(t *testing.T) {
	db, client, _, schema := workerClaimFixture(t)
	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	start, stop := base.Add(time.Second), base.Add(2*time.Second)
	for _, table := range AllSupportedSignalTypes() {
		t.Run(table, func(t *testing.T) {
			workerClaimInsert(t, db, schema, table, base.UnixNano())
			wanted := workerClaimInsert(t, db, schema, table, start.UnixNano())
			workerClaimInsert(t, db, schema, table, stop.UnixNano())
			options := claimOptions("bounds")
			options.StartTime = &start
			options.StopTime = &stop
			_, claims, err := client.ClaimEmbeddingSignals(context.Background(), table, 1, 19, schema, options, []SignalFieldConfig{{FieldName: "id", SignalKey: "id"}})
			require.NoError(t, err)
			require.Len(t, claims, 1)
			require.Equal(t, wanted, claims[0].SignalID)
		})
	}
}

func TestWorkerClaimsCrashVectorReceiptRecoveryAndFencing(t *testing.T) {
	db, client, store, schema := workerClaimFixture(t)
	workerClaimInsert(t, db, schema, "logs", time.Now().UnixNano())
	fields := []SignalFieldConfig{{FieldName: "id", SignalKey: "id"}}
	_, first, err := client.ClaimEmbeddingSignals(context.Background(), "logs", 1, 19, schema, claimOptions("version-one"), fields)
	require.NoError(t, err)
	require.Len(t, first, 1)
	metadata := map[string]interface{}{claimMetadataKey: first, "service_name": "claim-service"}
	vector, err := store.InsertEmbeddingRaw(context.Background(), time.Now().UnixNano(), []float32{.1, .2}, "claim-service", "original grouped body", metadata)
	require.NoError(t, err)
	// Simulated process crash after the vector commit, before its receipt. A new
	// process takes over only after expiry; the durable vector reference survives.
	_, err = db.ExecContext(context.Background(), fmt.Sprintf(`UPDATE %s.embedding_worker_claims SET leased_until=clock_timestamp()-interval '1 second'`, schema))
	require.NoError(t, err)
	secondClient := NewUnifiedSQLDatabaseClient(db)
	_, second, err := secondClient.ClaimEmbeddingSignals(context.Background(), "logs", 1, 19, schema, claimOptions("version-one"), fields)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.NotEqual(t, first[0].Token, second[0].Token)
	require.Equal(t, vector, second[0].VectorID)
	_, err = store.InsertEmbeddingRaw(context.Background(), time.Now().UnixNano(), []float32{.9, .9}, "claim-service", "stale body", metadata)
	require.ErrorIs(t, err, ErrEmbeddingLeaseLost)
	require.ErrorIs(t, client.CompleteEmbeddingClaims(context.Background(), first, []string{fmt.Sprint(vector)}), ErrEmbeddingLeaseLost)
	require.ErrorIs(t, client.RenewEmbeddingClaims(context.Background(), first, time.Minute), ErrEmbeddingLeaseLost)
	require.NoError(t, client.ReleaseEmbeddingClaims(context.Background(), first))
	require.NoError(t, secondClient.CompleteEmbeddingClaims(context.Background(), second, []string{fmt.Sprint(vector)}))
	var vectorCount, receiptCount int
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.claim_vectors`, schema)).Scan(&vectorCount))
	require.Equal(t, 1, vectorCount)
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT jsonb_array_length(metadata->'embeddings') FROM %s.logs`, schema)).Scan(&receiptCount))
	require.Equal(t, 1, receiptCount)
	var leaks bool
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT metadata ? $1 FROM %s.claim_vectors`, schema), claimMetadataKey).Scan(&leaks))
	require.False(t, leaks)
	_, claims, err := client.ClaimEmbeddingSignals(context.Background(), "logs", 1, 19, schema, claimOptions("version-one"), fields)
	require.NoError(t, err)
	require.Empty(t, claims)
	// Changed immutable configuration gets its own work/vector identity.
	_, changed, err := client.ClaimEmbeddingSignals(context.Background(), "logs", 1, 19, schema, claimOptions("version-two"), fields)
	require.NoError(t, err)
	require.Len(t, changed, 1)
	require.Zero(t, changed[0].VectorID)
}

type workerClaimEmbedder struct {
	EmbeddingProvider
	calls   atomic.Int64
	started chan struct{}
	block   bool
}

func (e *workerClaimEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	e.calls.Add(1)
	if e.started != nil {
		close(e.started)
	}
	if e.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	result := make([][]float32, len(texts))
	for i := range result {
		result[i] = []float32{.1, .2}
	}
	return result, nil
}

func claimProductionWorker(client *OtelSQLDatabaseClient, store *PGVectorCustomStore, schema, version string, lease time.Duration) *EmbeddingWorker {
	engine := content.NewContentTemplateEngine()
	_ = engine.RegisterTemplate("logs", "{{.body}}")
	return NewEmbeddingWorker(client, util.NoopLogger{}, store,
		WithEmbeddingSchema(schema), WithWorkerID("durable-worker"), WithConfigID(37), WithSignalType("logs"), WithMaxContextLength(8192),
		WithWorkerFetchOptions(&FetchOptions{ConfigurationVersion: version, LeaseDuration: lease}),
		WithVectorStoreConfigs(map[string]*VectorStoreConfig{"claim-store": {ConfigID: 19, VectorStore: store, ContentEngine: engine, TemplateContent: map[string]string{"logs": "{{.body}}"}, TemplateMetadata: map[string][]string{"logs": {"body", "service_name", "timestamp_ns"}}}}),
		WithMetadata(map[string]interface{}{"embedding_mode": "batch"}))
}

func TestWorkerClaimsProductionGroupedRetryAfterReceiptFailure(t *testing.T) {
	db, client, store, schema := workerClaimFixture(t)
	for range 2 {
		workerClaimInsert(t, db, schema, "logs", time.Now().UnixNano())
	}
	_, err := db.Exec(fmt.Sprintf(`UPDATE %s.logs SET body=to_jsonb('original body '||id::text)`, schema))
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`CREATE FUNCTION %s.fail_receipt() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.metadata ? 'embeddings' THEN RAISE EXCEPTION 'forced receipt failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER fail_receipt BEFORE UPDATE ON %s.logs FOR EACH ROW EXECUTE FUNCTION %s.fail_receipt()`, schema, schema, schema))
	require.NoError(t, err)
	embedder := &workerClaimEmbedder{}
	store.EmbProvider = embedder
	first := claimProductionWorker(client, store, schema, "actual-grouped", time.Second)
	first.Metadata["embedding_batch_group_signals"] = true
	require.ErrorContains(t, first.RunBatch(context.Background()), "forced receipt failure")
	var vectors, receipts int
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.claim_vectors`, schema)).Scan(&vectors))
	require.Equal(t, 1, vectors)
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.logs WHERE metadata ? 'embeddings'`, schema)).Scan(&receipts))
	require.Zero(t, receipts)
	_, err = db.Exec(fmt.Sprintf(`DROP TRIGGER fail_receipt ON %s.logs`, schema))
	require.NoError(t, err)
	retry := claimProductionWorker(NewUnifiedSQLDatabaseClient(db), store, schema, "actual-grouped", time.Second)
	retry.Metadata["embedding_batch_group_signals"] = true
	require.NoError(t, retry.RunBatch(context.Background()))
	require.Equal(t, int64(1), embedder.calls.Load(), "retry must recover grouped vector references without another inference")
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.claim_vectors`, schema)).Scan(&vectors))
	require.Equal(t, 1, vectors)
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.logs WHERE metadata ? 'embeddings'`, schema)).Scan(&receipts))
	require.Equal(t, 2, receipts)
	var rendered string
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT content FROM %s.claim_vectors`, schema)).Scan(&rendered))
	require.Contains(t, rendered, "original body 1")
	require.Contains(t, rendered, "original body 2")
}

func TestWorkerClaimsRecoveryCompletesReceiptWithoutInference(t *testing.T) {
	db, client, store, schema := workerClaimFixture(t)
	for range 2 {
		workerClaimInsert(t, db, schema, "logs", time.Now().UnixNano())
	}
	_, claims, err := client.ClaimEmbeddingSignals(context.Background(), "logs", 2, 19, schema, claimOptions("grouped-version"), []SignalFieldConfig{{FieldName: "id", SignalKey: "id"}})
	require.NoError(t, err)
	require.Len(t, claims, 2)
	vector, err := store.InsertEmbeddingRaw(context.Background(), time.Now().UnixNano(), []float32{.1, .2}, "claim-service", "grouped original content", map[string]interface{}{claimMetadataKey: claims})
	require.NoError(t, err)
	_, err = db.Exec(fmt.Sprintf(`UPDATE %s.embedding_worker_claims SET leased_until=clock_timestamp()-interval '1 second'`, schema))
	require.NoError(t, err)
	embedder := &workerClaimEmbedder{}
	store.EmbProvider = embedder
	worker := claimProductionWorker(NewUnifiedSQLDatabaseClient(db), store, schema, "grouped-version", time.Second)
	require.NoError(t, worker.RunBatch(context.Background()))
	require.Zero(t, embedder.calls.Load())
	var count int
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.logs WHERE metadata->'embeddings' @> $1::jsonb`, schema), fmt.Sprintf(`[{"processor_id":19,"embedded_id":"%d"}]`, vector)).Scan(&count))
	require.Equal(t, 2, count)
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.claim_vectors`, schema)).Scan(&count))
	require.Equal(t, 1, count)
}

func TestWorkerClaimsLeaseLossAndStopCancelInference(t *testing.T) {
	for _, reason := range []string{"lease-loss", "worker-stop"} {
		t.Run(reason, func(t *testing.T) {
			db, client, store, schema := workerClaimFixture(t)
			workerClaimInsert(t, db, schema, "logs", time.Now().UnixNano())
			embedder := &workerClaimEmbedder{started: make(chan struct{}), block: true}
			store.EmbProvider = embedder
			worker := claimProductionWorker(client, store, schema, "cancel-version", 300*time.Millisecond)
			done := make(chan error, 1)
			go func() { done <- worker.RunBatch(context.Background()) }()
			select {
			case <-embedder.started:
			case <-time.After(5 * time.Second):
				t.Fatal("worker never reached inference")
			}
			// Source rows remain writable while slow inference is in flight.
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, err := db.ExecContext(ctx, fmt.Sprintf(`UPDATE %s.logs SET metadata=metadata`, schema))
			cancel()
			require.NoError(t, err)
			if reason == "worker-stop" {
				worker.Stop()
			} else {
				_, err = db.Exec(fmt.Sprintf(`UPDATE %s.embedding_worker_claims SET lease_token='new-owner'`, schema))
				require.NoError(t, err)
			}
			select {
			case err := <-done:
				require.Error(t, err)
				if reason == "lease-loss" {
					require.ErrorIs(t, err, ErrEmbeddingLeaseLost)
				} else {
					require.ErrorIs(t, err, context.Canceled)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("lost/stopped lease failed to cancel and drain inference")
			}
			var count int
			require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.claim_vectors`, schema)).Scan(&count))
			require.Zero(t, count)
		})
	}
}

func TestWorkerClaimsRetentionCleansOrphanMappings(t *testing.T) {
	db, client, _, schema := workerClaimFixture(t)
	workerClaimInsert(t, db, schema, "logs", time.Now().UnixNano())
	_, claims, err := client.ClaimEmbeddingSignals(context.Background(), "logs", 1, 19, schema, claimOptions("cleanup"), []SignalFieldConfig{{FieldName: "id", SignalKey: "id"}})
	require.NoError(t, err)
	require.NoError(t, client.ReleaseEmbeddingClaims(context.Background(), claims))
	_, err = db.Exec(fmt.Sprintf(`DELETE FROM %s.logs`, schema))
	require.NoError(t, err)
	_, _, err = client.ClaimEmbeddingSignals(context.Background(), "logs", 1, 19, schema, claimOptions("cleanup"), []SignalFieldConfig{{FieldName: "id", SignalKey: "id"}})
	require.NoError(t, err)
	var count int
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s.embedding_worker_claims`, schema)).Scan(&count))
	require.Zero(t, count)
}

func TestWorkerClaimsRecoveryReembedsVectorRemovedByRetention(t *testing.T) {
	db, client, store, schema := workerClaimFixture(t)
	workerClaimInsert(t, db, schema, "logs", time.Now().UnixNano())
	_, claims, err := client.ClaimEmbeddingSignals(context.Background(), "logs", 1, 19, schema, claimOptions("retained-source"), []SignalFieldConfig{{FieldName: "id", SignalKey: "id"}})
	require.NoError(t, err)
	first, err := store.InsertEmbeddingRaw(context.Background(), time.Now().UnixNano(), []float32{.1, .2}, "claim-service", "before vector retention", map[string]interface{}{claimMetadataKey: claims})
	require.NoError(t, err)
	require.NoError(t, client.ReleaseEmbeddingClaims(context.Background(), claims))
	_, err = db.Exec(fmt.Sprintf(`DELETE FROM %s.claim_vectors`, schema))
	require.NoError(t, err)
	embedder := &workerClaimEmbedder{}
	store.EmbProvider = embedder
	worker := claimProductionWorker(NewUnifiedSQLDatabaseClient(db), store, schema, "retained-source", time.Second)
	require.NoError(t, worker.RunBatch(context.Background()))
	require.Equal(t, int64(1), embedder.calls.Load())
	var second int64
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT id FROM %s.claim_vectors`, schema)).Scan(&second))
	require.NotEqual(t, first, second)
	var receipt string
	require.NoError(t, db.QueryRow(fmt.Sprintf(`SELECT metadata->'embeddings'->0->>'embedded_id' FROM %s.logs`, schema)).Scan(&receipt))
	require.Equal(t, fmt.Sprint(second), receipt)
}
