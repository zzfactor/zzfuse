package repository

import (
	"strconv"
	"time"
)

// MetricQuery holds parameters for querying metrics.
type MetricQuery struct {
	TimeRange
	CommonBaseFilters
	CommonAdvancedFilters

	// Metric-specific filters
	MetricName string
}

// GaugeMetricEntry matches the gauge_metrics table
// All fields are pointers if nullable in DB

type GaugeMetricEntry struct {
	ID                    int64     // id BIGSERIAL
	TimestampNS           int64     // timestamp_ns BIGINT
	Timestamp             time.Time // timestamp TIMESTAMP (generated)
	// Service entity fields (OTel v1.39.0)
	ServiceName           string    // service_name TEXT
	ServiceNamespace      *string   // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID     *string   // service_instance_id TEXT (nullable) - OTel v1.39.0
	Name                  *string   // name TEXT (changed from string to *string)
	Description           *string   // description TEXT
	Unit                  *string   // unit TEXT
	Attributes            []byte    // attributes JSONB
	Value                 *float64  // value DOUBLE PRECISION
	Flags                 *int32    // flags INTEGER
	Exemplars             []byte    // exemplars JSONB
	ResourceAttributes    []byte    // resource_attributes JSONB
	ScopeName             *string   // scope_name TEXT
	ScopeVersion          *string   // scope_version TEXT
	ScopeAttributes       []byte    // scope_attributes JSONB
	ScopeDroppedAttrCount *int32    // scope_dropped_attr_count INTEGER
	Metadata              []byte    // metadata JSONB
	ResourceSchemaURL     *string   // resource_schema_url TEXT
	ScopeSchemaURL        *string   // scope_schema_url TEXT
}

// SetMetadata sets the metadata field
func (g *GaugeMetricEntry) SetMetadata(metadata []byte) {
	g.Metadata = metadata
}

// GetMetadata returns the metadata field
func (g *GaugeMetricEntry) GetMetadata() []byte {
	return g.Metadata
}

// SumMetricEntry matches the sum_metrics table

type SumMetricEntry struct {
	ID                     int64      // id BIGSERIAL
	TimestampNS            int64      // timestamp_ns BIGINT
	Timestamp              time.Time  // timestamp TIMESTAMP (generated)
	// Service entity fields (OTel v1.39.0)
	ServiceName            string     // service_name TEXT
	ServiceNamespace       *string    // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID      *string    // service_instance_id TEXT (nullable) - OTel v1.39.0
	ResourceAttributes     []byte     // resource_attributes JSONB
	ScopeName              *string    // scope_name TEXT
	ScopeVersion           *string    // scope_version TEXT
	ScopeAttributes        []byte     // scope_attributes JSONB
	ScopeDroppedAttrCount  *int32     // scope_dropped_attr_count INTEGER
	Name                   *string    // name TEXT
	Description            *string    // description TEXT
	Unit                   *string    // unit TEXT
	Attributes             []byte     // attributes JSONB
	StartTime              *time.Time // start_time TIMESTAMP
	Value                  *float64   // value DOUBLE PRECISION
	Flags                  *int32     // flags INTEGER
	Exemplars              []byte     // exemplars JSONB
	AggregationTemporality *int32     // aggregation_temporality INTEGER
	IsMonotonic            *bool      // is_monotonic BOOLEAN
	Metadata               []byte     // metadata JSONB
	ResourceSchemaURL      *string    // resource_schema_url TEXT
	ScopeSchemaURL         *string    // scope_schema_url TEXT
}

// SetMetadata sets the metadata field
func (s *SumMetricEntry) SetMetadata(metadata []byte) {
	s.Metadata = metadata
}

// GetMetadata returns the metadata field
func (s *SumMetricEntry) GetMetadata() []byte {
	return s.Metadata
}

// HistogramMetricEntry matches the histogram_metrics table

type HistogramMetricEntry struct {
	ID                     int64      // id BIGSERIAL
	TimestampNS            int64      // timestamp_ns BIGINT
	Timestamp              time.Time  // timestamp TIMESTAMP (generated)
	// Service entity fields (OTel v1.39.0)
	ServiceName            string     // service_name TEXT
	ServiceNamespace       *string    // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID      *string    // service_instance_id TEXT (nullable) - OTel v1.39.0
	Name                   *string    // name TEXT
	Description            *string    // description TEXT
	Unit                   *string    // unit TEXT
	Attributes             []byte     // attributes JSONB
	StartTime              *time.Time // start_time TIMESTAMP
	Count                  *int64     // count BIGINT
	Sum                    *float64   // sum DOUBLE PRECISION
	Min                    *float64   // min DOUBLE PRECISION (OTel optional)
	Max                    *float64   // max DOUBLE PRECISION (OTel optional)
	Buckets                []byte     // buckets JSONB
	Flags                  *int32     // flags INTEGER
	AggregationTemporality *int32     // aggregation_temporality INTEGER
	Exemplars              []byte     // exemplars JSONB
	ResourceAttributes     []byte     // resource_attributes JSONB
	ScopeName              *string    // scope_name TEXT
	ScopeVersion           *string    // scope_version TEXT
	ScopeAttributes        []byte     // scope_attributes JSONB
	ScopeDroppedAttrCount  *int32     // scope_dropped_attr_count INTEGER
	Metadata               []byte     // metadata JSONB
	ResourceSchemaURL      *string    // resource_schema_url TEXT
	ScopeSchemaURL         *string    // scope_schema_url TEXT
}

// SetMetadata sets the metadata field
func (h *HistogramMetricEntry) SetMetadata(metadata []byte) {
	h.Metadata = metadata
}

