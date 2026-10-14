package repository

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"
)

// =========================
// Trace Content Builder (Simplified)
// =========================

// TraceContentBuilder builds complete trace content by fetching and correlating all signals
type TraceContentBuilder struct {
	db            OtelDatabaseClient
	exemplarTypes []string // Specific metric types to query for exemplars
}

// NewTraceContentBuilder creates a new trace content builder
func NewTraceContentBuilder(db OtelDatabaseClient) *TraceContentBuilder {
	return &TraceContentBuilder{
		db: db,
	}
}

// NewTraceContentBuilderWithExemplarTypes creates a new TraceContentBuilder with specific exemplar types
func NewTraceContentBuilderWithExemplarTypes(db OtelDatabaseClient, exemplarTypes []string) *TraceContentBuilder {
	return &TraceContentBuilder{db: db, exemplarTypes: exemplarTypes}
}

// SetExemplarTypes sets the specific metric types to query for exemplars
func (b *TraceContentBuilder) SetExemplarTypes(exemplarTypes []string) {
	b.exemplarTypes = exemplarTypes
}

// SetExemplarMetricTypes sets the specific signal types to query for exemplars
func (b *TraceContentBuilder) SetExemplarMetricTypes(signalTypes []string) {
	b.exemplarTypes = make([]string, 0, len(signalTypes))
	for _, st := range signalTypes {
		if st != "" {
			b.exemplarTypes = append(b.exemplarTypes, st)
		}
	}
}

// GetExemplarMetricTypes returns the configured signal types
func (b *TraceContentBuilder) GetExemplarMetricTypes() []string {
	if len(b.exemplarTypes) > 0 {
		return b.exemplarTypes
	}
	// Default to all metric signal types if none specified
	return []string{
		SignalTypeHistogramMetrics,
		SignalTypeSummaryMetrics,
		SignalTypeGaugeMetrics,
		SignalTypeSumMetrics,
		SignalTypeExponentialHistogramMetrics,
	}
}

// BuildCompleteContent builds complete trace content for a given trace ID
func (b *TraceContentBuilder) BuildCompleteContent(ctx context.Context, traceID string) (*CompleteTraceContent, error) {
	// Fetch all signals for the trace
	spans, err := b.db.FetchSpansByTraceID(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spans for trace %s: %w", traceID, err)
	}

	if len(spans) == 0 {
		return nil, fmt.Errorf("no spans found for trace %s", traceID)
	}

	logs, err := b.db.FetchLogsByTraceID(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch logs for trace %s: %w", traceID, err)
	}

	metrics, err := b.db.FetchMetricsByTraceID(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics for trace %s: %w", traceID, err)
	}

	profiles, err := b.db.FetchProfilesByTraceID(ctx, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch profiles for trace %s: %w", traceID, err)
	}

	// Calculate trace timing
	startTime, endTime := b.calculateTraceTiming(spans)
	duration := endTime.Sub(startTime).Nanoseconds()

	// Get service name from first span
	serviceName := ""
	if len(spans) > 0 && spans[0].ServiceName != nil {
		serviceName = *spans[0].ServiceName
	}

	// Build complete trace content
	content := &CompleteTraceContent{
		TraceID:     traceID,
		ServiceName: serviceName,
		StartTime:   startTime,
		EndTime:     endTime,
		Duration:    duration,
		Spans:       spans,
		Logs:        logs,
		Metrics:     metrics,
		Profiles:    profiles,
		Metadata:    make(map[string]interface{}),
	}

	return content, nil
}

// BuildCompleteContentFromSpan builds complete trace content using a single span as the base
// This is useful when processing individual signals that might not have all trace data available yet
func (b *TraceContentBuilder) BuildCompleteContentFromSpan(ctx context.Context, span SpanData) (*CompleteTraceContent, error) {
	traceID := span.TraceID

	// Try to fetch all spans for the trace, fallback to single span if not found
	spans, err := b.db.FetchSpansByTraceID(ctx, traceID)
	if err != nil || len(spans) == 0 {
		spans = []SpanData{span}
	}

	// Fetch correlated signals with graceful error handling
	var logs []LogData

	// When building content from a specific span, only query logs by span ID
	// to avoid duplicates and get only logs that belong to this specific span
	if spanLogs, err := b.db.FetchLogsBySpanID(ctx, span.SpanID); err == nil {
		logs = spanLogs
	}

	metrics, _ := b.db.FetchMetricsByTraceID(ctx, traceID)
	profiles, _ := b.db.FetchProfilesByTraceID(ctx, traceID)

	// Calculate trace timing
	startTime, endTime := b.calculateTraceTiming(spans)
	duration := endTime.Sub(startTime).Nanoseconds()

	// Get service name from first span
	serviceName := ""
	if len(spans) > 0 && spans[0].ServiceName != nil {
		serviceName = *spans[0].ServiceName
	}

	return &CompleteTraceContent{
		TraceID:     traceID,
		ServiceName: serviceName,
		StartTime:   startTime,
		EndTime:     endTime,
		Duration:    duration,
		Spans:       spans,
		Logs:        logs,
		Metrics:     metrics,
		Profiles:    profiles,
		Metadata:    make(map[string]interface{}),
	}, nil
}

