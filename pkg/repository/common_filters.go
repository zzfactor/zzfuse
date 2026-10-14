package repository

import (
	"fmt"
	"regexp"
)

// CommonAdvancedFilters provides DRY advanced filtering capabilities
// that can be embedded in query structs and streaming filters
type CommonAdvancedFilters struct {
	// Text search capabilities
	Q      string   `json:"q,omitempty"`       // q (simple substring search)
	QMode  string   `json:"q_mode,omitempty"`  // q_mode (simple|regex)
	QFlags []string `json:"q_flags,omitempty"` // q_flags (i|m for case-insensitive, multiline)

	// Attribute filtering (for telemetry entry attributes)
	AttrEq     map[string]string   `json:"attr_eq,omitempty"`     // attr.eq[key]=value
	AttrIn     map[string][]string `json:"attr_in,omitempty"`     // attr.in[key]=v1,v2
	AttrExists map[string]bool     `json:"attr_exists,omitempty"` // attr.exists[key]=true

	// Resource attribute filtering (for resource-level attributes)
	ResAttrEq     map[string]string   `json:"res_attr_eq,omitempty"`     // res_attr.eq[key]=value
	ResAttrIn     map[string][]string `json:"res_attr_in,omitempty"`     // res_attr.in[key]=v1,v2
	ResAttrExists map[string]bool     `json:"res_attr_exists,omitempty"` // res_attr.exists[key]=true
}

// CommonServiceFilters provides DRY service-level filtering
type CommonServiceFilters struct {
	// Single service (backward compatibility)
	ServiceName string `json:"service_name,omitempty"`

	// Multi-service filtering (more powerful)
	ServiceNames []string `json:"service_names,omitempty"` // service_in[]

	// Wildcard service filtering
	ServiceNameLike     string `json:"service_name_like,omitempty"`     // SQL LIKE pattern (%, _)
	ServiceNameRegex    string `json:"service_name_regex,omitempty"`    // Regex pattern
	ServiceNamePrefix   string `json:"service_name_prefix,omitempty"`   // Prefix match (optimized)
	ServiceNameSuffix   string `json:"service_name_suffix,omitempty"`   // Suffix match
	ServiceNameContains string `json:"service_name_contains,omitempty"` // Substring match
}

// CommonPaginationFilters provides DRY pagination capabilities
type CommonPaginationFilters struct {
	Limit            int    `json:"limit,omitempty"`
	Offset           int    `json:"offset,omitempty"`            // for offset-based pagination
	Cursor           string `json:"cursor,omitempty"`            // for cursor-based pagination
	Direction        string `json:"direction,omitempty"`         // asc|desc, default desc
	DirectionResults string `json:"direction_results,omitempty"` // asc|desc, for result ordering
}

// CommonBaseFilters combines the most commonly used filters
type CommonBaseFilters struct {
	Schema string `json:"schema,omitempty"` // Schema for filtering
	CommonServiceFilters
	CommonPaginationFilters
}

// ValidateCommonBaseFilters validates the base filter fields
func ValidateCommonBaseFilters(filters CommonBaseFilters) error {
	// Validate limit
	if filters.Limit < 0 {
		return &SchemaValidationError{
			Field:   "limit",
			Value:   fmt.Sprintf("%d", filters.Limit),
			Message: "limit cannot be negative",
		}
	}

	// Validate offset
	if filters.Offset < 0 {
		return &SchemaValidationError{
			Field:   "offset",
			Value:   fmt.Sprintf("%d", filters.Offset),
			Message: "offset cannot be negative",
		}
	}

	// Validate direction
	if filters.Direction != "" && filters.Direction != "asc" && filters.Direction != "desc" {
		return &SchemaValidationError{
			Field:   "direction",
			Value:   filters.Direction,
			Message: "direction must be 'asc' or 'desc'",
		}
	}

	return nil
}

