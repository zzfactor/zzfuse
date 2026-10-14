package content

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
	"github.com/valyala/fasttemplate"
)

// ContentTemplateEngine provides high-performance template rendering for embedding content generation
type ContentTemplateEngine struct {
	complexTemplates map[string]*template.Template
	simpleTemplates  map[string]*fasttemplate.Template
	simpleFields     map[string]map[string]string
	pool             *TemplatePool
	funcMap          template.FuncMap
	metrics          *TemplateMetrics
	logger           util.Logger
}

// NewContentTemplateEngine creates a new content template engine
func NewContentTemplateEngine() *ContentTemplateEngine {
	return &ContentTemplateEngine{
		complexTemplates: make(map[string]*template.Template),
		simpleTemplates:  make(map[string]*fasttemplate.Template),
		simpleFields:     make(map[string]map[string]string),
		pool:             NewTemplatePool(),
		funcMap:          NewContentFuncMap(),
		metrics:          NewTemplateMetrics(),
	}
}

// RegisterTemplate registers a template with the engine
func (e *ContentTemplateEngine) RegisterTemplate(name, content string) error {
	// Parse every template with Go semantics, including the fast-path fallback.
	tmpl, err := template.New(name).Funcs(e.funcMap).Parse(content)
	if err != nil {
		return err
	}
	e.complexTemplates[name] = tmpl
	delete(e.simpleTemplates, name)
	delete(e.simpleFields, name)
	if fields, ok := simpleTemplateFields(content); ok {
		e.simpleTemplates[name] = fasttemplate.New(content, "{{", "}}")
		e.simpleFields[name] = fields
	}
	return nil
}

// Render renders a template using the most efficient method
func (e *ContentTemplateEngine) Render(templateName string, data map[string]interface{}) (string, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		if e.metrics != nil {
			e.metrics.RecordRender(templateName, duration)
		}
	}()

	// Try simple template first (fastest)
	if simpleTmpl, exists := e.simpleTemplates[templateName]; exists {
		if substitutions, ok := e.simpleSubstitutions(templateName, data); ok {
			return simpleTmpl.ExecuteString(substitutions), nil
		}
	}

	// Fall back to complex template with pool
	if complexTmpl, exists := e.complexTemplates[templateName]; exists {
		result, err := e.pool.RenderToString(complexTmpl, data)
		if err != nil && e.metrics != nil {
			e.metrics.RecordRenderError(templateName)
		}
		return result, err
	}

	if e.metrics != nil {
		e.metrics.RecordRenderError(templateName)
	}
	return "", fmt.Errorf("template not found: %s", templateName)
}

// RenderToWriter renders a template directly to an io.Writer (zero-allocation)
func (e *ContentTemplateEngine) RenderToWriter(w io.Writer, templateName string, data interface{}) error {
	// Try simple template first
	if simpleTmpl, exists := e.simpleTemplates[templateName]; exists {
		if dataMap, ok := data.(map[string]interface{}); ok {
			if substitutions, ok := e.simpleSubstitutions(templateName, dataMap); ok {
				_, err := simpleTmpl.Execute(w, substitutions)
				return err
			}
		}
		// Fall back to complex template if type conversion fails
	}

	// Use complex template with direct writer
	if complexTmpl, exists := e.complexTemplates[templateName]; exists {
		return e.pool.Render(w, complexTmpl, data)
	}

	return fmt.Errorf("template not found: %s", templateName)
}

// GetMetrics returns the current template metrics
func (e *ContentTemplateEngine) GetMetrics() map[string]interface{} {
	if e.metrics != nil {
		return e.metrics.GetMetrics()
	}
	return nil
}

// ResetMetrics resets all template metrics
func (e *ContentTemplateEngine) ResetMetrics() {
	if e.metrics != nil {
		e.metrics.ResetMetrics()
	}
}

var simpleFieldAction = regexp.MustCompile(`\{\{(\s*\.([A-Za-z_][A-Za-z0-9_]*)\s*)\}\}`)

// Only direct field lookups have a fast substitution equivalent. Nested fields,
// pipelines, whitespace trimming, and all other Go actions use text/template.
func simpleTemplateFields(content string) (map[string]string, bool) {
	fields := make(map[string]string)
	position := 0
	for _, match := range simpleFieldAction.FindAllStringSubmatchIndex(content, -1) {
		if strings.Contains(content[position:match[0]], "{{") {
			return nil, false
		}
		fields[content[match[2]:match[3]]] = content[match[4]:match[5]]
		position = match[1]
	}
	if strings.Contains(content[position:], "{{") {
		return nil, false
	}
	return fields, true
}

// Preformat scalar values so FastTemplate neither drops Go's leading-dot
// lookups nor panics on numeric fields. Other types retain Go's full semantics.
func (e *ContentTemplateEngine) simpleSubstitutions(name string, data map[string]interface{}) (map[string]interface{}, bool) {
	values := make(map[string]interface{}, len(e.simpleFields[name]))
	for tag, field := range e.simpleFields[name] {
		value, exists := data[field]
		if !exists || value == nil {
			values[tag] = "<no value>"
			continue
		}
		switch value.(type) {
		case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, uintptr, float32, float64, complex64, complex128:
			values[tag] = fmt.Sprint(value)
		default:
			return nil, false
		}
	}
	return values, true
}
