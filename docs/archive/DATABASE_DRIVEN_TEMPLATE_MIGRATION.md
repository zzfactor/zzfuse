# Database-Driven Template Migration

## Overview

Successfully replaced the hardcoded template in `FullSpanTransformer` with a database-driven template system, unifying the template architecture across the project.

## Changes Made

### 1. Database Template Configuration

**File**: `pkg/repository/internal/database/config_processor_template.go`

- **Added new template**: `"full_span"` template that matches the original hardcoded format
- **Template content**:
  ```go
  "full_span": `Span: {{.name}} ({{.span_id}})
  Trace: {{.trace_id}}
  Service: {{.service_name}}
  Parent: {{.parent_span_id | default "ROOT"}}
  Kind: {{.span_kind | default "UNSPECIFIED"}}
  Duration: {{.duration_ms | formatDuration}}
  Status: {{.status_code}}{{if .status_message}}
  Message: {{.status_message}}{{end}}{{if .log_count}}
  Logs: {{.logs_summary}}{{end}}{{if .metric_count}}
  Metrics: {{.metrics_summary}}{{end}}{{if .profile_count}}
  Profiles: {{.profiles_summary}}{{end}}`
  ```

- **Added metadata**: Complete field mapping for the full_span template

### 2. Enhanced FullSpanTransformer

**File**: `pkg/repository/processing_transformers.go`

- **Added template engine integration**: `FullSpanTransformer` now uses `ContentTemplateEngine`
- **New constructor**: `NewFullSpanTransformerWithTemplate()` for database-driven templates
- **Template rendering**: Uses database template with fallback to hardcoded format
- **Data preparation**: Properly maps span data to template variables

### 3. Enhanced TransformerFactory

**File**: `pkg/repository/processing_transformers.go`

- **Added template support**: `NewTransformerFactoryWithTemplates()` constructor
- **Conditional template usage**: Uses database templates when available, falls back to defaults
- **Backward compatibility**: Existing code continues to work unchanged

### 4. Updated Embedding Worker

**File**: `pkg/repository/embedding_worker_batch.go`

- **Database template integration**: Uses `TemplateContent` from database when available
- **Fallback mechanism**: Gracefully falls back to default templates if database templates unavailable
- **Logging**: Added debug logging for template usage

### 5. Comprehensive Testing

**File**: `pkg/repository/processing_transformers_test.go`

- **Database template tests**: Verify template rendering with database templates
- **Fallback tests**: Ensure hardcoded format works when database templates unavailable
- **Factory tests**: Verify transformer factory with and without database templates
- **Correlated signals tests**: Test template with logs, metrics, and profiles

## Template Variables

The `full_span` template supports the following variables:

| Variable | Type | Description |
|----------|------|-------------|
| `name` | string | Span name |
| `span_id` | string | Span ID |
| `trace_id` | string | Trace ID |
| `service_name` | string | Service name |
| `parent_span_id` | string | Parent span ID (defaults to "ROOT") |
| `span_kind` | string | Span kind (defaults to "UNSPECIFIED") |
| `duration_ms` | int64 | Duration in milliseconds |
| `status_code` | int | Status code |
| `status_message` | string | Status message (optional) |
| `log_count` | int | Number of correlated logs |
| `metric_count` | int | Number of correlated metrics |
| `profile_count` | int | Number of correlated profiles |
| `logs_summary` | string | Summarized log content |
| `metrics_summary` | string | Summarized metrics content |
| `profiles_summary` | string | Summarized profiles content |

## Benefits

### 1. **Unified Template System**
- Single source of truth for templates in the database
- Consistent template engine across all components
- Reduced code duplication

### 2. **Configurability**
- Templates can be modified without code changes
- Different templates for different environments
- Runtime template updates

### 3. **Backward Compatibility**
- Existing hardcoded format preserved as fallback
- No breaking changes to existing functionality
- Gradual migration path

### 4. **Performance**
- Uses optimized template engine with caching
- Zero-allocation functions where possible
- Efficient template rendering

### 5. **Maintainability**
- Centralized template management
- Consistent error handling
- Comprehensive test coverage

## Usage Examples

### Using Database Templates
```go
// Get template from database
templateRepo := database.NewTemplateConfigRepo(db, logger)
template, err := templateRepo.GetByID(ctx, templateID)

// Create transformer with database template
transformer := NewFullSpanTransformerWithTemplate(template.Content)

// Use in factory
factory := NewTransformerFactoryWithTemplates(template.Content)
transformer, err := factory.CreateTransformer("full_span")
```

### Fallback to Hardcoded Format
```go
// Create transformer without database template
transformer := NewFullSpanTransformer()

// Use in factory
factory := NewTransformerFactory()
transformer, err := factory.CreateTransformer("full_span")
```

## Migration Path

1. **Phase 1**: Database templates are available but optional
2. **Phase 2**: Gradually migrate to database templates
3. **Phase 3**: Remove hardcoded fallback (future)

## Testing

All tests pass:
- ✅ Database template rendering
- ✅ Fallback template rendering  
- ✅ Correlated signals handling
- ✅ Transformer factory with templates
- ✅ Transformer factory without templates

## Future Enhancements

1. **Template Versioning**: Support multiple template versions
2. **Template Validation**: Validate templates at creation time
3. **Template Inheritance**: Support template inheritance and overrides
4. **Performance Monitoring**: Add metrics for template rendering performance
5. **Template Caching**: Implement distributed template caching

## Conclusion

The migration successfully replaces the hardcoded template with a database-driven system while maintaining full backward compatibility. The unified template architecture provides better configurability, maintainability, and performance while preserving all existing functionality. 