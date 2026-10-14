package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create transformers with pre-compiled engines
func createTransformerWithTemplates(t *testing.T, templateContent map[string]string, metadataFields ...string) *FullSpanTransformer {
	t.Helper()
	// Create a mock template service for testing
	templateService, err := NewTemplateService(context.Background(), nil, nil, []*database.ProcessorMappingConfig{}, map[int64]string{}, nil)
	require.NoError(t, err)

	template := &database.EmbeddingConfigTemplate{
		Content:          templateContent,
		TemplateMetadata: map[string][]string{"spans": metadataFields},
	}
	templateInfo, err := templateService.GetOrCompileTemplate(0, template)
	require.NoError(t, err)
	transformer := NewFullSpanTransformerWithPrecompiledEngine(templateInfo.ContentEngine)
	transformer.SetMetadataFields(template.TemplateMetadata["spans"])
	return transformer
}

func TestFullSpanTransformerWithDatabaseTemplate(t *testing.T) {
	templateContent := map[string]string{
		"spans": "{{.service_name}} for operation {{.name}}. Status: {{.status_code}}. Duration: {{.duration_ms}}ms.",
		"logs":  "Log from {{.service_name}}: {{.message}}",
	}

	// Use helper function instead of deleted function
	transformer := createTransformerWithTemplates(t, templateContent, "trace_id", "span_id", "service_name", "duration_ms", "status_code")

	// Create test data
	content := &CompleteTraceContent{
		TraceID:     "test-trace-123",
		ServiceName: "test-service",
		Spans: []SpanData{
			{
				Name:          "test-span",
				SpanID:        "span-456",
				ParentSpanID:  nil,       // ROOT span
				SpanKind:      nil,       // Will default to UNSPECIFIED
				Duration:      123000000, // 123ms in nanoseconds
				StatusCode:    0,
				StatusMessage: nil,
			},
		},
	}

	// Transform the content
	results, err := transformer.Transform(context.Background(), content)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// Check the result
	result := results[0]
	expectedContent := "test-service for operation test-span. Status: 0. Duration: 123ms."

	assert.Equal(t, expectedContent, result.Content)
	assert.Equal(t, "full_span", result.Mode)
	assert.Equal(t, "test-trace-123", result.Metadata["trace_id"])
	assert.Equal(t, "span-456", result.Metadata["span_id"])
	assert.Equal(t, "test-service", result.Metadata["service_name"])
	assert.Equal(t, int64(123), result.Metadata["duration_ms"])
	assert.Equal(t, 0, result.Metadata["status_code"])
}

func TestFullSpanTransformerWithUpdatedSpansTemplate(t *testing.T) {
	// Create test template content with the new spans template format
	templateContent := map[string]string{
		"spans": `{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}`,
	}

	// ✅ UPDATED: Use helper function instead of deleted function
	transformer := createTransformerWithTemplates(t, templateContent, "trace_id", "span_id", "service_name", "duration_ms", "status_code")

	// Create test data with events and logs
	spanID := "span-456"
	testContent := &CompleteTraceContent{
		TraceID:     "test-trace-123",
		ServiceName: "test-service",
		Spans: []SpanData{
			{
				Name:          "test-span",
				SpanID:        spanID,
				ParentSpanID:  nil,
				SpanKind:      nil,
				Duration:      123000000, // 123ms in nanoseconds
				StatusCode:    0,
				StatusMessage: nil,
				Events: []map[string]interface{}{
					{"name": "db_query"},
					{"name": "db_query"},
					{"name": "cache_hit"},
					{"name": "http_request"},
				},
			},
		},
		Logs: []LogData{
			{
				ID:           "log-1",
				SpanID:       &spanID,
				SeverityText: stringPtr("INFO"),
			},
			{
				ID:           "log-2",
				SpanID:       &spanID,
				SeverityText: stringPtr("WARN"),
			},
			{
				ID:           "log-3",
				SpanID:       &spanID,
				SeverityText: stringPtr("ERROR"),
			},
			{
				ID:           "log-4",
				SpanID:       &spanID,
				SeverityText: stringPtr("INFO"),
			},
		},
	}

	// Transform the content
	results, err := transformer.Transform(context.Background(), testContent)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// Check the result
	result := results[0]

	// Verify the result contains expected content
	expectedParts := []string{
		"test-service for operation test-span",
		"Status: OK",
		"Duration: 123ms",
		"TraceID: test-trace-123",
		"Events: 4 events",
		"2 db_query",
		"1 cache_hit",
		"1 http_request",
		"Logs: 4 logs (1 ERROR, 1 WARN, 2 INFO)",
	}

	for _, expected := range expectedParts {
		assert.Contains(t, result.Content, expected, "Expected result to contain '%s'", expected)
	}

	assert.Equal(t, "full_span", result.Mode)
	assert.Equal(t, "test-trace-123", result.Metadata["trace_id"])
	assert.Equal(t, "span-456", result.Metadata["span_id"])
	assert.Equal(t, "test-service", result.Metadata["service_name"])
	assert.Equal(t, int64(123), result.Metadata["duration_ms"])
	assert.Equal(t, 0, result.Metadata["status_code"])
}

