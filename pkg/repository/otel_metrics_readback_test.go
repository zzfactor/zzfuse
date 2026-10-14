package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"
)

type metricReadbackCase struct {
	kind     string
	entry    interface{}
	response func(interface{}) MetricEntryResponse
	expected map[string]interface{}
}

func metricReadbackCases(at time.Time) []metricReadbackCase {
	start := at.Add(-5 * time.Second)
	value, sum, min, max := 0.0, 0.0, 0.0, 2.5
	negative, count, scale, zeroCount, threshold, offset := -8.5, int64(3), int32(-2), int64(0), 0.25, int32(-1)
	temporality, monotonic := int32(2), false
	cases := []metricReadbackCase{
		{"gauge", GaugeMetricEntry{Value: &value}, func(e interface{}) MetricEntryResponse { return toGaugeMetricEntryResponse(e.(GaugeMetricEntry)) }, map[string]interface{}{"value": 0}},
		{"sum", SumMetricEntry{StartTime: &start, Value: &negative, AggregationTemporality: &temporality, IsMonotonic: &monotonic}, func(e interface{}) MetricEntryResponse { return toSumMetricEntryResponse(e.(SumMetricEntry)) }, map[string]interface{}{"start_time": start.Format(time.RFC3339Nano), "value": -8.5, "aggregation_temporality": 2, "is_monotonic": false}},
		{"histogram", HistogramMetricEntry{StartTime: &start, Count: &count, Sum: &sum, Min: &min, Max: &max, Buckets: []byte(`{"bucket_counts":[1,2],"explicit_bounds":[1.25]}`), AggregationTemporality: &temporality}, func(e interface{}) MetricEntryResponse {
			return toHistogramMetricEntryResponse(e.(HistogramMetricEntry))
		}, map[string]interface{}{"start_time": start.Format(time.RFC3339Nano), "count": 3, "sum": 0, "min": 0, "max": 2.5, "buckets": map[string]interface{}{"bucket_counts": []int{1, 2}, "explicit_bounds": []float64{1.25}}, "aggregation_temporality": 2}},
		{"summary", SummaryMetricEntry{StartTime: &start, Count: &count, Sum: &sum, Quantiles: []byte(`[{"quantile":0.5,"value":1.75}]`)}, func(e interface{}) MetricEntryResponse { return toSummaryMetricEntryResponse(e.(SummaryMetricEntry)) }, map[string]interface{}{"start_time": start.Format(time.RFC3339Nano), "count": 3, "sum": 0, "quantiles": []interface{}{map[string]interface{}{"quantile": 0.5, "value": 1.75}}}},
		{"exponential_histogram", ExponentialHistogramMetricEntry{StartTime: &start, Count: &count, Sum: &sum, Min: &min, Max: &max, Scale: &scale, ZeroCount: &zeroCount, ZeroThreshold: &threshold, PositiveOffset: &offset, PositiveBucketCounts: []byte(`[1,2]`), NegativeOffset: &offset, NegativeBucketCounts: []byte(`[0,1]`), AggregationTemporality: &temporality}, func(e interface{}) MetricEntryResponse {
			return toExponentialHistogramMetricEntryResponse(e.(ExponentialHistogramMetricEntry))
		}, map[string]interface{}{"start_time": start.Format(time.RFC3339Nano), "count": 3, "sum": 0, "min": 0, "max": 2.5, "scale": -2, "zero_count": 0, "zero_threshold": 0.25, "positive_offset": -1, "positive_bucket_counts": []int{1, 2}, "negative_offset": -1, "negative_bucket_counts": []int{0, 1}, "aggregation_temporality": 2}},
	}
	namespace, instance, name, description, unit := "readback-team", "instance-42", "metric.measurement", "a measured value", "ms"
	scopeName, scopeVersion, resourceSchema, scopeSchema := "readback.scope", "1.2.3", "https://example.test/resource-schema", "https://example.test/scope-schema"
	flags, dropped := int32(0), int32(3)
	common := map[string]interface{}{
		"TimestampNS": at.UnixNano(), "Timestamp": at, "ServiceName": "readback-service", "ServiceNamespace": &namespace, "ServiceInstanceID": &instance,
		"Name": &name, "Description": &description, "Unit": &unit, "Attributes": []byte(`{"point":"attribute"}`), "Flags": &flags, "Exemplars": []byte(`[{"value":0,"trace_id":"1234"}]`),
		"ResourceAttributes": []byte(`{"resource":"attribute"}`), "ScopeName": &scopeName, "ScopeVersion": &scopeVersion, "ScopeAttributes": []byte(`{"scope":"attribute"}`), "ScopeDroppedAttrCount": &dropped,
		"Metadata": []byte(`{"test":"readback"}`), "ResourceSchemaURL": &resourceSchema, "ScopeSchemaURL": &scopeSchema,
	}
	expected := map[string]interface{}{
		"id": 0, "timestamp_ns": at.UnixNano(), "timestamp": at.Format(time.RFC3339Nano), "service_name": "readback-service", "service_namespace": namespace, "service_instance_id": instance,
		"name": name, "description": description, "unit": unit, "attributes": map[string]string{"point": "attribute"}, "flags": 0, "exemplars": []interface{}{map[string]interface{}{"value": 0, "trace_id": "1234"}},
		"resource_attributes": map[string]string{"resource": "attribute"}, "scope_name": scopeName, "scope_version": scopeVersion, "scope_attributes": map[string]string{"scope": "attribute"}, "scope_dropped_attr_count": 3,
		"metadata": map[string]string{"test": "readback"}, "resource_schema_url": resourceSchema, "scope_schema_url": scopeSchema,
	}
	for i := range cases {
		// Populate the common context independently of the production mapping code.
		entry := reflect.New(reflect.TypeOf(cases[i].entry)).Elem()
		entry.Set(reflect.ValueOf(cases[i].entry))
		for field, value := range common {
			entry.FieldByName(field).Set(reflect.ValueOf(value))
		}
		cases[i].entry = entry.Interface()
		for field, value := range expected {
			cases[i].expected[field] = value
		}
	}
	return cases
}