// calculateTraceTiming calculates the start and end time of a trace from its spans
func (b *TraceContentBuilder) calculateTraceTiming(spans []SpanData) (time.Time, time.Time) {
	if len(spans) == 0 {
		return time.Time{}, time.Time{}
	}

	startTime := spans[0].StartTime
	endTime := spans[0].EndTime

	for _, span := range spans {
		if span.StartTime.Before(startTime) {
			startTime = span.StartTime
		}
		if span.EndTime.After(endTime) {
			endTime = span.EndTime
		}
	}

	return startTime, endTime
}

// GetCorrelatedSignals gets signals correlated with a specific span
func (b *TraceContentBuilder) GetCorrelatedSignals(ctx context.Context, content *CompleteTraceContent, span SpanData) *CorrelatedSignals {
	correlated := &CorrelatedSignals{
		SpanID:    span.SpanID,
		Span:      span,
		Logs:      make([]LogData, 0),
		Exemplars: make([]ExemplarData, 0),
		Profiles:  make([]ProfileData, 0),
	}

	// Use optimized span-specific queries if database client is available
	if b.db != nil {
		// Fetch logs directly by span_id (TimescaleDB optimized)
		if logs, err := b.db.FetchLogsBySpanID(ctx, span.SpanID); err == nil {
			correlated.Logs = logs
		} else {
			// Fallback to in-memory filtering if direct query fails
			for _, log := range content.Logs {
				if log.SpanID != nil && *log.SpanID == span.SpanID {
					correlated.Logs = append(correlated.Logs, log)
				}
			}
		}

		// Fetch exemplars by specific metric types (1:1 mapping)
		// This ensures we only query the exact tables specified by the template
		var allExemplars []ExemplarData

		// Use configured exemplar types - templates must explicitly specify which metric types to query
		var metricTypes []string
		if len(b.exemplarTypes) > 0 {
			// Use the exemplar types directly - they should already be in the correct format
			metricTypes = b.exemplarTypes
		}
		// No default fallback - templates must explicitly specify which metric types to query

		// Fetch exemplars for each configured metric type (if any)
		for _, metricType := range metricTypes {
			if exemplars, err := b.db.FetchExemplarsBySpanIDFromType(ctx, span.SpanID, metricType); err == nil {
				allExemplars = append(allExemplars, exemplars...)
			}
		}

		correlated.Exemplars = allExemplars

		// Fetch profiles directly by span_id (TimescaleDB optimized)
		if profiles, err := b.db.FetchProfilesBySpanID(ctx, span.SpanID); err == nil {
			correlated.Profiles = profiles
		} else {
			// Fallback to in-memory filtering if direct query fails
			for _, profile := range content.Profiles {
				if profile.SpanID != nil && *profile.SpanID == span.SpanID {
					correlated.Profiles = append(correlated.Profiles, profile)
				}
			}
		}
	} else {
		// Fallback to in-memory filtering if no database client
		// Find logs with matching span_id
		for _, log := range content.Logs {
			if log.SpanID != nil && *log.SpanID == span.SpanID {
				correlated.Logs = append(correlated.Logs, log)
			}
		}

		// Find exemplars with matching span_id (from metrics data)
		for _, metric := range content.Metrics {
			if metric.SpanID != nil && *metric.SpanID == span.SpanID {
				// Extract exemplars from metric data if available
				// This is a simplified fallback - in practice, exemplars are stored in JSONB
			}
		}

		// Find profiles with matching span_id
		for _, profile := range content.Profiles {
			if profile.SpanID != nil && *profile.SpanID == span.SpanID {
				correlated.Profiles = append(correlated.Profiles, profile)
			}
		}
	}

	// Extract logs from span events (always include for completeness)
	correlated.Logs = append(correlated.Logs, b.extractLogsFromSpanEvents(span)...)

	return correlated
}

