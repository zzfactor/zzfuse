package repository

// =========================
// Attribute Template System
// =========================

// AttributeTemplate defines the interface for filtering attributes
// This allows for template-based attribute selection in the future
type AttributeTemplate interface {
	// Span attribute filtering
	ShouldIncludeSpanAttribute(key string) bool

	// Signal attribute filtering
	ShouldIncludeLogAttribute(key string) bool
	ShouldIncludeMetricAttribute(key string) bool
	ShouldIncludeProfileAttribute(key string) bool

	// Template metadata
	GetTemplateName() string
	GetDescription() string
}

// AllAttributesTemplate is the default template that includes ALL attributes
// This is used for the current implementation without filtering
type AllAttributesTemplate struct{}

// NewAllAttributesTemplate creates a new template that includes all attributes
func NewAllAttributesTemplate() *AllAttributesTemplate {
	return &AllAttributesTemplate{}
}

// ShouldIncludeSpanAttribute returns true for all span attributes (no filtering)
func (t *AllAttributesTemplate) ShouldIncludeSpanAttribute(key string) bool {
	return true
}

// ShouldIncludeLogAttribute returns true for all log attributes (no filtering)
func (t *AllAttributesTemplate) ShouldIncludeLogAttribute(key string) bool {
	return true
}

// ShouldIncludeMetricAttribute returns true for all metric attributes (no filtering)
func (t *AllAttributesTemplate) ShouldIncludeMetricAttribute(key string) bool {
	return true
}

// ShouldIncludeProfileAttribute returns true for all profile attributes (no filtering)
func (t *AllAttributesTemplate) ShouldIncludeProfileAttribute(key string) bool {
	return true
}

// GetTemplateName returns the template name
func (t *AllAttributesTemplate) GetTemplateName() string {
	return "all_attributes"
}

// GetDescription returns the template description
func (t *AllAttributesTemplate) GetDescription() string {
	return "Include all attributes without filtering (default template)"
}

// =========================
// Future Template Implementations (for reference)
// =========================

// FilteredAttributesTemplate will be implemented in the future
// to support template-based attribute filtering
type FilteredAttributesTemplate struct {
	// Template configuration will be added here
	// For now, this is just a placeholder
}

// Example of future template structure (not implemented yet):
/*
type FilteredAttributesTemplate struct {
	SpanAttributeRules   []string
	LogAttributeRules    []string
	MetricAttributeRules []string
	ProfileAttributeRules []string
}

func (t *FilteredAttributesTemplate) ShouldIncludeSpanAttribute(key string) bool {
	// Apply template rules to determine if attribute should be included
	// Example: include only db.*, http.*, service.* attributes
	return matchesTemplateRules(key, t.SpanAttributeRules)
}
*/
