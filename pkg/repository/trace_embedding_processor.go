package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/tmc/langchaingo/schema"
)

// TraceEmbeddingProcessor handles creating composite trace-level embeddings
type TraceEmbeddingProcessor struct {
	db          OtelDatabaseClient
	vectorStore VectorStoreClient
	logger      util.Logger
	configID    int64
	signalType  string
	metrics     *TraceEmbeddingMetrics
}

// TraceEmbeddingMetrics tracks operations for billing and monitoring
type TraceEmbeddingMetrics struct {
	VectorOperations int64
	VectorDimensions int64
	CombinationTime  time.Duration
	CostEstimate     float64
	mu               sync.RWMutex
}

// NewTraceEmbeddingMetrics creates new metrics tracker
func NewTraceEmbeddingMetrics() *TraceEmbeddingMetrics {
	return &TraceEmbeddingMetrics{}
}

// RecordOperation records a vector operation for billing
func (m *TraceEmbeddingMetrics) RecordOperation(dimensions int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.VectorOperations++
	m.VectorDimensions += int64(dimensions)
	m.CombinationTime += duration
	// Simple cost estimation: $0.0001 per vector operation
	m.CostEstimate += 0.0001
}

// NewTraceEmbeddingProcessor creates a new trace embedding processor
func NewTraceEmbeddingProcessor(db OtelDatabaseClient, vectorStore VectorStoreClient, logger util.Logger, configID int64, signalType string) *TraceEmbeddingProcessor {
	return &TraceEmbeddingProcessor{
		db:          db,
		vectorStore: vectorStore,
		logger:      logger,
		configID:    configID,
		signalType:  signalType,
		metrics:     NewTraceEmbeddingMetrics(),
	}
}

// ProcessTraceEmbeddings processes trace-level embeddings for qualifying root spans
func (p *TraceEmbeddingProcessor) ProcessTraceEmbeddings(ctx context.Context, batch []map[string]interface{}) error {
	// Only process for spans signal type
	if p.signalType != "spans" {
		return nil
	}

	p.logger.Info("Starting trace embedding processing",
		"batch_size", len(batch),
		"signal_type", p.signalType)

	// Debug: Log all spans in the batch to understand what we're processing
	for i, sig := range batch {
		spanID, _ := sig["span_id"].(string)
		traceID, _ := sig["trace_id"].(string)
		parentSpanID, _ := sig["parent_span_id"].(string)
		p.logger.Debug("Batch span info",
			"index", i,
			"span_id", spanID,
			"trace_id", traceID,
			"parent_span_id", parentSpanID,
			"has_parent_span_id", parentSpanID != "")
	}

	// Detect qualifying root spans
	qualifyingRootSpans := p.detectQualifyingRootSpans(ctx, batch)
	if len(qualifyingRootSpans) == 0 {
		p.logger.Info("No qualifying root spans found for trace embedding")
		return nil
	}

	p.logger.Info("Processing trace embeddings for root spans",
		"count", len(qualifyingRootSpans),
		"signal_type", p.signalType)

	// Process each qualifying root span sequentially
	successCount := 0
	errorCount := 0

	for _, rootSpan := range qualifyingRootSpans {
		// Check for context cancellation before each processing
		select {
		case <-ctx.Done():
			p.logger.Info("Context cancelled during trace embedding processing",
				"processed_count", successCount,
				"error_count", errorCount)
			return ctx.Err()
		default:
		}

		if err := p.createTraceLevelEmbedding(ctx, rootSpan); err != nil {
			errorCount++
			p.logger.Error("Failed to create trace embedding for root span",
				"span_id", rootSpan["span_id"],
				"trace_id", rootSpan["trace_id"],
				"error", err,
				"error_count", errorCount)
			// Continue with other root spans
			continue
		}

		successCount++
	}

	p.logger.Info("Trace embedding processing completed",
		"total_processed", len(qualifyingRootSpans),
		"success_count", successCount,
		"error_count", errorCount,
		"total_cost_estimate", p.metrics.CostEstimate)

	return nil
}