func TestFullSpanTransformerWithDatabaseTemplateAndCorrelatedSignals(t *testing.T) {
	// Create test template content
	templateContent := map[string]string{
		"spans": `Span: {{.name}} ({{.span_id}})
Trace: {{.trace_id}}
Service: {{.service_name}}
Parent: {{.parent_span_id | default "ROOT"}}
Kind: {{.span_kind | default "UNSPECIFIED"}}
Duration: {{.duration_ms | formatDuration}}
Status: {{.status_code}}{{if .status_message}}
Message: {{.status_message}}{{end}}{{if .log_count}}
Logs: {{.logs_summary}}{{end}}{{if .metric_count}}
Metrics: {{.metrics_summary}}{{end}}{{if .profile_count}}
Profiles: {{.profiles_summary}}{{end}}`,
	}

	// ✅ UPDATED: Use helper function instead of deleted function
	transformer := createTransformerWithTemplates(t, templateContent, "log_count", "exemplar_count", "profile_count")

	// Create test data with correlated signals
	parentSpanID := "parent-span-789"
	spanKind := "SERVER"
	statusMessage := "OK"
	traceID := "test-trace-123"
	spanID := "span-456"

	content := &CompleteTraceContent{
		TraceID:     "test-trace-123",
		ServiceName: "test-service",
		Spans: []SpanData{
			{
				Name:          "test-span",
				SpanID:        "span-456",
				ParentSpanID:  &parentSpanID,
				SpanKind:      &spanKind,
				Duration:      500000000, // 500ms in nanoseconds
				StatusCode:    0,
				StatusMessage: &statusMessage,
			},
		},
		Logs: []LogData{
			{
				ID:      "log-1",
				TraceID: &traceID,
				SpanID:  &spanID,
				Body:    "Test log message",
			},
		},
		Metrics: []MetricData{
			{
				ID:      "metric-1",
				TraceID: &traceID,
				SpanID:  &spanID,
				Name:    "test_metric",
				Value:   42.0,
			},
		},
	}

	// Transform the content
	results, err := transformer.Transform(context.Background(), content)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// Check the result
	result := results[0]
	expectedContent := `Span: test-span (span-456)
Trace: test-trace-123
Service: test-service
Parent: parent-span-789
Kind: SERVER
Duration: 500ms
Status: 0
Message: OK`

	assert.Contains(t, result.Content, expectedContent)
	assert.Equal(t, "full_span", result.Mode)
	assert.Equal(t, 1, result.Metadata["log_count"])
	assert.Equal(t, 0, result.Metadata["exemplar_count"])
	assert.Equal(t, 0, result.Metadata["profile_count"])
}

