package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository"
)

// VectorStoreSearchRequest defines the input for a vector search.
type VectorStoreSearchRequest struct {
	Query          string  `json:"query"`
	TopK           int     `json:"top_k"`
	StoreID        string  `json:"store_id"`
	ScoreThreshold float64 `json:"score_threshold,omitempty"` // Optional, default 0.3
}

// VectorStoreSearchResponse defines the output for a vector search.
type VectorStoreSearchResponse struct {
	Results []VectorStoreDocument `json:"results"`
}

type VectorStoreDocument struct {
	ID        string      `json:"id"`
	Content   interface{} `json:"content"`
	Score     float64     `json:"score"`
	Timestamp *string     `json:"timestamp,omitempty"`
}

// VectorStoreSearchService defines the interface for searching a vector store.
type VectorStoreSearchService interface {
	Search(ctx context.Context, req VectorStoreSearchRequest) (VectorStoreSearchResponse, error)
}

// Implementation using repository.VectorStoreClient

type vectorStoreSearchService struct {
	StoreProvider func(storeID string) (repository.VectorStoreClient, error)
}

func NewVectorStoreSearchService(provider func(storeID string) (repository.VectorStoreClient, error)) VectorStoreSearchService {
	return &vectorStoreSearchService{StoreProvider: provider}
}

func (s *vectorStoreSearchService) Search(ctx context.Context, req VectorStoreSearchRequest) (VectorStoreSearchResponse, error) {
	store, err := s.StoreProvider(req.StoreID)
	if err != nil {
		return VectorStoreSearchResponse{}, err
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}
	scoreThreshold := req.ScoreThreshold
	if scoreThreshold == 0 {
		scoreThreshold = 1.0 // Increased from 0.3 to allow more results through
	}
	results, err := store.SimilaritySearch(ctx, req.Query, req.TopK)
	if err != nil {
		return VectorStoreSearchResponse{}, err
	}
	resp := VectorStoreSearchResponse{Results: make([]VectorStoreDocument, 0, len(results))}
	for _, doc := range results {
		id := ""
		if doc.Metadata != nil {
			if idVal, ok := doc.Metadata["id"].(string); ok {
				id = idVal
			}
		}
		score := float64(doc.Score)
		if doc.Metadata != nil {
			if sVal, ok := doc.Metadata["score"].(float64); ok {
				score = sVal
			}
		}
		var timestamp *string
		if doc.Metadata != nil {
			if tsVal, ok := doc.Metadata["timestamp"]; ok {
				switch v := tsVal.(type) {
				case string:
					timestamp = &v
				case int64:
					ts := time.Unix(v, 0).UTC().Format(time.RFC3339)
					timestamp = &ts
				case float64:
					ts := time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
					timestamp = &ts
				}
			}
		}
		// Note: Lower scores are better matches (more similar)
		// So we want to include results with scores LESS than the threshold
		if score < scoreThreshold {

			resp.Results = append(resp.Results, VectorStoreDocument{
				ID:        id,
				Content:   doc.PageContent,
				Score:     score,
				Timestamp: timestamp,
			})
		}
	}
	return resp, nil
}

// HTTP handler for the API
func NewVectorStoreSearchHandler(svc VectorStoreSearchService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req VectorStoreSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request: " + err.Error()})
			return
		}
		ctx := r.Context()
		resp, err := svc.Search(ctx, req)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// NOTE: Vector store listing/management is now handled through the config system
// at /api/v1/config/vectorstores for proper admin access control
