// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package internal

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// MetricTypeConfig holds configuration for a specific metric type table
type MetricTypeConfig struct {
	Name string `mapstructure:"name"`
}

// MetricTablesConfigMapper maps metric types to their table config
type MetricTablesConfigMapper map[pmetric.MetricType]MetricTypeConfig

// AttributesToMap converts OpenTelemetry attributes to map for JSONB marshalling
func AttributesToMap(attrs pcommon.Map) map[string]interface{} {
	result := make(map[string]interface{})
	attrs.Range(func(k string, v pcommon.Value) bool {
		result[k] = v.AsString()
		return true
	})
	return result
}

// AttributeValueToInterface converts a pcommon.Value to a Go interface{} preserving type information
func AttributeValueToInterface(v pcommon.Value) interface{} {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.Str()
	case pcommon.ValueTypeBool:
		return v.Bool()
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeBytes:
		return v.Bytes().AsRaw()
	case pcommon.ValueTypeSlice:
		slice := v.Slice()
		result := make([]interface{}, slice.Len())
		for i := 0; i < slice.Len(); i++ {
			result[i] = AttributeValueToInterface(slice.At(i))
		}
		return result
	case pcommon.ValueTypeMap:
		m := v.Map()
		result := make(map[string]interface{})
		m.Range(func(k string, val pcommon.Value) bool {
			result[k] = AttributeValueToInterface(val)
			return true
		})
		return result
	default:
		return v.AsString()
	}
}

// GetServiceName extracts the service name from resource attributes
func GetServiceName(attrs pcommon.Map) string {
	serviceName, exists := attrs.Get("service.name")
	if exists {
		return serviceName.AsString()
	}
	return ""
}

// GetServiceNamespace extracts the service namespace from resource attributes
func GetServiceNamespace(attrs pcommon.Map) string {
	serviceNamespace, exists := attrs.Get("service.namespace")
	if exists {
		return serviceNamespace.AsString()
	}
	return ""
}

// GetServiceNamespacePtr extracts the service namespace from resource attributes as *string (nullable)
// OTel v1.39.0: service.namespace
func GetServiceNamespacePtr(attrs pcommon.Map) *string {
	serviceNamespace, exists := attrs.Get("service.namespace")
	if exists && serviceNamespace.AsString() != "" {
		s := serviceNamespace.AsString()
		return &s
	}
	return nil
}

// GetServiceInstanceID extracts the service instance ID from resource attributes
// OTel v1.39.0: service.instance.id
func GetServiceInstanceID(attrs pcommon.Map) string {
	serviceInstanceID, exists := attrs.Get("service.instance.id")
	if exists {
		return serviceInstanceID.AsString()
	}
	return ""
}

// GetServiceInstanceIDPtr extracts the service instance ID from resource attributes as *string (nullable)
// OTel v1.39.0: service.instance.id
func GetServiceInstanceIDPtr(attrs pcommon.Map) *string {
	serviceInstanceID, exists := attrs.Get("service.instance.id")
	if exists && serviceInstanceID.AsString() != "" {
		s := serviceInstanceID.AsString()
		return &s
	}
	return nil
}

// GetEventName extracts the event name from log attributes
// OTel v1.39.0: otel.event.name
func GetEventName(attrs pcommon.Map) *string {
	eventName, exists := attrs.Get("otel.event.name")
	if exists && eventName.AsString() != "" {
		s := eventName.AsString()
		return &s
	}
	return nil
}