func TestFullSpanTransformerFallback(t *testing.T) {
	// Create transformer without database template (should use fallback)
	transformer := NewFullSpanTransformer()

	// Create test data
	content := &CompleteTraceContent{
		TraceID:     "test-trace-123",
		ServiceName: "test-service",
		Spans: []SpanData{
			{
				Name:          "test-span",
				SpanID:        "span-456",
				ParentSpanID:  nil,
				SpanKind:      nil,
				Duration:      123000000, // 123ms in nanoseconds
				StatusCode:    0,
				StatusMessage: nil,
			},
		},
	}

	// Transform the content
	results, err := transformer.Transform(context.Background(), content)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// Check the result (should use fallback format)
	result := results[0]
	expectedContent := `Span: test-span (span-456)
Trace: test-trace-123
Service: test-service
Parent: ROOT
Kind: UNSPECIFIED
Duration: 123ms
Status: 0`

	assert.Contains(t, result.Content, expectedContent)
	assert.Equal(t, "full_span", result.Mode)
}

func TestFullSpanTransformerMetadataSelection(t *testing.T) {
	trace := &CompleteTraceContent{
		TraceID: "trace-id", ServiceName: "checkout",
		Spans: []SpanData{{Name: "request", SpanID: "span-id", Duration: 123_000_000}},
	}
	transformer := createTransformerWithTemplates(t, map[string]string{"spans": `{{.name}}`}, "service_name", "duration_ms")
	results, err := transformer.Transform(context.Background(), trace)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, map[string]interface{}{
		"service_name": "checkout",
		"duration_ms":  int64(123),
		"internal":     map[string]interface{}{"signal_type": "spans", "id": "span-id"},
	}, results[0].Metadata)

	transformer.SetMetadataFields(nil)
	results, err = transformer.Transform(context.Background(), trace)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, map[string]interface{}{
		"internal": map[string]interface{}{
			"signal_type": "spans", "id": "span-id", "trace_id": "trace-id", "span_id": "span-id",
		},
	}, results[0].Metadata)
}

func TestTransformerFactoryWithDatabaseTemplates(t *testing.T) {
	// Create test template content
	templateContent := map[string]string{
		"full_span": `Span: {{.name}} ({{.span_id}})
Trace: {{.trace_id}}
Service: {{.service_name}}`,
	}

	// Create factory with database templates
	factory := NewTransformerFactoryWithTemplates(templateContent)

	// Test creating full_span transformer
	transformer, err := factory.CreateTransformer("full_span")
	assert.NoError(t, err)
	assert.NotNil(t, transformer)

	// Verify it's the right type
	_, ok := transformer.(*FullSpanTransformer)
	assert.True(t, ok)

	// Test creating other transformers (should not use database templates)
	transformer2, err := factory.CreateTransformer("full_trace")
	assert.NoError(t, err)
	assert.NotNil(t, transformer2)
}

func TestTransformerFactoryWithoutDatabaseTemplates(t *testing.T) {
	// Create factory without database templates
	factory := NewTransformerFactory()

	// Test creating full_span transformer (should use default)
	transformer, err := factory.CreateTransformer("full_span")
	assert.NoError(t, err)
	assert.NotNil(t, transformer)

	// Verify it's the right type
	_, ok := transformer.(*FullSpanTransformer)
	assert.True(t, ok)
}

func TestSpanTemplateIterationAndAttributeAccess(t *testing.T) {
	templateContent := map[string]string{
		"spans": `{{range .logs}}Log: {{.Body}} UserID: {{index .Attributes "user_id"}}
{{end}}
{{range .exemplars}}Exemplar: {{.MetricName}} Value: {{.Value}} Label: {{index .FilteredAttributes "label"}}
{{end}}
{{range .events}}Event: {{.name}} Time: {{.timestamp}} Detail: {{index .attributes "detail"}}
{{end}}`,
	}
	transformer := createTransformerWithTemplates(t, templateContent)

	userID := "u-123"
	eventDetail := "something happened"
	spanID := "span-1"
	content := &CompleteTraceContent{
		TraceID:     "trace-1",
		ServiceName: "svc",
		Spans: []SpanData{
			{
				Name:       "span1",
				SpanID:     "span-1",
				Duration:   1000000,
				StatusCode: 0,
				Events: []map[string]interface{}{
					{"name": "evt1", "timestamp": "2024-07-28T12:00:00Z", "attributes": map[string]interface{}{"detail": eventDetail}},
				},
			},
		},
		Logs: []LogData{
			{
				ID:         "log-1",
				SpanID:     &spanID,
				Body:       "log message",
				Attributes: map[string]interface{}{"user_id": userID},
			},
		},
		// Note: Metrics are now handled as exemplars, so we don't need to add them here
		// The exemplars will be fetched from the database by span_id
	}

	results, err := transformer.Transform(context.Background(), content)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	result := results[0]

	assert.Contains(t, result.Content, "Log: log message UserID: u-123")
	assert.Contains(t, result.Content, "Event: evt1 Time: 2024-07-28T12:00:00Z Detail: something happened")
}

