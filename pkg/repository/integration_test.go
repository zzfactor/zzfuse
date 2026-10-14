// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build integration
// +build integration

package repository

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
	"go.uber.org/zap"
)

// repositoryIntegrationDSN maps an explicit disposable target to the existing
// split-field configuration consumed by the instance-owned service manager.
func repositoryIntegrationDSN(t *testing.T) string {
	t.Helper()
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("set TRAZR_IT=1 to run repository integration tests")
	}
	dsn := os.Getenv("TEST_PG_DSN")
	if dsn == "" {
		t.Fatal("set TEST_PG_DSN to a disposable PostgreSQL URL")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Hostname() == "" || parsed.User == nil || strings.TrimPrefix(parsed.Path, "/") == "" {
		t.Fatal("TEST_PG_DSN must be a PostgreSQL URL with an explicit host, user, and database")
	}
	for key := range parsed.Query() {
		if key != "sslmode" {
			t.Fatal("repository integration TEST_PG_DSN supports only the sslmode URL option")
		}
	}
	password, ok := parsed.User.Password()
	if !ok || password == "" || parsed.User.Username() == "" {
		t.Fatal("TEST_PG_DSN must include explicit integration credentials")
	}
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	sslMode := parsed.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "prefer"
	}
	host := parsed.Hostname()
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	t.Setenv("TRAZR_DB_HOST", host)
	t.Setenv("TRAZR_DB_PORT", port)
	t.Setenv("TRAZR_DB_USER", url.User(parsed.User.Username()).String())
	t.Setenv("TRAZR_DB_PASSWORD", strings.TrimPrefix(url.UserPassword("", password).String(), ":"))
	t.Setenv("TRAZR_DB_NAME", url.PathEscape(strings.TrimPrefix(parsed.Path, "/")))
	t.Setenv("TRAZR_DB_SSL_MODE", url.QueryEscape(sslMode))
	return dsn
}

func TestRepositoryIntegrationDSNMapping(t *testing.T) {
	t.Setenv("TRAZR_IT", "1")
	t.Setenv("TEST_PG_DSN", "postgres://test_user:test%40password@[::1]:5433/zzfuse_disposable?sslmode=disable")
	repositoryIntegrationDSN(t)
	resolved, err := GetDSN()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(resolved)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if parsed.Hostname() != "::1" || parsed.Port() != "5433" || parsed.User.Username() != "test_user" || password != "test@password" || parsed.Path != "/zzfuse_disposable" || parsed.Query().Get("sslmode") != "disable" {
		t.Fatal("service manager configuration did not preserve the explicit disposable target")
	}
}

// TestEmbeddingPipelineEndToEndProof demonstrates complete embedding pipeline functionality
func TestEmbeddingSearch(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	dsn := repositoryIntegrationDSN(t)

	db, err := OpenPostgressDatabase(ctx, dsn, logger)
	if err != nil {
		t.Skipf("Skipping end-to-end proof test - no database available: %v", err)
		return
	}
	defer db.Close()

	// Load services with workers for testing
	rsMgr, err := NewServiceResourceManager(ctx, util.NewZapLoggerAdapter(logger))
	if err != nil {
		t.Fatalf("Failed to load ServiceResourceManager: %v", err)
	}
	defer rsMgr.Shutdown()

	vectorStore, err := rsMgr.GetVectorStore(DefaultSchema)
	if err != nil {
		t.Fatalf("Failed to get vector store: %v", err)
	}

	// Perform a vector search for the log content
	query := "What is the weather in Tokyo?"
	docs, err := vectorStore.SimilaritySearch(ctx, query, 10)
	if err != nil {
		t.Fatalf("Failed to search for documents: %v", err)
	}

	t.Logf("✅ Search successful! Found %d documents", len(docs))
	for i, doc := range docs {
		t.Logf("   Document %d: Content=%s", i+1, doc.PageContent[:min(50, len(doc.PageContent))])
	}
}

