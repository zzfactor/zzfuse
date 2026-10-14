package util

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

// BenchmarkCountLogsBySeverity benchmarks the log counting function
func BenchmarkCountLogsBySeverity(b *testing.B) {
	// Test data with mixed types
	logs := []map[string]interface{}{
		{"severity_text": "ERROR"},
		{"severity_text": "WARN"},
		{"severity_text": "INFO"},
		{"severity_text": "DEBUG"},
		{"severity_text": "INFO"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CountLogsBySeverity(logs)
	}
}

// BenchmarkCountLogsBySeverityWithStructs benchmarks with LogData structs
func BenchmarkCountLogsBySeverityWithStructs(b *testing.B) {
	// Test data with LogData structs (type assertion path)
	logs := []LogData{
		{SeverityText: stringPtr("ERROR")},
		{SeverityText: stringPtr("WARN")},
		{SeverityText: stringPtr("INFO")},
		{SeverityText: stringPtr("DEBUG")},
		{SeverityText: stringPtr("INFO")},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CountLogsBySeverity(logs)
	}
}

// BenchmarkCountEventsByName benchmarks the event counting function
func BenchmarkCountEventsByName(b *testing.B) {
	// Test data
	events := []map[string]interface{}{
		{"name": "db_query"},
		{"name": "cache_hit"},
		{"name": "http_request"},
		{"name": "db_query"},
		{"name": "cache_hit"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CountEventsByName(events)
	}
}

// BenchmarkFormatLogCountsBySeverity benchmarks the log formatting function
func BenchmarkFormatLogCountsBySeverity(b *testing.B) {
	logs := []map[string]interface{}{
		{"severity_text": "ERROR"},
		{"severity_text": "WARN"},
		{"severity_text": "INFO"},
		{"severity_text": "DEBUG"},
		{"severity_text": "INFO"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatLogCountsBySeverity(logs)
	}
}

// BenchmarkFormatEventCountsByName benchmarks the event formatting function
func BenchmarkFormatEventCountsByName(b *testing.B) {
	events := []map[string]interface{}{
		{"name": "db_query"},
		{"name": "cache_hit"},
		{"name": "http_request"},
		{"name": "db_query"},
		{"name": "cache_hit"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		FormatEventCountsByName(events)
	}
}

// Helper function for benchmarks
func stringPtr(s string) *string {
	return &s
}

func TestFormatExemplarCountsByType(t *testing.T) {
	tests := []struct {
		name      string
		exemplars interface{}
		expected  string
	}{
		{
			name:      "nil exemplars",
			exemplars: nil,
			expected:  "0 exemplars",
		},
		{
			name:      "empty exemplars",
			exemplars: []interface{}{},
			expected:  "0 exemplars",
		},
		{
			name: "single exemplar",
			exemplars: []map[string]interface{}{
				{"metric_type": "histogram"},
			},
			expected: "1 exemplars (1 histogram)",
		},
		{
			name: "multiple exemplars same type",
			exemplars: []map[string]interface{}{
				{"metric_type": "histogram"},
				{"metric_type": "histogram"},
				{"metric_type": "histogram"},
			},
			expected: "3 exemplars (3 histogram)",
		},
		{
			name: "multiple exemplars different types",
			exemplars: []map[string]interface{}{
				{"metric_type": "histogram"},
				{"metric_type": "summary"},
				{"metric_type": "gauge"},
			},
			expected: "3 exemplars (1 gauge, 1 histogram, 1 summary)",
		},
		{
			name: "exemplars with unknown type",
			exemplars: []map[string]interface{}{
				{"metric_type": "histogram"},
				{"other_field": "value"},
			},
			expected: "2 exemplars (1 histogram, 1 unknown)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatExemplarCountsByType(tt.exemplars)
			assert.Equal(t, tt.expected, result)
		})
	}
}
