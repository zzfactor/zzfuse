# Trazr Extension API

## Overview

This extension exposes versioned HTTP APIs for querying OpenTelemetry logs, traces, and metrics from the Trazr backend. All endpoints are under `/api/v1/` and return JSON.

---

## Endpoints

### 1. Logs

**GET /api/v1/logs**

Query logs with flexible filters and cursor-based pagination.

**Query Parameters:**
- `start` (RFC3339, optional): Start time (default: 1 hour ago)
- `end` (RFC3339, optional): End time (default: now)
- `service` (string, optional): Filter by service name
- `severity` (string, optional): Filter by severity text
- `limit` (int, optional): Max results (default: 100)
- `cursor` (string, optional): Base64-encoded cursor for pagination (see below)
- `schema` (string, optional): Postgres schema to use (default: `otel`)

**Example:**
```
GET /api/v1/logs?service=api&start=2024-06-01T00:00:00Z&end=2024-06-01T01:00:00Z&limit=50
```

**Pagination Example:**
```
GET /api/v1/logs?limit=10
# Response includes: "NextCursor": "...", "HasMore": true
GET /api/v1/logs?limit=10&cursor=... # Use NextCursor from previous response
```

---

### 2. Traces

**GET /api/v1/traces**

Query traces with flexible filters and cursor-based pagination.

**Query Parameters:**
- `start` (RFC3339, optional): Start time (default: 1 hour ago)
- `end` (RFC3339, optional): End time (default: now)
- `service` (string, optional): Filter by service name
- `trace_id` (string, optional): Filter by trace ID
- `limit` (int, optional): Max results (default: 100)
- `cursor` (string, optional): Base64-encoded cursor for pagination
- `schema` (string, optional): Postgres schema to use (default: `otel`)

**Example:**
```
GET /api/v1/traces?service=api&trace_id=abc123&limit=10
```

**Pagination Example:**
```
GET /api/v1/traces?limit=10
# Response includes: "NextCursor": "...", "HasMore": true
GET /api/v1/traces?limit=10&cursor=... # Use NextCursor from previous response
```

---

### 3. Metrics

**GET /api/v1/metrics**

Query metrics from any supported OpenTelemetry metric table, with cursor-based pagination.

**Query Parameters:**
- `type` (string, required): Metric type. One of:
  - `gauge`
  - `sum`
  - `histogram`
  - `summary`
  - `exponential_histogram`
- `start` (RFC3339, optional): Start time (default: 1 hour ago)
- `end` (RFC3339, optional): End time (default: now)
- `service` (string, optional): Filter by service name
- `metric` (string, optional): Filter by metric name
- `limit` (int, optional): Max results (default: 100)
- `cursor` (string, optional): Base64-encoded cursor for pagination
- `schema` (string, optional): Postgres schema to use (default: `otel`)

**Example:**
```
GET /api/v1/metrics?type=gauge&service=api&metric=request_count&limit=20
```

**Pagination Example:**
```
GET /api/v1/metrics?type=gauge&limit=10
# Response includes: "NextCursor": "...", "HasMore": true
GET /api/v1/metrics?type=gauge&limit=10&cursor=... # Use NextCursor from previous response
```

**Response:**
- Returns a JSON object with an array of metric entries matching the selected type.
- Each entry contains all fields from the corresponding metrics table.
- All JSON/JSONB fields (e.g., Attributes, Metadata) are returned as proper JSON objects, not base64 strings.
- Pagination fields: `NextCursor` (base64 string, empty if no more results), `HasMore` (boolean).

---

## Pagination Details

- All endpoints support cursor-based pagination using a base64-encoded cursor string.
- The `NextCursor` field in the response should be passed as the `cursor` parameter in the next request.
- The `HasMore` boolean indicates if more results are available.
- Cursors are opaque to clients and encode the last row's timestamp and ID.
- If `NextCursor` is empty or `HasMore` is false, there are no more results.

---

## Error Handling

- All endpoints return standard HTTP error codes and a JSON error object on failure.
- Example:
```
{
  "error": "metric type is required (gauge, sum, histogram, summary, exponential_histogram)"
}
```

---

## Schema Parameter

- All endpoints accept an optional `schema` query parameter to control which Postgres schema is used (e.g., `otel.logs`).
- Default: `otel`
- Only valid Postgres identifiers are allowed (letters, numbers, _, -).
- The schema name is sanitized to prevent SQL injection. Invalid or missing values default to `otel`.

---

## Streaming Endpoints (WebSocket)

The following endpoints provide real-time streaming of new logs, traces, and metrics using WebSockets. Each client receives new signals as soon as they are inserted into the database (via Postgres LISTEN/NOTIFY triggers).

### Endpoints
- `/api/v1/stream/logs` — Streams new log entries
- `/api/v1/stream/traces` — Streams new trace entries
- `/api/v1/stream/metrics` — Streams new metric (gauge) entries
- `/api/v1/stream/metrics/sum` — Streams new metric (sum) entries
- `/api/v1/stream/metrics/histogram` — Streams new metric (histogram) entries
- `/api/v1/stream/metrics/summary` — Streams new metric (summary) entries
- `/api/v1/stream/metrics/exponential_histogram` — Streams new metric (exponential histogram) entries

