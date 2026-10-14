package repository

import (
	"strings"
	"text/template"
	"text/template/parse"

	templatepkg "github.com/medxops/trazr-otel/pkg/repository/template"
)

// TemplateFieldExtractor extracts field references from Go templates
type TemplateFieldExtractor struct {
	fieldMapping *templatepkg.FieldMapping
}

// NewTemplateFieldExtractor creates a new template field extractor
func NewTemplateFieldExtractor() *TemplateFieldExtractor {
	return &TemplateFieldExtractor{
		fieldMapping: templatepkg.NewFieldMapping(),
	}
}

// ExtractFieldsFromTemplate parses a Go template string and extracts all field references
func (t *TemplateFieldExtractor) ExtractFieldsFromTemplate(templateStr string) ([]string, error) {
	var fields []string

	// Create a template with custom functions to avoid parsing errors
	funcMap := template.FuncMap{
		"default":                   func(args ...interface{}) interface{} { return "" },
		"formatMap":                 func(args ...interface{}) interface{} { return "" },
		"formatArray":               func(args ...interface{}) interface{} { return "" },
		"formatEventCountsByName":   func(args ...interface{}) interface{} { return "" },
		"formatLogCountsBySeverity": func(args ...interface{}) interface{} { return "" },
	}

	// Parse the template with custom functions
	tmpl, err := template.New("tmp").Funcs(funcMap).Parse(templateStr)
	if err != nil {
		return nil, err
	}

	// Walk the parse tree to extract field references
	var extractFields func(parse.Node, bool)
	extractFields = func(node parse.Node, rootScope bool) {
		if node == nil {
			return
		}

		switch n := node.(type) {
		case *parse.FieldNode:
			if !rootScope {
				return
			}
			// n.Ident is a []string, e.g. ["resource_attributes", "trazr_random_seed"]
			fieldName := strings.Join(n.Ident, ".")
			fields = append(fields, fieldName)
		case *parse.VariableNode:
			// $.field still refers to the original signal inside range/with blocks.
			if len(n.Ident) > 1 && n.Ident[0] == "$" {
				fields = append(fields, strings.Join(n.Ident[1:], "."))
			}
		case *parse.ListNode:
			if n.Nodes != nil {
				for _, child := range n.Nodes {
					extractFields(child, rootScope)
				}
			}
		case *parse.ActionNode:
			if n.Pipe != nil {
				extractFields(n.Pipe, rootScope)
			}
		case *parse.PipeNode:
			if n.Cmds != nil {
				for _, cmd := range n.Cmds {
					extractFields(cmd, rootScope)
				}
			}
		case *parse.CommandNode:
			if n.Args != nil {
				for _, arg := range n.Args {
					extractFields(arg, rootScope)
				}
			}
		case *parse.IfNode:
			if n.Pipe != nil {
				extractFields(n.Pipe, rootScope)
			}
			if n.List != nil {
				extractFields(n.List, rootScope)
			}
			if n.ElseList != nil {
				extractFields(n.ElseList, rootScope)
			}
		case *parse.RangeNode:
			extractFields(n.Pipe, rootScope)
			if n.List != nil {
				extractFields(n.List, false)
			}
			if n.ElseList != nil {
				extractFields(n.ElseList, rootScope)
			}
		case *parse.WithNode:
			extractFields(n.Pipe, rootScope)
			if n.List != nil {
				extractFields(n.List, false)
			}
			if n.ElseList != nil {
				extractFields(n.ElseList, rootScope)
			}
		case *parse.TextNode:
			// Text nodes don't contain field references, skip
		case *parse.TemplateNode:
			// Template nodes are handled by the template engine, skip
		default:
			// For any other node types, we skip them as they don't contain field references
		}
	}

	extractFields(tmpl.Tree.Root, true)
	return t.fieldMapping.UniqueStrings(fields), nil
}

// ExtractFieldsFromTemplates extracts fields from all templates in a template content map
func (t *TemplateFieldExtractor) ExtractFieldsFromTemplates(templateContent map[string]string) (map[string][]string, error) {
	requiredFields := make(map[string][]string)

	for signalType, tmpl := range templateContent {
		fields, err := t.ExtractFieldsFromTemplate(tmpl)
		if err != nil {
			return nil, err
		}
		requiredFields[signalType] = fields
	}

	return requiredFields, nil
}

