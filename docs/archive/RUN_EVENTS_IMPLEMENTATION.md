# Run Events Implementation

## Overview

This document describes the implementation of event publishing for runs, similar to how logs are handled in the system. The implementation adds event-driven architecture support for run operations, enabling real-time notifications when runs are created, updated, or ended.

## Implementation Summary

### 1. Event Types Added

Two new event types were added to the telemetry event system:

- `run.created` - Published when a new run is created
- `run.updated` - Published when a run is updated (ended, metadata updated, etc.)

### 2. Files Modified

#### Core Implementation Files

1. **`pkg/repository/run_service.go`**
   - Added `RunEntryResponse` struct for API responses and event data
   - Added `ToRunEntryResponse()` function for data conversion
   - Enhanced `RunService` with event bus support
   - Added `NewRunServiceWithEventBus()` constructor
   - Modified `CreateRun()` to publish `run.created` events
   - Modified `EndRun()` to publish `run.updated` events
   - Modified `UpdateRunMetadata()` to publish `run.updated` events
   - Added `UpdateRunMetadataBatch()` for batch operations

2. **`pkg/repository/telemetry_events.go`**
   - Added `EventTypeRunCreated` and `EventTypeRunUpdated` constants
   - Updated `IsEvent()` function to include run events
   - Updated `GetEventCategory()` function to categorize run events

3. **`pkg/repository/utils.go`**
   - Added `int64Ptr()` and `int16Ptr()` helper functions for creating pointers

4. **`pkg/repository/run_service_test.go`** (new file)
   - Added unit tests for event publishing functionality
   - Added tests for `ToRunEntryResponse()` conversion

5. **`config-with-eventbus.yaml`**
   - Added run event types to the configuration

## Event Data Structure

### RunEntryResponse

```go
type RunEntryResponse struct {
    ID               int64                  `json:"id"`
    ServiceName      string                 `json:"service_name"`
    ScopeName        *string                `json:"scope_name,omitempty"`
    Name             string                 `json:"name"`
    StartTimestampNs int64                  `json:"start_timestamp_ns"`
    EndTimestampNs   *int64                 `json:"end_timestamp_ns,omitempty"`
    TraceID          *string                `json:"trace_id,omitempty"`
    ProcessorID      *int64                 `json:"processor_id,omitempty"`
    RunType          *int16                 `json:"run_type,omitempty"`
    Metadata         map[string]interface{} `json:"metadata,omitempty"`
    CreatedAt        string                 `json:"created_at"`
    UpdatedAt        string                 `json:"updated_at"`
}
```

## Event Publishing Points

### 1. Run Creation (`run.created`)

**Location**: `RunService.CreateRun()`

**Trigger**: When a new run is successfully created in the database

**Event Data**: Complete run information including ID, service name, name, timestamps, etc.

**Example Event**:
```json
{
  "type": "run.created",
  "timestamp": "2024-01-01T12:00:00Z",
  "schema": "",
  "data": {
    "id": 123,
    "service_name": "my-service",
    "name": "processing-run",
    "start_timestamp_ns": 1704110400000000000,
    "created_at": "2024-01-01T12:00:00Z",
    "updated_at": "2024-01-01T12:00:00Z"
  },
  "ids": ["123"]
}
```

### 2. Run Updates (`run.updated`)

**Location**: `RunService.EndRun()` and `RunService.UpdateRunMetadata()`

**Trigger**: When a run is ended or its metadata is updated

**Event Data**: Updated run information reflecting the current state

**Example Event**:
```json
{
  "type": "run.updated",
  "timestamp": "2024-01-01T13:00:00Z",
  "schema": "",
  "data": {
    "id": 123,
    "service_name": "my-service",
    "name": "processing-run",
    "start_timestamp_ns": 1704110400000000000,
    "end_timestamp_ns": 1704114000000000000,
    "metadata": {
      "SPAN_COUNT": 1000,
      "LOG_COUNT": 500
    },
    "created_at": "2024-01-01T12:00:00Z",
    "updated_at": "2024-01-01T13:00:00Z"
  },
  "ids": ["123"]
}
```

### 3. Batch Updates (`run.updated`)

