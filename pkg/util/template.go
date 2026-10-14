package util

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// LogData represents a log entry with severity information
// This is a simplified version for template processing
type LogData struct {
	ID           int64   `json:"id"`
	SeverityText *string `json:"severity_text"`
	Severity     int     `json:"severity"`
	Level        *string `json:"level"`
}

// =========================
// Zero-Allocation Functions (Type-Specific)
// =========================

// CategorizeInt categorizes int values without interface{} boxing
func CategorizeInt(count int) string {
	if count < 10 {
		return "small"
	} else if count <= 20 {
		return "medium"
	}
	return "large"
}

// CategorizeInt64 categorizes int64 values without interface{} boxing
func CategorizeInt64(count int64) string {
	if count < 10 {
		return "small"
	} else if count <= 20 {
		return "medium"
	}
	return "large"
}

// CategorizeFloat64 categorizes float64 values without interface{} boxing
func CategorizeFloat64(count float64) string {
	if count < 10 {
		return "small"
	} else if count <= 20 {
		return "medium"
	}
	return "large"
}

// Generic function for type-safe, zero-allocation categorization (Go 1.18+)
func CategorizeNumeric[T int | int64 | float64](v T) string {
	if v < 10 {
		return "small"
	} else if v <= 20 {
		return "medium"
	}
	return "large"
}

// =========================
// Log and Event Counting Functions
// =========================

// CountLogsBySeverity counts logs by severity level.
// It supports multiple input types:
//   - []interface{} containing map[string]interface{}
//   - []map[string]interface{}
//   - Struct slices with SeverityText, Severity, or Level fields
//
// Returns a map with severity levels as keys and counts as values.
// Severity levels are normalized to: ERROR, WARN, INFO, DEBUG
func CountLogsBySeverity(logs interface{}) map[string]int {
	counts := make(map[string]int)

	if logs == nil {
		return counts
	}

	// Handle different types of log data
	switch v := logs.(type) {
	case []interface{}:
		for _, log := range v {
			if logMap, ok := log.(map[string]interface{}); ok {
				severity := getLogSeverity(logMap)
				counts[severity]++
			} else {
				// Log unexpected data structure
				logUnexpectedDataType("log", log, "map[string]interface{}")
			}
		}
	case []map[string]interface{}:
		for _, log := range v {
			severity := getLogSeverity(log)
			counts[severity]++
		}
	case []LogData:
		// Direct type assertion for LogData structs (most common case)
		for _, log := range v {
			severity := getLogDataSeverity(log)
			counts[severity]++
		}
	default:
		// Handle other types using reflection (fallback)
		val := reflect.ValueOf(logs)
		if val.Kind() == reflect.Slice {
			for i := 0; i < val.Len(); i++ {
				elem := val.Index(i)
				if elem.Kind() == reflect.Struct {
					// Try to extract severity from struct fields
					severity := getLogSeverityFromStruct(elem)
					counts[severity]++
				} else {
					// Log unexpected element type
					logUnexpectedDataType("log element", elem.Interface(), "struct")
				}
			}
		} else {
			// Log unexpected input type
			logUnexpectedDataType("logs", logs, "slice")
		}
	}

	return counts
}

// CountEventsByName counts span events by name.
// It supports multiple input types:
//   - []interface{} containing map[string]interface{}
//   - []map[string]interface{}
//
// Returns a map with event names as keys and counts as values.
func CountEventsByName(events interface{}) map[string]int {
	counts := make(map[string]int)

	if events == nil {
		return counts
	}

	// Handle different types of event data
	switch v := events.(type) {
	case []interface{}:
		for _, event := range v {
			if eventMap, ok := event.(map[string]interface{}); ok {
				name := getEventName(eventMap)
				counts[name]++
			} else {
				// Log unexpected data structure
				logUnexpectedDataType("event", event, "map[string]interface{}")
			}
		}
	case []map[string]interface{}:
		for _, event := range v {
			name := getEventName(event)
			counts[name]++
		}
	default:
		// Log unexpected input type
		logUnexpectedDataType("events", events, "slice")
	}

	return counts
}

// FormatLogCountsBySeverity formats log counts by severity into a readable string.
// Example output: "5 logs (1 ERROR, 1 WARN, 2 INFO, 1 DEBUG)"
// Severities are ordered by importance: ERROR, WARN, INFO, DEBUG
func FormatLogCountsBySeverity(logs interface{}) string {
	counts := CountLogsBySeverity(logs)
	if len(counts) == 0 {
		return "0 logs"
	}

	var parts []string
	// Order by severity: ERROR, WARN, INFO, DEBUG
	severities := []string{"ERROR", "WARN", "INFO", "DEBUG"}
	for _, severity := range severities {
		if count, exists := counts[severity]; exists && count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, severity))
		}
	}

	total := 0
	for _, count := range counts {
		total += count
	}

	return fmt.Sprintf("%d logs (%s)", total, strings.Join(parts, ", "))
}

