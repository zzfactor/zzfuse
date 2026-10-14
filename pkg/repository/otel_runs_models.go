package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RunQuery holds parameters for querying runs.
type RunQuery struct {
	TimeRange
	ServiceName      string
	Name             string
	SessionID        string
	RunType          *int16
	ActiveOnly       bool
	Limit            int
	Schema           string
	Cursor           string // for cursor-based pagination
	Direction        string // asc|desc, default desc
	DisplayDirection string // asc|desc, default direction, not used in database query, return results
}

// RunEntry represents a run record in the database.
type RunEntry struct {
	ID               int64                  // id BIGSERIAL
	SessionID        *string                // session_id TEXT
	ServiceName      string                 // service_name TEXT
	ScopeName        *string                // scope_name TEXT
	Name             string                 // name TEXT
	StartTimestampNs int64                  // start_timestamp_ns BIGINT
	EndTimestampNs   *int64                 // end_timestamp_ns BIGINT
	TraceID          *string                // trace_id TEXT
	ProcessorID      *int64                 // processor_id BIGINT
	RunType          *int16                 // run_type SMALLINT
	Metadata         map[string]interface{} // metadata JSONB
	CreatedAt        time.Time              // created_at TIMESTAMPTZ
	UpdatedAt        time.Time              // updated_at TIMESTAMPTZ
}

// SetMetadata sets the metadata field
func (r *RunEntry) SetMetadata(metadata map[string]interface{}) {
	r.Metadata = metadata
}

// GetMetadata returns the metadata field
func (r *RunEntry) GetMetadata() map[string]interface{} {
	return r.Metadata
}

// NormalizeRunEntryJSONBFields ensures all JSONB fields are properly initialized.
func NormalizeRunEntryJSONBFields(entry *RunEntry) {
	if entry.Metadata == nil {
		entry.Metadata = make(map[string]interface{})
	}
}

// RunRepository defines the interface for run database operations
type RunRepository interface {
	QueryRuns(ctx context.Context, q RunQuery) ([]RunEntry, string, bool, error)
	InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error)
	UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error
	EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error
	GetRunByID(ctx context.Context, id int64, schema string) (RunEntry, error)
}

// RunStreamRepository defines the interface for run streaming operations
type RunStreamRepository interface {
	SubscribeRuns(ctx context.Context, filter RunFilter) (<-chan RunEntryResponse, func(), <-chan struct{}, error)
}

// RunFilter holds parameters for filtering runs in streams
type RunFilter struct {
	CommonBaseFilters     // Includes Schema field for schema-based filtering
	CommonAdvancedFilters // Includes advanced filtering capabilities

	// Run-specific filters
	Name       string
	SessionID  string
	RunType    *int16
	ActiveOnly bool
}

// runRepository implements RunRepository
type runRepository struct {
	db *sql.DB
}

// NewRunRepository creates a new run repository
func NewRunRepository(db *sql.DB) RunRepository {
	return &runRepository{db: db}
}

// QueryRuns implements RunRepository.QueryRuns
func (r *runRepository) QueryRuns(ctx context.Context, q RunQuery) ([]RunEntry, string, bool, error) {
	args := []interface{}{}
	where := "WHERE 1=1"
	idx := 1

	if q.Start != nil {
		where += fmt.Sprintf(" AND start_timestamp_ns >= $%d", idx)
		args = append(args, q.Start.UnixNano())
		idx++
	}
	if q.End != nil {
		where += fmt.Sprintf(" AND start_timestamp_ns <= $%d", idx)
		args = append(args, q.End.UnixNano())
		idx++
	}
	if q.ServiceName != "" {
		where += fmt.Sprintf(" AND service_name = $%d", idx)
		args = append(args, q.ServiceName)
		idx++
	}
	if q.Name != "" {
		where += fmt.Sprintf(" AND name = $%d", idx)
		args = append(args, q.Name)
		idx++
	}
	if q.SessionID != "" {
		where += fmt.Sprintf(" AND session_id = $%d", idx)
		args = append(args, q.SessionID)
		idx++
	}
	if q.RunType != nil {
		where += fmt.Sprintf(" AND run_type = $%d", idx)
		args = append(args, *q.RunType)
		idx++
	}
	if q.ActiveOnly {
		where += " AND end_timestamp_ns IS NULL"
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
			where += fmt.Sprintf(" AND (start_timestamp_ns, id) > ($%d, $%d)", idx, idx+1)
		} else {
			where += fmt.Sprintf(" AND (start_timestamp_ns, id) < ($%d, $%d)", idx, idx+1)
		}
		args = append(args, cursorNS, cursorIDInt)
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
	query := fmt.Sprintf(`SELECT id, session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, metadata, created_at, updated_at FROM %s.runs %s ORDER BY start_timestamp_ns %s, id %s LIMIT $%d`, q.Schema, where, dir, dir, idx)
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", false, fmt.Errorf("query runs: %w", err)
	}
	defer rows.Close()

	entries := []RunEntry{}
	var lastTS time.Time
	var lastID int64

	for rows.Next() {
		var entry RunEntry
		var sessionID sql.NullString
		var scopeName sql.NullString
		var endTimestampNs sql.NullInt64
		var traceID sql.NullString
		var processorID sql.NullInt64
		var runType sql.NullInt16
		var metadataJSON []byte

		err := rows.Scan(
			&entry.ID,
			&sessionID,
			&entry.ServiceName,
			&scopeName,
			&entry.Name,
			&entry.StartTimestampNs,
			&endTimestampNs,
			&traceID,
			&processorID,
			&runType,
			&metadataJSON,
			&entry.CreatedAt,
			&entry.UpdatedAt,
		)
		if err != nil {
			return nil, "", false, fmt.Errorf("scan run: %w", err)
		}

		if sessionID.Valid {
			entry.SessionID = &sessionID.String
		}
		if scopeName.Valid {
			entry.ScopeName = &scopeName.String
		}
		if endTimestampNs.Valid {
			entry.EndTimestampNs = &endTimestampNs.Int64
		}
		if traceID.Valid {
			entry.TraceID = &traceID.String
		}
		if processorID.Valid {
			entry.ProcessorID = &processorID.Int64
		}
		if runType.Valid {
			entry.RunType = &runType.Int16
		}

		if len(metadataJSON) > 0 {
			if err := json.Unmarshal(metadataJSON, &entry.Metadata); err != nil {
				return nil, "", false, fmt.Errorf("unmarshal metadata: %w", err)
			}
		}

		lastTS = time.Unix(0, entry.StartTimestampNs)
		lastID = entry.ID
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, "", false, fmt.Errorf("rows error: %w", err)
	}

	hasMore := len(entries) == limit
	var nextCursor string
	if hasMore && len(entries) > 0 {
		nextCursor = EncodeCursor(lastTS, strconv.FormatInt(lastID, 10))
	}

	return entries, nextCursor, hasMore, nil
}

