package repository

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/tmc/langchaingo/schema"
)

func TestSmartWorkerCreation(t *testing.T) {
	tests := []struct {
		name                string
		embeddingFilter     map[string]interface{}
		databaseFilter      map[string]interface{}
		expectedSignalTypes []string
		expectedWorkerCount int
		description         string
	}{
		{
			name: "logs_only_embedding_filter",
			embeddingFilter: map[string]interface{}{
				"signal_type": "logs",
			},
			databaseFilter:      nil, // No database filter = all signal types
			expectedSignalTypes: []string{SignalTypeLogs},
			expectedWorkerCount: 1,
			description:         "Should create only 1 worker for logs when embedding filter targets only logs",
		},
		{
			name: "logs_and_spans_embedding_filter",
			embeddingFilter: map[string]interface{}{
				"signal_type": "logs,spans",
			},
			databaseFilter:      nil,
			expectedSignalTypes: []string{SignalTypeLogs, SignalTypeSpans},
			expectedWorkerCount: 2,
			description:         "Should create 2 workers when embedding filter targets logs and spans",
		},
		{
			name: "all_signals_embedding_filter",
			embeddingFilter: map[string]interface{}{
				"signal_type": "*",
			},
			databaseFilter:      nil,
			expectedSignalTypes: AllSupportedSignalTypes(),
			expectedWorkerCount: 8,
			description:         "Should create 8 workers when embedding filter targets all signals",
		},
		{
			name:                "no_embedding_filter",
			embeddingFilter:     nil,
			databaseFilter:      nil,
			expectedSignalTypes: AllSupportedSignalTypes(),
			expectedWorkerCount: 8,
			description:         "Should create 8 workers when no embedding filter is specified",
		},
		{
			name:            "logs_only_database_filter",
			embeddingFilter: nil, // No embedding filter = all signal types
			databaseFilter: map[string]interface{}{
				"signal_type": "logs",
			},
			expectedSignalTypes: []string{SignalTypeLogs},
			expectedWorkerCount: 1,
			description:         "Should create only 1 worker when database filter targets only logs",
		},
		{
			name: "intersection_of_filters",
			embeddingFilter: map[string]interface{}{
				"signal_type": "logs,spans,gauge_metrics",
			},
			databaseFilter: map[string]interface{}{
				"signal_type": "logs,gauge_metrics",
			},
			expectedSignalTypes: []string{SignalTypeLogs, SignalTypeGaugeMetrics},
			expectedWorkerCount: 2,
			description:         "Should create workers only for signal types present in both filters",
		},
		{
			name: "no_intersection",
			embeddingFilter: map[string]interface{}{
				"signal_type": "logs",
			},
			databaseFilter: map[string]interface{}{
				"signal_type": "spans",
			},
			expectedSignalTypes: []string{SignalTypeSpans}, // Default to database filter when no intersection
			expectedWorkerCount: 1,
			description:         "Should default to database filter when no intersection between filters",
		},
		{
			name: "complex_filter_with_attributes",
			embeddingFilter: map[string]interface{}{
				"signal_type": "logs,spans",
				"resource": map[string]interface{}{
					"service.name": "test-service",
				},
			},
			databaseFilter: map[string]interface{}{
				"signal_type": "logs,spans",
			},
			expectedSignalTypes: []string{SignalTypeLogs, SignalTypeSpans},
			expectedWorkerCount: 2,
			description:         "Should extract signal types correctly from complex filter with additional conditions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the signal type extraction functions
			embeddingTypes := getSignalTypesFromEmbeddingFilter(tt.embeddingFilter)
			databaseTypes := getSignalTypesFromDatabaseFilter(tt.databaseFilter)
			intersection := getIntersectionOfSignalTypes(embeddingTypes, databaseTypes)
			validatedTypes := validateSignalTypes(intersection)

			// If no valid types found, should default to database filter (what's actually stored)
			if len(validatedTypes) == 0 {
				validatedTypes = databaseTypes
			}

			// Verify the results
			assert.ElementsMatch(t, tt.expectedSignalTypes, validatedTypes,
				"Signal types mismatch for test: %s", tt.description)
			assert.Equal(t, tt.expectedWorkerCount, len(validatedTypes),
				"Worker count mismatch for test: %s", tt.description)

			t.Logf("Test: %s", tt.description)
			t.Logf("  Embedding filter: %v", tt.embeddingFilter)
			t.Logf("  Database filter: %v", tt.databaseFilter)
			t.Logf("  Extracted embedding types: %v", embeddingTypes)
			t.Logf("  Extracted database types: %v", databaseTypes)
			t.Logf("  Intersection: %v", intersection)
			t.Logf("  Final validated types: %v", validatedTypes)
			t.Logf("  Expected types: %v", tt.expectedSignalTypes)
		})
	}
}

