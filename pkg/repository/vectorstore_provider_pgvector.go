package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "embed"

	"math"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	sqltemplate "github.com/medxops/trazr-otel/pkg/repository/template/sql"
	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/tmc/langchaingo/schema"
)

//go:embed internal/database/ddl/pgvector_store.sql
var pgvectorStoreSQL string

// VectorStoreMetrics tracks vector operations for billing and monitoring
type VectorStoreMetrics struct {
	VectorOperations int64
	VectorDimensions int64
	CombinationTime  time.Duration
	QualityCheckTime time.Duration
	CostEstimate     float64
	QualityIssues    int64
	mu               sync.RWMutex
}

// NewVectorStoreMetrics creates new metrics tracker
func NewVectorStoreMetrics() *VectorStoreMetrics {
	return &VectorStoreMetrics{}
}

// RecordOperation records a vector operation for billing
func (m *VectorStoreMetrics) RecordOperation(dimensions int, duration time.Duration, qualityIssues int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.VectorOperations++
	m.VectorDimensions += int64(dimensions)
	m.CombinationTime += duration
	m.QualityIssues += int64(qualityIssues)
	// Cost estimation: $0.0001 per vector operation + $0.00001 per dimension
	m.CostEstimate += 0.0001 + (float64(dimensions) * 0.00001)
}

// Add this struct near the top of the file, after imports or with related types
type PgvectorDDLParams struct {
	Schema          string
	Table           string
	Dimension       int
	RetentionMonths int
	FallbackMonths  int
}

// PGVectorCustomStore implements EmbeddingRepository for direct use
// In this project, 'namespace' maps to the Postgres schema for PGVector, matching LangChain Go's collection/namespace abstraction.
type PGVectorCustomStore struct {
	db                 *sql.DB
	schema             string // 'namespace' in config maps to this field
	table              string
	EmbProvider        EmbeddingProvider                      // Optional: used for query string embedding
	embeddingDimension int                                    // used for table creation
	useExternalPGStore bool                                   // true if this store owns the DB and should close it
	logger             util.Logger                            // Added for logging
	partitionConfig    *database.TimescaleDBMaintenanceConfig // Added for TimescaleDB maintenance
	metrics            *VectorStoreMetrics                    // Added for vector operations tracking
}

// PGVectorOption defines a functional option for PGVectorCustomStore
type PGVectorOption func(*PGVectorCustomStore)

// WithDB sets the database connection
func WithDB(db *sql.DB) PGVectorOption {
	return func(s *PGVectorCustomStore) { s.db = db }
}

// WithSchema sets the schema
func WithSchema(schema string) PGVectorOption {
	return func(s *PGVectorCustomStore) { s.schema = schema }
}

// WithTable sets the table
func WithTable(table string) PGVectorOption {
	return func(s *PGVectorCustomStore) { s.table = table }
}

// WithNamespace sets the namespace (maps to schema by default for PGVector; see LangChain Go docs)
func WithNamespace(namespace string) PGVectorOption {
	return func(s *PGVectorCustomStore) { s.schema = namespace }
}

// WithEmbedder sets the embedding provider
func WithEmbedder(provider EmbeddingProvider) PGVectorOption {
	return func(s *PGVectorCustomStore) { s.EmbProvider = provider }
}

// WithEmbeddingDimension sets the embedding dimension for table creation
func WithEmbeddingDimension(dim int) PGVectorOption {
	return func(s *PGVectorCustomStore) {
		if dim > 0 {
			s.embeddingDimension = dim
		}
	}
}

// WithExternalPGStore sets the useExternalPGStore flag
func WithExternalPGStore(owns bool) PGVectorOption {
	return func(s *PGVectorCustomStore) { s.useExternalPGStore = owns }
}

// WithPartitionConfig sets the TimescaleDB maintenance configuration
func WithPartitionConfig(config *database.TimescaleDBMaintenanceConfig) PGVectorOption {
	return func(s *PGVectorCustomStore) { s.partitionConfig = config }
}

// New creates a new PGVectorCustomStore using options
func New(opts ...PGVectorOption) *PGVectorCustomStore {
	s := &PGVectorCustomStore{
		metrics: NewVectorStoreMetrics(),
	}
	for _, opt := range opts {
		opt(s)
	}
	// If db and dimension are set, ensure table exists
	if s.db != nil && (s.embeddingDimension > 0 || s.EmbProvider != nil) {
		ctx := context.Background()
		_ = s.EnsureTable(ctx) // ignore error here; user should call explicitly for error handling
	}

	return s
}

