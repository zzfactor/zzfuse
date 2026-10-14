package repository

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Supported filter operators
const (
	OpEquals  = "="
	OpIn      = "IN"
	OpNotIn   = "NOT IN"
	OpLike    = "LIKE"
	OpGt      = ">"
	OpLt      = "<"
	OpGte     = ">="
	OpLte     = "<="
	OpBetween = "BETWEEN"
)

var SupportedOperators = map[string]struct{}{
	OpEquals:  {},
	OpIn:      {},
	OpNotIn:   {},
	OpLike:    {},
	OpGt:      {},
	OpLt:      {},
	OpGte:     {},
	OpLte:     {},
	OpBetween: {},
}

// quoteFieldName properly quotes PostgreSQL field names
func quoteFieldName(field string) string {
	// If field contains dots or is a reserved word, quote it
	if strings.Contains(field, ".") || isReservedWord(field) {
		return fmt.Sprintf(`"%s"`, field)
	}
	return field
}

// isReservedWord checks if a field name is a PostgreSQL reserved word
func isReservedWord(field string) bool {
	reservedWords := map[string]bool{
		"order": true, "group": true, "where": true, "select": true,
		"from": true, "as": true, "and": true, "or": true, "not": true,
		"in": true, "between": true, "like": true, "is": true, "null": true,
		"true": true, "false": true, "case": true, "when": true, "then": true,
		"else": true, "end": true, "distinct": true, "limit": true, "offset": true,
	}
	return reservedWords[strings.ToLower(field)]
}

// FilterCondition represents a single filter condition
type FilterCondition struct {
	Field    string
	Operator string
	Value    interface{}
}

// FilterConfig represents a parsed filter configuration with SQL caching and fast paths
// Updated for hierarchical structure: signal_type, resource, scope, attributes, and signal-specific fields

type FilterConfig struct {
	SignalType string // Signal type filter (empty = all, "*" = all, specific type, or comma-separated list)
	Resource   map[string]FilterCondition
	Scope      map[string]FilterCondition
	Attributes map[string]FilterCondition

	// Signal-specific filter sections
	Logs     *LogsFilter
	Metrics  *MetricsFilter
	Spans    *SpansFilter
	Profiles *ProfilesFilter

	// Fast path optimizations
	isEmpty                   bool
	isResourceServiceNameOnly bool
	resourceServiceName       string
}

// LogsFilter represents log-specific filter conditions
type LogsFilter struct {
	SeverityText   *FilterCondition
	SeverityNumber *FilterCondition
	Body           *FilterCondition
}

// MetricsFilter represents metric-specific filter conditions
type MetricsFilter struct {
	Name  *FilterCondition
	Value *FilterCondition
	Unit  *FilterCondition
	Type  *FilterCondition
}

// SpansFilter represents trace-specific filter conditions
type SpansFilter struct {
	Name       *FilterCondition
	Kind       *FilterCondition
	StatusCode *FilterCondition
	Duration   *FilterCondition
}

// ProfilesFilter represents profile-specific filter conditions
type ProfilesFilter struct {
	ProfileID              *FilterCondition
	StartTime              *FilterCondition
	EndTime                *FilterCondition
	Duration               *FilterCondition
	DroppedAttributesCount *FilterCondition
	OriginalPayloadFormat  *FilterCondition
	ProfileType            *FilterCondition // From attributes
	SampleType             *FilterCondition // From attributes
	SampleUnit             *FilterCondition // From attributes
}

// parseSignalSpecificField parses a single field from a signal-specific section
func parseSignalSpecificField(sectionMap map[string]interface{}, fieldName string) (*FilterCondition, error) {
	if fieldData, exists := sectionMap[fieldName]; exists {
		cond, err := parseCondition(fieldName, fieldData)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s condition: %w", fieldName, err)
		}
		return &cond, nil
	}
	return nil, nil
}

