package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"text/template"
)

// SQLTemplateEngine handles SQL DDL template processing
type SQLTemplateEngine struct {
	timescaleProcessor *TimescaleTemplateProcessor
}

// NewSQLTemplateEngine creates a new SQL template engine
func NewSQLTemplateEngine() *SQLTemplateEngine {
	return &SQLTemplateEngine{
		timescaleProcessor: NewTimescaleTemplateProcessor(),
	}
}

// ProcessTimescaleTemplate processes SQL templates with TimescaleDB configuration
func (e *SQLTemplateEngine) ProcessTimescaleTemplate(content string, config TimescaleConfig) string {
	return e.timescaleProcessor.Process(content, config)
}

// ExecuteDDLTemplate executes a DDL template with parameters and ensures proper schema handling
func (e *SQLTemplateEngine) ExecuteDDLTemplate(templateName, content string, params interface{}) (string, error) {
	tmpl, err := template.New(templateName).Parse(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse DDL template %s: %w", templateName, err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, params); err != nil {
		return "", fmt.Errorf("failed to execute DDL template %s: %w", templateName, err)
	}

	// Ensure schema is properly handled in the generated DDL
	ddlSQL := buf.String()
	ddlSQL = e.ensureSchemaPrefixes(ddlSQL, params)

	return ddlSQL, nil
}

// ValidateSchema validates that a schema exists and is accessible
func (e *SQLTemplateEngine) ValidateSchema(ctx context.Context, db *sql.DB, schema string) error {
	if schema == "" {
		return fmt.Errorf("schema name cannot be empty")
	}

	// Clean the schema name to handle potential search path issues
	// PostgreSQL search paths can be like "$user", public
	// We need to extract just the first schema name
	if strings.Contains(schema, ",") {
		parts := strings.Split(schema, ",")
		if len(parts) > 0 {
			schema = strings.TrimSpace(parts[0])
			// Remove quotes if present
			schema = strings.Trim(schema, `"'`)
		}
	}

	// Handle special PostgreSQL schemas
	if schema == "$user" {
		// Get current user's schema
		var currentUser string
		err := db.QueryRowContext(ctx, `SELECT current_user`).Scan(&currentUser)
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		schema = currentUser
	}

	// Check if schema exists
	query := `SELECT EXISTS(SELECT 1 FROM information_schema.schemata WHERE schema_name = $1)`
	var exists bool
	err := db.QueryRowContext(ctx, query, schema).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check schema existence for '%s': %w", schema, err)
	}

	if !exists {
		return fmt.Errorf("schema '%s' does not exist", schema)
	}

	// Check if we have permissions to use the schema
	query = `SELECT has_schema_privilege($1, 'USAGE')`
	var hasUsage bool
	err = db.QueryRowContext(ctx, query, schema).Scan(&hasUsage)
	if err != nil {
		return fmt.Errorf("failed to check schema permissions for '%s': %w", schema, err)
	}

	if !hasUsage {
		return fmt.Errorf("no USAGE permission on schema '%s'", schema)
	}

	return nil
}

// ensureSchemaPrefixes ensures that table references include proper schema prefixes
func (e *SQLTemplateEngine) ensureSchemaPrefixes(ddlSQL string, params interface{}) string {
	// Extract schema from params if available
	var schema string
	if paramsMap, ok := params.(map[string]interface{}); ok {
		if s, exists := paramsMap["Schema"]; exists {
			if schemaStr, ok := s.(string); ok && schemaStr != "" {
				schema = schemaStr
			}
		}
	}

	// If we have a schema, ensure table references are properly prefixed
	if schema != "" {
		// Common patterns that need schema prefixes
		patterns := []struct {
			pattern     string
			replacement string
		}{
			{"CREATE TABLE IF NOT EXISTS ", fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s.", schema)},
			{"CREATE TABLE ", fmt.Sprintf("CREATE TABLE %s.", schema)},
			{"DROP TABLE IF EXISTS ", fmt.Sprintf("DROP TABLE IF EXISTS %s.", schema)},
			{"DROP TABLE ", fmt.Sprintf("DROP TABLE %s.", schema)},
			{"ALTER TABLE ", fmt.Sprintf("ALTER TABLE %s.", schema)},
			{"INSERT INTO ", fmt.Sprintf("INSERT INTO %s.", schema)},
			{"SELECT * FROM ", fmt.Sprintf("SELECT * FROM %s.", schema)},
			{"CREATE INDEX ", fmt.Sprintf("CREATE INDEX %s.", schema)},
			{"CREATE UNIQUE INDEX ", fmt.Sprintf("CREATE UNIQUE INDEX %s.", schema)},
			{"DROP INDEX IF EXISTS ", fmt.Sprintf("DROP INDEX IF EXISTS %s.", schema)},
			{"DROP INDEX ", fmt.Sprintf("DROP INDEX %s.", schema)},
		}

		for _, p := range patterns {
			// Only replace if the pattern doesn't already have a schema prefix
			if !strings.Contains(ddlSQL, schema+".") {
				ddlSQL = strings.ReplaceAll(ddlSQL, p.pattern, p.replacement)
			}
		}
	}

	return ddlSQL
}

// ExecuteDDLWithValidation executes DDL with schema validation
func (e *SQLTemplateEngine) ExecuteDDLWithValidation(ctx context.Context, db *sql.DB, templateName, content string, params interface{}) error {
	// Extract schema for validation
	var schema string
	if paramsMap, ok := params.(map[string]interface{}); ok {
		if s, exists := paramsMap["Schema"]; exists {
			if schemaStr, ok := s.(string); ok && schemaStr != "" {
				schema = schemaStr
			}
		}
	}

	// Validate schema if provided
	if schema != "" {
		if err := e.ValidateSchema(ctx, db, schema); err != nil {
			return fmt.Errorf("schema validation failed: %w", err)
		}
	}

	// Execute the DDL template
	ddlSQL, err := e.ExecuteDDLTemplate(templateName, content, params)
	if err != nil {
		return fmt.Errorf("failed to execute DDL template: %w", err)
	}

	// Execute the DDL
	_, err = db.ExecContext(ctx, ddlSQL)
	if err != nil {
		return fmt.Errorf("failed to execute DDL: %w", err)
	}

	return nil
}
