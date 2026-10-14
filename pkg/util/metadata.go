package util

import (
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// FlattenMetadata flattens and sanitizes a nested map[string]interface{} for use as metadata.
// Nested keys are joined with dots (e.g., resource.service.name), matching OpenTelemetry conventions.
// All values are preserved with their original type (string, int, float64, bool, etc.).
func FlattenMetadata(prefix string, m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	flattenMetadataRec(prefix, m, result)
	return result
}

// ExtractAndFlattenResourceAttributes extracts and flattens all resource attributes from a pcommon.Resource.
// Keys are prefixed with "resource_" and values are preserved with their original type.
func ExtractAndFlattenResourceAttributes(res pcommon.Resource) map[string]interface{} {
	out := make(map[string]interface{})
	attrs := res.Attributes()
	attrs.Range(func(k string, v pcommon.Value) bool {
		out["resource_"+sanitizeKey("", k)] = valueAsAny(v)
		return true
	})
	return out
}

// ExtractAndFlattenAttributes extracts and flattens all attributes from a pcommon.Map.
// Keys are prefixed with the given prefix (e.g., "attribute_") and values are preserved with their original type.
func ExtractAndFlattenAttributes(attrs pcommon.Map, prefix string) map[string]interface{} {
	out := make(map[string]interface{})
	attrs.Range(func(k string, v pcommon.Value) bool {
		out[prefix+sanitizeKey("", k)] = valueAsAny(v)
		return true
	})
	return out
}

// FilterMetadata filters a metadata map according to include and exclude lists.
// If include is empty, all keys are included by default. Exclude always takes precedence.
// Both include and exclude lists support prefix matching (e.g., "resource_", "attribute_").
func FilterMetadata(meta map[string]string, include, exclude []string) map[string]string {
	result := make(map[string]string)
	for k, v := range meta {
		if matchesAnyPrefix(k, exclude) {
			continue // Exclude always takes precedence
		}
		if len(include) == 0 || matchesAnyPrefix(k, include) {
			result[k] = v
		}
	}
	return result
}

// FlattenKey normalizes a key for use as a database column or vector store field name.
// It converts dots and hyphens to underscores to ensure compatibility with SQL identifiers
// and vector store field naming conventions.
//
// Examples:
//   - "service.name" -> "service_name"
//   - "http.status-code" -> "http_status_code"
//   - "user.id" -> "user_id"
//
// This function is essential for creating valid PostgreSQL column names from OpenTelemetry
// attribute keys which often contain dots and other special characters.
func FlattenKey(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(key, ".", "_"), "-", "_")
}

func sanitizeKey(prefix, k string) string {
	if prefix != "" {
		return prefix + "_" + k
	}
	return k
}

// matchesAnyPrefix returns true if s starts with any prefix in the list.
func matchesAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if len(p) > 0 && len(s) >= len(p) && s[:len(p)] == p {
			return true
		}
	}
	return false
}

// valueAsAny converts a pcommon.Value to its native Go type.
func valueAsAny(v pcommon.Value) interface{} {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return v.Str()
	case pcommon.ValueTypeInt:
		return v.Int()
	case pcommon.ValueTypeDouble:
		return v.Double()
	case pcommon.ValueTypeBool:
		return v.Bool()
	case pcommon.ValueTypeBytes:
		return v.Bytes().AsRaw()
	default:
		return nil
	}
}

// flattenMetadataRec joins nested keys with dot notation for OTel compatibility.
// Preserves value types.
func flattenMetadataRec(prefix string, m map[string]interface{}, out map[string]interface{}) {
	for k, v := range m {
		var key string
		if prefix != "" {
			key = prefix + "." + k
		} else {
			key = k
		}
		switch val := v.(type) {
		case map[string]interface{}:
			flattenMetadataRec(key, val, out)
		default:
			out[key] = val // preserve type
		}
	}
}
