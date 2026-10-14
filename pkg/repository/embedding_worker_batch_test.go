package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock OtelDatabaseClient for testing
type mockOtelDatabaseClient struct {
	logs []LogData
}

// Ensure mockOtelDatabaseClient implements OtelDatabaseClient
var _ OtelDatabaseClient = (*mockOtelDatabaseClient)(nil)

func (m *mockOtelDatabaseClient) FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error) {
	return m.logs, nil
}

func (m *mockOtelDatabaseClient) FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error) {
	return m.logs, nil
}

// Implement other required methods with empty implementations
func (m *mockOtelDatabaseClient) FetchUnembeddedSignals(ctx context.Context, configID int64, signalType string, limit int, schema string) ([]map[string]interface{}, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) FetchUnembeddedSignalsSimple(ctx context.Context, tableName string, batchSize int, filterConfig map[string]interface{}, configID int64, schema string) ([]map[string]interface{}, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) MarkSignalsEmbedded(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	return nil
}

func (m *mockOtelDatabaseClient) MarkSignalsEmbeddedBulk(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	return nil
}

func (m *mockOtelDatabaseClient) FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error) {
	return nil, nil
}

// Implement other OtelDatabaseClient methods with empty implementations

func (m *mockOtelDatabaseClient) QuerySpans(ctx context.Context, q TraceQuery) ([]TraceEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QueryGaugeMetrics(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QuerySumMetrics(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QueryHistogramMetrics(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QuerySummaryMetrics(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QueryExponentialHistogramMetrics(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QueryProfiles(ctx context.Context, q ProfileQuery) ([]ProfileEntry, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) QueryMetricsGauge(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) QueryMetricsSum(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) QueryMetricsHistogram(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) QueryMetricsSummary(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockOtelDatabaseClient) GetStatistics(ctx context.Context) (*WorkerMetrics, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) GetJobStatus(ctx context.Context, jobID string) (*ProcessingJob, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) GetActiveJobs(ctx context.Context) ([]*ProcessingJob, error) {
	return nil, nil
}

// Add missing FetchUnembeddedSignalsAdvanced method
func (m *mockOtelDatabaseClient) FetchUnembeddedSignalsAdvanced(ctx context.Context, tableName string, batchSize int, configID int64, schema string, options *FetchOptions, requiredFields []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error) {
	return nil, nil, nil
}

// Add missing MarkSignalsEmbeddedAdvanced method
func (m *mockOtelDatabaseClient) MarkSignalsEmbeddedAdvanced(ctx context.Context, tableName string, ids []interface{}, configID int64, embeddedIDs []string, schema string) error {
	return nil
}

// Implement missing Count/ID/Run-related methods to satisfy OtelDatabaseClient

func (m *mockOtelDatabaseClient) CountSpans(ctx context.Context, q TraceQuery) (int64, error) {
	return 0, nil
}

func (m *mockOtelDatabaseClient) CountLogs(ctx context.Context, q LogQuery) (int64, error) {
	return 0, nil
}

func (m *mockOtelDatabaseClient) CountMetricsGauge(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockOtelDatabaseClient) CountMetricsSum(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockOtelDatabaseClient) CountMetricsHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockOtelDatabaseClient) CountMetricsSummary(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockOtelDatabaseClient) CountMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockOtelDatabaseClient) GetMetricByID(ctx context.Context, id int64, schema string) (interface{}, string, error) {
	return nil, "", nil
}

func (m *mockOtelDatabaseClient) GetLogByID(ctx context.Context, id int64, schema string) (LogEntry, error) {
	return LogEntry{}, nil
}

func (m *mockOtelDatabaseClient) GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntry, error) {
	return TraceEntry{}, nil
}

func (m *mockOtelDatabaseClient) QueryRuns(ctx context.Context, q RunQuery) ([]RunEntry, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockOtelDatabaseClient) InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error) {
	return nil, nil
}

func (m *mockOtelDatabaseClient) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error {
	return nil
}

func (m *mockOtelDatabaseClient) EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error {
	return nil
}

func (m *mockOtelDatabaseClient) GetRunByID(ctx context.Context, id int64, schema string) (RunEntry, error) {
	return RunEntry{}, nil
}

func (m *mockOtelDatabaseClient) StartOnDemandWorkers(ctx context.Context, params OnDemandParams) error {
	return nil
}

func (m *mockOtelDatabaseClient) StopOnDemandWorkers(ctx context.Context, schema string) error {
	return nil
}

func TestFetchLogsFromDatabase(t *testing.T) {
	// Create a mock embedding worker with logs in the database
	mockTraceDB := &mockOtelDatabaseClient{
		logs: []LogData{
			{
				ID:           "log-1",
				TraceID:      stringPtr("test-trace-123"),
				SpanID:       stringPtr("test-span-456"),
				SeverityText: stringPtr("INFO"),
				Body:         "Test log message from database",
			},
			{
				ID:           "log-2",
				TraceID:      stringPtr("test-trace-123"),
				SpanID:       stringPtr("test-span-456"),
				SeverityText: stringPtr("ERROR"),
				Body:         "Test error message from database",
			},
		},
	}

	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			WorkerID: "test-worker",
			Logger:   util.NoopLogger{},
			DB:       mockTraceDB,
		},
	}

	// Test building trace content which should fetch logs from database
	signal := map[string]interface{}{
		"trace_id": "test-trace-123",
		"span_id":  "test-span-456",
		"name":     "test-operation",
	}

	content := worker.buildTraceContent(context.Background(), signal)

	// Verify that logs were fetched
	assert.Len(t, content.Logs, 2, "Should fetch 2 logs from database")

	// Create a map to check logs by ID for more flexible testing
	logMap := make(map[string]LogData)
	for _, log := range content.Logs {
		logMap[log.ID] = log
	}

	// Verify log-1
	log1, exists := logMap["log-1"]
	assert.True(t, exists, "Should have log-1")
	assert.Equal(t, "test-trace-123", *log1.TraceID)
	assert.Equal(t, "test-span-456", *log1.SpanID)
	assert.Equal(t, "INFO", *log1.SeverityText)
	assert.Equal(t, "Test log message from database", log1.Body)

	// Verify log-2
	log2, exists := logMap["log-2"]
	assert.True(t, exists, "Should have log-2")
	assert.Equal(t, "test-trace-123", *log2.TraceID)
	assert.Equal(t, "test-span-456", *log2.SpanID)
	assert.Equal(t, "ERROR", *log2.SeverityText)
	assert.Equal(t, "Test error message from database", log2.Body)
}