// ValidateCommonAdvancedFilters validates the advanced filter fields
func ValidateCommonAdvancedFilters(filters CommonAdvancedFilters) error {
	// Validate QMode
	if filters.QMode != "" && filters.QMode != "simple" && filters.QMode != "regex" {
		return &SchemaValidationError{
			Field:   "q_mode",
			Value:   filters.QMode,
			Message: "q_mode must be 'simple' or 'regex'",
		}
	}

	// Validate QFlags
	for _, flag := range filters.QFlags {
		if flag != "i" && flag != "m" {
			return &SchemaValidationError{
				Field:   "q_flags",
				Value:   flag,
				Message: "q_flags must contain only 'i' (case-insensitive) or 'm' (multiline)",
			}
		}
	}

	// Validate attribute keys (prevent injection)
	for key := range filters.AttrEq {
		if err := validateAttributeKey(key); err != nil {
			return err
		}
	}
	for key := range filters.AttrIn {
		if err := validateAttributeKey(key); err != nil {
			return err
		}
	}
	for key := range filters.AttrExists {
		if err := validateAttributeKey(key); err != nil {
			return err
		}
	}
	for key := range filters.ResAttrEq {
		if err := validateAttributeKey(key); err != nil {
			return err
		}
	}
	for key := range filters.ResAttrIn {
		if err := validateAttributeKey(key); err != nil {
			return err
		}
	}
	for key := range filters.ResAttrExists {
		if err := validateAttributeKey(key); err != nil {
			return err
		}
	}

	return nil
}

// validateAttributeKey ensures attribute keys are safe for database queries
func validateAttributeKey(key string) error {
	if key == "" {
		return &SchemaValidationError{
			Field:   "attribute_key",
			Value:   key,
			Message: "attribute key cannot be empty",
		}
	}

	// Prevent SQL injection and other unsafe characters
	if len(key) > 255 {
		return &SchemaValidationError{
			Field:   "attribute_key",
			Value:   key,
			Message: "attribute key cannot exceed 255 characters",
		}
	}

	return nil
}

// ValidateCommonServiceFilters validates service filtering fields
func ValidateCommonServiceFilters(filters CommonServiceFilters) error {
	// Count non-empty service filter fields
	filterCount := 0
	if filters.ServiceName != "" {
		filterCount++
	}
	if len(filters.ServiceNames) > 0 {
		filterCount++
	}
	if filters.ServiceNameLike != "" {
		filterCount++
	}
	if filters.ServiceNameRegex != "" {
		filterCount++
	}
	if filters.ServiceNamePrefix != "" {
		filterCount++
	}
	if filters.ServiceNameSuffix != "" {
		filterCount++
	}
	if filters.ServiceNameContains != "" {
		filterCount++
	}

	// Only allow one service filter type at a time
	if filterCount > 1 {
		return &SchemaValidationError{
			Field:   "service_filters",
			Value:   "multiple service filter types specified",
			Message: "Cannot specify multiple service filter types simultaneously",
		}
	}

	// Validate regex pattern if provided
	if filters.ServiceNameRegex != "" {
		if err := validateRegexPattern(filters.ServiceNameRegex); err != nil {
			return &SchemaValidationError{
				Field:   "service_name_regex",
				Value:   filters.ServiceNameRegex,
				Message: fmt.Sprintf("Invalid regex pattern: %v", err),
			}
		}
	}

	return nil
}

func validateRegexPattern(pattern string) error {
	_, err := regexp.Compile(pattern)
	return err
}

// ValidateCommonPaginationFilters validates pagination fields
func ValidateCommonPaginationFilters(filters CommonPaginationFilters) error {
	if filters.Limit < 0 {
		return &SchemaValidationError{
			Field:   "limit",
			Value:   string(rune(filters.Limit)),
			Message: "limit cannot be negative",
		}
	}

	if filters.Offset < 0 {
		return &SchemaValidationError{
			Field:   "offset",
			Value:   string(rune(filters.Offset)),
			Message: "offset cannot be negative",
		}
	}

	if filters.Direction != "" && filters.Direction != "asc" && filters.Direction != "desc" {
		return &SchemaValidationError{
			Field:   "direction",
			Value:   filters.Direction,
			Message: "direction must be 'asc' or 'desc'",
		}
	}

	if filters.DirectionResults != "" && filters.DirectionResults != "asc" && filters.DirectionResults != "desc" {
		return &SchemaValidationError{
			Field:   "direction_results",
			Value:   filters.DirectionResults,
			Message: "direction_results must be 'asc' or 'desc'",
		}
	}

	return nil
}
