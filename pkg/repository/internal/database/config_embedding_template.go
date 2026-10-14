// Package database provides configuration management for embedding templates.
// It includes repository logic for CRUD operations and table initialization for the config_processor_template table.
package database

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

//go:embed ddl/config_processor_template.sql
var embeddingTemplateDDL string

// EmbeddingConfigTemplate holds the template config data
type EmbeddingConfigTemplate struct {
	ID               int64                  `json:"id"`
	Name             string                 `json:"name"`
	Content          map[string]string      `json:"content"`
	TemplateMetadata map[string][]string    `json:"template_metadata"`
	Description      string                 `json:"description"`
	Metadata         map[string]interface{} `json:"metadata"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
}

var DefaultTemplateContent = map[string]string{
	"logs": `Log Event from service {{.service_name | default "unknown"}}. Severity: {{.level | default "info"}}. Message: {{if .body}}{{.body}}{{else}}{{.message | default "no message"}}{{end}}. Attributes: {{.attributes | formatMap | default "none"}}.`,

	"spans": `{{.service_name | default "unknown"}} for operation '{{.name | default "unknown"}}'. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms | default 0}}ms. TraceID: {{.trace_id | default "unknown"}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}{{if .exemplars_histogram}} Histogram Exemplars: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_summary}} Summary Exemplars: {{range .exemplars_summary}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_sum}} Sum Exemplars: {{range .exemplars_sum}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`,

	"gauge_metrics": `Metric: The gauge '{{.name | default "unknown"}}' from service {{.service_name | default "unknown"}} recorded a value of {{.value | default 0}}. Labels: {{.labels | formatMap | default "none"}}.`,

	"counter_metrics": `Metric: The counter '{{.name | default "unknown"}}' from service {{.service_name | default "unknown"}} reached a total of {{.value | default 0}}. Labels: {{.labels | formatMap | default "none"}}.`,

	"sum_metrics": `Metric: The sum '{{.name | default "unknown"}}' from service {{.service_name | default "unknown"}} aggregated to {{.value | default 0}} {{.unit | default "units"}}. Labels: {{.labels | formatMap | default "none"}}.`,

	"histogram_metrics": `Metric: The histogram '{{.name | default "unknown"}}' from service {{.service_name | default "unknown"}} observed {{.count | default 0}} values with a total sum of {{.sum | default 0}}. Buckets: {{.buckets | formatArray | default "none"}}. Labels: {{.labels | formatMap | default "none"}}.`,

	"summary_metrics": `Metric: The summary '{{.name | default "unknown"}}' from service {{.service_name | default "unknown"}} observed {{.count | default 0}} values with a total sum of {{.sum | default 0}}. Quantiles: {{.quantiles | formatMap | default "none"}}. Labels: {{.labels | formatMap | default "none"}}.`,

	"exponential_histogram_metrics": `Metric: The exponential histogram '{{.name | default "unknown"}}' from service {{.service_name | default "unknown"}} observed {{.count | default 0}} values with a sum of {{.sum | default 0}} and scale of {{.scale | default 0}}. Labels: {{.labels | formatMap | default "none"}}.`,
}

var DefaultTemplateMetadata = map[string][]string{
	"generic":                       {"service_name", "timestamp_ns"},
	"logs":                          {"service_name", "level", "body", "message", "attributes"},
	"spans":                         {"name", "service_name", "status_code", "duration", "trace_id", "span_id", "attributes.region", "resource_attributes.trazr_random_seed"},
	"gauge_metrics":                 {"name", "service_name", "value", "labels"},
	"counter_metrics":               {"name", "service_name", "value", "labels"},
	"histogram_metrics":             {"name", "service_name", "count", "sum", "buckets", "labels"},
	"summary_metrics":               {"name", "service_name", "count", "sum", "quantiles", "labels"},
	"sum_metrics":                   {"name", "service_name", "value", "unit", "labels"},
	"exponential_histogram_metrics": {"name", "service_name", "count", "sum", "scale", "labels"},
}

// pgConfigEmbeddingTemplateRepo implements EmbeddingConfigTemplateInterface
// Add DB connection as needed (e.g., *sql.DB)
type pgConfigEmbeddingTemplateRepo struct {
	db     *sql.DB
	schema string
	logger util.Logger
}

func NewTemplateConfigRepo(ctx context.Context, db *sql.DB, logger util.Logger) (EmbeddingConfigTemplateInterface, error) {
	repo := &pgConfigEmbeddingTemplateRepo{db: db, schema: DefaultConfigSchema, logger: logger}
	if err := repo.EnsureTableAndDefault(ctx); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *pgConfigEmbeddingTemplateRepo) getTableName() string {
	return fmt.Sprintf("%s.config_processor_template", r.schema)
}

func (r *pgConfigEmbeddingTemplateRepo) EnsureTableAndDefault(ctx context.Context) error {
	// Check if table exists, create if it doesn't
	checkQuery := fmt.Sprintf(`SELECT 1 FROM %s LIMIT 1`, r.getTableName())
	_, err := r.db.QueryContext(ctx, checkQuery)
	if err != nil {
		if isTableNotExistError(err) {
			ddl := strings.ReplaceAll(embeddingTemplateDDL, "{{.Schema}}", r.schema)
			if _, err := r.db.ExecContext(ctx, ddl); err != nil {
				if r.logger != nil {
					r.logger.Error("Failed to create template table", err)
				}
				return fmt.Errorf("create table: %w", err)
			}
		} else {
			return fmt.Errorf("check table: %w", err)
		}
	}

	// Check if table is empty
	var count int
	err = r.db.QueryRowContext(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s`, r.getTableName())).Scan(&count)
	if err != nil {
		return fmt.Errorf("count templates: %w", err)
	}

	// Only insert default if table is empty
	if count == 0 {
		defaultConfig := &EmbeddingConfigTemplate{
			ID:               1,
			Name:             "default",
			Content:          DefaultTemplateContent,
			TemplateMetadata: DefaultTemplateMetadata,
			Description:      "Default template",
		}
		if err := r.Upsert(ctx, defaultConfig); err != nil {
			if r.logger != nil {
				r.logger.Error("Failed to insert default template", err)
			}
			return fmt.Errorf("insert default template: %w", err)
		}
	}

	return nil
}