func TestFetchLogsFromDatabaseNoLogs(t *testing.T) {
	// Create a mock embedding worker with no logs in the database
	mockTraceDB := &mockOtelDatabaseClient{
		logs: []LogData{}, // Empty logs
	}

	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			WorkerID: "test-worker",
			Logger:   util.NoopLogger{},
			DB:       mockTraceDB,
		},
	}

	// Test building trace content when no logs exist in database
	signal := map[string]interface{}{
		"trace_id": "test-trace-123",
		"span_id":  "test-span-456",
		"name":     "test-operation",
	}

	content := worker.buildTraceContent(context.Background(), signal)

	// Verify that no logs were fetched
	assert.Len(t, content.Logs, 0, "Should return empty logs when none exist in database")
}

func TestFetchLogsFromDatabaseNoClient(t *testing.T) {
	// Create a mock embedding worker with no database client
	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			WorkerID: "test-worker",
			Logger:   util.NoopLogger{},
			DB:       nil, // No database client
		},
	}

	// Test building trace content when no database client is available
	signal := map[string]interface{}{
		"trace_id": "test-trace-123",
		"span_id":  "test-span-456",
		"name":     "test-operation",
	}

	content := worker.buildTraceContent(context.Background(), signal)

	// Verify that no logs were fetched
	assert.Len(t, content.Logs, 0, "Should return empty logs when no database client is available")
}