// openPGVectorDatabase opens a pgvector DB connection with all config, secret, and pool logic.
func openPGVectorDatabase(ctx context.Context, config database.VectorStoreConfig, mainDB *sql.DB, logger util.Logger) (db *sql.DB, ownsDB bool, err error) {
	if config.Endpoint == "" {
		if mainDB == nil {
			logger.Error("Main database is required for local pgvector store but was nil")
			return nil, false, fmt.Errorf("main database is required for local pgvector store but was nil")
		}
		return mainDB, false, nil
	}
	// Build DSN from config and get password from metadata or env
	password := util.GetSecretFromMetadata(config.Metadata, "vectorstore_api_key")

	host, port, _ := net.SplitHostPort(config.Endpoint)
	sslMode := util.GetEnvString("TRAZR_DB_SSL_MODE", "prefer")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, config.User, password, config.Database, sslMode)
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		return nil, false, fmt.Errorf("failed to open DB for pgvector store: %w", err)
	}
	// Validate the connection
	if pingErr := db.PingContext(ctx); pingErr != nil {
		return nil, false, fmt.Errorf("failed to ping DB for pgvector store: %w", pingErr)
	}
	logger.Debug("Opened DB for pgvector store", "host=%s port=%s", host, port)
	ownsDB = true
	// Set pool settings from config struct fields
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	return db, ownsDB, nil
}