func (r *pgConfigEmbeddingTemplateRepo) GetByID(ctx context.Context, id int64) (*EmbeddingConfigTemplate, error) {
	query := fmt.Sprintf(`SELECT id, name, content, template_metadata, description, metadata, created_at, updated_at FROM %s WHERE id = $1`, r.getTableName())
	var tpl EmbeddingConfigTemplate
	var contentBytes, metadataBytes, metaBytes []byte
	if err := r.db.QueryRowContext(ctx, query, id).Scan(&tpl.ID, &tpl.Name, &contentBytes, &metadataBytes, &tpl.Description, &metaBytes, &tpl.CreatedAt, &tpl.UpdatedAt); err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to get template by id", err, "id", id)
		}
		return nil, fmt.Errorf("get template by id: %w", err)
	}

	return r.scanTemplateRow(&tpl, contentBytes, metadataBytes, metaBytes, "id", id)
}

func (r *pgConfigEmbeddingTemplateRepo) GetByName(ctx context.Context, name string) (*EmbeddingConfigTemplate, error) {
	query := fmt.Sprintf(`SELECT id, name, content, template_metadata, description, metadata, created_at, updated_at FROM %s WHERE name = $1`, r.getTableName())
	var tpl EmbeddingConfigTemplate
	var contentBytes, metadataBytes, metaBytes []byte
	if err := r.db.QueryRowContext(ctx, query, name).Scan(&tpl.ID, &tpl.Name, &contentBytes, &metadataBytes, &tpl.Description, &metaBytes, &tpl.CreatedAt, &tpl.UpdatedAt); err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to get template by name", err, "name", name)
		}
		return nil, fmt.Errorf("get template by name: %w", err)
	}
	return r.scanTemplateRow(&tpl, contentBytes, metadataBytes, metaBytes, "name", name)
}

