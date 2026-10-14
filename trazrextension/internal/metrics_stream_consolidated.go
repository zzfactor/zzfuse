package internal

import (
	"context"
	"fmt"
	"net/http"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// MetricStreamFilter holds filter parameters for metric streaming
type MetricStreamFilter struct {
	Schema      string
	ServiceName string
	MetricName  string
	Type        string
}

func authorizedMetricStreamFilter(ctx context.Context, filter any) (repository.MetricStreamFilter, error) {
	params, ok := filter.(MetricStreamFilter)
	if !ok || params.Schema == "" || !auth.UserHasSchemaAccess(ctx, params.Schema) {
		return repository.MetricStreamFilter{}, fmt.Errorf("access denied to stream schema")
	}
	repoFilter := repository.MetricStreamFilter{
		CommonBaseFilters: repository.CommonBaseFilters{Schema: params.Schema},
		MetricName:        params.MetricName,
	}
	repoFilter.ServiceName = params.ServiceName
	return repoFilter, nil
}

// GaugeMetricStreamingHandler implements StreamingHandler for gauge metrics
type GaugeMetricStreamingHandler struct {
	repo repository.GaugeMetricStreamRepository
}

func (h *GaugeMetricStreamingHandler) Subscribe(ctx context.Context, filter any) (<-chan repository.GaugeMetricEntry, func(), <-chan struct{}, error) {
	repoFilter, err := authorizedMetricStreamFilter(ctx, filter)
	if err != nil {
		return nil, nil, nil, err
	}
	return h.repo.SubscribeGaugeMetrics(ctx, repoFilter)
}

func (h *GaugeMetricStreamingHandler) Match(entry repository.GaugeMetricEntry, params any) bool {
	if p, ok := params.(MetricStreamFilter); ok {
		return matchGaugeMetricFilter(entry, metricFilterParams{
			ServiceName: p.ServiceName,
			MetricName:  p.MetricName,
		})
	}
	return true
}

// SumMetricStreamingHandler implements StreamingHandler for sum metrics
type SumMetricStreamingHandler struct {
	repo repository.SumMetricStreamRepository
}

func (h *SumMetricStreamingHandler) Subscribe(ctx context.Context, filter any) (<-chan repository.SumMetricEntry, func(), <-chan struct{}, error) {
	repoFilter, err := authorizedMetricStreamFilter(ctx, filter)
	if err != nil {
		return nil, nil, nil, err
	}
	return h.repo.SubscribeSumMetrics(ctx, repoFilter)
}

func (h *SumMetricStreamingHandler) Match(entry repository.SumMetricEntry, params any) bool {
	if p, ok := params.(MetricStreamFilter); ok {
		return matchSumMetricFilter(entry, metricFilterParams{
			ServiceName: p.ServiceName,
			MetricName:  p.MetricName,
		})
	}
	return true
}

// HistogramMetricStreamingHandler implements StreamingHandler for histogram metrics
type HistogramMetricStreamingHandler struct {
	repo repository.HistogramMetricStreamRepository
}

func (h *HistogramMetricStreamingHandler) Subscribe(ctx context.Context, filter any) (<-chan repository.HistogramMetricEntry, func(), <-chan struct{}, error) {
	repoFilter, err := authorizedMetricStreamFilter(ctx, filter)
	if err != nil {
		return nil, nil, nil, err
	}
	return h.repo.SubscribeHistogramMetrics(ctx, repoFilter)
}

func (h *HistogramMetricStreamingHandler) Match(entry repository.HistogramMetricEntry, params any) bool {
	if p, ok := params.(MetricStreamFilter); ok {
		return matchHistogramMetricFilter(entry, metricFilterParams{
			ServiceName: p.ServiceName,
			MetricName:  p.MetricName,
		})
	}
	return true
}

// SummaryMetricStreamingHandler implements StreamingHandler for summary metrics
type SummaryMetricStreamingHandler struct {
	repo repository.SummaryMetricStreamRepository
}

func (h *SummaryMetricStreamingHandler) Subscribe(ctx context.Context, filter any) (<-chan repository.SummaryMetricEntry, func(), <-chan struct{}, error) {
	repoFilter, err := authorizedMetricStreamFilter(ctx, filter)
	if err != nil {
		return nil, nil, nil, err
	}
	return h.repo.SubscribeSummaryMetrics(ctx, repoFilter)
}

func (h *SummaryMetricStreamingHandler) Match(entry repository.SummaryMetricEntry, params any) bool {
	if p, ok := params.(MetricStreamFilter); ok {
		return matchSummaryMetricFilter(entry, metricFilterParams{
			ServiceName: p.ServiceName,
			MetricName:  p.MetricName,
		})
	}
	return true
}

// ExponentialHistogramMetricStreamingHandler implements StreamingHandler for exponential histogram metrics
type ExponentialHistogramMetricStreamingHandler struct {
	repo repository.ExponentialHistogramMetricStreamRepository
}

func (h *ExponentialHistogramMetricStreamingHandler) Subscribe(ctx context.Context, filter any) (<-chan repository.ExponentialHistogramMetricEntry, func(), <-chan struct{}, error) {
	repoFilter, err := authorizedMetricStreamFilter(ctx, filter)
	if err != nil {
		return nil, nil, nil, err
	}
	return h.repo.SubscribeExponentialHistogramMetrics(ctx, repoFilter)
}

func (h *ExponentialHistogramMetricStreamingHandler) Match(entry repository.ExponentialHistogramMetricEntry, params any) bool {
	if p, ok := params.(MetricStreamFilter); ok {
		return matchExponentialHistogramMetricFilter(entry, metricFilterParams{
			ServiceName: p.ServiceName,
			MetricName:  p.MetricName,
		})
	}
	return true
}

// CreateMetricStreamHandler creates a generic metric streaming handler
func CreateMetricStreamHandler[T any](handler StreamingHandler[T]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		// Parse filter parameters
		params := MetricStreamFilter{
			Schema:      schema,
			ServiceName: r.URL.Query().Get("service"),
			MetricName:  r.URL.Query().Get("metric"),
		}

		// Upgrade to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			http.Error(w, "websocket upgrade failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer conn.Close()

		// Configure streaming
		config := DefaultStreamingConfig()
		config.WaitForEstablished = parseBool(r.URL.Query().Get("wait_for_established"))
		config.EnablePreload = false // Metrics don't support preload yet

		// Handle streaming
		if err := HandleWebSocketStream(handler, nil, params, conn, r, config); err != nil {
			// Error already logged in HandleWebSocketStream
			return
		}
	}
}

