package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel"
)

type LocalEmbeddingProvider struct {
	Endpoint string
	Timeout  time.Duration
	Client   *http.Client
	Headers  map[string]string
	Model    string
}

// embedRequest is the request payload for the local embedding service.
type embedRequest struct {
	Texts     []string `json:"texts"`
	ModelName string   `json:"model_name,omitempty"`
}

// embedResponse is the response payload from the local embedding service.
type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// LocalOption is a functional option for LocalEmbeddingProvider.
type LocalOption func(*LocalEmbeddingProvider)

// WithHTTPClient sets a custom HTTP client for the provider.
func WithHTTPClient(client *http.Client) LocalOption {
	return func(p *LocalEmbeddingProvider) {
		p.Client = client
	}
}

// WithHeaders sets custom headers for the provider.
func WithHeaders(headers map[string]string) LocalOption {
	return func(p *LocalEmbeddingProvider) {
		p.Headers = headers
	}
}

// WithModel sets the model name for the provider.
func WithModel(model string) LocalOption {
	return func(p *LocalEmbeddingProvider) {
		p.Model = model
	}
}

// EmbedDocuments generates embeddings for a batch of texts, using the provider's default model.
func (p *LocalEmbeddingProvider) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return p.EmbedDocumentsWithModel(ctx, texts, "")
}

// EmbedDocumentsWithModel generates embeddings for a batch of texts, allowing the caller to specify the model name per request.
func (p *LocalEmbeddingProvider) EmbedDocumentsWithModel(ctx context.Context, texts []string, modelName string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	tracer := otel.Tracer("trazrexporter.embedding.local")
	ctx, span := tracer.Start(ctx, "LocalEmbeddingProvider.EmbedDocumentsWithModel")
	defer span.End()
	url := p.Endpoint + "/sentencer/embed"
	// Use modelName parameter if provided, else header, else provider field
	finalModelName := modelName
	if finalModelName == "" && p.Headers != nil {
		if v, ok := p.Headers["X-Embedding-Model"]; ok && v != "" {
			finalModelName = v
		}
	}
	if finalModelName == "" && p.Model != "" {
		finalModelName = p.Model
	}
	payload, err := json.Marshal(embedRequest{Texts: texts, ModelName: finalModelName})
	if err != nil {
		return nil, fmt.Errorf("local embedding: marshal request: %w", err)
	}

	// Set timeout if specified
	if p.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("local embedding: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("local embedding: http post: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("local embedding: service error: %s: %s", resp.Status, string(body))
	}
	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("local embedding: decode response: %w", err)
	}
	if len(out.Embeddings) != len(texts) {
		return nil, fmt.Errorf("local embedding: embedding count mismatch: got %d, want %d", len(out.Embeddings), len(texts))
	}
	return out.Embeddings, nil
}

func (p *LocalEmbeddingProvider) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	tracer := otel.Tracer("trazrexporter.embedding.local")
	ctx, span := tracer.Start(ctx, "LocalEmbeddingProvider.EmbedQuery")
	defer span.End()
	vecs, err := p.EmbedDocumentsWithModel(ctx, []string{text}, "")
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("local embedding: no embedding returned")
	}
	return vecs[0], nil
}

func (p *LocalEmbeddingProvider) HealthCheck(ctx context.Context) error {
	tracer := otel.Tracer("trazrexporter.embedding.local")
	ctx, span := tracer.Start(ctx, "LocalEmbeddingProvider.HealthCheck")
	defer span.End()
	texts := []string{"healthcheck"}
	_, err := p.EmbedDocumentsWithModel(ctx, texts, "")
	return err
}

func (p *LocalEmbeddingProvider) Validate() error {
	if p.Endpoint == "" {
		return fmt.Errorf("local embedding: endpoint must be set")
	}
	return nil
}

// NewLocalEmbeddingProvider creates a new LocalEmbeddingProvider with options.
func NewLocalEmbeddingProvider(endpoint string, timeout time.Duration, opts ...LocalOption) EmbeddingProvider {
	apiKey := os.Getenv("EMBEDDING_API_KEY")
	if apiKey == "" {
		apiKey = "your-api-key-here"
	}
	p := &LocalEmbeddingProvider{
		Endpoint: endpoint,
		Timeout:  timeout,
		Client:   nil, // allow option to set client
		Headers:  nil, // allow option to set headers
		Model:    "",  // allow option to set model
	}
	for _, opt := range opts {
		opt(p)
	}
	if p.Client == nil {
		p.Client = http.DefaultClient
	}
	if p.Headers == nil {
		p.Headers = map[string]string{"X-API-Key": apiKey}
	} else {
		if _, ok := p.Headers["X-API-Key"]; !ok {
			p.Headers["X-API-Key"] = apiKey
		}
	}
	return p
}

// CloseIdleConnections closes idle HTTP connections if the client supports it.
func (p *LocalEmbeddingProvider) CloseIdleConnections() {
	if p.Client != nil {
		if tr, ok := p.Client.Transport.(interface{ CloseIdleConnections() }); ok {
			tr.CloseIdleConnections()
		}
	}
}

func (p *LocalEmbeddingProvider) Close() error {
	p.CloseIdleConnections()
	return nil
}
