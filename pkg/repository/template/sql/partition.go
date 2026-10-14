package sql

import (
	"fmt"
	"strings"
)

// TimescaleConfig holds configuration for TimescaleDB maintenance
type TimescaleConfig struct {
	Enabled         bool
	MonthsAhead     int
	RetentionMonths int
}

// TimescaleTemplateProcessor processes SQL templates with TimescaleDB configuration
type TimescaleTemplateProcessor struct{}

// NewTimescaleTemplateProcessor creates a new TimescaleDB template processor
func NewTimescaleTemplateProcessor() *TimescaleTemplateProcessor {
	return &TimescaleTemplateProcessor{}
}

// Process processes SQL templates with TimescaleDB configuration
func (p *TimescaleTemplateProcessor) Process(content string, config TimescaleConfig) string {
	// Validate configuration
	if config.MonthsAhead < 0 {
		config.MonthsAhead = 0
	}
	if config.RetentionMonths < 0 {
		config.RetentionMonths = 0
	}

	replacements := map[string]string{
		"{{.PartitionMonthsAhead}}":     fmt.Sprintf("%d", config.MonthsAhead),
		"{{.PartitionFallbackMonths}}":  fmt.Sprintf("%d", config.MonthsAhead),
		"{{.PartitionRetentionMonths}}": fmt.Sprintf("%d", config.RetentionMonths),
	}

	for placeholder, value := range replacements {
		content = strings.ReplaceAll(content, placeholder, value)
	}

	// Remove any remaining conditional partitioning blocks (always enabled for TimescaleDB)
	content = p.processConditionalPartitioning(content, true)

	return content
}

// processConditionalPartitioning handles {{if .EnablePartitioning}}...{{end}} blocks
func (p *TimescaleTemplateProcessor) processConditionalPartitioning(content string, enabled bool) string {
	if enabled {
		// Remove {{if .EnablePartitioning}} and {{end}} tags, keep content
		content = strings.ReplaceAll(content, "{{if .EnablePartitioning}}", "")
		content = strings.ReplaceAll(content, "{{end}}", "")
	} else {
		// Remove entire {{if .EnablePartitioning}}...{{end}} blocks
		content = p.removeConditionalBlocks(content, "{{if .EnablePartitioning}}", "{{end}}")
	}

	return content
}

// removeConditionalBlocks removes conditional blocks from SQL content
func (p *TimescaleTemplateProcessor) removeConditionalBlocks(content, startTag, endTag string) string {
	// Use a simpler approach: find and remove all conditional blocks
	var result strings.Builder
	lines := strings.Split(content, "\n")
	inBlock := false
	blockDepth := 0

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Check for start tag
		if strings.Contains(trimmedLine, startTag) {
			blockDepth++
			inBlock = true

			// Check if end tag is also on the same line
			if strings.Contains(trimmedLine, endTag) {
				blockDepth--
				inBlock = false
				// Handle both tags on the same line
				startIdx := strings.Index(line, startTag)
				endIdx := strings.Index(line, endTag) + len(endTag)

				// Keep the part before the start tag
				if startIdx > 0 {
					result.WriteString(line[:startIdx])
				}
				// Keep the part after the end tag
				if endIdx < len(line) {
					result.WriteString(line[endIdx:])
				}
				result.WriteString("\n")
			} else {
				// Only start tag on this line
				startIdx := strings.Index(line, startTag)
				if startIdx > 0 {
					// Keep the part before the start tag
					result.WriteString(line[:startIdx])
					result.WriteString("\n")
				}
			}
			continue
		}

		// Check for end tag (only if not already handled above)
		if strings.Contains(trimmedLine, endTag) {
			blockDepth--
			if blockDepth == 0 {
				inBlock = false
			}
			// Remove everything up to the end tag on this line
			endIdx := strings.Index(line, endTag) + len(endTag)
			if endIdx < len(line) {
				// Keep the part after the end tag
				result.WriteString(line[endIdx:])
				result.WriteString("\n")
			}
			continue
		}

		// Add line if not in a conditional block
		if !inBlock {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	// Clean up the result
	resultStr := strings.TrimSpace(result.String())

	// Clean up any double spaces that might result
	resultStr = strings.ReplaceAll(resultStr, "  ", " ")

	// Clean up any trailing commas or semicolons that might be orphaned
	resultStr = strings.ReplaceAll(resultStr, ", )", ")")
	resultStr = strings.ReplaceAll(resultStr, ", ;", ";")
	resultStr = strings.ReplaceAll(resultStr, " )", ")")
	resultStr = strings.ReplaceAll(resultStr, " ;", ";")

	return resultStr
}

// ValidateTimescaleConfig validates TimescaleDB configuration
func (p *TimescaleTemplateProcessor) ValidateTimescaleConfig(config TimescaleConfig) error {
	if config.MonthsAhead < 0 {
		return fmt.Errorf("months_ahead cannot be negative, got %d", config.MonthsAhead)
	}

	if config.RetentionMonths < 0 {
		return fmt.Errorf("retention_months cannot be negative, got %d", config.RetentionMonths)
	}

	if config.MonthsAhead > 120 {
		return fmt.Errorf("months_ahead cannot exceed 120, got %d", config.MonthsAhead)
	}

	if config.RetentionMonths > 120 {
		return fmt.Errorf("retention_months cannot exceed 120, got %d", config.RetentionMonths)
	}

	return nil
}

// ProcessWithValidation processes SQL templates with validation
func (p *TimescaleTemplateProcessor) ProcessWithValidation(content string, config TimescaleConfig) (string, error) {
	// Validate configuration first
	if err := p.ValidateTimescaleConfig(config); err != nil {
		return "", fmt.Errorf("invalid TimescaleDB configuration: %w", err)
	}

	// Process the template
	result := p.Process(content, config)

	// Basic validation of result
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf("processed template is empty")
	}

	return result, nil
}
