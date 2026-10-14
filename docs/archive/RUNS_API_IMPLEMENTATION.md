# Runs API Implementation

## Overview

This document describes the implementation of the Runs API endpoints in the OpenAPI specification, following the same pattern as other telemetry data types (logs, traces, metrics, profiles). The Runs API provides full CRUD operations and real-time streaming for run management.

## API Endpoints

### 1. Query Runs - `GET /api/v1/runs`

**Purpose**: Query runs with flexible filters and cursor-based pagination.

**Parameters**:
- `start` (string, date-time): Start time (RFC3339, optional; default: 1 hour ago)
- `end` (string, date-time): End time (RFC3339, optional; default: now)
- `service` (string): Filter by service name
- `name` (string): Filter by run name
- `session_id` (string): Filter by session ID
- `run_type` (integer): Filter by run type (1: time based, 2: trace based)
- `active_only` (boolean, default: false): Filter to only active (non-ended) runs
- `limit` (integer, default: 100): Max results
- `cursor` (string): Base64-encoded cursor for pagination
- `schema` (string): Postgres schema to use (default otel)

**Response**: `RunQueryResult` with array of `RunEntryResponse` objects

**Example**:
```bash
GET /api/v1/runs?service=api&active_only=true&limit=50
```

### 2. Insert Runs - `POST /api/v1/runs`

**Purpose**: Insert a single run entry or an array of run entries.

**Authentication**: Requires JWT Bearer authentication

**Request Body**: `RunEntry` or array of `RunEntry` objects

**Required Fields**:
- `ServiceName`: Service name
- `Name`: Run name
- `StartTimestampNs`: Start timestamp in nanoseconds

**Optional Fields**:
- `ScopeName`: Scope name
- `EndTimestampNs`: End timestamp in nanoseconds
- `TraceID`: Associated trace ID
- `ProcessorID`: Processor ID
- `RunType`: Run type (1: time based, 2: trace based)
- `SessionID`: Session ID for grouping runs
- `Metadata`: Run metadata object

**Response**: `InsertResult` with array of inserted IDs

**Example**:
```json
{
  "ServiceName": "api",
  "Name": "request-processing",
  "StartTimestampNs": 1754501063000000000,
  "RunType": 1,
  "SessionID": "user-session-123"
}
```

### 3. Update Run Metadata - `PUT /api/v1/runs`

**Purpose**: Update metadata for a specific run by ID.

**Authentication**: Requires JWT Bearer authentication

**Request Body**:
```json
{
  "ID": 123,
  "Metadata": {
    "SPAN_COUNT": 100,
    "LOG_COUNT": 50,
    "LOG_ERROR_COUNT": 2
  }
}
```

**Response**: Status confirmation

### 4. End Run - `DELETE /api/v1/runs`

**Purpose**: End a run by setting its end timestamp.

**Authentication**: Requires JWT Bearer authentication

**Request Body**:
```json
{
  "Name": "request-processing",
  "EndTimestampNs": 1754501163000000000
}
```

**Response**: Status confirmation

### 5. Stream Runs - `GET /api/v1/stream/runs`

**Purpose**: Stream new run entries in real time using WebSocket.

**Protocol**: WebSocket upgrade

**Message Format**: `RunEntryResponse` objects

**Features**:
- Real-time streaming of new run entries
- No historical data (only new entries)
- WebSocket-based for low latency

## Schema Definitions

### RunEntry (Insert Schema)

```yaml
RunEntry:
  type: object
  required: [ServiceName, Name, StartTimestampNs]
  properties:
    ServiceName:
      type: string
      description: Service name
    ScopeName:
      type: string
      description: Scope name (optional)
    Name:
      type: string
      description: Run name
    StartTimestampNs:
      type: integer
      description: Start timestamp in nanoseconds
    EndTimestampNs:
      type: integer
      description: End timestamp in nanoseconds (optional)
    TraceID:
      type: string
      description: Associated trace ID (optional)
    ProcessorID:
      type: integer
      description: Processor ID (optional)
    RunType:
      type: integer
      description: Run type (1: time based, 2: trace based)
    SessionID:
      type: string
      description: Session ID for grouping runs (optional)
    Metadata:
      type: object
      additionalProperties: true
      description: Run metadata (optional)
```

### RunEntryResponse (Query Schema)

```yaml
RunEntryResponse:
  type: object
  properties:
    ID:
      type: integer
    SessionID:
      type: string
    ServiceName:
      type: string
    ScopeName:
      type: string
    Name:
      type: string
    StartTimestampNs:
      type: integer
    EndTimestampNs:
      type: integer
    TraceID:
      type: string
    ProcessorID:
      type: integer
    RunType:
      type: integer
    Metadata:
      type: object
    CreatedAt:
      type: string
      format: date-time
    UpdatedAt:
      type: string
      format: date-time
```

