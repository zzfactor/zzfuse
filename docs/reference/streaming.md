# Streaming Client Implementation Guide

This guide provides instructions for implementing client-side support for the Trazr OTEL platform's real-time streaming system via WebSocket connections.

## Overview

The Trazr OTEL platform provides real-time streaming for logs, traces, metrics, and runs via WebSocket connections with:

- **Sub-second latency** for immediate visibility
- **Server-side filtering** to reduce network traffic
- **Automatic reconnection** with exponential backoff
- **Individual entry streaming** (not batches) for better UX

### Architecture Flow

```
Client WebSocket → Trazr Extension → Event Bus → Trazr Exporter → Database
```

## WebSocket Connection

### Available Endpoints

- **Logs**: `ws://localhost:18888/api/v1/stream/logs`
- **Traces**: `ws://localhost:18888/api/v1/stream/spans`
- **Metrics**: `ws://localhost:18888/api/v1/stream/metrics`
- **Runs**: `ws://localhost:18888/api/v1/stream/runs`

### Keepalive and Timeouts

- The server runs a continuous read loop and auto-responds to Ping frames with Pong.
- Server read deadlines are extended on each Pong; default read timeout is 90s (configurable via `extensions.trazr.ws_read_timeout`).
- If your client cannot send Ping control frames (e.g., browser), either:
  - Enable server pings via `extensions.trazr.ws_ping_interval: 30s`, or
  - Send a small heartbeat message at an interval to keep the connection active.
- Max inbound message size is limited by `extensions.trazr.ws_read_limit` (default ~1 MiB).

### Query Parameters

```javascript
// Basic filtering
const wsUrl = 'ws://localhost:18888/api/v1/stream/logs?service_name=my-service&severity=ERROR';

// Run-specific filtering
const runUrl = 'ws://localhost:18888/api/v1/stream/runs?service_name=my-service&session_id=user-123&active_only=true';

// Wait for subscription establishment
const wsUrl = 'ws://localhost:18888/api/v1/stream/logs?wait_for_established=1';
```

**Available Filters:**
- `service_name` - Filter by service
- `severity` - Filter by log severity (supports wildcards)
- `name` - Filter by run name
- `session_id` - Filter by session ID
- `run_type` - Filter by run type (1: time-based, 2: trace-based)
- `active_only` - Only active runs (for runs endpoint)

## Event Format

### Log Entry Response

```typescript
interface LogEntryResponse {
  id: string;                    // Unique log ID
  timestamp: string;             // RFC3339 timestamp
  service_name: string;          // Service name
  severity_text: string;        // Log severity
  body: string;                 // Log message
  trace_id?: string;            // OpenTelemetry trace ID
  span_id?: string;             // OpenTelemetry span ID
  attributes?: Record<string, any>; // Log attributes
  metadata?: Record<string, any>;   // Additional metadata
}
```

### Run Entry Response

```typescript
interface RunEntryResponse {
  id: number;                    // Unique run ID
  service_name: string;          // Service name
  name: string;                  // Run name
  start_timestamp_ns: number;    // Start timestamp (nanoseconds)
  end_timestamp_ns?: number;     // End timestamp (null for active runs)
  session_id?: string;           // Session ID for grouping
  run_type?: number;            // Run type (1: time-based, 2: trace-based)
  metadata?: Record<string, any>; // Run metadata
  created_at: string;           // Creation timestamp
  updated_at: string;           // Last update timestamp
}
```

## Client Implementation

### Basic WebSocket Client

