# WebSocket Coding Guidelines (trazr-otel)

These conventions ensure reliable, race-free WebSocket endpoints across the project.

## Core Principles
- **Serialize all writes per-connection.** Use `SafeWriteJSON()` and `SafeWriteControl()` from `trazrextension/internal/otel_stream.go`.
- **Register a per-connection write lock.** Call `RegisterWSSafe(conn)` after upgrade and `UnregisterWSSafe(conn)` on close.
- **Never perform concurrent reads.** Each connection must have at most one reader goroutine.
- **Continuously read to service control frames.** Keep a read loop or use `startWSControlRead()` to process Ping/Pong and avoid timeouts.
- **Set read limits and deadlines.** Always set `SetReadLimit()` and `SetReadDeadline()`; extend the deadline on Pong.
- **Prefer existing helpers.** Use `wsWriteWithTimeout()` for data frames and keep backpressure handling consistent.

## Lifecycle & Registration
- WebSocket handlers should be wrapped by the auth wrappers in `trazrextension/extension.go`, which already:
  - Upgrade, authenticate, and configure read limit/deadline and ping/pong handlers.
  - Call `internal.RegisterWSSafe(conn)` and defer `internal.UnregisterWSSafe(conn)` and `conn.Close()`.
- Inside handlers, do not reconfigure read handlers unless necessary; do not start additional readers beyond the one intended for the stream.

## Reads (Control/Data)
- Start a control-frame reader: `startWSControlRead(ctx, conn)` if the handler itself is responsible for reading control/data frames.
- If you run a custom read loop for client messages (e.g., filter changes), that loop is the only reader.
- On Pong: extend read deadline. On Ping: let the default handler respond, then extend the deadline.

## Writes
- **JSON/data frames:** `SafeWriteJSON(conn, payload)` or via `wsWriteWithTimeout(conn, payload, &consecutiveWriteErrors)` for backpressure accounting.
- **Control frames (Ping/Close):** `SafeWriteControl(conn, messageType, data, deadline)`.
- **Never call** `conn.WriteMessage`, `conn.WriteJSON`, or `conn.WriteControl` directly.
- **Avoid logging storms:** if `consecutiveWriteErrors` increases, batch or drop, then send periodic `{"type":"dropped", "count":N}` notices via safe writes.

## Timeouts, Limits & Backpressure
- Use a sensible read limit (~1MB) and read timeout (e.g., 120s) as configured in `extension.go`.
- Use `wsWriteWithTimeout` to bound write time and detect slow clients.
- Increment `consecutiveWriteErrors` on write errors, and emit batched `dropped` notices (e.g., every 10 drops).

## Preload Pattern (Optional)
- If the stream supports a preload snapshot:
  - Stream preload entries using `wsWriteWithTimeout` to preserve backpressure tracking.
  - Send a ready marker: `SafeWriteJSON(conn, map[string]string{"type":"ready"})` once preload completes.

## Closing
- On handler exit (in a `defer`): send a Close control frame via `SafeWriteControl(conn, websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "<reason>"), time.Now().Add(1*time.Second))`, then close the connection.
- Ensure no other goroutines are writing at shutdown; the mutex ensures serialization, but still signal exits cleanly via context cancellation.

## Error Handling
- Log unexpected errors at warn/error level with context (service/name/session/id if available), but avoid spamming logs on repeated transient write errors.
- For client protocol errors (bad messages), respond with a safe JSON error and continue when possible.

## Do / Don’t
- **Do**: Use safe helpers for all writes.
- **Do**: Maintain a single reader per connection.
- **Do**: Keep read deadlines fresh on Ping/Pong.
- **Do**: Use `wsWriteWithTimeout` for stream entries.
- **Don’t**: Spawn multiple readers on the same conn.
- **Don’t**: Call `conn.Write*` directly.
- **Don’t**: Block read loops on slow writes; reads and writes are independent.

## Minimal Handler Template (Go)

```go
func StreamExampleWS(repo Repo, qSvc QuerySvc, conn *websocket.Conn, r *http.Request) {
    ctx, cancel := context.WithCancel(r.Context())
    defer cancel()

    // Service control frames
    startWSControlRead(ctx, conn)

    // Optional preload
    if shouldPreload(r) {
        consecutive := 0
        res, err := qSvc.Query(...)
        if err != nil {
            _ = SafeWriteJSON(conn, map[string]string{"type":"error","message":"query failed"})
            return
        }
        for _, e := range res.Entries {
            if err := wsWriteWithTimeout(conn, e, &consecutive); err != nil {
                return
            }
        }
        _ = SafeWriteJSON(conn, map[string]string{"type":"ready"})
    }

    // Live stream
    ch, cancelSub, established, err := repo.Subscribe(ctx, Filter{})
    if err != nil {
        _ = SafeWriteJSON(conn, map[string]string{"type":"error","message":"subscribe failed"})
        return
    }
    defer cancelSub()

    // Example: wait for establishment
    <-established

    consecutive := 0
    dropped := 0
    for e := range ch {
        prev := consecutive
        if err := wsWriteWithTimeout(conn, e, &consecutive); err != nil {
            return
        }
        if consecutive > prev {
            dropped++
            if dropped >= 10 {
                _ = SafeWriteJSON(conn, map[string]interface{}{"type":"dropped","count": dropped})
                dropped = 0
            }
        }
    }
}
```

## Implementation Checklist
- [ ] Handler is wrapped by `wsAuthHandlerWrapper` or `wsScopedAuthHandlerWrapper` in `trazrextension/extension.go`.
- [ ] `RegisterWSSafe(conn)` called post-upgrade; `UnregisterWSSafe(conn)` deferred.
- [ ] Exactly one reader per connection; control frames serviced (via `startWSControlRead` or custom loop).
- [ ] All writes use `SafeWriteJSON`/`SafeWriteControl` or `wsWriteWithTimeout`.
- [ ] Preload phase (if any) ends with `{"type":"ready"}` marker.
- [ ] Backpressure handled via `consecutiveWriteErrors` and periodic `dropped` notices.
- [ ] Close frame sent via `SafeWriteControl` before `conn.Close()` in `defer`.
- [ ] Basic build and runtime tests pass; optionally verify with `-race`.

## References
- Gorilla WebSocket: Writes must be serialized per connection.
- Project helpers: see `trazrextension/internal/otel_stream.go` and `trazrextension/internal/otel_runs.go` for patterns.
