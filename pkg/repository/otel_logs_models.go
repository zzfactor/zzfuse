package repository

import (
	"strconv"
	"time"
)

// LogQuery holds parameters for querying logs.
type LogQuery struct {
	TimeRange
	CommonBaseFilters
	CommonAdvancedFilters

	// Log-specific filters
	Severity       string   // Backward-compatible single severity
	SeverityMin    *int     // severity_min
	SeverityMax    *int     // severity_max
	SeverityGroups []string // severity_group_in[]
	SeverityTextIn []string // severity_text_in[]
}

// LogEntry represents a log record in the database.
type LogEntry struct {
	ID                    int64      // id BIGSERIAL
	TimestampNS           int64      // timestamp_ns BIGINT
	Timestamp             time.Time  // timestamp TIMESTAMP (generated)
	ObservedTimestamp     *time.Time // observed_timestamp TIMESTAMP (nullable)
	TraceID               *string    // trace_id TEXT (nullable)
	SpanID                *string    // span_id TEXT (nullable)
	Flags                 *int32     // flags INTEGER (nullable)
	SeverityText          *string    // severity_text TEXT (nullable)
	SeverityNumber        *int32     // severity_number INTEGER (nullable)
	// Service entity fields (OTel v1.39.0)
	ServiceName           string     // service_name TEXT
	ServiceNamespace      *string    // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID     *string    // service_instance_id TEXT (nullable) - OTel v1.39.0
	EventName             *string    // event_name TEXT (nullable) - OTel v1.39.0 otel.event.name
	ResourceAttributes    []byte     // resource_attributes JSONB
	ResourceSchemaURL     *string    // resource_schema_url TEXT (nullable)
	ScopeName             *string    // scope_name TEXT (nullable)
	ScopeVersion          *string    // scope_version TEXT (nullable)
	ScopeAttributes       []byte     // scope_attributes JSONB
	ScopeDroppedAttrCount *int32     // scope_dropped_attr_count INTEGER (nullable)
	ScopeSchemaURL        *string    // scope_schema_url TEXT (nullable)
	Attributes            []byte     // attributes JSONB
	DroppedAttrCount      *int32     // dropped_attr_count INTEGER (nullable)
	Body                  []byte     // body JSONB
	Metadata              []byte     // metadata JSONB
}

// SetMetadata sets the metadata field
func (l *LogEntry) SetMetadata(metadata []byte) {
	l.Metadata = metadata
}

// GetMetadata returns the metadata field
func (l *LogEntry) GetMetadata() []byte {
	return l.Metadata
}

// Implement CursorableEntry interface for LogEntry
func (e LogEntry) GetTimestampNS() int64 {
	return e.TimestampNS
}

func (e LogEntry) GetIDString() string {
	return strconv.FormatInt(e.ID, 10)
}

// NormalizeLogEntryJSONBFields ensures all JSONB fields are non-empty.
func NormalizeLogEntryJSONBFields(e *LogEntry) {
	if len(e.Body) == 0 {
		e.Body = []byte("{}")
	}
	if len(e.ResourceAttributes) == 0 {
		e.ResourceAttributes = []byte("{}")
	}
	if len(e.ScopeAttributes) == 0 {
		e.ScopeAttributes = []byte("{}")
	}
	if len(e.Attributes) == 0 {
		e.Attributes = []byte("{}")
	}
	if len(e.Metadata) == 0 {
		e.Metadata = []byte("{}")
	}
}