// InsertEmbeddingRaw is the low-level insert that returns the generated ID
func (s *PGVectorCustomStore) InsertEmbeddingRaw(ctx context.Context, timestampNS int64, embedding []float32, serviceName, content string, metadata map[string]interface{}) (int64, error) {
	// Validate database connection first
	if s.db == nil {
		return 0, fmt.Errorf("database connection is nil")
	}
	embeddingValue, err := serializePGVector(embedding)
	if err != nil {
		return 0, err
	}
	if claims, ok := metadata[claimMetadataKey].([]EmbeddingClaim); ok && len(claims) > 0 {
		return s.insertFencedEmbedding(ctx, timestampNS, embeddingValue, serviceName, content, metadata, claims)
	}

	// Test connection before proceeding
	if err := s.db.PingContext(ctx); err != nil {
		if s.logger != nil {
			s.logger.Error("Database ping failed", "error", err, "schema", s.schema, "table", s.table)
		}
		return 0, fmt.Errorf("database connection test failed: %w", err)
	}

	// Log connection stats for debugging
	if s.logger != nil {
		stats := s.db.Stats()
		s.logger.Debug("Database connection stats",
			"open_connections", stats.OpenConnections,
			"in_use", stats.InUse,
			"idle", stats.Idle,
			"wait_count", stats.WaitCount,
			"wait_duration", stats.WaitDuration,
			"max_idle_closed", stats.MaxIdleClosed,
			"max_lifetime_closed", stats.MaxLifetimeClosed)
	}

	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal metadata: %w", err)
	}

	// Build safe table reference to prevent SQL injection
	tableRef, err := database.BuildSafeTableReference(s.schema, s.table)
	if err != nil {
		return 0, fmt.Errorf("invalid schema/table reference: %w", err)
	}

	// Check if table exists before inserting
	var tableExists bool
	checkQuery := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables 
		WHERE table_schema = $1 AND table_name = $2
	)`
	err = s.db.QueryRowContext(ctx, checkQuery, s.schema, s.table).Scan(&tableExists)
	if err != nil {
		if s.logger != nil {
			s.logger.Error("Failed to check if table exists", "error", err, "schema", s.schema, "table", s.table)
		}
		return 0, fmt.Errorf("failed to check if table exists: %w", err)
	}

	if !tableExists {
		if s.logger != nil {
			s.logger.Error("Table does not exist", "schema", s.schema, "table", s.table)
		}
		return 0, fmt.Errorf("table %s.%s does not exist", s.schema, s.table)
	}
	query := fmt.Sprintf(`INSERT INTO %s (timestamp_ns, service_name, content, content_hash, embedding, metadata) VALUES ($1, $2, $3, $4, $5::vector, $6) RETURNING id`, tableRef)

	// Log what we're about to insert
	if s.logger != nil {
		s.logger.Debug("Inserting into vector store",
			"content_length", len(content),
			"content_preview", func() string {
				if len(content) > 100 {
					return content[:100] + "..."
				}
				return content
			}(),
			"table", s.table,
			"schema", s.schema)
	}

	// Execute with retry logic for connection issues
	var lastErr error
	var generatedID int64
	for attempt := 1; attempt <= 3; attempt++ {
		err = s.db.QueryRowContext(ctx, query, timestampNS, serviceName, content, pgVectorContentHash(content), embeddingValue, string(metaJSON)).Scan(&generatedID)
		if err != nil {
			lastErr = err
			if s.logger != nil {
				s.logger.Error("Insert attempt failed",
					"attempt", attempt,
					"error", err,
					"table", s.table)
			}

			// If it's a connection error, try to ping and retry
			if strings.Contains(err.Error(), "unexpected EOF") ||
				strings.Contains(err.Error(), "connection") ||
				strings.Contains(err.Error(), "broken pipe") {
				if pingErr := s.db.PingContext(ctx); pingErr != nil {
					return 0, fmt.Errorf("database connection lost and cannot be restored: %w (original error: %v)", pingErr, err)
				}
				// Wait a bit before retry
				time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
				continue
			}
			// For non-connection errors, don't retry
			break
		}
		// Success
		return generatedID, nil
	}

	return 0, fmt.Errorf("insert embedding failed after 3 attempts: %w", lastErr)
}

// FindBaselineByContentHash finds existing embeddings with the same content hash
func (s *PGVectorCustomStore) FindBaselineByContentHash(ctx context.Context, content string) ([]interface{}, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}

	tableRef, err := database.BuildSafeTableReference(s.schema, s.table)
	if err != nil {
		return nil, fmt.Errorf("invalid schema/table reference: %w", err)
	}
	query := fmt.Sprintf(`SELECT id, content, metadata FROM %s WHERE content_hash = $1`, tableRef)
	rows, err := s.db.QueryContext(ctx, query, pgVectorContentHash(content))
	if err != nil {
		return nil, fmt.Errorf("baseline search failed: %w", err)
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var id, content, metaJSON string
		if err := rows.Scan(&id, &content, &metaJSON); err != nil {
			return nil, fmt.Errorf("scan baseline: %w", err)
		}
		var meta map[string]interface{}
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		docs = append(docs, Document{ID: id, PageContent: content, Metadata: meta})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("baseline rows: %w", err)
	}

	if s.logger != nil {
		s.logger.Info("Baseline search completed",
			"content_length", len(content),
			"baselines_found", len(docs))
	}

	// Convert Document slice to interface{} slice
	result := make([]interface{}, len(docs))
	for i, doc := range docs {
		result[i] = doc
	}

	return result, nil
}

// Match the historical PostgreSQL digest(content, 'sha256') without requiring
// a legacy bootstrap function or a tenant-dependent search_path.
func pgVectorContentHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}

// pgvector stores float32 values. Roundtrip formatting preserves every finite
// inference component, including values smaller than six decimal places.
func serializePGVector(embedding []float32) (string, error) {
	if len(embedding) == 0 {
		return "", fmt.Errorf("embedding must contain at least one component")
	}
	parts := make([]string, len(embedding))
	for i, value := range embedding {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return "", fmt.Errorf("embedding component %d is not finite", i)
		}
		parts[i] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

// SearchSimilar implements EmbeddingRepository. It supports both vector and query string search.
func (s *PGVectorCustomStore) SearchSimilar(ctx context.Context, query []float32, k int, filter map[string]interface{}) ([]any, error) {
	var vec []float32
	if len(query) > 0 {
		vec = query
	} else if filter != nil && s.EmbProvider != nil {
		if qstr, ok := filter["query_string"].(string); ok && qstr != "" {
			v, err := s.EmbProvider.EmbedQuery(ctx, qstr)
			if err != nil {
				return nil, err
			}
			vec = v
		}
	}
	if len(vec) == 0 {
		return nil, fmt.Errorf("no query vector provided for similarity search")
	}
	docs, err := s.SimilaritySearchByVector(ctx, vec, k)
	if err != nil {
		return nil, err
	}
	results := make([]any, len(docs))
	for i, doc := range docs {
		results[i] = doc
	}
	return results, nil
}

// DeleteEmbedding implements EmbeddingRepository. It deletes by embedding ID.
func (s *PGVectorCustomStore) DeleteEmbedding(ctx context.Context, embeddingID string) error {
	return s.Delete(ctx, []string{embeddingID})
}

// HealthCheck implements EmbeddingRepository. Optionally check DB health.
func (s *PGVectorCustomStore) HealthCheck(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

// Close implements EmbeddingRepository. Closes the DB connection.
func (s *PGVectorCustomStore) Close() error {
	if s.useExternalPGStore && s.db != nil {
		return s.db.Close()
	}
	return nil
}

// parseSearchOptions parses opts for filter and operator
func parseSearchOptions(opts ...interface{}) (map[string]interface{}, string) {
	var filter map[string]interface{}
	operator := "cosine" // default
	for _, opt := range opts {
		switch v := opt.(type) {
		case map[string]interface{}:
			filter = v
		case string:
			operator = v
		}
	}
	return filter, operator
}

// SimilaritySearch implements the langchaingo VectorStore interface.
// Note: Results should be filtered by a score threshold in the caller (e.g., only return docs with score < 0.3).
func (s *PGVectorCustomStore) SimilaritySearch(ctx context.Context, query string, k int, opts ...interface{}) ([]schema.Document, error) {
	if s.EmbProvider == nil {
		return nil, fmt.Errorf("embedding provider not configured")
	}
	vector, err := s.EmbProvider.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding query failed: %w", err)
	}
	return s.SimilaritySearchByVector(ctx, vector, k, opts...)
}

// SimilaritySearchByVector implements the langchaingo VectorStore interface.
func (s *PGVectorCustomStore) SimilaritySearchByVector(ctx context.Context, query []float32, k int, opts ...interface{}) ([]schema.Document, error) {
	filter, _ := parseSearchOptions(opts...)
	vecStr := make([]string, len(query))
	for i, v := range query {
		vecStr[i] = fmt.Sprintf("%f", v)
	}
	embeddingLiteral := fmt.Sprintf("'[%s]'", strings.Join(vecStr, ","))
	where := ""
	args := []interface{}{k}
	if len(filter) > 0 {
		clauses := make([]string, 0, len(filter))
		argIdx := 2
		for k, v := range filter {
			clauses = append(clauses, fmt.Sprintf("metadata->>$%d = $%d", argIdx, argIdx+1))
			args = append(args, k, v)
			argIdx += 2
		}
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	// Fetch id, content, metadata, and score using cosine distance (<=>)
	queryStr := fmt.Sprintf(`SELECT id, content, metadata, (embedding <=> %s) as score FROM %s.%s %s ORDER BY score LIMIT $1`, embeddingLiteral, s.schema, s.table, where)
	rows, err := s.db.QueryContext(ctx, queryStr, args...)
	if err != nil {
		return nil, fmt.Errorf("similarity search: %w", err)
	}
	defer rows.Close()
	var docs []schema.Document
	for rows.Next() {
		var id, content, metaJSON string
		var score float64
		if err := rows.Scan(&id, &content, &metaJSON, &score); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var metadata map[string]interface{}
		_ = json.Unmarshal([]byte(metaJSON), &metadata)

		// Log what we're retrieving
		if s.logger != nil {
			s.logger.Info("Retrieved from vector store",
				"id", id,
				"content_length", len(content),
				"content_preview", func() string {
					if len(content) > 100 {
						return content[:100] + "..."
					}
					return content
				}(),
				"table", s.table)
		}

		docs = append(docs, schema.Document{
			PageContent: content,
			Metadata:    metadata,
			Score:       float32(score),
		})
	}
	return docs, nil
}

// SimilaritySearchVec returns the top K most similar embedding IDs to the query vector, with optional metadata filter and operator
func (s *PGVectorCustomStore) SimilaritySearchVec(ctx context.Context, queryVec []float32, topK int, filter map[string]interface{}, operator string) ([]string, error) {
	vecStr := make([]string, len(queryVec))
	for i, v := range queryVec {
		vecStr[i] = fmt.Sprintf("%f", v)
	}
	embeddingLiteral := fmt.Sprintf("'[%s]'", strings.Join(vecStr, ","))
	where := ""
	args := []interface{}{topK}
	if len(filter) > 0 {
		clauses := make([]string, 0, len(filter))
		argIdx := 2
		for k, v := range filter {
			clauses = append(clauses, fmt.Sprintf("metadata->>$%d = $%d", argIdx, argIdx+1))
			args = append(args, k, v)
			argIdx += 2
		}
		where = "WHERE " + strings.Join(clauses, " AND ")
	}
	query := fmt.Sprintf(`SELECT id FROM %s.%s %s ORDER BY embedding <-> %s LIMIT $1`, s.schema, s.table, where, embeddingLiteral)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("similarity search: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// Document represents a text chunk and its metadata for vector storage
// Compatible with LangChain Go conventions
// You may want to move this to a shared types file if needed
type Document struct {
	ID          string
	PageContent string
	Metadata    map[string]interface{}
}

// VectorDocument represents a document with embedded vector data
type VectorDocument struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Embedding []float32              `json:"embedding"`
	Metadata  map[string]interface{} `json:"metadata"`
	Model     string                 `json:"model"`
	Dimension int                    `json:"dimension"`
	CreatedAt time.Time              `json:"created_at"`
}

// VectorQualityMetrics tracks vector quality for billing and monitoring
type VectorQualityMetrics struct {
	AverageMagnitude     float64
	DimensionConsistency bool
	ZeroVectorCount      int
	InvalidValueCount    int
	ModelConsistency     bool
	TotalVectors         int
}

type ScoredDocument struct {
	Document
	Score float64
}

// AddDocuments implements the langchaingo VectorStore interface.
func (s *PGVectorCustomStore) AddDocuments(ctx context.Context, docs []schema.Document) ([]string, error) {
	if s.EmbProvider == nil {
		return nil, fmt.Errorf("embedding provider not configured")
	}
	texts := make([]string, len(docs))
	serviceNames := make([]string, len(docs))
	for i, doc := range docs {
		texts[i] = doc.PageContent
		if v, ok := doc.Metadata["service_name"].(string); ok && v != "" {
			serviceNames[i] = v
		} else if v, ok := doc.Metadata["service.name"].(string); ok && v != "" {
			serviceNames[i] = v
		} else {
			serviceNames[i] = "default"
		}
	}
	embeddings, err := s.EmbProvider.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	ids := make([]string, len(docs))
	for i, doc := range docs {
		timestampNS := ensureTimestampNS(doc.Metadata)
		generatedID, err := s.InsertEmbeddingRaw(ctx, timestampNS, embeddings[i], serviceNames[i], doc.PageContent, doc.Metadata)
		if err != nil {
			return ids, err
		}
		// Convert the generated int64 ID to string for consistency with the interface
		ids[i] = strconv.FormatInt(generatedID, 10)
	}
	return ids, nil
}

// Delete removes documents by ID
func (s *PGVectorCustomStore) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(`DELETE FROM %s.%s WHERE id IN (%s)`, s.schema, s.table, strings.Join(placeholders, ","))
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

// SimilaritySearchWithScore returns top K documents with similarity scores
func (s *PGVectorCustomStore) SimilaritySearchWithScore(ctx context.Context, queryVec []float32, k int) ([]ScoredDocument, error) {
	vecStr := make([]string, len(queryVec))
	for i, v := range queryVec {
		vecStr[i] = fmt.Sprintf("%f", v)
	}
	embeddingLiteral := fmt.Sprintf("'[%s]'", strings.Join(vecStr, ","))
	query := fmt.Sprintf(`SELECT id, content, metadata, (embedding <-> %s) as score FROM %s.%s ORDER BY score LIMIT $1`, embeddingLiteral, s.schema, s.table)
	rows, err := s.db.QueryContext(ctx, query, k)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []ScoredDocument
	for rows.Next() {
		var id, content, metaJSON string
		var score float64
		if err := rows.Scan(&id, &content, &metaJSON, &score); err != nil {
			return nil, err
		}
		var meta map[string]interface{}
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		docs = append(docs, ScoredDocument{Document: Document{ID: id, PageContent: content, Metadata: meta}, Score: score})
	}
	return docs, nil
}

// GetByIDs fetches documents by their IDs
func (s *PGVectorCustomStore) GetByIDs(ctx context.Context, ids []string) ([]Document, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(`SELECT id, content, metadata FROM %s.%s WHERE id IN (%s)`, s.schema, s.table, strings.Join(placeholders, ","))
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []Document
	for rows.Next() {
		var id, content, metaJSON string
		if err := rows.Scan(&id, &content, &metaJSON); err != nil {
			return nil, err
		}
		var meta map[string]interface{}
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		docs = append(docs, Document{ID: id, PageContent: content, Metadata: meta})
	}
	return docs, nil
}

// MaxMarginalRelevanceSearch is not implemented in this thin wrapper
func (s *PGVectorCustomStore) MaxMarginalRelevanceSearch(ctx context.Context, queryVec []float32, k, fetchK int, lambdaMult float64) ([]Document, error) {
	return nil, fmt.Errorf("MaxMarginalRelevanceSearch not implemented")
}

// SearchOperator defines the distance operator for vector search
// Supported: "cosine", "l2", "ip"
type SearchOperator string

const (
	OperatorCosine SearchOperator = "cosine"
	OperatorL2     SearchOperator = "l2"
	OperatorIP     SearchOperator = "ip"
)

// SearchOption allows configuring advanced search
// e.g., WithOperator, WithFilter, WithPagination

type SearchOption func(*searchParams)

type searchParams struct {
	operator SearchOperator
	filters  map[string]interface{}
	limit    int
	offset   int
}

// WithOperator sets the distance operator (cosine, l2, ip)
func WithOperator(op SearchOperator) SearchOption {
	return func(p *searchParams) { p.operator = op }
}

// WithFilter sets additional filters (by metadata, service_name, etc.)
func WithFilter(filters map[string]interface{}) SearchOption {
	return func(p *searchParams) { p.filters = filters }
}

// WithPagination sets offset and limit
func WithPagination(offset, limit int) SearchOption {
	return func(p *searchParams) { p.offset = offset; p.limit = limit }
}

// AdvancedSearch supports filtering, hybrid search, operator selection, and pagination
func (s *PGVectorCustomStore) AdvancedSearch(ctx context.Context, queryVec []float32, k int, opts ...SearchOption) ([]Document, error) {
	params := &searchParams{
		operator: OperatorCosine, // default
		filters:  map[string]interface{}{},
		limit:    k,
		offset:   0,
	}
	for _, opt := range opts {
		opt(params)
	}

	// Build vector literal
	vecStr := make([]string, len(queryVec))
	for i, v := range queryVec {
		vecStr[i] = fmt.Sprintf("%f", v)
	}
	embeddingLiteral := fmt.Sprintf("'[%s]'", strings.Join(vecStr, ","))

	// Choose operator
	var op string
	switch params.operator {
	case OperatorCosine:
		op = "<=>" // cosine distance
	case OperatorL2:
		op = "<->" // L2 distance
	case OperatorIP:
		op = "<#>" // negative inner product
	default:
		op = "<=>" // fallback to cosine
	}

	// Build WHERE clause for filters
	whereClauses := []string{}
	args := []interface{}{}
	argIdx := 1
	for k, v := range params.filters {
		switch k {
		case "service_name":
			whereClauses = append(whereClauses, fmt.Sprintf("service_name = $%d", argIdx))
			args = append(args, v)
			argIdx++
		case "content":
			whereClauses = append(whereClauses, fmt.Sprintf("content ILIKE $%d", argIdx))
			args = append(args, "%"+fmt.Sprintf("%v", v)+"%")
			argIdx++
		case "timestamp_ns_gte":
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp_ns >= $%d", argIdx))
			args = append(args, v)
			argIdx++
		case "timestamp_ns_lte":
			whereClauses = append(whereClauses, fmt.Sprintf("timestamp_ns <= $%d", argIdx))
			args = append(args, v)
			argIdx++
		default:
			// Assume metadata JSONB filter
			whereClauses = append(whereClauses, fmt.Sprintf("metadata @> $%d", argIdx))
			metaJSON, _ := json.Marshal(map[string]interface{}{k: v})
			args = append(args, string(metaJSON))
			argIdx++
		}
	}
	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Pagination
	limit := params.limit
	if limit <= 0 {
		limit = 10
	}
	offset := params.offset

	// Final SQL
	query := fmt.Sprintf(`SELECT id, content, metadata, (embedding %s %s) as score FROM %s.%s %s ORDER BY score LIMIT $%d`,
		op, embeddingLiteral, s.schema, s.table, whereSQL, argIdx)
	args = append(args, limit)
	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIdx+1)
		args = append(args, offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var docs []Document
	for rows.Next() {
		var id, content, metaJSON string
		var score float64
		if err := rows.Scan(&id, &content, &metaJSON, &score); err != nil {
			return nil, err
		}
		var meta map[string]interface{}
		_ = json.Unmarshal([]byte(metaJSON), &meta)
		docs = append(docs, Document{ID: id, PageContent: content, Metadata: meta})
	}
	return docs, nil
}

// EnsureTable checks if the vector table exists, and creates it if not (with correct dimension)
func (s *PGVectorCustomStore) EnsureTable(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("db is not set")
	}
	if s.schema == "" || s.table == "" {
		return fmt.Errorf("schema and table must be set")
	}
	// Check if table exists
	row := s.db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2
	)`, s.schema, s.table)
	var exists bool
	if err := row.Scan(&exists); err != nil {
		return fmt.Errorf("EnsureTable: scan error: %w", err)
	}
	if exists {
		return nil
	}
	// Need embedding dimension
	dim := s.embeddingDimension
	if dim == 0 && s.EmbProvider != nil {
		// Try to infer from provider by embedding a dummy string
		vec, err := s.EmbProvider.EmbedQuery(ctx, "dimension_probe")
		if err == nil && len(vec) > 0 {
			dim = len(vec)
		}
	}
	if dim <= 0 {
		return fmt.Errorf("embedding dimension must be set to create table (got %d)", dim)
	}
	// Use new SQL template engine for DDL generation
	sqlEngine := sqltemplate.NewSQLTemplateEngine()

	// Get TimescaleDB maintenance configuration
	retentionMonths := 2
	fallbackMonths := 3

	if s.partitionConfig != nil {
		// Convert retention days to months for compatibility
		retentionMonths = 2                                   // Default for vector stores
		fallbackMonths = s.partitionConfig.RetentionDays / 30 // Convert days to months

		if s.logger != nil {
			s.logger.Debug("Using TimescaleDB maintenance configuration for vector store table",
				"table", s.table,
				"schema", s.schema,
				"retention_months", retentionMonths,
				"fallback_months", fallbackMonths,
				"config_enabled", s.partitionConfig.Enabled,
				"config_retention_days", s.partitionConfig.RetentionDays)
		}
	} else {
		if s.logger != nil {
			s.logger.Debug("No TimescaleDB maintenance configuration, using defaults for vector store table",
				"table", s.table,
				"schema", s.schema,
				"retention_months", retentionMonths,
				"fallback_months", fallbackMonths)
		}
	}

	// Process the SQL template with TimescaleDB configuration
	processedSQL := sqlEngine.ProcessTimescaleTemplate(pgvectorStoreSQL, sqltemplate.TimescaleConfig{
		Enabled:         true, // Always enabled for TimescaleDB
		MonthsAhead:     retentionMonths,
		RetentionMonths: fallbackMonths,
	})

	// Execute the processed template with parameters
	params := PgvectorDDLParams{
		Schema:          s.schema,
		Table:           s.table,
		Dimension:       dim,
		RetentionMonths: retentionMonths,
		FallbackMonths:  fallbackMonths,
	}

	// Use enhanced DDL execution with validation
	if err := sqlEngine.ExecuteDDLWithValidation(ctx, s.db, "pgvector", processedSQL, params); err != nil {
		return fmt.Errorf("failed to execute pgvector DDL with validation: %w", err)
	}

	return nil
}

