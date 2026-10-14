package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

type schemaProbe struct {
	calls  int
	schema string
}

func (p *schemaProbe) record(schema string) {
	p.calls++
	p.schema = schema
}

type schemaLogService struct {
	repository.LogQueryService
	repository.LogWriterService
	probe *schemaProbe
}

func (s *schemaLogService) GetLogByID(_ context.Context, id int64, schema string) (repository.LogEntryResponse, error) {
	s.probe.record(schema)
	return repository.LogEntryResponse{ID: id}, nil
}
func (s *schemaLogService) GenerateCursor(_ context.Context, _ int, q repository.LogQuery) (repository.LogQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.LogQueryResult{NextCursor: "log-cursor"}, nil
}
func (s *schemaLogService) QueryLogs(_ context.Context, q repository.LogQuery) (repository.LogQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.LogQueryResult{}, nil
}
func (s *schemaLogService) InsertLogs(_ context.Context, _ []repository.LogEntry, schema string) ([]string, error) {
	s.probe.record(schema)
	return []string{"42"}, nil
}

type schemaTraceService struct {
	repository.TraceQueryService
	repository.TraceWriterService
	probe *schemaProbe
}

func (s *schemaTraceService) GetSpanByID(_ context.Context, id int64, schema string) (repository.TraceEntryResponse, error) {
	s.probe.record(schema)
	return repository.TraceEntryResponse{SpanID: "span"}, nil
}
func (s *schemaTraceService) GenerateCursor(_ context.Context, _ int, q repository.TraceQuery) (repository.TraceQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.TraceQueryResult{NextCursor: "trace-cursor"}, nil
}
func (s *schemaTraceService) QuerySpans(_ context.Context, q repository.TraceQuery) (repository.TraceQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.TraceQueryResult{}, nil
}
func (s *schemaTraceService) InsertSpans(_ context.Context, _ []repository.TraceEntry, schema string) ([]string, error) {
	s.probe.record(schema)
	return []string{"42"}, nil
}

type schemaMetricService struct {
	repository.MetricQueryService
	repository.MetricWriterService
	probe *schemaProbe
}

func (s *schemaMetricService) GetMetricByID(_ context.Context, id int64, schema string) (repository.MetricEntryResponse, error) {
	s.probe.record(schema)
	return repository.MetricEntryResponse{ID: id}, nil
}
func (s *schemaMetricService) GetMetricByIDTyped(ctx context.Context, _ string, id int64, schema string) (repository.MetricEntryResponse, error) {
	return s.GetMetricByID(ctx, id, schema)
}
func (s *schemaMetricService) GenerateCursor(_ context.Context, _ int, q repository.MetricQuery) (repository.MetricQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.MetricQueryResult{NextCursor: "metric-cursor"}, nil
}
func (s *schemaMetricService) QueryMetricsGauge(_ context.Context, q repository.MetricQuery) (repository.MetricQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.MetricQueryResult{}, nil
}
func (s *schemaMetricService) QueryMetricsSum(ctx context.Context, q repository.MetricQuery) (repository.MetricQueryResult, error) {
	return s.QueryMetricsGauge(ctx, q)
}
func (s *schemaMetricService) QueryMetricsHistogram(ctx context.Context, q repository.MetricQuery) (repository.MetricQueryResult, error) {
	return s.QueryMetricsGauge(ctx, q)
}
func (s *schemaMetricService) QueryMetricsSummary(ctx context.Context, q repository.MetricQuery) (repository.MetricQueryResult, error) {
	return s.QueryMetricsGauge(ctx, q)
}
func (s *schemaMetricService) QueryMetricsExponentialHistogram(ctx context.Context, q repository.MetricQuery) (repository.MetricQueryResult, error) {
	return s.QueryMetricsGauge(ctx, q)
}

func (s *schemaMetricService) InsertGaugeMetrics(_ context.Context, _ []repository.GaugeMetricEntry, schema string) ([]string, error) {
	s.probe.record(schema)
	return []string{"42"}, nil
}

type schemaRunService struct {
	repository.RunQueryService
	repository.RunWriterService
	probe *schemaProbe
}

