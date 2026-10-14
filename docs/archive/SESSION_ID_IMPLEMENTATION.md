# Session ID Implementation for Runs Table

## Overview

This document describes the implementation of the new `session_id` field in the runs table. The session_id field allows for grouping runs by session, enabling better organization and querying of run data.

## Database Schema Changes

### 1. Table Structure Update

**File**: `pkg/repository/internal/database/ddl/runs.sql`

Added the `session_id` field to the runs table:

```sql
CREATE TABLE IF NOT EXISTS {{.Schema}}.runs (
    id BIGSERIAL PRIMARY KEY,
    session_id TEXT,                    -- NEW FIELD
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

### 2. Index Addition

Added a new index for efficient session-based queries:

```sql
-- 7. Session ID for session-based queries
CREATE INDEX IF NOT EXISTS idx_runs_session_id ON {{.Schema}}.runs(session_id) WHERE session_id IS NOT NULL;
```

## Go Code Changes

### 1. Database Model Updates

**File**: `pkg/repository/internal/database/runs.go`

#### Run Struct Update
```go
type Run struct {
    ID               int64                  `db:"id" json:"id"`
    SessionID        *string                `db:"session_id" json:"session_id,omitempty"`  // NEW FIELD
    ServiceName      string                 `db:"service_name" json:"service_name"`
    // ... other fields
}
```

#### New Constructor Functions
```go
// NewRunWithSessionID creates a new Run with session ID
func NewRunWithSessionID(sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) *Run

// NewRunWithSessionIDAndStructuredMetadata creates a new Run with session ID and structured metadata
func NewRunWithSessionIDAndStructuredMetadata(sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata *RunMetadata) *Run
```

#### Repository Method Updates
```go
// Create method now includes session_id with default empty string
func (r *RunRepository) Create(ctx context.Context, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) (int64, error)

// New method for creating runs with explicit session_id
func (r *RunRepository) CreateWithSessionID(ctx context.Context, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) (int64, error)

// New method for updating runs with session_id
func (r *RunRepository) UpdateWithSessionID(ctx context.Context, id int64, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) error

// New method for querying runs by session_id
func (r *RunRepository) GetBySessionID(ctx context.Context, sessionID string) ([]*Run, error)
```

#### Query Updates
All SELECT queries now include `session_id` in the column list:
- `GetByID()`
- `GetByProcessorID()`
- `GetByTraceID()`
- `GetActiveRunsByServiceName()`

#### Scan Function Updates
Updated both `scanRunFromRow()` and `scanRunFromSingleRow()` to include session_id in the scan order.

### 2. Interface Updates

**File**: `pkg/repository/internal/database/config_interface.go`

Updated `RunRepositoryInterface` to include new session_id methods:
```go
type RunRepositoryInterface interface {
    // ... existing methods
    CreateWithSessionID(ctx context.Context, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) (int64, error)
    UpdateWithSessionID(ctx context.Context, id int64, sessionID *string, serviceName string, scopeName *string, name string, startTimestampNs int64, endTimestampNs *int64, traceID *string, processorID *int64, runType *int16, metadata map[string]interface{}) error
    GetBySessionID(ctx context.Context, sessionID string) ([]*Run, error)
}
```

### 3. Service Layer Updates

**File**: `pkg/repository/run_service.go`

#### RunEntryResponse Update
```go
type RunEntryResponse struct {
    ID               int64                  `json:"id"`
    SessionID        *string                `json:"session_id,omitempty"`  // NEW FIELD
    ServiceName      string                 `json:"service_name"`
    // ... other fields
}
```

#### New Service Methods
```go
// CreateRunWithSessionID creates a new run with session ID
func (s *RunService) CreateRunWithSessionID(ctx context.Context, sessionID *string, serviceName, name string, startTimestampNs int64, runType *int16) (int64, error)

