package repository

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"

	"database/sql"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
	qdrantclient "github.com/qdrant/go-client/qdrant"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/schema"
	"github.com/tmc/langchaingo/vectorstores"
	"github.com/tmc/langchaingo/vectorstores/pinecone"
	"github.com/tmc/langchaingo/vectorstores/qdrant"
)

type LangChainEmbeddingRepository struct {
	Store vectorstores.VectorStore
}

// NewVectorStore creates a vector store from configuration.
//
// For the "pgvector" provider:
// - Connection pool settings (max_open_conns, max_idle_conns, conn_max_lifetime) are resolved in this order:
//  1. From the vector store config's metadata (if present)
//  2. From environment variables (TRAZR_DB_MAX_OPEN_CONNS, etc.)
//  3. From built-in defaults (see util.DefaultDBMaxOpenConns, etc.)
//
// - Passwords are resolved from metadata using the "pgstore_password" key, which should reference an environment variable (e.g., "env:TRAZR_DB_PASSWORD").
// - This allows per-store, per-environment, or global tuning of connection settings and secrets.
//
// - Dimension validation: The embedding config's Dimension field is used for table creation, with validation against the actual provider dimension.
//
// - TimescaleDB maintenance: The partitionConfig parameter controls TimescaleDB maintenance for pgvector stores.
func NewVectorStore(ctx context.Context, schema string, vectorstoreName string, config database.VectorStoreConfig, embProvider EmbeddingProvider, embConfig *database.EmbeddingConfig, mainDB *sql.DB, logger util.Logger, partitionConfig *database.TimescaleDBMaintenanceConfig) (VectorStoreClient, error) {

	switch config.Provider {
	case "postgres":
		db, useExternalPGStore, err := openPGVectorDatabase(ctx, config, mainDB, logger)
		if err != nil {
			return nil, err
		}

		tableName := "pgvector_" + vectorstoreName

		// Get dimension from embedding config, with validation
		dimension := 0
		if embConfig != nil && embConfig.Dimension > 0 {
			dimension = embConfig.Dimension
		}

		// Validate dimension against actual provider if possible
		if dimension > 0 && embProvider != nil {
			// Test the provider to get actual dimension
			testVec, err := embProvider.EmbedQuery(ctx, "dimension_test")
			if err == nil && len(testVec) > 0 {
				actualDimension := len(testVec)
				if dimension != actualDimension {
					if logger != nil {
						logger.Error("Dimension mismatch detected",
							"config_dimension", dimension,
							"provider_dimension", actualDimension,
							"embedding_config", embConfig.Name,
							"provider", embConfig.Provider,
							"model", embConfig.Model)
					}
					// Use the actual dimension from provider for table creation
					dimension = actualDimension
				}
			}
		}

		store := New(
			WithDB(db),
			WithSchema(schema),
			WithTable(tableName),
			WithEmbedder(embProvider),
			WithEmbeddingDimension(dimension),
			WithExternalPGStore(useExternalPGStore),
			WithPartitionConfig(partitionConfig),
		)
		if logger != nil {
			store.logger = logger
		}
		// Set pool settings from config (not metadata)
		db.SetMaxOpenConns(config.MaxOpenConns)
		db.SetMaxIdleConns(config.MaxIdleConns)
		db.SetConnMaxLifetime(config.ConnMaxLifetime)
		return store, nil

	case "pinecone":
		apiKey := util.GetSecretFromMetadata(config.Metadata, "vectorstore_api_key")
		if apiKey == "" {
			return nil, fmt.Errorf("pinecone vectorstore: no api key")
		}

		// Use schema and table from config for vector store
		namespace := schema + "_" + vectorstoreName

		store, err := pinecone.New(
			pinecone.WithAPIKey(apiKey),
			pinecone.WithHost(config.Endpoint),
			pinecone.WithNameSpace(namespace),
			pinecone.WithEmbedder(embProvider.(embeddings.Embedder)),
		)
		if err != nil {
			return nil, err
		}
		return &LangChainEmbeddingRepository{Store: store}, nil

	case "qdrant":
		apiKey := util.GetSecretFromMetadata(config.Metadata, "vectorstore_api_key")
		if apiKey == "" {
			return nil, fmt.Errorf("qdrant vectorstore: no api key")
		}

		parsedURL, err := url.Parse(config.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid Qdrant endpoint: %w", err)
		}
		collectionName := schema + "_" + vectorstoreName

		// Create the native Qdrant Go client
		qdrantHost := parsedURL.Hostname()
		qdrantPort := parsedURL.Port()
		if qdrantPort == "" {
			qdrantPort = "6334"
		}
		qdrantPortInt, err := strconv.Atoi(qdrantPort)
		if err != nil {
			return nil, fmt.Errorf("invalid Qdrant port: %w", err)
		}
		qdrantCfg := &qdrantclient.Config{
			Host:   qdrantHost,
			Port:   qdrantPortInt,
			APIKey: apiKey,
			UseTLS: false,
		}

		nativeClient, err := qdrantclient.NewClient(qdrantCfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create native Qdrant client: %w", err)
		}

		store, err := qdrant.New(
			qdrant.WithURL(*parsedURL),
			qdrant.WithAPIKey(apiKey),
			qdrant.WithCollectionName(collectionName),
			qdrant.WithEmbedder(embProvider.(embeddings.Embedder)),
		)
		if err != nil {
			return nil, err
		}

		// Get vector size from embedding config if available
		vectorSize := 768 // default fallback
		if embConfig != nil && embConfig.Dimension > 0 {
			vectorSize = embConfig.Dimension
		} else if embProvider != nil {
			// Test the provider to get actual dimension
			testVec, err := embProvider.EmbedQuery(ctx, "dimension_test")
			if err == nil && len(testVec) > 0 {
				vectorSize = len(testVec)
			}
		}

		distance := "Cosine"
		if err := EnsureQdrantCollection(ctx, nativeClient, collectionName, vectorSize, distance); err != nil {
			return nil, fmt.Errorf("failed to ensure Qdrant collection: %w", err)
		}

		return &LangChainEmbeddingRepository{Store: &QdrantStoreWithClient{VectorStore: store, Native: nativeClient}}, nil
		// TODO: Implement Weaviate and Chroma
		/*
			case "weaviate":
				var apiKey string
				if v, ok := config.Metadata["secret_api_key"].(string); ok {
					apiKey = util.GetSecret(v)
				}
				if apiKey == "" {
					return nil, fmt.Errorf("weaviate vectorstore: no api key")
				}

				store, err := weaviate.New(
					weaviate.WithURL(config.Endpoint),
					weaviate.WithAPIKey(apiKey),
					weaviate.WithNameSpace(schema + "_" + config.Name + "_" + strconv.FormatInt(config.ID, 10)),
					weaviate.WithEmbedder(embProvider.(embeddings.Embedder)),
				)
				if err != nil {
					return nil, err
				}
				return &LangChainEmbeddingRepository{Store: store}, nil

			case "chroma":
				store, err := chroma.New(
					chroma.WithURL(config.Endpoint),
					chroma.WithNameSpace(schema + "_" + config.Name + "_" + strconv.FormatInt(config.ID, 10)),
					chroma.WithEmbedder(embProvider.(embeddings.Embedder)),
				)
				if err != nil {
					return nil, err
				}
				return &LangChainEmbeddingRepository{Store: store}, nil
		*/

	default:
		return nil, fmt.Errorf("unsupported vector store provider: %s", config.Provider)
	}
}

