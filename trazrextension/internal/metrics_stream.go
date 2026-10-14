package internal

import (
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/repository"
)

func parseMetricFilterParams(r *http.Request) metricFilterParams {
	// Use snake_case parameter names to match OpenAPI spec
	return metricFilterParams{
		ServiceName: r.URL.Query().Get("service"),
		MetricName:  r.URL.Query().Get("metric_name"),
	}
}

func matchGaugeMetricFilter(entry repository.GaugeMetricEntry, params metricFilterParams) bool {
	if params.ServiceName != "" && entry.ServiceName != params.ServiceName {
		return false
	}
	if params.MetricName != "" && entry.Name != nil && *entry.Name != params.MetricName {
		return false
	}
	return true
}

// Metrics streaming: use boolean wait_for_established
func StreamFilteredGaugeMetricsWS(repo repository.GaugeMetricStreamRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		params := parseMetricFilterParams(r)
		ctx := r.Context()
		ch, cancel, establishedC, err := repo.SubscribeGaugeMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, "stream error: "+err.Error())
			return
		}
		defer cancel()
		if parseBool(r.URL.Query().Get("wait_for_established")) {
			<-establishedC
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()
		startWSControlRead(ctx, conn)
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchGaugeMetricFilter(entry, params) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	}
}

// Similar updates for Sum/Histogram/Summary/ExponentialHistogram
func matchSumMetricFilter(entry repository.SumMetricEntry, params metricFilterParams) bool {
	if params.ServiceName != "" && entry.ServiceName != params.ServiceName {
		return false
	}
	if params.MetricName != "" && entry.Name != nil && *entry.Name != params.MetricName {
		return false
	}
	return true
}

func StreamFilteredSumMetricsWS(repo repository.SumMetricStreamRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		params := parseMetricFilterParams(r)
		ctx := r.Context()
		ch, cancel, establishedC, err := repo.SubscribeSumMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, "stream error: "+err.Error())
			return
		}
		defer cancel()
		if parseBool(r.URL.Query().Get("wait_for_established")) {
			<-establishedC
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()
		startWSControlRead(ctx, conn)
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchSumMetricFilter(entry, params) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	}
}

func matchHistogramMetricFilter(entry repository.HistogramMetricEntry, params metricFilterParams) bool {
	if params.ServiceName != "" && entry.ServiceName != params.ServiceName {
		return false
	}
	if params.MetricName != "" && entry.Name != nil && *entry.Name != params.MetricName {
		return false
	}
	return true
}

func StreamFilteredHistogramMetricsWS(repo repository.HistogramMetricStreamRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		params := parseMetricFilterParams(r)
		ctx := r.Context()
		ch, cancel, establishedC, err := repo.SubscribeHistogramMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, "stream error: "+err.Error())
			return
		}
		defer cancel()
		if parseBool(r.URL.Query().Get("wait_for_established")) {
			<-establishedC
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()
		startWSControlRead(ctx, conn)
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchHistogramMetricFilter(entry, params) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	}
}

func matchSummaryMetricFilter(entry repository.SummaryMetricEntry, params metricFilterParams) bool {
	if params.ServiceName != "" && entry.ServiceName != params.ServiceName {
		return false
	}
	if params.MetricName != "" && entry.Name != nil && *entry.Name != params.MetricName {
		return false
	}
	return true
}

func StreamFilteredSummaryMetricsWS(repo repository.SummaryMetricStreamRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		params := parseMetricFilterParams(r)
		ctx := r.Context()
		ch, cancel, establishedC, err := repo.SubscribeSummaryMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, "stream error: "+err.Error())
			return
		}
		defer cancel()
		if parseBool(r.URL.Query().Get("wait_for_established")) {
			<-establishedC
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()
		startWSControlRead(ctx, conn)
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchSummaryMetricFilter(entry, params) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	}
}

func matchExponentialHistogramMetricFilter(entry repository.ExponentialHistogramMetricEntry, params metricFilterParams) bool {
	if params.ServiceName != "" && entry.ServiceName != params.ServiceName {
		return false
	}
	if params.MetricName != "" && entry.Name != nil && *entry.Name != params.MetricName {
		return false
	}
	return true
}

func StreamFilteredExponentialHistogramMetricsWS(repo repository.ExponentialHistogramMetricStreamRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		params := parseMetricFilterParams(r)
		ctx := r.Context()
		ch, cancel, establishedC, err := repo.SubscribeExponentialHistogramMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, "stream error: "+err.Error())
			return
		}
		defer cancel()
		if parseBool(r.URL.Query().Get("wait_for_established")) {
			<-establishedC
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()
		startWSControlRead(ctx, conn)
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchExponentialHistogramMetricFilter(entry, params) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	}
}

