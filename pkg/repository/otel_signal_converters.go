package repository

import (
	"encoding/json"
	"fmt"
)

// =========================
// Signal Data Converters
// =========================

// LogEntryConverter implements SignalDataConverter for LogEntry
type LogEntryConverter struct {
	entry *LogEntry
}

// NewLogEntryConverter creates a new LogEntryConverter
func NewLogEntryConverter(entry *LogEntry) *LogEntryConverter {
	return &LogEntryConverter{entry: entry}
}

// ToTemplateData converts LogEntry to template-compatible data structure
func (lec *LogEntryConverter) ToTemplateData() map[string]interface{} {
	// Helper function to safely dereference string pointers
	derefString := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	// Helper function to safely dereference int32 pointers
	derefInt32 := func(i *int32) int32 {
		if i == nil {
			return 0
		}
		return *i
	}

	// Helper function to safely dereference int32 pointers for flags
	derefInt32Ptr := func(i *int32) *int32 {
		return i
	}

	signalData := map[string]interface{}{
		"id":              lec.entry.ID,
		"timestamp_ns":    lec.entry.TimestampNS,
		"service_name":    lec.entry.ServiceName,
		"severity_text":   derefString(lec.entry.SeverityText),
		"severity_number": derefInt32(lec.entry.SeverityNumber),
		"level":           derefString(lec.entry.SeverityText), // Alias for template compatibility
		"trace_id":        derefString(lec.entry.TraceID),
		"span_id":         derefString(lec.entry.SpanID),
		"flags":           derefInt32Ptr(lec.entry.Flags),
		"message":         "", // Will be populated from body if available
	}

	// Parse JSONB fields
	if len(lec.entry.ResourceAttributes) > 0 {
		var resourceAttrs map[string]interface{}
		if err := json.Unmarshal(lec.entry.ResourceAttributes, &resourceAttrs); err == nil {
			signalData["resource_attributes"] = resourceAttrs
		}
	}

	if len(lec.entry.Attributes) > 0 {
		var attrs map[string]interface{}
		if err := json.Unmarshal(lec.entry.Attributes, &attrs); err == nil {
			signalData["attributes"] = attrs
			// Also map attributes to labels for template compatibility
			signalData["labels"] = attrs
		}
	}

	if len(lec.entry.Body) > 0 {
		var body interface{}
		if err := json.Unmarshal(lec.entry.Body, &body); err == nil {
			signalData["body"] = body
			// Also populate message field for template compatibility
			if bodyStr, ok := body.(string); ok {
				signalData["message"] = bodyStr
			} else {
				signalData["message"] = fmt.Sprintf("%v", body)
			}
		}
	}

	// Extract service name from resource attributes if not set
	if signalData["service_name"] == "" {
		if resourceAttrs, ok := signalData["resource_attributes"].(map[string]interface{}); ok {
			if serviceName, ok := resourceAttrs["service.name"].(string); ok {
				signalData["service_name"] = serviceName
			}
		}
	}

	return signalData
}

// GetSignalType returns the signal type for logs
func (lec *LogEntryConverter) GetSignalType() string {
	return SignalTypeLogs
}

// GetMetadata returns the metadata bytes
func (lec *LogEntryConverter) GetMetadata() []byte {
	return lec.entry.Metadata
}

// SetMetadata sets the metadata bytes
func (lec *LogEntryConverter) SetMetadata(metadata []byte) {
	lec.entry.Metadata = metadata
}

// =========================
// Trace Converters
// =========================

// TraceEntryConverter implements SignalDataConverter for TraceEntry
type TraceEntryConverter struct {
	entry *TraceEntry
}

// NewTraceEntryConverter creates a new TraceEntryConverter
func NewTraceEntryConverter(entry *TraceEntry) *TraceEntryConverter {
	return &TraceEntryConverter{entry: entry}
}

// ToTemplateData converts TraceEntry to template-compatible data structure
func (tec *TraceEntryConverter) ToTemplateData() map[string]interface{} {
	signalData := map[string]interface{}{
		"id":           tec.entry.ID,
		"timestamp_ns": tec.entry.TimestampNS,
		"service_name": tec.entry.ServiceName,
		"trace_id":     tec.entry.TraceID,
		"span_id":      tec.entry.SpanID,
		"name":         tec.entry.Name,
		"span_kind":    tec.entry.SpanKind,
		"status_code":  tec.entry.StatusCode,
		"duration":     tec.entry.DurationNS,
		"duration_ms":  tec.entry.DurationNS / 1000000, // Convert nanoseconds to milliseconds
		"logs":         []interface{}{},                // Placeholder for logs - would need to be populated from related logs
	}

	// Parse JSONB fields
	if len(tec.entry.ResourceAttributes) > 0 {
		var resourceAttrs map[string]interface{}
		if err := json.Unmarshal(tec.entry.ResourceAttributes, &resourceAttrs); err == nil {
			signalData["resource_attributes"] = resourceAttrs
		}
	}

	if len(tec.entry.Attributes) > 0 {
		var attrs map[string]interface{}
		if err := json.Unmarshal(tec.entry.Attributes, &attrs); err == nil {
			signalData["attributes"] = attrs
			// Also map attributes to labels for template compatibility
			signalData["labels"] = attrs
		}
	}

	if len(tec.entry.Events) > 0 {
		var events []map[string]interface{}
		if err := json.Unmarshal(tec.entry.Events, &events); err == nil {
			signalData["events"] = events
		}
	}

	if len(tec.entry.Links) > 0 {
		var links []map[string]interface{}
		if err := json.Unmarshal(tec.entry.Links, &links); err == nil {
			signalData["links"] = links
		}
	}

	// Extract service name from resource attributes if not set
	if signalData["service_name"] == "" {
		if resourceAttrs, ok := signalData["resource_attributes"].(map[string]interface{}); ok {
			if serviceName, ok := resourceAttrs["service.name"].(string); ok {
				signalData["service_name"] = serviceName
			}
		}
	}

	return signalData
}