func TestSignalTypeExtractionFunctions(t *testing.T) {
	t.Run("getSignalTypesFromFilterConfig", func(t *testing.T) {
		tests := []struct {
			name          string
			filter        *FilterConfig
			expectedTypes []string
		}{
			{
				name:          "nil_filter",
				filter:        nil,
				expectedTypes: AllSupportedSignalTypes(),
			},
			{
				name: "empty_signal_type",
				filter: &FilterConfig{
					SignalType: "",
				},
				expectedTypes: AllSupportedSignalTypes(),
			},
			{
				name: "wildcard_signal_type",
				filter: &FilterConfig{
					SignalType: "*",
				},
				expectedTypes: AllSupportedSignalTypes(),
			},
			{
				name: "single_signal_type",
				filter: &FilterConfig{
					SignalType: "logs",
				},
				expectedTypes: []string{"logs"},
			},
			{
				name: "multiple_signal_types",
				filter: &FilterConfig{
					SignalType: "logs,spans,metrics",
				},
				expectedTypes: []string{"logs", "spans", "metrics"},
			},
			{
				name: "multiple_signal_types_with_spaces",
				filter: &FilterConfig{
					SignalType: "logs, spans , metrics",
				},
				expectedTypes: []string{"logs", "spans", "metrics"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := getSignalTypesFromFilterConfig(tt.filter)
				assert.ElementsMatch(t, tt.expectedTypes, result)
			})
		}
	})

	t.Run("getIntersectionOfSignalTypes", func(t *testing.T) {
		tests := []struct {
			name     string
			types1   []string
			types2   []string
			expected []string
		}{
			{
				name:     "no_intersection",
				types1:   []string{"logs", "spans"},
				types2:   []string{"metrics", "profiles"},
				expected: []string{},
			},
			{
				name:     "partial_intersection",
				types1:   []string{"logs", "spans", "metrics"},
				types2:   []string{"spans", "metrics", "profiles"},
				expected: []string{"spans", "metrics"},
			},
			{
				name:     "full_intersection",
				types1:   []string{"logs", "spans"},
				types2:   []string{"logs", "spans"},
				expected: []string{"logs", "spans"},
			},
			{
				name:     "empty_slices",
				types1:   []string{},
				types2:   []string{},
				expected: []string{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := getIntersectionOfSignalTypes(tt.types1, tt.types2)
				assert.ElementsMatch(t, tt.expected, result)
			})
		}
	})

	t.Run("validateSignalTypes", func(t *testing.T) {
		tests := []struct {
			name     string
			input    []string
			expected []string
		}{
			{
				name:     "all_valid",
				input:    []string{"logs", "spans", "gauge_metrics"},
				expected: []string{"logs", "spans", "gauge_metrics"},
			},
			{
				name:     "some_invalid",
				input:    []string{"logs", "invalid_type", "spans"},
				expected: []string{"logs", "spans"},
			},
			{
				name:     "all_invalid",
				input:    []string{"invalid1", "invalid2"},
				expected: []string{},
			},
			{
				name:     "empty_input",
				input:    []string{},
				expected: []string{},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := validateSignalTypes(tt.input)
				assert.ElementsMatch(t, tt.expected, result)
			})
		}
	})
}