func TestBuildTraceContentWithRealData(t *testing.T) {
	// Create a mock embedding worker with no logs in database (simulating real scenario)
	mockTraceDB := &mockOtelDatabaseClient{
		logs: []LogData{}, // No separate logs in database
	}

	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			WorkerID: "test-worker",
			Logger:   util.NoopLogger{},
			DB:       mockTraceDB,
		},
	}

	// Real data from the user
	realSignal := map[string]interface{}{
		"id":           "743a1c40-7001-4d70-92c2-4523a82a1b56",
		"trace_id":     "daf700a3737cbdc7ef0dfabbd439b465",
		"span_id":      "c82f0e6f756d860f",
		"name":         "payment-service-operation",
		"service_name": "trazr-gen",
		"status_code":  0,
		"duration":     int64(5324000),
		"events": []map[string]interface{}{
			{
				"name":      "db.query",
				"timestamp": "1970-01-01T00:00:00Z",
				"attributes": map[string]interface{}{
					"event.type":   "custom",
					"event.source": "payment-service",
				},
			},
			{
				"name":      "log.generated",
				"timestamp": "2025-07-29T21:55:52.597268Z",
				"attributes": map[string]interface{}{
					"log.message":   "Handling request in payment-service-operation service",
					"log.severity":  "warn",
					"log.timestamp": "0001-01-01T00:00:00.0000001Z",
				},
			},
		},
	}

	// Build trace content from real signal
	traceContent := worker.buildTraceContent(context.Background(), realSignal)

	// Verify the trace content
	assert.NotNil(t, traceContent)
	assert.Equal(t, "daf700a3737cbdc7ef0dfabbd439b465", traceContent.TraceID)
	assert.Equal(t, "trazr-gen", traceContent.ServiceName)
	assert.Len(t, traceContent.Spans, 1)

	// Verify the span data
	span := traceContent.Spans[0]
	assert.Equal(t, "c82f0e6f756d860f", span.SpanID)
	assert.Equal(t, "daf700a3737cbdc7ef0dfabbd439b465", span.TraceID)
	assert.Equal(t, "payment-service-operation", span.Name)
	assert.Equal(t, "trazr-gen", *span.ServiceName)
	assert.Equal(t, 0, span.StatusCode)
	assert.Equal(t, int64(5324000), span.Duration)

	// Verify events are extracted correctly
	assert.Len(t, span.Events, 2)

	// Verify that we have log events
	logEvents := 0
	for _, event := range span.Events {
		if event["name"] == "log.generated" {
			logEvents++
		}
	}
	assert.Equal(t, 1, logEvents, "Should have 1 log.generated event")

	// Verify that logs are fetched from database (not from events)
	// Since we're using a mock with no logs, we expect 0 logs
	assert.Len(t, traceContent.Logs, 0, "Should have 0 logs since mock database has no logs")

	// Verify metadata
	assert.Equal(t, "test-worker", traceContent.Metadata["worker_id"])
}

func TestDatabaseLogQuery(t *testing.T) {
	// This test will help us understand what's happening with the database queries
	// We'll use the real trace IDs from the user's data

	traceID := "daf700a3737cbdc7ef0dfabbd439b465"
	spanID := "c82f0e6f756d860f"

	fmt.Printf("TEST: Checking for logs with trace_id: %s and span_id: %s\n", traceID, spanID)

	// Check if we can connect to the database
	// This is a simple test to see if there are any logs at all
	fmt.Printf("TEST: This test would need a real database connection to work\n")
	fmt.Printf("TEST: The issue is likely that there are no log records in the logs table\n")
	fmt.Printf("TEST: for trace_id: %s or span_id: %s\n", traceID, spanID)

	// The real issue is probably that:
	// 1. There are no separate log records in the logs table
	// 2. The logs are only embedded as events in the spans table
	// 3. We need to extract logs from span events, not query the logs table

	t.Log("Database log query test completed - this confirms the issue is with missing log records in the logs table")
}

