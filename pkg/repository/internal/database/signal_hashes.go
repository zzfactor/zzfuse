package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	// Embed the DDL
	_ "embed"
)

//go:embed ddl/signal_hashes.sql
var signalHashesDDL string

// Signal hash flags
const (
	SignalHashFlagNormal   = 0 // Normal signal
	SignalHashFlagBaseline = 1 // Baseline signal
	SignalHashFlagAnomaly  = 2 // Anomaly signal
)

// Signal types
const (
	SignalTypeLog     = 1
	SignalTypeTrace   = 2
	SignalTypeMetric  = 3
	SignalTypeProfile = 4
)

// SignalHash represents a record in the signal_hashes table
type SignalHash struct {
	TimestampNs int64  `db:"timestamp_ns" json:"timestamp_ns"`
	Hash        string `db:"hash" json:"hash"`
	RunID       int64  `db:"run_id" json:"run_id"`
	SignalType  int16  `db:"signal_type" json:"signal_type"`
	ID          int64  `db:"signal_id" json:"id"`
	Flags       int16  `db:"flags" json:"flags"`
}

// NewSignalHash creates a new SignalHash with the provided timestamp
func NewSignalHash(hash string, runID int64, signalType int16, id int64, flags int16, timestampNs int64) *SignalHash {
	return &SignalHash{
		TimestampNs: timestampNs,
		Hash:        hash,
		RunID:       runID,
		SignalType:  signalType,
		ID:          id,
		Flags:       flags,
	}
}

// SignalHashRepository provides methods to interact with the signal_hashes table
type SignalHashRepository struct {
	DB     *sql.DB
	Schema string
}

// NewSignalHashRepository creates a new repository instance
func NewSignalHashRepository(ctx context.Context, db *sql.DB, schema string) (*SignalHashRepository, error) {
	repo := &SignalHashRepository{DB: db, Schema: schema}
	if err := repo.EnsureTable(ctx, schema); err != nil {
		return nil, fmt.Errorf("failed to ensure signal_hashes table: %w", err)
	}
	return repo, nil
}

// Create inserts a new signal hash record
func (r *SignalHashRepository) Create(ctx context.Context, sh *SignalHash) error {
	query := fmt.Sprintf(`INSERT INTO %s.signal_hashes (timestamp_ns, hash, run_id, signal_type, signal_id, flags)
		VALUES ($1, $2, $3, $4, $5, $6)`, r.Schema)

	_, err := r.DB.ExecContext(ctx, query,
		sh.TimestampNs, sh.Hash, sh.RunID, sh.SignalType, sh.ID, sh.Flags,
	)
	return err
}

// GetByHash retrieves a signal hash by hash
func (r *SignalHashRepository) GetByHash(ctx context.Context, hash string) (*SignalHash, error) {
	if hash == "" {
		return nil, fmt.Errorf("hash cannot be empty")
	}

	query := fmt.Sprintf(`SELECT timestamp_ns, hash, run_id, signal_type, signal_id, flags 
		FROM %s.signal_hashes WHERE hash = $1`, r.Schema)

	row := r.DB.QueryRowContext(ctx, query, hash)
	var sh SignalHash
	if err := row.Scan(&sh.TimestampNs, &sh.Hash, &sh.RunID, &sh.SignalType, &sh.ID, &sh.Flags); err != nil {
		if err == sql.ErrNoRows || isTableNotExistError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get signal hash: %w", err)
	}
	return &sh, nil
}

