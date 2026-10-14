package repository

import (
	"time"
)

// =========================
// Trace Data Types
// =========================

// SpanData represents a span with all its attributes and metadata
type SpanData struct {
	ID                 string                   `json:"id"`
	TraceID            string                   `json:"trace_id"`
	SpanID             string                   `json:"span_id"`
	ParentSpanID       *string                  `json:"parent_span_id,omitempty"`
	TraceState         *string                  `json:"trace_state,omitempty"`
	Name               string                   `json:"name"`
	SpanKind           *string                  `json:"span_kind,omitempty"`
	ServiceName        *string                  `json:"service_name,omitempty"`
	ResourceAttributes map[string]interface{}   `json:"resource_attributes"`
	ScopeName          *string                  `json:"scope_name,omitempty"`
	ScopeVersion       *string                  `json:"scope_version,omitempty"`
	ScopeAttributes    map[string]interface{}   `json:"scope_attributes"`
	Attributes         map[string]interface{}   `json:"attributes"`
	Events             []map[string]interface{} `json:"events"`
	Links              []map[string]interface{} `json:"links"`
	StatusCode         int                      `json:"status_code"`
	StatusMessage      *string                  `json:"status_message,omitempty"`
	StartTime          time.Time                `json:"start_time"`
	EndTime            time.Time                `json:"end_time"`
	Duration           int64                    `json:"duration"`
	Metadata           map[string]interface{}   `json:"metadata"`
}

// LogData represents a log entry with all its attributes and metadata
type LogData struct {
	ID                 string                 `json:"id"`
	TraceID            *string                `json:"trace_id,omitempty"`
	SpanID             *string                `json:"span_id,omitempty"`
	Timestamp          time.Time              `json:"timestamp"`
	ObservedTimestamp  *time.Time             `json:"observed_timestamp,omitempty"`
	SeverityText       *string                `json:"severity_text,omitempty"`
	SeverityNumber     *int                   `json:"severity_number,omitempty"`
	ServiceName        *string                `json:"service_name,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resource_attributes"`
	ScopeName          *string                `json:"scope_name,omitempty"`
	ScopeVersion       *string                `json:"scope_version,omitempty"`
	ScopeAttributes    map[string]interface{} `json:"scope_attributes"`
	Attributes         map[string]interface{} `json:"attributes"`
	Body               interface{}            `json:"body"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// MetricData represents a metric with all its attributes and metadata
type MetricData struct {
	ID                 string                 `json:"id"`
	TraceID            *string                `json:"trace_id,omitempty"`
	SpanID             *string                `json:"span_id,omitempty"`
	Timestamp          time.Time              `json:"timestamp"`
	Name               string                 `json:"name"`
	Description        *string                `json:"description,omitempty"`
	Unit               *string                `json:"unit,omitempty"`
	ServiceName        *string                `json:"service_name,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resource_attributes"`
	ScopeName          *string                `json:"scope_name,omitempty"`
	ScopeVersion       *string                `json:"scope_version,omitempty"`
	ScopeAttributes    map[string]interface{} `json:"scope_attributes"`
	Attributes         map[string]interface{} `json:"attributes"`
	Value              interface{}            `json:"value"`
	Metadata           map[string]interface{} `json:"metadata"`
	MetricType         string                 `json:"metric_type"` // gauge, sum, histogram, etc.
}

// ProfileData represents a profile with all its attributes and metadata
type ProfileData struct {
	ID                 string                 `json:"id"`
	TraceID            *string                `json:"trace_id,omitempty"`
	SpanID             *string                `json:"span_id,omitempty"`
	Timestamp          time.Time              `json:"timestamp"`
	ProfileType        *string                `json:"profile_type,omitempty"`
	Duration           *int64                 `json:"duration,omitempty"`
	SampleType         *string                `json:"sample_type,omitempty"`
	SampleUnit         *string                `json:"sample_unit,omitempty"`
	ProfileBytes       []byte                 `json:"profile_bytes,omitempty"`
	ServiceName        *string                `json:"service_name,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resource_attributes"`
	ScopeName          *string                `json:"scope_name,omitempty"`
	ScopeVersion       *string                `json:"scope_version,omitempty"`
	ScopeAttributes    map[string]interface{} `json:"scope_attributes"`
	Attributes         map[string]interface{} `json:"attributes"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// ExemplarData represents an exemplar from metrics with trace correlation
type ExemplarData struct {
	ID                 string                 `json:"id"`
	TraceID            *string                `json:"trace_id,omitempty"`
	SpanID             *string                `json:"span_id,omitempty"`
	Timestamp          time.Time              `json:"timestamp"`
	MetricName         string                 `json:"metric_name"`
	MetricType         string                 `json:"metric_type"`
	Value              interface{}            `json:"value"`
	FilteredAttributes map[string]interface{} `json:"filtered_attributes"`
	ServiceName        *string                `json:"service_name,omitempty"`
	ResourceAttributes map[string]interface{} `json:"resource_attributes"`
	ScopeName          *string                `json:"scope_name,omitempty"`
	ScopeVersion       *string                `json:"scope_version,omitempty"`
	ScopeAttributes    map[string]interface{} `json:"scope_attributes"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// CompleteTraceContent represents a complete trace with all its signals
type CompleteTraceContent struct {
	TraceID     string                 `json:"trace_id"`
	ServiceName string                 `json:"service_name"`
	StartTime   time.Time              `json:"start_time"`
	EndTime     time.Time              `json:"end_time"`
	Duration    int64                  `json:"duration"`
	Spans       []SpanData             `json:"spans"`
	Logs        []LogData              `json:"logs"`
	Metrics     []MetricData           `json:"metrics"`
	Profiles    []ProfileData          `json:"profiles"`
	Metadata    map[string]interface{} `json:"metadata"`
}
