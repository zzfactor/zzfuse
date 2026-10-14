package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

var ErrEmbeddingLeaseLost = errors.New("embedding worker lease lost")
var ErrEmbeddingVectorMissing = errors.New("committed embedding vector no longer exists")

const claimSignalKey = "_zzfuse_embedding_claim"
const claimMetadataKey = "_zzfuse_embedding_fences"
const defaultEmbeddingLease = 60 * time.Second

// EmbeddingClaim identifies one immutable configuration's attempt. Tokens stay
// private to the worker/write boundary and are never stored in vector metadata.
type EmbeddingClaim struct {
	Schema, SignalTable, Version, SignalID, Token string
	ProcessorID                                   int64
	VectorSchema, VectorTable                     string
	VectorID                                      int64
}

// This optional interface leaves read-only/database mocks compatible. Production
// SQL workers use it, rather than the legacy transaction-returning fetch API.
type EmbeddingClaimDatabase interface {
	ClaimEmbeddingSignals(context.Context, string, int, int64, string, *FetchOptions, []SignalFieldConfig) ([]map[string]interface{}, []EmbeddingClaim, error)
	RenewEmbeddingClaims(context.Context, []EmbeddingClaim, time.Duration) error
	CompleteEmbeddingClaims(context.Context, []EmbeddingClaim, []string) error
	ReleaseEmbeddingClaims(context.Context, []EmbeddingClaim) error
	ResetEmbeddingClaimVector(context.Context, EmbeddingClaim) error
}

func claimKey(c EmbeddingClaim) string {
	return c.Schema + "/" + c.SignalTable + "/" + fmt.Sprint(c.ProcessorID) + "/" + c.Version + "/" + c.SignalID
}

func validatedEmbeddingTable(schema, table string) (string, error) {
	if !stringSliceContains(AllSupportedSignalTypes(), table) {
		return "", fmt.Errorf("unsupported embedding signal table %q", table)
	}
	return database.BuildSafeTableReference(schema, table)
}

func validateFetchBounds(options *FetchOptions) error {
	if options == nil {
		return nil
	}
	for _, bound := range []*time.Time{options.StartTime, options.StopTime} {
		if bound != nil && !time.Unix(0, bound.UnixNano()).Equal(*bound) {
			return fmt.Errorf("embedding time bound exceeds nanosecond timestamp range")
		}
	}
	if options.StartTime != nil && options.StopTime != nil && !options.StartTime.Before(*options.StopTime) {
		return fmt.Errorf("embedding start time must precede stop time")
	}
	return nil
}