// MapTemplateFieldsToDBColumns maps template field names to database field configurations
func (t *TemplateFieldExtractor) MapTemplateFieldsToDBColumns(signalType string, templateFields []string) []SignalFieldConfig {
	var fieldConfigs []SignalFieldConfig

	// Always include essential fields regardless of template
	essentialFields := []string{"id", "timestamp_ns", "service_name"}
	for _, field := range essentialFields {
		if config := t.getFieldConfig(signalType, field); config != nil {
			fieldConfigs = append(fieldConfigs, *config)
		}
	}

	// Add template-specific fields
	for _, templateField := range templateFields {
		if config := t.getFieldConfig(signalType, templateField); config != nil {
			fieldConfigs = append(fieldConfigs, *config)
		}
	}

	return fieldConfigs
}

// getFieldConfig maps a template field name to a database field configuration
func (t *TemplateFieldExtractor) getFieldConfig(signalType string, fieldName string) *SignalFieldConfig {
	// Map signal type to field config signal type
	fieldConfigSignalType := t.fieldMapping.GetSignalTypeForFieldConfig(signalType)

	// Get the field configurations for this signal type
	configs, exists := signalFieldConfigs[fieldConfigSignalType]
	if !exists {
		return nil
	}

	// Try direct field name match first
	for _, config := range configs {
		if config.SignalKey == fieldName {
			return &config
		}
	}

	// Handle common field name variations
	dbFieldName := t.fieldMapping.MapTemplateFieldToDBField(fieldName)
	if dbFieldName != fieldName {
		for _, config := range configs {
			if config.SignalKey == dbFieldName {
				return &config
			}
		}
	}

	return nil
}

// AnalyzeTemplate analyzes a template to determine what data it needs
func (t *TemplateFieldExtractor) AnalyzeTemplate(templateContent string) *TemplateAnalysis {
	analysis := &TemplateAnalysis{
		NeedsLogs:      false,
		NeedsExemplars: false,
		NeedsProfiles:  false,
		NeedsEvents:    false,
		ExemplarTypes:  make([]string, 0),
	}

	// Extract required fields from template
	requiredFields, err := t.ExtractFieldsFromTemplate(templateContent)
	if err != nil {
		// If extraction fails, we'll still analyze for correlated signals
		requiredFields = []string{}
	}

	// Check for log-related fields
	if strings.Contains(templateContent, ".logs") || strings.Contains(templateContent, "log_count") {
		analysis.NeedsLogs = true
	}

	// Check for exemplar-related fields
	if strings.Contains(templateContent, ".exemplars") || strings.Contains(templateContent, "exemplar_count") {
		analysis.NeedsExemplars = true
	}

	// Check for profile-related fields
	if strings.Contains(templateContent, ".profiles") || strings.Contains(templateContent, "profile_count") {
		analysis.NeedsProfiles = true
	}

	// Check for event-related fields
	if strings.Contains(templateContent, ".events") || strings.Contains(templateContent, "event_count") {
		analysis.NeedsEvents = true
	}

	// Analyze exemplar types needed
	if analysis.NeedsExemplars {
		if strings.Contains(templateContent, "exemplars_histogram") ||
			strings.Contains(templateContent, "exemplar_count_histogram") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeHistogramMetrics)
		}
		if strings.Contains(templateContent, "exemplars_summary") ||
			strings.Contains(templateContent, "exemplar_count_summary") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeSummaryMetrics)
		}
		if strings.Contains(templateContent, "exemplars_gauge") ||
			strings.Contains(templateContent, "exemplar_count_gauge") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeGaugeMetrics)
		}
		if strings.Contains(templateContent, "exemplars_sum") ||
			strings.Contains(templateContent, "exemplar_count_sum") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeSumMetrics)
		}
		if strings.Contains(templateContent, "exemplars_exponential_histogram") ||
			strings.Contains(templateContent, "exemplar_count_exponential_histogram") {
			analysis.ExemplarTypes = append(analysis.ExemplarTypes, SignalTypeExponentialHistogramMetrics)
		}
	}

	// Set required fields
	analysis.RequiredFields = requiredFields

	return analysis
}