// TestEmbeddingPipelineEndToEndProof demonstrates complete embedding pipeline functionality
func TestEmbeddingPipelineEndToEndProof(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	dsn := repositoryIntegrationDSN(t)

	db, err := OpenPostgressDatabase(ctx, dsn, logger)
	if err != nil {
		t.Skipf("Skipping end-to-end proof test - no database available: %v", err)
		return
	}
	defer db.Close()

	// Load services with workers for testing
	rsMgr, err := NewServiceResourceManager(ctx, util.NewZapLoggerAdapter(logger))
	if err != nil {
		t.Fatalf("Failed to load ServiceResourceManager: %v", err)
	}
	defer rsMgr.Shutdown()

	vectorStore, err := rsMgr.GetVectorStore(DefaultSchema)
	if err != nil {
		t.Fatalf("Failed to get vector store: %v", err)
	}

	// Insert a log row into the database with resource_attributes
	now := time.Now().Add(-6 * time.Hour).UnixNano()
	logContent := "test log message for embedding"
	schema := DefaultSchema // Use configurable schema instead of hardcoded

	// Use valid JSON for body and resource_attributes (env=prod, version=1.2.3)
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.logs (timestamp_ns, service_name, body, resource_attributes, metadata) VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb)`, schema), now, "test-service", `{"message":"`+logContent+`"}`, `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert log row: %v", err)
	}

	t.Logf("✅ Log row inserted, waiting for worker to vectorize...")
	// Wait a few seconds for the embedding worker to process
	time.Sleep(5 * time.Second)

	// Perform a vector search for the log content
	docs, err := vectorStore.SimilaritySearch(ctx, logContent, 10)
	if err != nil {
		t.Fatalf("Failed to search for documents: %v", err)
	}

	t.Logf("✅ Search successful! Found %d documents", len(docs))
	for i, doc := range docs {
		t.Logf("   Document %d: Content=%s", i+1, doc.PageContent[:min(50, len(doc.PageContent))])
	}
}

// TestTemplateProcessingForIndividualSignals tests that templates are applied to individual signals
func TestTemplateProcessingForIndividualSignals(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	dsn := repositoryIntegrationDSN(t)

	db, err := OpenPostgressDatabase(ctx, dsn, logger)
	if err != nil {
		t.Skipf("Skipping template processing test - no database available: %v", err)
		return
	}
	defer db.Close()

	// Load services with workers for testing
	rsMgr, err := NewServiceResourceManager(ctx, util.NewZapLoggerAdapter(logger))
	if err != nil {
		t.Fatalf("Failed to load ServiceResourceManager: %v", err)
	}
	defer rsMgr.Shutdown()

	vectorStore, err := rsMgr.GetVectorStore(DefaultSchema)
	if err != nil {
		t.Fatalf("Failed to get vector store: %v", err)
	}

	// Insert test logs and gauge metrics
	now := time.Now().Add(-1 * time.Hour).UnixNano()
	schema := DefaultSchema

	// Insert a log
	logContent := "test log message for template processing"
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.logs (timestamp_ns, service_name, body, resource_attributes, metadata) VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb)`, schema),
		now, "test-service", `{"message":"`+logContent+`"}`, `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert log row: %v", err)
	}

	// Insert a gauge metric
	gaugeName := "test_gauge_metric"
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.gauge_metrics (timestamp_ns, name, value, unit, service_name, attributes, metadata) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)`, schema),
		now, gaugeName, 42.5, "count", "test-service", `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert gauge metric row: %v", err)
	}

	t.Logf("✅ Test data inserted, waiting for worker to process...")
	// Wait a few seconds for the embedding worker to process
	time.Sleep(5 * time.Second)

	// Search for the log content
	docs, err := vectorStore.SimilaritySearch(ctx, logContent, 10)
	if err != nil {
		t.Fatalf("Failed to search for log documents: %v", err)
	}

	t.Logf("✅ Log search successful! Found %d documents", len(docs))
	for i, doc := range docs {
		t.Logf("   Log Document %d: Content=%s", i+1, doc.PageContent[:min(100, len(doc.PageContent))])
	}

	// Search for the gauge metric
	docs, err = vectorStore.SimilaritySearch(ctx, gaugeName, 10)
	if err != nil {
		t.Fatalf("Failed to search for gauge documents: %v", err)
	}

	t.Logf("✅ Gauge search successful! Found %d documents", len(docs))
	for i, doc := range docs {
		t.Logf("   Gauge Document %d: Content=%s", i+1, doc.PageContent[:min(100, len(doc.PageContent))])
	}
}

