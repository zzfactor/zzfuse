package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	// Embed the DDL
	_ "embed"
)

//go:embed ddl/runs.sql
var runsDDL string

// RunMetadata represents the structured metadata for a run
type RunMetadata struct {
	SpanCount       int64 `json:"SPAN_COUNT,omitempty"`
	SpanEventCount  int64 `json:"SPAN_EVENT_COUNT,omitempty"`
	LogCount        int64 `json:"LOG_COUNT,omitempty"`
	LogInfoCount    int64 `json:"LOG_INFO_COUNT,omitempty"`
	LogWarningCount int64 `json:"LOG_WARNING_COUNT,omitempty"`
	LogDebugCount   int64 `json:"LOG_DEBUG_COUNT,omitempty"`
	LogErrorCount   int64 `json:"LOG_ERROR_COUNT,omitempty"`
	SpanErrorCount  int64 `json:"SPAN_ERROR_COUNT,omitempty"`
}

// NewRunMetadata creates a new RunMetadata with default values
func NewRunMetadata() *RunMetadata {
	return &RunMetadata{}
}

// ToMap converts RunMetadata to a map[string]interface{} for database storage
func (rm *RunMetadata) ToMap() map[string]interface{} {
	result := make(map[string]interface{})

	if rm.SpanCount > 0 {
		result["SPAN_COUNT"] = rm.SpanCount
	}
	if rm.SpanEventCount > 0 {
		result["SPAN_EVENT_COUNT"] = rm.SpanEventCount
	}
	if rm.LogCount > 0 {
		result["LOG_COUNT"] = rm.LogCount
	}
	if rm.LogInfoCount > 0 {
		result["LOG_INFO_COUNT"] = rm.LogInfoCount
	}
	if rm.LogWarningCount > 0 {
		result["LOG_WARNING_COUNT"] = rm.LogWarningCount
	}
	if rm.LogDebugCount > 0 {
		result["LOG_DEBUG_COUNT"] = rm.LogDebugCount
	}
	if rm.LogErrorCount > 0 {
		result["LOG_ERROR_COUNT"] = rm.LogErrorCount
	}
	if rm.SpanErrorCount > 0 {
		result["SPAN_ERROR_COUNT"] = rm.SpanErrorCount
	}

	return result
}

// FromMap creates RunMetadata from a map[string]interface{} from database
func FromMap(data map[string]interface{}) *RunMetadata {
	rm := NewRunMetadata()

	if val, ok := data["SPAN_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.SpanCount = int64(count)
		}
	}
	if val, ok := data["SPAN_EVENT_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.SpanEventCount = int64(count)
		}
	}
	if val, ok := data["LOG_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.LogCount = int64(count)
		}
	}
	if val, ok := data["LOG_INFO_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.LogInfoCount = int64(count)
		}
	}
	if val, ok := data["LOG_WARNING_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.LogWarningCount = int64(count)
		}
	}
	if val, ok := data["LOG_DEBUG_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.LogDebugCount = int64(count)
		}
	}
	if val, ok := data["LOG_ERROR_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.LogErrorCount = int64(count)
		}
	}
	if val, ok := data["SPAN_ERROR_COUNT"]; ok {
		if count, ok := val.(float64); ok {
			rm.SpanErrorCount = int64(count)
		}
	}

	return rm
}