func (r *LangChainEmbeddingRepository) DeleteEmbedding(ctx context.Context, embeddingID string) error {
	// If the underlying VectorStore supports deletion, call it; otherwise, return nil or an error.
	return nil // TODO: implement if supported
}

func (r *LangChainEmbeddingRepository) HealthCheck(ctx context.Context) error {
	// If the underlying VectorStore supports health checks, call it; otherwise, return nil
	return nil // TODO: implement if supported
}

func (r *LangChainEmbeddingRepository) Close() error {
	// If the underlying VectorStore supports closing, call it; otherwise, return nil
	return nil // TODO: implement if supported
}

// AddDocuments implements VectorStoreClient by delegating to the underlying Store.
func (r *LangChainEmbeddingRepository) AddDocuments(ctx context.Context, docs []schema.Document) ([]string, error) {
	return r.Store.AddDocuments(ctx, docs)
}

// SimilaritySearch implements VectorStoreClient by delegating to the underlying Store.
func (r *LangChainEmbeddingRepository) SimilaritySearch(ctx context.Context, query string, k int, opts ...interface{}) ([]schema.Document, error) {
	// Only pass supported options (vectorstores.Option)
	var options []vectorstores.Option
	for _, o := range opts {
		if opt, ok := o.(vectorstores.Option); ok {
			options = append(options, opt)
		}
	}
	return r.Store.SimilaritySearch(ctx, query, k, options...)
}