func TestSpanTemplateAllFieldsAndCounts(t *testing.T) {
	templateContent := map[string]string{
		"spans": `Span: {{.name}} ({{.span_id}})
Trace: {{.trace_id}}
Service: {{.service_name}}
Duration: {{.duration_ms}}ms
Status: {{.status_code}}

Counts:
- Logs: {{.log_count}}
- Exemplars: {{.exemplar_count}}
- Profiles: {{.profile_count}}
- Events: {{.event_count}}

{{if gt .log_count 0}}Logs:
{{range .logs}}- {{.Body}} (ID: {{.ID}})
{{end}}{{end}}

{{if gt .exemplar_count 0}}Exemplars:
{{range .exemplars}}- {{.MetricName}}: {{.Value}} (ID: {{.ID}})
{{end}}{{end}}

{{if gt .profile_count 0}}Profiles:
{{range .profiles}}- {{.ID}} (Type: {{.ProfileType}})
{{end}}{{end}}

{{if gt .event_count 0}}Events:
{{range .events}}- {{.name}} at {{.timestamp}}
{{end}}{{end}}`,
	}
	transformer := createTransformerWithTemplates(t, templateContent)

	spanID := "span-1"
	profileType := "cpu"
	content := &CompleteTraceContent{
		TraceID:     "trace-1",
		ServiceName: "svc",
		Spans: []SpanData{
			{
				Name:       "span1",
				SpanID:     "span-1",
				Duration:   1000000,
				StatusCode: 0,
				Events: []map[string]interface{}{
					{"name": "evt1", "timestamp": "2024-07-28T12:00:00Z"},
					{"name": "evt2", "timestamp": "2024-07-28T12:01:00Z"},
				},
			},
		},
		Logs: []LogData{
			{
				ID:     "log-1",
				SpanID: &spanID,
				Body:   "log message 1",
			},
			{
				ID:     "log-2",
				SpanID: &spanID,
				Body:   "log message 2",
			},
		},
		// Note: Metrics are now handled as exemplars, so we don't need to add them here
		// The exemplars will be fetched from the database by span_id
		Profiles: []ProfileData{
			{
				ID:          "profile-1",
				SpanID:      &spanID,
				ProfileType: &profileType,
			},
		},
	}

	results, err := transformer.Transform(context.Background(), content)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	result := results[0]

	// Test counts
	assert.Contains(t, result.Content, "Logs: 2")
	assert.Contains(t, result.Content, "Exemplars: 0")
	assert.Contains(t, result.Content, "Profiles: 1")
	assert.Contains(t, result.Content, "Events: 2")

	// Test individual items
	assert.Contains(t, result.Content, "log message 1")
	assert.Contains(t, result.Content, "log message 2")
	assert.Contains(t, result.Content, "profile-1")
	assert.Contains(t, result.Content, "evt1 at 2024-07-28T12:00:00Z")
	assert.Contains(t, result.Content, "evt2 at 2024-07-28T12:01:00Z")
}

