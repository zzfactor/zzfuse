package repository

import (
	"fmt"
	"strings"
)

// BuildServiceFiltersSQL builds SQL WHERE clauses and arguments for service filtering
func BuildServiceFiltersSQL(filters CommonServiceFilters, startIdx int) (string, []interface{}, int) {
	var where string
	var args []interface{}
	idx := startIdx

	// Services - exact match
	if filters.ServiceName != "" {
		where += fmt.Sprintf(" AND service_name = $%d", idx)
		args = append(args, filters.ServiceName)
		idx++
	} else if len(filters.ServiceNames) > 0 {
		placeholders := make([]string, 0, len(filters.ServiceNames))
		for range filters.ServiceNames {
			placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
			idx++
		}
		where += fmt.Sprintf(" AND service_name IN (%s)", strings.Join(placeholders, ","))
		for _, s := range filters.ServiceNames {
			args = append(args, s)
		}
	} else if filters.ServiceNameLike != "" {
		// SQL LIKE pattern matching
		where += fmt.Sprintf(" AND service_name LIKE $%d", idx)
		args = append(args, filters.ServiceNameLike)
		idx++
	} else if filters.ServiceNameRegex != "" {
		// PostgreSQL regex pattern matching
		where += fmt.Sprintf(" AND service_name ~ $%d", idx)
		args = append(args, filters.ServiceNameRegex)
		idx++
	} else if filters.ServiceNamePrefix != "" {
		// Prefix matching using LIKE
		where += fmt.Sprintf(" AND service_name LIKE $%d", idx)
		args = append(args, filters.ServiceNamePrefix+"%")
		idx++
	} else if filters.ServiceNameSuffix != "" {
		// Suffix matching using LIKE
		where += fmt.Sprintf(" AND service_name LIKE $%d", idx)
		args = append(args, "%"+filters.ServiceNameSuffix)
		idx++
	} else if filters.ServiceNameContains != "" {
		// Substring matching using LIKE
		where += fmt.Sprintf(" AND service_name LIKE $%d", idx)
		args = append(args, "%"+filters.ServiceNameContains+"%")
		idx++
	}

	return where, args, idx
}

// BuildAdvancedFiltersSQL builds SQL WHERE clauses and arguments for advanced attribute filtering
func BuildAdvancedFiltersSQL(filters CommonAdvancedFilters, startIdx int) (string, []interface{}, int) {
	var where string
	var args []interface{}
	idx := startIdx

	// Attributes
	for key, val := range filters.AttrEq {
		where += fmt.Sprintf(" AND attributes ->> $%d = $%d", idx, idx+1)
		args = append(args, key, val)
		idx += 2
	}
	for key, list := range filters.AttrIn {
		if len(list) == 0 {
			continue
		}
		ph := make([]string, 0, len(list))
		for range list {
			ph = append(ph, fmt.Sprintf("$%d", idx))
			idx++
		}
		where += fmt.Sprintf(" AND attributes ->> $%d IN (%s)", idx, strings.Join(ph, ","))
		args = append(args, key)
		for _, v := range list {
			args = append(args, v)
		}
		idx++
	}
	for key, exists := range filters.AttrExists {
		if exists {
			where += fmt.Sprintf(" AND attributes ? $%d", idx)
			args = append(args, key)
			idx++
		} else {
			where += fmt.Sprintf(" AND NOT (attributes ? $%d)", idx)
			args = append(args, key)
			idx++
		}
	}

	// Resource Attributes
	for key, val := range filters.ResAttrEq {
		where += fmt.Sprintf(" AND resource_attributes ->> $%d = $%d", idx, idx+1)
		args = append(args, key, val)
		idx += 2
	}
	for key, list := range filters.ResAttrIn {
		if len(list) == 0 {
			continue
		}
		ph := make([]string, 0, len(list))
		for range list {
			ph = append(ph, fmt.Sprintf("$%d", idx))
			idx++
		}
		where += fmt.Sprintf(" AND resource_attributes ->> $%d IN (%s)", idx, strings.Join(ph, ","))
		args = append(args, key)
		for _, v := range list {
			args = append(args, v)
		}
		idx++
	}
	for key, exists := range filters.ResAttrExists {
		if exists {
			where += fmt.Sprintf(" AND resource_attributes ? $%d", idx)
			args = append(args, key)
			idx++
		} else {
			where += fmt.Sprintf(" AND NOT (resource_attributes ? $%d)", idx)
			args = append(args, key)
			idx++
		}
	}

	return where, args, idx
}