// Consolidated metric streaming handlers using the new infrastructure
func StreamFilteredGaugeMetricsWSConsolidated(repo repository.GaugeMetricStreamRepository) http.HandlerFunc {
	handler := &GaugeMetricStreamingHandler{repo: repo}
	return CreateMetricStreamHandler[repository.GaugeMetricEntry](handler)
}

func StreamFilteredSumMetricsWSConsolidated(repo repository.SumMetricStreamRepository) http.HandlerFunc {
	handler := &SumMetricStreamingHandler{repo: repo}
	return CreateMetricStreamHandler[repository.SumMetricEntry](handler)
}

func StreamFilteredHistogramMetricsWSConsolidated(repo repository.HistogramMetricStreamRepository) http.HandlerFunc {
	handler := &HistogramMetricStreamingHandler{repo: repo}
	return CreateMetricStreamHandler[repository.HistogramMetricEntry](handler)
}

func StreamFilteredSummaryMetricsWSConsolidated(repo repository.SummaryMetricStreamRepository) http.HandlerFunc {
	handler := &SummaryMetricStreamingHandler{repo: repo}
	return CreateMetricStreamHandler[repository.SummaryMetricEntry](handler)
}

func StreamFilteredExponentialHistogramMetricsWSConsolidated(repo repository.ExponentialHistogramMetricStreamRepository) http.HandlerFunc {
	handler := &ExponentialHistogramMetricStreamingHandler{repo: repo}
	return CreateMetricStreamHandler[repository.ExponentialHistogramMetricEntry](handler)
}
