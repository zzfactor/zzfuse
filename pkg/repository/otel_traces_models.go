package repository

import (
	"strconv"
	"time"
)

// TraceQuery holds parameters for querying spans.
type TraceQuery struct {
	TimeRange
	CommonBaseFilters
	CommonAdvancedFilters

	// Trace-specific filters
	TraceID string
}

// TraceEntry represents a trace record in the database.
type TraceEntry struct {
	ID                    int64     // id BIGSERIAL
	TimestampNS           int64     // timestamp_ns BIGINT
	Timestamp             time.Time // timestamp TIMESTAMP (generated)
	TraceID               string    // trace_id TEXT
	SpanID                string    // span_id TEXT
	ParentSpanID          *string   // parent_span_id TEXT (nullable)
	TraceState            *string   // trace_state TEXT (nullable)
	Flags                 *int32    // flags INTEGER (nullable) - OTel span-level flags
	Name                  string    // name TEXT
	SpanKind              string    // span_kind TEXT
	// Service entity fields (OTel v1.39.0)
	ServiceName           string    // service_name TEXT
	ServiceNamespace      *string   // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID     *string   // service_instance_id TEXT (nullable) - OTel v1.39.0
	ResourceAttributes    []byte    // resource_attributes JSONB
	ScopeName             *string   // scope_name TEXT (nullable)
	ScopeVersion          *string   // scope_version TEXT (nullable)
	ScopeAttributes       []byte    // scope_attributes JSONB
	ScopeDroppedAttrCount *int32    // scope_dropped_attr_count INTEGER (nullable)
	Attributes            []byte    // attributes JSONB
	DroppedAttrCount      *int32    // dropped_attr_count INTEGER (nullable)
	Events                []byte    // events JSONB
	DroppedEventsCount    *int32    // dropped_events_count INTEGER (nullable)
	Links                 []byte    // links JSONB
	DroppedLinksCount     *int32    // dropped_links_count INTEGER (nullable)
	StatusCode            *int32    // status_code INTEGER (nullable)
	StatusMessage         *string   // status_message TEXT (nullable)
	StartTime             time.Time // start_time TIMESTAMP
	EndTime               time.Time // end_time TIMESTAMP
	DurationNS            int64     // duration_ns BIGINT
	Metadata              []byte    // metadata JSONB

	ResourceSchemaURL *string // resource_schema_url TEXT (nullable)
	ScopeSchemaURL    *string // scope_schema_url TEXT (nullable)
}

// Implement CursorableEntry interface for TraceEntry
func (e TraceEntry) GetTimestampNS() int64 {
	return e.TimestampNS
}

func (e TraceEntry) GetIDString() string {
	return strconv.FormatInt(e.ID, 10)
}