func (s *schemaRunService) GetRunByID(_ context.Context, id int64, schema string) (repository.RunEntryResponse, error) {
	s.probe.record(schema)
	return repository.RunEntryResponse{ID: id}, nil
}
func (s *schemaRunService) QueryRuns(_ context.Context, q repository.RunQuery) (repository.RunQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.RunQueryResult{}, nil
}
func (s *schemaRunService) InsertRuns(_ context.Context, _ []repository.RunEntry, schema string) ([]int64, error) {
	s.probe.record(schema)
	return []int64{42}, nil
}
func (s *schemaRunService) UpdateRunMetadata(_ context.Context, _ int64, _ map[string]interface{}, schema string) error {
	s.probe.record(schema)
	return nil
}
func (s *schemaRunService) EndRun(_ context.Context, _ string, _ int64, schema string) error {
	s.probe.record(schema)
	return nil
}

type schemaProfileService struct {
	repository.ProfileQueryService
	repository.ProfileWriterService
	probe *schemaProbe
}

func (s *schemaProfileService) QueryProfiles(_ context.Context, q repository.ProfileQuery) (repository.ProfileQueryResult, error) {
	s.probe.record(q.Schema)
	return repository.ProfileQueryResult{}, nil
}
func (s *schemaProfileService) InsertProfiles(_ context.Context, _ []repository.ProfileEntry, schema string) ([]string, error) {
	s.probe.record(schema)
	return []string{"42"}, nil
}