func (c *SQLDatabaseClient) ClaimEmbeddingSignals(ctx context.Context, table string, batchSize int, processorID int64, schema string, options *FetchOptions, fields []SignalFieldConfig) ([]map[string]interface{}, []EmbeddingClaim, error) {
	if options == nil || options.ConfigurationVersion == "" {
		return nil, nil, fmt.Errorf("durable embedding requires a configuration version")
	}
	if batchSize < 1 || batchSize > 10000 || processorID < 1 {
		return nil, nil, fmt.Errorf("invalid embedding batch size or processor ID")
	}
	if err := validateFetchBounds(options); err != nil {
		return nil, nil, err
	}
	tableRef, err := validatedEmbeddingTable(schema, table)
	if err != nil {
		return nil, nil, err
	}
	claimsRef, err := database.BuildSafeTableReference(schema, "embedding_worker_claims")
	if err != nil {
		return nil, nil, err
	}
	if err := c.ensureClaimTables(ctx, schema); err != nil {
		return nil, nil, err
	}
	// Bound orphan cleanup to avoid accumulating mappings after telemetry retention
	// or tenant data removal. Only this table is referenced by each cleanup pass.
	cleanup := fmt.Sprintf(`DELETE FROM %[1]s WHERE ctid IN (SELECT ctid FROM %[1]s c WHERE c.signal_table=$1 AND c.leased_until<clock_timestamp() AND NOT EXISTS (SELECT 1 FROM %[2]s s WHERE s.id::text=c.signal_id) LIMIT 512)`, claimsRef, tableRef)
	if _, err := c.DB.ExecContext(ctx, cleanup, table); err != nil {
		return nil, nil, err
	}
	token := rand.Text()
	lease := options.LeaseDuration
	if lease <= 0 {
		lease = defaultEmbeddingLease
	}
	args := []interface{}{table, processorID, options.ConfigurationVersion, token, lease.Milliseconds(), batchSize}
	where := `NOT (COALESCE(s.metadata->'embeddings', '[]'::jsonb) @> jsonb_build_array(jsonb_build_object('processor_id', $2::bigint, 'config_version', $3::text)))`
	if options.StartTime != nil {
		args = append(args, options.StartTime.UnixNano())
		where += fmt.Sprintf(" AND s.timestamp_ns >= $%d", len(args))
	}
	if options.StopTime != nil {
		args = append(args, options.StopTime.UnixNano())
		where += fmt.Sprintf(" AND s.timestamp_ns < $%d", len(args))
	}
	order := "DESC"
	if options.OrderBy == "ASC" {
		order = "ASC"
	}
	query := fmt.Sprintf(`WITH candidates AS MATERIALIZED (
	 SELECT s.id::text AS id FROM %[1]s s WHERE %[3]s
	 AND NOT EXISTS (SELECT 1 FROM %[2]s c WHERE c.signal_table=$1 AND c.processor_id=$2 AND c.config_version=$3 AND c.signal_id=s.id::text AND c.leased_until>clock_timestamp())
	 ORDER BY s.timestamp_ns %[4]s, s.id %[4]s LIMIT $6
	) INSERT INTO %[2]s AS c (signal_table,processor_id,config_version,signal_id,lease_token,leased_until)
	 SELECT $1,$2,$3,id,$4,clock_timestamp()+$5*interval '1 millisecond' FROM candidates
	 ON CONFLICT (signal_table,processor_id,config_version,signal_id) DO UPDATE SET lease_token=EXCLUDED.lease_token,leased_until=EXCLUDED.leased_until,completed=FALSE
	 WHERE c.leased_until<=clock_timestamp()`, tableRef, claimsRef, where, order)
	if _, err = c.DB.ExecContext(ctx, query, args...); err != nil {
		return nil, nil, fmt.Errorf("acquire embedding claims: %w", err)
	}
	if len(fields) == 0 {
		fields = append([]SignalFieldConfig(nil), signalFieldConfigs[table]...)
	} else {
		fields = append([]SignalFieldConfig(nil), fields...)
	}
	allowed := make(map[string]bool)
	for _, f := range signalFieldConfigs[table] {
		allowed[f.FieldName] = true
	}
	hasID := false
	selected := make([]string, 0, len(fields)+3)
	for _, f := range fields {
		if !allowed[f.FieldName] {
			return nil, nil, fmt.Errorf("invalid embedding field %q", f.FieldName)
		}
		selected = append(selected, "s."+f.FieldName)
		if f.SignalKey == "id" {
			hasID = true
		}
	}
	if !hasID {
		fields = append(fields, SignalFieldConfig{FieldName: "id", SignalKey: "id"})
		selected = append(selected, "s.id")
	}
	fields = append(fields, SignalFieldConfig{SignalKey: "_vector_id"}, SignalFieldConfig{SignalKey: "_vector_schema"}, SignalFieldConfig{SignalKey: "_vector_table"})
	selected = append(selected, "c.vector_id", "c.vector_schema", "c.vector_table")
	read := fmt.Sprintf(`SELECT %s FROM %s s JOIN %s c ON c.signal_id=s.id::text WHERE c.signal_table=$1 AND c.processor_id=$2 AND c.config_version=$3 AND c.lease_token=$4 AND NOT c.completed ORDER BY s.timestamp_ns %s,s.id %s`, strings.Join(selected, ","), tableRef, claimsRef, order, order)
	rows, err := c.DB.QueryContext(ctx, read, table, processorID, options.ConfigurationVersion, token)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var signals []map[string]interface{}
	var claims []EmbeddingClaim
	for rows.Next() {
		signal, err := c.scanSignalRow(rows, table, fields)
		if err != nil {
			return nil, nil, err
		}
		claim := EmbeddingClaim{Schema: schema, SignalTable: table, ProcessorID: processorID, Version: options.ConfigurationVersion, SignalID: fmt.Sprint(signal["id"]), Token: token}
		claim.VectorID, _ = signal["_vector_id"].(int64)
		claim.VectorSchema, _ = signal["_vector_schema"].(string)
		claim.VectorTable, _ = signal["_vector_table"].(string)
		delete(signal, "_vector_id")
		delete(signal, "_vector_schema")
		delete(signal, "_vector_table")
		signal[claimSignalKey] = claim
		signals = append(signals, signal)
		claims = append(claims, claim)
	}
	return signals, claims, rows.Err()
}

