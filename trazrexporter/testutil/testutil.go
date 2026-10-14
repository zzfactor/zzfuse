package testutil

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// TestMetricEntry represents a test metric data point.
type TestMetricEntry struct {
	Value  float64
	Env    string
	UserID string
	TS     time.Time
}

// TestSpanEntry represents a test span data point.
type TestSpanEntry struct {
	Name     string
	SpanAttr string
	Env      string
	UserID   string
	Start    time.Time
	End      time.Time
}

// CreateTestLogs creates OpenTelemetry logs for testing.
func CreateTestLogs(serviceName string, entries []struct {
	Body string
	Foo  string
	TS   time.Time
}) plog.Logs {
	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", serviceName)
	sl := rl.ScopeLogs().AppendEmpty()
	for _, entry := range entries {
		logRec := sl.LogRecords().AppendEmpty()
		logRec.Body().SetStr(entry.Body)
		logRec.SetTimestamp(pcommon.NewTimestampFromTime(entry.TS))
		logRec.Attributes().PutStr("foo", entry.Foo)
	}
	return logs
}

// MarshalLogsToBytes marshals logs to JSON bytes for testing.
func MarshalLogsToBytes(logs plog.Logs) []byte {
	b, _ := (&plog.JSONMarshaler{}).MarshalLogs(logs)
	return b
}

// AssertPromotedEqual asserts that two promoted values are equal in tests.
func AssertPromotedEqual(t *testing.T, got, want interface{}, field, entry string) {
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Errorf("%s mismatch for %q: got %v (%T), want %v (%T)", field, entry, got, got, want, want)
	}
}

// CreateTestMetrics creates OpenTelemetry metrics for testing.
func CreateTestMetrics(metricName, serviceName string, entries []TestMetricEntry) pmetric.Metrics {
	metrics := pmetric.NewMetrics()
	rm := metrics.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("service.name", serviceName)
	for _, entry := range entries {
		m := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		m.SetName(metricName)
		g := m.SetEmptyGauge()
		dp := g.DataPoints().AppendEmpty()
		dp.SetDoubleValue(entry.Value)
		dp.Attributes().PutStr("env", entry.Env)
		dp.Attributes().PutStr("user_id", entry.UserID)
		dp.SetTimestamp(pcommon.NewTimestampFromTime(entry.TS))
		dp.SetStartTimestamp(pcommon.NewTimestampFromTime(entry.TS))
	}
	return metrics
}

// CreateTestTraces creates OpenTelemetry spans for testing.
func CreateTestTraces(serviceName string, entries []TestSpanEntry) ptrace.Traces {
	spans := ptrace.NewTraces()
	rs := spans.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", serviceName)
	ss := rs.ScopeSpans().AppendEmpty()
	for _, entry := range entries {
		span := ss.Spans().AppendEmpty()
		span.SetName(entry.Name)
		span.Attributes().PutStr("span_attr", entry.SpanAttr)
		span.Attributes().PutStr("env", entry.Env)
		span.Attributes().PutStr("user_id", entry.UserID)
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(entry.Start))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(entry.End))
	}
	return spans
}

// EnsurePGVectorExtension ensures the pgvector extension is enabled on the given DB.
func EnsurePGVectorExtension(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec("CREATE EXTENSION IF NOT EXISTS vector;"); err != nil {
		t.Fatalf("failed to create pgvector extension: %v", err)
	}
}

// RecreateVectorStoreTable drops and recreates the given table using the provided schema sync function and config.
// This ensures the table schema is always correct and avoids dimension mismatch issues in tests.
func RecreateVectorStoreTable(t *testing.T, db *sql.DB, tableName string, embeddingDimension int, syncSchema func(*sql.DB, string, int) error) {
	t.Helper()
	EnsurePGVectorExtension(t, db)
	if _, err := db.Exec("DROP TABLE IF EXISTS " + tableName + ";"); err != nil {
		t.Fatalf("failed to drop table %s: %v", tableName, err)
	}
	if err := syncSchema(db, tableName, embeddingDimension); err != nil {
		t.Fatalf("failed to recreate table %s: %v", tableName, err)
	}
}

// EnsureTableExists ensures the table exists using a provided schema sync function.
// Pass a function like func(db *sql.DB, tableName string, cfg interface{}) error.
func EnsureTableExists(t *testing.T, db *sql.DB, tableName string, embeddingDimension int, syncSchema func(*sql.DB, string, int) error) {
	t.Helper()
	EnsurePGVectorExtension(t, db)
	if err := syncSchema(db, tableName, embeddingDimension); err != nil {
		t.Fatalf("failed to ensure table %s exists: %v", tableName, err)
	}
}