// detectQualifyingRootSpans identifies root spans that qualify for trace-level embedding
func (p *TraceEmbeddingProcessor) detectQualifyingRootSpans(ctx context.Context, batch []map[string]interface{}) []map[string]interface{} {
	var qualifyingRoots []map[string]interface{}

	for _, sig := range batch {
		// Get span_id and trace_id first for logging
		spanID, ok := sig["span_id"].(string)
		if !ok {
			p.logger.Debug("Skipping signal without span_id")
			continue
		}

		traceID, ok := sig["trace_id"].(string)
		if !ok {
			p.logger.Debug("Skipping signal without trace_id", "span_id", spanID)
			continue
		}

		// Check if this is a root span (no parent_span_id or parent_span_id is empty/nil)
		parentSpanID, hasParent := sig["parent_span_id"]
		if hasParent && parentSpanID != nil && parentSpanID != "" {
			parentStr := fmt.Sprintf("%v", parentSpanID)
			p.logger.Debug("Skipping non-root span - has parent_span_id",
				"span_id", spanID,
				"trace_id", traceID,
				"parent_span_id", parentStr)
			continue // Not a root span
		}

		p.logger.Debug("Found potential root span",
			"span_id", spanID,
			"trace_id", traceID,
			"has_parent_span_id", hasParent,
			"parent_span_id_value", parentSpanID)

		// Query the database to get updated embedding information
		hasEmbedding, hasTraceEmbedding, err := p.checkEmbeddingStatus(ctx, spanID)
		if err != nil {
			p.logger.Error("Failed to check embedding status",
				"span_id", spanID,
				"trace_id", traceID,
				"error", err)
			continue
		}

		if !hasEmbedding {
			p.logger.Debug("No individual embedding found", "span_id", spanID)
			continue // No individual embedding
		}

		if hasTraceEmbedding {
			p.logger.Debug("Already trace-embedded", "span_id", spanID)
			continue // Already trace-embedded
		}

		// Check if has children (query database)
		hasChildren, err := p.hasChildSpans(ctx, traceID, spanID)
		if err != nil {
			p.logger.Error("Failed to check for child spans",
				"span_id", spanID,
				"trace_id", traceID,
				"error", err)
			continue
		}

		if !hasChildren {
			p.logger.Debug("No child spans found", "span_id", spanID, "trace_id", traceID)
			continue // No children, not qualifying
		}

		// All conditions met, add to qualifying list
		qualifyingRoots = append(qualifyingRoots, sig)
		p.logger.Info("Found qualifying root span for trace embedding",
			"span_id", spanID,
			"trace_id", traceID)
	}

	p.logger.Info("Root span detection completed",
		"total_signals_checked", len(batch),
		"qualifying_root_spans", len(qualifyingRoots))

	return qualifyingRoots
}

// checkEmbeddingStatus queries the database to check if a span has individual and trace embeddings
func (p *TraceEmbeddingProcessor) checkEmbeddingStatus(ctx context.Context, spanID string) (hasEmbedding, hasTraceEmbedding bool, err error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, false, ctx.Err()
	default:
	}

	// Try to get the underlying database
	db, ok := p.getUnderlyingDB()
	if !ok {
		// Fallback: assume embedding exists if we can't check
		return true, false, nil
	}

	// Get the schema and construct the proper table name
	schema := p.getSchema()
	fullTableName := fmt.Sprintf("%s.%s", schema, SignalTypeSpans)
	if schema == "" {
		fullTableName = SignalTypeSpans
	}

	// Query the metadata JSONB field for embedding information
	query := fmt.Sprintf(`
		SELECT 
			metadata->'embeddings' as embeddings,
			metadata->'embeddings'->0->>'embedded_id' as embedded_id,
			metadata->'embeddings'->0->>'trace_embedded_id' as trace_embedded_id
		FROM %s 
		WHERE span_id = $1 
		LIMIT 1`, fullTableName)

	var embeddingsData []byte
	var embeddedID sql.NullString
	var traceEmbeddedID sql.NullString

	err = db.QueryRowContext(ctx, query, spanID).Scan(&embeddingsData, &embeddedID, &traceEmbeddedID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, false, nil
		}
		return false, false, fmt.Errorf("failed to query embedding status: %w", err)
	}

	// Check if embeddings array exists and has at least one entry
	hasEmbedding = embeddedID.Valid && embeddedID.String != ""
	hasTraceEmbedding = traceEmbeddedID.Valid && traceEmbeddedID.String != ""

	return hasEmbedding, hasTraceEmbedding, nil
}