// GetSignalType returns the signal type for traces
func (tec *TraceEntryConverter) GetSignalType() string {
	return SignalTypeSpans
}

// GetMetadata returns the metadata bytes
func (tec *TraceEntryConverter) GetMetadata() []byte {
	return tec.entry.Metadata
}

// SetMetadata sets the metadata bytes
func (tec *TraceEntryConverter) SetMetadata(metadata []byte) {
	tec.entry.Metadata = metadata
}

// =========================
// Metric Converters
// =========================

// MetricEntryConverter implements SignalDataConverter for metric entries
type MetricEntryConverter struct {
	entry interface{}
}

// NewMetricEntryConverter creates a new MetricEntryConverter
func NewMetricEntryConverter(entry interface{}) *MetricEntryConverter {
	return &MetricEntryConverter{entry: entry}
}

// ToTemplateData converts metric entry to template-compatible data structure
func (mec *MetricEntryConverter) ToTemplateData() map[string]interface{} {
	signalData := map[string]interface{}{
		// ✅ FIXED: Use the actual signal type instead of generic "metrics"
		"signal_type": mec.GetSignalType(),
	}

	// Helper function to safely dereference string pointers
	derefString := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	// Helper function to safely dereference float64 pointers
	derefFloat64 := func(f *float64) float64 {
		if f == nil {
			return 0.0
		}
		return *f
	}

	// Type switch to handle different metric types
	switch entry := mec.entry.(type) {
	case *GaugeMetricEntry:
		signalData["id"] = entry.ID
		signalData["timestamp_ns"] = entry.TimestampNS
		signalData["service_name"] = entry.ServiceName
		signalData["name"] = derefString(entry.Name) // ✅ FIXED: Use 'name' instead of 'metric_name' to match template expectations
		signalData["metric_type"] = "gauge"
		signalData["value"] = derefFloat64(entry.Value)

	case *SumMetricEntry:
		signalData["id"] = entry.ID
		signalData["timestamp_ns"] = entry.TimestampNS
		signalData["service_name"] = entry.ServiceName
		signalData["name"] = derefString(entry.Name) // ✅ FIXED: Use 'name' instead of 'metric_name' to match template expectations
		signalData["metric_type"] = "sum"
		signalData["value"] = derefFloat64(entry.Value)
		signalData["unit"] = derefString(entry.Unit) // ✅ ADDED: Add unit field for sum metrics

	case *HistogramMetricEntry:
		signalData["id"] = entry.ID
		signalData["timestamp_ns"] = entry.TimestampNS
		signalData["service_name"] = entry.ServiceName
		signalData["name"] = derefString(entry.Name) // ✅ FIXED: Use 'name' instead of 'metric_name' to match template expectations
		signalData["metric_type"] = "histogram"
		// ✅ ADDED: Add histogram-specific fields
		if entry.Count != nil {
			signalData["count"] = *entry.Count
		}
		if entry.Sum != nil {
			signalData["sum"] = *entry.Sum
		}
		if entry.Buckets != nil {
			signalData["buckets"] = entry.Buckets
		}

	case *SummaryMetricEntry:
		signalData["id"] = entry.ID
		signalData["timestamp_ns"] = entry.TimestampNS
		signalData["service_name"] = entry.ServiceName
		signalData["name"] = derefString(entry.Name) // ✅ FIXED: Use 'name' instead of 'metric_name' to match template expectations
		signalData["metric_type"] = "summary"
		// ✅ ADDED: Add summary-specific fields
		if entry.Count != nil {
			signalData["count"] = *entry.Count
		}
		if entry.Sum != nil {
			signalData["sum"] = *entry.Sum
		}
		if entry.Quantiles != nil {
			signalData["quantiles"] = entry.Quantiles
		}

	case *ExponentialHistogramMetricEntry:
		signalData["id"] = entry.ID
		signalData["timestamp_ns"] = entry.TimestampNS
		signalData["service_name"] = entry.ServiceName
		signalData["name"] = derefString(entry.Name) // ✅ FIXED: Use 'name' instead of 'metric_name' to match template expectations
		signalData["metric_type"] = "exponential_histogram"
		// ✅ ADDED: Add exponential histogram-specific fields
		if entry.Count != nil {
			signalData["count"] = *entry.Count
		}
		if entry.Sum != nil {
			signalData["sum"] = *entry.Sum
		}
		if entry.Scale != nil {
			signalData["scale"] = *entry.Scale
		}
	}

	// Parse JSONB fields for all metric types
	if metricEntry, ok := mec.entry.(interface{ GetResourceAttributes() []byte }); ok {
		if len(metricEntry.GetResourceAttributes()) > 0 {
			var resourceAttrs map[string]interface{}
			if err := json.Unmarshal(metricEntry.GetResourceAttributes(), &resourceAttrs); err == nil {
				signalData["resource_attributes"] = resourceAttrs
			}
		}
	}

	if metricEntry, ok := mec.entry.(interface{ GetAttributes() []byte }); ok {
		if len(metricEntry.GetAttributes()) > 0 {
			var attrs map[string]interface{}
			if err := json.Unmarshal(metricEntry.GetAttributes(), &attrs); err == nil {
				signalData["attributes"] = attrs
				// ✅ FIXED: Also map attributes to labels for template compatibility
				signalData["labels"] = attrs
			}
		}
	}

	// Extract service name from resource attributes if not set
	if signalData["service_name"] == "" {
		if resourceAttrs, ok := signalData["resource_attributes"].(map[string]interface{}); ok {
			if serviceName, ok := resourceAttrs["service.name"].(string); ok {
				signalData["service_name"] = serviceName
			}
		}
	}

	return signalData
}