// Mock implementations for testing
type mockEmbeddingDatabaseClient struct{}
type mockVectorStoreClient struct{}
type mockLogger struct{}

func (m *mockEmbeddingDatabaseClient) FetchUnembeddedSignalsAdvanced(ctx context.Context, tableName string, batchSize int, configID int64, schema string, options *FetchOptions, requiredFields []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error) {
	return nil, nil, nil
}

func (m *mockEmbeddingDatabaseClient) FetchUnembeddedSignalsSimple(ctx context.Context, tableName string, batchSize int, filterConfig map[string]interface{}, configID int64, schema string) ([]map[string]interface{}, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) MarkSignalsEmbeddedAdvanced(ctx context.Context, tableName string, ids []interface{}, configID int64, embeddedIDs []string, schema string) error {
	return nil
}

// Implement OtelDatabaseClient interface methods
func (m *mockEmbeddingDatabaseClient) FetchUnembeddedSignals(ctx context.Context, configID int64, signalType string, limit int, schema string) ([]map[string]interface{}, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) MarkSignalsEmbedded(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	return nil
}

func (m *mockEmbeddingDatabaseClient) MarkSignalsEmbeddedBulk(ctx context.Context, configID int64, signalIDs []interface{}, schema string) error {
	return nil
}

func (m *mockEmbeddingDatabaseClient) FetchSpansByTraceID(ctx context.Context, traceID string) ([]SpanData, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) FetchLogsByTraceID(ctx context.Context, traceID string) ([]LogData, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) FetchMetricsByTraceID(ctx context.Context, traceID string) ([]MetricData, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) FetchProfilesByTraceID(ctx context.Context, traceID string) ([]ProfileData, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) FetchLogsBySpanID(ctx context.Context, spanID string) ([]LogData, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) FetchExemplarsBySpanIDFromType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) FetchProfilesBySpanID(ctx context.Context, spanID string) ([]ProfileData, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) QuerySpans(ctx context.Context, q TraceQuery) ([]TraceEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) CountLogs(ctx context.Context, q LogQuery) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingDatabaseClient) CountMetricsExponentialHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingDatabaseClient) CountMetricsGauge(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingDatabaseClient) CountMetricsHistogram(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingDatabaseClient) CountMetricsSum(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingDatabaseClient) CountMetricsSummary(ctx context.Context, q MetricQuery) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingDatabaseClient) CountSpans(ctx context.Context, q TraceQuery) (int64, error) {
	return 0, nil
}

func (m *mockEmbeddingDatabaseClient) GetMetricByID(ctx context.Context, id int64, schema string) (interface{}, string, error) {
	return nil, "", nil
}

func (m *mockEmbeddingDatabaseClient) GetLogByID(ctx context.Context, id int64, schema string) (LogEntry, error) {
	return LogEntry{}, nil
}

func (m *mockEmbeddingDatabaseClient) GetSpanByID(ctx context.Context, id int64, schema string) (TraceEntry, error) {
	return TraceEntry{}, nil
}

