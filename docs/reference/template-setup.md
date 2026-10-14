# Embedding Template Setup Guide

## Overview

This guide explains how to configure and use the template system for OpenTelemetry signals (logs, traces, metrics, profiles) in the Trazr OTEL pipeline. The system uses Go text templates to generate searchable text content from your telemetry data.

## 🏗️ **Database Setup**

### 1. Template Configuration Table

The system automatically creates the template configuration table when you start the service:

```sql
-- This table is created automatically in the 'admin' schema
CREATE TABLE admin.config_processor_template (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(255) UNIQUE NOT NULL,
    content JSONB DEFAULT '{}',           -- Template content for each signal type
    template_metadata JSONB DEFAULT '{}', -- Metadata fields to include
    metadata JSONB DEFAULT '{}',
    description TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### 2. Default Template Installation

The system automatically installs a default template (ID=1) with these configurations:

```json
{
  "id": 1,
  "name": "default",
  "content": {
    "logs": "Log Event from service {{.service_name | default \"unknown\"}}. Severity: {{.level | default \"info\"}}. Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}. Attributes: {{.attributes | formatMap}}.",
    "traces": "Trace Span observed in service {{.service_name | default \"unknown\"}} for operation {{.name}}. Status: {{.status_code | default \"OK\"}}. Duration: {{.duration_ms}}ms. TraceID: {{.trace_id}}.",
    "gauge": "Metric: The gauge \"{{.name}}\" from service {{.service_name | default \"unknown\"}} recorded a value of {{.value}}. Labels: {{.labels | formatMap}}.",
    "counter": "Metric: The counter \"{{.name}}\" from service {{.service_name | default \"unknown\"}} reached a total of {{.value}}. Labels: {{.labels | formatMap}}.",
    "sum": "Metric: The sum \"{{.name}}\" from service {{.service_name | default \"unknown\"}} aggregated to {{.value}} {{.unit}}. Labels: {{.labels | formatMap}}.",
    "histogram": "Metric: The histogram \"{{.name}}\" from service {{.service_name | default \"unknown\"}} observed {{.count}} values with a total sum of {{.sum}}. Buckets: {{.buckets | formatArray}}. Labels: {{.labels | formatMap}}.",
    "summary": "Metric: The summary \"{{.name}}\" from service {{.service_name | default \"unknown\"}} observed {{.count}} values with a total sum of {{.sum}}. Quantiles: {{.quantiles | formatMap}}. Labels: {{.labels | formatMap}}.",
    "exponential_histogram": "Metric: The exponential histogram \"{{.name}}\" from service {{.service_name | default \"unknown\"}} observed {{.count}} values with a sum of {{.sum}} and scale of {{.scale}}. Labels: {{.labels | formatMap}}."
  },
  "template_metadata": {
    "generic": ["service_name", "timestamp_ns"],
    "logs": ["level", "event_name"],
    "traces": ["name", "span_kind", "status_code"],
    "gauge": ["name", "unit", "attributes"],
    "counter": ["name", "unit", "attributes"],
    "histogram": ["name", "unit", "attributes"],
    "summary": ["name", "unit", "attributes"],
    "sum": ["name", "unit", "attributes"],
    "exponential_histogram": ["name", "unit", "attributes"]
  }
}
```

## 📝 **Template Configuration**

### 1. Available Fields by Signal Type

#### **Logs Table (`logs`)**
```go
// Top-level fields available in templates:
"id", "timestamp_ns", "timestamp", "observed_timestamp", 
"trace_id", "span_id", "flags", "severity_text", "severity_number", 
"service_name", "body", "dropped_attr_count", "metadata"

// Nested maps:
"resource_attributes" (map), "attributes" (map)

// Scope fields:
"scope_name", "scope_version", "scope_attributes" (map), 
"scope_dropped_attr_count", "scope_schema_url", "resource_schema_url"
```

#### **Traces Table (`traces`)**
```go
// Top-level fields available in templates:
"id", "timestamp_ns", "timestamp", "trace_id", "span_id", 
"parent_span_id", "trace_state", "name", "span_kind", "service_name", 
"status_code", "status_message", "start_time", "end_time", "duration", 
"dropped_attr_count", "dropped_events_count", "dropped_links_count", "metadata"

