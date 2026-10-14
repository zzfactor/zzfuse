package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/tmc/langchaingo/schema"
)

// ChromaVectorStore implements VectorStoreClient using a FastAPI Chroma integration.
type ChromaVectorStore struct {
	Endpoint   string            // FastAPI base URL (e.g., http://localhost:8000)
	ChromaURL  string            // Chroma server URL (e.g., http://localhost:8010)
	Collection string            // Collection name (namespace)
	APIKey     string            // API key for FastAPI
	Client     *http.Client      // HTTP client
	Headers    map[string]string // Extra headers
	Timeout    time.Duration     // Request timeout
}

// ChromaOption is a functional option for ChromaVectorStore.
type ChromaOption func(*ChromaVectorStore)

// WithChromaHTTPClient sets a custom HTTP client.
func WithChromaHTTPClient(client *http.Client) ChromaOption {
	return func(s *ChromaVectorStore) { s.Client = client }
}

// WithChromaHeaders sets custom headers.
func WithChromaHeaders(headers map[string]string) ChromaOption {
	return func(s *ChromaVectorStore) { s.Headers = headers }
}

// WithChromaTimeout sets a custom timeout.
func WithChromaTimeout(timeout time.Duration) ChromaOption {
	return func(s *ChromaVectorStore) { s.Timeout = timeout }
}

// NewChromaVectorStore creates a new ChromaVectorStore with options.
func NewChromaVectorStore(endpoint, chromaURL, collection string, opts ...ChromaOption) *ChromaVectorStore {
	apiKey := os.Getenv("CHROMA_API_KEY")
	if apiKey == "" {
		apiKey = "your-api-key-here"
	}
	// DEBUG: Hardcoded API key for troubleshooting. Revert to env-based for production!
	apiKey = "your-api-key-here"
	s := &ChromaVectorStore{
		Endpoint:   endpoint,
		ChromaURL:  chromaURL,
		Collection: collection,
		APIKey:     apiKey,
		Client:     http.DefaultClient,
		Headers:    map[string]string{"X-API-Key": apiKey},
		Timeout:    10 * time.Second,
	}
	for _, opt := range opts {
		opt(s)
	}
	if s.Headers == nil {
		s.Headers = map[string]string{"X-API-Key": apiKey}
	} else if _, ok := s.Headers["X-API-Key"]; !ok {
		s.Headers["X-API-Key"] = apiKey
	}
	return s
}

// AddDocumentsRequest matches the FastAPI model.
type AddDocumentsRequest struct {
	Collection string           `json:"collection"`
	Documents  []string         `json:"documents"`
	IDs        []string         `json:"ids"`
	Metadatas  []map[string]any `json:"metadatas"`
	Embeddings [][]float32      `json:"embeddings,omitempty"`
}

// QueryRequest matches the FastAPI model.
type QueryRequest struct {
	Collection      string         `json:"collection"`
	QueryTexts      []string       `json:"query_texts,omitempty"`
	QueryEmbeddings [][]float32    `json:"query_embeddings,omitempty"`
	NResults        int            `json:"n_results"`
	Where           map[string]any `json:"where,omitempty"`
	WhereDocument   map[string]any `json:"where_document,omitempty"`
	Include         []string       `json:"include,omitempty"`
}