// GetSignalType returns the signal type for metrics
func (mec *MetricEntryConverter) GetSignalType() string {
	// Return the specific metric type based on the entry type
	switch mec.entry.(type) {
	case *GaugeMetricEntry:
		return SignalTypeGaugeMetrics
	case *SumMetricEntry:
		return SignalTypeSumMetrics
	case *HistogramMetricEntry:
		return SignalTypeHistogramMetrics
	case *SummaryMetricEntry:
		return SignalTypeSummaryMetrics
	case *ExponentialHistogramMetricEntry:
		return SignalTypeExponentialHistogramMetrics
	default:
		return SignalTypeMetrics
	}
}

// GetMetadata returns the metadata bytes
func (mec *MetricEntryConverter) GetMetadata() []byte {
	if metricEntry, ok := mec.entry.(interface{ GetMetadata() []byte }); ok {
		return metricEntry.GetMetadata()
	}
	return nil
}

// SetMetadata sets the metadata bytes
func (mec *MetricEntryConverter) SetMetadata(metadata []byte) {
	if metricEntry, ok := mec.entry.(interface{ SetMetadata([]byte) }); ok {
		metricEntry.SetMetadata(metadata)
	}
}

// =========================
// Profile Converters
// =========================

// ProfileEntryConverter implements SignalDataConverter for ProfileEntry
type ProfileEntryConverter struct {
	entry *ProfileEntry
}

// NewProfileEntryConverter creates a new ProfileEntryConverter
func NewProfileEntryConverter(entry *ProfileEntry) *ProfileEntryConverter {
	return &ProfileEntryConverter{entry: entry}
}

// ToTemplateData converts ProfileEntry to template-compatible data structure
func (pec *ProfileEntryConverter) ToTemplateData() map[string]interface{} {
	// Helper function to safely dereference string pointers
	derefString := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	signalData := map[string]interface{}{
		"id":           pec.entry.ID,
		"timestamp_ns": pec.entry.TimestampNS,
		"service_name": pec.entry.ServiceName,
		"profile_type": pec.entry.ProfileType,
		"duration":     pec.entry.DurationNS,
		"sample_type":  derefString(pec.entry.SampleType),
		"sample_unit":  derefString(pec.entry.SampleUnit),
		"profile_size": len(pec.entry.ProfileBytes),
	}

	// Parse JSONB fields
	if len(pec.entry.ResourceAttributes) > 0 {
		var resourceAttrs map[string]interface{}
		if err := json.Unmarshal(pec.entry.ResourceAttributes, &resourceAttrs); err == nil {
			signalData["resource_attributes"] = resourceAttrs
		}
	}

	if len(pec.entry.Attributes) > 0 {
		var attrs map[string]interface{}
		if err := json.Unmarshal(pec.entry.Attributes, &attrs); err == nil {
			signalData["attributes"] = attrs
			// Also map attributes to labels for template compatibility
			signalData["labels"] = attrs
		}
	}

	// Extract service name from resource attributes if not set
	if signalData["service_name"] == "" {
		if resourceAttrs, ok := signalData["resource_attributes"].(map[string]interface{}); ok {
			if serviceName, ok := resourceAttrs["service.name"].(string); ok {
				signalData["service_name"] = serviceName
			}
		}
	}

	return signalData
}

// GetSignalType returns the signal type for profiles
func (pec *ProfileEntryConverter) GetSignalType() string {
	return SignalTypeProfiles
}

// GetMetadata returns the metadata bytes
func (pec *ProfileEntryConverter) GetMetadata() []byte {
	return pec.entry.Metadata
}

// SetMetadata sets the metadata bytes
func (pec *ProfileEntryConverter) SetMetadata(metadata []byte) {
	pec.entry.Metadata = metadata
}