// GetCorrelatedSignalsOptimized fetches only the data that the template actually needs
func (b *TraceContentBuilder) GetCorrelatedSignalsOptimized(ctx context.Context, content *CompleteTraceContent, span SpanData, templateAnalysis *TemplateAnalysis) *CorrelatedSignals {
	correlated := &CorrelatedSignals{
		SpanID:    span.SpanID,
		Span:      span,
		Logs:      make([]LogData, 0),
		Exemplars: make([]ExemplarData, 0),
		Profiles:  make([]ProfileData, 0),
	}

	// Only query database if we have a database client and template needs the data
	if b.db != nil {
		// Only fetch logs if template needs them
		if templateAnalysis.NeedsLogs {
			if logs, err := b.db.FetchLogsBySpanID(ctx, span.SpanID); err == nil {
				correlated.Logs = logs
			} else {
				// Fallback to in-memory filtering if direct query fails
				for _, log := range content.Logs {
					if log.SpanID != nil && *log.SpanID == span.SpanID {
						correlated.Logs = append(correlated.Logs, log)
					}
				}
			}
		}

		// Only fetch exemplars if template needs them and specific types are configured
		if templateAnalysis.NeedsExemplars && len(templateAnalysis.ExemplarTypes) > 0 {
			var allExemplars []ExemplarData

			// Fetch exemplars for each needed metric type
			for _, signalType := range templateAnalysis.ExemplarTypes {
				if exemplars, err := b.db.FetchExemplarsBySpanIDFromType(ctx, span.SpanID, signalType); err == nil {
					allExemplars = append(allExemplars, exemplars...)
				}
			}

			correlated.Exemplars = allExemplars
		}

		// Only fetch profiles if template needs them
		if templateAnalysis.NeedsProfiles {
			if profiles, err := b.db.FetchProfilesBySpanID(ctx, span.SpanID); err == nil {
				correlated.Profiles = profiles
			} else {
				// Fallback to in-memory filtering if direct query fails
				for _, profile := range content.Profiles {
					if profile.SpanID != nil && *profile.SpanID == span.SpanID {
						correlated.Profiles = append(correlated.Profiles, profile)
					}
				}
			}
		}
	} else {
		// Fallback to in-memory filtering if no database client
		// Only process data that template needs
		if templateAnalysis.NeedsLogs {
			for _, log := range content.Logs {
				if log.SpanID != nil && *log.SpanID == span.SpanID {
					correlated.Logs = append(correlated.Logs, log)
				}
			}
		}

		if templateAnalysis.NeedsExemplars {
			// Find exemplars with matching span_id (from metrics data)
			for _, metric := range content.Metrics {
				if metric.SpanID != nil && *metric.SpanID == span.SpanID {
					// Extract exemplars from metric data if available
					// This is a simplified fallback - in practice, exemplars are stored in JSONB
				}
			}
		}

		if templateAnalysis.NeedsProfiles {
			for _, profile := range content.Profiles {
				if profile.SpanID != nil && *profile.SpanID == span.SpanID {
					correlated.Profiles = append(correlated.Profiles, profile)
				}
			}
		}
	}

	// Extract logs from span events (regardless of template analysis - always needed for fallback)
	correlated.Logs = append(correlated.Logs, b.extractLogsFromSpanEvents(span)...)

	return correlated
}

// GetExemplarsByType fetches exemplars of a specific type for a span
func (b *TraceContentBuilder) GetExemplarsByType(ctx context.Context, spanID string, signalType string) ([]ExemplarData, error) {
	return b.db.FetchExemplarsBySpanIDFromType(ctx, spanID, signalType)
}

// GetExemplarsByTypes fetches exemplars of multiple types for a span
func (b *TraceContentBuilder) GetExemplarsByTypes(ctx context.Context, spanID string, signalTypes []string) ([]ExemplarData, error) {
	var allExemplars []ExemplarData
	for _, signalType := range signalTypes {
		if exemplars, err := b.GetExemplarsByType(ctx, spanID, signalType); err == nil {
			allExemplars = append(allExemplars, exemplars...)
		}
	}
	return allExemplars, nil
}