// AddDocuments adds documents to the Chroma collection.
func (s *ChromaVectorStore) AddDocuments(ctx context.Context, docs []schema.Document) ([]string, error) {
	ids := make([]string, len(docs))
	documents := make([]string, len(docs))
	metadatas := make([]map[string]any, len(docs))
	for i, doc := range docs {
		documents[i] = doc.PageContent
		metadatas[i] = doc.Metadata
		if id, ok := doc.Metadata["id"].(string); ok && id != "" {
			ids[i] = id
		} else {
			ids[i] = fmt.Sprintf("doc-%d-%d", time.Now().UnixNano(), i)
		}
	}
	payload := AddDocumentsRequest{
		Collection: s.Collection,
		Documents:  documents,
		IDs:        ids,
		Metadatas:  metadatas,
	}
	url := fmt.Sprintf("%s/chroma/add-documents?chroma_url=%s", s.Endpoint, s.ChromaURL)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("chroma: marshal add-documents: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("chroma: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chroma: http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chroma: add-documents error: %s: %s", resp.Status, string(b))
	}
	var out struct {
		Status string `json:"status"`
		Count  int    `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("chroma: decode add-documents: %w", err)
	}
	return ids, nil
}

// SimilaritySearch performs a text-based similarity search.
func (s *ChromaVectorStore) SimilaritySearch(ctx context.Context, query string, k int, opts ...interface{}) ([]schema.Document, error) {
	payload := QueryRequest{
		Collection: s.Collection,
		QueryTexts: []string{query},
		NResults:   k,
		Include:    []string{"documents", "metadatas", "embeddings"},
	}
	url := fmt.Sprintf("%s/chroma/query?chroma_url=%s", s.Endpoint, s.ChromaURL)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("chroma: marshal query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("chroma: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chroma: http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chroma: query error: %s: %s", resp.Status, string(b))
	}
	var out struct {
		Documents  [][]string         `json:"documents"`
		Metadatas  [][]map[string]any `json:"metadatas"`
		Embeddings [][]float32        `json:"embeddings"`
		IDs        [][]string         `json:"ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("chroma: decode query: %w", err)
	}
	// Flatten results
	var docs []schema.Document
	for i := range out.Documents {
		for j := range out.Documents[i] {
			docs = append(docs, schema.Document{
				PageContent: out.Documents[i][j],
				Metadata:    out.Metadatas[i][j],
			})
		}
	}
	return docs, nil
}