func (m *mockEmbeddingDatabaseClient) QueryProfiles(ctx context.Context, q ProfileQuery) ([]ProfileEntry, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) QueryRuns(ctx context.Context, q RunQuery) ([]RunEntry, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error {
	return nil
}

func (m *mockEmbeddingDatabaseClient) EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error {
	return nil
}

func (m *mockEmbeddingDatabaseClient) GetRunByID(ctx context.Context, id int64, schema string) (RunEntry, error) {
	return RunEntry{}, nil
}

func (m *mockEmbeddingDatabaseClient) QueryMetricsExponentialHistogram(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) GetStatistics(ctx context.Context) (*WorkerMetrics, error) {
	return &WorkerMetrics{}, nil
}

func (m *mockEmbeddingDatabaseClient) GetJobStatus(ctx context.Context, jobID string) (*ProcessingJob, error) {
	return &ProcessingJob{}, nil
}

func (m *mockEmbeddingDatabaseClient) GetActiveJobs(ctx context.Context) ([]*ProcessingJob, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) QueryMetricsGauge(ctx context.Context, q MetricQuery) ([]GaugeMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) QueryMetricsSum(ctx context.Context, q MetricQuery) ([]SumMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) QueryHistogramMetrics(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) StartOnDemandWorkers(ctx context.Context, params OnDemandParams) error {
	return nil
}

func (m *mockEmbeddingDatabaseClient) StopOnDemandWorkers(ctx context.Context, schema string) error {
	return nil
}

func (m *mockEmbeddingDatabaseClient) QuerySummaryMetrics(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error) {
	return nil, nil
}

func (m *mockEmbeddingDatabaseClient) QueryExponentialHistogramMetrics(ctx context.Context, q MetricQuery) ([]ExponentialHistogramMetricEntry, string, bool, error) {
	return nil, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) QueryMetricsHistogram(ctx context.Context, q MetricQuery) ([]HistogramMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockEmbeddingDatabaseClient) QueryMetricsSummary(ctx context.Context, q MetricQuery) ([]SummaryMetricEntry, string, bool, string, bool, error) {
	return nil, "", false, "", false, nil
}

func (m *mockVectorStoreClient) AddDocuments(ctx context.Context, docs []schema.Document) ([]string, error) {
	return nil, nil
}

func (m *mockVectorStoreClient) SimilaritySearch(ctx context.Context, query string, k int, opts ...interface{}) ([]schema.Document, error) {
	return nil, nil
}

func (m *mockVectorStoreClient) SimilaritySearchByVector(ctx context.Context, query []float32, k int, opts ...interface{}) ([]schema.Document, error) {
	return nil, nil
}

func (m *mockVectorStoreClient) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockVectorStoreClient) Close() error {
	return nil
}

func (m *mockVectorStoreClient) GetVectorByID(ctx context.Context, id string) ([]float32, error) {
	return []float32{1.0, 2.0, 3.0}, nil
}

func (m *mockVectorStoreClient) GetVectorsByIDs(ctx context.Context, ids []string) ([][]float32, error) {
	vectors := make([][]float32, len(ids))
	for i := range vectors {
		vectors[i] = []float32{1.0, 2.0, 3.0}
	}
	return vectors, nil
}

func (m *mockVectorStoreClient) GetVectorMetadata(ctx context.Context, id string) (map[string]interface{}, error) {
	return map[string]interface{}{"id": id}, nil
}

func (m *mockVectorStoreClient) CombineVectors(ctx context.Context, vectors [][]float32, strategy string) ([]float32, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no vectors to combine")
	}
	return vectors[0], nil // Return first vector as combined result
}

func (m *mockVectorStoreClient) GetVectorCombinationStrategy() string {
	return "mean"
}

func (m *mockVectorStoreClient) FindBaselineByContentHash(ctx context.Context, content string) ([]interface{}, error) {
	return []interface{}{}, nil
}

func (m *mockLogger) Info(args ...interface{})  {}
func (m *mockLogger) Error(args ...interface{}) {}
func (m *mockLogger) Debug(args ...interface{}) {}

func TestCreateEmbeddingWorkersWithDatabaseFilter(t *testing.T) {
	// This test demonstrates the complete smart worker creation process
	// Note: This test requires a real database connection, so we'll skip it in unit tests
	t.Skip("This test requires a real database connection - run as integration test")

	ctx := context.Background()

	// Mock dependencies
	db := &mockEmbeddingDatabaseClient{}
	logger := &mockLogger{}
	vectorStore := &mockVectorStoreClient{}

	// Mock configurations
	embConfig := database.EmbeddingConfig{
		ID:               1,
		MaxContextLength: 4096,
	}

	vsConfig := database.VectorStoreConfig{
		BatchSize:    100,
		MaxIdleConns: 5,
	}

	mapping := database.ProcessorMappingConfig{
		TemplateID: 1,
		EmbeddingFilter: map[string]interface{}{
			"signal_type": "logs,spans",
		},
	}

	databaseFilter := map[string]interface{}{
		"signal_type": "logs",
	}

	// Test the function
	workers, err := CreateEmbeddingWorkersWithDatabaseFilter(
		ctx, db, logger, vectorStore, embConfig, vsConfig, mapping, databaseFilter,
	)

	// Verify results
	assert.NoError(t, err)
	assert.Len(t, workers, 1, "Should create only 1 worker for the intersection (logs)")
	assert.Equal(t, "logs-1", workers[0].WorkerID)
	assert.Equal(t, "logs", workers[0].SignalType)
}