// GetRunsBySessionID returns all runs for a specific session ID
func (s *RunService) GetRunsBySessionID(ctx context.Context, sessionID string) ([]*dbconfig.Run, error)
```

### 4. Utility Functions

**File**: `pkg/repository/utils.go`

Added helper functions for creating pointers:
```go
// int64Ptr returns a pointer to the given int64
func int64Ptr(i int64) *int64

// int16Ptr returns a pointer to the given int16
func int16Ptr(i int16) *int16
```

### 5. Test Updates

**File**: `pkg/repository/run_service_test.go`

Updated tests to include session_id:
```go
func TestRunEntryResponseConversion(t *testing.T) {
    run := &dbconfig.Run{
        ID:               123,
        SessionID:        stringPtr("session-123"),  // NEW FIELD
        ServiceName:      "test-service",
        // ... other fields
    }
    
    response := ToRunEntryResponse(run)
    assert.Equal(t, "session-123", *response.SessionID)  // NEW ASSERTION
}
```

## Backward Compatibility

### 1. Default Behavior
- The existing `Create()` method now includes `session_id` with a default empty string value
- All existing code continues to work without changes
- The `session_id` field is optional and can be `NULL` in the database

### 2. Existing Methods
- `CreateRun()` - Uses default empty session_id
- `EndRun()` - No changes needed
- `UpdateRunMetadata()` - No changes needed
- All query methods - Automatically include session_id in results

### 3. New Methods
- `CreateRunWithSessionID()` - For explicit session_id creation
- `GetRunsBySessionID()` - For session-based queries

## Usage Examples

### 1. Creating Runs with Session ID

```go
// Create run with session ID
sessionID := "user-session-123"
runID, err := runService.CreateRunWithSessionID(ctx, &sessionID, "my-service", "processing-run", startTime, &runType)

// Create run without session ID (uses empty string)
runID, err := runService.CreateRun(ctx, "my-service", "processing-run", startTime, &runType)
```

### 2. Querying Runs by Session

```go
// Get all runs for a specific session
runs, err := runService.GetRunsBySessionID(ctx, "user-session-123")
for _, run := range runs {
    fmt.Printf("Run: %s, Service: %s\n", run.Name, run.ServiceName)
}
```

### 3. Event Publishing

The session_id is automatically included in event data:
```json
{
  "type": "run.created",
  "data": {
    "id": 123,
    "session_id": "user-session-123",
    "service_name": "my-service",
    "name": "processing-run"
  }
}
```

## Database Migration

### 1. Schema Update
The new `session_id` column is added as nullable, so existing databases will work without migration.

### 2. Index Creation
The new index is created with `IF NOT EXISTS`, so it's safe to run multiple times.

### 3. Data Consistency
- Existing runs will have `session_id = NULL` or empty string
- New runs will have `session_id` set as specified or empty string by default

## Performance Considerations

### 1. Index Impact
- The new `session_id` index only includes non-NULL values
- Minimal performance impact on existing queries
- Efficient session-based queries

### 2. Storage Impact
- `session_id` is a TEXT field, so storage impact depends on session ID length
- Index storage is minimal due to NULL filtering

### 3. Query Performance
- Session-based queries are now optimized with the new index
- Existing queries maintain their performance characteristics

## Testing

### 1. Unit Tests
```bash
cd pkg/repository
go test -v -run "TestRun"
```

### 2. Integration Tests
The existing test suite continues to pass with the new session_id field.

## Future Enhancements

### 1. Session Management
- Add session lifecycle management
- Session cleanup and archival
- Session-based analytics

### 2. API Endpoints
- Add REST API endpoints for session-based operations
- Session-based filtering in existing endpoints

### 3. Monitoring
- Add session-based metrics
- Session duration tracking
- Session performance analytics

## Migration Notes

- **Zero Downtime**: Changes are backward compatible
- **Gradual Rollout**: Can be enabled per service or globally
- **Optional Usage**: Session ID is optional and can be adopted gradually
- **Monitoring**: Existing monitoring continues to work
- **Events**: Session ID is automatically included in event data 