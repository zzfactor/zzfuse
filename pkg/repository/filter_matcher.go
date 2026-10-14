package repository

import (
	"encoding/json"
	"regexp"
	"strings"
)

// FilterMatcher provides consistent attribute filtering logic for both query and streaming filters
type FilterMatcher struct{}

// NewFilterMatcher creates a new filter matcher instance
func NewFilterMatcher() *FilterMatcher {
	return &FilterMatcher{}
}

// MatchesCommonAdvancedFilters checks if the given data matches the advanced filter criteria
// This ensures consistent behavior between SQL queries and streaming filters
func (fm *FilterMatcher) MatchesCommonAdvancedFilters(filters CommonAdvancedFilters, attributes, resourceAttributes map[string]interface{}, bodyText string) bool {
	// Text search filtering
	if !fm.matchesTextSearch(filters, bodyText) {
		return false
	}

	// Attribute filtering
	if !fm.matchesAttributeFilters(filters, attributes) {
		return false
	}

	// Resource attribute filtering
	if !fm.matchesResourceAttributeFilters(filters, resourceAttributes) {
		return false
	}

	return true
}

// matchesTextSearch handles Q, QMode, and QFlags filtering
func (fm *FilterMatcher) matchesTextSearch(filters CommonAdvancedFilters, bodyText string) bool {
	if strings.TrimSpace(filters.Q) == "" {
		return true // No text search filter
	}

	searchText := filters.Q
	targetText := bodyText

	// Handle QFlags for case-insensitive search
	caseInsensitive := false
	for _, flag := range filters.QFlags {
		if strings.ToLower(flag) == "i" {
			caseInsensitive = true
			break
		}
	}

	if caseInsensitive {
		searchText = strings.ToLower(searchText)
		targetText = strings.ToLower(targetText)
	}

	// Handle QMode
	switch strings.ToLower(filters.QMode) {
	case "regex":
		// Compile regex with appropriate flags
		flags := ""
		if caseInsensitive {
			flags = "(?i)"
		}
		regexPattern := flags + searchText

		regex, err := regexp.Compile(regexPattern)
		if err != nil {
			return false // Invalid regex fails the match
		}
		return regex.MatchString(targetText)

	case "simple", "":
		// Simple substring search (default)
		return strings.Contains(targetText, searchText)

	default:
		return false // Unknown QMode fails the match
	}
}

// matchesAttributeFilters handles AttrEq, AttrIn, AttrExists filtering
func (fm *FilterMatcher) matchesAttributeFilters(filters CommonAdvancedFilters, attributes map[string]interface{}) bool {
	// AttrEq filtering
	for key, expectedValue := range filters.AttrEq {
		actualValue, exists := attributes[key]
		if !exists {
			return false
		}
		if fm.convertToString(actualValue) != expectedValue {
			return false
		}
	}

	// AttrIn filtering
	for key, allowedValues := range filters.AttrIn {
		actualValue, exists := attributes[key]
		if !exists {
			return false
		}
		actualStr := fm.convertToString(actualValue)
		found := false
		for _, allowedValue := range allowedValues {
			if actualStr == allowedValue {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// AttrExists filtering
	for key, shouldExist := range filters.AttrExists {
		_, exists := attributes[key]
		if shouldExist && !exists {
			return false
		}
		if !shouldExist && exists {
			return false
		}
	}

	return true
}

// matchesResourceAttributeFilters handles ResAttrEq, ResAttrIn, ResAttrExists filtering
func (fm *FilterMatcher) matchesResourceAttributeFilters(filters CommonAdvancedFilters, resourceAttributes map[string]interface{}) bool {
	// ResAttrEq filtering
	for key, expectedValue := range filters.ResAttrEq {
		actualValue, exists := resourceAttributes[key]
		if !exists {
			return false
		}
		if fm.convertToString(actualValue) != expectedValue {
			return false
		}
	}

	// ResAttrIn filtering
	for key, allowedValues := range filters.ResAttrIn {
		actualValue, exists := resourceAttributes[key]
		if !exists {
			return false
		}
		actualStr := fm.convertToString(actualValue)
		found := false
		for _, allowedValue := range allowedValues {
			if actualStr == allowedValue {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// ResAttrExists filtering
	for key, shouldExist := range filters.ResAttrExists {
		_, exists := resourceAttributes[key]
		if shouldExist && !exists {
			return false
		}
		if !shouldExist && exists {
			return false
		}
	}

	return true
}

// convertToString safely converts interface{} to string for comparison
func (fm *FilterMatcher) convertToString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case nil:
		return ""
	default:
		// For complex types, use JSON representation
		if bytes, err := json.Marshal(v); err == nil {
			return string(bytes)
		}
		return ""
	}
}

// MatchesCommonBaseFilters checks basic filter criteria (service name, pagination, etc.)
func (fm *FilterMatcher) MatchesCommonBaseFilters(filters CommonBaseFilters, serviceName string) bool {
	return fm.matchesServiceFilters(filters.CommonServiceFilters, serviceName)
}

// matchesServiceFilters handles all service name filtering variants
func (fm *FilterMatcher) matchesServiceFilters(filters CommonServiceFilters, serviceName string) bool {
	// Exact service name match
	if filters.ServiceName != "" {
		return serviceName == filters.ServiceName
	}

	// Multiple services (IN list)
	if len(filters.ServiceNames) > 0 {
		for _, allowedService := range filters.ServiceNames {
			if serviceName == allowedService {
				return true
			}
		}
		return false
	}

	// LIKE pattern matching (SQL-style % and _ wildcards)
	if filters.ServiceNameLike != "" {
		return fm.matchesLikePattern(serviceName, filters.ServiceNameLike)
	}

	// Regex pattern matching
	if filters.ServiceNameRegex != "" {
		return fm.matchesRegexPattern(serviceName, filters.ServiceNameRegex)
	}

	// Prefix matching (optimized)
	if filters.ServiceNamePrefix != "" {
		return strings.HasPrefix(serviceName, filters.ServiceNamePrefix)
	}

	// Suffix matching
	if filters.ServiceNameSuffix != "" {
		return strings.HasSuffix(serviceName, filters.ServiceNameSuffix)
	}

	// Substring matching
	if filters.ServiceNameContains != "" {
		return strings.Contains(serviceName, filters.ServiceNameContains)
	}

	// No service filters specified - match all
	return true
}

// ExtractAttributesFromJSON safely extracts attributes from JSON fields
func (fm *FilterMatcher) ExtractAttributesFromJSON(jsonData []byte) map[string]interface{} {
	if len(jsonData) == 0 {
		return make(map[string]interface{})
	}

	var attributes map[string]interface{}
	if err := json.Unmarshal(jsonData, &attributes); err != nil {
		return make(map[string]interface{})
	}

	return attributes
}

// matchesLikePattern implements SQL LIKE pattern matching with % and _ wildcards
func (fm *FilterMatcher) matchesLikePattern(text, pattern string) bool {
	// Convert SQL LIKE pattern to regex
	// % matches any sequence of characters
	// _ matches any single character
	regexPattern := strings.ReplaceAll(pattern, "%", ".*")
	regexPattern = strings.ReplaceAll(regexPattern, "_", ".")
	regexPattern = "^" + regexPattern + "$"

	matched, err := regexp.MatchString(regexPattern, text)
	return err == nil && matched
}

// matchesRegexPattern implements regex pattern matching
func (fm *FilterMatcher) matchesRegexPattern(text, pattern string) bool {
	matched, err := regexp.MatchString(pattern, text)
	return err == nil && matched
}