// GetMetadata returns the metadata field
func (h *HistogramMetricEntry) GetMetadata() []byte {
	return h.Metadata
}

// SummaryMetricEntry matches the summary_metrics table

type SummaryMetricEntry struct {
	ID                    int64      // id BIGSERIAL
	TimestampNS           int64      // timestamp_ns BIGINT
	Timestamp             time.Time  // timestamp TIMESTAMP (generated)
	// Service entity fields (OTel v1.39.0)
	ServiceName           string     // service_name TEXT
	ServiceNamespace      *string    // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID     *string    // service_instance_id TEXT (nullable) - OTel v1.39.0
	Name                  *string    // name TEXT
	Description           *string    // description TEXT
	Unit                  *string    // unit TEXT
	Attributes            []byte     // attributes JSONB
	StartTime             *time.Time // start_time TIMESTAMP
	Count                 *int64     // count BIGINT
	Sum                   *float64   // sum DOUBLE PRECISION
	Quantiles             []byte     // quantiles JSONB
	Flags                 *int32     // flags INTEGER
	Exemplars             []byte     // exemplars JSONB
	ResourceAttributes    []byte     // resource_attributes JSONB
	ScopeName             *string    // scope_name TEXT
	ScopeVersion          *string    // scope_version TEXT
	ScopeAttributes       []byte     // scope_attributes JSONB
	ScopeDroppedAttrCount *int32     // scope_dropped_attr_count INTEGER
	Metadata              []byte     // metadata JSONB
	ResourceSchemaURL     *string    // resource_schema_url TEXT
	ScopeSchemaURL        *string    // scope_schema_url TEXT
}

// SetMetadata sets the metadata field
func (s *SummaryMetricEntry) SetMetadata(metadata []byte) {
	s.Metadata = metadata
}

// GetMetadata returns the metadata field
func (s *SummaryMetricEntry) GetMetadata() []byte {
	return s.Metadata
}

// ExponentialHistogramMetricEntry matches the exponential_histogram_metrics table

type ExponentialHistogramMetricEntry struct {
	ID                     int64      // id BIGSERIAL
	TimestampNS            int64      // timestamp_ns BIGINT
	Timestamp              time.Time  // timestamp TIMESTAMP (generated)
	// Service entity fields (OTel v1.39.0)
	ServiceName            string     // service_name TEXT
	ServiceNamespace       *string    // service_namespace TEXT (nullable) - OTel v1.39.0
	ServiceInstanceID      *string    // service_instance_id TEXT (nullable) - OTel v1.39.0
	ResourceAttributes     []byte     // resource_attributes JSONB
	ScopeName              *string    // scope_name TEXT
	ScopeVersion           *string    // scope_version TEXT
	ScopeAttributes        []byte     // scope_attributes JSONB
	ScopeDroppedAttrCount  *int32     // scope_dropped_attr_count INTEGER
	Name                   *string    // name TEXT
	Description            *string    // description TEXT
	Unit                   *string    // unit TEXT
	Attributes             []byte     // attributes JSONB
	StartTime              *time.Time // start_time TIMESTAMP
	Count                  *int64     // count BIGINT
	Sum                    *float64   // sum DOUBLE PRECISION
	Scale                  *int32     // scale INTEGER
	ZeroCount              *int64     // zero_count BIGINT
	ZeroThreshold          *float64   // zero_threshold DOUBLE PRECISION (OTel zero bucket boundary)
	PositiveOffset         *int32     // positive_offset INTEGER
	PositiveBucketCounts   []byte     // positive_bucket_counts JSONB
	NegativeOffset         *int32     // negative_offset INTEGER
	NegativeBucketCounts   []byte     // negative_bucket_counts JSONB
	Exemplars              []byte     // exemplars JSONB
	Flags                  *int32     // flags INTEGER
	Min                    *float64   // min DOUBLE PRECISION
	Max                    *float64   // max DOUBLE PRECISION
	AggregationTemporality *int32     // aggregation_temporality INTEGER
	Metadata               []byte     // metadata JSONB
	ResourceSchemaURL      *string    // resource_schema_url TEXT
	ScopeSchemaURL         *string    // scope_schema_url TEXT
}

// SetMetadata sets the metadata field
func (e *ExponentialHistogramMetricEntry) SetMetadata(metadata []byte) {
	e.Metadata = metadata
}

// GetMetadata returns the metadata field
func (e *ExponentialHistogramMetricEntry) GetMetadata() []byte {
	return e.Metadata
}

// Implement CursorableEntry interface for all metric types
func (g GaugeMetricEntry) GetTimestampNS() int64 {
	return g.TimestampNS
}

func (g GaugeMetricEntry) GetIDString() string {
	return strconv.FormatInt(g.ID, 10)
}

func (s SumMetricEntry) GetTimestampNS() int64 {
	return s.TimestampNS
}

func (s SumMetricEntry) GetIDString() string {
	return strconv.FormatInt(s.ID, 10)
}

func (h HistogramMetricEntry) GetTimestampNS() int64 {
	return h.TimestampNS
}

func (h HistogramMetricEntry) GetIDString() string {
	return strconv.FormatInt(h.ID, 10)
}

func (s SummaryMetricEntry) GetTimestampNS() int64 {
	return s.TimestampNS
}

func (s SummaryMetricEntry) GetIDString() string {
	return strconv.FormatInt(s.ID, 10)
}

func (e ExponentialHistogramMetricEntry) GetTimestampNS() int64 {
	return e.TimestampNS
}

func (e ExponentialHistogramMetricEntry) GetIDString() string {
	return strconv.FormatInt(e.ID, 10)
}
