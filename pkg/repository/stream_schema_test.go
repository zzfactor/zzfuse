package repository

import (
	"context"
	"testing"

	"github.com/medxops/trazr-otel/pkg/event"
)

func assertEventSchemaIsolation[T any](t *testing.T, handler event.EventHandler, messages <-chan T, payload any) {
	t.Helper()
	for _, schema := range []string{"tenant_b", ""} {
		if err := handler.HandleEvent(context.Background(), event.Event{Schema: schema, Data: payload}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-messages:
			t.Fatalf("event from schema %q crossed the tenant boundary", schema)
		default:
		}
	}
	if err := handler.HandleEvent(context.Background(), event.Event{Schema: "tenant_a", Data: payload}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-messages:
	default:
		t.Fatal("event from authorized schema was not delivered")
	}
}

func TestStreamEventSchemaIsolation(t *testing.T) {
	t.Parallel()
	base := CommonBaseFilters{Schema: "tenant_a"}
	done := make(chan struct{})
	t.Run("logs", func(t *testing.T) {
		messages := make(chan LogEntryResponse, 1)
		handler := &logEventHandler{filter: LogFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []LogEntryResponse{{ID: 42}})
	})
	t.Run("spans", func(t *testing.T) {
		messages := make(chan TraceEntry, 1)
		handler := &traceEventHandler{filter: TraceFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []TraceEntry{{SpanID: "span"}})
	})
	t.Run("runs", func(t *testing.T) {
		messages := make(chan RunEntryResponse, 1)
		handler := &runEventHandler{filter: RunFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []RunEntryResponse{{ID: 42}})
	})
	t.Run("gauge", func(t *testing.T) {
		messages := make(chan GaugeMetricEntry, 1)
		handler := &gaugeMetricEventHandler{filter: MetricStreamFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []GaugeMetricEntry{{ID: 42}})
	})
	t.Run("sum", func(t *testing.T) {
		messages := make(chan SumMetricEntry, 1)
		handler := &sumMetricEventHandler{filter: MetricStreamFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []SumMetricEntry{{ID: 42}})
	})
	t.Run("histogram", func(t *testing.T) {
		messages := make(chan HistogramMetricEntry, 1)
		handler := &histogramMetricEventHandler{filter: MetricStreamFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []HistogramMetricEntry{{ID: 42}})
	})
	t.Run("summary", func(t *testing.T) {
		messages := make(chan SummaryMetricEntry, 1)
		handler := &summaryMetricEventHandler{filter: MetricStreamFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []SummaryMetricEntry{{ID: 42}})
	})
	t.Run("exponential histogram", func(t *testing.T) {
		messages := make(chan ExponentialHistogramMetricEntry, 1)
		handler := &exponentialHistogramMetricEventHandler{filter: MetricStreamFilter{CommonBaseFilters: base}, msgCh: messages, done: done}
		assertEventSchemaIsolation(t, handler, messages, []ExponentialHistogramMetricEntry{{ID: 42}})
	})
}
