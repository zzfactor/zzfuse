package repository

import (
	"time"
)

// ProfileQuery holds parameters for querying profiles.
type ProfileQuery struct {
	TimeRange
	CommonBaseFilters
	CommonAdvancedFilters

	// Profile-specific filters
	ProfileType string
}

// ProfileEntry represents a profile record in the database.
type ProfileEntry struct {
	ID          int64     // id BIGSERIAL
	TimestampNS int64     // timestamp_ns BIGINT
	Timestamp   time.Time // timestamp TIMESTAMP (generated)
	// Service entity fields (OTel v1.39.0)
	ServiceName           string  // service_name TEXT
	ServiceNamespace      *string // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID     *string // service_instance_id TEXT (nullable) - OTel v1.39.0
	ProfileType           string  // profile_type TEXT
	DurationNS            int64   // duration_ns BIGINT
	SampleType            *string // sample_type TEXT (nullable)
	SampleUnit            *string // sample_unit TEXT (nullable)
	ProfileBytes          []byte  // profile_bytes BYTEA
	ProfileStorageVersion int16   // 0: unversioned legacy, 1: current OTLP development encoding
	ProfileEncoding       string
	ProfileSchemaVersion  *string
	OtelProfileID         *string // source identity, distinct from the database row ID
	ProfileTimeUnixNano   *string // exact uint64 decimal; timestamp_ns is the signed query index
	DurationNano          *string // exact uint64 decimal; duration remains a legacy signed summary
	Attributes            []byte  // attributes JSONB
	ResourceAttributes    []byte  // resource_attributes JSONB
	ResourceSchemaURL     *string // resource_schema_url TEXT (nullable)
	ScopeName             *string // scope_name TEXT (nullable)
	ScopeVersion          *string // scope_version TEXT (nullable)
	ScopeAttributes       []byte  // scope_attributes JSONB
	ScopeDroppedAttrCount *int32  // scope_dropped_attr_count INTEGER (nullable)
	ScopeSchemaURL        *string // scope_schema_url TEXT (nullable)
	Metadata              []byte  // metadata JSONB
}
