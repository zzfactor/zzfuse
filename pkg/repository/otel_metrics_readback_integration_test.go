//go:build integration

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// Use an explicitly supplied disposable TimescaleDB database and a unique
// schema. Production DDL and production insert/query service paths are used.
func TestMetricStorageAndQueryRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_METRIC_READBACK_DSN")
	if os.Getenv("TRAZR_IT") != "1" || dsn == "" {
		t.Skip("requires TRAZR_IT=1 and TEST_METRIC_READBACK_DSN for a disposable TimescaleDB database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("metric_readback_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	initializer := &database.SchemaConfigRepository{DB: db}
	if err := initializer.EnsureOtelDataTables(ctx, db, schema); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SKIP_TEMPLATE_PROCESSING", "true")
	t.Setenv("SKIP_HASH_PROCESSING", "true")
	client := NewUnifiedSQLDatabaseClient(db)
	writer := NewMetricWriterServiceWithUnifiedClient(client)
	service := NewMetricQueryServiceWithUnifiedClient(client)
	at := time.Now().UTC().Truncate(time.Millisecond)
	for _, tc := range metricReadbackCases(at) {
		t.Run(tc.kind, func(t *testing.T) {
			ids, err := insertMetricReadback(ctx, writer, schema, tc.entry)
			if err != nil {
				t.Fatal(err)
			}
			if len(ids) != 1 || ids[0] != "1" {
				t.Fatalf("fixture must demonstrate colliding IDs across metric types: %v", ids)
			}
			tc.expected["id"] = 1
			q := MetricQuery{CommonBaseFilters: CommonBaseFilters{Schema: schema, CommonPaginationFilters: CommonPaginationFilters{Limit: 10, Direction: "asc"}}, MetricName: "metric.measurement"}
			q.ServiceName = "readback-service"
			result, err := queryMetricReadback(ctx, service, tc.kind, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Entries) != 1 {
				t.Fatalf("wanted one metric, got %d", len(result.Entries))
			}
			assertMetricResponse(t, result.Entries[0], tc.expected)
			byID, err := service.GetMetricByIDTyped(ctx, tc.kind, 1, schema)
			if err != nil {
				t.Fatal(err)
			}
			assertMetricResponse(t, byID, tc.expected)
			if _, err := service.GetMetricByIDTyped(ctx, tc.kind, 99999, schema); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("missing typed metric: %v", err)
			}
			second := reflect.New(reflect.TypeOf(tc.entry)).Elem()
			second.Set(reflect.ValueOf(tc.entry))
			second.FieldByName("TimestampNS").SetInt(at.Add(time.Second).UnixNano())
			second.FieldByName("Timestamp").Set(reflect.ValueOf(at.Add(time.Second)))
			if ids, err := insertMetricReadback(ctx, writer, schema, second.Interface()); err != nil || len(ids) != 1 || ids[0] != "2" {
				t.Fatalf("second insert ids=%v err=%v", ids, err)
			}
			q.Limit = 1
			firstPage, err := queryMetricReadback(ctx, service, tc.kind, q)
			if err != nil || len(firstPage.Entries) != 1 || firstPage.Entries[0].ID != 1 || !firstPage.HasMore || firstPage.NextCursor == "" {
				t.Fatalf("first metric page=%+v err=%v", firstPage, err)
			}
			q.Cursor = firstPage.NextCursor
			nextPage, err := queryMetricReadback(ctx, service, tc.kind, q)
			if err != nil || len(nextPage.Entries) != 1 || nextPage.Entries[0].ID != 2 || nextPage.HasMore || !nextPage.HasPrev {
				t.Fatalf("next metric page=%+v err=%v", nextPage, err)
			}
		})
	}
	if _, err := service.GetMetricByIDTyped(ctx, "gauge;DROP TABLE", 1, schema); err == nil {
		t.Fatal("unsupported metric type accepted")
	}
}

func insertMetricReadback(ctx context.Context, writer MetricWriterService, schema string, entry interface{}) ([]string, error) {
	switch e := entry.(type) {
	case GaugeMetricEntry:
		return writer.InsertGaugeMetrics(ctx, []GaugeMetricEntry{e}, schema)
	case SumMetricEntry:
		return writer.InsertSumMetrics(ctx, []SumMetricEntry{e}, schema)
	case HistogramMetricEntry:
		return writer.InsertHistogramMetrics(ctx, []HistogramMetricEntry{e}, schema)
	case SummaryMetricEntry:
		return writer.InsertSummaryMetrics(ctx, []SummaryMetricEntry{e}, schema)
	case ExponentialHistogramMetricEntry:
		return writer.InsertExponentialHistogramMetrics(ctx, []ExponentialHistogramMetricEntry{e}, schema)
	default:
		return nil, fmt.Errorf("unsupported fixture: %T", entry)
	}
}

func queryMetricReadback(ctx context.Context, service MetricQueryService, kind string, q MetricQuery) (MetricQueryResult, error) {
	switch kind {
	case "gauge":
		return service.QueryMetricsGauge(ctx, q)
	case "sum":
		return service.QueryMetricsSum(ctx, q)
	case "histogram":
		return service.QueryMetricsHistogram(ctx, q)
	case "summary":
		return service.QueryMetricsSummary(ctx, q)
	case "exponential_histogram":
		return service.QueryMetricsExponentialHistogram(ctx, q)
	default:
		return MetricQueryResult{}, fmt.Errorf("unsupported fixture type %q", kind)
	}
}