// Helper to extract or set timestamp_ns in metadata
func ensureTimestampNS(meta map[string]any) int64 {
	if v, ok := meta["timestamp_ns"]; ok {
		switch t := v.(type) {
		case int64:
			return t
		case float64:
			return int64(t)
		case int:
			return int64(t)
		case json.Number:
			i, _ := t.Int64()
			return i
		case string:
			if i, err := strconv.ParseInt(t, 10, 64); err == nil {
				return i
			}
		}
	}
	now := time.Now().UnixNano()
	meta["timestamp_ns"] = now
	return now
}

// Implement EmbeddingInfoProvider for PGVectorCustomStore
func (s *PGVectorCustomStore) EmbeddingModel() string {
	if s.EmbProvider != nil {
		if info, ok := s.EmbProvider.(EmbeddingInfoProvider); ok {
			return info.EmbeddingModel()
		}
	}
	return ""
}
func (s *PGVectorCustomStore) EmbeddingDimension() int {
	return s.embeddingDimension
}

// GetVectorByID retrieves a vector by ID using direct database query
func (s *PGVectorCustomStore) GetVectorByID(ctx context.Context, id string) ([]float32, error) {
	query := fmt.Sprintf(`
		SELECT embedding::text 
		FROM %s.%s 
		WHERE id = $1
	`, s.schema, s.table)

	var embeddingStr string
	err := s.db.QueryRowContext(ctx, query, id).Scan(&embeddingStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get vector for ID %s: %w", id, err)
	}

	// Parse the pgvector string format
	embedding, err := s.parsePGVectorString(embeddingStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse vector for ID %s: %w", id, err)
	}

	return s.validateAndNormalizeVector(embedding)
}