```javascript
class TrazrStreamingClient {
  constructor(url = 'ws://localhost:18888/api/v1/stream/logs') {
    this.url = url;
    this.ws = null;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;
    this.listeners = new Map();
    this.isConnected = false;
    this.dataType = this.detectDataType(url);
  }

  detectDataType(url) {
    if (url.includes('/stream/runs')) return 'run';
    if (url.includes('/stream/spans')) return 'trace';
    if (url.includes('/stream/metrics')) return 'metric';
    return 'log';
  }

  connect() {
    try {
      this.ws = new WebSocket(this.url);
      this.setupEventHandlers();
    } catch (error) {
      console.error('Failed to create WebSocket connection:', error);
      this.scheduleReconnect();
    }
  }

  setupEventHandlers() {
    this.ws.onopen = () => {
      console.log('WebSocket connected to Trazr streaming');
      this.isConnected = true;
      this.reconnectAttempts = 0;
      this.emit('connected');
    };

    this.ws.onmessage = (event) => {
      try {
        const entry = JSON.parse(event.data);
        
        // Validate and emit based on data type
        if (this.dataType === 'run') {
          if (!entry.id || !entry.service_name || !entry.name) {
            console.warn('Received incomplete run entry:', entry);
            return;
          }
          this.emit('run', entry);
        } else {
          // Default log validation
          if (!entry.id || !entry.timestamp || !entry.service_name || !entry.body) {
            console.warn('Received incomplete log entry:', entry);
            return;
          }
          this.emit('log', entry);
        }
      } catch (error) {
        console.error('Failed to parse entry:', error);
      }
    };

    this.ws.onclose = (event) => {
      console.log('WebSocket disconnected:', event.code, event.reason);
      this.isConnected = false;
      this.emit('disconnected', event);
      
      if (!event.wasClean) {
        this.scheduleReconnect();
      }
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      this.emit('error', error);
    };
  }

  scheduleReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnection attempts reached');
      this.emit('max_reconnect_reached');
      return;
    }

    this.reconnectAttempts++;
    const delay = 1000 * Math.pow(2, this.reconnectAttempts - 1);
    
    setTimeout(() => {
      this.connect();
    }, delay);
  }

  on(event, callback) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    this.listeners.get(event).push(callback);
  }

  emit(event, data) {
    const callbacks = this.listeners.get(event) || [];
    callbacks.forEach(callback => {
      try {
        callback(data);
      } catch (error) {
        console.error(`Error in event listener for ${event}:`, error);
      }
    });
  }

  disconnect() {
    if (this.ws) {
      this.ws.close(1000, 'Client disconnect');
    }
  }
}
```

### React Hook

```javascript
import { useState, useEffect, useCallback } from 'react';

export function useTrazrStreaming(url = 'ws://localhost:18888/api/v1/stream/logs') {
  const [logs, setLogs] = useState([]);
  const [runs, setRuns] = useState([]);
  const [isConnected, setIsConnected] = useState(false);
  const [error, setError] = useState(null);

  useEffect(() => {
    const client = new TrazrStreamingClient(url);

    client.on('connected', () => {
      setIsConnected(true);
      setError(null);
    });

    client.on('disconnected', () => {
      setIsConnected(false);
    });

    client.on('error', (err) => {
      setError(err);
    });

    client.on('log', (logEntry) => {
      setLogs(prev => [...prev.slice(-999), logEntry]); // Keep last 1000
    });

    client.on('run', (runEntry) => {
      setRuns(prev => [...prev.slice(-499), runEntry]); // Keep last 500
    });

    client.connect();

    return () => {
      client.disconnect();
    };
  }, [url]);

  const clearLogs = useCallback(() => setLogs([]), []);
  const clearRuns = useCallback(() => setRuns([]), []);

  return {
    logs,
    runs,
    isConnected,
    error,
    clearLogs,
    clearRuns
  };
}
```

## Examples

### Simple Log Viewer