// CorrelatedSignals represents signals correlated with a specific span
type CorrelatedSignals struct {
	SpanID    string         `json:"span_id"`
	Span      SpanData       `json:"span"`
	Logs      []LogData      `json:"logs"`
	Exemplars []ExemplarData `json:"exemplars"`
	Profiles  []ProfileData  `json:"profiles"`
}

// summarizeByType creates a summary of items grouped by a specific field
func (b *TraceContentBuilder) summarizeByType(items interface{}, itemType string, getType func(interface{}) string) string {
	val := reflect.ValueOf(items)
	if val.Kind() != reflect.Slice || val.Len() == 0 {
		return fmt.Sprintf("0 %s", itemType)
	}

	// Count by type
	typeCount := make(map[string]int)
	for i := 0; i < val.Len(); i++ {
		itemType := getType(val.Index(i).Interface())
		typeCount[itemType]++
	}

	// Build summary with sorted keys for consistent output
	var keys []string
	for key := range typeCount {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var parts []string
	for _, key := range keys {
		count := typeCount[key]
		parts = append(parts, fmt.Sprintf("%d %s", count, key))
	}

	return fmt.Sprintf("%d %s (%s)", val.Len(), itemType, strings.Join(parts, ", "))
}

// SummarizeLogs creates a summary of logs
func (b *TraceContentBuilder) SummarizeLogs(logs []LogData) string {
	return b.summarizeByType(logs, "logs", func(item interface{}) string {
		log := item.(LogData)
		level := "INFO"
		if log.SeverityText != nil {
			level = strings.ToUpper(*log.SeverityText)
		}
		return level
	})
}

// SummarizeExemplars creates a summary of exemplars
func (b *TraceContentBuilder) SummarizeExemplars(exemplars []ExemplarData) string {
	return b.summarizeByType(exemplars, "exemplars", func(item interface{}) string {
		exemplar := item.(ExemplarData)
		return exemplar.MetricType
	})
}

// SummarizeProfiles creates a summary of profiles
func (b *TraceContentBuilder) SummarizeProfiles(profiles []ProfileData) string {
	return b.summarizeByType(profiles, "profiles", func(item interface{}) string {
		profile := item.(ProfileData)
		profileType := "unknown"
		if profile.ProfileType != nil {
			profileType = *profile.ProfileType
		}
		return profileType
	})
}

// BuildTextRepresentation creates a text representation of the complete trace content
func (b *TraceContentBuilder) BuildTextRepresentation(content *CompleteTraceContent) string {
	var builder strings.Builder

	// Trace header
	builder.WriteString(fmt.Sprintf("Trace: %s\n", content.TraceID))
	builder.WriteString(fmt.Sprintf("Service: %s\n", content.ServiceName))
	builder.WriteString(fmt.Sprintf("Duration: %s\n", time.Duration(content.Duration).String()))
	builder.WriteString(fmt.Sprintf("Spans: %d, Logs: %d, Metrics: %d, Profiles: %d\n\n",
		len(content.Spans), len(content.Logs), len(content.Metrics), len(content.Profiles)))

	// Spans
	builder.WriteString("SPANS:\n")
	for _, span := range content.Spans {
		builder.WriteString(fmt.Sprintf("  %s (%s) -> %s\n", span.Name, span.SpanID,
			func() string {
				if span.ParentSpanID != nil {
					return *span.ParentSpanID
				}
				return "ROOT"
			}()))
		builder.WriteString(fmt.Sprintf("    Kind: %s, Duration: %s, Status: %d\n",
			func() string {
				if span.SpanKind != nil {
					return *span.SpanKind
				}
				return "UNSPECIFIED"
			}(), formatDuration(span.Duration), span.StatusCode))

		if span.StatusMessage != nil {
			builder.WriteString(fmt.Sprintf("    Message: %s\n", *span.StatusMessage))
		}
	}

	// Logs summary
	if len(content.Logs) > 0 {
		builder.WriteString(fmt.Sprintf("\nLOGS: %s\n", b.SummarizeLogs(content.Logs)))
	}

	// Metrics summary (using exemplars instead)
	if len(content.Metrics) > 0 {
		// Convert metrics to exemplars for summary
		exemplars := make([]ExemplarData, 0, len(content.Metrics))
		for _, metric := range content.Metrics {
			exemplars = append(exemplars, ExemplarData{
				MetricName: metric.Name,
				Value:      metric.Value,
				MetricType: metric.MetricType,
			})
		}
		builder.WriteString(fmt.Sprintf("METRICS: %s\n", b.SummarizeExemplars(exemplars)))
	}

	// Profiles summary
	if len(content.Profiles) > 0 {
		builder.WriteString(fmt.Sprintf("PROFILES: %s\n", b.SummarizeProfiles(content.Profiles)))
	}

	return builder.String()
}

// TemplateAnalysis represents what data a template needs
type TemplateAnalysis struct {
	NeedsLogs      bool
	NeedsExemplars bool
	NeedsProfiles  bool
	NeedsEvents    bool
	ExemplarTypes  []string // Specific signal types needed
	RequiredFields []string // Fields that the template actually uses
}

// AnalyzeTemplate analyzes a template to determine what data it needs
func (b *TraceContentBuilder) AnalyzeTemplate(templateContent string) *TemplateAnalysis {
	analysis := &TemplateAnalysis{
		NeedsLogs:      false,
		NeedsExemplars: false,
		NeedsProfiles:  false,
		NeedsEvents:    false,
		ExemplarTypes:  make([]string, 0),
	}

	// Check for logs usage
	if strings.Contains(templateContent, "{{if .logs}}") ||
		strings.Contains(templateContent, "{{.logs") ||
		strings.Contains(templateContent, "{{.log_count") {
		analysis.NeedsLogs = true
	}

	// Check for exemplars usage
	if strings.Contains(templateContent, "{{if .exemplars") ||
		strings.Contains(templateContent, "{{.exemplars") ||
		strings.Contains(templateContent, "{{.exemplar_count") {
		analysis.NeedsExemplars = true

		// Determine specific exemplar types needed
		if strings.Contains(templateContent, "exemplars_histogram") ||
			strings.Contains(templateContent, "exemplar_count_histogram") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeHistogramMetrics)
		}
		if strings.Contains(templateContent, "exemplars_summary") ||
			strings.Contains(templateContent, "exemplar_count_summary") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeSummaryMetrics)
		}
		if strings.Contains(templateContent, "exemplars_gauge") ||
			strings.Contains(templateContent, "exemplar_count_gauge") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeGaugeMetrics)
		}
		if strings.Contains(templateContent, "exemplars_sum") ||
			strings.Contains(templateContent, "exemplar_count_sum") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeSumMetrics)
		}
		if strings.Contains(templateContent, "exemplars_exponential_histogram") ||
			strings.Contains(templateContent, "exemplar_count_exponential_histogram") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeExponentialHistogramMetrics)
		}
	}

	// Check for profiles usage
	if strings.Contains(templateContent, "{{if .profiles}}") ||
		strings.Contains(templateContent, "{{.profiles") ||
		strings.Contains(templateContent, "{{.profile_count") {
		analysis.NeedsProfiles = true
	}

	// Check for events usage
	if strings.Contains(templateContent, "{{if .events}}") ||
		strings.Contains(templateContent, "{{.events") ||
		strings.Contains(templateContent, "{{.event_count") {
		analysis.NeedsEvents = true
	}

	return analysis
}