// hasChildSpans checks if a span has any child spans in the database
func (p *TraceEmbeddingProcessor) hasChildSpans(ctx context.Context, traceID, parentSpanID string) (bool, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	// Try to get spans using type assertion to OtelDatabaseClient
	if otelDB, ok := p.db.(OtelDatabaseClient); ok {
		spans, err := otelDB.FetchSpansByTraceID(ctx, traceID)
		if err != nil {
			return false, fmt.Errorf("failed to fetch spans for trace %s: %w", traceID, err)
		}

		// Check if any span has this as parent
		for _, span := range spans {
			if span.ParentSpanID != nil && *span.ParentSpanID == parentSpanID {
				return true, nil
			}
		}
		return false, nil
	}

	// Fallback: assume all root spans have children for now
	// This can be enhanced later with direct database queries
	p.logger.Debug("Using fallback child span detection",
		"trace_id", traceID,
		"parent_span_id", parentSpanID)
	return true, nil
}

// GetMetrics returns the current metrics for billing and monitoring
func (p *TraceEmbeddingProcessor) GetMetrics() *TraceEmbeddingMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	// Return a copy to avoid race conditions
	return &TraceEmbeddingMetrics{
		VectorOperations: p.metrics.VectorOperations,
		VectorDimensions: p.metrics.VectorDimensions,
		CombinationTime:  p.metrics.CombinationTime,
		CostEstimate:     p.metrics.CostEstimate,
	}
}

// getSpansForTrace retrieves all spans for a given trace
func (p *TraceEmbeddingProcessor) getSpansForTrace(ctx context.Context, traceID string) ([]SpanData, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Try to get spans using type assertion to OtelDatabaseClient
	if otelDB, ok := p.db.(OtelDatabaseClient); ok {
		return otelDB.FetchSpansByTraceID(ctx, traceID)
	}

	// Fallback: return empty slice if OtelDatabaseClient is not available
	p.logger.Debug("OtelDatabaseClient not available, using fallback", "trace_id", traceID)
	return []SpanData{}, nil
}

// getVectorsForSpans retrieves vectors for spans by getting their embedded_ids from the database
func (p *TraceEmbeddingProcessor) getVectorsForSpans(ctx context.Context, spans []SpanData) ([][]float32, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get embedded_ids from the database for all spans
	embeddedIDs, err := p.getEmbeddedIDsForSpans(ctx, spans)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedded IDs for spans: %w", err)
	}

	if len(embeddedIDs) == 0 {
		return nil, fmt.Errorf("no embedded IDs found for spans")
	}

	// Use batch retrieval for efficiency
	spanVectors, err := p.vectorStore.GetVectorsByIDs(ctx, embeddedIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get vectors for spans: %w", err)
	}

	// Filter out empty vectors
	validVectors := make([][]float32, 0, len(spanVectors))
	for i, vector := range spanVectors {
		if len(vector) > 0 {
			validVectors = append(validVectors, vector)
		} else {
			p.logger.Debug("Empty vector for embedded ID", "embedded_id", embeddedIDs[i])
		}
	}

	return validVectors, nil
}