```jsx
import React from 'react';
import { useTrazrStreaming } from './useTrazrStreaming';

function LogViewer() {
  const { logs, isConnected, error } = useTrazrStreaming();

  if (error) {
    return <div className="error">Connection error: {error.message}</div>;
  }

  return (
    <div className="log-viewer">
      <div className="status">
        Status: {isConnected ? '🟢 Connected' : '🔴 Disconnected'}
      </div>
      
      <div className="logs">
        {logs.map(log => (
          <div key={log.id} className={`log-entry severity-${log.severity_text.toLowerCase()}`}>
            <span className="timestamp">{log.timestamp}</span>
            <span className="service">{log.service_name}</span>
            <span className="severity">{log.severity_text}</span>
            <span className="message">{log.body}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
```

### Run Viewer

```jsx
import React from 'react';
import { useTrazrStreaming } from './useTrazrStreaming';

function RunViewer() {
  const { runs, isConnected, error } = useTrazrStreaming('ws://localhost:18888/api/v1/stream/runs');

  if (error) {
    return <div className="error">Connection error: {error.message}</div>;
  }

  return (
    <div className="run-viewer">
      <div className="status">
        Status: {isConnected ? '🟢 Connected' : '🔴 Disconnected'}
      </div>
      
      <div className="runs">
        {runs.map(run => (
          <div key={run.id} className={`run-entry ${run.end_timestamp_ns ? 'completed' : 'active'}`}>
            <div className="run-header">
              <span className="name">{run.name}</span>
              <span className="service">{run.service_name}</span>
              <span className="status">{run.end_timestamp_ns ? 'Completed' : 'Active'}</span>
            </div>
            <div className="run-details">
              <span className="session">{run.session_id || 'No Session'}</span>
              <span className="duration">
                {run.end_timestamp_ns 
                  ? `${((run.end_timestamp_ns - run.start_timestamp_ns) / 1000000000).toFixed(2)}s`
                  : 'Running...'
                }
              </span>
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
```

## Best Practices

### 1. Memory Management
```javascript
// Limit storage to prevent memory issues
const MAX_LOGS = 1000;
const MAX_RUNS = 500;

setLogs(prev => [...prev.slice(-MAX_LOGS + 1), logEntry]);
```

### 2. Server-Side Filtering
```javascript
// Use query parameters to reduce network traffic
const wsUrl = 'ws://localhost:18888/api/v1/stream/logs?service_name=critical-service&severity=ERROR';
const runUrl = 'ws://localhost:18888/api/v1/stream/runs?active_only=true&session_id=user-123';
```

### 3. Performance Optimization
```javascript
// Use debouncing for UI updates in high-volume scenarios
const debouncedSetLogs = useMemo(() => debounce(setLogs, 100), []);

// Virtual scrolling for large lists
import { FixedSizeList as List } from 'react-window';
```

## Testing

### Manual Testing
```bash
# Using wscat
wscat -c "ws://localhost:18888/api/v1/stream/logs?service_name=test-service"

# Health check
curl http://localhost:18888/health
```

### JavaScript Testing
```javascript
const ws = new WebSocket('ws://localhost:18888/api/v1/stream/logs');

ws.onopen = () => console.log('Connected to Trazr streaming');
ws.onmessage = (event) => console.log('Received:', JSON.parse(event.data));
ws.onerror = (error) => console.error('WebSocket error:', error);
```

## Troubleshooting

### Common Issues

1. **Connection Refused**
   - Verify Trazr extension is running on port 18888
   - Check firewall settings

2. **No Data Received**
   - Verify data is being sent to Trazr exporter
   - Check event bus configuration
   - Monitor extension logs

3. **High Memory Usage**
   - Implement log rotation/limiting
   - Use virtual scrolling for large lists
   - Clear data periodically

4. **Performance Issues**
   - Use server-side filtering
   - Implement UI debouncing
   - Consider virtual scrolling

## Summary

The Trazr streaming system provides real-time visibility into logs, traces, metrics, and runs with:

- **Individual entry streaming** for immediate visibility
- **Server-side filtering** for performance
- **Automatic reconnection** for reliability
- **Comprehensive client libraries** for easy integration

The system is production-ready and optimized for real-time monitoring and debugging applications. 