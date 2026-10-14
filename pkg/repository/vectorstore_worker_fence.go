package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"sort"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// SupportsEmbeddingClaims is the provider contract for atomic vector fencing.
// Separate database pools are deliberately unsupported even for equal DSNs.
func (s *PGVectorCustomStore) SupportsEmbeddingClaims(db *sql.DB) bool {
	return db != nil && s.db == db
}

// The inference call has already returned. Only this short transaction locks
// claims, inserts a vector and records its recoverable reference atomically.
func (s *PGVectorCustomStore) insertFencedEmbedding(ctx context.Context, timestamp int64, embedding, service, content string, metadata map[string]interface{}, claims []EmbeddingClaim) (int64, error) {
	claims = append([]EmbeddingClaim(nil), claims...)
	sort.Slice(claims, func(i, j int) bool { return claimKey(claims[i]) < claimKey(claims[j]) })
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var existing int64
	for i, claim := range claims {
		if i > 0 && claimKey(claim) == claimKey(claims[i-1]) {
			return 0, fmt.Errorf("duplicate embedding claim in vector write")
		}
		id, err := lockEmbeddingClaim(ctx, tx, claim)
		if err != nil {
			return 0, err
		}
		if i == 0 {
			existing = id
		} else if existing != id {
			return 0, fmt.Errorf("partially committed vector group must recover its receipts first")
		}
	}
	tableRef, err := database.BuildSafeTableReference(s.schema, s.table)
	if err != nil {
		return 0, err
	}
	if existing > 0 {
		var found bool
		if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s WHERE id=$1)`, tableRef), existing).Scan(&found); err != nil {
			return 0, err
		}
		if found {
			return existing, tx.Commit()
		}
	}
	clean := maps.Clone(metadata)
	delete(clean, claimMetadataKey)
	metaJSON, err := json.Marshal(clean)
	if err != nil {
		return 0, err
	}
	var id int64
	query := fmt.Sprintf(`INSERT INTO %s(timestamp_ns,service_name,content,content_hash,embedding,metadata) VALUES($1,$2,$3,$4,$5::vector,$6) RETURNING id`, tableRef)
	if err := tx.QueryRowContext(ctx, query, timestamp, service, content, pgVectorContentHash(content), embedding, string(metaJSON)).Scan(&id); err != nil {
		return 0, err
	}
	for _, claim := range claims {
		ref, err := database.BuildSafeTableReference(claim.Schema, "embedding_worker_claims")
		if err != nil {
			return 0, err
		}
		result, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET vector_schema=$6,vector_table=$7,vector_id=$8 WHERE signal_table=$1 AND processor_id=$2 AND config_version=$3 AND signal_id=$4 AND lease_token=$5 AND NOT completed AND leased_until>clock_timestamp()`, ref), claim.SignalTable, claim.ProcessorID, claim.Version, claim.SignalID, claim.Token, s.schema, s.table, id)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if count != 1 {
			return 0, ErrEmbeddingLeaseLost
		}
	}
	return id, tx.Commit()
}