func TestSignalHandlersSchemaAuthorization(t *testing.T) {
	t.Parallel()
	endpoints := []struct {
		name, method, path, body string
		handler                  func(*schemaProbe) http.HandlerFunc
	}{
		{"log by ID", "GET", "/api/v1/logs/42?", "", func(p *schemaProbe) http.HandlerFunc { return NewLogGetByIDHandler(&schemaLogService{probe: p}) }},
		{"span by ID", "GET", "/api/v1/spans/42?", "", func(p *schemaProbe) http.HandlerFunc { return NewSpanGetByIDHandler(&schemaTraceService{probe: p}) }},
		{"run by ID", "GET", "/api/v1/runs/42?", "", func(p *schemaProbe) http.HandlerFunc { return NewRunGetByIDHandler(&schemaRunService{probe: p}) }},
		{"metric by ID", "GET", "/api/v1/metrics/42?", "", func(p *schemaProbe) http.HandlerFunc { return NewMetricGetByIDHandler(&schemaMetricService{probe: p}) }},
		{"typed metric by ID", "GET", "/api/v1/metrics/gauge/42?", "", func(p *schemaProbe) http.HandlerFunc { return NewMetricGetByIDHandler(&schemaMetricService{probe: p}) }},
		{"log cursor", "GET", "/api/v1/logs/cursor?offset=1&", "", func(p *schemaProbe) http.HandlerFunc {
			return NewLogCursorGenerationHandler(&schemaLogService{probe: p})
		}},
		{"trace cursor", "GET", "/api/v1/traces/cursor?offset=1&", "", func(p *schemaProbe) http.HandlerFunc {
			return NewTraceCursorGenerationHandler(&schemaTraceService{probe: p})
		}},
		{"metric cursor", "GET", "/api/v1/metrics/cursor?offset=1&", "", func(p *schemaProbe) http.HandlerFunc {
			return NewMetricCursorGenerationHandler(&schemaMetricService{probe: p})
		}},
		{"metric query", "GET", "/api/v1/metrics?type=gauge&", "", func(p *schemaProbe) http.HandlerFunc { return NewMetricQueryV1Handler(&schemaMetricService{probe: p}) }},
		{"run query", "GET", "/api/v1/runs?", "", func(p *schemaProbe) http.HandlerFunc { return NewRunQueryV1Handler(&schemaRunService{probe: p}) }},
		{"profile query", "GET", "/api/v1/profiles?", "", func(p *schemaProbe) http.HandlerFunc { return NewProfileQueryHandler(&schemaProfileService{probe: p}) }},
		{"metric insert", "POST", "/api/v1/metrics?type=gauge&", `{"timestamp":"2026-01-01T00:00:00Z","service_name":"test"}`, func(p *schemaProbe) http.HandlerFunc { return NewMetricInsertHandler(&schemaMetricService{probe: p}) }},
		{"run insert", "POST", "/api/v1/runs?", `{"name":"test","service_name":"test","start_timestamp_ns":1}`, func(p *schemaProbe) http.HandlerFunc { return NewRunInsertHandler(&schemaRunService{probe: p}) }},
		{"run update", "PUT", "/api/v1/runs?", `{"id":42,"metadata":{}}`, func(p *schemaProbe) http.HandlerFunc { return NewRunUpdateHandler(&schemaRunService{probe: p}) }},
		{"run end", "DELETE", "/api/v1/runs?", `{"name":"test","end_timestamp_ns":2}`, func(p *schemaProbe) http.HandlerFunc { return NewRunEndHandler(&schemaRunService{probe: p}) }},
		{"profile insert", "POST", "/api/v1/profiles?", `{"timestamp":"2026-01-01T00:00:00Z","service_name":"test"}`, func(p *schemaProbe) http.HandlerFunc { return NewProfileInsertHandler(&schemaProfileService{probe: p}) }},
	}
	users := []struct {
		name, schema string
		user         *auth.UserContext
		status       int
	}{
		{"allowed schema", "tenant_a", &auth.UserContext{Schemas: []string{"tenant_a"}}, http.StatusOK},
		{"denied schema", "tenant_b", &auth.UserContext{Schemas: []string{"tenant_a"}}, http.StatusForbidden},
		{"default denied", "", &auth.UserContext{Schemas: []string{"tenant_a"}}, http.StatusForbidden},
		{"default allowed", "", &auth.UserContext{Schemas: []string{DefaultSchema}}, http.StatusOK},
		{"admin", "tenant_b", &auth.UserContext{Schemas: []string{"tenant_a"}, Roles: []string{"admin"}}, http.StatusOK},
		{"empty grants deny", "tenant_b", &auth.UserContext{}, http.StatusForbidden},
		{"missing user", "tenant_a", nil, http.StatusUnauthorized},
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			t.Parallel()
			for _, user := range users {
				t.Run(user.name, func(t *testing.T) {
					probe := &schemaProbe{}
					req := httptest.NewRequest(endpoint.method, endpoint.path+"schema="+user.schema, strings.NewReader(endpoint.body))
					if user.user != nil {
						req = req.WithContext(auth.WithUserContext(req.Context(), user.user))
					}
					rr := httptest.NewRecorder()
					endpoint.handler(probe)(rr, req)
					if rr.Code != user.status {
						t.Fatalf("status = %d, want %d; body = %s", rr.Code, user.status, rr.Body.String())
					}
					if user.status != http.StatusOK {
						if probe.calls != 0 {
							t.Fatalf("denied request reached service %d times", probe.calls)
						}
						var response ErrorResponse
						if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil || response.Error == "" {
							t.Fatalf("expected JSON error response: %s", rr.Body.String())
						}
						return
					}
					wantSchema := user.schema
					if wantSchema == "" {
						wantSchema = DefaultSchema
					}
					if probe.calls != 1 || probe.schema != wantSchema {
						t.Fatalf("service calls = %d, schema = %q; want 1, %q", probe.calls, probe.schema, wantSchema)
					}
				})
			}
		})
	}
}

func TestSignalPreloadRejectsUnauthorizedSchema(t *testing.T) {
	t.Parallel()
	params := &CommonQueryParams{Schema: "tenant_b", Cursor: "cursor"}
	ctx := auth.WithUserContext(context.Background(), &auth.UserContext{Schemas: []string{"tenant_a"}})
	probe := &schemaProbe{}
	executors := []PreloadExecutor[any]{
		NewLogsPreloadExecutor(&schemaLogService{probe: probe}),
		NewTracesPreloadExecutor(&schemaTraceService{probe: probe}),
		NewMetricsPreloadExecutor(&schemaMetricService{probe: probe}),
		NewRunsPreloadExecutor(&schemaRunService{probe: probe}),
	}
	for _, executor := range executors {
		if err := executor.ExecutePreload(ctx, params, nil); err == nil {
			t.Errorf("%T allowed unauthorized preload", executor)
		}
	}
	if probe.calls != 0 {
		t.Fatalf("unauthorized preload reached services %d times", probe.calls)
	}
}