// SimilaritySearchByVector performs similarity search using a vector query
func (r *LangChainEmbeddingRepository) SimilaritySearchByVector(ctx context.Context, query []float32, k int, opts ...interface{}) ([]schema.Document, error) {
	// LangChain VectorStore doesn't have SimilaritySearchByVector, so we'll return an error
	return nil, fmt.Errorf("SimilaritySearchByVector is not supported by LangChain VectorStore")
}

// GetVectorByID retrieves a vector by ID using LangChain's similarity search
func (r *LangChainEmbeddingRepository) GetVectorByID(ctx context.Context, id string) ([]float32, error) {
	// Use similarity search with exact ID match as a workaround
	query := fmt.Sprintf("id:%s", id)
	docs, err := r.Store.SimilaritySearch(ctx, query, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to search for vector with ID %s: %w", id, err)
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("no document found for ID %s", id)
	}

	// Extract vector from document metadata
	if vectorData, ok := docs[0].Metadata["embedding"]; ok {
		if vector, ok := vectorData.([]float32); ok {
			return r.validateAndNormalizeVector(vector)
		}
		if vector, ok := vectorData.([]interface{}); ok {
			return r.convertInterfaceSliceToFloat32(vector)
		}
	}

	return nil, fmt.Errorf("no vector found in document metadata for ID %s", id)
}

// GetVectorsByIDs retrieves multiple vectors by IDs efficiently
func (r *LangChainEmbeddingRepository) GetVectorsByIDs(ctx context.Context, ids []string) ([][]float32, error) {
	if len(ids) == 0 {
		return [][]float32{}, nil
	}

	vectors := make([][]float32, 0, len(ids))
	errors := make([]string, 0)

	// Process vectors in batches to avoid overwhelming the vector store
	batchSize := 10
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}

		batch := ids[i:end]
		batchVectors, err := r.getVectorsBatch(ctx, batch)
		if err != nil {
			errors = append(errors, fmt.Sprintf("batch %d-%d: %v", i, end-1, err))
			continue
		}

		vectors = append(vectors, batchVectors...)
	}

	if len(errors) > 0 {
		// Log errors but don't fail completely
		fmt.Printf("Some vector retrievals failed: %v\n", errors)
	}

	return vectors, nil
}

// getVectorsBatch retrieves a batch of vectors
func (r *LangChainEmbeddingRepository) getVectorsBatch(ctx context.Context, ids []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(ids))

	for _, id := range ids {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		vector, err := r.GetVectorByID(ctx, id)
		if err != nil {
			// Log error but continue with other vectors
			fmt.Printf("Failed to get vector for ID %s: %v\n", id, err)
			continue
		}

		vectors = append(vectors, vector)
	}

	return vectors, nil
}