// InsertRuns implements RunRepository.InsertRuns
func (r *runRepository) InsertRuns(ctx context.Context, entries []RunEntry, schema string) ([]int64, error) {
	if len(entries) == 0 {
		return []int64{}, nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s.runs (session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, metadata) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`, schema)

	ids := make([]int64, 0, len(entries))
	for _, entry := range entries {
		NormalizeRunEntryJSONBFields(&entry)

		metadataJSON, err := json.Marshal(entry.Metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}

		var id int64
		err = tx.QueryRowContext(ctx, query,
			entry.SessionID,
			entry.ServiceName,
			entry.ScopeName,
			entry.Name,
			entry.StartTimestampNs,
			entry.EndTimestampNs,
			entry.TraceID,
			entry.ProcessorID,
			entry.RunType,
			metadataJSON,
		).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("insert run: %w", err)
		}
		ids = append(ids, id)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transaction: %w", err)
	}

	return ids, nil
}

// UpdateRunMetadata implements RunRepository.UpdateRunMetadata
func (r *runRepository) UpdateRunMetadata(ctx context.Context, runID int64, metadata map[string]interface{}, schema string) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := fmt.Sprintf(`UPDATE %s.runs SET metadata = $1, updated_at = NOW() WHERE id = $2`, schema)
	result, err := r.db.ExecContext(ctx, query, metadataJSON, runID)
	if err != nil {
		return fmt.Errorf("update run metadata: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("run not found: %d", runID)
	}

	return nil
}

// GetRunByID fetches a single run by its ID
func (r *runRepository) GetRunByID(ctx context.Context, id int64, schema string) (RunEntry, error) {
	query := fmt.Sprintf(`SELECT id, session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, metadata, created_at, updated_at FROM %s.runs WHERE id = $1`, schema)

	var entry RunEntry
	var sessionID sql.NullString
	var scopeName sql.NullString
	var endTimestampNs sql.NullInt64
	var traceID sql.NullString
	var processorID sql.NullInt64
	var runType sql.NullInt16
	var metadataJSON []byte

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&entry.ID,
		&sessionID,
		&entry.ServiceName,
		&scopeName,
		&entry.Name,
		&entry.StartTimestampNs,
		&endTimestampNs,
		&traceID,
		&processorID,
		&runType,
		&metadataJSON,
		&entry.CreatedAt,
		&entry.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return RunEntry{}, fmt.Errorf("run not found: %d", id)
		}
		return RunEntry{}, fmt.Errorf("get run by id: %w", err)
	}

	if sessionID.Valid {
		entry.SessionID = &sessionID.String
	}
	if scopeName.Valid {
		entry.ScopeName = &scopeName.String
	}
	if endTimestampNs.Valid {
		entry.EndTimestampNs = &endTimestampNs.Int64
	}
	if traceID.Valid {
		entry.TraceID = &traceID.String
	}
	if processorID.Valid {
		entry.ProcessorID = &processorID.Int64
	}
	if runType.Valid {
		entry.RunType = &runType.Int16
	}
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &entry.Metadata); err != nil {
			return RunEntry{}, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return entry, nil
}

// EndRun implements RunRepository.EndRun
func (r *runRepository) EndRun(ctx context.Context, name string, endTimestampNs int64, schema string) error {
	query := fmt.Sprintf(`UPDATE %s.runs SET end_timestamp_ns = $1, updated_at = NOW() WHERE name = $2 AND end_timestamp_ns IS NULL`, schema)
	result, err := r.db.ExecContext(ctx, query, endTimestampNs, name)
	if err != nil {
		return fmt.Errorf("end run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("run not found or already ended: %s", name)
	}

	return nil
}