// Run represents a record in the runs table
type Run struct {
	ID               int64                  `db:"id" json:"id"`
	SessionID        *string                `db:"session_id" json:"session_id,omitempty"`
	ServiceName      string                 `db:"service_name" json:"service_name"`
	ScopeName        *string                `db:"scope_name" json:"scope_name"`
	Name             string                 `db:"name" json:"name"`
	StartTimestampNs int64                  `db:"start_timestamp_ns" json:"start_timestamp_ns"`
	EndTimestampNs   *int64                 `db:"end_timestamp_ns" json:"end_timestamp_ns"`
	TraceID          *string                `db:"trace_id" json:"trace_id"`
	ProcessorID      *int64                 `db:"processor_id" json:"processor_id"`
	RunType          *int16                 `db:"run_type" json:"run_type"`
	Metadata         map[string]interface{} `db:"metadata" json:"metadata"`
	CreatedAt        time.Time              `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time              `db:"updated_at" json:"updated_at"`
}

// GetStructuredMetadata returns the metadata as a structured RunMetadata object
func (r *Run) GetStructuredMetadata() *RunMetadata {
	return FromMap(r.Metadata)
}

// SetStructuredMetadata sets the metadata from a structured RunMetadata object
func (r *Run) SetStructuredMetadata(metadata *RunMetadata) {
	r.Metadata = metadata.ToMap()
}

// NewRun creates a new Run with the provided parameters
func NewRun(serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) *Run {
	return &Run{
		ServiceName:      serviceName,
		ScopeName:        scopeName,
		Name:             name,
		StartTimestampNs: startTimestampNs,
		EndTimestampNs:   endTimestampNs,
		TraceID:          traceID,
		ProcessorID:      processorID,
		RunType:          runType,
		Metadata:         metadata,
	}
}

// NewRunWithSessionID creates a new Run with session ID
func NewRunWithSessionID(sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) *Run {
	return &Run{
		SessionID:        sessionID,
		ServiceName:      serviceName,
		ScopeName:        scopeName,
		Name:             name,
		StartTimestampNs: startTimestampNs,
		EndTimestampNs:   endTimestampNs,
		TraceID:          traceID,
		ProcessorID:      processorID,
		RunType:          runType,
		Metadata:         metadata,
	}
}

// NewRunWithStructuredMetadata creates a new Run with structured metadata
func NewRunWithStructuredMetadata(serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata *RunMetadata) *Run {
	return &Run{
		ServiceName:      serviceName,
		ScopeName:        scopeName,
		Name:             name,
		StartTimestampNs: startTimestampNs,
		EndTimestampNs:   endTimestampNs,
		TraceID:          traceID,
		ProcessorID:      processorID,
		RunType:          runType,
		Metadata:         metadata.ToMap(),
	}
}

// NewRunWithSessionIDAndStructuredMetadata creates a new Run with session ID and structured metadata
func NewRunWithSessionIDAndStructuredMetadata(sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata *RunMetadata) *Run {
	return &Run{
		SessionID:        sessionID,
		ServiceName:      serviceName,
		ScopeName:        scopeName,
		Name:             name,
		StartTimestampNs: startTimestampNs,
		EndTimestampNs:   endTimestampNs,
		TraceID:          traceID,
		ProcessorID:      processorID,
		RunType:          runType,
		Metadata:         metadata.ToMap(),
	}
}

// RunRepository provides methods to interact with the runs table
type RunRepository struct {
	DB     *sql.DB
	Schema string
}

// NewRunRepository creates a new repository instance
func NewRunRepository(ctx context.Context, db *sql.DB, schema string) (*RunRepository, error) {
	repo := &RunRepository{DB: db, Schema: schema}
	if err := repo.EnsureTable(ctx, schema); err != nil {
		return nil, fmt.Errorf("failed to ensure runs table: %w", err)
	}
	return repo, nil
}

// EnsureTable creates the runs table if it doesn't exist
func (r *RunRepository) EnsureTable(ctx context.Context, schema string) error {
	// First, check if schema exists to avoid DDL execution on non-existent schemas
	var exists bool
	checkSchemaQuery := `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`
	if err := r.DB.QueryRowContext(ctx, checkSchemaQuery, schema).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check schema existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("schema '%s' does not exist", schema)
	}

	// Replace schema placeholder in DDL
	ddl := strings.ReplaceAll(runsDDL, "{{.Schema}}", schema)

	// Execute DDL
	_, err := r.DB.ExecContext(ctx, ddl)
	if err != nil {
		return fmt.Errorf("failed to create runs table: %w", err)
	}

	return nil
}

// GetByID returns the run with the specified ID
func (r *RunRepository) GetByID(ctx context.Context, id int64) (*Run, error) {
	query := fmt.Sprintf(`SELECT id, session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, 
		metadata, created_at, updated_at 
		FROM %s.runs WHERE id = $1`, r.Schema)

	row := r.DB.QueryRowContext(ctx, query, id)
	return r.scanRunFromSingleRow(row)
}

// GetActiveIdByName returns the ID of the most recent run with that name
func (r *RunRepository) GetActiveIdByName(ctx context.Context, name string) (int64, error) {
	query := fmt.Sprintf(`SELECT id FROM %s.runs WHERE name = $1 ORDER BY start_timestamp_ns DESC LIMIT 1`, r.Schema)

	var id int64
	err := r.DB.QueryRowContext(ctx, query, name).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get active run ID by name: %w", err)
	}
	return id, nil
}

// GetCount returns the count of runs with that name
func (r *RunRepository) GetCount(ctx context.Context, name string) (int64, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s.runs WHERE name = $1`, r.Schema)

	var count int64
	err := r.DB.QueryRowContext(ctx, query, name).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get run count: %w", err)
	}
	return count, nil
}

