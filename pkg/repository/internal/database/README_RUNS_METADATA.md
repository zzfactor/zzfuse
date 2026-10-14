# Runs Table Structured Metadata

This document describes the structured metadata functionality for the runs table, which provides a type-safe way to work with metadata fields for span counts, log counts, and error counts.

## Overview

The runs table uses a JSONB column for metadata storage, which allows flexible key-value storage. The structured metadata system provides:

- Type-safe access to specific metadata fields
- Automatic conversion between structured types and database storage
- Helper methods for common operations like incrementing counters
- Validation and error handling

## Metadata Fields

The following metadata fields are supported:

| Field | Type | Description |
|-------|------|-------------|
| `SPAN_COUNT` | int64 | Total number of spans processed |
| `SPAN_EVENT_COUNT` | int64 | Total number of span events processed |
| `LOG_COUNT` | int64 | Total number of logs processed |
| `LOG_INFO_COUNT` | int64 | Number of info-level logs |
| `LOG_WARNING_COUNT` | int64 | Number of warning-level logs |
| `LOG_DEBUG_COUNT` | int64 | Number of debug-level logs |
| `LOG_ERROR_COUNT` | int64 | Number of error-level logs |
| `SPAN_ERROR_COUNT` | int64 | Number of spans with errors |

## Usage Examples

### Basic Usage

```go
// Create structured metadata
metadata := &RunMetadata{
    SpanCount:       1000,
    SpanEventCount:  500,
    LogCount:        2000,
    LogInfoCount:    1500,
    LogWarningCount: 300,
    LogDebugCount:   150,
    LogErrorCount:   50,
    SpanErrorCount:  25,
}

	// Create a run with structured metadata
	scopeName := "my-scope"
	run := NewRunWithStructuredMetadata(
		"my-service",
		&scopeName,
		"processing-run",
		time.Now().UnixNano(),
		nil, // endTimestampNs
		nil, // traceID
		nil, // processorID
		nil, // runType
		metadata,
	)

// Access structured metadata
structuredMetadata := run.GetStructuredMetadata()
fmt.Printf("Span Count: %d\n", structuredMetadata.SpanCount)
fmt.Printf("Log Error Count: %d\n", structuredMetadata.LogErrorCount)
```

### Repository Operations

```go
// Create repository
repo := NewRunRepository(ctx, db, schema)

	// Create a run with structured metadata
	scopeName := "example-scope"
	runID, err := repo.CreateWithStructuredMetadata(
		ctx,
		"example-service",
		&scopeName,
		"example-run",
		time.Now().UnixNano(),
		nil, // endTimestampNs
		nil, // traceID
		nil, // processorID
		nil, // runType
		metadata,
	)

// Retrieve structured metadata
retrievedMetadata, err := repo.GetStructuredMetadata(ctx, runID)

// Update specific metadata fields
err = repo.UpdateMetadataField(ctx, runID, "SPAN_COUNT", 150)

// Increment metadata fields
err = repo.IncrementMetadataField(ctx, runID, "LOG_ERROR_COUNT", 3)

// Update entire structured metadata
err = repo.UpdateStructuredMetadata(ctx, runID, newMetadata)
```

### Incremental Updates

```go
// Increment various counters
counters := map[string]int64{
    "SPAN_COUNT":       10,
    "SPAN_EVENT_COUNT": 5,
    "LOG_COUNT":        20,
    "LOG_INFO_COUNT":   15,
    "LOG_WARNING_COUNT": 3,
    "LOG_DEBUG_COUNT":   2,
    "LOG_ERROR_COUNT":   1,
    "SPAN_ERROR_COUNT":  1,
}

for field, increment := range counters {
    err := repo.IncrementMetadataField(ctx, runID, field, increment)
    if err != nil {
        log.Printf("Failed to increment %s: %v", field, err)
        continue
    }
}
```

## API Reference

### RunMetadata Struct

```go
type RunMetadata struct {
    SpanCount       int64 `json:"SPAN_COUNT,omitempty"`
    SpanEventCount  int64 `json:"SPAN_EVENT_COUNT,omitempty"`
    LogCount        int64 `json:"LOG_COUNT,omitempty"`
    LogInfoCount    int64 `json:"LOG_INFO_COUNT,omitempty"`
    LogWarningCount int64 `json:"LOG_WARNING_COUNT,omitempty"`
    LogDebugCount   int64 `json:"LOG_DEBUG_COUNT,omitempty"`
    LogErrorCount   int64 `json:"LOG_ERROR_COUNT,omitempty"`
    SpanErrorCount  int64 `json:"SPAN_ERROR_COUNT,omitempty"`
}
```

### RunMetadata Methods

- `NewRunMetadata()` - Creates a new RunMetadata with default values
- `ToMap()` - Converts RunMetadata to map[string]interface{} for database storage
- `FromMap(data map[string]interface{})` - Creates RunMetadata from database map

### Run Methods

- `GetStructuredMetadata()` - Returns metadata as structured RunMetadata object
- `SetStructuredMetadata(metadata *RunMetadata)` - Sets metadata from structured object

### RunRepository Methods

- `CreateWithStructuredMetadata(...)` - Creates run with structured metadata
- `UpdateWithStructuredMetadata(...)` - Updates run with structured metadata
- `UpdateMetadataField(id, field, value)` - Updates specific metadata field
- `IncrementMetadataField(id, field, increment)` - Increments specific metadata field
- `UpdateStructuredMetadata(id, metadata)` - Updates entire structured metadata
- `GetStructuredMetadata(id)` - Retrieves structured metadata for a run

## Database Schema

The metadata is stored in a JSONB column, and scope_name is a separate column:

```sql
CREATE TABLE IF NOT EXISTS {{.Schema}}.runs (
    id BIGSERIAL PRIMARY KEY,
    service_name TEXT NOT NULL,
    scope_name TEXT,
    name TEXT NOT NULL, 
    start_timestamp_ns BIGINT NOT NULL,
    end_timestamp_ns BIGINT,
    trace_id TEXT,
    processor_id BIGINT,
    run_type SMALLINT,
    metadata JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

## Best Practices

1. **Use structured metadata for type safety**: Always use the `RunMetadata` struct instead of raw maps when possible.

2. **Incremental updates for performance**: Use `IncrementMetadataField` for frequent counter updates instead of retrieving, updating, and saving the entire metadata.

3. **Handle zero values**: The `ToMap()` method excludes zero values to keep the JSONB storage clean.

4. **Error handling**: Always check for errors when performing database operations.

5. **Context propagation**: Use context.Context for all database operations to support timeouts and cancellation.

## Testing

Run the tests to verify the functionality:

```bash
cd pkg/repository/internal/database
go test -v -run TestRunMetadata
```

## Migration from Raw Maps

If you're currently using raw maps for metadata, you can migrate gradually:

```go
// Old way
metadata := map[string]interface{}{
    "SPAN_COUNT": 100,
    "LOG_COUNT":  200,
}

// New way
metadata := &RunMetadata{
    SpanCount: 100,
    LogCount:  200,
}

// Convert existing map to structured metadata
existingMap := map[string]interface{}{
    "SPAN_COUNT": float64(100),
    "LOG_COUNT":  float64(200),
}
structuredMetadata := FromMap(existingMap)
``` 