// ParseFilterConfig parses a hierarchical filter config (from JSON/YAML) into a FilterConfig struct.
func ParseFilterConfig(config map[string]interface{}) (*FilterConfig, error) {
	fc := &FilterConfig{
		Resource:   make(map[string]FilterCondition),
		Scope:      make(map[string]FilterCondition),
		Attributes: make(map[string]FilterCondition),
	}

	// Parse signal_type first
	if signalType, exists := config["signal_type"]; exists {
		if signalTypeStr, ok := signalType.(string); ok {
			// Handle single string value
			if signalTypeStr == SignalTypeAll {
				fc.SignalType = "" // Empty means "all signal types"
			} else {
				fc.SignalType = signalTypeStr
			}
		} else if signalTypeArr, ok := signalType.([]interface{}); ok {
			// Handle array of signal types
			var types []string
			for _, t := range signalTypeArr {
				if str, ok := t.(string); ok {
					if str == SignalTypeAll {
						// If any element is "*", accept all signal types
						fc.SignalType = ""
						break
					}
					types = append(types, str)
				}
			}
			if fc.SignalType != "" { // Not overridden by "*"
				fc.SignalType = strings.Join(types, ",")
			}
		}
	}
	// If signal_type is not specified, fc.SignalType remains empty (accept all)

	// Fast path: all sections empty
	if len(config) == 0 || (config["resource"] == nil && config["scope"] == nil && config["attributes"] == nil &&
		config["logs"] == nil && config["metrics"] == nil && config["spans"] == nil && config["profiles"] == nil) {
		fc.isEmpty = true
		return fc, nil
	}

	// Parse each section
	for section, dest := range map[string]*map[string]FilterCondition{
		"resource":   &fc.Resource,
		"scope":      &fc.Scope,
		"attributes": &fc.Attributes,
	} {
		if sectionData, exists := config[section]; exists {
			if sectionMap, ok := sectionData.(map[string]interface{}); ok {
				for field, condition := range sectionMap {
					cond, err := parseCondition(field, condition)
					if err != nil {
						return nil, fmt.Errorf("failed to parse %s condition for field %s: %w", section, field, err)
					}
					(*dest)[field] = cond
				}
			} else {
				return nil, fmt.Errorf("invalid %s section: must be a map", section)
			}
		}
	}

	// Parse signal-specific sections
	if logsData, exists := config["logs"]; exists {
		if logsMap, ok := logsData.(map[string]interface{}); ok {
			fc.Logs = &LogsFilter{}

			if cond, err := parseSignalSpecificField(logsMap, "severity_text"); err != nil {
				return nil, fmt.Errorf("failed to parse logs.severity_text: %w", err)
			} else {
				fc.Logs.SeverityText = cond
			}

			if cond, err := parseSignalSpecificField(logsMap, "severity_number"); err != nil {
				return nil, fmt.Errorf("failed to parse logs.severity_number: %w", err)
			} else {
				fc.Logs.SeverityNumber = cond
			}

			if cond, err := parseSignalSpecificField(logsMap, "body"); err != nil {
				return nil, fmt.Errorf("failed to parse logs.body: %w", err)
			} else {
				fc.Logs.Body = cond
			}
		} else {
			return nil, fmt.Errorf("invalid logs section: must be a map")
		}
	}

	if metricsData, exists := config["metrics"]; exists {
		if metricsMap, ok := metricsData.(map[string]interface{}); ok {
			fc.Metrics = &MetricsFilter{}

			if cond, err := parseSignalSpecificField(metricsMap, "name"); err != nil {
				return nil, fmt.Errorf("failed to parse metrics.name: %w", err)
			} else {
				fc.Metrics.Name = cond
			}

			if cond, err := parseSignalSpecificField(metricsMap, "value"); err != nil {
				return nil, fmt.Errorf("failed to parse metrics.value: %w", err)
			} else {
				fc.Metrics.Value = cond
			}

			if cond, err := parseSignalSpecificField(metricsMap, "unit"); err != nil {
				return nil, fmt.Errorf("failed to parse metrics.unit: %w", err)
			} else {
				fc.Metrics.Unit = cond
			}

			if cond, err := parseSignalSpecificField(metricsMap, "type"); err != nil {
				return nil, fmt.Errorf("failed to parse metrics.type: %w", err)
			} else {
				fc.Metrics.Type = cond
			}
		} else {
			return nil, fmt.Errorf("invalid metrics section: must be a map")
		}
	}

	if spansData, exists := config["spans"]; exists {
		if spansMap, ok := spansData.(map[string]interface{}); ok {
			fc.Spans = &SpansFilter{}

			if cond, err := parseSignalSpecificField(spansMap, "name"); err != nil {
				return nil, fmt.Errorf("failed to parse spans.name: %w", err)
			} else {
				fc.Spans.Name = cond
			}

			if cond, err := parseSignalSpecificField(spansMap, "kind"); err != nil {
				return nil, fmt.Errorf("failed to parse spans.kind: %w", err)
			} else {
				fc.Spans.Kind = cond
			}

			if cond, err := parseSignalSpecificField(spansMap, "status_code"); err != nil {
				return nil, fmt.Errorf("failed to parse spans.status_code: %w", err)
			} else {
				fc.Spans.StatusCode = cond
			}

			if cond, err := parseSignalSpecificField(spansMap, "duration"); err != nil {
				return nil, fmt.Errorf("failed to parse spans.duration: %w", err)
			} else {
				fc.Spans.Duration = cond
			}
		} else {
			return nil, fmt.Errorf("invalid spans section: must be a map")
		}
	}

	if profilesData, exists := config["profiles"]; exists {
		if profilesMap, ok := profilesData.(map[string]interface{}); ok {
			fc.Profiles = &ProfilesFilter{}

			if cond, err := parseSignalSpecificField(profilesMap, "profile_id"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.profile_id: %w", err)
			} else {
				fc.Profiles.ProfileID = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "start_time"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.start_time: %w", err)
			} else {
				fc.Profiles.StartTime = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "end_time"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.end_time: %w", err)
			} else {
				fc.Profiles.EndTime = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "duration"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.duration: %w", err)
			} else {
				fc.Profiles.Duration = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "dropped_attributes_count"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.dropped_attributes_count: %w", err)
			} else {
				fc.Profiles.DroppedAttributesCount = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "original_payload_format"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.original_payload_format: %w", err)
			} else {
				fc.Profiles.OriginalPayloadFormat = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "profile_type"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.profile_type: %w", err)
			} else {
				fc.Profiles.ProfileType = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "sample_type"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.sample_type: %w", err)
			} else {
				fc.Profiles.SampleType = cond
			}

			if cond, err := parseSignalSpecificField(profilesMap, "sample_unit"); err != nil {
				return nil, fmt.Errorf("failed to parse profiles.sample_unit: %w", err)
			} else {
				fc.Profiles.SampleUnit = cond
			}
		} else {
			return nil, fmt.Errorf("invalid profiles section: must be a map")
		}
	}

	// Fast path: resource section only, service.name only
	if len(fc.Resource) == 1 {
		if cond, ok := fc.Resource["service.name"]; ok && cond.Operator == OpEquals {
			if value, ok := cond.Value.(string); ok && value != "*" {
				fc.isResourceServiceNameOnly = true
				fc.resourceServiceName = value
			}
		}
	}

	// Fast path: all sections empty
	if len(fc.Resource) == 0 && len(fc.Scope) == 0 && len(fc.Attributes) == 0 &&
		fc.Logs == nil && fc.Metrics == nil && fc.Spans == nil {
		fc.isEmpty = true
	}

	return fc, nil
}