// extractLogsFromSpanEvents extracts logs from span events
func (b *TraceContentBuilder) extractLogsFromSpanEvents(span SpanData) []LogData {
	var logs []LogData

	for _, event := range span.Events {
		// Check if this is a log event
		if eventName, ok := event["name"].(string); ok {
			if eventName == "log.generated" || strings.HasPrefix(eventName, "log.") {
				// Extract log data from event attributes
				log := LogData{
					SpanID: &span.SpanID,
				}

				// Extract attributes
				if attrs, ok := event["attributes"].(map[string]interface{}); ok {
					// Extract severity
					if severity, ok := attrs["log.severity"].(string); ok {
						log.SeverityText = &severity
					}

					// Extract message
					if message, ok := attrs["log.message"].(string); ok {
						log.Body = &message
					}

					// Extract timestamp
					if timestamp, ok := attrs["log.timestamp"].(string); ok {
						if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
							log.Timestamp = t
						}
					}

					// Store all attributes
					log.Attributes = attrs
				}

				// Extract timestamp from event if not found in attributes
				if log.Timestamp.IsZero() {
					if timestamp, ok := event["timestamp"].(string); ok {
						if t, err := time.Parse(time.RFC3339, timestamp); err == nil {
							log.Timestamp = t
						}
					}
				}

				logs = append(logs, log)
			}
		}
	}

	return logs
}