func metricJSON(t *testing.T, value interface{}) interface{} {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var parsed interface{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&parsed); err != nil {
		t.Fatal(err)
	}
	return parsed
}

type legacyMetricReadback struct {
	MetricRepository
	entry interface{}
	typ   string
	calls int
}

func (r *legacyMetricReadback) GetMetricByID(context.Context, int64, string) (interface{}, string, error) {
	r.calls++
	return r.entry, r.typ, nil
}

func TestTypedMetricReadbackLegacyClientFallback(t *testing.T) {
	fixture := metricReadbackCases(time.Now().UTC())[0]
	repo := &legacyMetricReadback{entry: fixture.entry, typ: "metrics_gauge"}
	service := NewMetricQueryService(repo)
	response, err := service.GetMetricByIDTyped(context.Background(), "gauge", 1, "test_schema")
	if err != nil || repo.calls != 1 {
		t.Fatal("legacy client fallback failed", err)
	}
	assertMetricResponse(t, response, fixture.expected)
	if _, err := service.GetMetricByIDTyped(context.Background(), "sum", 1, "test_schema"); err == nil {
		t.Fatal("legacy client returned the wrong metric type")
	}
	before := repo.calls
	if _, err := service.GetMetricByIDTyped(context.Background(), "gauge;DROP TABLE", 1, "test_schema"); err == nil || repo.calls != before {
		t.Fatal("unsupported metric type reached repository")
	}
	if _, _, err := (&metricRepository{}).GetMetricByIDTyped(context.Background(), "unknown", 1, "test_schema"); err == nil {
		t.Fatal("unsupported metric type reached database")
	}
}

func assertMetricResponse(t *testing.T, response MetricEntryResponse, expected map[string]interface{}) {
	t.Helper()
	actual, want := metricJSON(t, response), metricJSON(t, expected)
	// PostgreSQL may return a session time zone offset. Compare the exact instant,
	// preserving fractional precision, rather than requiring an equivalent Z form.
	for _, parsed := range []interface{}{actual, want} {
		fields := parsed.(map[string]interface{})
		for _, key := range []string{"timestamp", "start_time"} {
			if value, ok := fields[key].(string); ok {
				instant, err := time.Parse(time.RFC3339Nano, value)
				if err != nil {
					t.Fatal(err)
				}
				fields[key] = instant.UTC().Format(time.RFC3339Nano)
			}
		}
	}
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("metric JSON lost measurements or context:\nactual=%#v\nexpected=%#v", actual, want)
	}
}

func TestMetricResponsesRetainMeasurementsAndContext(t *testing.T) {
	for _, tc := range metricReadbackCases(time.Date(2026, 10, 14, 12, 0, 0, 123000000, time.UTC)) {
		t.Run(tc.kind, func(t *testing.T) { assertMetricResponse(t, tc.response(tc.entry), tc.expected) })
	}
}

func TestMetricResponsesOmitAbsentMeasurements(t *testing.T) {
	responses := []MetricEntryResponse{toGaugeMetricEntryResponse(GaugeMetricEntry{}), toSumMetricEntryResponse(SumMetricEntry{}), toHistogramMetricEntryResponse(HistogramMetricEntry{}), toSummaryMetricEntryResponse(SummaryMetricEntry{}), toExponentialHistogramMetricEntryResponse(ExponentialHistogramMetricEntry{})}
	for _, response := range responses {
		actual := metricJSON(t, response).(map[string]interface{})
		for _, field := range []string{"value", "count", "sum", "start_time", "buckets", "quantiles", "min", "max", "scale", "zero_count", "zero_threshold", "positive_offset", "positive_bucket_counts", "negative_offset", "negative_bucket_counts", "aggregation_temporality", "is_monotonic", "flags", "exemplars"} {
			if _, ok := actual[field]; ok {
				t.Fatalf("absent measurement %q appears in response", field)
			}
		}
	}
}

func TestMetricReadColumnsMatchProvisionedDDL(t *testing.T) {
	columnSets := map[string]MetricColumnSet{"gauge": gaugeColumnSet, "sum": sumColumnSet, "histogram": histogramColumnSet, "summary": summaryColumnSet, "exponential_histogram": expHistogramColumnSet}
	columnPattern := regexp.MustCompile(`(?m)^\s+([a-z_]+)\s+(?:BIGSERIAL|BIGINT|TIMESTAMP|TEXT|JSONB|DOUBLE|INTEGER|BOOLEAN)\b`)
	for kind, set := range columnSets {
		t.Run(kind, func(t *testing.T) {
			data, err := os.ReadFile("internal/database/ddl/" + kind + "_metrics.sql")
			if err != nil {
				t.Fatal(err)
			}
			columns := map[string]bool{}
			for _, m := range columnPattern.FindAllStringSubmatch(string(data), -1) {
				columns[m[1]] = true
			}
			seen := map[string]bool{}
			for _, column := range set.SelectColumns {
				if !columns[column] || seen[column] {
					t.Fatalf("invalid or repeated selected column %q", column)
				}
				seen[column] = true
			}
			if len(columns) != len(seen) {
				t.Fatalf("read drops provisioned columns: selected=%v ddl=%v", seen, columns)
			}
			for _, column := range set.InsertColumns {
				if column == "id" || column == "timestamp" || !columns[column] {
					t.Fatalf("invalid writable column %q", column)
				}
			}
		})
	}
}
