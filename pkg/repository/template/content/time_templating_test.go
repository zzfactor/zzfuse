package content

import (
	"testing"
	"time"
)

// Test data with various time-related fields
var timeTestData = map[string]interface{}{
	"service_name":     "user-service",
	"name":             "process_request",
	"status_code":      "OK",
	"log_count":        15,
	"duration_ms":      250,
	"duration_seconds": 0.25,
	"error_count":      2,
	"span_count":       8,
	"timestamp":        time.Now(),
	"start_time":       time.Now().Add(-time.Hour),
	"end_time":         time.Now(),
	"attributes": map[string]interface{}{
		"user_id":    "12345",
		"request_id": "req-abc-123",
		"endpoint":   "/api/users",
		"method":     "GET",
		"created_at": "2024-01-15T10:30:00Z",
		"expires_at": "2024-01-15T11:30:00Z",
	},
}

func TestTimeTemplatingExamples(t *testing.T) {
	engine := NewContentTemplateEngine()

	// Test 1: Basic time formatting
	t.Run("Basic Time Formatting", func(t *testing.T) {
		template := `Request processed at {{.timestamp | date "2006-01-02 15:04:05"}}`
		engine.RegisterTemplate("basic_time", template)

		result, err := engine.Render("basic_time", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Basic time formatting: %s", result)
	})

	// Test 2: Duration formatting and categorization
	t.Run("Duration Formatting", func(t *testing.T) {
		template := `Duration: {{.duration_ms | formatDuration}} ({{.duration_ms | categorizeDuration}})`
		engine.RegisterTemplate("duration", template)

		result, err := engine.Render("duration", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Duration formatting: %s", result)
	})

	// Test 3: Time calculations with ago
	t.Run("Time Calculations", func(t *testing.T) {
		template := `Started {{.start_time | ago}} ago, finished {{.end_time | ago}} ago`
		engine.RegisterTemplate("time_calc", template)

		result, err := engine.Render("time_calc", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Time calculations: %s", result)
	})

	// Test 4: Unix epoch conversion
	t.Run("Unix Epoch", func(t *testing.T) {
		template := `Unix timestamp: {{.timestamp | unixEpoch}}`
		engine.RegisterTemplate("unix", template)

		result, err := engine.Render("unix", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Unix epoch: %s", result)
	})

	// Test 5: Duration from seconds
	t.Run("Duration from Seconds", func(t *testing.T) {
		template := `Duration: {{.duration_seconds | duration}}`
		engine.RegisterTemplate("duration_sec", template)

		result, err := engine.Render("duration_sec", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Duration from seconds: %s", result)
	})

	// Test 6: Date modification
	t.Run("Date Modification", func(t *testing.T) {
		template := `One hour ago: {{.timestamp | dateModify "-1h" | date "2006-01-02 15:04:05"}}`
		engine.RegisterTemplate("date_mod", template)

		result, err := engine.Render("date_mod", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Date modification: %s", result)
	})

	// Test 7: String to date conversion
	t.Run("String to Date", func(t *testing.T) {
		template := `Created: {{.attributes.created_at | toDate "2006-01-02T15:04:05Z" | date "Jan 02, 2006 at 3:04 PM"}}`
		engine.RegisterTemplate("string_date", template)

		result, err := engine.Render("string_date", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("String to date: %s", result)
	})

	// Test 8: Timezone handling
	t.Run("Timezone Handling", func(t *testing.T) {
		template := `UTC: {{dateInZone "2006-01-02 15:04:05" .timestamp "UTC"}}`
		engine.RegisterTemplate("timezone", template)

		result, err := engine.Render("timezone", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Timezone handling: %s", result)
	})

	// Test 9: HTML date formatting
	t.Run("HTML Date Formatting", func(t *testing.T) {
		template := `HTML Date: {{.timestamp | htmlDate}}`
		engine.RegisterTemplate("html_date", template)

		result, err := engine.Render("html_date", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("HTML date: %s", result)
	})

	// Test 10: Complex time-based template (like from examples)
	t.Run("Complex Time Template", func(t *testing.T) {
		template := `Trace from {{.service_name}}: {{.duration_ms | categorizeDuration}} performance ({{.duration_ms | formatDuration}}), started {{.start_time | ago}} ago, completed at {{.end_time | date "15:04:05"}}`
		engine.RegisterTemplate("complex_time", template)

		result, err := engine.Render("complex_time", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Complex time template: %s", result)
	})

	// Test 11: Conditional time logic
	t.Run("Conditional Time Logic", func(t *testing.T) {
		template := `{{if gt .duration_ms 1000}}
Slow operation: {{.duration_ms | formatDuration}} ({{.duration_ms | categorizeDuration}})
{{else if gt .duration_ms 100}}
Normal operation: {{.duration_ms | formatDuration}} ({{.duration_ms | categorizeDuration}})
{{else}}
Fast operation: {{.duration_ms | formatDuration}} ({{.duration_ms | categorizeDuration}})
{{end}}`
		engine.RegisterTemplate("conditional_time", template)

		result, err := engine.Render("conditional_time", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Conditional time logic: %s", result)
	})

	// Test 12: Duration rounding
	t.Run("Duration Rounding", func(t *testing.T) {
		template := `Rounded duration: {{.duration_seconds | duration | durationRound}}`
		engine.RegisterTemplate("duration_round", template)

		result, err := engine.Render("duration_round", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Duration rounding: %s", result)
	})
}

// Test all the examples from TEMPLATE_CATEGORIZATION_EXAMPLES.md
func TestCategorizationExamples(t *testing.T) {
	engine := NewContentTemplateEngine()

	// Test the exact template from the examples
	t.Run("Original Request Template", func(t *testing.T) {
		template := `service={{.service_name}} operation={{.name}} status={{.status_code}}{{if .status_message}} error="{{.status_message}}"{{end}}{{if .log_count}} log_volume="{{.log_count | categorizeLogCount}}"{{end}}{{range $key, $value := .attributes}} attr_{{$key}}="{{$value}}"{{end}}`
		engine.RegisterTemplate("original", template)

		result, err := engine.Render("original", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Original request template: %s", result)
	})

	// Test the full trace template from examples
	t.Run("Full Trace Template", func(t *testing.T) {
		template := `Trace {{.trace_id | default "unknown"}} from {{.service_name}}: {{.duration_ms | categorizeDuration}} performance ({{.duration_ms}}ms), {{.span_count | categorizeSpanCount}} complexity ({{.span_count}} spans), {{.log_count | categorizeLogCount}} logging ({{.log_count}} logs){{if and .error_count .span_count}}, {{.error_count | categorizeErrorRate .span_count}} error rate{{end}}`
		engine.RegisterTemplate("full_trace", template)

		// Add trace_id to test data
		testData := timeTestData
		testData["trace_id"] = "abc123"

		result, err := engine.Render("full_trace", testData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Full trace template: %s", result)
	})

	// Test conditional logic with categorization
	t.Run("Conditional Logic with Categorization", func(t *testing.T) {
		template := `{{if gt .log_count 10}}
High log volume detected: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{else if gt .log_count 5}}
Moderate log volume: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{else}}
Low log volume: {{.log_count}} logs ({{.log_count | categorizeLogCount}})
{{end}}

{{if and (eq .status_code "ERROR") (gt .duration_ms 1000)}}
Slow error detected: {{.duration_ms}}ms duration ({{.duration_ms | categorizeDuration}})
{{end}}`
		engine.RegisterTemplate("conditional_cat", template)

		result, err := engine.Render("conditional_cat", timeTestData)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("Conditional logic with categorization: %s", result)
	})
}

// Benchmark time templating performance
func BenchmarkTimeTemplating(b *testing.B) {
	engine := NewContentTemplateEngine()

	// Simple time template
	simpleTemplate := `{{.timestamp | date "2006-01-02 15:04:05"}}`
	engine.RegisterTemplate("simple_time", simpleTemplate)

	// Complex time template
	complexTemplate := `Duration: {{.duration_ms | formatDuration}} ({{.duration_ms | categorizeDuration}}), started {{.start_time | ago}} ago`
	engine.RegisterTemplate("complex_time", complexTemplate)

	b.Run("Simple Time Template", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_, err := engine.Render("simple_time", timeTestData)
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Complex Time Template", func(b *testing.B) {
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_, err := engine.Render("complex_time", timeTestData)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