// parseCondition parses a single condition into a FilterCondition struct
func parseCondition(field string, condition interface{}) (FilterCondition, error) {
	// Only DSL format is supported for wildcards and all other conditions
	if condMap, ok := condition.(map[string]interface{}); ok {
		op, opOk := condMap["op"].(string)
		value, valueOk := condMap["value"]
		if !opOk || !valueOk {
			return FilterCondition{}, fmt.Errorf("missing operator or value in condition")
		}
		if _, supported := SupportedOperators[op]; !supported {
			return FilterCondition{}, fmt.Errorf("unsupported operator: %s", op)
		}
		return FilterCondition{
			Field:    field,
			Operator: op,
			Value:    value,
		}, nil
	}
	return FilterCondition{}, fmt.Errorf("invalid condition format: only DSL notation is supported")
}

// ValidateFilterConfig checks that all operators in the hierarchical config are supported.
func ValidateFilterConfig(config map[string]interface{}) error {
	// Validate each section
	for section, sectionMap := range config {
		if section == "resource" || section == "scope" || section == "attributes" {
			if sectionData, ok := sectionMap.(map[string]interface{}); ok {
				for field, condition := range sectionData {
					if cond, ok := condition.(map[string]interface{}); ok {
						if op, ok := cond["op"].(string); ok {
							if _, supported := SupportedOperators[op]; !supported {
								return fmt.Errorf("unsupported operator: %s for field %s in %s section", op, field, section)
							}
						} else {
							return fmt.Errorf("missing operator for field %s in %s section", field, section)
						}
					} else {
						return fmt.Errorf("all conditions must use DSL notation (map with 'op' and 'value') for field %s in %s section", field, section)
					}
				}
			} else {
				return fmt.Errorf("invalid %s section: must be a map", section)
			}
		} else {
			return fmt.Errorf("unknown section: %s (expected 'resource', 'scope', or 'attributes')", section)
		}
	}
	return nil
}