### How it works
- Each endpoint upgrades the HTTP connection to a WebSocket.
- The server subscribes to the relevant Postgres NOTIFY channel (e.g., `otel.new_log`, `otel.new_metric_sum`, etc.).
- Each new signal is sent as a JSON message to all connected clients.
- Only new signals are streamed (no history, at-most-once delivery).
- If the client disconnects, it must reconnect to resume streaming.
- Payloads are limited to 8KB (Postgres NOTIFY limit).

#### Trigger and LISTEN/UNLISTEN Behavior
- The **PostgreSQL trigger** that issues NOTIFY is **permanent** and always fires on every insert, regardless of listeners.
- The **Go process** only issues a LISTEN for a channel when at least one client is connected to the corresponding WebSocket endpoint. When the last client disconnects, the Go process issues an UNLISTEN for that channel.
- **Notifications are only delivered to active listeners**. If no Go process is listening, notifications are dropped by PostgreSQL (not queued).
- This means streaming is only active when a client is connected, but the trigger always fires.

### Example (JavaScript)
```js
const ws = new WebSocket('ws://localhost:18888/api/v1/stream/metrics/sum');
ws.onmessage = (event) => {
  const metric = JSON.parse(event.data);
  console.log('New sum metric:', metric);
};
```

### Limitations
- Only new signals are streamed (no replay or catch-up).
- Delivery is at-most-once (notifications may be lost if the client is disconnected or slow).
- Filtering is not yet supported at the subscription level (all new signals are sent to all clients).

---

## Network (WebSocket Keepalive)

WebSocket streaming reliability is tuned via the following extension configuration parameters. Defaults are applied if the YAML omits them.

### Parameters (under `extensions.trazr`)

- `ws_read_timeout` (duration)
  - Default: `90s`
  - Purpose: Server read deadline, extended on each Pong.
- `ws_read_limit` (bytes)
  - Default: `1048576` (~1 MiB)
  - Purpose: Maximum size the server will read per inbound message.
- `ws_ping_interval` (duration)
  - Default: `0` (disabled)
  - Purpose: If > 0, server sends Ping control frames on this interval.
- `ws_write_timeout` (duration)
  - Default: not applied yet
  - Purpose: Reserved for future use to standardize write deadlines.

Notes:
- Client pings are supported and preferred where possible. The server continuously reads and auto-Pongs, extending the read deadline on each Pong.
- Enable `ws_ping_interval` (e.g., `30s`) if clients cannot Ping (e.g., browsers) or if intermediaries drop idle connections.

### Example

```yaml
extensions:
  trazr:
    ws_read_timeout: 90s
    ws_read_limit: 1048576
    # ws_ping_interval: 30s
    # ws_write_timeout: 5s
```
---

## API Best Practices

- All JSON/JSONB fields are returned as proper JSON objects, not base64 strings.
- Use the `limit` and `cursor` parameters for efficient, scalable pagination.
- Use the `HasMore` flag to determine if more results are available.
- Always check for and handle error responses.
- Use the `schema` parameter to query custom Postgres schemas if needed.

---

For more details, see the code in `trazrextension/internal/signals_query.go` and related files.

## Schema Parameter (Required for All Queries)

All endpoints require a `schema` query parameter. This controls which Postgres schema is used for queries (e.g., `otel.logs`).

- Default: `otel`
- Example: `/api/v1/logs?schema=otel` or `/api/v1/logs?schema=customschema`
- Only valid Postgres identifiers are allowed (letters, numbers, _, -)

**Security Note:** The schema name is sanitized to prevent SQL injection. Invalid or missing values default to `otel`.

## Streaming Endpoints (WebSocket)

The following endpoints provide real-time streaming of new logs, traces, and metrics using WebSockets. Each client receives new signals as soon as they are inserted into the database (via Postgres LISTEN/NOTIFY triggers).

### Endpoints
- `/api/v1/stream/logs` — Streams new log entries
- `/api/v1/stream/traces` — Streams new trace entries
- `/api/v1/stream/metrics` — Streams new metric (gauge) entries

### How it works
- Each endpoint upgrades the HTTP connection to a WebSocket.
- The server subscribes to the relevant Postgres NOTIFY channel (e.g., `new_log`).
- Each new signal is sent as a JSON message to all connected clients.
- Only new signals are streamed (no history, at-most-once delivery).
- If the client disconnects, it must reconnect to resume streaming.
- Payloads are limited to 8KB (Postgres NOTIFY limit).

### Example (JavaScript)
```js
const ws = new WebSocket('ws://localhost:18888/api/v1/stream/logs');
ws.onmessage = (event) => {
  const log = JSON.parse(event.data);
  console.log('New log:', log);
};
```

### Limitations
- Only new signals are streamed (no replay or catch-up).
- Delivery is at-most-once (notifications may be lost if the client is disconnected or slow).
- Filtering is not yet supported at the subscription level (all new signals are sent to all clients). 