### RunQueryResult

```yaml
RunQueryResult:
  type: object
  properties:
    Entries:
      type: array
      items:
        $ref: '#/components/schemas/RunEntryResponse'
    NextCursor:
      type: string
    HasMore:
      type: boolean
```

## Usage Examples

### 1. Create a New Run

```bash
curl -X POST http://localhost:18888/api/v1/runs \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "ServiceName": "api",
    "Name": "request-processing",
    "StartTimestampNs": 1754501063000000000,
    "RunType": 1,
    "SessionID": "user-session-123"
  }'
```

### 2. Query Active Runs

```bash
curl "http://localhost:18888/api/v1/runs?service=api&active_only=true&limit=10"
```

### 3. Query Runs by Session

```bash
curl "http://localhost:18888/api/v1/runs?session_id=user-session-123"
```

### 4. Update Run Metadata

```bash
curl -X PUT http://localhost:18888/api/v1/runs \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "ID": 123,
    "Metadata": {
      "SPAN_COUNT": 100,
      "LOG_COUNT": 50,
      "LOG_ERROR_COUNT": 2
    }
  }'
```

### 5. End a Run

```bash
curl -X DELETE http://localhost:18888/api/v1/runs \
  -H "Authorization: Bearer <jwt-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "Name": "request-processing",
    "EndTimestampNs": 1754501163000000000
  }'
```

### 6. WebSocket Streaming

```javascript
const ws = new WebSocket('ws://localhost:18888/api/v1/stream/runs');
ws.onmessage = function(event) {
  const run = JSON.parse(event.data);
  console.log('New run:', run);
};
```

## Key Features

### 1. Session Support
- `session_id` field for grouping related runs
- Session-based filtering in queries
- Session-aware streaming

### 2. Run Types
- Time-based runs (type 1)
- Trace-based runs (type 2)
- Type filtering in queries

### 3. Active Run Filtering
- `active_only` parameter to filter non-ended runs
- Useful for monitoring ongoing operations

### 4. Metadata Support
- Flexible metadata object for run-specific data
- Structured metadata with common fields (SPAN_COUNT, LOG_COUNT, etc.)
- Metadata updates via PUT endpoint

### 5. Real-time Streaming
- WebSocket-based streaming for new runs
- Low-latency event delivery
- No historical data (only new entries)

### 6. Cursor-based Pagination
- Efficient pagination for large result sets
- Base64-encoded cursors
- Consistent with other telemetry APIs

## Integration with Event System

The Runs API integrates with the event bus system:

- **Event Types**: `run.created`, `run.updated`
- **Event Data**: Complete run information including session_id
- **Real-time**: Events published immediately on run operations
- **Consistent**: Same event format as other telemetry data

## Error Handling

### Common Error Responses

- `400 Bad Request`: Invalid input parameters or request body
- `401 Unauthorized`: Missing or invalid JWT token
- `404 Not Found`: Run not found (for update/delete operations)
- `500 Internal Server Error`: Server-side errors

### Error Response Format

```json
{
  "error": "Error message description"
}
```

## Security

### Authentication
- JWT Bearer token required for write operations (POST, PUT, DELETE)
- Read operations (GET) may be public depending on configuration
- Token validation and authorization checks

### Authorization
- Service-level access control
- Schema-based isolation
- Session-based permissions (future enhancement)

## Performance Considerations

### Query Optimization
- Indexed fields for efficient filtering
- Cursor-based pagination for large datasets
- Session_id index for session-based queries

### Streaming Performance
- WebSocket for low-latency streaming
- Event-driven architecture
- Efficient message serialization

### Batch Operations
- Support for batch inserts
- Bulk metadata updates
- Optimized database operations

## Future Enhancements

### 1. Advanced Filtering
- Complex query expressions
- Full-text search on run names
- Metadata-based filtering

### 2. Analytics
- Run duration statistics
- Session-based analytics
- Performance metrics

### 3. Management Features
- Run lifecycle management
- Bulk operations
- Run templates

### 4. Integration
- Grafana dashboards
- Alerting rules
- External system integration

## Implementation Notes

### Database Schema
- Uses the `runs` table with `session_id` field
- Optimized indexes for common query patterns
- JSONB metadata for flexible data storage

### Event Publishing
- Automatic event publishing on run operations
- Session-aware event routing
- Consistent event format across all operations

### API Consistency
- Follows same patterns as logs, traces, metrics APIs
- Consistent parameter naming and response formats
- Standardized error handling and authentication 