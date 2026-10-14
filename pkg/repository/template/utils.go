package template

import (
	"fmt"
	"strings"
)

// FieldMappingConfig holds configuration for field mapping
type FieldMappingConfig struct {
	JSONBFields   map[string]bool   `json:"jsonb_fields"`
	FieldMappings map[string]string `json:"field_mappings"`
	SignalTypeMap map[string]string `json:"signal_type_map"`
}

const (
	SignalTypeLog                        = 1
	SignalTypeTrace                      = 2
	SignalTypeMetric                     = 3
	SignalTypeProfile                    = 4
	SignalTypeGaugeMetric                = 5
	SignalTypeSumMetric                  = 6
	SignalTypeHistogramMetric            = 7
	SignalTypeSummaryMetric              = 8
	SignalTypeExponentialHistogramMetric = 9
	SignalTypeCounterMetric              = 10
)

// SignalTypeToTableMap maps signal type constants to their corresponding database table names
var SignalTypeToTableMap = map[int16]string{
	SignalTypeLog:                        "logs",
	SignalTypeTrace:                      "spans",
	SignalTypeProfile:                    "profiles",
	SignalTypeGaugeMetric:                "gauge_metrics",
	SignalTypeSumMetric:                  "sum_metrics",
	SignalTypeHistogramMetric:            "histogram_metrics",
	SignalTypeSummaryMetric:              "summary_metrics",
	SignalTypeExponentialHistogramMetric: "exponential_histogram_metrics",
	SignalTypeCounterMetric:              "sum_metrics",
}

// DefaultFieldMappingConfig returns default field mapping configuration
func DefaultFieldMappingConfig() *FieldMappingConfig {
	return &FieldMappingConfig{
		JSONBFields: map[string]bool{
			"attributes":                      true,
			"resource_attributes":             true,
			"scope_attributes":                true,
			"events":                          true,
			"logs":                            true,
			"exemplars_histogram":             true,
			"exemplars_summary":               true,
			"exemplars_gauge":                 true,
			"exemplars_sum":                   true,
			"exemplars_exponential_histogram": true,
		},
		FieldMappings: map[string]string{
			"duration_ms": "duration",
			"labels":      "attributes",
			"level":       "severity_text",
			"message":     "body",
		},
		SignalTypeMap: map[string]string{
			"gauge_metrics":                 "gauge_metrics",
			"sum_metrics":                   "sum_metrics",
			"histogram_metrics":             "histogram_metrics",
			"summary_metrics":               "summary_metrics",
			"exponential_histogram_metrics": "exponential_histogram_metrics",
			"counter_metrics":               "sum_metrics",
			"spans":                         "spans",
			"logs":                          "logs",
		},
	}
}

// FieldMapping provides centralized field mapping utilities
type FieldMapping struct {
	config *FieldMappingConfig
}

// NewFieldMapping creates a new field mapping utility with default configuration
func NewFieldMapping() *FieldMapping {
	return NewFieldMappingWithConfig(DefaultFieldMappingConfig())
}

// NewFieldMappingWithConfig creates a new field mapping utility with custom configuration
func NewFieldMappingWithConfig(config *FieldMappingConfig) *FieldMapping {
	return &FieldMapping{
		config: config,
	}
}

// IsJSONBField checks if a field is a JSONB field
func (fm *FieldMapping) IsJSONBField(field string) bool {
	// Check if it's a known JSONB field
	if fm.config.JSONBFields[field] {
		return true
	}

	// Check if it contains dots (nested field access)
	if strings.Contains(field, ".") {
		parts := strings.Split(field, ".")
		return fm.config.JSONBFields[parts[0]]
	}

	return false
}

// ParseJSONBField parses a JSONB field reference into base field and specific key
func (fm *FieldMapping) ParseJSONBField(field string) (baseField, specificKey string) {
	if !strings.Contains(field, ".") {
		return field, ""
	}

	parts := strings.Split(field, ".")
	if len(parts) < 2 {
		return field, ""
	}

	baseField = parts[0]
	specificKey = strings.Join(parts[1:], ".")
	return baseField, specificKey
}

// MapTemplateFieldToDBField handles common field name variations between templates and database
func (fm *FieldMapping) MapTemplateFieldToDBField(fieldName string) string {
	if mapped, exists := fm.config.FieldMappings[fieldName]; exists {
		return mapped
	}
	return fieldName
}

// GetSignalTypeForFieldConfig maps worker signal types to field config signal types
func (fm *FieldMapping) GetSignalTypeForFieldConfig(signalType string) string {
	if mapped, exists := fm.config.SignalTypeMap[signalType]; exists {
		return mapped
	}
	return signalType
}

// MapKeysToSlice converts a map[string]bool to []string
func (fm *FieldMapping) MapKeysToSlice(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	return result
}

// ConvertJSONBMap converts the JSONB field map structure
func (fm *FieldMapping) ConvertJSONBMap(m map[string]map[string]bool) map[string][]string {
	result := make(map[string][]string)
	for baseField, keys := range m {
		result[baseField] = fm.MapKeysToSlice(keys)
	}
	return result
}

// UniqueStrings removes duplicate strings from a slice
func (fm *FieldMapping) UniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	result := []string{}

	for _, item := range slice {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}

	return result
}

// ValidateTemplateContent validates template content
func (fm *FieldMapping) ValidateTemplateContent(templateContent map[string]string, maxSize int) error {
	if len(templateContent) == 0 {
		return fmt.Errorf("no template content provided")
	}

	for key, content := range templateContent {
		if content == "" {
			return fmt.Errorf("template content for key '%s' cannot be empty", key)
		}
		if len(content) > maxSize {
			return fmt.Errorf("template content for key '%s' exceeds maximum size of %d bytes", key, maxSize)
		}
	}

	return nil
}

// GenerateSQLSelect generates the SQL SELECT clause for optimized field lists
func (fm *FieldMapping) GenerateSQLSelect(optimizedList *OptimizedFieldList) string {
	var selectClause []string

	// Add static fields
	for _, field := range optimizedList.StaticFields {
		selectClause = append(selectClause, field)
	}

	// Add full JSONB fields
	for _, field := range optimizedList.FullJSONBFields {
		selectClause = append(selectClause, field)
	}

	// Add specific JSONB field extractions
	for baseField, keys := range optimizedList.JSONBFields {
		for _, key := range keys {
			// Convert underscore format to dot format for database
			dbKey := strings.ReplaceAll(key, "_", ".")
			alias := fmt.Sprintf("%s_%s", baseField, strings.ReplaceAll(key, ".", "_"))
			selectClause = append(selectClause,
				fmt.Sprintf("COALESCE(%s->>'%s', '') as %s", baseField, dbKey, alias))
		}
	}

	return strings.Join(selectClause, ", ")
}

// GetRequiredFields returns all required fields for database query
func (fm *FieldMapping) GetRequiredFields(optimizedList *OptimizedFieldList) []string {
	var fields []string

	// Add static fields
	fields = append(fields, optimizedList.StaticFields...)

	// Add full JSONB fields
	fields = append(fields, optimizedList.FullJSONBFields...)

	// Add base JSONB fields for specific extractions
	for baseField := range optimizedList.JSONBFields {
		fields = append(fields, baseField)
	}

	return fields
}

// OptimizedFieldList represents the optimized field configuration for database queries
type OptimizedFieldList struct {
	SignalType      string
	StaticFields    []string            // Regular database columns
	JSONBFields     map[string][]string // JSONB field -> specific keys to extract
	FullJSONBFields []string            // Full JSONB fields to include
}