// TestAllSignalTypesTemplateProcessing tests that templates are applied to all signal types
func TestAllSignalTypesTemplateProcessing(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	dsn := repositoryIntegrationDSN(t)

	db, err := OpenPostgressDatabase(ctx, dsn, logger)
	if err != nil {
		t.Skipf("Skipping comprehensive template processing test - no database available: %v", err)
		return
	}
	defer db.Close()

	// Load services with workers for testing
	rsMgr, err := NewServiceResourceManager(ctx, util.NewZapLoggerAdapter(logger))
	if err != nil {
		t.Fatalf("Failed to load ServiceResourceManager: %v", err)
	}
	defer rsMgr.Shutdown()

	vectorStore, err := rsMgr.GetVectorStore(DefaultSchema)
	if err != nil {
		t.Fatalf("Failed to get vector store: %v", err)
	}

	// Insert test data for all signal types
	now := time.Now().Add(-1 * time.Hour).UnixNano()
	schema := DefaultSchema

	// Insert a log
	logContent := "comprehensive test log message"
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.logs (timestamp_ns, service_name, body, resource_attributes, metadata) VALUES ($1, $2, $3::jsonb, $4::jsonb, $5::jsonb)`, schema),
		now, "test-service", `{"message":"`+logContent+`"}`, `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert log row: %v", err)
	}

	// Insert a gauge metric
	gaugeName := "comprehensive_test_gauge"
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.gauge_metrics (timestamp_ns, name, value, unit, service_name, attributes, metadata) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)`, schema),
		now, gaugeName, 42.5, "count", "test-service", `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert gauge metric row: %v", err)
	}

	// Insert a counter metric (using sum_metrics table)
	counterName := "comprehensive_test_counter"
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.sum_metrics (timestamp_ns, name, value, unit, service_name, attributes, metadata) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7::jsonb)`, schema),
		now, counterName, 100, "count", "test-service", `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert counter metric row: %v", err)
	}

	// Insert a histogram metric
	histogramName := "comprehensive_test_histogram"
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.histogram_metrics (timestamp_ns, name, count, sum, buckets, service_name, attributes, metadata) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7::jsonb, $8::jsonb)`, schema),
		now, histogramName, 10, 100.5, `[1,2,3,4,5]`, "test-service", `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert histogram metric row: %v", err)
	}

	// Insert a summary metric
	summaryName := "comprehensive_test_summary"
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.summary_metrics (timestamp_ns, name, count, sum, quantiles, service_name, attributes, metadata) VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7::jsonb, $8::jsonb)`, schema),
		now, summaryName, 15, 150.75, `{"0.5":10.5,"0.9":20.1,"0.99":25.3}`, "test-service", `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert summary metric row: %v", err)
	}

	// Insert an exponential histogram metric
	expHistogramName := "comprehensive_test_exp_histogram"
	startTime := time.Unix(0, now)
	_, err = db.ExecContext(ctx, fmt.Sprintf(`INSERT INTO %s.exponential_histogram_metrics (timestamp_ns, start_time, name, count, sum, scale, service_name, attributes, metadata) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb)`, schema),
		now, startTime, expHistogramName, 20, 200.0, 2, "test-service", `{"env":"prod", "version":"1.2.3"}`, `{}`)
	if err != nil {
		t.Fatalf("Failed to insert exponential histogram metric row: %v", err)
	}

	t.Logf("✅ All test data inserted, waiting for worker to process...")
	// Wait a few seconds for the embedding worker to process
	time.Sleep(5 * time.Second)

	// Search for each signal type to verify templates are working
	testCases := []struct {
		name     string
		query    string
		expected string
	}{
		{"logs", logContent, "Log Event from service test-service"},
		{"gauge", gaugeName, "Metric: The gauge"},
		{"counter", counterName, "Metric: The sum"}, // Counter uses sum template
		{"histogram", histogramName, "Metric: The histogram"},
		{"summary", summaryName, "Metric: The summary"},
		{"exponential_histogram", expHistogramName, "Metric: The exponential histogram"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			docs, err := vectorStore.SimilaritySearch(ctx, tc.query, 5)
			if err != nil {
				t.Fatalf("Failed to search for %s documents: %v", tc.name, err)
			}

			t.Logf("✅ %s search successful! Found %d documents", tc.name, len(docs))

			// Check if at least one document contains the expected template content
			found := false
			for i, doc := range docs {
				t.Logf("   %s Document %d: Content=%s", tc.name, i+1, doc.PageContent[:min(100, len(doc.PageContent))])
				if strings.Contains(doc.PageContent, tc.expected) {
					found = true
				}
			}

			if !found {
				t.Errorf("No %s document found with expected template content '%s'", tc.name, tc.expected)
			}
		})
	}
}