// SimilaritySearchByVector performs a vector-based similarity search.
func (s *ChromaVectorStore) SimilaritySearchByVector(ctx context.Context, query []float32, k int, opts ...interface{}) ([]schema.Document, error) {
	payload := QueryRequest{
		Collection:      s.Collection,
		QueryEmbeddings: [][]float32{query},
		NResults:        k,
		Include:         []string{"documents", "metadatas", "embeddings"},
	}
	url := fmt.Sprintf("%s/chroma/query?chroma_url=%s", s.Endpoint, s.ChromaURL)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("chroma: marshal query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("chroma: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.doRequest(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chroma: http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chroma: query error: %s: %s", resp.Status, string(b))
	}
	var out struct {
		Documents  [][]string         `json:"documents"`
		Metadatas  [][]map[string]any `json:"metadatas"`
		Embeddings [][]float32        `json:"embeddings"`
		IDs        [][]string         `json:"ids"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("chroma: decode query: %w", err)
	}
	// Flatten results
	var docs []schema.Document
	for i := range out.Documents {
		for j := range out.Documents[i] {
			docs = append(docs, schema.Document{
				PageContent: out.Documents[i][j],
				Metadata:    out.Metadatas[i][j],
			})
		}
	}
	return docs, nil
}

// HealthCheck checks the health of the Chroma backend.
func (s *ChromaVectorStore) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("%s/chroma/health?chroma_url=%s", s.Endpoint, s.ChromaURL)
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, s.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("chroma: new request: %w", err)
	}
	resp, err := s.doRequest(ctx, req)
	if err != nil {
		return fmt.Errorf("chroma: http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("chroma: health error: %s: %s", resp.Status, string(b))
	}
	return nil
}

// Close closes the ChromaVectorStore.
func (s *ChromaVectorStore) Close() error {
	// No cleanup needed for HTTP client
	return nil
}

// GetVectorByID retrieves a vector by ID using Chroma's get method
func (s *ChromaVectorStore) GetVectorByID(ctx context.Context, id string) ([]float32, error) {
	// Chroma doesn't have a direct get by ID method, so we'll use a query with exact ID match
	query := fmt.Sprintf("id:%s", id)
	docs, err := s.SimilaritySearch(ctx, query, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to search for vector with ID %s: %w", id, err)
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("no document found for ID %s", id)
	}

	// Extract vector from document metadata
	if vectorData, ok := docs[0].Metadata["embedding"]; ok {
		if vector, ok := vectorData.([]float32); ok {
			return s.validateAndNormalizeVector(vector)
		}
		if vector, ok := vectorData.([]interface{}); ok {
			return s.convertInterfaceSliceToFloat32(vector)
		}
	}

	return nil, fmt.Errorf("no vector found in document metadata for ID %s", id)
}

// GetVectorsByIDs retrieves multiple vectors by IDs efficiently
func (s *ChromaVectorStore) GetVectorsByIDs(ctx context.Context, ids []string) ([][]float32, error) {
	if len(ids) == 0 {
		return [][]float32{}, nil
	}

	vectors := make([][]float32, 0, len(ids))
	errors := make([]string, 0)

	// Process vectors in batches to avoid overwhelming the Chroma server
	batchSize := 10
	for i := 0; i < len(ids); i += batchSize {
		end := i + batchSize
		if end > len(ids) {
			end = len(ids)
		}

		batch := ids[i:end]
		batchVectors, err := s.getVectorsBatch(ctx, batch)
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
func (s *ChromaVectorStore) getVectorsBatch(ctx context.Context, ids []string) ([][]float32, error) {
	vectors := make([][]float32, 0, len(ids))

	for _, id := range ids {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		vector, err := s.GetVectorByID(ctx, id)
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
func (s *ChromaVectorStore) GetVectorMetadata(ctx context.Context, id string) (map[string]interface{}, error) {
	query := fmt.Sprintf("id:%s", id)
	docs, err := s.SimilaritySearch(ctx, query, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to search for metadata with ID %s: %w", id, err)
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("no document found for ID %s", id)
	}

	return docs[0].Metadata, nil
}

// CombineVectors combines multiple vectors using the specified strategy
func (s *ChromaVectorStore) CombineVectors(ctx context.Context, vectors [][]float32, strategy string) ([]float32, error) {
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
		return s.meanPoolVectors(vectors, vectorDimension)
	case "max":
		return s.maxPoolVectors(vectors, vectorDimension)
	case "concatenation":
		return s.concatenateVectors(vectors)
	case "weighted_mean":
		return s.weightedMeanVectors(vectors, vectorDimension)
	default:
		// Default to mean pooling
		return s.meanPoolVectors(vectors, vectorDimension)
	}
}

// GetVectorCombinationStrategy gets the vector combination strategy
func (s *ChromaVectorStore) GetVectorCombinationStrategy() string {
	// Default to "mean" if not specified
	return "mean"
}

// validateAndNormalizeVector validates and normalizes a vector
func (s *ChromaVectorStore) validateAndNormalizeVector(vector []float32) ([]float32, error) {
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
func (s *ChromaVectorStore) convertInterfaceSliceToFloat32(data []interface{}) ([]float32, error) {
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
	return s.validateAndNormalizeVector(vector)
}

// meanPoolVectors averages all vectors
func (s *ChromaVectorStore) meanPoolVectors(vectors [][]float32, dimension int) ([]float32, error) {
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
func (s *ChromaVectorStore) maxPoolVectors(vectors [][]float32, dimension int) ([]float32, error) {
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
func (s *ChromaVectorStore) concatenateVectors(vectors [][]float32) ([]float32, error) {
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
func (s *ChromaVectorStore) weightedMeanVectors(vectors [][]float32, dimension int) ([]float32, error) {
	// For now, use equal weights (same as mean pooling)
	return s.meanPoolVectors(vectors, dimension)
}

// FindBaselineByContentHash finds existing embeddings with the same content hash
// Note: Chroma doesn't have native content hash support, so we use content matching
func (s *ChromaVectorStore) FindBaselineByContentHash(ctx context.Context, content string) ([]interface{}, error) {
	// For Chroma, we'll search by content similarity as a fallback
	// This is not as efficient as hash-based lookup but provides baseline functionality
	docs, err := s.SimilaritySearch(ctx, content, 10)
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

func (s *ChromaVectorStore) doRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Always set the X-API-Key header explicitly
	req.Header.Set("X-API-Key", s.APIKey)
	// Set any additional headers
	for k, v := range s.Headers {
		req.Header.Set(k, v)
	}
	// Debug print all headers
	fmt.Println("Go request headers:")
	for k, v := range req.Header {
		fmt.Printf("  %s: %v\n", k, v)
	}
	return s.Client.Do(req)
}