// FormatEventCountsByName formats event counts by name into a readable string.
// Example output: "4 events (2 db_query, 1 cache_hit, 1 http_request)"
// Events are sorted alphabetically for consistent output.
func FormatEventCountsByName(events interface{}) string {
	counts := CountEventsByName(events)
	if len(counts) == 0 {
		return "0 events"
	}

	// Sort event names for consistent output
	var eventNames []string
	for name := range counts {
		eventNames = append(eventNames, name)
	}
	sort.Strings(eventNames)

	var parts []string
	for _, name := range eventNames {
		count := counts[name]
		parts = append(parts, fmt.Sprintf("%d %s", count, name))
	}

	total := 0
	for _, count := range counts {
		total += count
	}

	return fmt.Sprintf("%d events (%s)", total, strings.Join(parts, ", "))
}

// FormatExemplarCountsByType formats exemplar counts by metric type into a readable string.
// Example output: "3 exemplars (2 histogram, 1 summary)"
// Types are sorted alphabetically for consistent output.
func FormatExemplarCountsByType(exemplars interface{}) string {
	counts := CountExemplarsByType(exemplars)
	if len(counts) == 0 {
		return "0 exemplars"
	}

	// Sort metric types for consistent output
	var metricTypes []string
	for metricType := range counts {
		metricTypes = append(metricTypes, metricType)
	}
	sort.Strings(metricTypes)

	var parts []string
	for _, metricType := range metricTypes {
		count := counts[metricType]
		parts = append(parts, fmt.Sprintf("%d %s", count, metricType))
	}

	total := 0
	for _, count := range counts {
		total += count
	}

	return fmt.Sprintf("%d exemplars (%s)", total, strings.Join(parts, ", "))
}

// CountExemplarsByType counts exemplars by metric type.
// It supports multiple input types:
//   - []interface{} containing map[string]interface{}
//   - []ExemplarData (from repository package)
//   - []map[string]interface{}
func CountExemplarsByType(exemplars interface{}) map[string]int {
	counts := make(map[string]int)

	if exemplars == nil {
		return counts
	}

	val := reflect.ValueOf(exemplars)
	if val.Kind() != reflect.Slice {
		return counts
	}

	for i := 0; i < val.Len(); i++ {
		item := val.Index(i)
		metricType := getExemplarMetricType(item)
		counts[metricType]++
	}

	return counts
}

// Helper function to extract metric type from exemplar data
func getExemplarMetricType(exemplar reflect.Value) string {
	// Handle map[string]interface{}
	if exemplar.Kind() == reflect.Map {
		metricTypeKey := reflect.ValueOf("metric_type")
		if exemplar.MapIndex(metricTypeKey).IsValid() && !exemplar.MapIndex(metricTypeKey).IsZero() {
			if metricType, ok := exemplar.MapIndex(metricTypeKey).Interface().(string); ok && metricType != "" {
				return metricType
			}
		}
		metricTypeKey = reflect.ValueOf("MetricType")
		if exemplar.MapIndex(metricTypeKey).IsValid() && !exemplar.MapIndex(metricTypeKey).IsZero() {
			if metricType, ok := exemplar.MapIndex(metricTypeKey).Interface().(string); ok && metricType != "" {
				return metricType
			}
		}
	}

	// Handle struct (ExemplarData)
	if exemplar.Kind() == reflect.Struct {
		metricTypeField := exemplar.FieldByName("MetricType")
		if metricTypeField.IsValid() && !metricTypeField.IsZero() {
			return metricTypeField.String()
		}
	}

	// Handle interface{} that might be a map
	if exemplar.Kind() == reflect.Interface {
		if exemplarMap, ok := exemplar.Interface().(map[string]interface{}); ok {
			if metricType, ok := exemplarMap["metric_type"].(string); ok && metricType != "" {
				return metricType
			}
			if metricType, ok := exemplarMap["MetricType"].(string); ok && metricType != "" {
				return metricType
			}
		}
	}

	// Default to "unknown" if no metric type found
	return "unknown"
}

// Helper function to extract severity from log data
func getLogSeverity(log map[string]interface{}) string {
	// Try different possible severity fields
	if severity, ok := log["severity_text"].(string); ok && severity != "" {
		return mapStringSeverity(severity)
	}
	if severity, ok := log["severity_number"].(int); ok {
		return mapNumericSeverity(severity)
	}
	if level, ok := log["level"].(string); ok && level != "" {
		return mapStringSeverity(level)
	}
	// Default to INFO if no severity information found
	return "INFO"
}

