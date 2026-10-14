package repository

import (
	"testing"
)

func TestSignalTypeSimplification(t *testing.T) {
	// Test that we can use signal types directly
	signalTypes := []string{
		SignalTypeHistogramMetrics,
		SignalTypeGaugeMetrics,
		SignalTypeSumMetrics,
		SignalTypeSummaryMetrics,
		SignalTypeExponentialHistogramMetrics,
	}

	// Verify signal types are correctly defined
	expectedTypes := []string{
		"histogram_metrics",
		"gauge_metrics",
		"sum_metrics",
		"summary_metrics",
		"exponential_histogram_metrics",
	}

	for i, signalType := range signalTypes {
		if signalType != expectedTypes[i] {
			t.Errorf("Signal type mismatch: got %s, want %s", signalType, expectedTypes[i])
		}
	}

	// Test template field extractor with signal types
	extractor := NewTemplateFieldExtractor()

	// Test template that needs histogram exemplars
	template := `{{.service_name}} for operation {{.name}}. {{if .exemplars_histogram}} Histogram Exemplars: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`

	analysis := extractor.AnalyzeTemplate(template)

	if !analysis.NeedsExemplars {
		t.Error("Template should need exemplars")
	}

	if len(analysis.ExemplarTypes) != 1 {
		t.Errorf("Expected 1 exemplar type, got %d", len(analysis.ExemplarTypes))
	}

	if analysis.ExemplarTypes[0] != SignalTypeHistogramMetrics {
		t.Errorf("Expected histogram_metrics, got %s", analysis.ExemplarTypes[0])
	}
}