// SplitStringArray splits a string into an array, handling both single values and comma-separated values
func SplitStringArray(s string) []string {
	if s == "" {
		return []string{}
	}
	// Split by comma and trim whitespace
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Contains checks if a string slice contains the given string
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// GetTrazrRunFlags extracts the run flags from resource attributes
func GetTrazrRunFlags(attrs pcommon.Map) []string {
	trazrRunFlags, exists := attrs.Get("trazr.run.flags")
	if exists {
		return SplitStringArray(trazrRunFlags.AsString())
	}
	return []string{}
}

// GetTrazrRunStart extracts the run start signal from log attributes
// Supports both naming conventions: trazr.run.start and trazr.time.bound.start
func GetTrazrRunStart(attrs pcommon.Map) string {
	// Try trazr.run.start first (primary convention)
	if trazrRunStart, exists := attrs.Get("trazr.run.start"); exists {
		return trazrRunStart.AsString()
	}
	// Try trazr.time.bound.start (alternative convention)
	if trazrTimeBoundStart, exists := attrs.Get("trazr.time.bound.start"); exists {
		return trazrTimeBoundStart.AsString()
	}
	return ""
}

// GetServiceName extracts the service name from resource attributes
func GetTrazrRunStop(attrs pcommon.Map) string {
	trazrRunStart, exists := attrs.Get("trazr.run.stop")
	if exists {
		return trazrRunStart.AsString()
	}
	return ""
}

// TraceIDToHex converts a trace ID to hex string
func TraceIDToHex(traceID pcommon.TraceID) string {
	if traceID.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(traceID[:])
}

// SpanIDToHex converts a span ID to hex string
func SpanIDToHex(spanID pcommon.SpanID) string {
	if spanID.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(spanID[:])
}

// IsDBExistsError checks if the error indicates a database/table already exists
func IsDBExistsError(err error) bool {
	return strings.Contains(err.Error(), "already exists") ||
		strings.Contains(err.Error(), "duplicate key value")
}

// DoWithTx executes a function within a database transaction
func DoWithTx(ctx context.Context, db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("db.Begin: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// ConvertEventsToJSONB converts OpenTelemetry span events to JSONB format
func ConvertEventsToJSONB(events ptrace.SpanEventSlice) []byte {
	result := make([]map[string]interface{}, 0, events.Len())
	for i := 0; i < events.Len(); i++ {
		event := events.At(i)
		eventMap := map[string]interface{}{
			"timestamp":  event.Timestamp().AsTime(),
			"name":       event.Name(),
			"attributes": AttributesToMap(event.Attributes()),
		}
		result = append(result, eventMap)
	}
	jsonData, _ := json.Marshal(result)
	return jsonData
}

// ConvertLinksToJSONB converts OpenTelemetry span links to JSONB format
func ConvertLinksToJSONB(links ptrace.SpanLinkSlice) []byte {
	result := make([]map[string]interface{}, 0, links.Len())
	for i := 0; i < links.Len(); i++ {
		link := links.At(i)
		linkMap := map[string]interface{}{
			"trace_id":    TraceIDToHex(link.TraceID()),
			"span_id":     SpanIDToHex(link.SpanID()),
			"trace_state": link.TraceState().AsRaw(),
			"attributes":  AttributesToMap(link.Attributes()),
		}
		result = append(result, linkMap)
	}
	jsonData, _ := json.Marshal(result)
	return jsonData
}

func NormalizeTimestamp(t time.Time) sql.NullTime {
	if t.IsZero() || t.Equal(time.Unix(0, 0).UTC()) {
		now := time.Now().UTC()
		return sql.NullTime{Time: now, Valid: true}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// Pointer helpers - delegate to shared pkg/util
var (
	PtrFromString  = util.PtrFromString
	PtrFromInt64   = util.PtrFromInt64
	PtrFromFloat64 = util.PtrFromFloat64
	PtrFromInt32   = util.PtrFromInt32
)

// convertExemplars converts OpenTelemetry exemplars to PostgreSQL JSONB format
func ConvertExemplars(exemplars pmetric.ExemplarSlice) []byte {
	if exemplars.Len() == 0 {
		return []byte("[]")
	}

	result := make([]map[string]interface{}, 0, exemplars.Len())
	for i := 0; i < exemplars.Len(); i++ {
		exemplar := exemplars.At(i)

		traceID := exemplar.TraceID()
		spanID := exemplar.SpanID()

		exemplarMap := map[string]interface{}{
			"filtered_attributes": AttributesToMap(exemplar.FilteredAttributes()),
			"timestamp":           exemplar.Timestamp().AsTime(),
			"value":               GetValue(exemplar.IntValue(), exemplar.DoubleValue(), exemplar.ValueType()),
		}

		// Only add trace_id and span_id if they are not empty
		if !traceID.IsEmpty() {
			exemplarMap["trace_id"] = hex.EncodeToString(traceID[:])
		}
		if !spanID.IsEmpty() {
			exemplarMap["span_id"] = hex.EncodeToString(spanID[:])
		}

		result = append(result, exemplarMap)
	}

	jsonData, _ := json.Marshal(result)
	return jsonData
}

// https://github.com/open-telemetry/opentelemetry-proto/blob/main/opentelemetry/proto/metrics/v1/metrics.proto#L358
// define two types for one datapoint value, PostgreSQL uses float64 to store them
func GetValue(intValue int64, floatValue float64, dataType any) float64 {
	switch t := dataType.(type) {
	case pmetric.ExemplarValueType:
		switch t {
		case pmetric.ExemplarValueTypeDouble:
			return floatValue
		case pmetric.ExemplarValueTypeInt:
			return float64(intValue)
		case pmetric.ExemplarValueTypeEmpty:
			return 0.0
		default:
			return 0.0
		}
	case pmetric.NumberDataPointValueType:
		switch t {
		case pmetric.NumberDataPointValueTypeDouble:
			return floatValue
		case pmetric.NumberDataPointValueTypeInt:
			return float64(intValue)
		case pmetric.NumberDataPointValueTypeEmpty:
			return 0.0
		default:
			return 0.0
		}
	default:
		return 0.0
	}
}

// convertValueAtQuantileToJSONB converts summary value at quantile to JSONB
func ConvertValueAtQuantileToJSONB(valueAtQuantile pmetric.SummaryDataPointValueAtQuantileSlice) []byte {
	if valueAtQuantile.Len() == 0 {
		return []byte("[]")
	}

	result := make([]map[string]interface{}, 0, valueAtQuantile.Len())
	for i := 0; i < valueAtQuantile.Len(); i++ {
		vaq := valueAtQuantile.At(i)
		result = append(result, map[string]interface{}{
			"quantile": vaq.Quantile(),
			"value":    vaq.Value(),
		})
	}

	jsonData, _ := json.Marshal(result)
	return jsonData
}

// SafeJSON marshals a value to JSON, returning "{}" on error instead of panicking.
func SafeJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}
