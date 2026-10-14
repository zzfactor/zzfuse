package repository

import (
	"context"
	"database/sql"
	"time"
)

// DataRow represents a row in the data table with telemetry information
type DataRow struct {
	ID                 int64
	SignalType         string
	ObservedTimestamp  time.Time
	Timestamp          time.Time
	TraceId            string
	SpanId             string
	ParentSpanId       string
	Body               string
	ResourceAttributes map[string]interface{}
	Attributes         map[string]interface{}
	Metadata           map[string]interface{}
}

// DataFilterParams defines filters for querying the data table
type DataFilterParams struct {
	Namespace     string // Used for routing only, not for SQL filtering
	StartTime     time.Time
	EndTime       time.Time
	TraceID       string
	SpanID        string
	Metadata      map[string]interface{}
	ResourceAttrs map[string]interface{}
	Attributes    map[string]interface{}
	Limit         int
	Offset        int
}

// OtelDatabase defines the interface for telemetry data table operations
type OtelDatabase interface {
	Insert(ctx context.Context, rec DataRow) error
	Filter(ctx context.Context, params DataFilterParams) ([]DataRow, error)
	Close() error
	HealthCheck(ctx context.Context) error
	Reset(ctx context.Context) error
	DB() *sql.DB
}

// EmbeddingMetadataUpdater defines methods for managing embedding metadata in the data table
type EmbeddingMetadataUpdater interface {
	RemoveEmbeddingReference(ctx context.Context, namespace, embeddingID string) error
	MarkEmbedded(ctx context.Context, rowID int64, embeddingID, vectorStore, createdAt string) error
}

// DataTableConfig holds configuration for a per-service data table
type DataTableConfig struct {
	TableName string
	// Add more fields as needed (DB config, indexes, etc.)
}