// Helper function to extract name from event data
func getEventName(event map[string]interface{}) string {
	if name, ok := event["name"].(string); ok && name != "" {
		return name
	}
	// Default to "unknown" if no name found
	return "unknown"
}

// Helper function to extract severity from struct using reflection
func getLogSeverityFromStruct(val reflect.Value) string {
	// Try common severity field names
	severityFields := []string{"SeverityText", "Severity", "Level"}

	for _, fieldName := range severityFields {
		field := val.FieldByName(fieldName)
		if field.IsValid() && !field.IsZero() {
			switch field.Kind() {
			case reflect.String:
				return mapStringSeverity(field.String())
			case reflect.Int, reflect.Int64:
				return mapNumericSeverity(int(field.Int()))
			case reflect.Ptr:
				if !field.IsNil() {
					elem := field.Elem()
					switch elem.Kind() {
					case reflect.String:
						return mapStringSeverity(elem.String())
					case reflect.Int, reflect.Int64:
						return mapNumericSeverity(int(elem.Int()))
					}
				}
			}
		}
	}

	// Default to INFO if no severity found
	return "INFO"
}

// Helper function to extract severity from LogData struct
func getLogDataSeverity(log LogData) string {
	if log.SeverityText != nil && *log.SeverityText != "" {
		return mapStringSeverity(*log.SeverityText)
	}
	if log.Severity != 0 {
		return mapNumericSeverity(log.Severity)
	}
	if log.Level != nil && *log.Level != "" {
		return mapStringSeverity(*log.Level)
	}
	// Default to INFO if no severity information found
	return "INFO"
}

// =========================
// Unified Severity Mapping
// =========================

// mapSeverityToStandard maps various severity formats to standard levels.
// Returns one of: ERROR, WARN, INFO, DEBUG
func mapSeverityToStandard(severity interface{}) string {
	switch v := severity.(type) {
	case string:
		return mapStringSeverity(v)
	case int:
		return mapNumericSeverity(v)
	case int64:
		return mapNumericSeverity(int(v))
	case float64:
		return mapNumericSeverity(int(v))
	default:
		return "INFO" // Default fallback
	}
}

// mapStringSeverity maps string severity values to standard levels
func mapStringSeverity(severity string) string {
	upper := strings.ToUpper(strings.TrimSpace(severity))
	switch upper {
	case "ERROR", "FATAL", "CRITICAL":
		return "ERROR"
	case "WARN", "WARNING":
		return "WARN"
	case "INFO", "INFORMATION":
		return "INFO"
	case "DEBUG", "TRACE":
		return "DEBUG"
	default:
		return "INFO" // Default for unknown values
	}
}

// mapNumericSeverity maps numeric severity values to standard levels
// Based on OpenTelemetry severity number mapping
func mapNumericSeverity(severity int) string {
	switch {
	case severity >= 17: // FATAL, CRITICAL
		return "ERROR"
	case severity >= 13: // ERROR
		return "ERROR"
	case severity >= 9: // WARN
		return "WARN"
	case severity >= 5: // INFO
		return "INFO"
	default: // DEBUG, TRACE
		return "DEBUG"
	}
}

// =========================
// Legacy Functions (Interface-based, for backward compatibility)
// =========================

// CategorizeLogCount categorizes log count into small/medium/large
func CategorizeLogCount(count interface{}) string {
	var num int
	switch v := count.(type) {
	case int:
		num = v
	case int64:
		num = int(v)
	case float64:
		num = int(v)
	default:
		return "unknown"
	}

	if num < 10 {
		return "small"
	} else if num <= 20 {
		return "medium"
	} else {
		return "large"
	}
}

// CategorizeDuration categorizes duration into fast/normal/slow
func CategorizeDuration(durationMs interface{}) string {
	var ms int
	switch v := durationMs.(type) {
	case int:
		ms = v
	case int64:
		ms = int(v)
	case float64:
		ms = int(v)
	default:
		return "unknown"
	}

	if ms < 100 {
		return "fast"
	} else if ms <= 1000 {
		return "normal"
	} else {
		return "slow"
	}
}

