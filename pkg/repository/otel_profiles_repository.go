package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ProfileRepository interface {
	QueryProfiles(ctx context.Context, q ProfileQuery) ([]ProfileEntry, string, bool, error)
	InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error)
}

type profileRepository struct {
	db *sql.DB
}

func (r *profileRepository) QueryProfiles(ctx context.Context, q ProfileQuery) ([]ProfileEntry, string, bool, error) {
	args := []interface{}{}
	where := "WHERE 1=1"
	idx := 1

	// Time range filtering (only apply if no cursor is present)
	if q.Cursor == "" {
		where += " AND timestamp >= $1 AND timestamp <= $2"
		args = append(args, q.Start, q.End)
		idx = 3
	}

	if q.ServiceName != "" {
		where += fmt.Sprintf(" AND service_name = $%d", idx)
		args = append(args, q.ServiceName)
		idx++
	}
	if q.ProfileType != "" {
		where += fmt.Sprintf(" AND profile_type = $%d", idx)
		args = append(args, q.ProfileType)
		idx++
	}
	if q.Cursor != "" {
		cursorNS, cursorID, err := ParseCursor(q.Cursor)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor: %w", err)
		}
		// Convert string cursorID back to int64 for database comparison
		cursorIDInt, err := strconv.ParseInt(cursorID, 10, 64)
		if err != nil {
			return nil, "", false, fmt.Errorf("invalid cursor ID: %w", err)
		}
		if strings.ToLower(q.Direction) == "asc" {
			where += fmt.Sprintf(" AND (timestamp, id) > ($%d, $%d)", idx, idx+1)
		} else {
			where += fmt.Sprintf(" AND (timestamp, id) < ($%d, $%d)", idx, idx+1)
		}
		args = append(args, time.Unix(0, cursorNS), cursorIDInt)
		idx += 2
	}
	limit := 100
	if q.Limit > 0 {
		limit = q.Limit
	}
	dir := "DESC"
	if strings.ToLower(q.Direction) == "asc" {
		dir = "ASC"
	}
	schema := EnsureSchema(q.Schema)
	query := fmt.Sprintf(`SELECT id, timestamp_ns, timestamp, service_name, service_namespace, service_instance_id, profile_type, COALESCE(duration, 0), sample_type, sample_unit, profile_bytes, attributes, resource_attributes, resource_schema_url, scope_name, scope_version, scope_attributes, scope_dropped_attr_count, scope_schema_url, metadata, profile_storage_version, profile_encoding, profile_schema_version, otel_profile_id, profile_time_unix_nano, duration_nano FROM %s.profiles %s ORDER BY timestamp %s, id %s LIMIT $%d`, schema, where, dir, dir, idx)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("query profiles: %w", err)
	}
	defer rows.Close()
	entries := make([]ProfileEntry, 0, limit)
	var lastTS time.Time
	var lastID string
	for rows.Next() {
		var e ProfileEntry
		err := rows.Scan(
			&e.ID,
			&e.TimestampNS,
			&e.Timestamp,
			&e.ServiceName,
			&e.ServiceNamespace, &e.ServiceInstanceID,
			&e.ProfileType,
			&e.DurationNS,
			&e.SampleType,
			&e.SampleUnit,
			&e.ProfileBytes,
			&e.Attributes,
			&e.ResourceAttributes,
			&e.ResourceSchemaURL,
			&e.ScopeName,
			&e.ScopeVersion,
			&e.ScopeAttributes,
			&e.ScopeDroppedAttrCount,
			&e.ScopeSchemaURL,
			&e.Metadata,
			&e.ProfileStorageVersion, &e.ProfileEncoding, &e.ProfileSchemaVersion, &e.OtelProfileID, &e.ProfileTimeUnixNano, &e.DurationNano,
		)
		if err != nil {
			return nil, "", false, fmt.Errorf("scan profile row: %w", err)
		}
		entries = append(entries, e)
		lastTS = e.Timestamp
		lastID = fmt.Sprintf("%d", e.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("rows error: %w", err)
	}
	var nextCursor string
	var hasMore bool
	if len(entries) > 0 {
		// Always generate cursor from last entry for streaming continuity
		nextCursor = EncodeCursor(lastTS, lastID)
		hasMore = len(entries) == limit // Only set hasMore if we got full page
	}
	return entries, nextCursor, hasMore, nil
}

func (r *profileRepository) InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error) {
	return InsertProfileEntries(ctx, r.db, entries, schema)
}
