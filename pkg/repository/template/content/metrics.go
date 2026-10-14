package content

import (
	"sort"
	"sync"
	"time"
)

// TemplateMetrics tracks template rendering performance and errors
type TemplateMetrics struct {
	mu              sync.RWMutex
	renderCounts    map[string]int64
	renderErrors    map[string]int64
	renderDurations map[string][]time.Duration
	lastReset       time.Time
}

// NewTemplateMetrics creates a new template metrics tracker
func NewTemplateMetrics() *TemplateMetrics {
	return &TemplateMetrics{
		renderCounts:    make(map[string]int64),
		renderErrors:    make(map[string]int64),
		renderDurations: make(map[string][]time.Duration),
		lastReset:       time.Now(),
	}
}

// RecordRender records a successful template render
func (tm *TemplateMetrics) RecordRender(templateName string, duration time.Duration) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.renderCounts[templateName]++

	// Keep only last 100 durations for each template to prevent memory leaks
	if len(tm.renderDurations[templateName]) >= 100 {
		tm.renderDurations[templateName] = tm.renderDurations[templateName][1:]
	}
	tm.renderDurations[templateName] = append(tm.renderDurations[templateName], duration)
}

// RecordRenderError records a template render error
func (tm *TemplateMetrics) RecordRenderError(templateName string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.renderErrors[templateName]++
}

// GetMetrics returns a copy of current metrics
func (tm *TemplateMetrics) GetMetrics() map[string]interface{} {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	metrics := make(map[string]interface{})

	// Template-specific metrics
	templateMetrics := make(map[string]map[string]interface{})
	for templateName := range tm.renderCounts {
		templateMetrics[templateName] = map[string]interface{}{
			"render_count":    tm.renderCounts[templateName],
			"error_count":     tm.renderErrors[templateName],
			"success_rate":    tm.calculateSuccessRate(templateName),
			"avg_duration_ms": tm.calculateAverageDuration(templateName),
			"p95_duration_ms": tm.calculatePercentileDuration(templateName, 95),
			"p99_duration_ms": tm.calculatePercentileDuration(templateName, 99),
		}
	}

	metrics["templates"] = templateMetrics
	metrics["total_renders"] = tm.getTotalRenders()
	metrics["total_errors"] = tm.getTotalErrors()
	metrics["overall_success_rate"] = tm.calculateOverallSuccessRate()
	metrics["last_reset"] = tm.lastReset

	return metrics
}

// calculateSuccessRate calculates success rate for a template
func (tm *TemplateMetrics) calculateSuccessRate(templateName string) float64 {
	total := tm.renderCounts[templateName] + tm.renderErrors[templateName]
	if total == 0 {
		return 0.0
	}
	return float64(tm.renderCounts[templateName]) / float64(total) * 100.0
}

// calculateAverageDuration calculates average render duration for a template
func (tm *TemplateMetrics) calculateAverageDuration(templateName string) float64 {
	durations := tm.renderDurations[templateName]
	if len(durations) == 0 {
		return 0.0
	}

	var total time.Duration
	for _, d := range durations {
		total += d
	}
	return float64(total.Microseconds()) / float64(len(durations)) / 1000.0 // Convert to ms
}

// calculatePercentileDuration calculates percentile duration for a template
func (tm *TemplateMetrics) calculatePercentileDuration(templateName string, percentile int) float64 {
	durations := tm.renderDurations[templateName]
	if len(durations) == 0 {
		return 0.0
	}

	// Sort durations
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := int(float64(len(sorted)-1) * float64(percentile) / 100.0)
	return float64(sorted[index].Microseconds()) / 1000.0 // Convert to ms
}

// getTotalRenders returns total render count across all templates
func (tm *TemplateMetrics) getTotalRenders() int64 {
	var total int64
	for _, count := range tm.renderCounts {
		total += count
	}
	return total
}

// getTotalErrors returns total error count across all templates
func (tm *TemplateMetrics) getTotalErrors() int64 {
	var total int64
	for _, count := range tm.renderErrors {
		total += count
	}
	return total
}

// calculateOverallSuccessRate calculates overall success rate
func (tm *TemplateMetrics) calculateOverallSuccessRate() float64 {
	total := tm.getTotalRenders() + tm.getTotalErrors()
	if total == 0 {
		return 0.0
	}
	return float64(tm.getTotalRenders()) / float64(total) * 100.0
}

// ResetMetrics resets all metrics
func (tm *TemplateMetrics) ResetMetrics() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.renderCounts = make(map[string]int64)
	tm.renderErrors = make(map[string]int64)
	tm.renderDurations = make(map[string][]time.Duration)
	tm.lastReset = time.Now()
}