// getEmbeddedIDsForSpans queries the database to get embedded_ids for the given spans
func (p *TraceEmbeddingProcessor) getEmbeddedIDsForSpans(ctx context.Context, spans []SpanData) ([]string, error) {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Try to get the underlying database
	db, ok := p.getUnderlyingDB()
	if !ok {
		// Fallback: use span IDs as embedded IDs (not ideal but functional)
		spanIDs := make([]string, len(spans))
		for i, span := range spans {
			spanIDs[i] = span.SpanID
		}
		return spanIDs, nil
	}

	// Get the schema and construct the proper table name
	schema := p.getSchema()
	fullTableName := fmt.Sprintf("%s.%s", schema, SignalTypeSpans)
	if schema == "" {
		fullTableName = SignalTypeSpans
	}

	// Build query to get embedded_ids for all spans
	spanIDs := make([]string, len(spans))
	for i, span := range spans {
		spanIDs[i] = span.SpanID
	}

	// Create placeholders for the IN clause
	placeholders := make([]string, len(spanIDs))
	args := make([]interface{}, len(spanIDs))
	for i, spanID := range spanIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = spanID
	}

	query := fmt.Sprintf(`SELECT span_id, metadata->'embeddings'->0->>'embedded_id' as embedded_id FROM %s WHERE span_id IN (%s) AND metadata->'embeddings'->0->>'embedded_id' IS NOT NULL AND metadata->'embeddings'->0->>'embedded_id' != ''`,
		fullTableName, strings.Join(placeholders, ","))

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query embedded IDs: %w", err)
	}
	defer rows.Close()

	// Map span_id to embedded_id
	embeddedIDMap := make(map[string]string)
	for rows.Next() {
		var spanID, embeddedID string
		if err := rows.Scan(&spanID, &embeddedID); err != nil {
			return nil, fmt.Errorf("failed to scan embedded ID row: %w", err)
		}
		embeddedIDMap[spanID] = embeddedID
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating embedded ID rows: %w", err)
	}

	// Return embedded IDs in the same order as input spans
	embeddedIDs := make([]string, 0, len(spans))
	for _, span := range spans {
		if embeddedID, exists := embeddedIDMap[span.SpanID]; exists {
			embeddedIDs = append(embeddedIDs, embeddedID)
		} else {
			p.logger.Debug("No embedded ID found for span", "span_id", span.SpanID)
		}
	}

	return embeddedIDs, nil
}

// createTraceLevelEmbedding creates a composite vector for an entire trace
func (p *TraceEmbeddingProcessor) createTraceLevelEmbedding(ctx context.Context, rootSpan map[string]interface{}) error {
	startTime := time.Now()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	traceID, ok := rootSpan["trace_id"].(string)
	if !ok {
		return fmt.Errorf("invalid trace_id in root span")
	}

	spanID, ok := rootSpan["span_id"].(string)
	if !ok {
		return fmt.Errorf("invalid span_id in root span")
	}

	// Get all spans for this trace
	spans, err := p.getSpansForTrace(ctx, traceID)
	if err != nil {
		return fmt.Errorf("failed to get spans for trace %s: %w", traceID, err)
	}

	if len(spans) == 0 {
		return fmt.Errorf("no spans found for trace %s", traceID)
	}

	// Get vectors for all spans in this trace
	spanVectors, err := p.getVectorsForSpans(ctx, spans)
	if err != nil {
		return fmt.Errorf("failed to get vectors for spans in trace %s: %w", traceID, err)
	}

	if len(spanVectors) == 0 {
		return fmt.Errorf("no vectors found for trace %s", traceID)
	}

	// Get vector combination strategy from vector store metadata
	strategy := p.vectorStore.GetVectorCombinationStrategy()

	// Combine vectors with timing and metrics
	combinationStart := time.Now()
	compositeVector, err := p.vectorStore.CombineVectors(ctx, spanVectors, strategy)
	combinationDuration := time.Since(combinationStart)

	if err != nil {
		return fmt.Errorf("failed to combine vectors for trace %s: %w", traceID, err)
	}

	// Record metrics for billing
	if len(compositeVector) > 0 {
		p.metrics.RecordOperation(len(compositeVector), combinationDuration)
		// Log vector combination metrics
		p.logger.Info("Vector combination completed",
			"vector_dimensions", len(compositeVector),
			"combination_time_ms", combinationDuration.Milliseconds())
	}

	p.logger.Info("Creating trace-level embedding",
		"trace_id", traceID,
		"span_count", len(spans),
		"strategy", strategy,
		"vector_dimensions", len(compositeVector),
		"combination_time_ms", combinationDuration.Milliseconds())

	// Create trace-level document
	traceDoc := schema.Document{
		PageContent: p.buildTraceLevelText(traceID, spanID, len(spans)),
		Metadata: map[string]interface{}{
			"id":                  uuid.NewString(), // Generate proper UUID
			"span_id":             "trace",
			"trace_id":            traceID,
			"signal_type":         p.signalType,
			"processor_id":        p.configID,
			"span_count":          len(spans),
			"root_span_id":        spanID,
			"vector_type":         "composite_trace",
			"strategy":            strategy,
			"vector_dimensions":   len(compositeVector),
			"combination_time_ms": combinationDuration.Milliseconds(),
		},
	}

	// Add to vector store
	embeddedIDs, err := p.vectorStore.AddDocuments(ctx, []schema.Document{traceDoc})
	if err != nil {
		return fmt.Errorf("failed to add trace document to vector store: %w", err)
	}

	if len(embeddedIDs) == 0 {
		return fmt.Errorf("no embedded IDs returned for trace document")
	}

	traceEmbeddedID := embeddedIDs[0]

	// Update the root span's trace_embedded_id in the database
	err = p.updateRootSpanTraceEmbeddedID(ctx, spanID, traceEmbeddedID)
	if err != nil {
		return fmt.Errorf("failed to update root span trace_embedded_id: %w", err)
	}

	totalDuration := time.Since(startTime)
	p.logger.Info("Generated trace vector combination",
		"trace_id", traceID,
		"root_span_id", spanID,
		"span_count", len(spans),
		"strategy", strategy,
		"trace_embedded_id", traceEmbeddedID,
		"total_time_ms", totalDuration.Milliseconds(),
		"cost_estimate", p.metrics.CostEstimate)

	return nil
}

