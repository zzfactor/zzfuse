package repository

import (
	"testing"
)

func TestSignalProcessorRegistry(t *testing.T) {
	registry := NewProcessorRegistry()

	// Test that all processors are registered
	processors := []SignalType{
		SignalTypeLogs,
		SignalTypeSpans,
		SignalTypeGaugeMetrics,
		SignalTypeSumMetrics,
		SignalTypeHistogramMetrics,
		SignalTypeSummaryMetrics,
		SignalTypeExponentialHistogramMetrics,
		SignalTypeProfiles,
	}

	for _, signalType := range processors {
		processor, err := registry.GetProcessor(signalType)
		if err != nil {
			t.Errorf("Failed to get processor for %s: %v", signalType, err)
		}
		if processor == nil {
			t.Errorf("Processor for %s is nil", signalType)
		}
		// MetricProcessor returns(SignalTypeMetrics for all metric types
		if processor.GetSignalType() != signalType {
			// Check if this is a metric type that should return(SignalTypeMetrics
			isMetricType := signalType == SignalTypeGaugeMetrics ||
				signalType == SignalTypeSumMetrics ||
				signalType == SignalTypeHistogramMetrics ||
				signalType == SignalTypeSummaryMetrics ||
				signalType == SignalTypeExponentialHistogramMetrics

			if isMetricType && processor.GetSignalType() == SignalTypeMetrics {
				// This is correct behavior
				continue
			}

			t.Errorf("Processor for %s returned wrong signal type: %s", signalType, processor.GetSignalType())
		}
	}
}

func TestLogProcessorExtractText(t *testing.T) {
	processor := NewLogProcessor()

	tests := []struct {
		name     string
		signal   map[string]interface{}
		expected string
	}{
		{
			name: "body as string",
			signal: map[string]interface{}{
				"id":   "123",
				"body": "test log message",
			},
			expected: "test log message",
		},
		{
			name: "body as map with message",
			signal: map[string]interface{}{
				"id": "123",
				"body": map[string]interface{}{
					"message": "test message",
				},
			},
			expected: "test message",
		},
		{
			name: "attributes with message",
			signal: map[string]interface{}{
				"id": "123",
				"attributes": map[string]interface{}{
					"message": "test message",
				},
			},
			expected: "test message",
		},
		{
			name: "fallback to id",
			signal: map[string]interface{}{
				"id": "123",
			},
			expected: "Log signal 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := processor.ExtractText(tt.signal)
			if err != nil {
				t.Errorf("ExtractText failed: %v", err)
			}
			if text != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, text)
			}
		})
	}
}

func TestMetricProcessorExtractText(t *testing.T) {
	processor := NewMetricProcessor()

	tests := []struct {
		name     string
		signal   map[string]interface{}
		expected string
	}{
		{
			name: "complete metric",
			signal: map[string]interface{}{
				"id":    "123",
				"name":  "cpu_usage",
				"value": 85.5,
				"unit":  "percent",
			},
			expected: "cpu_usage 85.5 percent",
		},
		{
			name: "metric without unit",
			signal: map[string]interface{}{
				"id":    "123",
				"name":  "memory_usage",
				"value": 1024,
			},
			expected: "memory_usage 1024",
		},
		{
			name: "fallback to id",
			signal: map[string]interface{}{
				"id": "123",
			},
			expected: "Metric 123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, err := processor.ExtractText(tt.signal)
			if err != nil {
				t.Errorf("ExtractText failed: %v", err)
			}
			if text != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, text)
			}
		})
	}
}

func TestProcessorRegistryProcessSignals(t *testing.T) {
	registry := NewProcessorRegistry()

	signals := []interface{}{
		map[string]interface{}{
			"id":   "log1",
			"body": "test log 1",
		},
		map[string]interface{}{
			"id":   "log2",
			"body": "test log 2",
		},
		map[string]interface{}{
			"id":    "metric1",
			"name":  "cpu_usage",
			"value": 85.5,
		},
	}

	config := ChunkingConfig{
		MaxSpansPerChunk: 2,
		MaxMessageLength: 1000,
	}

	chunks, err := registry.ProcessSignals(signals, config)
	if err != nil {
		t.Errorf("ProcessSignals failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected chunks, got empty result")
	}

	// Should have at least 2 chunks (logs and metrics)
	if len(chunks) < 2 {
		t.Errorf("Expected at least 2 chunks, got %d", len(chunks))
	}
}