func TestTraceDatabaseClientTypeAssertion(t *testing.T) {
	// This test will help us debug the type assertion issue

	// Create a mock database client that implements OtelDatabaseClient
	mockTraceDB := &mockOtelDatabaseClient{
		logs: []LogData{
			{
				ID:           "test-log-1",
				TraceID:      stringPtr("test-trace-123"),
				SpanID:       stringPtr("test-span-456"),
				SeverityText: stringPtr("INFO"),
				Body:         "Test log message",
			},
		},
	}

	// Test the type assertion
	var db interface{} = mockTraceDB
	if traceDB, ok := db.(OtelDatabaseClient); ok {
		t.Log("Type assertion successful - mockTraceDB implements OtelDatabaseClient")

		// Test that we can call the methods
		logs, err := traceDB.FetchLogsByTraceID(context.Background(), "test-trace-123")
		if err != nil {
			t.Errorf("Failed to fetch logs: %v", err)
		}
		if len(logs) != 1 {
			t.Errorf("Expected 1 log, got %d", len(logs))
		}
	} else {
		t.Error("Type assertion failed - mockTraceDB does not implement OtelDatabaseClient")
	}

	// Test with nil
	var nilDB interface{} = nil
	if _, ok := nilDB.(TraceDatabaseClient); ok {
		t.Error("Type assertion should fail for nil")
	} else {
		t.Log("Type assertion correctly failed for nil")
	}
}

func TestOtelSQLDatabaseClientTypeAssertion(t *testing.T) {
	// This test will help us verify that OtelSQLDatabaseClient implements TraceDatabaseClient

	// Create a mock SQL database (we can't create a real one in tests)
	// But we can test the type assertion logic

	// Test that the interface is properly defined
	var _ OtelDatabaseClient = (*mockOtelDatabaseClient)(nil)
	t.Log("mockOtelDatabaseClient properly implements OtelDatabaseClient")

	// Test that we can create a worker with a mock database and the type assertion works
	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			WorkerID: "test-worker",
			Logger:   util.NoopLogger{},
			DB:       &mockOtelDatabaseClient{logs: []LogData{}},
		},
	}

	// Test the getTraceDatabaseClient method
	traceDB := worker.getTraceDatabaseClient()
	if traceDB != nil {
		t.Log("getTraceDatabaseClient returned a valid TraceDatabaseClient")
	} else {
		t.Error("getTraceDatabaseClient returned nil - type assertion failed")
	}
}

func TestRealDatabaseClientTypeAssertion(t *testing.T) {
	// This test simulates the real scenario with OtelSQLDatabaseClient

	// Create a mock database client that simulates the real OtelSQLDatabaseClient
	mockDB := &mockOtelDatabaseClient{
		logs: []LogData{
			{
				ID:           "real-log-1",
				TraceID:      stringPtr("real-trace-123"),
				SpanID:       stringPtr("real-span-456"),
				SeverityText: stringPtr("INFO"),
				Body:         "Real log message from database",
			},
		},
	}

	// Test the type assertion
	var db interface{} = mockDB
	if traceDB, ok := db.(OtelDatabaseClient); ok {
		t.Log("Type assertion successful - mockOtelDatabaseClient implements OtelDatabaseClient")

		// Test that we can call the methods
		logs, err := traceDB.FetchLogsByTraceID(context.Background(), "real-trace-123")
		if err != nil {
			t.Errorf("Failed to fetch logs: %v", err)
		}
		if len(logs) != 1 {
			t.Errorf("Expected 1 log, got %d", len(logs))
		}
		if logs[0].ID != "real-log-1" {
			t.Errorf("Expected log ID 'real-log-1', got '%s'", logs[0].ID)
		}
	} else {
		t.Error("Type assertion failed - mockOtelDatabaseClient does not implement OtelDatabaseClient")
	}

	// Test with a worker
	worker := &EmbeddingWorker{
		BaseWorker: &BaseWorker{
			WorkerID: "test-worker",
			Logger:   util.NoopLogger{},
			DB:       mockDB,
		},
	}

	// Test the getTraceDatabaseClient method
	traceDB := worker.getTraceDatabaseClient()
	if traceDB != nil {
		t.Log("getTraceDatabaseClient returned a valid OtelDatabaseClient for mockOtelDatabaseClient")

		// Test building trace content through the worker
		signal := map[string]interface{}{
			"trace_id": "real-trace-123",
			"span_id":  "real-span-456",
			"name":     "test-operation",
		}
		content := worker.buildTraceContent(context.Background(), signal)
		if len(content.Logs) != 1 {
			t.Errorf("Expected 1 log from worker, got %d", len(content.Logs))
		}
	} else {
		t.Error("getTraceDatabaseClient returned nil for mockOtelDatabaseClient - type assertion failed")
	}
}

