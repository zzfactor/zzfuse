package repository

import (
	"fmt"
	"strings"
)

// SchemaValidationError represents a schema validation error
type SchemaValidationError struct {
	Field   string
	Value   string
	Message string
}

func (e SchemaValidationError) Error() string {
	return fmt.Sprintf("schema validation error for field '%s' with value '%s': %s", e.Field, e.Value, e.Message)
}

// ValidateSchema validates a schema string against common patterns
func ValidateSchema(schema string) error {
	if schema == "" {
		return nil // Empty schema is valid (no filtering)
	}

	// Trim whitespace
	schema = strings.TrimSpace(schema)

	// Check for common invalid characters
	if strings.ContainsAny(schema, "\n\r\t") {
		return &SchemaValidationError{
			Field:   "schema",
			Value:   schema,
			Message: "schema cannot contain newlines or tabs",
		}
	}

	// Check length (reasonable limit)
	if len(schema) > 255 {
		return &SchemaValidationError{
			Field:   "schema",
			Value:   schema,
			Message: "schema length cannot exceed 255 characters",
		}
	}

	return nil
}

// ValidateLogQuery validates a LogQuery struct including schema and other fields.
func ValidateLogQuery(query LogQuery) []string {
	var errors []string

	// Validate schema
	if err := ValidateSchema(query.Schema); err != nil {
		errors = append(errors, err.Error())
	}

	// Validate common base filters
	if baseErr := ValidateCommonBaseFilters(query.CommonBaseFilters); baseErr != nil {
		errors = append(errors, baseErr.Error())
	}

	// Validate common advanced filters
	if advErr := ValidateCommonAdvancedFilters(query.CommonAdvancedFilters); advErr != nil {
		errors = append(errors, advErr.Error())
	}

	// Add log-specific validations here
	if query.SeverityMin != nil && *query.SeverityMin < 0 {
		errors = append(errors, "severity_min cannot be negative")
	}
	if query.SeverityMax != nil && *query.SeverityMax < 0 {
		errors = append(errors, "severity_max cannot be negative")
	}
	if query.SeverityMin != nil && query.SeverityMax != nil && *query.SeverityMin > *query.SeverityMax {
		errors = append(errors, "severity_min cannot be greater than severity_max")
	}

	return errors
}

// ValidateTraceQuery validates a TraceQuery struct including schema validation
func ValidateTraceQuery(query TraceQuery) error {
	if err := ValidateSchema(query.Schema); err != nil {
		return err
	}

	// Additional TraceQuery-specific validations can be added here
	if query.Limit < 0 {
		return &SchemaValidationError{
			Field:   "limit",
			Value:   fmt.Sprintf("%d", query.Limit),
			Message: "limit cannot be negative",
		}
	}

	return nil
}

// ValidateMetricQuery validates a MetricQuery struct including schema validation
func ValidateMetricQuery(query MetricQuery) error {
	if err := ValidateSchema(query.Schema); err != nil {
		return err
	}

	// Additional MetricQuery-specific validations can be added here
	if query.Limit < 0 {
		return &SchemaValidationError{
			Field:   "limit",
			Value:   fmt.Sprintf("%d", query.Limit),
			Message: "limit cannot be negative",
		}
	}

	return nil
}
