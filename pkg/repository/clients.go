package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/tmc/langchaingo/schema"
)

// Errors for pool and client operations
var (
	ErrServiceDropped = errors.New("service dropped by policy")
	ErrQueueOverflow  = errors.New("signal queue overflow")
	ErrPoolNotFound   = errors.New("connection pool not found")
	ErrInvalidConfig  = errors.New("invalid configuration")
	ErrPoolClosed     = errors.New("connection pool closed")
)

// DatabaseClient represents a telemetry-aware database client interface
// This is the primary interface for database operations with built-in OpenTelemetry support
type DatabaseClient interface {
	// Telemetry-aware CRUD operations
	Insert(ctx context.Context, rec interface{}) error
	Filter(ctx context.Context, params interface{}) ([]interface{}, error)
	Reset(ctx context.Context) error

	// Standard database operations
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)

	// Connection management
	HealthCheck(ctx context.Context) error
	Close() error

	// Pool information
	Stats() sql.DBStats
	SetMaxOpenConns(n int)
	SetMaxIdleConns(n int)
	SetConnMaxLifetime(d time.Duration)
	SetConnMaxIdleTime(d time.Duration)

	// Service-specific metadata
	ServiceName() string

	// Raw database access (for cases where direct *sql.DB is needed)
	DB() *sql.DB
}

// VectorStoreClient represents a client for vector store operations
// VectorStoreClient matches langchaingo's VectorStore interface with enhanced vector operations
type VectorStoreClient interface {
	AddDocuments(ctx context.Context, docs []schema.Document) ([]string, error)
	SimilaritySearch(ctx context.Context, query string, k int, opts ...interface{}) ([]schema.Document, error)
	SimilaritySearchByVector(ctx context.Context, query []float32, k int, opts ...interface{}) ([]schema.Document, error)
	HealthCheck(ctx context.Context) error
	Close() error

	// Enhanced vector operations for trace embedding
	GetVectorByID(ctx context.Context, id string) ([]float32, error)
	GetVectorsByIDs(ctx context.Context, ids []string) ([][]float32, error)
	GetVectorMetadata(ctx context.Context, id string) (map[string]interface{}, error)
	CombineVectors(ctx context.Context, vectors [][]float32, strategy string) ([]float32, error)
	GetVectorCombinationStrategy() string

	// Baseline comparison operations
	FindBaselineByContentHash(ctx context.Context, content string) ([]interface{}, error)
}

// EmbeddingClient represents a client for embedding operations
type EmbeddingClient interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
	EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error)
	HealthCheck(ctx context.Context) error
	Validate() error
	Close() error
}