// Nested maps and arrays:
"resource_attributes" (map), "attributes" (map), 
"events" (array), "links" (array)

// Scope fields:
"scope_name", "scope_version", "scope_attributes" (map), 
"scope_dropped_attr_count", "scope_schema_url", "resource_schema_url"
```

#### **Metrics Tables (Gauge, Sum, Histogram, Summary, Exponential Histogram)**
```go
// Common top-level fields:
"id", "timestamp_ns", "timestamp", "service_name", "name", 
"description", "unit", "value", "count", "sum", "scale", 
"zero_count", "positive_offset", "negative_offset", "min", "max", 
"aggregation_temporality", "is_monotonic", "flags", 
"start_time_ns", "start_time", "dropped_attr_count", "metadata"

// Nested maps and arrays:
"attributes" (map), "resource_attributes" (map)
"positive_bucket_counts" (array/map), "negative_bucket_counts" (array/map), 
"buckets" (array/map), "quantiles" (array/map), "exemplars" (array)

// Scope fields:
"scope_name", "scope_version", "scope_attributes" (map), 
"scope_dropped_attr_count", "scope_schema_url", "resource_schema_url"
```

### 2. Template Syntax Examples

#### **Basic Field Access**
```go
// Direct field access
"{{.service_name}}"
"{{.trace_id}}"
"{{.span_id}}"
"{{.name}}"
"{{.value}}"
```

#### **Nested Field Access**
```go
// Access nested map fields
"{{.attributes.http_method}}"
"{{.resource_attributes.service.name}}"
"{{.scope_attributes.version}}"
```

#### **Conditional Logic**
```go
// Handle missing fields gracefully
"Service: {{if .service_name}}{{.service_name}}{{else}}unknown{{end}}"
"Status: {{.status_code | default \"OK\"}}"
"Message: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}"
```

#### **Template Functions**
```go
// Built-in functions (sprig library)
"{{.attributes | formatMap}}"           // Format map as string
"{{.buckets | formatArray}}"           // Format array as string
"{{.service_name | default \"unknown\"}}" // Provide default value
"{{.timestamp | date \"2006-01-02\"}}" // Format date
```

### 3. Creating Custom Templates

#### **Step 1: Create Template Content**
```sql
-- Insert a custom template
INSERT INTO admin.config_processor_template (
    name, 
    content, 
    template_metadata, 
    description
) VALUES (
    'custom-debug-template',
    '{
        "logs": "DEBUG LOG: Service {{.service_name}} at {{.timestamp | date \"15:04:05\"}} - {{.severity_text | upper}}: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}",
        "traces": "TRACE: {{.name}} ({{.span_id}}) in {{.service_name}} - Status: {{.status_code}}, Duration: {{.duration}}ns",
        "gauge": "GAUGE: {{.name}} = {{.value}} {{.unit}} in {{.service_name}}"
    }'::jsonb,
    '{
        "generic": ["service_name", "timestamp_ns"],
        "logs": ["severity_text", "severity_number"],
        "traces": ["name", "span_id", "status_code", "duration"],
        "gauge": ["name", "value", "unit"]
    }'::jsonb,
    'Custom template for debugging with detailed information'
);
```

#### **Step 2: Update Vector Store Mapping**
```sql
-- Update your vector store mapping to use the custom template
UPDATE admin.config_otel_processor 
SET template_id = (SELECT id FROM admin.config_processor_template WHERE name = 'custom-debug-template')
WHERE name = 'your-vector-store-name';
```

## 🎯 **Best Practices**

### 1. Template Design Principles

#### **Prioritize Important Information**
```go
// Good: Important fields first
"ERROR in {{.service_name}}: {{.name}} - {{.status_message}}"

// Bad: Less important info first
"Timestamp: {{.timestamp}} - ERROR in {{.service_name}}: {{.name}}"
```

#### **Handle Missing Data Gracefully**
```go
// Good: Always provide fallbacks
"Service: {{.service_name | default \"unknown\"}}"
"Status: {{.status_code | default \"UNKNOWN\"}}"

