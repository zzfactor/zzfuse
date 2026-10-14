package repository

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Data models

// LogEntry and LogQuery definitions
// ... (copy from otel_logs_service.go)

// Remove duplicate declarations of LogEntry, LogQuery, TimeRange, parseCursor, encodeCursor, normalizeLogEntryJSONBFields if they are already defined elsewhere in the package.
// Only keep the repository interface, implementation, and unique DB helpers here.

// Repository interface

type LogRepository interface {
	QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, string, bool, error)
	InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error)
	GetLogByID(ctx context.Context, id int64, schema string) (LogEntry, error)
	CountLogs(ctx context.Context, q LogQuery) (int64, error)
}

// CountLogs returns the total number of rows matching the given filters.
// Note: cursor, direction and limit are ignored; regex q_mode is not supported here (the handler should reject it).
func (r *logRepository) CountLogs(ctx context.Context, q LogQuery) (int64, error) {
	schema := EnsureSchema(q.Schema)
	
	args := []interface{}{}
	where := "WHERE 1=1"
	idx := 1

	// Time range filtering
	if q.Start != nil {
		where += fmt.Sprintf(" AND timestamp_ns >= $%d", idx)
		args = append(args, q.Start.UnixNano())
		idx++
	}
	if q.End != nil {
		where += fmt.Sprintf(" AND timestamp_ns <= $%d", idx)
		args = append(args, q.End.UnixNano())
		idx++
	}

	// Service filtering using common builder
	serviceWhere, serviceArgs, newIdx := BuildServiceFiltersSQL(q.CommonServiceFilters, idx)
	where += serviceWhere
	args = append(args, serviceArgs...)
	idx = newIdx

	// Severity filters
	if q.Severity != "" {
		where += fmt.Sprintf(" AND LOWER(severity_text) = LOWER($%d)", idx)
		args = append(args, q.Severity)
		idx++
	}
	if q.SeverityMin != nil {
		where += fmt.Sprintf(" AND severity_number >= $%d", idx)
		args = append(args, *q.SeverityMin)
		idx++
	}
	if q.SeverityMax != nil {
		where += fmt.Sprintf(" AND severity_number <= $%d", idx)
		args = append(args, *q.SeverityMax)
		idx++
	}
	if len(q.SeverityGroups) > 0 {
		groupClauses := make([]string, 0, len(q.SeverityGroups))
		for _, g := range q.SeverityGroups {
			switch strings.ToUpper(g) {
			case "TRACE":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 1, 4)
				idx += 2
			case "DEBUG":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 5, 8)
				idx += 2
			case "INFO":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 9, 12)
				idx += 2
			case "WARN":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 13, 16)
				idx += 2
			case "ERROR":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 17, 20)
				idx += 2
			case "FATAL":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 21, 24)
				idx += 2
			}
		}
		if len(groupClauses) > 0 {
			where += " AND (" + strings.Join(groupClauses, " OR ") + ")"
		}
	}
	if len(q.SeverityTextIn) > 0 {
		placeholders := make([]string, 0, len(q.SeverityTextIn))
		for range q.SeverityTextIn {
			placeholders = append(placeholders, fmt.Sprintf("LOWER($%d)", idx))
			idx++
		}
		where += " AND LOWER(severity_text) IN (" + strings.Join(placeholders, ",") + ")"
		for _, s := range q.SeverityTextIn {
			args = append(args, s)
		}
	}

	// Advanced filtering using common builder
	advancedWhere, advancedArgs, newIdx := BuildAdvancedFiltersSQL(q.CommonAdvancedFilters, idx)
	where += advancedWhere
	args = append(args, advancedArgs...)
	idx = newIdx

	// Text search on body JSONB: support both ILIKE and regex
	if strings.TrimSpace(q.Q) != "" {
		if strings.ToLower(q.QMode) == "regex" {
			// Build inline flags prefix similar to QueryLogs (supports i,m)
			prefix := ""
			for _, f := range q.QFlags {
				if f == "i" {
					prefix += "(?i)"
				}
				if f == "m" {
					prefix += "(?m)"
				}
			}
			where += fmt.Sprintf(" AND body::text ~ $%d", idx)
			args = append(args, prefix+q.Q)
			idx++
		} else {
			where += fmt.Sprintf(" AND body::text ILIKE $%d", idx)
			args = append(args, "%"+q.Q+"%")
			idx++
		}
	}

	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s.logs %s`, schema, where)
	var count int64
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count logs: %w", err)
	}
	return count, nil
}

// Implementation

type logRepository struct {
	db *sql.DB
}

// NewLogRepository creates a new log repository
func NewLogRepository(db *sql.DB) LogRepository {
	return &logRepository{db: db}
}

// Helper functions for cursor encoding/decoding, normalization, etc.

// Remove duplicate declarations of LogEntry, LogQuery, TimeRange, parseCursor, encodeCursor, normalizeLogEntryJSONBFields if they are already defined elsewhere in the package.
// Only keep the repository interface, implementation, and unique DB helpers here.

// QueryLogs implementation (returns entries, nextCursor, hasMore, prevCursor, hasPrev, error)
func (r *logRepository) QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, string, bool, error) {
	schema := EnsureSchema(q.Schema)
	
	args := []interface{}{}
	where := "WHERE 1=1"
	idx := 1

	// Time range filtering (only apply if no cursor is present)
	if q.Cursor == "" {
		if q.Start != nil {
			where += fmt.Sprintf(" AND timestamp_ns >= $%d", idx)
			args = append(args, q.Start.UnixNano())
			idx++
		}
		if q.End != nil {
			where += fmt.Sprintf(" AND timestamp_ns <= $%d", idx)
			args = append(args, q.End.UnixNano())
			idx++
		}
	}

	// Services: legacy single or service_in[]
	if q.ServiceName != "" {
		where += fmt.Sprintf(" AND service_name = $%d", idx)
		args = append(args, q.ServiceName)
		idx++
	} else if len(q.ServiceNames) > 0 {
		placeholders := make([]string, 0, len(q.ServiceNames))
		for range q.ServiceNames {
			placeholders = append(placeholders, fmt.Sprintf("$%d", idx))
			idx++
		}
		where += fmt.Sprintf(" AND service_name IN (%s)", strings.Join(placeholders, ","))
		for _, s := range q.ServiceNames {
			args = append(args, s)
		}
	}

	// Severity: legacy text, numeric ranges, groups, exact text list
	if q.Severity != "" {
		where += fmt.Sprintf(" AND LOWER(severity_text) = LOWER($%d)", idx)
		args = append(args, q.Severity)
		idx++
	}
	if q.SeverityMin != nil {
		where += fmt.Sprintf(" AND severity_number >= $%d", idx)
		args = append(args, *q.SeverityMin)
		idx++
	}
	if q.SeverityMax != nil {
		where += fmt.Sprintf(" AND severity_number <= $%d", idx)
		args = append(args, *q.SeverityMax)
		idx++
	}
	if len(q.SeverityGroups) > 0 {
		// Map groups to numeric ranges
		groupClauses := make([]string, 0, len(q.SeverityGroups))
		for _, g := range q.SeverityGroups {
			switch strings.ToUpper(g) {
			case "TRACE":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 1, 4)
				idx += 2
			case "DEBUG":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 5, 8)
				idx += 2
			case "INFO":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 9, 12)
				idx += 2
			case "WARN":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 13, 16)
				idx += 2
			case "ERROR":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 17, 20)
				idx += 2
			case "FATAL":
				groupClauses = append(groupClauses, fmt.Sprintf("(severity_number BETWEEN $%d AND $%d)", idx, idx+1))
				args = append(args, 21, 24)
				idx += 2
			}
		}
		if len(groupClauses) > 0 {
			where += " AND (" + strings.Join(groupClauses, " OR ") + ")"
		}
	}
	if len(q.SeverityTextIn) > 0 {
		placeholders := make([]string, 0, len(q.SeverityTextIn))
		for range q.SeverityTextIn {
			placeholders = append(placeholders, fmt.Sprintf("LOWER($%d)", idx))
			idx++
		}
		where += " AND LOWER(severity_text) IN (" + strings.Join(placeholders, ",") + ")"
		for _, s := range q.SeverityTextIn {
			args = append(args, s)
		}
	}

	// Attributes: eq, in, exists for attributes and resource_attributes
	// attr.eq
	for key, val := range q.AttrEq {
		where += fmt.Sprintf(" AND attributes ->> $%d = $%d", idx, idx+1)
		args = append(args, key, val)
		idx += 2
	}
	// attr.in
	for key, list := range q.AttrIn {
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
	// attr.exists
	for key, exists := range q.AttrExists {
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
	// res_attr.eq
	for key, val := range q.ResAttrEq {
		where += fmt.Sprintf(" AND resource_attributes ->> $%d = $%d", idx, idx+1)
		args = append(args, key, val)
		idx += 2
	}
	// res_attr.in
	for key, list := range q.ResAttrIn {
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
	// res_attr.exists
	for key, exists := range q.ResAttrExists {
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

	// Simple text search on body JSONB
	if strings.TrimSpace(q.Q) != "" && strings.ToLower(q.QMode) != "regex" {
		where += fmt.Sprintf(" AND body::text ILIKE $%d", idx)
		args = append(args, "%"+q.Q+"%")
		idx++
	}

	// Cursor pagination: direction-aware (use timestamp_ns for tighter pruning)
	if q.Cursor != "" {
		cursorNS, cursorID, err := ParseCursor(q.Cursor)
		if err != nil {
			return nil, "", false, "", false, fmt.Errorf("invalid cursor: %w", err)
		}
		// Convert string cursorID back to int64 for database comparison
		cursorIDInt, err := strconv.ParseInt(cursorID, 10, 64)
		if err != nil {
			return nil, "", false, "", false, fmt.Errorf("invalid cursor ID: %w", err)
		}
		cmp := "<"
		if strings.ToLower(q.Direction) == "asc" {
			cmp = ">"
		}
		where += fmt.Sprintf(" AND (timestamp_ns, id) %s ($%d, $%d)", cmp, idx, idx+1)
		args = append(args, cursorNS, cursorIDInt)
		idx += 2
	}

	limit := 100
	if q.Limit > 0 {
		limit = q.Limit
	}
	orderDir := "DESC"
	if strings.ToLower(q.Direction) == "asc" {
		orderDir = "ASC"
	}
	// Build the base query
	query := fmt.Sprintf(`SELECT id, timestamp_ns, timestamp, observed_timestamp, trace_id, span_id, flags, severity_text, severity_number, service_name, service_namespace, service_instance_id, event_name, resource_attributes, resource_schema_url, scope_name, scope_version, scope_attributes, scope_dropped_attr_count, attributes, scope_schema_url, dropped_attr_count, body, metadata FROM %s.logs %s ORDER BY timestamp_ns %s, id %s`, schema, where, orderDir, orderDir)

	// Add LIMIT+1 for hasMore detection and OFFSET if specified
	if q.Offset > 0 {
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", idx, idx+1)
		args = append(args, limit+1, q.Offset)
	} else {
		query += fmt.Sprintf(" LIMIT $%d", idx)
		args = append(args, limit+1)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", false, "", false, fmt.Errorf("query logs: %w", err)
	}
	defer rows.Close()
	entries := make([]LogEntry, 0, limit)
	var hasMore bool
	var hasPrev bool

	for rows.Next() {
		var e LogEntry
		if len(entries) == limit { // enforce limit after regex filtering
			hasMore = true
			break
		}
		err := rows.Scan(
			&e.ID,
			&e.TimestampNS,
			&e.Timestamp,
			&e.ObservedTimestamp,
			&e.TraceID,
			&e.SpanID,
			&e.Flags,
			&e.SeverityText,
			&e.SeverityNumber,
			&e.ServiceName,
			&e.ServiceNamespace,
			&e.ServiceInstanceID,
			&e.EventName,
			&e.ResourceAttributes,
			&e.ResourceSchemaURL,
			&e.ScopeName,
			&e.ScopeVersion,
			&e.ScopeAttributes,
			&e.ScopeDroppedAttrCount,
			&e.Attributes,
			&e.ScopeSchemaURL,
			&e.DroppedAttrCount,
			&e.Body,
			&e.Metadata,
		)
		if err != nil {
			return nil, "", false, "", false, fmt.Errorf("scan log row: %w", err)
		}
		// If regex mode is enabled, filter here (body JSON text)
		if strings.TrimSpace(q.Q) != "" && strings.ToLower(q.QMode) == "regex" {
			pattern := q.Q
			flags := ""
			for _, f := range q.QFlags {
				flags += f
			}
			// Apply case-insensitive (?i) and multi-line (?m) as requested
			prefix := ""
			if strings.Contains(flags, "i") {
				prefix += "(?i)"
			}
			if strings.Contains(flags, "m") {
				prefix += "(?m)"
			}
			re, reErr := regexp.Compile(prefix + pattern)
			if reErr != nil {
				continue
			}
			if !re.MatchString(string(e.Body)) {
				continue
			}
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, "", false, fmt.Errorf("rows error: %w", err)
	}
	var nextCursor string
	var prevCursor string

	// Use shared cursor generation utility
	nextCursor, prevCursor, hasPrev = generateCursors(entries, hasMore, q.Cursor != "")
	return entries, nextCursor, hasMore, prevCursor, hasPrev, nil
}

// InsertLogs implementation (bulk insert)
func (r *logRepository) InsertLogs(ctx context.Context, entries []LogEntry, schemaName string) ([]string, error) {
	return InsertLogEntries(ctx, r.db, entries, schemaName)
}

// GetLogByID returns a single log entry by ID
func (r *logRepository) GetLogByID(ctx context.Context, id int64, schema string) (LogEntry, error) {
	schema = EnsureSchema(schema)
	// Reuse the same column order as in QueryLogs
	query := fmt.Sprintf(`SELECT id, timestamp_ns, timestamp, observed_timestamp, trace_id, span_id, flags, severity_text, severity_number, service_name, service_namespace, service_instance_id, event_name, resource_attributes, resource_schema_url, scope_name, scope_version, scope_attributes, scope_dropped_attr_count, attributes, scope_schema_url, dropped_attr_count, body, metadata FROM %s.logs WHERE id = $1 LIMIT 1`, schema)
	var e LogEntry
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&e.ID,
		&e.TimestampNS,
		&e.Timestamp,
		&e.ObservedTimestamp,
		&e.TraceID,
		&e.SpanID,
		&e.Flags,
		&e.SeverityText,
		&e.SeverityNumber,
		&e.ServiceName,
		&e.ServiceNamespace,
		&e.ServiceInstanceID,
		&e.EventName,
		&e.ResourceAttributes,
		&e.ResourceSchemaURL,
		&e.ScopeName,
		&e.ScopeVersion,
		&e.ScopeAttributes,
		&e.ScopeDroppedAttrCount,
		&e.Attributes,
		&e.ScopeSchemaURL,
		&e.DroppedAttrCount,
		&e.Body,
		&e.Metadata,
	)
	if err != nil {
		return LogEntry{}, err
	}
	return e, nil
}
