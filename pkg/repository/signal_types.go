package repository

import (
	"fmt"
	"time"
)

// =========================
// Unified Signal Types (DRY Implementation)
// =========================

// SignalType represents the type of signal
type SignalType string

const (
	// Logs
	SignalTypeLogs = "logs"

	// Spans
	SignalTypeSpans = "spans"

	// Metrics - individual metric types
	SignalTypeGaugeMetrics                = "gauge_metrics"
	SignalTypeSumMetrics                  = "sum_metrics"
	SignalTypeHistogramMetrics            = "histogram_metrics"
	SignalTypeSummaryMetrics              = "summary_metrics"
	SignalTypeExponentialHistogramMetrics = "exponential_histogram_metrics"

	// Metrics - aggregated constants for convenience
	SignalTypeMetrics = "metrics" // Generic metrics (not used in table names, but useful for filtering)

	// Profiles
	SignalTypeProfiles = "profiles"

	// Wildcard for accepting all signal types
	SignalTypeAll = "*"
)

// BaseSignal represents common fields for all signal types
type BaseSignal struct {
	ID                 string                 `json:"id"`
	TraceID            *string                `json:"trace_id,omitempty"`
	SpanID             *string                `json:"span_id,omitempty"`
	Timestamp          time.Time              `json:"timestamp"`
	ServiceName        *string                `json:"service_name,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resource_attributes"`
	ScopeName          *string                `json:"scope_name,omitempty"`
	ScopeVersion       *string                `json:"scope_version,omitempty"`
	ScopeAttributes    map[string]interface{} `json:"scope_attributes"`
	Attributes         map[string]interface{} `json:"attributes"`
	Metadata           map[string]interface{} `json:"metadata"`
	SignalType         SignalType             `json:"signal_type"`
}

// SpanSignal represents a span with all its attributes
type SpanSignal struct {
	BaseSignal
	ParentSpanID  *string                  `json:"parent_span_id,omitempty"`
	TraceState    *string                  `json:"trace_state,omitempty"`
	Name          string                   `json:"name"`
	SpanKind      *string                  `json:"span_kind,omitempty"`
	Events        []map[string]interface{} `json:"events"`
	Links         []map[string]interface{} `json:"links"`
	StatusCode    int                      `json:"status_code"`
	StatusMessage *string                  `json:"status_message,omitempty"`
	EndTime       time.Time                `json:"end_time"`
	Duration      int64                    `json:"duration"`
}

// LogSignal represents a log entry
type LogSignal struct {
	BaseSignal
	ObservedTimestamp *time.Time  `json:"observed_timestamp,omitempty"`
	SeverityText      *string     `json:"severity_text,omitempty"`
	SeverityNumber    *int        `json:"severity_number,omitempty"`
	Body              interface{} `json:"body"`
}

// MetricSignal represents a metric
type MetricSignal struct {
	BaseSignal
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Unit        *string     `json:"unit,omitempty"`
	Value       interface{} `json:"value"`
	MetricType  string      `json:"metric_type"` // gauge, sum, histogram, etc.
}

// ProfileSignal represents a profile
type ProfileSignal struct {
	BaseSignal
	ProfileType  *string `json:"profile_type,omitempty"`
	Duration     *int64  `json:"duration,omitempty"`
	SampleType   *string `json:"sample_type,omitempty"`
	SampleUnit   *string `json:"sample_unit,omitempty"`
	ProfileBytes []byte  `json:"profile_bytes,omitempty"`
}

// SignalData represents a unified signal structure for backward compatibility
type SignalData struct {
	ID         string                 `json:"id"`
	TraceID    string                 `json:"trace_id"`
	SpanID     *string                `json:"span_id,omitempty"`
	SignalType string                 `json:"signal_type"`
	Content    string                 `json:"content"`
	Metadata   map[string]interface{} `json:"metadata"`
	Timestamp  int64                  `json:"timestamp"`
}

// Note: CompleteTraceContent and ProcessedContent are defined in other files
// to avoid circular dependencies

// =========================
// Utility Functions (Backward Compatibility)
// =========================

// AllSignalTypes returns a slice of all available signal types
func AllSignalTypes() []string {
	return []string{
		SignalTypeLogs,
		SignalTypeSpans,
		SignalTypeGaugeMetrics,
		SignalTypeSumMetrics,
		SignalTypeHistogramMetrics,
		SignalTypeSummaryMetrics,
		SignalTypeExponentialHistogramMetrics,
		SignalTypeProfiles,
	}
}

// AllMetricTypes returns a slice of all metric signal types
func AllMetricTypes() []string {
	return []string{
		SignalTypeGaugeMetrics,
		SignalTypeSumMetrics,
		SignalTypeHistogramMetrics,
		SignalTypeSummaryMetrics,
		SignalTypeExponentialHistogramMetrics,
	}
}

// IsValidSignalType checks if the given signal type is valid
func IsValidSignalType(signalType string) bool {
	validTypes := AllSignalTypes()
	for _, validType := range validTypes {
		if validType == signalType {
			return true
		}
	}
	return signalType == SignalTypeAll
}

// IsMetricType checks if the given signal type is a metric type
func IsMetricType(signalType string) bool {
	metricTypes := AllMetricTypes()
	for _, metricType := range metricTypes {
		if metricType == signalType {
			return true
		}
	}
	return false
}