// HashExists checks if a hash exists
func (r *SignalHashRepository) HashExists(ctx context.Context, hash string) (bool, error) {
	query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s.signal_hashes WHERE hash = $1)`, r.Schema)
	var exists bool
	err := r.DB.QueryRowContext(ctx, query, hash).Scan(&exists)
	if err != nil && isTableNotExistError(err) {
		return false, nil
	}
	return exists, err
}

// UpdateFlags updates the flags for a specific signal hash
func (r *SignalHashRepository) UpdateFlags(ctx context.Context, hash string, flags int16) error {
	query := fmt.Sprintf(`UPDATE %s.signal_hashes SET flags = $1 WHERE hash = $2`, r.Schema)
	_, err := r.DB.ExecContext(ctx, query, flags, hash)
	return err
}

// GetByTimeRange retrieves signal hashes within a time range
func (r *SignalHashRepository) GetByTimeRange(ctx context.Context, startTime, endTime int64) ([]*SignalHash, error) {
	query := fmt.Sprintf(`SELECT timestamp_ns, hash, run_id, signal_type, signal_id, flags 
		FROM %s.signal_hashes 
		WHERE timestamp_ns BETWEEN $1 AND $2
		ORDER BY timestamp_ns DESC`, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, startTime, endTime)
	if err != nil {
		if isTableNotExistError(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get signal hashes by time range: %w", err)
	}
	defer rows.Close()

	var hashes []*SignalHash
	for rows.Next() {
		var sh SignalHash
		if err := rows.Scan(&sh.TimestampNs, &sh.Hash, &sh.RunID, &sh.SignalType, &sh.ID, &sh.Flags); err != nil {
			return nil, err
		}
		hashes = append(hashes, &sh)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return hashes, nil
}

// GetBySignalType retrieves signal hashes by signal type
func (r *SignalHashRepository) GetBySignalType(ctx context.Context, signalType int16) ([]*SignalHash, error) {
	query := fmt.Sprintf(`SELECT timestamp_ns, hash, run_id, signal_type, signal_id, flags 
		FROM %s.signal_hashes WHERE signal_type = $1 ORDER BY timestamp_ns DESC`, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, signalType)
	if err != nil {
		if isTableNotExistError(err) {
			return []*SignalHash{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var hashes []*SignalHash
	for rows.Next() {
		var sh SignalHash
		if err := rows.Scan(&sh.TimestampNs, &sh.Hash, &sh.RunID, &sh.SignalType, &sh.ID, &sh.Flags); err != nil {
			return nil, err
		}
		hashes = append(hashes, &sh)
	}
	return hashes, nil
}

// CountByHash counts signal hashes for a hash
func (r *SignalHashRepository) CountByHash(ctx context.Context, hash string) (int64, error) {
	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s.signal_hashes WHERE hash = $1`, r.Schema)
	var count int64
	err := r.DB.QueryRowContext(ctx, query, hash).Scan(&count)
	if err != nil && isTableNotExistError(err) {
		return 0, nil
	}
	return count, err
}

// DeleteByTimeRange deletes signal hashes older than the specified timestamp
func (r *SignalHashRepository) DeleteByTimeRange(ctx context.Context, olderThan int64) error {
	query := fmt.Sprintf(`DELETE FROM %s.signal_hashes WHERE timestamp_ns < $1`, r.Schema)
	_, err := r.DB.ExecContext(ctx, query, olderThan)
	return err
}

