package template

import (
	"fmt"
	"strings"
)

// SQLBuilder provides optimized SQL generation with string builders and pre-computed mappings
type SQLBuilder struct {
	fieldMapping *FieldMapping
	// Pre-computed string replacements for performance
	underscoreToDot map[string]string
	dotToUnderscore map[string]string
}

// NewSQLBuilder creates a new SQL builder
func NewSQLBuilder(fieldMapping *FieldMapping) *SQLBuilder {
	return &SQLBuilder{
		fieldMapping:    fieldMapping,
		underscoreToDot: make(map[string]string),
		dotToUnderscore: make(map[string]string),
	}
}

// BuildSelectClause builds an optimized SQL SELECT clause
func (sb *SQLBuilder) BuildSelectClause(optimizedList *OptimizedFieldList) string {
	var builder strings.Builder

	// Pre-allocate capacity for better performance
	estimatedCapacity := len(optimizedList.StaticFields) +
		len(optimizedList.FullJSONBFields) +
		len(optimizedList.JSONBFields)*2 // Estimate 2 fields per JSONB field
	builder.Grow(estimatedCapacity * 20) // Estimate 20 chars per field

	// Add static fields
	for i, field := range optimizedList.StaticFields {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(field)
	}

	// Add full JSONB fields
	for _, field := range optimizedList.FullJSONBFields {
		if builder.Len() > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(field)
	}

	// Add specific JSONB field extractions with optimized string operations
	for baseField, keys := range optimizedList.JSONBFields {
		for _, key := range keys {
			if builder.Len() > 0 {
				builder.WriteString(", ")
			}

			// Use pre-computed mappings for better performance
			dbKey := sb.getUnderscoreToDot(key)
			alias := sb.buildAlias(baseField, key)

			// Build COALESCE clause efficiently
			builder.WriteString("COALESCE(")
			builder.WriteString(baseField)
			builder.WriteString("->>'")
			builder.WriteString(dbKey)
			builder.WriteString("', '') as ")
			builder.WriteString(alias)
		}
	}

	return builder.String()
}

// getUnderscoreToDot converts underscores to dots with caching
func (sb *SQLBuilder) getUnderscoreToDot(key string) string {
	if cached, exists := sb.underscoreToDot[key]; exists {
		return cached
	}

	result := strings.ReplaceAll(key, "_", ".")
	sb.underscoreToDot[key] = result
	return result
}

// buildAlias builds an alias efficiently
func (sb *SQLBuilder) buildAlias(baseField, key string) string {
	aliasKey := baseField + "_" + key
	if cached, exists := sb.dotToUnderscore[aliasKey]; exists {
		return cached
	}

	result := baseField + "_" + strings.ReplaceAll(key, ".", "_")
	sb.dotToUnderscore[aliasKey] = result
	return result
}

// BuildWhereClause builds an optimized WHERE clause
func (sb *SQLBuilder) BuildWhereClause(conditions map[string]interface{}) string {
	if len(conditions) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(conditions) * 30) // Estimate 30 chars per condition

	first := true
	for field, value := range conditions {
		if !first {
			builder.WriteString(" AND ")
		}
		first = false

		if sb.fieldMapping.IsJSONBField(field) {
			// Handle JSONB field
			baseField, specificKey := sb.fieldMapping.ParseJSONBField(field)
			if specificKey != "" {
				// Specific JSONB field
				dbKey := sb.getUnderscoreToDot(specificKey)
				builder.WriteString(baseField)
				builder.WriteString("->>'")
				builder.WriteString(dbKey)
				builder.WriteString("' = '")
				builder.WriteString(fmt.Sprintf("%v", value))
				builder.WriteString("'")
			} else {
				// Full JSONB field
				builder.WriteString(baseField)
				builder.WriteString(" @> '")
				builder.WriteString(fmt.Sprintf("%v", value))
				builder.WriteString("'::jsonb")
			}
		} else {
			// Regular field
			builder.WriteString(field)
			builder.WriteString(" = '")
			builder.WriteString(fmt.Sprintf("%v", value))
			builder.WriteString("'")
		}
	}

	return builder.String()
}

// BuildOrderByClause builds an optimized ORDER BY clause
func (sb *SQLBuilder) BuildOrderByClause(fields []string, directions []string) string {
	if len(fields) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(fields) * 15) // Estimate 15 chars per field

	for i, field := range fields {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(field)

		if i < len(directions) && directions[i] != "" {
			builder.WriteString(" ")
			builder.WriteString(strings.ToUpper(directions[i]))
		}
	}

	return builder.String()
}

// BuildLimitOffsetClause builds LIMIT and OFFSET clauses
func (sb *SQLBuilder) BuildLimitOffsetClause(limit, offset int) string {
	if limit <= 0 && offset <= 0 {
		return ""
	}

	var builder strings.Builder
	builder.Grow(50) // Estimate capacity

	if limit > 0 {
		builder.WriteString(" LIMIT ")
		builder.WriteString(fmt.Sprintf("%d", limit))
	}

	if offset > 0 {
		builder.WriteString(" OFFSET ")
		builder.WriteString(fmt.Sprintf("%d", offset))
	}

	return builder.String()
}

// BuildFullQuery builds a complete optimized SQL query
func (sb *SQLBuilder) BuildFullQuery(tableName string, optimizedList *OptimizedFieldList, conditions map[string]interface{}, orderBy []string, directions []string, limit, offset int) string {
	var builder strings.Builder
	builder.Grow(200) // Estimate initial capacity

	// SELECT clause
	builder.WriteString("SELECT ")
	builder.WriteString(sb.BuildSelectClause(optimizedList))

	// FROM clause
	builder.WriteString(" FROM ")
	builder.WriteString(tableName)

	// WHERE clause
	if whereClause := sb.BuildWhereClause(conditions); whereClause != "" {
		builder.WriteString(" WHERE ")
		builder.WriteString(whereClause)
	}

	// ORDER BY clause
	if orderByClause := sb.BuildOrderByClause(orderBy, directions); orderByClause != "" {
		builder.WriteString(" ORDER BY ")
		builder.WriteString(orderByClause)
	}

	// LIMIT/OFFSET clause
	if limitOffsetClause := sb.BuildLimitOffsetClause(limit, offset); limitOffsetClause != "" {
		builder.WriteString(limitOffsetClause)
	}

	return builder.String()
}

// ClearCache clears the string replacement caches
func (sb *SQLBuilder) ClearCache() {
	sb.underscoreToDot = make(map[string]string)
	sb.dotToUnderscore = make(map[string]string)
}

// GetCacheStats returns cache statistics
func (sb *SQLBuilder) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"underscore_to_dot_cache_size": len(sb.underscoreToDot),
		"dot_to_underscore_cache_size": len(sb.dotToUnderscore),
	}
}
