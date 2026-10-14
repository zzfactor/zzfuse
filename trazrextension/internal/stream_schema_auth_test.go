package internal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

var errStopSubscription = errors.New("stop after recording subscription")

type schemaStreamProbe struct {
	schemas chan string
}

func (p *schemaStreamProbe) record(schema string) {
	p.schemas <- schema
}

func (p *schemaStreamProbe) SubscribeLogs(_ context.Context, filter repository.LogFilter) (<-chan repository.LogEntryResponse, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}
func (p *schemaStreamProbe) SubscribeSpans(_ context.Context, filter repository.TraceFilter) (<-chan repository.TraceEntry, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}
func (p *schemaStreamProbe) SubscribeRuns(_ context.Context, filter repository.RunFilter) (<-chan repository.RunEntryResponse, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}
func (p *schemaStreamProbe) SubscribeGaugeMetrics(_ context.Context, filter repository.MetricStreamFilter) (<-chan repository.GaugeMetricEntry, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}
func (p *schemaStreamProbe) SubscribeSumMetrics(_ context.Context, filter repository.MetricStreamFilter) (<-chan repository.SumMetricEntry, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}
func (p *schemaStreamProbe) SubscribeHistogramMetrics(_ context.Context, filter repository.MetricStreamFilter) (<-chan repository.HistogramMetricEntry, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}
func (p *schemaStreamProbe) SubscribeSummaryMetrics(_ context.Context, filter repository.MetricStreamFilter) (<-chan repository.SummaryMetricEntry, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}
func (p *schemaStreamProbe) SubscribeExponentialHistogramMetrics(_ context.Context, filter repository.MetricStreamFilter) (<-chan repository.ExponentialHistogramMetricEntry, func(), <-chan struct{}, error) {
	p.record(filter.Schema)
	return nil, nil, nil, errStopSubscription
}

// The mock repository records the actual subscription filter, including the schema
// consumed by repository event handlers. It stops before a long-lived stream starts.
func TestSignalStreamSchemaSubscriptions(t *testing.T) {
	cases := []struct {
		name string
		run  func(*schemaStreamProbe, *schemaProbe, *websocket.Conn, *http.Request)
	}{
		{"logs", func(p *schemaStreamProbe, q *schemaProbe, conn *websocket.Conn, r *http.Request) {
			StreamLogs(p, &schemaLogService{probe: q}, conn, r)
		}},
		{"spans", func(p *schemaStreamProbe, q *schemaProbe, conn *websocket.Conn, r *http.Request) {
			StreamSpans(p, &schemaTraceService{probe: q}, conn, r)
		}},
		{"runs", func(p *schemaStreamProbe, q *schemaProbe, conn *websocket.Conn, r *http.Request) {
			StreamRunsWSWithPreload(p, &schemaRunService{probe: q}, conn, r)
		}},
	}
	for _, kind := range []string{"gauge", "sum", "histogram", "summary", "exponential_histogram"} {
		cases = append(cases, struct {
			name string
			run  func(*schemaStreamProbe, *schemaProbe, *websocket.Conn, *http.Request)
		}{"metrics " + kind, func(p *schemaStreamProbe, q *schemaProbe, conn *websocket.Conn, r *http.Request) {
			values := r.URL.Query()
			values.Set("type", kind)
			r.URL.RawQuery = values.Encode()
			StreamMetrics(p, p, p, p, p, &schemaMetricService{probe: q}, conn, r)
		}})
	}
	for _, tc := range cases {
		t.Run(tc.name+" denied", func(t *testing.T) {
			p := &schemaStreamProbe{schemas: make(chan string, 1)}
			q := &schemaProbe{}
			req := httptest.NewRequest("GET", "/?schema=tenant_b", nil)
			req = req.WithContext(auth.WithUserContext(req.Context(), &auth.UserContext{Schemas: []string{"tenant_a"}}))
			tc.run(p, q, nil, req)
			if len(p.schemas) != 0 || q.calls != 0 {
				t.Fatalf("denied stream subscribed or preloaded: subscriptions %d, queries %d", len(p.schemas), q.calls)
			}
		})
		t.Run(tc.name+" allowed", func(t *testing.T) {
			p := &schemaStreamProbe{schemas: make(chan string, 1)}
			q := &schemaProbe{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r = r.WithContext(auth.WithUserContext(r.Context(), &auth.UserContext{Schemas: []string{"tenant_a"}}))
				conn, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				tc.run(p, q, conn, r)
			}))
			defer server.Close()
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/?schema=tenant_a", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			select {
			case schema := <-p.schemas:
				if schema != "tenant_a" {
					t.Fatalf("subscription schema = %q, want tenant_a", schema)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("subscription was not established")
			}
		})
	}
}

func TestLegacyMetricStreamHandlersDenySchemaBeforeSubscribe(t *testing.T) {
	p := &schemaStreamProbe{schemas: make(chan string, 10)}
	handlers := []http.HandlerFunc{
		StreamFilteredGaugeMetricsWS(p),
		StreamFilteredSumMetricsWS(p),
		StreamFilteredHistogramMetricsWS(p),
		StreamFilteredSummaryMetricsWS(p),
		StreamFilteredExponentialHistogramMetricsWS(p),
		StreamFilteredGaugeMetricsWSConsolidated(p),
		StreamFilteredSumMetricsWSConsolidated(p),
		StreamFilteredHistogramMetricsWSConsolidated(p),
		StreamFilteredSummaryMetricsWSConsolidated(p),
		StreamFilteredExponentialHistogramMetricsWSConsolidated(p),
	}
	for _, handler := range handlers {
		req := httptest.NewRequest("GET", "/?schema=tenant_b", nil)
		req = req.WithContext(auth.WithUserContext(req.Context(), &auth.UserContext{Schemas: []string{"tenant_a"}}))
		rr := httptest.NewRecorder()
		handler(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rr.Code)
		}
	}
	if len(p.schemas) != 0 {
		t.Fatal("denied metric request established a subscription")
	}
}
