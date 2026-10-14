package internal

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/metrics"
)

// StreamPerfMonitorWS streams periodic performance/health metrics over WebSocket.
// Supports optional preload of a snapshot and configurable update interval.
func StreamPerfMonitorWS(conn *websocket.Conn, r *http.Request) {
	// Query params
	intervalStr := r.URL.Query().Get("interval") // seconds
	if intervalStr == "" {
		intervalStr = "2"
	}
	intervalSec, err := strconv.Atoi(intervalStr)
	if err != nil || intervalSec <= 0 {
		intervalSec = 2
	}
	// summary=true sends a compact subset; otherwise full metrics
	summary := parseBool(r.URL.Query().Get("summary"))
	// preload=true sends an initial snapshot immediately
	preload := parseBool(r.URL.Query().Get("preload"))
	// align with other streams: wait_for_established flag
	waitForEstablished := parseBool(r.URL.Query().Get("wait_for_established"))

	// Optional: fields filter (comma-separated)
	fieldsRaw := r.URL.Query().Get("fields")
	var fieldsFilter map[string]struct{}
	if strings.TrimSpace(fieldsRaw) != "" {
		fieldsFilter = make(map[string]struct{})
		for _, f := range strings.Split(fieldsRaw, ",") {
			f = strings.TrimSpace(f)
			if f != "" {
				fieldsFilter[strings.ToLower(f)] = struct{}{}
			}
		}
	}

	// helper to filter metrics map
	selectFields := func(all map[string]interface{}) map[string]interface{} {
		if len(fieldsFilter) == 0 {
			return all
		}
		out := make(map[string]interface{})
		for k, v := range all {
			if _, ok := fieldsFilter[strings.ToLower(k)]; ok {
				out[k] = v
			}
		}
		return out
	}

	// Preload snapshot
	readySent := false
	if preload {
		var data map[string]interface{}
		if summary {
			data = metrics.GetMetricsSummary()
		} else {
			data = metrics.GetAllMetrics()
		}
		payload := map[string]interface{}{
			"type":    "perf_update",
			"metrics": selectFields(data),
		}
		consecutiveWriteErrors := 0
		if err := wsWriteWithTimeout(conn, payload, &consecutiveWriteErrors); err != nil {
			return
		}
		_ = SafeWriteJSON(conn, map[string]string{"type": "ready"})
		readySent = true
	}

	// Periodic streaming
	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	consecutiveWriteErrors := 0
	droppedSinceNotice := 0

	// If caller wants to wait for establishment and we haven't sent ready yet, send it now
	if waitForEstablished && !readySent {
		_ = conn.WriteJSON(map[string]string{"type": "ready"})
		readySent = true
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			// gather core metrics
			var data map[string]interface{}
			if summary {
				data = metrics.GetMetricsSummary()
			} else {
				data = metrics.GetAllMetrics()
			}

			// add system CPU/memory utilization
			// sys := metrics.GetSystemMetrics()
			// data["system"] = sys

			// compute alloc rate from memory stats (bytes/s -> MB/s)
			// Use MemoryMetricsCollector snapshot
			memStats := metrics.GetMemoryMetrics()
			var totalAlloc int64
			if v, ok := memStats["total_alloc"].(int64); ok {
				totalAlloc = v
			}
			// inline simple previous value tracking via request context
			type ctxKey string
			const kPrevAlloc ctxKey = "prev_total_alloc"
			const kPrevTime ctxKey = "prev_time"
			prevAllocAny := r.Context().Value(kPrevAlloc)
			prevTimeAny := r.Context().Value(kPrevTime)
			now := time.Now()
			allocRateMBs := 0.0
			if pa, ok := prevAllocAny.(int64); ok {
				if pt, ok := prevTimeAny.(time.Time); ok {
					dt := now.Sub(pt).Seconds()
					if dt > 0 {
						allocRateMBs = float64(totalAlloc-pa) / dt / (1024 * 1024)
					}
				}
			}
			// update context by recreating request with values
			ctx := r.Context()
			ctx = context.WithValue(ctx, kPrevAlloc, totalAlloc)
			ctx = context.WithValue(ctx, kPrevTime, now)
			*r = *r.WithContext(ctx)

			data["memory_util"] = map[string]interface{}{
				"alloc_rate_mb_s": allocRateMBs,
			}

			// add network rates using two samples around the tick interval
			// capture previous from context, compute rate to current
			// const kPrevNet ctxKey = "prev_net"
			// prevNetAny := r.Context().Value(kPrevNet)
			// currNet := metrics.SampleNetwork()
			// if pn, ok := prevNetAny.(metrics.NetSample); ok {
			// 	data["network"] = pn.Rate(currNet)
			// }
			// ctx = context.WithValue(ctx, kPrevNet, currNet)
			// *r = *r.WithContext(ctx)

			payload := map[string]interface{}{
				"type":    "perf_update",
				"metrics": selectFields(data),
			}
			prev := consecutiveWriteErrors
			if err := wsWriteWithTimeout(conn, payload, &consecutiveWriteErrors); err != nil {
				return
			}
			if consecutiveWriteErrors > prev {
				droppedSinceNotice += (consecutiveWriteErrors - prev)
				if droppedSinceNotice >= 10 {
					_ = SafeWriteJSON(conn, map[string]interface{}{"type": "dropped", "count": droppedSinceNotice})
					droppedSinceNotice = 0
				}
			}
		}
	}
}
