package database

import (
	"testing"
	"time"
)

func TestRunMetadata_ToMap(t *testing.T) {
	metadata := &RunMetadata{
		SpanCount:       100,
		SpanEventCount:  50,
		LogCount:        200,
		LogInfoCount:    150,
		LogWarningCount: 30,
		LogDebugCount:   15,
		LogErrorCount:   5,
		SpanErrorCount:  10,
	}

	result := metadata.ToMap()

	expected := map[string]interface{}{
		"SPAN_COUNT":        int64(100),
		"SPAN_EVENT_COUNT":  int64(50),
		"LOG_COUNT":         int64(200),
		"LOG_INFO_COUNT":    int64(150),
		"LOG_WARNING_COUNT": int64(30),
		"LOG_DEBUG_COUNT":   int64(15),
		"LOG_ERROR_COUNT":   int64(5),
		"SPAN_ERROR_COUNT":  int64(10),
	}

	for key, expectedValue := range expected {
		if actualValue, exists := result[key]; !exists {
			t.Errorf("Expected key %s to exist in result", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected %s to be %v, got %v", key, expectedValue, actualValue)
		}
	}
}

func TestRunMetadata_ToMap_ZeroValues(t *testing.T) {
	metadata := &RunMetadata{
		SpanCount: 0,
		LogCount:  0,
	}

	result := metadata.ToMap()

	// Should not include zero values
	if len(result) != 0 {
		t.Errorf("Expected empty map for zero values, got %v", result)
	}
}

func TestFromMap(t *testing.T) {
	data := map[string]interface{}{
		"SPAN_COUNT":        float64(100),
		"SPAN_EVENT_COUNT":  float64(50),
		"LOG_COUNT":         float64(200),
		"LOG_INFO_COUNT":    float64(150),
		"LOG_WARNING_COUNT": float64(30),
		"LOG_DEBUG_COUNT":   float64(15),
		"LOG_ERROR_COUNT":   float64(5),
		"SPAN_ERROR_COUNT":  float64(10),
	}

	metadata := FromMap(data)

	if metadata.SpanCount != 100 {
		t.Errorf("Expected SpanCount to be 100, got %d", metadata.SpanCount)
	}
	if metadata.SpanEventCount != 50 {
		t.Errorf("Expected SpanEventCount to be 50, got %d", metadata.SpanEventCount)
	}
	if metadata.LogCount != 200 {
		t.Errorf("Expected LogCount to be 200, got %d", metadata.LogCount)
	}
	if metadata.LogInfoCount != 150 {
		t.Errorf("Expected LogInfoCount to be 150, got %d", metadata.LogInfoCount)
	}
	if metadata.LogWarningCount != 30 {
		t.Errorf("Expected LogWarningCount to be 30, got %d", metadata.LogWarningCount)
	}
	if metadata.LogDebugCount != 15 {
		t.Errorf("Expected LogDebugCount to be 15, got %d", metadata.LogDebugCount)
	}
	if metadata.LogErrorCount != 5 {
		t.Errorf("Expected LogErrorCount to be 5, got %d", metadata.LogErrorCount)
	}
	if metadata.SpanErrorCount != 10 {
		t.Errorf("Expected SpanErrorCount to be 10, got %d", metadata.SpanErrorCount)
	}
}

func TestFromMap_MissingFields(t *testing.T) {
	data := map[string]interface{}{
		"SPAN_COUNT": float64(100),
		"LOG_COUNT":  float64(200),
	}

	metadata := FromMap(data)

	if metadata.SpanCount != 100 {
		t.Errorf("Expected SpanCount to be 100, got %d", metadata.SpanCount)
	}
	if metadata.LogCount != 200 {
		t.Errorf("Expected LogCount to be 200, got %d", metadata.LogCount)
	}
	// Missing fields should be zero
	if metadata.SpanEventCount != 0 {
		t.Errorf("Expected SpanEventCount to be 0, got %d", metadata.SpanEventCount)
	}
}

func TestRun_GetStructuredMetadata(t *testing.T) {
	run := &Run{
		Metadata: map[string]interface{}{
			"SPAN_COUNT":      float64(100),
			"LOG_COUNT":       float64(200),
			"LOG_ERROR_COUNT": float64(5),
		},
	}

	metadata := run.GetStructuredMetadata()

	if metadata.SpanCount != 100 {
		t.Errorf("Expected SpanCount to be 100, got %d", metadata.SpanCount)
	}
	if metadata.LogCount != 200 {
		t.Errorf("Expected LogCount to be 200, got %d", metadata.LogCount)
	}
	if metadata.LogErrorCount != 5 {
		t.Errorf("Expected LogErrorCount to be 5, got %d", metadata.LogErrorCount)
	}
}

func TestRun_SetStructuredMetadata(t *testing.T) {
	run := &Run{}
	metadata := &RunMetadata{
		SpanCount:     100,
		LogCount:      200,
		LogErrorCount: 5,
	}

	run.SetStructuredMetadata(metadata)

	expected := map[string]interface{}{
		"SPAN_COUNT":      int64(100),
		"LOG_COUNT":       int64(200),
		"LOG_ERROR_COUNT": int64(5),
	}

	for key, expectedValue := range expected {
		if actualValue, exists := run.Metadata[key]; !exists {
			t.Errorf("Expected key %s to exist in metadata", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected %s to be %v, got %v", key, expectedValue, actualValue)
		}
	}
}

func TestNewRunWithStructuredMetadata(t *testing.T) {
	metadata := &RunMetadata{
		SpanCount:     100,
		LogCount:      200,
		LogErrorCount: 5,
	}

	scopeName := "test-scope"
	run := NewRunWithStructuredMetadata(
		"test-service",
		&scopeName,
		"test-run",
		1234567890,
		nil,
		nil,
		nil,
		nil,
		metadata,
	)

	if run.ServiceName != "test-service" {
		t.Errorf("Expected ServiceName to be 'test-service', got %s", run.ServiceName)
	}
	if run.Name != "test-run" {
		t.Errorf("Expected Name to be 'test-run', got %s", run.Name)
	}
	if run.StartTimestampNs != 1234567890 {
		t.Errorf("Expected StartTimestampNs to be 1234567890, got %d", run.StartTimestampNs)
	}

	// Check that metadata was properly converted
	if run.Metadata["SPAN_COUNT"] != int64(100) {
		t.Errorf("Expected SPAN_COUNT to be 100, got %v", run.Metadata["SPAN_COUNT"])
	}
	if run.Metadata["LOG_COUNT"] != int64(200) {
		t.Errorf("Expected LOG_COUNT to be 200, got %v", run.Metadata["LOG_COUNT"])
	}
	if run.Metadata["LOG_ERROR_COUNT"] != int64(5) {
		t.Errorf("Expected LOG_ERROR_COUNT to be 5, got %v", run.Metadata["LOG_ERROR_COUNT"])
	}
}

// Example usage function to demonstrate how to use the structured metadata
func ExampleRunMetadata_usage() {
	// Create a new run with structured metadata
	metadata := &RunMetadata{
		SpanCount:       1000,
		SpanEventCount:  500,
		LogCount:        2000,
		LogInfoCount:    1500,
		LogWarningCount: 300,
		LogDebugCount:   150,
		LogErrorCount:   50,
		SpanErrorCount:  25,
	}

	// Create a run with the structured metadata
	scopeName := "my-scope"
	run := NewRunWithStructuredMetadata(
		"my-service",
		&scopeName,
		"processing-run",
		time.Now().UnixNano(),
		nil,
		nil,
		nil,
		nil,
		metadata,
	)

	// Access structured metadata
	structuredMetadata := run.GetStructuredMetadata()
	_ = structuredMetadata.SpanCount
	_ = structuredMetadata.LogErrorCount

	// Update metadata
	structuredMetadata.SpanCount += 50
	run.SetStructuredMetadata(structuredMetadata)

	// Convert back to map for database storage
	dbMetadata := structuredMetadata.ToMap()
	_ = dbMetadata
}
