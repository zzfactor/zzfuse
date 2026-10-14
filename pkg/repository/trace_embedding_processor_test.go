package repository

import (
	"context"
	"testing"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestTraceEmbeddingProcessorCreation tests that the processor can be created
func TestTraceEmbeddingProcessorCreation(t *testing.T) {
	mockDB := &MockEmbeddingDatabaseClient{}
	mockVectorStore := &MockVectorStoreClient{}
	mockLogger := &MockLogger{}

	processor := NewTraceEmbeddingProcessor(mockDB, mockVectorStore, mockLogger, 1, "spans")

	assert.NotNil(t, processor)
	assert.Equal(t, mockDB, processor.db)
	assert.Equal(t, mockVectorStore, processor.vectorStore)
	assert.Equal(t, mockLogger, processor.logger)
	assert.Equal(t, int64(1), processor.configID)
	assert.Equal(t, "spans", processor.signalType)
}

// TestTraceEmbeddingProcessorNonSpansSignalType tests that non-spans signals are ignored
func TestTraceEmbeddingProcessorNonSpansSignalType(t *testing.T) {
	mockDB := &MockEmbeddingDatabaseClient{}
	mockVectorStore := &MockVectorStoreClient{}
	mockLogger := &MockLogger{}

	processor := NewTraceEmbeddingProcessor(mockDB, mockVectorStore, mockLogger, 1, "logs")

	// Process with non-spans signal type
	batch := []map[string]interface{}{
		{"id": "1", "span_id": "span1", "trace_id": "trace1"},
	}

	err := processor.ProcessTraceEmbeddings(context.Background(), batch)
	assert.NoError(t, err) // Should return early without error
}

// TestTraceEmbeddingProcessorMetrics tests that metrics are properly tracked
func TestTraceEmbeddingProcessorMetrics(t *testing.T) {
	metrics := NewTraceEmbeddingMetrics()

	// Record some operations
	metrics.RecordOperation(768, 100*time.Millisecond)
	metrics.RecordOperation(768, 50*time.Millisecond)

	assert.Equal(t, int64(2), metrics.VectorOperations)
	assert.Equal(t, int64(1536), metrics.VectorDimensions)
	assert.Equal(t, 150*time.Millisecond, metrics.CombinationTime)
	assert.Equal(t, 0.0002, metrics.CostEstimate)
}

// TestTraceEmbeddingProcessorDetectQualifyingRootSpans tests root span detection
func TestTraceEmbeddingProcessorDetectQualifyingRootSpans(t *testing.T) {
	mockDB := &MockEmbeddingDatabaseClient{}
	mockVectorStore := &MockVectorStoreClient{}
	mockLogger := util.NoopLogger{}

	processor := NewTraceEmbeddingProcessor(mockDB, mockVectorStore, mockLogger, 1, "spans")

	// Test batch with root spans and child spans
	batch := []map[string]interface{}{
		{
			"span_id":  "root1",
			"trace_id": "trace1",
			// No parent_span_id = root span
		},
		{
			"span_id":        "child1",
			"trace_id":       "trace1",
			"parent_span_id": "root1", // Child span
		},
		{
			"span_id":  "root2",
			"trace_id": "trace2",
			// No parent_span_id = root span
		},
	}

	// Mock the database calls
	mockDB.On("FetchSpansByTraceID", mock.Anything, "trace1").Return([]SpanData{
		{SpanID: "root1", ParentSpanID: nil},
		{SpanID: "child1", ParentSpanID: stringPtr("root1")},
	}, nil)
	mockDB.On("FetchSpansByTraceID", mock.Anything, "trace2").Return([]SpanData{
		{SpanID: "root2", ParentSpanID: nil},
	}, nil)

	qualifyingRoots := processor.detectQualifyingRootSpans(context.Background(), batch)

	// The database client is available: only the root with children qualifies.
	require.Len(t, qualifyingRoots, 1)
	assert.Equal(t, "root1", qualifyingRoots[0]["span_id"])
	assert.Equal(t, "trace1", qualifyingRoots[0]["trace_id"])
	mockDB.AssertExpectations(t)
}

// MockRow is a mock implementation for database rows
type MockRow struct {
	embeddedID      string
	traceEmbeddedID string
}

func (m *MockRow) Scan(dest ...interface{}) error {
	if len(dest) >= 2 {
		// Mock the scan behavior for embedded_id and trace_embedded_id
		if embeddedID, ok := dest[0].(*interface{}); ok {
			*embeddedID = m.embeddedID
		}
		if traceEmbeddedID, ok := dest[1].(*interface{}); ok {
			*traceEmbeddedID = m.traceEmbeddedID
		}
	}
	return nil
}