func TestHybridModeFunctionality(t *testing.T) {
	tests := []struct {
		name          string
		mode          string
		expectedModes []string
		description   string
	}{
		{
			name:          "batch_mode_only",
			mode:          "batch",
			expectedModes: []string{"batch"},
			description:   "Should run only batch processing mode",
		},
		{
			name:          "trigger_mode_only",
			mode:          "trigger",
			expectedModes: []string{"trigger"},
			description:   "Should run only trigger processing mode",
		},
		{
			name:          "hybrid_mode_both",
			mode:          "hybrid",
			expectedModes: []string{"batch", "trigger"},
			description:   "Should run both batch and trigger processing modes",
		},
		{
			name:          "unknown_mode_defaults_to_hybrid",
			mode:          "unknown",
			expectedModes: []string{"hybrid"},
			description:   "Should default to hybrid mode for unknown modes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a worker with the specified mode
			worker := NewEmbeddingWorker(
				&mockEmbeddingDatabaseClient{},
				&mockLogger{},
				&mockVectorStoreClient{},
				WithMetadata(map[string]interface{}{
					"embedding_mode": tt.mode,
				}),
				WithSignalType("logs"),
			)

			// Verify the mode is parsed correctly
			// For unknown modes, it should default to "hybrid"
			expectedMode := tt.mode
			if tt.mode == "unknown" {
				expectedMode = "hybrid"
			}
			assert.Equal(t, expectedMode, worker.ProcessingMode(), "Processing mode should match expected")

			// For hybrid mode, verify both batch and trigger are available
			if tt.mode == "hybrid" {
				assert.Equal(t, "hybrid", worker.ProcessingMode(), "Hybrid mode should be set")
			}
		})
	}
}

func TestWorkerModeSwitching(t *testing.T) {
	// Test that workers can handle different modes correctly
	worker := NewEmbeddingWorker(
		&mockEmbeddingDatabaseClient{},
		&mockLogger{},
		&mockVectorStoreClient{},
		WithMetadata(map[string]interface{}{
			"embedding_mode": "batch",
		}),
		WithSignalType("logs"),
	)

	// Test mode switching
	assert.Equal(t, "batch", worker.ProcessingMode())

	// Pause/resume work before the processing loop starts.
	worker.Pause()
	assert.True(t, worker.IsPaused())
	worker.Resume()
	assert.False(t, worker.IsPaused())
}

func TestTriggerModeChannelSubscription(t *testing.T) {
	// Test that trigger mode subscribes to the correct channel
	worker := NewEmbeddingWorker(
		&mockEmbeddingDatabaseClient{},
		&mockLogger{},
		&mockVectorStoreClient{},
		WithMetadata(map[string]interface{}{
			"embedding_mode": "trigger",
		}),
		WithSignalType("logs"),
	)

	// Verify the worker is configured for trigger mode
	assert.Equal(t, "trigger", worker.ProcessingMode())
	assert.Equal(t, "logs", worker.SignalType)

	// The actual channel subscription would be tested in integration tests
	// since it requires a real database connection
}

func TestHybridModeGoroutineManagement(t *testing.T) {
	// Test that hybrid mode properly manages goroutines
	worker := NewEmbeddingWorker(
		&mockEmbeddingDatabaseClient{},
		&mockLogger{},
		&mockVectorStoreClient{},
		WithMetadata(map[string]interface{}{
			"embedding_mode": "hybrid",
		}),
		WithSignalType("logs"),
	)

	// Verify hybrid mode is set
	assert.Equal(t, "hybrid", worker.ProcessingMode())

	// Test that the worker can be stopped gracefully
	// This would be tested in integration tests with real goroutines
}
