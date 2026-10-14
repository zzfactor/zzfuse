package repository

// getStringPtr safely extracts a string pointer from a signal
func getStringPtr(data map[string]interface{}, key string) *string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return &str
		}
	}
	return nil
}

// getIntValue safely extracts an int value from a signal
func getIntValue(data map[string]interface{}, key string) int {
	if val, ok := data[key]; ok {
		if num, ok := val.(int); ok {
			return num
		}
		if num, ok := val.(float64); ok {
			return int(num)
		}
	}
	return 0
}

// getInt64Value safely extracts an int64 value from a signal
func getInt64Value(data map[string]interface{}, key string) int64 {
	if val, ok := data[key]; ok {
		if num, ok := val.(int64); ok {
			return num
		}
		if num, ok := val.(float64); ok {
			return int64(num)
		}
		if num, ok := val.(int); ok {
			return int64(num)
		}
	}
	return 0
}

// getStringValue safely extracts a string value from a map
func getStringValue(data map[string]interface{}, key string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// stringPtr returns a pointer to the given string
func stringPtr(s string) *string {
	return &s
}

// int64Ptr returns a pointer to the given int64
func int64Ptr(i int64) *int64 {
	return &i
}

// int16Ptr returns a pointer to the given int16
func int16Ptr(i int16) *int16 {
	return &i
}

// AllSupportedSignalTypes returns the canonical list of supported signal types for embedding workers.
func AllSupportedSignalTypes() []string {
	return []string{
		SignalTypeLogs,
		SignalTypeSpans,
		SignalTypeGaugeMetrics,
		SignalTypeSumMetrics,
		SignalTypeHistogramMetrics,
		SignalTypeSummaryMetrics,
		SignalTypeExponentialHistogramMetrics,
		SignalTypeProfiles,
	}
}