// GetAllIds returns all run IDs with that name
func (r *RunRepository) GetAllIds(ctx context.Context, name string) ([]int64, error) {
	query := fmt.Sprintf(`SELECT id FROM %s.runs WHERE name = $1 ORDER BY start_timestamp_ns DESC`, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get run IDs: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan run ID: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return ids, nil
}

// Create creates a new run record
func (r *RunRepository) Create(ctx context.Context, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) (int64, error) {
	query := fmt.Sprintf(`INSERT INTO %s.runs (session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, metadata) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`, r.Schema)

	var id int64
	// Default to empty string for session_id
	sessionID := ""
	err := r.DB.QueryRowContext(ctx, query, sessionID, serviceName, scopeName, name, startTimestampNs, endTimestampNs, traceID, processorID, runType, metadata).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create run: %w", err)
	}
	return id, nil
}

// CreateWithSessionID creates a new run record with session ID
func (r *RunRepository) CreateWithSessionID(ctx context.Context, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) (int64, error) {
	query := fmt.Sprintf(`INSERT INTO %s.runs (session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, metadata) 
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING id`, r.Schema)

	var id int64
	err := r.DB.QueryRowContext(ctx, query, sessionID, serviceName, scopeName, name, startTimestampNs, endTimestampNs, traceID, processorID, runType, metadata).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to create run: %w", err)
	}
	return id, nil
}

// Update updates an existing run record
func (r *RunRepository) Update(ctx context.Context, id int64, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) error {
	query := fmt.Sprintf(`UPDATE %s.runs SET service_name = $1, scope_name = $2, name = $3, start_timestamp_ns = $4, end_timestamp_ns = $5, trace_id = $6, processor_id = $7, run_type = $8, metadata = $9 
		WHERE id = $10`, r.Schema)

	result, err := r.DB.ExecContext(ctx, query, serviceName, scopeName, name, startTimestampNs, endTimestampNs, traceID, processorID, runType, metadata, id)
	if err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no run found with ID %d", id)
	}

	return nil
}