func (c *SQLDatabaseClient) ensureClaimTables(ctx context.Context, schema string) error {
	c.claimsMu.Lock()
	defer c.claimsMu.Unlock()
	if c.claimSchemas[schema] {
		return nil
	}
	if err := database.EnsureEmbeddingWorkerClaims(ctx, c.DB, schema); err != nil {
		return err
	}
	if c.claimSchemas == nil {
		c.claimSchemas = make(map[string]bool)
	}
	c.claimSchemas[schema] = true
	return nil
}

func (c *SQLDatabaseClient) RenewEmbeddingClaims(ctx context.Context, claims []EmbeddingClaim, lease time.Duration) error {
	for _, claim := range claims {
		ref, err := database.BuildSafeTableReference(claim.Schema, "embedding_worker_claims")
		if err != nil {
			return err
		}
		result, err := c.DB.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET leased_until=clock_timestamp()+$6*interval '1 millisecond' WHERE signal_table=$1 AND processor_id=$2 AND config_version=$3 AND signal_id=$4 AND lease_token=$5 AND NOT completed AND leased_until>clock_timestamp()`, ref), claim.SignalTable, claim.ProcessorID, claim.Version, claim.SignalID, claim.Token, lease.Milliseconds())
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrEmbeddingLeaseLost
		}
	}
	return nil
}

func (c *SQLDatabaseClient) ReleaseEmbeddingClaims(ctx context.Context, claims []EmbeddingClaim) error {
	for _, claim := range claims {
		ref, err := database.BuildSafeTableReference(claim.Schema, "embedding_worker_claims")
		if err != nil {
			return err
		}
		if _, err = c.DB.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET leased_until=clock_timestamp() WHERE signal_table=$1 AND processor_id=$2 AND config_version=$3 AND signal_id=$4 AND lease_token=$5 AND NOT completed`, ref), claim.SignalTable, claim.ProcessorID, claim.Version, claim.SignalID, claim.Token); err != nil {
			return err
		}
	}
	return nil
}