// Bad: May render as "<no value>"
"Service: {{.service_name}}"
```

#### **Use Consistent Formatting**
```go
// Good: Consistent structure across signal types
"{{.signal_type | upper}}: {{.name}} in {{.service_name}} - {{.description}}"

// Bad: Inconsistent formatting
"Log: {{.body}}" vs "Trace {{.trace_id}}" vs "Metric {{.name}}"
```

### 2. Performance Considerations

#### **Keep Templates Concise**
```go
// Good: Focused on key information
"{{.service_name}}: {{.name}} - {{.status_code}}"

// Bad: Too verbose
"Service {{.service_name}} processed operation {{.name}} with status {{.status_code}} and message {{.status_message}} at timestamp {{.timestamp}} with trace ID {{.trace_id}} and span ID {{.span_id}}"
```

#### **Use Efficient Field Access**
```go
// Good: Direct field access
"{{.service_name}}"

// Bad: Nested access when not needed
"{{.attributes.service_name}}"
```

### 3. Search Optimization

#### **Include Searchable Keywords**
```go
// Good: Include operation names, error types, etc.
"{{.name}} operation in {{.service_name}} - {{.status_code}}"

// Bad: Generic descriptions
"Span in service with status"
```

#### **Use Consistent Terminology**
```go
// Good: Consistent naming
"ERROR in {{.service_name}}: {{.name}}"
"WARNING in {{.service_name}}: {{.name}}"

// Bad: Inconsistent terminology
"Error in {{.service_name}}: {{.name}}" vs "Warning in {{.service_name}}: {{.name}}"
```

## 🔧 **Advanced Configuration**

### 1. Multiple Templates for Different Use Cases

```sql
-- Create specialized templates
INSERT INTO admin.config_processor_template (name, content, description) VALUES
('error-focus', '{
    "logs": "ERROR: {{.service_name}} - {{.severity_text}}: {{if .body}}{{.body}}{{else}}{{.message}}{{end}}",
    "traces": "ERROR SPAN: {{.name}} in {{.service_name}} - {{.status_message}}"
}', 'Template focused on error conditions'),

('performance-focus', '{
    "traces": "PERF: {{.name}} in {{.service_name}} - Duration: {{.duration}}ns",
    "gauge": "PERF: {{.name}} = {{.value}} {{.unit}} in {{.service_name}}"
}', 'Template focused on performance metrics'),