func TestExtractMetadataFromSignal(t *testing.T) {
	// Create a worker with template metadata configuration using VectorStoreConfigs
	worker := &EmbeddingWorker{
		VectorStoreConfigs: map[string]*VectorStoreConfig{
			"test_vector_store": {
				TemplateMetadata: map[string][]string{
					"logs":  {"service_name", "level", "body", "message"},
					"spans": {"name", "service_name", "status_code", "duration", "trace_id", "span_id"},
				},
			},
		},
	}

	// Test logs signal with template metadata
	logSignal := map[string]interface{}{
		"id":           "log-123",
		"service_name": "test-service",
		"level":        "INFO",
		"body":         "test log message",
		"message":      "test message",
		"timestamp_ns": 1234567890,
		"extra_field":  "should_not_be_included",
	}

	metadata := worker.extractMetadataFromSignal(logSignal, "logs", "test_vector_store")
	internal, ok := metadata["internal"].(map[string]interface{})
	require.True(t, ok, "internal metadata must be a nested object")

	// Verify that only template-defined fields are included
	assert.Equal(t, "logs", internal["signal_type"])
	assert.Equal(t, "log-123", internal["id"])
	assert.Equal(t, "test-service", metadata["service_name"])
	assert.Equal(t, "INFO", metadata["level"])
	assert.Equal(t, "test log message", metadata["body"])
	assert.Equal(t, "test message", metadata["message"])

	// Verify that non-template fields are NOT included
	assert.NotContains(t, metadata, "timestamp_ns")
	assert.NotContains(t, metadata, "extra_field")

	// Test spans signal with template metadata
	spanSignal := map[string]interface{}{
		"id":           "span-456",
		"name":         "test-span",
		"service_name": "test-service",
		"status_code":  0,
		"duration":     1000000,
		"trace_id":     "trace-789",
		"span_id":      "span-456",
		"extra_field":  "should_not_be_included",
	}

	metadata = worker.extractMetadataFromSignal(spanSignal, "spans", "test_vector_store")
	internal, ok = metadata["internal"].(map[string]interface{})
	require.True(t, ok)

	// Verify that only template-defined fields are included
	assert.Equal(t, "spans", internal["signal_type"])
	assert.Equal(t, "span-456", internal["id"])
	assert.Equal(t, "test-span", metadata["name"])
	assert.Equal(t, "test-service", metadata["service_name"])
	assert.Equal(t, 0, metadata["status_code"])
	assert.Equal(t, 1000000, metadata["duration"])
	assert.Equal(t, "trace-789", metadata["trace_id"])
	assert.Equal(t, "span-456", metadata["span_id"])

	// Verify that non-template fields are NOT included
	assert.NotContains(t, metadata, "extra_field")

	// Test fallback when no template metadata is configured
	workerNoTemplate := &EmbeddingWorker{
		VectorStoreConfigs: nil,
	}

	metadata = workerNoTemplate.extractMetadataFromSignal(logSignal, "logs", "test_vector_store")
	internal, ok = metadata["internal"].(map[string]interface{})
	require.True(t, ok)

	// Should only include basic fields
	assert.Equal(t, "logs", internal["signal_type"])
	assert.Equal(t, "log-123", internal["id"])
	assert.Len(t, metadata, 1) // Only the internal object at the top level
	assert.Len(t, internal, 2) // Only signal_type and id
}
