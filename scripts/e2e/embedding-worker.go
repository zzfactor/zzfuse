// Run only through scripts/test-stack-e2e.py against its disposable database.
// This driver owns one real SRM and uses the production manager/worker pipeline.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/event"
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/pkg/util"
	"go.uber.org/zap"
)

type workerSnapshot struct {
	ID                 string `json:"id"`
	SignalType         string `json:"signal_type"`
	Running            bool   `json:"running"`
	LastError          string `json:"last_error"`
	TotalJobsProcessed int64  `json:"total_jobs_processed"`
	SuccessfulJobs     int64  `json:"successful_jobs"`
	FailedJobs         int64  `json:"failed_jobs"`
	ProcessedSignals   int64  `json:"processed_signals"`
	TokensUsed         int64  `json:"tokens_used"`
	ActiveJobs         int64  `json:"active_jobs"`
}

func main() {
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func execute() error {
	if os.Getenv("ZZFUSE_E2E_ALLOW_DISPOSABLE_DB") != "true" || os.Getenv("TRAZR_DB_NAME") != "zzfuse_stack_e2e" || os.Getenv("TRAZR_DB_HOST") != "127.0.0.1" {
		return fmt.Errorf("worker E2E requires the explicitly authorized disposable localhost database")
	}
	expected, err := strconv.Atoi(os.Getenv("ZZFUSE_E2E_EXPECTED_LOGS"))
	if err != nil || expected < 1 || expected > 100 {
		return fmt.Errorf("invalid expected Collector log count")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	ctx, timeoutCancel := context.WithTimeout(ctx, 180*time.Second)
	defer timeoutCancel()
	dsn, err := repository.GetDSN()
	if err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	var written int
	if err = db.QueryRowContext(ctx, "SELECT count(*) FROM otel.logs").Scan(&written); err != nil || written != expected {
		return fmt.Errorf("real Collector logs unavailable: count=%d err=%v", written, err)
	}
	// These are test-owned configuration rows in a fresh database. The Collector
	// has already completed telemetry writes; this new SRM loads the real config.
	if _, err = db.ExecContext(ctx, `UPDATE admin.config_embedding_provider SET provider='sbert', endpoint=$1,
		model='sentence-transformers/all-MiniLM-L6-v2', dimension=384, max_context_length=256,
		timeout_ms=120000, headers='{}', metadata='{}' WHERE id=1`, os.Getenv("ZZFUSE_E2E_EMBEDDING_URL")); err != nil {
		return fmt.Errorf("configure real embedding endpoint: %w", err)
	}
	if _, err = db.ExecContext(ctx, `UPDATE admin.config_processor_template SET
		content='{"logs":"{{.body}}"}', template_metadata='{"generic":["service_name","timestamp_ns"],"logs":["body"]}' WHERE id=1;
		UPDATE admin.config_otel_processor SET enabled=(id=1), processing_mode='template',
		filter='{"signal_type":"logs"}', metadata='{"embedding_mode":"batch","embedding_batch_interval":1,"embedding_batch_group_signals":false}';`); err != nil {
		return fmt.Errorf("configure test worker/template: %w", err)
	}
	loggerConfig := zap.NewProductionConfig()
	loggerConfig.Level = zap.NewAtomicLevelAt(zap.WarnLevel)
	zapLogger, err := loggerConfig.Build()
	if err != nil {
		return err
	}
	defer zapLogger.Sync()
	srm, err := repository.NewServiceResourceManager(ctx, util.NewZapLoggerAdapter(zapLogger))
	if err != nil {
		return fmt.Errorf("create independent production SRM: %w", err)
	}
	defer srm.Shutdown()
	bus, err := srm.GetEventBus()
	if err != nil {
		return err
	}
	if _, ok := bus.(*event.NATSEventBus); !ok {
		return fmt.Errorf("worker manager fell back from real NATS")
	}
	if err = srm.ReloadSchemaWorkers(ctx, "otel"); err != nil {
		return fmt.Errorf("load on-demand production workers: %w", err)
	}
	srm.StartAllWorkers()
	defer srm.StopAllWorkers()
	var last []workerSnapshot
	for {
		last, err = snapshots(srm)
		if err != nil {
			return err
		}
		if len(last) != 1 || last[0].SignalType != "logs" {
			return fmt.Errorf("expected one real log worker, got %+v", last)
		}
		w := last[0]
		if w.FailedJobs != 0 || w.LastError != "" {
			return fmt.Errorf("production embedding worker failed: %+v", w)
		}
		if w.SuccessfulJobs > 0 && w.ProcessedSignals >= int64(expected) && w.ActiveJobs == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("worker completion timed out: %+v: %w", last, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	srm.StopAllWorkers()
	last, err = snapshots(srm)
	if err != nil {
		return err
	}
	w := last[0]
	if w.Running || w.ActiveJobs != 0 || w.ProcessedSignals != int64(expected) || w.TokensUsed <= 0 || w.FailedJobs != 0 {
		return fmt.Errorf("incorrect stopped production worker counters: %+v", w)
	}
	var receiptCount int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM otel.logs
		WHERE metadata->'embeddings' @> '[{"processor_id":1}]'`).Scan(&receiptCount); err != nil || receiptCount != expected {
		return fmt.Errorf("real embedding receipts missing: count=%d err=%v", receiptCount, err)
	}
	var count, minDimension, maxDimension int
	if err = db.QueryRowContext(ctx, `SELECT count(*), min(vector_dims(embedding)), max(vector_dims(embedding))
		FROM otel.pgvector_default`).Scan(&count, &minDimension, &maxDimension); err != nil || count != expected || minDimension != 384 || maxDimension != 384 {
		return fmt.Errorf("real pgvector rows/dimensions incorrect: rows=%d dims=%d..%d err=%v", count, minDimension, maxDimension, err)
	}
	var sourceMatches int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM otel.pgvector_default v
		JOIN otel.logs l ON v.content=(l.body #>> '{}')
		WHERE v.timestamp_ns=l.timestamp_ns AND v.service_name=l.service_name
		AND v.metadata->>'body'=(l.body #>> '{}') AND v.metadata->>'service_name'=l.service_name
		AND (v.metadata->>'timestamp_ns')::bigint=l.timestamp_ns`).Scan(&sourceMatches); err != nil || sourceMatches != expected {
		return fmt.Errorf("configured content/metadata lost source log context: matched=%d err=%v", sourceMatches, err)
	}
	store, err := srm.GetVectorStore("otel")
	if err != nil {
		return err
	}
	query := os.Getenv("ZZFUSE_E2E_QUERY")
	if query == "" {
		return fmt.Errorf("missing real semantic query")
	}
	docs, err := store.SimilaritySearch(ctx, query, 2)
	if err != nil || len(docs) != expected || docs[0].PageContent != query {
		return fmt.Errorf("real query embedding/nearest match failed: documents=%v err=%v", docs, err)
	}
	rows, err := db.QueryContext(ctx, `SELECT embedding::text FROM otel.pgvector_default`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return err
		}
		var vector []float64
		if err = json.Unmarshal([]byte(raw), &vector); err != nil || len(vector) != 384 {
			return fmt.Errorf("invalid stored inference vector: %v", err)
		}
		norm := 0.0
		for _, value := range vector {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return fmt.Errorf("non-finite stored inference vector")
			}
			norm += value * value
		}
		if norm < 0.1 {
			return fmt.Errorf("stored inference vector is empty")
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	raw, err := json.Marshal(last)
	if err != nil {
		return err
	}
	fmt.Println("production_worker_snapshots=" + string(raw))
	fmt.Printf("ZZFUSE_WORKER_E2E_PASS: %d actual Collector logs embedded into 384-dimensional pgvector and queried\n", expected)
	return nil
}

func snapshots(srm *repository.ServiceResourceManager) ([]workerSnapshot, error) {
	status := srm.GetWorkerStatus()["otel"]
	raw, err := json.Marshal(status["workers"])
	if err != nil {
		return nil, err
	}
	var result []workerSnapshot
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}