// GetVectorsByIDs retrieves multiple vectors by IDs efficiently
func (s *PGVectorCustomStore) GetVectorsByIDs(ctx context.Context, ids []string) ([][]float32, error) {
	if len(ids) == 0 {
		return [][]float32{}, nil
	}

	// Build query with placeholders
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT embedding::text 
		FROM %s.%s 
		WHERE id IN (%s)
		ORDER BY array_position(ARRAY[%s], id)
	`, s.schema, s.table, strings.Join(placeholders, ","), strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get vectors for IDs: %w", err)
	}
	defer rows.Close()

	vectors := make([][]float32, 0, len(ids))
	for rows.Next() {
		var embeddingStr string
		if err := rows.Scan(&embeddingStr); err != nil {
			return nil, fmt.Errorf("failed to scan vector: %w", err)
		}

		// Parse the pgvector string format: "[1.0,2.0,3.0]"
		embedding, err := s.parsePGVectorString(embeddingStr)
		if err != nil {
			s.logger.Error("Failed to parse pgvector string", "error", err, "vector_string", embeddingStr)
			continue
		}

		validatedVector, err := s.validateAndNormalizeVector(embedding)
		if err != nil {
			s.logger.Error("Failed to validate vector", "error", err)
			continue
		}
		vectors = append(vectors, validatedVector)
	}

	return vectors, nil
}

// parsePGVectorString parses a pgvector string format like "[1.0,2.0,3.0]" into []float32
func (s *PGVectorCustomStore) parsePGVectorString(vectorStr string) ([]float32, error) {
	// Remove brackets and split by comma
	vectorStr = strings.TrimSpace(vectorStr)
	if !strings.HasPrefix(vectorStr, "[") || !strings.HasSuffix(vectorStr, "]") {
		return nil, fmt.Errorf("invalid pgvector format: %s", vectorStr)
	}

	vectorStr = strings.Trim(vectorStr, "[]")
	if vectorStr == "" {
		return []float32{}, nil
	}

	// Split by comma and parse each float
	parts := strings.Split(vectorStr, ",")
	vector := make([]float32, len(parts))

	for i, part := range parts {
		part = strings.TrimSpace(part)
		val, err := strconv.ParseFloat(part, 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse vector element %d '%s': %w", i, part, err)
		}
		vector[i] = float32(val)
	}

	return vector, nil
}

// GetVectorMetadata retrieves metadata for a vector by ID
func (s *PGVectorCustomStore) GetVectorMetadata(ctx context.Context, id string) (map[string]interface{}, error) {
	query := fmt.Sprintf(`
		SELECT metadata 
		FROM %s.%s 
		WHERE id = $1
	`, s.schema, s.table)

	var metadataJSON []byte
	err := s.db.QueryRowContext(ctx, query, id).Scan(&metadataJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for ID %s: %w", id, err)
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal(metadataJSON, &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return metadata, nil
}

// CombineVectors combines multiple vectors using the specified strategy
func (s *PGVectorCustomStore) CombineVectors(ctx context.Context, vectors [][]float32, strategy string) ([]float32, error) {
	startTime := time.Now()

	if len(vectors) == 0 {
		return nil, fmt.Errorf("no vectors to combine")
	}

	// Validate vector dimensions
	vectorDimension := len(vectors[0])
	if vectorDimension == 0 {
		return nil, fmt.Errorf("invalid vector dimension: 0")
	}

	// Check for dimension consistency and quality
	qualityIssues := 0
	for i, vector := range vectors {
		if len(vector) != vectorDimension {
			return nil, fmt.Errorf("vector dimension mismatch: expected %d, got %d at index %d", vectorDimension, len(vector), i)
		}

		// Check for quality issues
		for _, val := range vector {
			if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
				qualityIssues++
			}
		}
	}

	// Check for overflow in concatenation
	if strategy == "concatenation" {
		totalLength := 0
		for _, vector := range vectors {
			if totalLength > 0 && totalLength > (1<<30) { // 1GB limit
				return nil, fmt.Errorf("concatenated vector would be too large: %d dimensions", totalLength)
			}
			totalLength += len(vector)
		}
	}

	var result []float32
	var err error

	switch strategy {
	case "mean":
		result, err = s.meanPoolVectors(vectors, vectorDimension)
	case "max":
		result, err = s.maxPoolVectors(vectors, vectorDimension)
	case "concatenation":
		result, err = s.concatenateVectors(vectors)
	case "weighted_mean":
		result, err = s.weightedMeanVectors(vectors, vectorDimension)
	default:
		s.logger.Debug("Unknown strategy, defaulting to mean", "strategy", strategy)
		result, err = s.meanPoolVectors(vectors, vectorDimension)
	}

	if err != nil {
		return nil, err
	}

	// Record metrics
	duration := time.Since(startTime)
	if len(result) > 0 {
		s.metrics.RecordOperation(len(result), duration, qualityIssues)
	}

	return result, nil
}

// GetVectorCombinationStrategy gets the vector combination strategy from vector store metadata
func (s *PGVectorCustomStore) GetVectorCombinationStrategy() string {
	// Default to "mean" if not specified
	defaultStrategy := "mean"

	// Try to get strategy from vector store metadata
	// This would require extending the VectorStoreClient interface or adding metadata access methods
	// For now, we'll use the default strategy

	s.logger.Debug("Using default vector combination strategy", "strategy", defaultStrategy)
	return defaultStrategy
}

// validateAndNormalizeVector validates and normalizes a vector
func (s *PGVectorCustomStore) validateAndNormalizeVector(vector []float32) ([]float32, error) {
	if len(vector) == 0 {
		return nil, fmt.Errorf("empty vector")
	}

	// Check for invalid values
	invalidCount := 0
	zeroCount := 0
	magnitude := 0.0

	for i, val := range vector {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			invalidCount++
			vector[i] = 0.0 // Replace invalid values with 0
		}
		if val == 0.0 {
			zeroCount++
		}
		magnitude += float64(val * val)
	}

	magnitude = math.Sqrt(magnitude)

	// Log quality issues
	if invalidCount > 0 || zeroCount == len(vector) {
		s.logger.Error("Vector quality issues detected",
			"invalid_values", invalidCount,
			"zero_values", zeroCount,
			"total_dimensions", len(vector))
	}

	// Normalize vector if magnitude is not zero
	if magnitude > 0 {
		for i := range vector {
			vector[i] = float32(float64(vector[i]) / magnitude)
		}
	}

	return vector, nil
}

// meanPoolVectors averages all vectors
func (s *PGVectorCustomStore) meanPoolVectors(vectors [][]float32, dimension int) ([]float32, error) {
	result := make([]float32, dimension)

	for _, vector := range vectors {
		if len(vector) != dimension {
			return nil, fmt.Errorf("vector dimension mismatch: expected %d, got %d", dimension, len(vector))
		}
		for i, val := range vector {
			result[i] += val
		}
	}

	// Average
	for i := range result {
		result[i] /= float32(len(vectors))
	}

	return result, nil
}

// maxPoolVectors takes maximum value for each dimension
func (s *PGVectorCustomStore) maxPoolVectors(vectors [][]float32, dimension int) ([]float32, error) {
	result := make([]float32, dimension)

	for _, vector := range vectors {
		if len(vector) != dimension {
			return nil, fmt.Errorf("vector dimension mismatch: expected %d, got %d", dimension, len(vector))
		}
		for i, val := range vector {
			if val > result[i] {
				result[i] = val
			}
		}
	}

	return result, nil
}

// concatenateVectors appends all vectors end-to-end
func (s *PGVectorCustomStore) concatenateVectors(vectors [][]float32) ([]float32, error) {
	totalLength := 0
	for _, vector := range vectors {
		totalLength += len(vector)
	}

	result := make([]float32, totalLength)
	offset := 0
	for _, vector := range vectors {
		copy(result[offset:], vector)
		offset += len(vector)
	}

	return result, nil
}

// weightedMeanVectors combines vectors with equal weights (can be extended for different weights)
func (s *PGVectorCustomStore) weightedMeanVectors(vectors [][]float32, dimension int) ([]float32, error) {
	// For now, use equal weights (same as mean pooling)
	// This can be extended to use different weights based on span importance
	return s.meanPoolVectors(vectors, dimension)
}