// GetVectorMetadata retrieves metadata for a vector by ID
func (r *LangChainEmbeddingRepository) GetVectorMetadata(ctx context.Context, id string) (map[string]interface{}, error) {
	query := fmt.Sprintf("id:%s", id)
	docs, err := r.Store.SimilaritySearch(ctx, query, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to search for metadata with ID %s: %w", id, err)
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("no document found for ID %s", id)
	}

	return docs[0].Metadata, nil
}

// CombineVectors combines multiple vectors using the specified strategy
func (r *LangChainEmbeddingRepository) CombineVectors(ctx context.Context, vectors [][]float32, strategy string) ([]float32, error) {
	if len(vectors) == 0 {
		return nil, fmt.Errorf("no vectors to combine")
	}

	// Validate vector dimensions
	vectorDimension := len(vectors[0])
	if vectorDimension == 0 {
		return nil, fmt.Errorf("invalid vector dimension: 0")
	}

	// Check for dimension consistency
	for i, vector := range vectors {
		if len(vector) != vectorDimension {
			return nil, fmt.Errorf("vector dimension mismatch: expected %d, got %d at index %d", vectorDimension, len(vector), i)
		}
	}

	switch strategy {
	case "mean":
		return r.meanPoolVectors(vectors, vectorDimension)
	case "max":
		return r.maxPoolVectors(vectors, vectorDimension)
	case "concatenation":
		return r.concatenateVectors(vectors)
	case "weighted_mean":
		return r.weightedMeanVectors(vectors, vectorDimension)
	default:
		// Default to mean pooling
		return r.meanPoolVectors(vectors, vectorDimension)
	}
}

// GetVectorCombinationStrategy gets the vector combination strategy
func (r *LangChainEmbeddingRepository) GetVectorCombinationStrategy() string {
	// Default to "mean" if not specified
	return "mean"
}

// validateAndNormalizeVector validates and normalizes a vector
func (r *LangChainEmbeddingRepository) validateAndNormalizeVector(vector []float32) ([]float32, error) {
	if len(vector) == 0 {
		return nil, fmt.Errorf("empty vector")
	}

	// Check for invalid values and normalize
	magnitude := 0.0
	for i, val := range vector {
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			vector[i] = 0.0 // Replace NaN/Inf with 0
		}
		magnitude += float64(val * val)
	}

	magnitude = float64(magnitude)
	if magnitude > 0 {
		for i := range vector {
			vector[i] = float32(float64(vector[i]) / magnitude)
		}
	}

	return vector, nil
}

// convertInterfaceSliceToFloat32 converts []interface{} to []float32
func (r *LangChainEmbeddingRepository) convertInterfaceSliceToFloat32(data []interface{}) ([]float32, error) {
	vector := make([]float32, len(data))
	for i, v := range data {
		switch val := v.(type) {
		case float64:
			vector[i] = float32(val)
		case float32:
			vector[i] = val
		case int:
			vector[i] = float32(val)
		case int64:
			vector[i] = float32(val)
		default:
			return nil, fmt.Errorf("unsupported type for vector element %d: %T", i, v)
		}
	}
	return r.validateAndNormalizeVector(vector)
}