// buildTraceLevelText creates text representation for trace-level embedding
func (p *TraceEmbeddingProcessor) buildTraceLevelText(traceID, rootSpanID string, spanCount int) string {
	return fmt.Sprintf("Composite Trace Summary - Trace ID: %s, Root Span: %s, Total Spans: %d",
		traceID, rootSpanID, spanCount)
}

// updateRootSpanTraceEmbeddedID updates the trace_embedded_id for a root span
func (p *TraceEmbeddingProcessor) updateRootSpanTraceEmbeddedID(ctx context.Context, spanID, traceEmbeddedID string) error {
	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Try to update using type assertion to get access to the underlying database
	if sqlDB, ok := p.getUnderlyingDB(); ok {
		// Get the schema and construct the proper table name
		schema := p.getSchema()
		fullTableName := fmt.Sprintf("%s.%s", schema, SignalTypeSpans)
		if schema == "" {
			fullTableName = SignalTypeSpans
		}

		// Update the embeddings metadata to include trace_embedded_id
		query := fmt.Sprintf(`
			UPDATE %s 
			SET metadata = jsonb_set(
				COALESCE(metadata, '{}'::jsonb),
				'{embeddings,0,trace_embedded_id}',
				$2::jsonb
			)
			WHERE span_id = $1
		`, fullTableName)

		_, err := sqlDB.ExecContext(ctx, query, spanID, fmt.Sprintf(`"%s"`, traceEmbeddedID))
		if err != nil {
			return fmt.Errorf("failed to update trace_embedded_id: %w", err)
		}

		p.logger.Debug("Updated trace_embedded_id in database",
			"span_id", spanID,
			"trace_embedded_id", traceEmbeddedID)
		return nil
	}

	// Fallback: just log the update
	p.logger.Info("Database update not available, logging trace_embedded_id update",
		"span_id", spanID,
		"trace_embedded_id", traceEmbeddedID)
	return nil
}

// getUnderlyingDB attempts to get the underlying *sql.DB from the database client
func (p *TraceEmbeddingProcessor) getUnderlyingDB() (*sql.DB, bool) {
	// Try to get the underlying database
	typedDB, ok := p.db.(*OtelSQLDatabaseClient)
	if !ok {
		return nil, false
	}
	return typedDB.SQLDatabaseClient.DB, true
}

// getSchema returns the database schema
func (p *TraceEmbeddingProcessor) getSchema() string {
	// Try to get the schema from the database client
	typedDB, ok := p.db.(*OtelSQLDatabaseClient)
	if !ok {
		// Fallback to default schema
		return DefaultSchema
	}
	if typedDB.SQLDatabaseClient.Schema == "" {
		return DefaultSchema
	}
	return typedDB.SQLDatabaseClient.Schema
}