// CategorizeErrorRate categorizes error rate into low/medium/high
func CategorizeErrorRate(errorCount, totalCount interface{}) string {
	var errors, total int
	switch v := errorCount.(type) {
	case int:
		errors = v
	case int64:
		errors = int(v)
	case float64:
		errors = int(v)
	default:
		return "unknown"
	}

	switch v := totalCount.(type) {
	case int:
		total = v
	case int64:
		total = int(v)
	case float64:
		total = int(v)
	default:
		return "unknown"
	}

	if total == 0 {
		return "no_data"
	}

	rate := float64(errors) / float64(total)
	if rate < 0.01 {
		return "low"
	} else if rate <= 0.05 {
		return "medium"
	} else {
		return "high"
	}
}

// CategorizeSpanCount categorizes span count into simple/medium/complex
func CategorizeSpanCount(count interface{}) string {
	var num int
	switch v := count.(type) {
	case int:
		num = v
	case int64:
		num = int(v)
	case float64:
		num = int(v)
	default:
		return "unknown"
	}

	if num < 5 {
		return "simple"
	} else if num <= 20 {
		return "medium"
	} else {
		return "complex"
	}
}

// =========================
// Zero-Allocation Helper Functions
// =========================

// FastJoin joins strings with minimal allocations
func FastJoin(elems []string, sep string) string {
	if len(elems) == 0 {
		return ""
	}
	if len(elems) == 1 {
		return elems[0]
	}

	n := len(sep) * (len(elems) - 1)
	for i := 0; i < len(elems); i++ {
		n += len(elems[i])
	}

	var b strings.Builder
	b.Grow(n)
	b.WriteString(elems[0])
	for _, s := range elems[1:] {
		b.WriteString(sep)
		b.WriteString(s)
	}
	return b.String()
}

// FastConcat concatenates strings without allocations
func FastConcat(strs ...string) string {
	if len(strs) == 0 {
		return ""
	}
	if len(strs) == 1 {
		return strs[0]
	}

	var b strings.Builder
	for _, s := range strs {
		b.WriteString(s)
	}
	return b.String()
}

// FastReplace performs string replacement efficiently
func FastReplace(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

// FastContains checks if a string contains a substring
func FastContains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// =========================
// Conditional Helper Functions
// =========================

// IfThenElse provides conditional logic in templates
func IfThenElse(condition bool, thenVal, elseVal interface{}) interface{} {
	if condition {
		return thenVal
	}
	return elseVal
}

// DefaultIfEmpty returns defaultValue if value is empty
func DefaultIfEmpty(value, defaultValue interface{}) interface{} {
	if value == nil || value == "" {
		return defaultValue
	}
	return value
}

// Coalesce returns the first non-empty value
func Coalesce(values ...interface{}) interface{} {
	for _, v := range values {
		if v != nil && v != "" {
			return v
		}
	}
	return ""
}

// =========================
// Template Pool for High-Throughput
// =========================

// =========================
// Template-Compatible Format Functions (Interface{} versions)
// =========================

// FormatBytesTemplate formats bytes into human-readable format (interface{} version for templates)
func FormatBytesTemplate(bytes interface{}) string {
	var b int64
	switch v := bytes.(type) {
	case int:
		b = int64(v)
	case int64:
		b = v
	case float64:
		b = int64(v)
	default:
		return "0B"
	}

	return FormatBytes(b)
}

// FormatDurationTemplate formats duration in milliseconds to human-readable format (interface{} version for templates)
func FormatDurationTemplate(durationMs interface{}) string {
	var ms float64
	switch v := durationMs.(type) {
	case int:
		ms = float64(v) / 1000.0 // Convert ms to seconds
	case int64:
		ms = float64(v) / 1000.0
	case float64:
		ms = v / 1000.0
	default:
		return "0ms"
	}

	return FormatDuration(ms)
}

// FormatPercentageTemplate formats a ratio as a percentage (interface{} version for templates)
func FormatPercentageTemplate(numerator, denominator interface{}) string {
	var num, den float64

	switch v := numerator.(type) {
	case int:
		num = float64(v)
	case int64:
		num = float64(v)
	case float64:
		num = v
	default:
		return "0%"
	}

	switch v := denominator.(type) {
	case int:
		den = float64(v)
	case int64:
		den = float64(v)
	case float64:
		den = v
	default:
		return "0%"
	}

	if den == 0 {
		return "0%"
	}

	ratio := num / den
	return FormatPercentage(ratio)
}

// =========================
// Error Logging
// =========================

// logUnexpectedDataType logs unexpected data types for debugging
// In a production environment, this would use a proper logger
func logUnexpectedDataType(dataType string, actual interface{}, expected string) {
	// For now, we'll use a no-op to avoid adding logger dependencies
	// In production, this should use: logger.Warn("Unexpected data type", "dataType", dataType, "actual", fmt.Sprintf("%T", actual), "expected", expected)
	_ = dataType
	_ = actual
	_ = expected
}
