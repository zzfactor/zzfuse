package repository

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/ollama"
	"github.com/tmc/langchaingo/llms/openai"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

// EmbeddingProvider is a generic interface for generating vector embeddings from text.
type EmbeddingProvider interface {
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	HealthCheck(ctx context.Context) error
	Validate() error
	Close() error
}

type rateLimiter interface {
	Wait(ctx context.Context) error
	WaitN(ctx context.Context, n int) error
}

// simpleRateLimiter implements rate limiting using golang.org/x/time/rate
type simpleRateLimiter struct {
	limiter *rate.Limiter
}

func newSimpleRateLimiter(requestsPerSecond int) *simpleRateLimiter {
	if requestsPerSecond <= 0 {
		return nil
	}
	return &simpleRateLimiter{
		limiter: rate.NewLimiter(rate.Limit(requestsPerSecond), requestsPerSecond),
	}
}

func (r *simpleRateLimiter) Wait(ctx context.Context) error {
	if r == nil || r.limiter == nil {
		return nil
	}
	return r.limiter.Wait(ctx)
}

func (r *simpleRateLimiter) WaitN(ctx context.Context, n int) error {
	if r == nil || r.limiter == nil {
		return nil
	}
	return r.limiter.WaitN(ctx, n)
}

// EmbeddingProviderWithConfig wraps an EmbeddingProvider with timeout, rate limiting, normalization, and headers.
type EmbeddingProviderWithConfig struct {
	base      EmbeddingProvider
	timeout   time.Duration
	ratelimit rateLimiter
	normalize bool
}

func (e *EmbeddingProviderWithConfig) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	if e.ratelimit != nil {
		if err := e.ratelimit.WaitN(ctx, len(texts)); err != nil {
			return nil, err
		}
	}
	vecs, err := e.base.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, err
	}
	if e.normalize {
		for i := range vecs {
			vecs[i] = normalizeVec(vecs[i])
		}
	}
	return vecs, nil
}

func (e *EmbeddingProviderWithConfig) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}
	if e.ratelimit != nil {
		if err := e.ratelimit.Wait(ctx); err != nil {
			return nil, err
		}
	}
	vec, err := e.base.EmbedQuery(ctx, text)
	if err != nil {
		return nil, err
	}
	if e.normalize {
		vec = normalizeVec(vec)
	}
	return vec, nil
}

func (e *EmbeddingProviderWithConfig) HealthCheck(ctx context.Context) error {
	return e.base.HealthCheck(ctx)
}

func (e *EmbeddingProviderWithConfig) Validate() error {
	return e.base.Validate()
}

func (e *EmbeddingProviderWithConfig) Close() error {
	return e.base.Close()
}

// tracedEmbedder wraps an EmbeddingProvider with OpenTelemetry tracing
type tracedEmbedder struct {
	base     EmbeddingProvider
	tracer   trace.Tracer
	provider string
}

func (t *tracedEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	ctx, span := t.tracer.Start(ctx, "embed_documents")
	defer span.End()
	span.SetAttributes(
		attribute.Int64("text_count", int64(len(texts))),
		attribute.String("provider", t.provider),
	)
	return t.base.EmbedDocuments(ctx, texts)
}

func (t *tracedEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	ctx, span := t.tracer.Start(ctx, "embed_query")
	defer span.End()
	span.SetAttributes(
		attribute.String("provider", t.provider),
	)
	return t.base.EmbedQuery(ctx, text)
}

func (t *tracedEmbedder) HealthCheck(ctx context.Context) error {
	ctx, span := t.tracer.Start(ctx, "health_check")
	defer span.End()
	span.SetAttributes(attribute.String("provider", t.provider))
	return t.base.HealthCheck(ctx)
}

func (t *tracedEmbedder) Validate() error {
	return t.base.Validate()
}

func (t *tracedEmbedder) Close() error {
	return t.base.Close()
}