func lockEmbeddingClaim(ctx context.Context, tx *sql.Tx, claim EmbeddingClaim) (int64, error) {
	ref, err := database.BuildSafeTableReference(claim.Schema, "embedding_worker_claims")
	if err != nil {
		return 0, err
	}
	var vectorID sql.NullInt64
	err = tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT vector_id FROM %s WHERE signal_table=$1 AND processor_id=$2 AND config_version=$3 AND signal_id=$4 AND lease_token=$5 AND NOT completed AND leased_until>clock_timestamp() FOR UPDATE`, ref), claim.SignalTable, claim.ProcessorID, claim.Version, claim.SignalID, claim.Token).Scan(&vectorID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrEmbeddingLeaseLost
	}
	return vectorID.Int64, err
}

func (c *SQLDatabaseClient) CompleteEmbeddingClaims(ctx context.Context, claims []EmbeddingClaim, vectors []string) error {
	if len(claims) != len(vectors) {
		return fmt.Errorf("embedding claim/vector count mismatch")
	}
	type pair struct {
		claim  EmbeddingClaim
		vector string
	}
	pairs := make([]pair, len(claims))
	for i, claim := range claims {
		pairs[i] = pair{claim, vectors[i]}
	}
	sort.Slice(pairs, func(i, j int) bool { return claimKey(pairs[i].claim) < claimKey(pairs[j].claim) })
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, entry := range pairs {
		claim := entry.claim
		stored, err := lockEmbeddingClaim(ctx, tx, claim)
		if err != nil {
			return err
		}
		if fmt.Sprint(stored) != entry.vector || stored == 0 {
			return fmt.Errorf("embedding vector was not committed by this claim")
		}
		// Recovery must not issue a receipt for a vector removed by retention.
		var vectorSchema, vectorTable string
		claimsRef, _ := database.BuildSafeTableReference(claim.Schema, "embedding_worker_claims")
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT vector_schema,vector_table FROM %s WHERE signal_table=$1 AND processor_id=$2 AND config_version=$3 AND signal_id=$4`, claimsRef), claim.SignalTable, claim.ProcessorID, claim.Version, claim.SignalID).Scan(&vectorSchema, &vectorTable); err != nil {
			return err
		}
		vectorRef, err := database.BuildSafeTableReference(vectorSchema, vectorTable)
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE id=$1)`, vectorRef), stored).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrEmbeddingVectorMissing
		}
		sourceRef, err := validatedEmbeddingTable(claim.Schema, claim.SignalTable)
		if err != nil {
			return err
		}
		receipt, _ := json.Marshal(map[string]interface{}{"processor_id": claim.ProcessorID, "config_version": claim.Version, "embedded_id": entry.vector})
		query := fmt.Sprintf(`UPDATE %s SET metadata=jsonb_set(COALESCE(metadata,'{}'::jsonb),'{embeddings}',COALESCE((SELECT jsonb_agg(e) FROM jsonb_array_elements(COALESCE(metadata->'embeddings','[]'::jsonb)) e WHERE NOT(e @> jsonb_build_object('processor_id',$1::bigint,'config_version',$2::text))),'[]'::jsonb)||jsonb_build_array($3::jsonb)) WHERE id::text=$4`, sourceRef)
		result, err := tx.ExecContext(ctx, query, claim.ProcessorID, claim.Version, string(receipt), claim.SignalID)
		if err != nil {
			return err
		}
		count, _ := result.RowsAffected()
		if count != 1 {
			return fmt.Errorf("embedding source signal disappeared")
		}
		result, err = tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET completed=TRUE,leased_until=clock_timestamp() WHERE signal_table=$1 AND processor_id=$2 AND config_version=$3 AND signal_id=$4 AND lease_token=$5 AND leased_until>clock_timestamp()`, claimsRef), claim.SignalTable, claim.ProcessorID, claim.Version, claim.SignalID, claim.Token)
		if err != nil {
			return err
		}
		count, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrEmbeddingLeaseLost
		}
	}
	return tx.Commit()
}

func (c *SQLDatabaseClient) ResetEmbeddingClaimVector(ctx context.Context, claim EmbeddingClaim) error {
	ref, err := database.BuildSafeTableReference(claim.Schema, "embedding_worker_claims")
	if err != nil {
		return err
	}
	result, err := c.DB.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET vector_id=NULL,vector_schema=NULL,vector_table=NULL WHERE signal_table=$1 AND processor_id=$2 AND config_version=$3 AND signal_id=$4 AND lease_token=$5 AND NOT completed AND leased_until>clock_timestamp()`, ref), claim.SignalTable, claim.ProcessorID, claim.Version, claim.SignalID, claim.Token)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrEmbeddingLeaseLost
	}
	return nil
}