// GetTableNameForSignalType returns the database table name for a given signal type
func GetTableNameForSignalType(signalType string) string {
	switch signalType {
	case SignalTypeLogs:
		return "logs"
	case SignalTypeSpans:
		return "spans"
	case SignalTypeGaugeMetrics:
		return "gauge_metrics"
	case SignalTypeSumMetrics:
		return "sum_metrics"
	case SignalTypeHistogramMetrics:
		return "histogram_metrics"
	case SignalTypeSummaryMetrics:
		return "summary_metrics"
	case SignalTypeExponentialHistogramMetrics:
		return "exponential_histogram_metrics"
	case SignalTypeProfiles:
		return "profiles"
	default:
		return ""
	}
}

// =========================
// Conversion Helpers (DRY Implementation)
// =========================

// ToSignalData converts any signal to the legacy SignalData format
func (s *SpanSignal) ToSignalData() SignalData {
	return SignalData{
		ID:         s.ID,
		TraceID:    *s.TraceID,
		SpanID:     s.SpanID,
		SignalType: string(s.SignalType),
		Content:    s.Name, // Basic content representation
		Metadata:   s.Metadata,
		Timestamp:  s.Timestamp.UnixNano(),
	}
}

func (s *LogSignal) ToSignalData() SignalData {
	return SignalData{
		ID:         s.ID,
		TraceID:    *s.TraceID,
		SpanID:     s.SpanID,
		SignalType: string(s.SignalType),
		Content:    fmt.Sprintf("%v", s.Body),
		Metadata:   s.Metadata,
		Timestamp:  s.Timestamp.UnixNano(),
	}
}

func (s *MetricSignal) ToSignalData() SignalData {
	return SignalData{
		ID:         s.ID,
		TraceID:    *s.TraceID,
		SpanID:     s.SpanID,
		SignalType: string(s.SignalType),
		Content:    fmt.Sprintf("%s: %v", s.Name, s.Value),
		Metadata:   s.Metadata,
		Timestamp:  s.Timestamp.UnixNano(),
	}
}

func (s *ProfileSignal) ToSignalData() SignalData {
	return SignalData{
		ID:         s.ID,
		TraceID:    *s.TraceID,
		SpanID:     s.SpanID,
		SignalType: string(s.SignalType),
		Content:    fmt.Sprintf("Profile: %s (%d bytes)", *s.ProfileType, len(s.ProfileBytes)),
		Metadata:   s.Metadata,
		Timestamp:  s.Timestamp.UnixNano(),
	}
}

// ToMap converts any signal to map[string]interface{} for embedding worker compatibility
func (s *SpanSignal) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":                  s.ID,
		"trace_id":            s.TraceID,
		"span_id":             s.SpanID,
		"parent_span_id":      s.ParentSpanID,
		"trace_state":         s.TraceState,
		"name":                s.Name,
		"span_kind":           s.SpanKind,
		"service_name":        s.ServiceName,
		"resource_attributes": s.ResourceAttributes,
		"scope_name":          s.ScopeName,
		"scope_version":       s.ScopeVersion,
		"scope_attributes":    s.ScopeAttributes,
		"attributes":          s.Attributes,
		"events":              s.Events,
		"links":               s.Links,
		"status_code":         s.StatusCode,
		"status_message":      s.StatusMessage,
		"timestamp":           s.Timestamp,
		"end_time":            s.EndTime,
		"duration":            s.Duration,
		"metadata":            s.Metadata,
		"signal_type":         string(s.SignalType),
	}
}

func (s *LogSignal) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":                  s.ID,
		"trace_id":            s.TraceID,
		"span_id":             s.SpanID,
		"observed_timestamp":  s.ObservedTimestamp,
		"severity_text":       s.SeverityText,
		"severity_number":     s.SeverityNumber,
		"service_name":        s.ServiceName,
		"resource_attributes": s.ResourceAttributes,
		"scope_name":          s.ScopeName,
		"scope_version":       s.ScopeVersion,
		"scope_attributes":    s.ScopeAttributes,
		"attributes":          s.Attributes,
		"body":                s.Body,
		"metadata":            s.Metadata,
		"timestamp":           s.Timestamp,
		"signal_type":         string(s.SignalType),
	}
}

func (s *MetricSignal) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":                  s.ID,
		"trace_id":            s.TraceID,
		"span_id":             s.SpanID,
		"name":                s.Name,
		"description":         s.Description,
		"unit":                s.Unit,
		"service_name":        s.ServiceName,
		"resource_attributes": s.ResourceAttributes,
		"scope_name":          s.ScopeName,
		"scope_version":       s.ScopeVersion,
		"scope_attributes":    s.ScopeAttributes,
		"attributes":          s.Attributes,
		"value":               s.Value,
		"metadata":            s.Metadata,
		"timestamp":           s.Timestamp,
		"signal_type":         string(s.SignalType),
		"metric_type":         s.MetricType,
	}
}

func (s *ProfileSignal) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":                  s.ID,
		"trace_id":            s.TraceID,
		"span_id":             s.SpanID,
		"profile_type":        s.ProfileType,
		"duration":            s.Duration,
		"sample_type":         s.SampleType,
		"sample_unit":         s.SampleUnit,
		"profile_bytes":       s.ProfileBytes,
		"service_name":        s.ServiceName,
		"resource_attributes": s.ResourceAttributes,
		"scope_name":          s.ScopeName,
		"scope_version":       s.ScopeVersion,
		"scope_attributes":    s.ScopeAttributes,
		"attributes":          s.Attributes,
		"metadata":            s.Metadata,
		"timestamp":           s.Timestamp,
		"signal_type":         string(s.SignalType),
	}
}
