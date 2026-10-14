package content

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"text/template"
)

func TestRenderMatchesGoTemplateSemantics(t *testing.T) {
	data := map[string]interface{}{
		"service_name": "checkout",
		"duration_ms":  int64(123),
		"status_code":  0,
		"enabled":      false,
		"empty":        "",
		"nil":          nil,
		"attributes":   map[string]interface{}{"region": "east"},
	}
	for _, text := range []string{
		`{{.service_name}} duration={{.duration_ms}} status={{.status_code}} enabled={{.enabled}}`,
		`{{ .service_name }} missing={{.missing}} empty={{.empty}} nil={{.nil}}`,
		`{{.service_name}} region={{.attributes.region}} duration={{.duration_ms}}`,
		`missing nested={{.missing.region}}`,
		`{{.service_name}}|{{.attributes}}`,
		` before {{- .service_name -}} after `,
		`{{.service_name}}/{{.service_name}}/{{.duration_ms|printf "%d"}}`,
	} {
		t.Run(text, func(t *testing.T) {
			engine := NewContentTemplateEngine()
			if err := engine.RegisterTemplate("test", text); err != nil {
				t.Fatal(err)
			}
			reference, err := template.New("reference").Funcs(NewContentFuncMap()).Parse(text)
			if err != nil {
				t.Fatal(err)
			}
			var expected bytes.Buffer
			if err := reference.Execute(&expected, data); err != nil {
				t.Fatal(err)
			}
			actual, err := engine.Render("test", data)
			if err != nil {
				t.Fatal(err)
			}
			if actual != expected.String() {
				t.Fatalf("Render = %q, want %q", actual, expected.String())
			}
			var writer bytes.Buffer
			if err := engine.RenderToWriter(&writer, "test", data); err != nil {
				t.Fatal(err)
			}
			if writer.String() != expected.String() {
				t.Fatalf("RenderToWriter = %q, want %q", writer.String(), expected.String())
			}
		})
	}
}

func TestReplacingFastTemplateUsesNewDefinition(t *testing.T) {
	engine := NewContentTemplateEngine()
	if err := engine.RegisterTemplate("test", `old {{.name}}`); err != nil {
		t.Fatal(err)
	}
	if err := engine.RegisterTemplate("test", `{{if .name}}new {{.name}}{{end}}`); err != nil {
		t.Fatal(err)
	}
	actual, err := engine.Render("test", map[string]interface{}{"name": "value"})
	if err != nil || actual != "new value" {
		t.Fatalf("Render = %q, %v; want new value", actual, err)
	}
}

type rejectedWriter struct{ err error }

func (w rejectedWriter) Write([]byte) (int, error) { return 0, w.err }

func TestFastTemplateWriterPropagatesError(t *testing.T) {
	engine := NewContentTemplateEngine()
	if err := engine.RegisterTemplate("test", `{{.name}}`); err != nil {
		t.Fatal(err)
	}
	err := engine.RenderToWriter(rejectedWriter{err: io.ErrClosedPipe}, "test", map[string]interface{}{"name": "value"})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("writer error = %v, want %v", err, io.ErrClosedPipe)
	}
}