// Consolidated metrics WS with preload then live dispatch
func StreamMetrics(
	gRepo repository.GaugeMetricStreamRepository,
	sRepo repository.SumMetricStreamRepository,
	hRepo repository.HistogramMetricStreamRepository,
	suRepo repository.SummaryMetricStreamRepository,
	exRepo repository.ExponentialHistogramMetricStreamRepository,
	querySvc repository.MetricQueryService,
	conn *websocket.Conn,
	r *http.Request,
) {
	// Use common query parameter parsing
	commonParams := ParseCommonQueryParams(r, 1000)
	schema, allowed := authorizedStreamSchema(r)
	if !allowed {
		return
	}

	qv := r.URL.Query()
	// Use snake_case parameter names to match OpenAPI spec
	typeParam := strings.ToLower(qv.Get("type"))
	service := qv.Get("service")
	metric := qv.Get("metric_name")
	wait := parseBool(qv.Get("wait_for_established"))

	ctx := r.Context()

	// Preload if cursor or time parameters are provided
	if commonParams.Cursor != "" || commonParams.Start != nil {
		var mq repository.MetricQuery

		if commonParams.Cursor != "" {
			// Cursor-based preload - ignore time parameters when cursor is present
			mq = repository.MetricQuery{
				MetricName: metric,
			}
			// Set embedded CommonBaseFilters fields
			mq.CommonBaseFilters.Cursor = commonParams.Cursor
			mq.CommonBaseFilters.ServiceName = service
			mq.CommonBaseFilters.Schema = commonParams.Schema
			mq.CommonBaseFilters.Direction = "asc" // Always ascending for streaming preload
			mq.CommonBaseFilters.Limit = commonParams.Limit
		} else {
			// Time-based preload (existing behavior)
			mq = repository.MetricQuery{
				TimeRange:  repository.TimeRange{Start: commonParams.Start, End: commonParams.End},
				MetricName: metric,
			}
			// Set embedded CommonBaseFilters fields
			mq.CommonBaseFilters.ServiceName = service
			mq.CommonBaseFilters.Schema = commonParams.Schema
			mq.CommonBaseFilters.Direction = "asc"
			mq.CommonBaseFilters.Limit = commonParams.Limit
		}
		var res repository.MetricQueryResult
		switch typeParam {
		case "", "gauge":
			res, _ = querySvc.QueryMetricsGauge(ctx, mq)
		case "sum":
			res, _ = querySvc.QueryMetricsSum(ctx, mq)
		case "histogram":
			res, _ = querySvc.QueryMetricsHistogram(ctx, mq)
		case "summary":
			res, _ = querySvc.QueryMetricsSummary(ctx, mq)
		case "exponential_histogram":
			res, _ = querySvc.QueryMetricsExponentialHistogram(ctx, mq)
		default:
			_ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": "unsupported metric type"})
			return
		}
		consecutiveWriteErrors := 0
		for _, e := range res.Entries {
			if err := wsWriteWithTimeout(conn, e, &consecutiveWriteErrors); err != nil {
				return
			}
		}
		// Optional ready marker
		_ = SafeWriteJSON(conn, map[string]string{"type": "ready"})
	}

	// Live stream subscribe based on type
	startWSControlRead(ctx, conn)
	switch typeParam {
	case "", "gauge":
		ch, cancel, establishedC, err := gRepo.SubscribeGaugeMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			_ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": "stream error"})
			return
		}
		defer cancel()
		if wait {
			<-establishedC
		}
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchGaugeMetricFilter(entry, metricFilterParams{ServiceName: service, MetricName: metric}) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	case "sum":
		ch, cancel, establishedC, err := sRepo.SubscribeSumMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			_ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": "stream error"})
			return
		}
		defer cancel()
		if wait {
			<-establishedC
		}
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchSumMetricFilter(entry, metricFilterParams{ServiceName: service, MetricName: metric}) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	case "histogram":
		ch, cancel, establishedC, err := hRepo.SubscribeHistogramMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			_ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": "stream error"})
			return
		}
		defer cancel()
		if wait {
			<-establishedC
		}
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchHistogramMetricFilter(entry, metricFilterParams{ServiceName: service, MetricName: metric}) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	case "summary":
		ch, cancel, establishedC, err := suRepo.SubscribeSummaryMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			_ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": "stream error"})
			return
		}
		defer cancel()
		if wait {
			<-establishedC
		}
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchSummaryMetricFilter(entry, metricFilterParams{ServiceName: service, MetricName: metric}) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	case "exponential_histogram":
		ch, cancel, establishedC, err := exRepo.SubscribeExponentialHistogramMetrics(ctx, repository.MetricStreamFilter{CommonBaseFilters: repository.CommonBaseFilters{Schema: schema}})
		if err != nil {
			_ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": "stream error"})
			return
		}
		defer cancel()
		if wait {
			<-establishedC
		}
		consecutiveWriteErrors := 0
		for entry := range ch {
			if !matchExponentialHistogramMetricFilter(entry, metricFilterParams{ServiceName: service, MetricName: metric}) {
				continue
			}
			if err := wsWriteWithTimeout(conn, entry, &consecutiveWriteErrors); err != nil {
				return
			}
		}
	default:
		_ = SafeWriteJSON(conn, map[string]string{"type": "error", "message": "unsupported metric type"})
		return
	}
}