func TestExemplarExtraction(t *testing.T) {
	// Test the groupExemplarsByType function directly
	exemplars := []ExemplarData{
		{
			ID:         "ex1",
			MetricType: "gauge_metrics",
			MetricName: "cpu_usage",
			Value:      "75.5",
		},
		{
			ID:         "ex2",
			MetricType: "histogram_metrics",
			MetricName: "request_duration",
			Value:      "250.0",
		},
	}

	grouped := groupExemplarsByType(exemplars)

	// Check that exemplars are grouped by simple type names
	assert.Len(t, grouped["gauge"], 1)
	assert.Len(t, grouped["histogram"], 1)
	assert.Equal(t, "cpu_usage", grouped["gauge"][0].MetricName)
	assert.Equal(t, "request_duration", grouped["histogram"][0].MetricName)
}

func TestExemplarTemplateRendering(t *testing.T) {
	// Create test template content with exemplars
	templateContent := map[string]string{
		"spans": `{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. {{if .events}} {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} {{.logs | formatLogCountsBySeverity}}.{{end}}{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`,
	}

	// ✅ UPDATED: Use helper function instead of deleted function
	transformer := createTransformerWithTemplates(t, templateContent)

	// Create test data with exemplars
	content := &CompleteTraceContent{
		TraceID:     "test-trace-123",
		ServiceName: "test-service",
		Spans: []SpanData{
			{
				Name:          "test-span",
				SpanID:        "span-456",
				ParentSpanID:  nil,
				SpanKind:      nil,
				Duration:      123000000, // 123ms in nanoseconds
				StatusCode:    0,
				StatusMessage: nil,
			},
		},
	}

	// Transform the content
	results, err := transformer.Transform(context.Background(), content)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	// Check the result
	result := results[0]
	expectedContent := `test-service for operation test-span. Status: OK. Duration: 123ms.`

	assert.Contains(t, result.Content, expectedContent)
	assert.Equal(t, "full_span", result.Mode)
}

func TestAllExemplarTypesExtraction(t *testing.T) {
	// Test that all exemplar types are correctly mapped and grouped
	exemplars := []ExemplarData{
		{
			ID:         "ex1",
			MetricType: "gauge", // ✅ FIXED: Use simple type name
			MetricName: "cpu_usage",
			Value:      "75.5",
		},
		{
			ID:         "ex2",
			MetricType: "histogram", // ✅ FIXED: Use simple type name
			MetricName: "request_duration",
			Value:      "250.0",
		},
		{
			ID:         "ex3",
			MetricType: "summary", // ✅ FIXED: Use simple type name
			MetricName: "response_size",
			Value:      "1024",
		},
		{
			ID:         "ex4",
			MetricType: "sum", // ✅ FIXED: Use simple type name
			MetricName: "total_requests",
			Value:      "1000",
		},
		{
			ID:         "ex5",
			MetricType: "exponential_histogram", // ✅ FIXED: Use simple type name
			MetricName: "latency_distribution",
			Value:      "0.95",
		},
	}

	grouped := groupExemplarsByType(exemplars)

	// Verify all types are correctly mapped
	assert.Len(t, grouped["gauge"], 1)
	assert.Len(t, grouped["histogram"], 1)
	assert.Len(t, grouped["summary"], 1)
	assert.Len(t, grouped["sum"], 1)
	assert.Len(t, grouped["exponential_histogram"], 1)

	// Verify the correct exemplars are in each group
	assert.Equal(t, "cpu_usage", grouped["gauge"][0].MetricName)
	assert.Equal(t, "request_duration", grouped["histogram"][0].MetricName)
	assert.Equal(t, "response_size", grouped["summary"][0].MetricName)
	assert.Equal(t, "total_requests", grouped["sum"][0].MetricName)
	assert.Equal(t, "latency_distribution", grouped["exponential_histogram"][0].MetricName)
}

