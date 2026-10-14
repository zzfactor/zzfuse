// Package util provides utility functions for processing OpenTelemetry attributes and data structures.
// These functions handle common operations like service name extraction, attribute flattening,
// and data type conversions for telemetry data processing.
package util

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// ExtractServiceName extracts the service name from OpenTelemetry resource attributes.
// It looks for the standard "service.name" attribute and returns its string value.
// Returns an empty string if the service name is not found or is not a string type.
//
// This function is commonly used for service-based routing and filtering in telemetry pipelines.
func ExtractServiceName(attrs pcommon.Map) string {
	v, ok := attrs.Get("service.name")
	if ok && v.Type() == pcommon.ValueTypeStr {
		return v.Str()
	}
	return ""
}

// ExtractAttributesAndResource merges data point attributes and resource attributes into a single map.
// Both attribute maps are converted to string values and merged, with data point attributes
// taking precedence over resource attributes in case of key conflicts.
//
// Parameters:
//   - dpAttrs: Data point level attributes (higher precedence)
//   - resourceAttrs: Resource level attributes (lower precedence)
//
// Returns a map[string]interface{} containing all merged attributes as string values.
// This function is useful for creating flat attribute representations for storage or analysis.
func ExtractAttributesAndResource(dpAttrs, resourceAttrs pcommon.Map) map[string]interface{} {
	meta := make(map[string]interface{})
	dpAttrs.Range(func(k string, v pcommon.Value) bool {
		meta[k] = v.AsString()
		return true
	})
	resourceAttrs.Range(func(k string, v pcommon.Value) bool {
		meta[k] = v.AsString()
		return true
	})
	return meta
}

// FormatArray formats a slice or array as a comma-separated string.
func FormatArray(arr interface{}) string {
	switch v := arr.(type) {
	case []interface{}:
		var out []string
		for _, elem := range v {
			out = append(out, fmt.Sprintf("%v", elem))
		}
		return strings.Join(out, ", ")
	case []string:
		return strings.Join(v, ", ")
	case []float64:
		var out []string
		for _, elem := range v {
			out = append(out, fmt.Sprintf("%v", elem))
		}
		return strings.Join(out, ", ")
	default:
		return fmt.Sprintf("%v", arr)
	}
}

// FormatMap formats a map as key1=val1, key2=val2, ...
func FormatMap(m interface{}) string {
	if m == nil {
		return ""
	}
	var out []string
	switch v := m.(type) {
	case map[string]interface{}:
		for k, val := range v {
			out = append(out, fmt.Sprintf("%s=%v", k, val))
		}
	case map[string]string:
		for k, val := range v {
			out = append(out, fmt.Sprintf("%s=%s", k, val))
		}
	default:
		return fmt.Sprintf("%v", m)
	}
	return strings.Join(out, ", ")
}

// FormatLabels formats an attributes map as label1=val1,label2=val2
func FormatLabels(attrs interface{}) string {
	if attrs == nil {
		return ""
	}
	var out []string
	switch v := attrs.(type) {
	case map[string]interface{}:
		for k, val := range v {
			out = append(out, fmt.Sprintf("%s=%v", k, val))
		}
	case map[string]string:
		for k, val := range v {
			out = append(out, fmt.Sprintf("%s=%s", k, val))
		}
	default:
		return fmt.Sprintf("%v", attrs)
	}
	return strings.Join(out, ",")
}
