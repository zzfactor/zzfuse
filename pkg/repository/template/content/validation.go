package content

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
)

// TemplateValidator provides comprehensive template validation
type TemplateValidator struct {
	allowedFunctions  map[string]bool
	dangerousPatterns []*regexp.Regexp
	maxTemplateLength int
}

// NewTemplateValidator creates a new template validator with security checks
func NewTemplateValidator() *TemplateValidator {
	return &TemplateValidator{
		allowedFunctions: map[string]bool{
			"formatMap": true, "formatArray": true, "formatLabels": true,
			"categorizeLogCount": true, "categorizeDuration": true, "categorizeErrorRate": true,
			"categorizeSpanCount": true, "fastJoin": true, "fastConcat": true,
			"fastReplace": true, "fastContains": true, "formatBytes": true,
			"formatDuration": true, "formatPercentage": true, "ifThenElse": true,
			"defaultIfEmpty": true, "coalesce": true, "categorizeInt": true,
			"categorizeInt64": true, "categorizeFloat64": true, "countLogsBySeverity": true,
			"countEventsByName": true, "formatLogCountsBySeverity": true,
			"formatEventCountsByName": true, "default": true, "if": true, "else": true,
			"end": true, "range": true, "len": true, "index": true, "printf": true,
			"sprintf": true, "trim": true, "upper": true, "lower": true, "title": true,
		},
		dangerousPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)(exec|system|eval|os\.|runtime\.)`),
			regexp.MustCompile(`(?i)(http|https|ftp|file)://`),
			regexp.MustCompile(`(?i)(script|javascript|vbscript)`),
			regexp.MustCompile(`(?i)(union|select|insert|update|delete|drop|create|alter)`),
		},
		maxTemplateLength: 10000, // 10KB limit
	}
}

// ValidateTemplate performs comprehensive template validation
func (tv *TemplateValidator) ValidateTemplate(name, content string) error {
	// Check template length
	if len(content) > tv.maxTemplateLength {
		return fmt.Errorf("template '%s' exceeds maximum length of %d characters", name, tv.maxTemplateLength)
	}

	// Check for dangerous patterns
	for _, pattern := range tv.dangerousPatterns {
		if pattern.MatchString(content) {
			return fmt.Errorf("template '%s' contains potentially dangerous pattern: %s", name, pattern.String())
		}
	}

	// Validate template syntax
	funcMap := NewContentFuncMap()
	_, err := template.New(name).Funcs(funcMap).Parse(content)
	if err != nil {
		return fmt.Errorf("template '%s' has syntax error: %w", name, err)
	}

	// Check for function usage
	if err := tv.validateFunctionUsage(content); err != nil {
		return fmt.Errorf("template '%s' function validation failed: %w", name, err)
	}

	// Check for balanced delimiters
	if err := tv.validateDelimiters(content); err != nil {
		return fmt.Errorf("template '%s' delimiter validation failed: %w", name, err)
	}

	return nil
}

// validateFunctionUsage checks that only allowed functions are used
func (tv *TemplateValidator) validateFunctionUsage(content string) error {
	// Extract function names from template
	functionPattern := regexp.MustCompile(`{{\s*\.([a-zA-Z_][a-zA-Z0-9_]*)\s*\|?\s*([a-zA-Z_][a-zA-Z0-9_]*)`)
	matches := functionPattern.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		if len(match) > 2 && match[2] != "" {
			functionName := match[2]
			if !tv.allowedFunctions[functionName] {
				return fmt.Errorf("function '%s' is not allowed", functionName)
			}
		}
	}

	return nil
}

// validateDelimiters checks for balanced template delimiters
func (tv *TemplateValidator) validateDelimiters(content string) error {
	openCount := strings.Count(content, "{{")
	closeCount := strings.Count(content, "}}")

	if openCount != closeCount {
		return fmt.Errorf("unbalanced template delimiters: %d opening vs %d closing", openCount, closeCount)
	}

	return nil
}

// ValidateTemplateSet validates a complete set of templates
func (tv *TemplateValidator) ValidateTemplateSet(templates map[string]string) error {
	for name, content := range templates {
		if err := tv.ValidateTemplate(name, content); err != nil {
			return err
		}
	}
	return nil
}
