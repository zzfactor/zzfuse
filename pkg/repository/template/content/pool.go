package content

import (
	"bytes"
	"io"
	"sync"
	"text/template"
)

// TemplatePool provides efficient template rendering with buffer reuse
type TemplatePool struct {
	pool sync.Pool
}

// NewTemplatePool creates a new template pool
func NewTemplatePool() *TemplatePool {
	return &TemplatePool{
		pool: sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			},
		},
	}
}

// Render renders a template directly to an io.Writer (zero-allocation)
func (tp *TemplatePool) Render(w io.Writer, tmpl *template.Template, data interface{}) error {
	return tmpl.Execute(w, data)
}

// RenderToString renders a template to a string (with unavoidable allocation)
func (tp *TemplatePool) RenderToString(tmpl *template.Template, data interface{}) (string, error) {
	buf := tp.pool.Get().(*bytes.Buffer)
	buf.Reset()
	defer tp.pool.Put(buf)

	err := tmpl.Execute(buf, data)
	return buf.String(), err
}