func (r *pgConfigEmbeddingTemplateRepo) GetAll(ctx context.Context) ([]*EmbeddingConfigTemplate, error) {
	query := fmt.Sprintf(`SELECT id, name, content, template_metadata, description, metadata, created_at, updated_at FROM %s`, r.getTableName())
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to get all templates", err)
		}
		return nil, fmt.Errorf("get all templates: %w", err)
	}
	defer rows.Close()
	var templates []*EmbeddingConfigTemplate
	for rows.Next() {
		var tpl EmbeddingConfigTemplate
		var contentBytes, metadataBytes, metaBytes []byte
		if err := rows.Scan(&tpl.ID, &tpl.Name, &contentBytes, &metadataBytes, &tpl.Description, &metaBytes, &tpl.CreatedAt, &tpl.UpdatedAt); err != nil {
			if r.logger != nil {
				r.logger.Error("Failed to scan template row", err)
			}
			return nil, fmt.Errorf("scan template: %w", err)
		}
		scannedTpl, err := r.scanTemplateRow(&tpl, contentBytes, metadataBytes, metaBytes, "row", "all")
		if err != nil {
			return nil, err
		}
		templates = append(templates, scannedTpl)
	}
	return templates, nil
}

func (r *pgConfigEmbeddingTemplateRepo) Upsert(ctx context.Context, config *EmbeddingConfigTemplate) error {
	query := fmt.Sprintf(`INSERT INTO %s (id, name, content, template_metadata, description, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, content = EXCLUDED.content, template_metadata = EXCLUDED.template_metadata, description = EXCLUDED.description, metadata = EXCLUDED.metadata, updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, updated_at`, r.getTableName())
	contentBytes, err := json.Marshal(config.Content)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to marshal template content for upsert", err)
		}
		return fmt.Errorf("marshal content: %w", err)
	}
	metadataBytes, err := json.Marshal(config.TemplateMetadata)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to marshal template metadata for upsert", err)
		}
		return fmt.Errorf("marshal metadata: %w", err)
	}
	err = r.db.QueryRowContext(ctx, query, config.ID, config.Name, contentBytes, metadataBytes, config.Description, config.Metadata, config.CreatedAt, config.UpdatedAt).Scan(&config.ID, &config.CreatedAt, &config.UpdatedAt)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to upsert template", err)
		}
		return fmt.Errorf("upsert template: %w", err)
	}
	return nil
}

func (r *pgConfigEmbeddingTemplateRepo) Delete(ctx context.Context, id int64) error {
	query := fmt.Sprintf(`DELETE FROM %s WHERE id = $1`, r.getTableName())
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to delete template", err, "id", id)
		}
		return fmt.Errorf("delete template: %w", err)
	}
	return nil
}

// scanTemplateRow is a helper method to scan template data from database bytes
func (r *pgConfigEmbeddingTemplateRepo) scanTemplateRow(tpl *EmbeddingConfigTemplate, contentBytes, metadataBytes, metaBytes []byte, fieldType string, fieldValue interface{}) (*EmbeddingConfigTemplate, error) {
	var err error

	// Use utility functions for JSON unmarshaling
	tpl.Content, err = unmarshalJSONFieldGeneric[map[string]string](string(contentBytes), "content")
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to unmarshal template content", err, fieldType, fieldValue)
		}
		return nil, err
	}

	tpl.TemplateMetadata, err = unmarshalJSONFieldGeneric[map[string][]string](string(metadataBytes), "template_metadata")
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to unmarshal template metadata", err, fieldType, fieldValue)
		}
		return nil, err
	}

	tpl.Metadata, err = unmarshalJSONField(string(metaBytes), "metadata")
	if err != nil {
		if r.logger != nil {
			r.logger.Error("Failed to unmarshal template meta", err, fieldType, fieldValue)
		}
		return nil, err
	}

	return tpl, nil
}