('business-focus', '{
    "logs": "BUSINESS: {{.service_name}} - {{if .body}}{{.body}}{{else}}{{.message}}{{end}}",
    "traces": "BUSINESS: {{.name}} operation in {{.service_name}}"
}', 'Template focused on business operations');
```

### 2. Template Versioning

```sql
-- Add version information to metadata
UPDATE admin.config_processor_template 
SET metadata = '{"version": "1.2", "author": "team", "last_updated": "2024-01-15"}'
WHERE name = 'default';
```

### 3. Conditional Templates with Categorization

```sql
-- Create template with conditional logic and categorization
INSERT INTO admin.config_processor_template (name, content) VALUES (
    'smart-template',
    '{
        "logs": "{{if eq .severity_text \"ERROR\"}}ERROR ALERT{{else if eq .severity_text \"WARN\"}}WARNING{{else}}INFO{{end}}: {{.service_name}} - {{if .body}}{{.body}}{{else}}{{.message}}{{end}}",
        "traces": "{{if eq .status_code 1}}SUCCESS{{else}}FAILED{{end}}: {{.name}} in {{.service_name}} - {{.duration}}ns - {{.duration | categorizeDuration}} performance",
        "full_trace": "Trace {{.trace_id}} from {{.service_name}}: Duration {{.duration_ms}}ms ({{.duration_ms | categorizeDuration}}), Spans {{.span_count}} ({{.span_count | categorizeSpanCount}}), Logs {{.log_count}} ({{.log_count | categorizeLogCount}}){{if and .error_count .span_count}}, Error Rate {{.error_count | categorizeErrorRate .span_count}}{{end}}"
    }'
);
```

### 4. Advanced Categorization Examples

```sql
-- Template with comprehensive categorization
INSERT INTO admin.config_processor_template (name, content) VALUES (
    'categorized-template',
    '{
        "traces": "service={{.service_name}} operation={{.name}} status={{.status_code}}{{if .status_message}} error=\"{{.status_message}}\"{{end}}{{if .log_count}} log_volume=\"{{.log_count | categorizeLogCount}}\"{{end}}{{if .duration_ms}} performance=\"{{.duration_ms | categorizeDuration}}\"{{end}}{{range $key, $value := .attributes}} attr_{{$key}}=\"{{$value}}\"{{end}}",
        "full_trace": "Trace {{.trace_id}} from {{.service_name}}: {{.duration_ms | categorizeDuration}} performance ({{.duration_ms}}ms), {{.span_count | categorizeSpanCount}} complexity ({{.span_count}} spans), {{.log_count | categorizeLogCount}} logging ({{.log_count}} logs){{if and .error_count .span_count}}, {{.error_count | categorizeErrorRate .span_count}} error rate{{end}}"
    }'
);
    }'
);
```

## 🚀 **Testing Templates**

### 1. Validate Template Syntax

```go
// Test template rendering
func TestTemplateRendering() {
    template := "Service: {{.service_name}}. Operation: {{.name}}."
    data := map[string]interface{}{
        "service_name": "my-service",
        "name": "test-operation",
    }
    
    // Render template
    rendered := renderTemplate(template, data)
    fmt.Println(rendered) // Output: "Service: my-service. Operation: test-operation."
}
```

### 2. Test with Real Data

```sql
-- Test template with actual signal data
SELECT 
    tpl.content->>'traces' as template,
    sig.service_name,
    sig.name,
    sig.status_code
FROM admin.config_processor_template tpl,
     otel.traces sig
WHERE tpl.name = 'default' 
LIMIT 5;
```

## 📊 **Monitoring Template Performance**

### 1. Template Usage Metrics

```sql
-- Monitor which templates are being used
SELECT 
    tpl.name as template_name,
    COUNT(*) as usage_count,
    AVG(LENGTH(rendered_content)) as avg_content_length
FROM embedding_logs el
JOIN admin.config_processor_template tpl ON el.template_id = tpl.id
GROUP BY tpl.name
ORDER BY usage_count DESC;
```

### 2. Template Effectiveness

```sql
-- Monitor search result quality
SELECT 
    template_name,
    AVG(search_relevance_score) as avg_relevance,
    COUNT(*) as search_count
FROM search_logs
GROUP BY template_name
ORDER BY avg_relevance DESC;
```

## 🔄 **Template Updates**

### 1. Safe Template Updates

```sql
-- Create new version before updating
INSERT INTO admin.config_processor_template (name, content, description)
SELECT 
    name || '-v2', 
    content, 
    description || ' (Updated version)'
FROM admin.config_processor_template 
WHERE name = 'default';

-- Test new template, then update mapping
UPDATE admin.config_otel_processor 
SET template_id = (SELECT id FROM admin.config_processor_template WHERE name = 'default-v2')
WHERE name = 'your-vector-store-name';
```

### 2. Rollback Plan

```sql
-- Keep old template for rollback
-- If issues occur, simply update the mapping back
UPDATE admin.config_otel_processor 
SET template_id = (SELECT id FROM admin.config_processor_template WHERE name = 'default')
WHERE name = 'your-vector-store-name';
```

---

## 📚 **Additional Resources**

- **Template Field Reference**: See `template_embedding.md` for complete field listings
- **Sprig Functions**: [Sprig Template Functions](http://masterminds.github.io/sprig/) for advanced template features
- **Go Template Syntax**: [Go Template Documentation](https://golang.org/pkg/text/template/) for syntax reference
- **Architecture Guide**: See `ARCHITECTURE_EMBEDDING_CONTENT.md` for system design details

This guide should help you configure effective embedding templates for your OpenTelemetry signals. Start with the default template and customize based on your specific use cases and search requirements. 