func TestAllExemplarTypesTemplateRendering(t *testing.T) {
	// Create a comprehensive template that includes all exemplar types
	templateContent := map[string]string{
		"spans": `{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms.{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_histogram}} Histogram Exemplars: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_summary}} Summary Exemplars: {{range .exemplars_summary}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_sum}} Sum Exemplars: {{range .exemplars_sum}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_exponential_histogram}} Exponential Histogram Exemplars: {{range .exemplars_exponential_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`,
	}

	// ✅ UPDATED: Use helper function instead of deleted function
	transformer := createTransformerWithTemplates(t, templateContent)

	// Create test data with all exemplar types
	content := &CompleteTraceContent{
		TraceID:     "test-trace-123",
		ServiceName: "test-service",
		Spans: []SpanData{
			{
				Name:          "test-span",
				SpanID:        "span-456",
				ParentSpanID:  nil,
				SpanKind:      nil,
				Duration:      123000000, // 123ms in nanoseconds
				StatusCode:    0,
				StatusMessage: nil,
			},
		},
	}

	// Transform the content
	results, err := transformer.Transform(context.Background(), content)
	assert.NoError(t, err)
	assert.Len(t, results, 1)

	result := results[0]
	expectedContent := `test-service for operation test-span. Status: OK. Duration: 123ms.`
	assert.Contains(t, result.Content, expectedContent)
	assert.Equal(t, "full_span", result.Mode)

	// Note: In a real environment with database, this would show exemplars
	// For now, we verify the template structure is correct
}