// BatchCreate inserts multiple signal hashes in a single transaction
func (r *SignalHashRepository) BatchCreate(ctx context.Context, hashes []*SignalHash) error {
	if len(hashes) == 0 {
		return nil
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s.signal_hashes (timestamp_ns, hash, run_id, signal_type, signal_id, flags)
		VALUES ($1, $2, $3, $4, $5, $6)`, r.Schema)

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, sh := range hashes {
		_, err := stmt.ExecContext(ctx,
			sh.TimestampNs, sh.Hash, sh.RunID, sh.SignalType, sh.ID, sh.Flags,
		)
		if err != nil {
			return fmt.Errorf("failed to insert signal hash: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BatchCreateOrUpdate inserts multiple signal hashes with conflict resolution
func (r *SignalHashRepository) BatchCreateOrUpdate(ctx context.Context, hashes []*SignalHash) error {
	if len(hashes) == 0 {
		return nil
	}

	tx, err := r.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := fmt.Sprintf(`INSERT INTO %s.signal_hashes (timestamp_ns, hash, run_id, signal_type, signal_id, flags)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (timestamp_ns, hash) 
		DO UPDATE SET 
			run_id = EXCLUDED.run_id,
			signal_type = EXCLUDED.signal_type,
			signal_id = EXCLUDED.signal_id,
			flags = EXCLUDED.flags`, r.Schema)

	stmt, err := tx.PrepareContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, sh := range hashes {
		_, err := stmt.ExecContext(ctx,
			sh.TimestampNs, sh.Hash, sh.RunID, sh.SignalType, sh.ID, sh.Flags,
		)
		if err != nil {
			return fmt.Errorf("failed to insert or update signal hash: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// BatchCheckExists checks which hashes exist in the database
func (r *SignalHashRepository) BatchCheckExists(ctx context.Context, hashes []string) (map[string]bool, error) {
	if len(hashes) == 0 {
		return make(map[string]bool), nil
	}

	// Build the IN clause with proper placeholders
	placeholders := make([]string, len(hashes))
	args := make([]interface{}, len(hashes))
	for i, hash := range hashes {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = hash
	}

	query := fmt.Sprintf(`SELECT hash FROM %s.signal_hashes WHERE hash IN (%s)`,
		r.Schema, strings.Join(placeholders, ","))

	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		if isTableNotExistError(err) {
			return make(map[string]bool), nil
		}
		return nil, fmt.Errorf("failed to check hash existence: %w", err)
	}
	defer rows.Close()

	existingHashes := make(map[string]bool)
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return nil, fmt.Errorf("failed to scan hash: %w", err)
		}
		existingHashes[hash] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return existingHashes, nil
}

// GetByFlags retrieves signal hashes by flags
func (r *SignalHashRepository) GetByFlags(ctx context.Context, flags int16) ([]*SignalHash, error) {
	query := fmt.Sprintf(`SELECT timestamp_ns, hash, run_id, signal_type, signal_id, flags 
		FROM %s.signal_hashes WHERE flags = $1 ORDER BY timestamp_ns DESC`, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, flags)
	if err != nil {
		if isTableNotExistError(err) {
			return []*SignalHash{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var hashes []*SignalHash
	for rows.Next() {
		var sh SignalHash
		if err := rows.Scan(&sh.TimestampNs, &sh.Hash, &sh.RunID, &sh.SignalType, &sh.ID, &sh.Flags); err != nil {
			return nil, err
		}
		hashes = append(hashes, &sh)
	}
	return hashes, nil
}

// GetDuplicateHashes finds duplicate hashes within a time range
func (r *SignalHashRepository) GetDuplicateHashes(ctx context.Context, startTime, endTime int64) ([]*SignalHash, error) {
	query := fmt.Sprintf(`SELECT timestamp_ns, hash, run_id, signal_type, signal_id, flags 
		FROM %s.signal_hashes 
		WHERE timestamp_ns BETWEEN $1 AND $2
		AND hash IN (
			SELECT hash 
			FROM %s.signal_hashes 
			WHERE timestamp_ns BETWEEN $1 AND $2
			GROUP BY hash 
			HAVING COUNT(*) > 1
		)
		ORDER BY hash, timestamp_ns DESC`, r.Schema, r.Schema)

	rows, err := r.DB.QueryContext(ctx, query, startTime, endTime)
	if err != nil {
		if isTableNotExistError(err) {
			return []*SignalHash{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	var hashes []*SignalHash
	for rows.Next() {
		var sh SignalHash
		if err := rows.Scan(&sh.TimestampNs, &sh.Hash, &sh.RunID, &sh.SignalType, &sh.ID, &sh.Flags); err != nil {
			return nil, err
		}
		hashes = append(hashes, &sh)
	}
	return hashes, nil
}

// EnsureTable creates the signal_hashes table if it doesn't exist
func (r *SignalHashRepository) EnsureTable(ctx context.Context, schema string) error {
	// First, check if schema exists to avoid DDL execution on non-existent schemas
	var exists bool
	checkSchemaQuery := `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`
	if err := r.DB.QueryRowContext(ctx, checkSchemaQuery, schema).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check schema existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("schema '%s' does not exist", schema)
	}

	ddl := strings.ReplaceAll(signalHashesDDL, "{{.Schema}}", schema)

	// Execute the entire DDL as a single statement to avoid issues with DO blocks
	if _, err := r.DB.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("exec DDL: %w", err)
	}

	return nil
}