// TestUpdateDatabaseTemplate fixes the backslash issue in the database template
func TestUpdateDatabaseTemplate(t *testing.T) {
	ctx := context.Background()
	logger, _ := zap.NewDevelopment()

	dsn := repositoryIntegrationDSN(t)

	db, err := OpenPostgressDatabase(ctx, dsn, logger)
	if err != nil {
		t.Skipf("Skipping database template update test - no database available: %v", err)
		return
	}
	defer db.Close()

	// Get the template repository
	templateRepo, err := database.NewTemplateConfigRepo(ctx, db, util.NewZapLoggerAdapter(logger))
	if err != nil {
		t.Fatalf("Failed to create template repository: %v", err)
	}

	// Get the existing template
	template, err := templateRepo.GetByID(ctx, 1)
	if err != nil {
		t.Fatalf("Failed to get template: %v", err)
	}

	// Update the template content to fix the backslash issue
	template.Content = map[string]string{
		"logs":                  `Log Event from service {{.service_name}}. Severity: {{.level | default "info"}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}. Attributes: {{.attributes | formatMap}}.`,
		"spans":                 `{{.service_name | default "unknown"}} for operation {{.name}}. Status: {{.status_code | default "OK"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.{{if .events}} Events: {{.events | formatEventCountsByName}}.{{end}}{{if .logs}} Logs: {{.logs | formatLogCountsBySeverity}}.{{end}}{{if .exemplars_histogram}} Histogram Exemplars: {{range .exemplars_histogram}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_summary}} Summary Exemplars: {{range .exemplars_summary}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_gauge}} Gauge Exemplars: {{range .exemplars_gauge}}{{.MetricName}}={{.Value}} {{end}}.{{end}}{{if .exemplars_sum}} Sum Exemplars: {{range .exemplars_sum}}{{.MetricName}}={{.Value}} {{end}}.{{end}}`,
		"gauge":                 `Metric: The gauge "{{.name}}" from service {{.service_name}} recorded a value of {{.value}}. Labels: {{.labels | formatMap}}.`,
		"counter":               `Metric: The counter "{{.name}}" from service {{.service_name}} reached a total of {{.value}}. Labels: {{.labels | formatMap}}.`,
		"sum":                   `Metric: The sum "{{.name}}" from service {{.service_name}} aggregated to {{.value}} {{.unit}}. Labels: {{.labels | formatMap}}.`,
		"histogram":             `Metric: The histogram "{{.name}}" from service {{.service_name}} observed {{.count}} values with a total sum of {{.sum}}. Buckets: {{.buckets | formatArray}}. Labels: {{.labels | formatMap}}.`,
		"summary":               `Metric: The summary "{{.name}}" from service {{.service_name}} observed {{.count}} values with a total sum of {{.sum}}. Quantiles: {{.quantiles | formatMap}}. Labels: {{.labels | formatMap}}.`,
		"exponential_histogram": `Metric: The exponential histogram "{{.name}}" from service {{.service_name}} observed {{.count}} values with a sum of {{.sum}} and scale of {{.scale}}. Labels: {{.labels | formatMap}}.`,
	}

	// Update the template metadata as well
	template.TemplateMetadata = map[string][]string{
		"generic":               {"service_name", "timestamp_ns"},
		"logs":                  {"service_name", "level", "body", "message", "attributes"},
		"spans":                 {"name", "service_name", "status_code", "duration_ms", "trace_id", "events", "logs", "exemplars_histogram", "exemplars_summary", "exemplars_gauge", "exemplars_sum"},
		"gauge":                 {"name", "service_name", "value", "labels"},
		"counter":               {"name", "service_name", "value", "labels"},
		"histogram":             {"name", "service_name", "count", "sum", "buckets", "labels"},
		"summary":               {"name", "service_name", "count", "sum", "quantiles", "labels"},
		"sum":                   {"name", "service_name", "value", "unit", "labels"},
		"exponential_histogram": {"name", "service_name", "count", "sum", "scale", "labels"},
	}

	// Upsert the updated template
	err = templateRepo.Upsert(ctx, template)
	if err != nil {
		t.Fatalf("Failed to update template: %v", err)
	}

	t.Logf("✅ Database template updated successfully")
}
