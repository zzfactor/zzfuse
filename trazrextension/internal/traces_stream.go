package internal

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// Spans streaming: use boolean wait_for_established
func StreamFilteredSpansWS(repo repository.TraceStreamRepository, conn *websocket.Conn, r *http.Request) {
	schema, allowed := authorizedStreamSchema(r)
	if !allowed {
		return
	}
	// Use snake_case parameter names to match OpenAPI spec
	service := r.URL.Query().Get("service")
	traceID := r.URL.Query().Get("trace_id")
	spanID := r.URL.Query().Get("span_id")
	spanName := r.URL.Query().Get("span_name")
	waitForEstablished := parseBool(r.URL.Query().Get("wait_for_established"))
	ctx := r.Context()
	traceCh, cancel, establishedC, err := repo.SubscribeSpans(ctx, repository.TraceFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
	if err != nil {
		fmt.Printf("[StreamFilteredSpansWS] SubscribeSpans error: %v\n", err)
		return
	}
	defer cancel()
	if waitForEstablished {
		<-establishedC
	}
	fmt.Printf("[StreamFilteredSpansWS] WebSocket connection established\n")
	defer func() {
		fmt.Printf("[StreamFilteredSpansWS] Closing WebSocket connection\n")
		_ = SafeWriteControl(conn, websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "trace stream ended"), time.Now().Add(time.Second))
		conn.Close()
	}()
	startWSControlRead(ctx, conn)
	consecutiveWriteErrors := 0
	for entry := range traceCh {
		if !matchTraceFilter(entry, service, traceID, spanID, spanName) {
			continue
		}
		if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
			return
		}
	}
}

// Spans streaming with optional preload
func StreamSpans(repo repository.TraceStreamRepository, querySvc repository.TraceQueryService, conn *websocket.Conn, r *http.Request) {
	// Use common query parameter parsing
	commonParams := ParseCommonQueryParams(r, 1000)
	schema, allowed := authorizedStreamSchema(r)
	if !allowed {
		return
	}

	qv := r.URL.Query()
	// Use snake_case parameter names to match OpenAPI spec
	service := qv.Get("service")
	traceID := qv.Get("trace_id")
	spanName := qv.Get("span_name")
	waitForEstablished := parseBool(qv.Get("wait_for_established"))

	ctx := r.Context()

	traceCh, cancel, establishedC, err := repo.SubscribeSpans(ctx, repository.TraceFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
	if err != nil {
		fmt.Printf("[StreamFilteredSpansWS] SubscribeSpans error: %v\n", err)
		return
	}
	defer cancel()
	if waitForEstablished {
		<-establishedC
	}

	// Preload if cursor or time parameters are provided
	if commonParams.Cursor != "" || commonParams.Start != nil {
		var preQ repository.TraceQuery

		if commonParams.Cursor != "" {
			// Cursor-based preload - ignore time parameters when cursor is present
			preQ = repository.TraceQuery{
				TraceID: traceID,
			}
			// Set embedded CommonBaseFilters fields
			preQ.CommonBaseFilters.Cursor = commonParams.Cursor
			preQ.CommonBaseFilters.ServiceName = service
			preQ.CommonBaseFilters.Schema = commonParams.Schema
			preQ.CommonBaseFilters.Direction = commonParams.Direction // Respect user's direction preference
			preQ.CommonBaseFilters.Limit = commonParams.Limit
		} else {
			// Time-based preload (existing behavior)
			preQ = repository.TraceQuery{
				TimeRange: repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
				TraceID:   traceID,
			}
			// Set embedded CommonBaseFilters fields
			preQ.CommonBaseFilters.ServiceName = service
			preQ.CommonBaseFilters.Schema = commonParams.Schema
			preQ.CommonBaseFilters.Direction = commonParams.Direction // Respect user's direction preference
			preQ.CommonBaseFilters.Limit = commonParams.Limit
		}

		if res, err := querySvc.QuerySpans(ctx, preQ); err == nil {
			consecutiveWriteErrors := 0
			for _, e := range res.Entries {
				if err := wsWriteWithTimeout(conn, e, &consecutiveWriteErrors); err != nil {
					return
				}
			}
		}
		// Optional ready marker to signal preload complete
		_ = SafeWriteJSON(conn, map[string]string{"type": "ready"})
	}

	defer func() {
		_ = SafeWriteControl(conn, websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "trace stream ended"), time.Now().Add(time.Second))
		conn.Close()
	}()
	startWSControlRead(ctx, conn)
	consecutiveWriteErrors := 0
	for entry := range traceCh {
		if !matchTraceFilter(entry, service, traceID, "", spanName) {
			continue
		}
		if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
			return
		}
	}
}