**Location**: `RunService.UpdateRunMetadataBatch()`

**Trigger**: When multiple runs are updated in a batch operation

**Event Data**: Array of updated run information

**Example Event**:
```json
{
  "type": "run.updated",
  "timestamp": "2024-01-01T14:00:00Z",
  "schema": "",
  "data": [
    {
      "id": 123,
      "service_name": "service-1",
      "name": "run-1",
      "metadata": {"SPAN_COUNT": 100}
    },
    {
      "id": 124,
      "service_name": "service-2", 
      "name": "run-2",
      "metadata": {"LOG_COUNT": 200}
    }
  ],
  "ids": ["123", "124"]
}
```

## Integration with Event Bus

### Event Bus Configuration

The run events are automatically integrated with the existing event bus system:

1. **In-Memory Event Bus**: For single-process deployments
2. **NATS Event Bus**: For distributed deployments

### Configuration Example

```yaml
extensions:
  trazr:
    event_bus:
      type: "nats"  # or "in-memory"
      nats:
        url: "nats://localhost:4222"
      common:
        event_types:
          - "log.inserted"
          - "trace.inserted"
          - "metric.inserted"
          - "profile.inserted"
          - "run.created"    # New
          - "run.updated"    # New
```

## Usage Examples

### Creating a Run Service with Event Bus

```go
// Create event bus
factory := event.NewEventBusFactory("", "")
eventBus, err := factory.CreateEventBus(ctx)
if err != nil {
    return err
}

// Create run service with event bus
runService := NewRunServiceWithEventBus(ctx, db, "my_schema", eventBus)

// Create a run (will publish run.created event)
runID, err := runService.CreateRun(ctx, "my-service", "processing-run", startTime, &runType)
if err != nil {
    return err
}

// End a run (will publish run.updated event)
err = runService.EndRun(ctx, "processing-run", endTime)
if err != nil {
    return err
}
```

### Subscribing to Run Events

```go
// Register handler for run events
handler := &myRunEventHandler{}
eventBus.RegisterHandler(EventTypeRunCreated.ToEventType(), handler)
eventBus.RegisterHandler(EventTypeRunUpdated.ToEventType(), handler)

// Handler implementation
func (h *myRunEventHandler) HandleEvent(ctx context.Context, event event.Event) error {
    switch event.Type {
    case EventTypeRunCreated.ToEventType():
        // Handle run creation
        runData := event.Data.(RunEntryResponse)
        log.Printf("New run created: %s", runData.Name)
        
    case EventTypeRunUpdated.ToEventType():
        // Handle run updates
        if runData, ok := event.Data.(RunEntryResponse); ok {
            log.Printf("Run updated: %s", runData.Name)
        } else if runBatch, ok := event.Data.([]RunEntryResponse); ok {
            log.Printf("Batch update: %d runs", len(runBatch))
        }
    }
    return nil
}
```

## Benefits

1. **Real-time Notifications**: Immediate awareness of run state changes
2. **Decoupled Architecture**: Run operations are independent of event consumers
3. **Scalable**: Supports both in-memory and distributed event buses
4. **Consistent**: Follows the same pattern as logs, traces, and metrics
5. **Extensible**: Easy to add new run event types or modify existing ones

## Testing

The implementation includes comprehensive tests:

```bash
# Run the tests
cd pkg/repository
go test -v -run "TestRun"
```

Tests cover:
- Event bus integration
- Data conversion (`ToRunEntryResponse`)
- Event type registration
- Helper function functionality

## Future Enhancements

1. **Additional Event Types**: Could add `run.deleted`, `run.started`, `run.completed`
2. **Event Filtering**: Add filtering capabilities for specific run types or services
3. **Event Persistence**: Store events for audit trails or replay capabilities
4. **Metrics Integration**: Add Prometheus metrics for run event processing
5. **Webhook Support**: Send run events to external systems via webhooks

## Migration Notes

- **Backward Compatible**: Existing code continues to work without changes
- **Optional**: Event bus integration is optional and can be disabled
- **Gradual Rollout**: Can be enabled per service or globally
- **Monitoring**: Events are automatically included in existing monitoring systems 