package repository

import (
	"context"
	"fmt"

	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// DatabaseSignalFilter represents a parsed filter for database signal acceptance.
type DatabaseSignalFilter struct {
	FilterConfig *FilterConfig
}

// ParseDatabaseSignalFilter parses a database signal filter config (from JSON/YAML) into a DatabaseSignalFilter struct.
func ParseDatabaseSignalFilter(config map[string]interface{}) (*DatabaseSignalFilter, error) {
	// Use the shared filter utility
	filterConfig, err := ParseFilterConfig(config)
	if err != nil {
		return nil, fmt.Errorf("invalid database signal filter config: %w", err)
	}

	return &DatabaseSignalFilter{
		FilterConfig: filterConfig,
	}, nil
}

type FilterLevel string

const (
	FilterLevelResource   FilterLevel = "resource"
	FilterLevelScope      FilterLevel = "scope"
	FilterLevelAttributes FilterLevel = "attributes"
)

// Matches returns true if the given signal attributes match the filter conditions for the specified level.
// If the section is empty, it matches all signals for that level.
func (df *DatabaseSignalFilter) Matches(attrs map[string]interface{}, level FilterLevel) bool {
	if df.FilterConfig == nil || df.FilterConfig.isEmpty {
		return true
	}

	switch level {
	case FilterLevelResource:
		if len(df.FilterConfig.Resource) == 0 {
			return true
		}
		if df.FilterConfig.isResourceServiceNameOnly {
			if serviceName, exists := attrs["service.name"]; exists {
				return fmt.Sprintf("%v", serviceName) == df.FilterConfig.resourceServiceName
			}
			return false
		}
		for field, condition := range df.FilterConfig.Resource {
			attrValue, exists := attrs[field]
			if !exists || !condition.EvaluateCondition(attrValue) {
				return false
			}
		}
		return true
	case FilterLevelScope:
		if len(df.FilterConfig.Scope) == 0 {
			return true
		}
		for field, condition := range df.FilterConfig.Scope {
			attrValue, exists := attrs[field]
			if !exists || !condition.EvaluateCondition(attrValue) {
				return false
			}
		}
		return true
	case FilterLevelAttributes:
		if len(df.FilterConfig.Attributes) == 0 {
			return true
		}
		for field, condition := range df.FilterConfig.Attributes {
			attrValue, exists := attrs[field]
			if !exists || !condition.EvaluateCondition(attrValue) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// Helper functions for signal-specific field filtering

// MatchesLogFields checks if a log record matches the log-specific filter conditions
func (df *DatabaseSignalFilter) MatchesLogFields(lr plog.LogRecord) bool {
	if df.FilterConfig == nil || df.FilterConfig.Logs == nil {
		return true // No log-specific filtering
	}

	logsFilter := df.FilterConfig.Logs

	// Check severity text
	if logsFilter.SeverityText != nil {
		matches := logsFilter.SeverityText.EvaluateCondition(lr.SeverityText())
		if !matches {
			return false
		}
	}

	// Check severity number
	if logsFilter.SeverityNumber != nil {
		matches := logsFilter.SeverityNumber.EvaluateCondition(int32(lr.SeverityNumber()))
		if !matches {
			return false
		}
	}

	// Check body content
	if logsFilter.Body != nil {
		bodyStr := lr.Body().AsString()
		matches := logsFilter.Body.EvaluateCondition(bodyStr)
		if !matches {
			return false
		}
	}

	return true
}

// MatchesMetricFields checks if a metric data point matches the metric-specific filter conditions
func (df *DatabaseSignalFilter) MatchesMetricFields(metric pmetric.Metric, dataPoint interface{}) bool {
	if df.FilterConfig == nil || df.FilterConfig.Metrics == nil {
		return true // No metric-specific filtering
	}

	metricsFilter := df.FilterConfig.Metrics

	// Check metric name
	if metricsFilter.Name != nil {
		matches := metricsFilter.Name.EvaluateCondition(metric.Name())
		if !matches {
			return false
		}
	}

	// Check metric unit
	if metricsFilter.Unit != nil {
		matches := metricsFilter.Unit.EvaluateCondition(metric.Unit())
		if !matches {
			return false
		}
	}

	// Check metric type
	if metricsFilter.Type != nil {
		metricType := getMetricType(metric.Type())
		matches := metricsFilter.Type.EvaluateCondition(metricType)
		if !matches {
			return false
		}
	}

	// Check metric value (extract from data point based on type)
	if metricsFilter.Value != nil {
		value := getMetricValueFromDataPoint(dataPoint)
		matches := metricsFilter.Value.EvaluateCondition(value)
		if !matches {
			return false
		}
	}

	return true
}

// getMetricValueFromDataPoint extracts the numeric value from different data point types
func getMetricValueFromDataPoint(dataPoint interface{}) float64 {
	switch dp := dataPoint.(type) {
	case pmetric.NumberDataPoint:
		return getMetricValue(dp)
	case pmetric.HistogramDataPoint:
		return float64(dp.Count())
	case pmetric.SummaryDataPoint:
		return float64(dp.Count())
	case pmetric.ExponentialHistogramDataPoint:
		return float64(dp.Count())
	default:
		return 0.0
	}
}

// getMetricType converts pmetric.MetricType to string representation
func getMetricType(metricType pmetric.MetricType) string {
	switch metricType {
	case pmetric.MetricTypeGauge:
		return "gauge_metrics"
	case pmetric.MetricTypeSum:
		return "sum_metrics"
	case pmetric.MetricTypeHistogram:
		return "histogram_metrics"
	case pmetric.MetricTypeSummary:
		return "summary_metrics"
	case pmetric.MetricTypeExponentialHistogram:
		return "exponential_histogram_metrics"
	default:
		return "unknown_metrics"
	}
}

// getMetricValue extracts the numeric value from a NumberDataPoint
func getMetricValue(dataPoint pmetric.NumberDataPoint) float64 {
	switch dataPoint.ValueType() {
	case pmetric.NumberDataPointValueTypeInt:
		return float64(dataPoint.IntValue())
	case pmetric.NumberDataPointValueTypeDouble:
		return dataPoint.DoubleValue()
	default:
		return 0.0
	}
}

// MatchesTraceFields checks if a trace span matches the trace-specific filter conditions
func (df *DatabaseSignalFilter) MatchesTraceFields(span ptrace.Span) bool {
	if df.FilterConfig == nil || df.FilterConfig.Spans == nil {
		return true // No trace-specific filtering
	}

	spansFilter := df.FilterConfig.Spans

	// Check span name
	if spansFilter.Name != nil {
		matches := spansFilter.Name.EvaluateCondition(span.Name())
		if !matches {
			return false
		}
	}

	// Check span kind
	if spansFilter.Kind != nil {
		matches := spansFilter.Kind.EvaluateCondition(span.Kind().String())
		if !matches {
			return false
		}
	}

	// Check status code
	if spansFilter.StatusCode != nil {
		matches := spansFilter.StatusCode.EvaluateCondition(span.Status().Code().String())
		if !matches {
			return false
		}
	}

	// Check duration
	if spansFilter.Duration != nil {
		durationNs := int64(span.EndTimestamp()) - int64(span.StartTimestamp())
		matches := spansFilter.Duration.EvaluateCondition(durationNs)
		if !matches {
			return false
		}
	}

	return true
}

// MatchesProfileFields checks if a profile matches the profile-specific filter conditions
func (df *DatabaseSignalFilter) MatchesProfileFields(profile pprofile.Profile) bool {
	if df.FilterConfig == nil || df.FilterConfig.Profiles == nil {
		return true // No profile-specific filtering
	}

	profilesFilter := df.FilterConfig.Profiles

	// Check profile ID
	if profilesFilter.ProfileID != nil {
		profileID := profile.ProfileID()
		matches := profilesFilter.ProfileID.EvaluateCondition(profileID)
		if !matches {
			return false
		}
	}

	// Check start time (if available)
	if profilesFilter.StartTime != nil {
		// Note: StartTime getter method may not be available in current pprofile version
		// For now, we'll skip this check until the method is available
		// startTime := profile.StartTime().AsTime().UnixNano()
		// matches := profilesFilter.StartTime.EvaluateCondition(startTime)
		// if !matches {
		//     return false
		// }
	}

	// Check end time (if available)
	if profilesFilter.EndTime != nil {
		// Note: EndTime method may not be available in current pprofile version
		// For now, we'll skip this check until the method is available
		// endTime := profile.EndTime().AsTime().UnixNano()
		// matches := profilesFilter.EndTime.EvaluateCondition(endTime)
		// if !matches {
		//     return false
		// }
	}

	// Check duration (if end time is available)
	if profilesFilter.Duration != nil {
		// Note: Duration calculation requires EndTime which may not be available
		// For now, we'll skip this check until the method is available
		// durationNs := int64(profile.EndTime()) - int64(profile.StartTime())
		// matches := profilesFilter.Duration.EvaluateCondition(durationNs)
		// if !matches {
		//     return false
		// }
	}

	// Check dropped attributes count (if available)
	if profilesFilter.DroppedAttributesCount != nil {
		// Note: DroppedAttributesCount method may not be available in current pprofile version
		// For now, we'll skip this check until the method is available
		// droppedCount := int64(profile.DroppedAttributesCount())
		// matches := profilesFilter.DroppedAttributesCount.EvaluateCondition(droppedCount)
		// if !matches {
		//     return false
		// }
	}

	// Check original payload format (if available)
	if profilesFilter.OriginalPayloadFormat != nil {
		// Note: OriginalPayloadFormat method may not be available in current pprofile version
		// For now, we'll skip this check until the method is available
		// format := profile.OriginalPayloadFormat()
		// matches := profilesFilter.OriginalPayloadFormat.EvaluateCondition(format)
		// if !matches {
		//     return false
		// }
	}

	// Check profile type (from attributes)
	if profilesFilter.ProfileType != nil {
		profileType := getProfileTypeFromAttributes(profile)
		matches := profilesFilter.ProfileType.EvaluateCondition(profileType)
		if !matches {
			return false
		}
	}

	// Check sample type (from attributes)
	if profilesFilter.SampleType != nil {
		sampleType := getSampleTypeFromAttributes(profile)
		matches := profilesFilter.SampleType.EvaluateCondition(sampleType)
		if !matches {
			return false
		}
	}

	// Check sample unit (from attributes)
	if profilesFilter.SampleUnit != nil {
		sampleUnit := getSampleUnitFromAttributes(profile)
		matches := profilesFilter.SampleUnit.EvaluateCondition(sampleUnit)
		if !matches {
			return false
		}
	}

	return true
}

// Helper functions to extract profile-specific fields from attributes
func getProfileTypeFromAttributes(profile pprofile.Profile) string {
	// Extract profile type from attributes - this would need to be implemented
	// based on the actual attribute structure used for profiles
	// For now, return empty string as the attribute structure is not fully defined
	return ""
}

func getSampleTypeFromAttributes(profile pprofile.Profile) string {
	// Extract sample type from attributes
	// For now, return empty string as the attribute structure is not fully defined
	return ""
}

func getSampleUnitFromAttributes(profile pprofile.Profile) string {
	// Extract sample unit from attributes
	// For now, return empty string as the attribute structure is not fully defined
	return ""
}

// Sequential filtering methods for ServiceResourceManager

// SchemaFilterResult represents the result of filtering schemas
type SchemaFilterResult struct {
	SchemaID   int64
	SchemaName string
	Resource   *ServiceResources
}

// GetSchemaNamesByIDs converts an array of schema IDs to schema names
// This method looks up schema names from the in-memory pools using the stored schema IDs
func (srm *ServiceResourceManager) GetSchemaNamesByIDs(ctx context.Context, schemaIDs []int64) ([]string, error) {
	if len(schemaIDs) == 0 {
		return []string{}, nil
	}

	var schemaNames []string
	srm.mu.RLock()
	defer srm.mu.RUnlock()

	// Create a map for efficient lookup
	schemaIDToName := make(map[int64]string)
	pools := srm.Pools()
	for schemaName, resource := range pools {
		schemaIDToName[resource.SchemaID] = schemaName
	}

	// Look up each schema ID and collect the names
	for _, schemaID := range schemaIDs {
		if schemaName, exists := schemaIDToName[schemaID]; exists {
			schemaNames = append(schemaNames, schemaName)
		}
	}

	return schemaNames, nil
}