// GetBySessionID returns all runs for a specific session ID
func (r *RunRepository) GetBySessionID(ctx context.Context, sessionID string) ([]*Run, error) {
	query := fmt.Sprintf(`SELECT id, session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, 
		metadata, created_at, updated_at 
		FROM %s.runs WHERE session_id = $1 ORDER BY start_timestamp_ns DESC`, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get runs by session ID: %w", err)
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		run, err := r.scanRunFromRow(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return runs, nil
}

// UpdateWithSessionID updates an existing run record including session ID
func (r *RunRepository) UpdateWithSessionID(ctx context.Context, id int64, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) error {
	query := fmt.Sprintf(`UPDATE %s.runs SET session_id = $1, service_name = $2, scope_name = $3, name = $4, start_timestamp_ns = $5, end_timestamp_ns = $6, trace_id = $7, processor_id = $8, run_type = $9, metadata = $10 
		WHERE id = $11`, r.Schema)

	result, err := r.DB.ExecContext(ctx, query, sessionID, serviceName, scopeName, name, startTimestampNs, endTimestampNs, traceID, processorID, runType, metadata, id)
	if err != nil {
		return fmt.Errorf("failed to update run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no run found with ID %d", id)
	}

	return nil
}

// Delete deletes a run record by ID
func (r *RunRepository) Delete(ctx context.Context, id int64) error {
	query := fmt.Sprintf(`DELETE FROM %s.runs WHERE id = $1`, r.Schema)

	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no run found with ID %d", id)
	}

	return nil
}

// GetByProcessorID returns all runs for a specific processor
func (r *RunRepository) GetByProcessorID(ctx context.Context, processorID int64) ([]*Run, error) {
	query := fmt.Sprintf(`SELECT id, session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, 
		metadata, created_at, updated_at 
		FROM %s.runs WHERE processor_id = $1 ORDER BY start_timestamp_ns DESC`, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, processorID)
	if err != nil {
		return nil, fmt.Errorf("failed to get runs by processor ID: %w", err)
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		run, err := r.scanRunFromRow(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return runs, nil
}

// GetByTraceID returns a run by its trace ID
func (r *RunRepository) GetByTraceID(ctx context.Context, traceID string) (*Run, error) {
	query := fmt.Sprintf(`SELECT id, session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, 
		metadata, created_at, updated_at 
		FROM %s.runs WHERE trace_id = $1`, r.Schema)

	row := r.DB.QueryRowContext(ctx, query, traceID)
	return r.scanRunFromSingleRow(row)
}

// EndRun ends a run by setting end_timestamp_ns
func (r *RunRepository) EndRun(ctx context.Context, id int64, endTimestampNs int64) error {
	query := fmt.Sprintf(`UPDATE %s.runs SET end_timestamp_ns = $1 WHERE id = $2`, r.Schema)

	result, err := r.DB.ExecContext(ctx, query, endTimestampNs, id)
	if err != nil {
		return fmt.Errorf("failed to end run: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no run found with ID %d", id)
	}

	return nil
}

// CreateWithStructuredMetadata creates a new run record with structured metadata
func (r *RunRepository) CreateWithStructuredMetadata(ctx context.Context, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata *RunMetadata) (int64, error) {
	return r.Create(ctx, serviceName, scopeName, name, startTimestampNs, endTimestampNs, traceID, processorID, runType, metadata.ToMap())
}

// UpdateWithStructuredMetadata updates an existing run record with structured metadata
func (r *RunRepository) UpdateWithStructuredMetadata(ctx context.Context, id int64, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata *RunMetadata) error {
	return r.Update(ctx, id, serviceName, scopeName, name, startTimestampNs, endTimestampNs, traceID, processorID, runType, metadata.ToMap())
}

// UpdateMetadataField updates a specific metadata field for a run
func (r *RunRepository) UpdateMetadataField(ctx context.Context, id int64, field string, value interface{}) error {
	query := fmt.Sprintf(`UPDATE %s.runs SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{%s}', $1::jsonb) WHERE id = $2`, r.Schema, field)

	result, err := r.DB.ExecContext(ctx, query, value, id)
	if err != nil {
		return fmt.Errorf("failed to update metadata field %s: %w", field, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no run found with ID %d", id)
	}

	return nil
}

// IncrementMetadataField increments a specific metadata field for a run
func (r *RunRepository) IncrementMetadataField(ctx context.Context, id int64, field string, increment int64) error {
	query := fmt.Sprintf(`UPDATE %s.runs SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{%s}', COALESCE((metadata->>'%s')::bigint, 0) + $1) WHERE id = $2`, r.Schema, field, field)

	result, err := r.DB.ExecContext(ctx, query, increment, id)
	if err != nil {
		return fmt.Errorf("failed to increment metadata field %s: %w", field, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no run found with ID %d", id)
	}

	return nil
}

// UpdateStructuredMetadata updates the metadata for a run using structured metadata
func (r *RunRepository) UpdateStructuredMetadata(ctx context.Context, id int64, metadata *RunMetadata) error {
	query := fmt.Sprintf(`UPDATE %s.runs SET metadata = $1 WHERE id = $2`, r.Schema)

	result, err := r.DB.ExecContext(ctx, query, metadata.ToMap(), id)
	if err != nil {
		return fmt.Errorf("failed to update structured metadata: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no run found with ID %d", id)
	}

	return nil
}

// GetStructuredMetadata returns the structured metadata for a run
func (r *RunRepository) GetStructuredMetadata(ctx context.Context, id int64) (*RunMetadata, error) {
	run, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("no run found with ID %d", id)
	}
	return run.GetStructuredMetadata(), nil
}

// GetActiveRunsByServiceName returns all active runs for a specific service
func (r *RunRepository) GetActiveRunsByServiceName(ctx context.Context, serviceName string) ([]*Run, error) {
	query := fmt.Sprintf(`SELECT id, session_id, service_name, scope_name, name, start_timestamp_ns, end_timestamp_ns, trace_id, processor_id, run_type, 
		metadata, created_at, updated_at 
		FROM %s.runs WHERE service_name = $1 AND end_timestamp_ns IS NULL ORDER BY start_timestamp_ns DESC`, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get active runs by service name: %w", err)
	}
	defer rows.Close()

	var runs []*Run
	for rows.Next() {
		run, err := r.scanRunFromRow(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return runs, nil
}

// scanRunFromRow scans a run from a database row, handling JSONB metadata conversion
func (r *RunRepository) scanRunFromRow(rows *sql.Rows) (*Run, error) {
	var run Run
	var metadataBytes []byte

	if err := rows.Scan(&run.ID, &run.SessionID, &run.ServiceName, &run.ScopeName, &run.Name, &run.StartTimestampNs, &run.EndTimestampNs, &run.TraceID, &run.ProcessorID, &run.RunType,
		&metadataBytes, &run.CreatedAt, &run.UpdatedAt); err != nil {
		return nil, fmt.Errorf("failed to scan run: %w", err)
	}

	// Convert JSONB bytes to map[string]interface{}
	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &run.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	} else {
		run.Metadata = make(map[string]interface{})
	}

	return &run, nil
}

// scanRunFromSingleRow scans a run from a single database row, handling JSONB metadata conversion
func (r *RunRepository) scanRunFromSingleRow(row *sql.Row) (*Run, error) {
	var run Run
	var metadataBytes []byte

	if err := row.Scan(&run.ID, &run.SessionID, &run.ServiceName, &run.ScopeName, &run.Name, &run.StartTimestampNs, &run.EndTimestampNs, &run.TraceID, &run.ProcessorID, &run.RunType,
		&metadataBytes, &run.CreatedAt, &run.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan run: %w", err)
	}

	// Convert JSONB bytes to map[string]interface{}
	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &run.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	} else {
		run.Metadata = make(map[string]interface{})
	}

	return &run, nil
}