// normalizeVec normalizes a vector to unit length
func normalizeVec(vec []float32) []float32 {
	var sum float32
	for _, v := range vec {
		sum += v * v
	}
	if sum == 0 {
		return vec
	}
	norm := float32(math.Sqrt(float64(sum)))
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

// EmbeddingInfoProvider provides model and dimension info for embedding providers.
type EmbeddingInfoProvider interface {
	EmbeddingModel() string
	EmbeddingDimension() int
}

// NewEmbeddingProcessor creates an EmbeddingProvider based on config.Provider, supporting cross-cutting options and tracing for all providers.
func NewEmbeddingProcessor(ctx context.Context, config *database.EmbeddingConfig) (EmbeddingProvider, error) {
	var base EmbeddingProvider
	var providerName string

	switch config.Provider {
	case "openai":
		// default: { "secrets": { "embedding_api_key": "ENV:OPENAI_API_KEY" }
		apiKey := util.GetSecretFromMetadata(config.Metadata, "embedding_api_key")
		if apiKey == "" {
			return nil, fmt.Errorf("openai embedding: no api key")
		}
		model := config.Model
		if model == "" {
			model = "text-embedding-ada-002"
		}
		client, err := openai.New(openai.WithToken(apiKey), openai.WithModel(model))
		if err != nil {
			return nil, fmt.Errorf("openai embedding: %w", err)
		}
		// Create embeddings using the OpenAI client
		embedder, err := embeddings.NewEmbedder(client)
		if err != nil {
			return nil, fmt.Errorf("openai embedding: new embedder: %w", err)
		}
		base = &embedderWithNoopHealth{embedder}
		providerName = "openai"

	case "ollama":
		// apiKey := util.GetSecretFromMetadata(config.Metadata, "embedding_api_key")
		endpoint := config.Endpoint
		if endpoint == "" {
			endpoint = util.GetEnvString("EMBEDDING_OLLAMA_ENDPOINT", "http://localhost:11434")
		}
		model := config.Model
		if model == "" {
			model = "nomic-embed-text"
		}
		client, err := ollama.New(
			ollama.WithModel(model),
			ollama.WithServerURL(endpoint),
		)
		if err != nil {
			return nil, fmt.Errorf("ollama embedding: new client: %w", err)
		}
		embedder, err := embeddings.NewEmbedder(client)
		if err != nil {
			return nil, fmt.Errorf("ollama embedding: new embedder: %w", err)
		}
		base = &embedderWithNoopHealth{embedder}
		providerName = "ollama"

	case "sbert":
		endpoint := config.Endpoint
		if endpoint == "" {
			endpoint = util.GetEnvString("EMBEDDING_SBERT_ENDPOINT", "http://localhost:8000")
		}
		timeout := config.Timeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		headers := config.Headers
		if headers == nil {
			headers = map[string]string{}
		}
		client := http.DefaultClient
		if v, ok := config.Metadata["http_client"].(*http.Client); ok && v != nil {
			client = v
		}
		model := config.Model
		if model == "" {
			model = "sentence-transformers/all-MiniLM-L6-v2"
		}
		base = NewLocalEmbeddingProvider(endpoint, timeout, WithHeaders(headers), WithHTTPClient(client), WithModel(model))
		providerName = "sbert"

	default:
		return nil, fmt.Errorf("unsupported embedding provider: %s", config.Provider)
	}

	// Cross-cutting options
	timeout := config.Timeout
	normalize := config.Normalize

	// Use RateLimit field from config instead of metadata
	var limiter rateLimiter
	if config.RateLimit > 0 {
		limiter = newSimpleRateLimiter(config.RateLimit)
	} else {
		// Fallback to metadata for backward compatibility
		if v, ok := config.Metadata["rate_limiter"].(rateLimiter); ok {
			limiter = v
		}
	}

	// Wrap with EmbeddingProviderWithConfig
	wrapped := &EmbeddingProviderWithConfig{
		base:      base,
		timeout:   timeout,
		ratelimit: limiter,
		normalize: normalize,
	}

	// Always wrap with tracing
	tracer := otel.Tracer("trazrexporter.embedding." + providerName)
	return &tracedEmbedder{base: wrapped, tracer: tracer, provider: providerName}, nil
}

// embedderWithNoopHealth wraps an embedder to provide no-op HealthCheck/Validate/Close.
type embedderWithNoopHealth struct {
	emb interface {
		EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
		EmbedQuery(ctx context.Context, text string) ([]float32, error)
	}
}

func (e *embedderWithNoopHealth) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	return e.emb.EmbedDocuments(ctx, texts)
}
func (e *embedderWithNoopHealth) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	return e.emb.EmbedQuery(ctx, text)
}
func (e *embedderWithNoopHealth) HealthCheck(ctx context.Context) error { return nil }
func (e *embedderWithNoopHealth) Validate() error                       { return nil }
func (e *embedderWithNoopHealth) Close() error                          { return nil }