// meanPoolVectors averages all vectors
func (r *LangChainEmbeddingRepository) meanPoolVectors(vectors [][]float32, dimension int) ([]float32, error) {
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
func (r *LangChainEmbeddingRepository) maxPoolVectors(vectors [][]float32, dimension int) ([]float32, error) {
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
func (r *LangChainEmbeddingRepository) concatenateVectors(vectors [][]float32) ([]float32, error) {
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
func (r *LangChainEmbeddingRepository) weightedMeanVectors(vectors [][]float32, dimension int) ([]float32, error) {
	// For now, use equal weights (same as mean pooling)
	return r.meanPoolVectors(vectors, dimension)
}

// FindBaselineByContentHash finds existing embeddings with the same content hash
// Note: LangChain stores don't have native content hash support, so we use content matching
func (r *LangChainEmbeddingRepository) FindBaselineByContentHash(ctx context.Context, content string) ([]interface{}, error) {
	// For LangChain stores, we'll search by content similarity as a fallback
	// This is not as efficient as hash-based lookup but provides baseline functionality
	docs, err := r.SimilaritySearch(ctx, content, 10)
	if err != nil {
		return nil, fmt.Errorf("baseline search failed: %w", err)
	}

	// Filter for exact content matches
	var baselineDocs []interface{}
	for _, doc := range docs {
		if doc.PageContent == content {
			baselineDocs = append(baselineDocs, Document{
				ID:          doc.Metadata["id"].(string),
				PageContent: doc.PageContent,
				Metadata:    doc.Metadata,
			})
		}
	}

	return baselineDocs, nil
}

// NativeClientProvider exposes the underlying native client for advanced operations (e.g., collection management).
type NativeClientProvider interface {
	// NativeClient returns the underlying client (e.g., Qdrant, Chroma, Pinecone, etc.) or nil if not supported.
	NativeClient() any
}

// Ensure LangChainEmbeddingRepository implements NativeClientProvider for supported stores.
func (r *LangChainEmbeddingRepository) NativeClient() any {
	switch store := r.Store.(type) {
	case interface{ NativeClient() any }:
		return store.NativeClient()
	default:
		return nil
	}
}

// Example: Implement NativeClient for Qdrant, Chroma, Pinecone, Weaviate wrappers if needed.
// For demonstration, here is a Qdrant example. Repeat for others as needed.

// ... in your Qdrant wrapper (if you have a custom type):
// func (q *QdrantStore) NativeClient() any {
// 	return q.client // underlying Qdrant Go client
// }

// For PGVectorCustomStore, you may want to expose the *sql.DB:
// func (s *PGVectorCustomStore) NativeClient() any {
// 	return s.db
// }

// QdrantCollectionManager provides collection management for Qdrant vector stores.
// Only available if the underlying store is Qdrant.
type QdrantCollectionManager interface {
	EnsureCollection(ctx context.Context, collectionName string, vectorSize int, distance string) error
}

// EnsureQdrantCollection checks if a Qdrant collection exists and creates it if not.
// Requires github.com/qdrant/go-client as a dependency.
func EnsureQdrantCollection(ctx context.Context, client any, collectionName string, vectorSize int, distance string) error {
	qdrantClient, ok := client.(*qdrantclient.Client)
	if !ok {
		return fmt.Errorf("provided client does not support Qdrant collection management")
	}

	dist := qdrantclient.Distance_Cosine
	if distance == "Euclid" {
		dist = qdrantclient.Distance_Euclid
	} else if distance == "Dot" {
		dist = qdrantclient.Distance_Dot
	}
	// Always try to create the collection; if it already exists, treat as success
	err := qdrantClient.CreateCollection(ctx, &qdrantclient.CreateCollection{
		CollectionName: collectionName,
		VectorsConfig: qdrantclient.NewVectorsConfig(&qdrantclient.VectorParams{
			Size:     uint64(vectorSize),
			Distance: dist,
		}),
	})
	if err != nil {
		if err.Error() == "already exists" || err.Error() == "Collection already exists" {
			return nil
		}
		return fmt.Errorf("failed to create Qdrant collection: %w", err)
	}
	return nil
}

// QdrantStoreWithClient wraps the LangChainGo Qdrant store and exposes the native Qdrant Go client.
type QdrantStoreWithClient struct {
	vectorstores.VectorStore
	Native *qdrantclient.Client // native Qdrant Go client
}

func (q *QdrantStoreWithClient) NativeClient() any {
	return q.Native
}