// isNumeric checks if a value is numeric
func isNumeric(val interface{}) bool {
	switch val.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

// isBoolean checks if a value is a boolean (true or false)
func isBoolean(val interface{}) bool {
	switch v := val.(type) {
	case bool:
		return true
	case string:
		return v == "true" || v == "false"
	}
	return false
}

// ToSQLWhereClause generates a SQL WHERE clause from signal_type and all three sections.
func (fc *FilterConfig) ToSQLWhereClause() (string, error) {
	var clauses []string

	// Add signal_type condition first (if specified)
	if fc.SignalType != "" {
		if strings.Contains(fc.SignalType, ",") {
			// Multiple signal types - use IN clause
			types := strings.Split(fc.SignalType, ",")
			placeholders := make([]string, len(types))
			for i := range types {
				placeholders[i] = "?"
			}
			clauses = append(clauses, fmt.Sprintf("signal_type IN (%s)", strings.Join(placeholders, ",")))
		} else {
			// Single signal type - use equals
			clauses = append(clauses, "signal_type = ?")
		}
	}

	// Add conditions from all three sections
	for _, section := range []map[string]FilterCondition{fc.Resource, fc.Scope, fc.Attributes} {
		for _, cond := range section {
			clause, err := cond.ToSQLClause()
			if err != nil {
				return "", err
			}
			clauses = append(clauses, clause)
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return strings.Join(clauses, " AND "), nil
}

// ToSQLClause generates a SQL clause for a single condition
func (fc *FilterCondition) ToSQLClause() (string, error) {
	quotedField := quoteFieldName(fc.Field)

	switch fc.Operator {
	case OpEquals:
		if isNumeric(fc.Value) {
			return fmt.Sprintf("%s = %v", quotedField, fc.Value), nil
		}
		// Handle boolean values without quotes
		if isBoolean(fc.Value) {
			return fmt.Sprintf("%s = %v", quotedField, fc.Value), nil
		}
		return fmt.Sprintf("%s = '%v'", quotedField, fc.Value), nil

	case OpIn:
		if slice, ok := fc.Value.([]interface{}); ok {
			var values []string
			for _, v := range slice {
				if isNumeric(v) {
					values = append(values, fmt.Sprintf("%v", v))
				} else {
					values = append(values, fmt.Sprintf("'%v'", v))
				}
			}
			return fmt.Sprintf("%s IN (%s)", quotedField, strings.Join(values, ", ")), nil
		}
		return "", fmt.Errorf("IN operator requires a slice value")

	case OpNotIn:
		if slice, ok := fc.Value.([]interface{}); ok {
			var values []string
			for _, v := range slice {
				if isNumeric(v) {
					values = append(values, fmt.Sprintf("%v", v))
				} else {
					values = append(values, fmt.Sprintf("'%v'", v))
				}
			}
			return fmt.Sprintf("%s NOT IN (%s)", quotedField, strings.Join(values, ", ")), nil
		}
		return "", fmt.Errorf("NOT IN operator requires a slice value")

	case OpLike:
		return fmt.Sprintf("%s LIKE '%v'", quotedField, fc.Value), nil

	case OpGt:
		if !isNumeric(fc.Value) {
			return "", fmt.Errorf("> operator requires numeric value")
		}
		return fmt.Sprintf("%s > %v", quotedField, fc.Value), nil

	case OpLt:
		if !isNumeric(fc.Value) {
			return "", fmt.Errorf("< operator requires numeric value")
		}
		return fmt.Sprintf("%s < %v", quotedField, fc.Value), nil

	case OpGte:
		if !isNumeric(fc.Value) {
			return "", fmt.Errorf(">= operator requires numeric value")
		}
		return fmt.Sprintf("%s >= %v", quotedField, fc.Value), nil

	case OpLte:
		if !isNumeric(fc.Value) {
			return "", fmt.Errorf("<= operator requires numeric value")
		}
		return fmt.Sprintf("%s <= %v", quotedField, fc.Value), nil

	case OpBetween:
		if slice, ok := fc.Value.([]interface{}); ok && len(slice) == 2 {
			if !isNumeric(slice[0]) || !isNumeric(slice[1]) {
				return "", fmt.Errorf("BETWEEN operator requires numeric values")
			}
			return fmt.Sprintf("%s BETWEEN %v AND %v", quotedField, slice[0], slice[1]), nil
		}
		return "", fmt.Errorf("BETWEEN operator requires exactly 2 numeric values")

	default:
		return "", errors.New("unsupported operator in SQL generation")
	}
}

// ToSQLClauseWithArgs generates a SQL clause with parameterized arguments for safe query execution
func (fc *FilterCondition) ToSQLClauseWithArgs(startArgIndex int) (string, []interface{}, error) {
	quotedField := quoteFieldName(fc.Field)

	switch fc.Operator {
	case OpEquals:
		return fmt.Sprintf("%s = $%d", quotedField, startArgIndex), []interface{}{fc.Value}, nil

	case OpIn:
		if slice, ok := fc.Value.([]interface{}); ok {
			placeholders := make([]string, len(slice))
			args := make([]interface{}, len(slice))
			for i, v := range slice {
				placeholders[i] = fmt.Sprintf("$%d", startArgIndex+i)
				args[i] = v
			}
			return fmt.Sprintf("%s IN (%s)", quotedField, strings.Join(placeholders, ", ")), args, nil
		}
		return "", nil, fmt.Errorf("IN operator requires a slice value")

	case OpNotIn:
		if slice, ok := fc.Value.([]interface{}); ok {
			placeholders := make([]string, len(slice))
			args := make([]interface{}, len(slice))
			for i, v := range slice {
				placeholders[i] = fmt.Sprintf("$%d", startArgIndex+i)
				args[i] = v
			}
			return fmt.Sprintf("%s NOT IN (%s)", quotedField, strings.Join(placeholders, ", ")), args, nil
		}
		return "", nil, fmt.Errorf("NOT IN operator requires a slice value")

	case OpLike:
		return fmt.Sprintf("%s LIKE $%d", quotedField, startArgIndex), []interface{}{fc.Value}, nil

	case OpGt:
		if !isNumeric(fc.Value) {
			return "", nil, fmt.Errorf("> operator requires numeric value")
		}
		return fmt.Sprintf("%s > $%d", quotedField, startArgIndex), []interface{}{fc.Value}, nil

	case OpLt:
		if !isNumeric(fc.Value) {
			return "", nil, fmt.Errorf("< operator requires numeric value")
		}
		return fmt.Sprintf("%s < $%d", quotedField, startArgIndex), []interface{}{fc.Value}, nil

	case OpGte:
		if !isNumeric(fc.Value) {
			return "", nil, fmt.Errorf(">= operator requires numeric value")
		}
		return fmt.Sprintf("%s >= $%d", quotedField, startArgIndex), []interface{}{fc.Value}, nil

	case OpLte:
		if !isNumeric(fc.Value) {
			return "", nil, fmt.Errorf("<= operator requires numeric value")
		}
		return fmt.Sprintf("%s <= $%d", quotedField, startArgIndex), []interface{}{fc.Value}, nil

	case OpBetween:
		if slice, ok := fc.Value.([]interface{}); ok && len(slice) == 2 {
			if !isNumeric(slice[0]) || !isNumeric(slice[1]) {
				return "", nil, fmt.Errorf("BETWEEN operator requires numeric values")
			}
			return fmt.Sprintf("%s BETWEEN $%d AND $%d", quotedField, startArgIndex, startArgIndex+1), slice, nil
		}
		return "", nil, fmt.Errorf("BETWEEN operator requires exactly 2 numeric values")

	default:
		return "", nil, errors.New("unsupported operator in SQL generation")
	}
}

// EvaluateCondition evaluates a single condition against a value with optimized type-safe comparisons
func (fc *FilterCondition) EvaluateCondition(value interface{}) bool {
	switch fc.Operator {
	case OpEquals:
		// Special case for wildcard
		if fc.Value == "*" {
			return true // Wildcard always matches
		}
		return reflect.DeepEqual(value, fc.Value)

	case OpIn:
		return fc.evaluateInOperator(value)

	case OpNotIn:
		return fc.evaluateNotInOperator(value)

	case OpLike:
		return fc.evaluateLikeOperator(value)

	case OpGt:
		return fc.evaluateNumericComparison(value, func(a, b float64) bool { return a > b })

	case OpLt:
		return fc.evaluateNumericComparison(value, func(a, b float64) bool { return a < b })

	case OpGte:
		return fc.evaluateNumericComparison(value, func(a, b float64) bool { return a >= b })

	case OpLte:
		return fc.evaluateNumericComparison(value, func(a, b float64) bool { return a <= b })

	case OpBetween:
		return fc.evaluateBetweenOperator(value)

	default:
		return false
	}
}

// evaluateInOperator optimizes IN operator evaluation
func (fc *FilterCondition) evaluateInOperator(value interface{}) bool {
	if slice, ok := fc.Value.([]interface{}); ok {
		for _, v := range slice {
			if reflect.DeepEqual(value, v) {
				return true
			}
		}
	}
	return false
}

// evaluateNotInOperator optimizes NOT IN operator evaluation
func (fc *FilterCondition) evaluateNotInOperator(value interface{}) bool {
	if slice, ok := fc.Value.([]interface{}); ok {
		for _, v := range slice {
			if reflect.DeepEqual(value, v) {
				return false
			}
		}
		return true
	}
	return false
}

// evaluateLikeOperator optimizes LIKE operator evaluation
func (fc *FilterCondition) evaluateLikeOperator(value interface{}) bool {
	valueStr := fmt.Sprintf("%v", value)
	patternStr := fmt.Sprintf("%v", fc.Value)
	// Simple wildcard matching (can be enhanced with regex)
	return strings.Contains(valueStr, strings.ReplaceAll(patternStr, "%", ""))
}

// evaluateNumericComparison handles numeric comparisons with type conversion
func (fc *FilterCondition) evaluateNumericComparison(value interface{}, compare func(float64, float64) bool) bool {
	valFloat, ok1 := toFloat64(value)
	filterFloat, ok2 := toFloat64(fc.Value)
	if !ok1 || !ok2 {
		return false
	}
	return compare(valFloat, filterFloat)
}

// evaluateBetweenOperator handles BETWEEN operator evaluation
func (fc *FilterCondition) evaluateBetweenOperator(value interface{}) bool {
	if slice, ok := fc.Value.([]interface{}); ok && len(slice) == 2 {
		valFloat, ok1 := toFloat64(value)
		minFloat, ok2 := toFloat64(slice[0])
		maxFloat, ok3 := toFloat64(slice[1])
		if !ok1 || !ok2 || !ok3 {
			return false
		}
		return valFloat >= minFloat && valFloat <= maxFloat
	}
	return false
}

// toFloat64 converts various numeric types to float64 for comparison
func toFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}
