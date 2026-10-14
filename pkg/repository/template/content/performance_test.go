package content

import (
	"testing"
	"text/template"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

// Benchmark data that represents typical telemetry data
var benchmarkData = map[string]interface{}{
	"service_name": "user-service",
	"name":         "process_request",
	"status_code":  "OK",
	"log_count":    15,
	"duration_ms":  250,
	"error_count":  2,
	"span_count":   8,
	"attributes": map[string]interface{}{
		"user_id":    "12345",
		"request_id": "req-abc-123",
		"endpoint":   "/api/users",
		"method":     "GET",
	},
	"resource_attributes": map[string]interface{}{
		"service.name":    "user-service",
		"service.version": "1.2.3",
		"deployment.env":  "production",
	},
}

// Simple template for FastTemplate optimization
const simpleTemplate = `service={{.service_name}} operation={{.name}} status={{.status_code}} duration={{.duration_ms}}ms`

// Complex template for text/template with functions
const complexTemplate = `Service: {{.service_name}}
Operation: {{.name}}
Status: {{.status_code}}
Log Volume: {{.log_count | categorizeLogCount}}
Duration: {{.duration_ms | formatDuration}}
Error Rate: {{.error_count | categorizeErrorRate .span_count}}
Span Count: {{.span_count | categorizeSpanCount}}
Attributes: {{.attributes | formatMap}}
Resource: {{.resource_attributes | formatMap}}`

func BenchmarkContentTemplateEngine_SimpleTemplate(b *testing.B) {
	engine := NewContentTemplateEngine()
	engine.RegisterTemplate("simple", simpleTemplate)
	engine.RegisterTemplate("complex", complexTemplate)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.Render("simple", benchmarkData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContentTemplateEngine_ComplexTemplate(b *testing.B) {
	engine := NewContentTemplateEngine()
	engine.RegisterTemplate("simple", simpleTemplate)
	engine.RegisterTemplate("complex", complexTemplate)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := engine.Render("complex", benchmarkData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContentTemplateEngine_RenderToWriter(b *testing.B) {
	engine := NewContentTemplateEngine()
	engine.RegisterTemplate("simple", simpleTemplate)
	engine.RegisterTemplate("complex", complexTemplate)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Use a buffer to simulate io.Writer
		var buf []byte
		err := engine.RenderToWriter(&mockWriter{&buf}, "simple", benchmarkData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemplatePool_Render(b *testing.B) {
	pool := NewTemplatePool()

	// Create a template for testing
	tmpl, err := template.New("test").Parse(simpleTemplate)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Use a buffer to simulate io.Writer
		var buf []byte
		err := pool.Render(&mockWriter{&buf}, tmpl, benchmarkData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTemplatePool_RenderToString(b *testing.B) {
	pool := NewTemplatePool()

	// Create a template for testing
	tmpl, err := template.New("test").Parse(simpleTemplate)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, err := pool.RenderToString(tmpl, benchmarkData)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Test performance improvements with different template types
func TestPerformanceImprovements(t *testing.T) {
	engine := NewContentTemplateEngine()
	engine.RegisterTemplate("simple", simpleTemplate)
	engine.RegisterTemplate("complex", complexTemplate)

	// Test simple template performance
	start := time.Now()
	for i := 0; i < 10000; i++ {
		_, err := engine.Render("simple", benchmarkData)
		if err != nil {
			t.Fatal(err)
		}
	}
	simpleDuration := time.Since(start)

	// Test complex template performance
	start = time.Now()
	for i := 0; i < 10000; i++ {
		_, err := engine.Render("complex", benchmarkData)
		if err != nil {
			t.Fatal(err)
		}
	}
	complexDuration := time.Since(start)

	t.Logf("Simple template (10k renders): %v", simpleDuration)
	t.Logf("Complex template (10k renders): %v", complexDuration)
	t.Logf("Performance ratio (simple/complex): %.2f", float64(simpleDuration)/float64(complexDuration))

	// Verify that simple templates are significantly faster
	if float64(simpleDuration) > float64(complexDuration)*0.8 {
		t.Log("Warning: Simple template not significantly faster than complex template")
	}
}

// Mock writer for testing RenderToWriter
type mockWriter struct {
	buf *[]byte
}

func (w *mockWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

// Benchmark template function performance
func BenchmarkTemplateFunctions(b *testing.B) {
	b.Run("CategorizeLogCount", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = util.CategorizeLogCount(15)
		}
	})

	b.Run("CategorizeDuration", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = util.CategorizeDuration(250)
		}
	})

	b.Run("FormatMap", func(b *testing.B) {
		data := map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
			"key3": 123,
		}
		for i := 0; i < b.N; i++ {
			_ = util.FormatMap(data)
		}
	})
}

// Test concurrent template rendering performance
func BenchmarkConcurrentTemplateRendering(b *testing.B) {
	engine := NewContentTemplateEngine()
	engine.RegisterTemplate("simple", simpleTemplate)
	engine.RegisterTemplate("complex", complexTemplate)

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := engine.Render("simple", benchmarkData)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