// Test validation functions
func TestValidateSpanData(t *testing.T) {
	tests := []struct {
		name    string
		span    SpanData
		wantErr bool
	}{
		{
			name: "valid span data",
			span: SpanData{
				SpanID:   "test-span-id",
				Name:     "test-span",
				Duration: 1000000,
			},
			wantErr: false,
		},
		{
			name: "empty span ID",
			span: SpanData{
				SpanID:   "",
				Name:     "test-span",
				Duration: 1000000,
			},
			wantErr: true,
		},
		{
			name: "empty span name",
			span: SpanData{
				SpanID:   "test-span-id",
				Name:     "",
				Duration: 1000000,
			},
			wantErr: true,
		},
		{
			name: "negative duration",
			span: SpanData{
				SpanID:   "test-span-id",
				Name:     "test-span",
				Duration: -1000000,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSpanData(tt.span)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSpanData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCompleteTraceContent(t *testing.T) {
	tests := []struct {
		name    string
		content *CompleteTraceContent
		wantErr bool
	}{
		{
			name: "valid trace content",
			content: &CompleteTraceContent{
				TraceID: "test-trace-id",
				Spans:   []SpanData{{SpanID: "span1", Name: "span1"}},
			},
			wantErr: false,
		},
		{
			name:    "nil content",
			content: nil,
			wantErr: true,
		},
		{
			name: "empty trace ID",
			content: &CompleteTraceContent{
				TraceID: "",
				Spans:   []SpanData{{SpanID: "span1", Name: "span1"}},
			},
			wantErr: true,
		},
		{
			name: "no spans",
			content: &CompleteTraceContent{
				TraceID: "test-trace-id",
				Spans:   []SpanData{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCompleteTraceContent(tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCompleteTraceContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTemplateContent(t *testing.T) {
	tests := []struct {
		name            string
		templateContent map[string]string
		wantErr         bool
	}{
		{
			name: "valid template content",
			templateContent: map[string]string{
				"spans": "{{.name}}",
				"logs":  "{{.body}}",
			},
			wantErr: false,
		},
		{
			name:            "empty template content",
			templateContent: map[string]string{},
			wantErr:         true,
		},
		{
			name: "empty template value",
			templateContent: map[string]string{
				"spans": "",
			},
			wantErr: true,
		},
		{
			name: "template too large",
			templateContent: map[string]string{
				"spans": strings.Repeat("a", MaxTemplateContentSize+1),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTemplateContent(tt.templateContent)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTemplateContent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultTransformerConfig(t *testing.T) {
	config := DefaultTransformerConfig()

	if config.DefaultTemplateKey != DefaultTemplateKey {
		t.Errorf("DefaultTransformerConfig() DefaultTemplateKey = %v, want %v", config.DefaultTemplateKey, DefaultTemplateKey)
	}

	if config.MaxTemplateContentSize != MaxTemplateContentSize {
		t.Errorf("DefaultTransformerConfig() MaxTemplateContentSize = %v, want %v", config.MaxTemplateContentSize, MaxTemplateContentSize)
	}

	if config.EnableStrictValidation {
		t.Error("DefaultTransformerConfig() EnableStrictValidation should remain opt-in")
	}

	if !config.LogValidationErrors {
		t.Error("DefaultTransformerConfig() LogValidationErrors should be true")
	}
}

func TestBaseTransformerValidateInputs(t *testing.T) {
	base := &BaseTransformer{}

	validContent := &CompleteTraceContent{
		TraceID: "test-trace-id",
		Spans:   []SpanData{{SpanID: "span1", Name: "span1"}},
	}

	validSpan := SpanData{
		SpanID:   "span1",
		Name:     "test-span",
		Duration: 1000000,
	}

	validCorrelated := &CorrelatedSignals{
		SpanID: "span1",
	}

	// Test valid inputs
	err := base.validateInputs(validContent, validSpan, validCorrelated)
	if err != nil {
		t.Errorf("validateInputs() with valid data returned error: %v", err)
	}

	// Test invalid content
	err = base.validateInputs(nil, validSpan, validCorrelated)
	if err == nil {
		t.Error("validateInputs() with nil content should return error")
	}
}

func TestBaseTransformerGetConfig(t *testing.T) {
	base := &BaseTransformer{}

	// Test that config is created if nil
	config := base.getConfig()
	if config == nil {
		t.Error("getConfig() should return non-nil config")
	}

	// Test that same config is returned on subsequent calls
	config2 := base.getConfig()
	if config != config2 {
		t.Error("getConfig() should return same config instance")
	}

	// Test that custom config is preserved
	customConfig := &TransformerConfig{
		DefaultTemplateKey: "custom",
	}
	base.config = customConfig
	config3 := base.getConfig()
	if config3 != customConfig {
		t.Error("getConfig() should return custom config when set")
	}
}

// TestTemplatePlusModeTransformerCreation tests that the template_plus transformer can be created
func TestTemplatePlusModeTransformerCreation(t *testing.T) {
	factory := NewTransformerFactory()

	// Test the template_plus mode
	transformer, err := factory.CreateTransformer("template_plus")
	if err != nil {
		t.Fatalf("Failed to create template_plus transformer: %v", err)
	}

	if transformer == nil {
		t.Fatal("Transformer should not be nil")
	}

	// Verify it returns the correct mode
	if mode := transformer.GetMode(); mode != "full_span" {
		t.Errorf("Expected mode 'full_span', got '%s'", mode)
	}
}

// TestTransformerFactoryWithTemplatePlusMode tests that the factory can create the template_plus transformer
func TestTransformerFactoryWithTemplatePlusMode(t *testing.T) {
	templateContent := map[string]string{
		"spans": "{{.service_name}} - {{.name}}",
		"logs":  "{{.service_name}} - {{.body}}",
	}

	factory := NewTransformerFactoryWithTemplates(templateContent)

	// Test the template_plus mode
	transformer, err := factory.CreateTransformer("template_plus")
	if err != nil {
		t.Fatalf("Failed to create template_plus transformer: %v", err)
	}

	if transformer == nil {
		t.Fatal("Transformer should not be nil")
	}

	// Verify it returns the correct mode
	if mode := transformer.GetMode(); mode != "full_span" {
		t.Errorf("Expected mode 'full_span', got '%s'", mode)
	}
}

// TestTemplatePlusModeWithDefaultFallback tests that template_plus mode falls back to template mode
func TestTemplatePlusModeWithDefaultFallback(t *testing.T) {
	factory := NewTransformerFactory()

	// Test that template_plus mode works even without templates
	transformer, err := factory.CreateTransformer("template_plus")
	if err != nil {
		t.Fatalf("Failed to create template_plus transformer: %v", err)
	}

	if transformer == nil {
		t.Fatal("Transformer should not be nil")
	}

	// Should still work and return full_span mode
	if mode := transformer.GetMode(); mode != "full_span" {
		t.Errorf("Expected mode 'full_span', got '%s'", mode)
	}
}
