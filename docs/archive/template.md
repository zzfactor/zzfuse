# Template Engine Integration Design

## Overview

This document outlines the design for integrating template processing during OpenTelemetry data insertion while maintaining minimal impact on existing business logic and making the template engine self-contained for future extraction as a standalone package.

## Design Principles

1. **Minimal Impact**: Existing code paths remain unchanged
2. **Self-Contained**: Template engine is independent and can be extracted as a package
3. **Optional Integration**: Template processing is opt-in, not mandatory
4. **Performance First**: Zero-allocation operations and efficient processing
5. **Backward Compatibility**: All existing functionality continues to work
6. **Dual-Mode Processing**: Support both pre-insert and worker-based template processing

## Architecture Overview

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   HTTP Handler  │───▶│  Database Layer │───▶│  Template       │
│   (Extension)   │    │  (Repository)   │    │  Engine         │
│   (No Template  │    │  (Transparent)  │    │  (Package)      │
│    Logic)       │    └─────────────────┘    └─────────────────┘
└─────────────────┘             │                       │
         │                      ▼                       │
         │              ┌─────────────────┐             │
         │              │  Check Config   │             │
         │              │  config_otel_   │             │
         │              │  processor      │             │
         │              └─────────────────┘             │
         │                      │                       │
         │                      ▼                       │
         │              ┌─────────────────┐             │
         │              │  Process        │             │
         │              │  Templates      │             │
         │              │  (if enabled)   │             │
         │              └─────────────────┘             │
         │                      │                       │
         └──────────────────────┼───────────────────────┘
                                ▼
                   ┌─────────────────────────┐
                   │   Existing Metadata     │
                   │   JSONB Column          │
                   │   (Enhanced with        │
                   │    template_processing) │
                   └─────────────────────────┘
```

**Key Design Principle**: Template processing is handled transparently in the database layer. Clients (HTTP handlers, exporters) call regular insert methods and template processing happens automatically based on database configuration.

**Key Design Principle**: All template processing results are stored in the existing `metadata` JSONB column without any database schema changes.

## Core Components

### 1. Template Engine Package (`pkg/template/`)

A self-contained package that handles all template processing logic and database configuration reading:

```go
// pkg/template/engine.go
package template

import (
    "context"
    "time"
)

// Engine provides template processing capabilities
type Engine struct {
    contentEngine *content.ContentTemplateEngine
    config        *Config
    metrics       *Metrics
}

// Config defines template processing configuration
type Config struct {
    ProcessorConfigID int64  `json:"processor_config_id"` // One-to-one with template_id
    EnablePreInsert   bool   `json:"enable_pre_insert"`   // Process during insertion
    EnableWorkerMode  bool   `json:"enable_worker_mode"`  // Process during embedding
    StoreResults      bool   `json:"store_results"`
    PerformanceMode   string `json:"performance_mode"`    // "fast", "balanced", "complete"
}

// ProcessorConfig represents the config_otel_processor table structure
type ProcessorConfig struct {
    ID                  int64             `json:"id"`
    Name                string            `json:"name"`
    DatabaseID          int64             `json:"database_id"`
    VectorStoreID       *int64            `json:"vectorstore_id"`
    ProcessorTemplateID int64             `json:"template_id"` // One-to-one with template
    ProcessingMode      string            `json:"processing_mode"`       // 'logs', 'spans', 'metrics', 'all'
    Enabled             bool              `json:"enabled"`
    Routing             map[string]interface{} `json:"routing"`
    Filter              map[string]interface{} `json:"filter"`
    Metadata            map[string]interface{} `json:"metadata"`
    CreatedAt           time.Time         `json:"created_at"`
    UpdatedAt           time.Time         `json:"updated_at"`
}

// ProcessorTemplate represents the config_processor_template table structure
type ProcessorTemplate struct {
    ID              int64             `json:"id"`
    Name            string            `json:"name"`
    Content         map[string]interface{} `json:"content"`         // Template content for each signal type
    TemplateMetadata map[string]interface{} `json:"template_metadata"` // Template metadata for each signal type
    Metadata        map[string]interface{} `json:"metadata"`
    Description     string            `json:"description"`
    CreatedAt       time.Time         `json:"created_at"`
    UpdatedAt       time.Time         `json:"updated_at"`
}

// ProcessingResult contains template processing output
type ProcessingResult struct {
    Content       string                 `json:"content"`
    Metadata      map[string]interface{} `json:"metadata"`
    TemplateID    int64                  `json:"template_id"`
    ProcessedAt   time.Time              `json:"processed_at"`
    Mode          string                 `json:"mode"` // "pre_insert", "worker_mode"
    Error         string                 `json:"error,omitempty"`
}

// ProcessSignal processes a signal with a shared template (many-to-one relationship)
func (e *Engine) ProcessSignal(
    ctx context.Context,
    signalType string,
    signalData map[string]interface{},
    processorConfigID int64,
) (*ProcessingResult, error) {
    // Get processor configuration
    processorConfig, err := e.getProcessorConfig(ctx, processorConfigID)
    if err != nil {
        return nil, fmt.Errorf("failed to get processor config: %w", err)
    }
    
    // Get shared processor template (many-to-one relationship)
    processorTemplate, err := e.getProcessorTemplate(ctx, processorConfig.ProcessorTemplateID)
    if err != nil {
        return nil, fmt.Errorf("failed to get processor template: %w", err)
    }
    
    // Process content template
    var content string
    if contentTemplate, ok := processorTemplate.Content[signalType]; ok {
        content, err = e.renderContentTemplate(ctx, signalType, signalData, contentTemplate)
        if err != nil {
            return nil, fmt.Errorf("content template rendering failed: %w", err)
        }
    }
    
    // Process metadata template
    var metadata map[string]interface{}
    if metadataTemplate, ok := processorTemplate.TemplateMetadata[signalType]; ok {
        metadata = e.extractMetadataFromSignal(signalData, metadataTemplate)
    }
    
    return &ProcessingResult{
        Content:     content,
        Metadata:    metadata,
        TemplateID:  processorTemplate.ID,
        ProcessedAt: time.Now(),
        Mode:        "pre_insert",
    }, nil
}
```

### 2. Database Layer Integration

The database layer (repository) transparently handles template processing:

```go
// pkg/repository/otel_logs_repository.go
func (r *logRepository) InsertLogsWithTemplates(ctx context.Context, entries []LogEntry, schema string, config *template.Config) ([]string, error) {
    // ✅ NEW: Use template package to check if template processing should be enabled
    processorConfigReader := template.NewProcessorConfigReader(r.db)
    templateConfig, err := processorConfigReader.ShouldEnableTemplateProcessing(ctx, schema)
    if err != nil {
        return nil, fmt.Errorf("failed to check template processing: %w", err)
    }

    // If template processing is enabled, process templates in memory
    if templateConfig != nil && templateConfig.EnablePreInsert {
        // Create template engine for processing
        templateEngine := template.NewEngine()
        
        // Process each entry with templates
        processedEntries := make([]LogEntry, len(entries))
        copy(processedEntries, entries)

        for i := range processedEntries {
            // Convert LogEntry to template-compatible data structure
            signalData := r.convertLogEntryToTemplateData(&processedEntries[i])

            // Process template using template engine
            result, err := templateEngine.ProcessSignal(ctx, "logs", signalData, templateConfig.ProcessorConfigID)
            if err != nil {
                // Log error but continue processing other entries
                continue
            }

            // Store template results in metadata
            r.storeTemplateResultsInMetadata(&processedEntries[i], result)
        }

        // Insert the processed entries
        return r.InsertLogs(ctx, processedEntries, schema)
    }

    // Fall back to regular insert (no template processing)
    return r.InsertLogs(ctx, entries, schema)
}
```

### 3. Client Integration (Extension/Exporter)

Clients use regular insert methods - template processing is transparent:

```go
// Extension HTTP Handler (No Template Logic)
func NewLogInsertHandler(logWriter repo.LogWriterService) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // ... parse request body ...
        
        // ✅ Simple: Just call regular insert method
        // Template processing is handled transparently in the database layer
        ids, err := logWriter.InsertLogs(r.Context(), entries, schema)
        if err != nil {
            // ... handle error ...
        }
        
        // ... return response ...
    }
}

// Exporter (No Template Logic)
func (e *logsExporter) pushLogsData(ctx context.Context, ld plog.Logs) error {
    // ... process log records ...
    
    for schemaName, entries := range groups {
        // ✅ Simple: Just call regular insert method
        // Template processing is handled transparently in the database layer
        _, err := e.logWriter.InsertLogs(ctx, entries, schemaName)
        if err != nil {
            return fmt.Errorf("insert logs (schema=%s): %w", schemaName, err)
        }
    }
    return nil
}
```

**Key Benefits:**
- ✅ **No template logic in clients** - Extension and exporter remain simple
- ✅ **Transparent processing** - Template processing happens automatically
- ✅ **Database-driven configuration** - Controlled by `config_otel_processor` table
- ✅ **Backward compatibility** - Existing code continues to work unchanged
```

## Implementation Example: Log Insertion

### Database-Driven Template Processing Flow

**Key Concept**: Template processing is handled transparently in the database layer based on `config_otel_processor` configuration.

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   HTTP Request  │───▶│  Database Layer │───▶│  Check Config   │───▶│  Template       │
│   (Extension)   │    │  (Repository)   │    │  config_otel_   │    │  Engine         │
│   (No Template  │    │  (Transparent)  │    │  processor      │    │  (Package)      │
│    Logic)       │    └─────────────────┘    └─────────────────┘    └─────────────────┘
└─────────────────┘             │                       │                       │
         │                      ▼                       ▼                       │
         │              ┌─────────────────┐    ┌─────────────────┐             │
         │              │  Process        │    │  Store Results  │             │
         │              │  Templates      │    │  in Metadata    │             │
         │              │  (if enabled)   │    │  (JSONB)        │             │
         │              └─────────────────┘    └─────────────────┘             │
         │                      │                       │                       │
         └──────────────────────┼───────────────────────┼───────────────────────┘
                                ▼                       ▼
                   ┌─────────────────────────┐    ┌─────────────────────────┐
                   │   Regular InsertLogs    │    │   Enhanced Metadata     │
                   │   (Unchanged)           │    │   with template_processing│
                   └─────────────────────────┘    └─────────────────────────┘
```

### Current Log Insertion Flow (Unchanged)

```go
// Current implementation remains exactly the same
func (r *logRepository) InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error) {
    // Existing implementation - NO CHANGES
    if len(entries) == 0 {
        return nil, nil
    }
    
    const maxBatch = 500
    var insertedIDs []string
    
    for start := 0; start < len(entries); start += maxBatch {
        // ... existing batch processing logic ...
    }
    
    return insertedIDs, nil
}
```

### In-Memory Processing Benefits

1. **🚀 Zero Database Overhead**: Template processing happens entirely in memory (with cached configurations)
2. **⚡ Maximum Performance**: No additional database queries during insertion
3. **🔄 Atomic Operations**: Template processing and database insertion happen together
4. **💾 Memory Efficient**: Uses existing data structures without duplication
5. **🛡️ Error Isolation**: Template processing errors don't affect database operations
6. **🔄 Template Sharing**: Multiple processors efficiently share the same template instances

### In-Memory Processing Steps

1. **Data Preparation**: Convert LogEntry to template-compatible format in memory
2. **Template Processing**: Apply template engine to generate content and metadata
3. **Metadata Enhancement**: Store template results in existing metadata field
4. **Database Insertion**: Insert enhanced entries using existing insert logic

### Performance Characteristics

- **Template Processing**: ~1-5ms per entry (depending on template complexity)
- **Memory Usage**: Minimal overhead (only metadata enhancement)
- **Database Impact**: Zero additional queries during insertion (with proper caching)
- **Scalability**: Linear scaling with entry count

### Performance Monitoring & Optimization

**Key Performance Metrics to Monitor**:
```go
// Performance metrics for template processing
type TemplatePerformanceMetrics struct {
    ProcessingLatency    time.Duration
    CacheHitRate        float64
    JSONUnmarshalTime   time.Duration
    MemoryAllocations   int64
    DatabaseQueries     int64
    TemplateCompileTime time.Duration
}

// Monitor these metrics to identify bottlenecks
func (e *Engine) recordPerformanceMetrics(metrics TemplatePerformanceMetrics) {
    // Record metrics for monitoring and alerting
    e.metrics.ProcessingLatency.Observe(metrics.ProcessingLatency.Seconds())
    e.metrics.CacheHitRate.Set(metrics.CacheHitRate)
    e.metrics.JSONUnmarshalTime.Observe(metrics.JSONUnmarshalTime.Seconds())
}
```

**Performance Optimization Checklist**:
1. **Configuration Caching**: Ensure processor configs and templates are cached
2. **JSON Parsing**: Monitor JSON unmarshalling performance, consider optimized libraries
3. **Memory Allocation**: Profile memory allocations during template processing
4. **Template Compilation**: Cache compiled templates to avoid re-compilation
5. **Database Queries**: Ensure zero database queries during insertion phase

### Enhanced Log Insertion with In-Memory Template Processing

```go
// pkg/repository/otel_logs_repository.go

// ✅ NEW: Template-aware insert method with in-memory processing
func (r *logRepository) InsertLogsWithTemplates(
    ctx context.Context,
    entries []LogEntry,
    schema string,
    templateConfig *template.Config,
) ([]string, error) {
    
    // ✅ IN-MEMORY PRE-INSERT PROCESSING
    if templateConfig != nil && templateConfig.EnablePreInsert {
        // Process templates in memory BEFORE database insertion
        processedEntries := r.processTemplatesInMemory(ctx, entries, templateConfig)
        
        // Insert the processed entries (with template results in metadata)
        return r.InsertLogs(ctx, processedEntries, schema)
    }
    
    // Fall back to original insert (no template processing)
    return r.InsertLogs(ctx, entries, schema)
}

// ✅ NEW: In-memory template processing (one-to-one relationship)
func (r *logRepository) processTemplatesInMemory(
    ctx context.Context,
    entries []LogEntry,
    config *template.Config,
) []LogEntry {
    
    // Create a copy of entries to avoid modifying originals
    processedEntries := make([]LogEntry, len(entries))
    copy(processedEntries, entries)
    
    // Process each entry in memory
    for i := range processedEntries {
        // Convert LogEntry to template-compatible data structure
        signalData := r.convertLogEntryToTemplateData(&processedEntries[i])
        
        // Process template in memory (no database queries)
        result, err := r.templateEngine.ProcessSignal(ctx, "logs", signalData, config.ProcessorConfigID)
        if err != nil {
            // Log error but continue processing other entries
            r.logger.Error("template processing failed", 
                "error", err, 
                "entry_id", processedEntries[i].ID,
                "config_id", config.ProcessorConfigID)
            continue
        }
        
        // Store template results in metadata (in-memory modification)
        r.storeTemplateResultsInMemory(&processedEntries[i], result)
    }
    
    return processedEntries
}

// ✅ NEW: Convert LogEntry to template data (in-memory)
func (r *logRepository) convertLogEntryToTemplateData(entry *LogEntry) map[string]interface{} {
    signalData := map[string]interface{}{
        "id":                    entry.ID,
        "timestamp_ns":          entry.TimestampNS,
        "service_name":          entry.ServiceName,
        "severity_text":         entry.SeverityText,
        "severity_number":       entry.SeverityNumber,
        "trace_id":              entry.TraceID,
        "span_id":               entry.SpanID,
        "body":                  string(entry.Body),
    }
    
    // Parse JSONB fields in memory
    // Note: json.Unmarshal can be a performance bottleneck under high throughput
    // Consider using specialized JSON libraries (e.g., fastjson, jsoniter) if profiling reveals this as a hot spot
    if len(entry.ResourceAttributes) > 0 {
        var resourceAttrs map[string]interface{}
        if err := json.Unmarshal(entry.ResourceAttributes, &resourceAttrs); err == nil {
            signalData["resource_attributes"] = resourceAttrs
        }
    }
    
    if len(entry.Attributes) > 0 {
        var attrs map[string]interface{}
        if err := json.Unmarshal(entry.Attributes, &attrs); err == nil {
            signalData["attributes"] = attrs
        }
    }
    
    return signalData
}

// ✅ NEW: Store template results in memory (no database writes)
func (r *logRepository) storeTemplateResultsInMemory(entry *LogEntry, result *template.ProcessingResult) {
    // Parse existing metadata in memory
    existingMetadata := make(map[string]interface{})
    if len(entry.Metadata) > 0 {
        if err := json.Unmarshal(entry.Metadata, &existingMetadata); err == nil {
            // Add template processing results to existing metadata
            existingMetadata["template_processing"] = map[string]interface{}{
                "content":        result.Content,
                "metadata":       result.Metadata,
                "template_id":    result.TemplateID,
                "processed_at":   result.ProcessedAt,
                "mode":          "pre_insert",
            }
            
            // Update entry metadata in memory
            if metadataBytes, err := json.Marshal(existingMetadata); err == nil {
                entry.Metadata = metadataBytes
            }
        }
    } else {
        // Create new metadata with template results
        newMetadata := map[string]interface{}{
            "template_processing": map[string]interface{}{
                "content":        result.Content,
                "metadata":       result.Metadata,
                "template_id":    result.TemplateID,
                "processed_at":   result.ProcessedAt,
                "mode":          "pre_insert",
            },
        }
        
        if metadataBytes, err := json.Marshal(newMetadata); err == nil {
            entry.Metadata = metadataBytes
        }
    }
}

// ✅ NEW: Template engine field (optional)
type logRepository struct {
    db             *sql.DB
    templateEngine *template.Engine  // Optional template engine
}
```

### HTTP Handler Enhancement (Minimal Changes)

```go
// trazrextension/internal/otel_insert.go

// ✅ NEW: Enhanced handler with optional template processing
func NewLogInsertHandlerWithTemplates(
    logWriter repo.LogWriterService,
    templateEngine *template.Engine,
) http.HandlerFunc {
    
    return func(w http.ResponseWriter, r *http.Request) {
        schema := r.URL.Query().Get("schema")
        if schema == "" {
            schema = "otel"
        }
        
        // ✅ NEW: Parse template configuration from query params
        templateConfig := parseTemplateConfigFromQuery(r.URL.Query())
        
        var dtos []LogEntryDTO
        // ... existing DTO parsing logic (unchanged) ...
        
        var entries []repo.LogEntry
        for _, dto := range dtos {
            entry, err := dto.ToLogEntry()
            if err != nil {
                writeInsertError(w, http.StatusBadRequest, err.Error())
                return
            }
            entries = append(entries, entry)
        }
        
        // ✅ NEW: Choose insert method based on template configuration
        var ids []string
        var err error
        
        if templateConfig != nil && templateConfig.EnablePreInsert {
            // Use template-aware insert
            ids, err = logWriter.InsertLogsWithTemplates(r.Context(), entries, schema, templateConfig)
        } else {
            // Use original insert (unchanged)
            ids, err = logWriter.InsertLogs(r.Context(), entries, schema)
        }
        
        if err != nil {
            writeInsertError(w, http.StatusInternalServerError, err.Error())
            return
        }
        
        writeInsertResult(w, ids)
    }
}

// ✅ NEW: Helper function to parse template config from query params
func parseTemplateConfigFromQuery(query url.Values) *template.Config {
    configIDStr := query.Get("processor_config_id")
    if configIDStr == "" {
        return nil // No template processing
    }
    
    configID, err := strconv.ParseInt(configIDStr, 10, 64)
    if err != nil {
        return nil
    }
    
    return &template.Config{
        ProcessorConfigID: configID,
        EnablePreInsert:   query.Get("enable_pre_insert") == "true",
        EnableWorkerMode:  query.Get("enable_worker_mode") == "true",
        StoreResults:      query.Get("store_results") != "false", // Default true
        PerformanceMode:   query.Get("performance_mode"),          // Default "balanced"
    }
}
```

## Template Data Storage Strategy

### Using Existing Metadata JSONB Column

**Key Principle**: Store all template processing results in the existing `metadata` JSONB column without creating any new database columns.

```go
// pkg/template/storage.go
package template

import (
    "encoding/json"
    "time"
)

// storeTemplateResults stores template results in existing metadata JSONB field
func (pm *ProcessingMiddleware) storeTemplateResults(entry *LogEntry, result *ProcessingResult) {
    // Parse existing metadata (preserve all existing data)
    existingMetadata := make(map[string]interface{})
    if len(entry.Metadata) > 0 {
        if err := json.Unmarshal(entry.Metadata, &existingMetadata); err == nil {
            // Add template processing results to existing metadata
            existingMetadata["template_processing"] = map[string]interface{}{
                "content":        result.Content,
                "metadata":       result.Metadata,
                "template_id":    result.TemplateID,
                "processed_at":   result.ProcessedAt,
                "mode":          result.Mode,
            }
            
            // Update entry metadata with combined data
            if metadataBytes, err := json.Marshal(existingMetadata); err == nil {
                entry.Metadata = metadataBytes
            }
        }
    } else {
        // Create new metadata with only template results
        newMetadata := map[string]interface{}{
            "template_processing": map[string]interface{}{
                "content":        result.Content,
                "metadata":       result.Metadata,
                "template_id":    result.TemplateID,
                "processed_at":   result.ProcessedAt,
                "mode":          result.Mode,
            },
        }
        
        if metadataBytes, err := json.Marshal(newMetadata); err == nil {
            entry.Metadata = metadataBytes
        }
    }
}

// retrieveTemplateResults extracts template results from metadata
func retrieveTemplateResults(metadata []byte) (*ProcessingResult, error) {
    if len(metadata) == 0 {
        return nil, nil
    }
    
    var metadataMap map[string]interface{}
    if err := json.Unmarshal(metadata, &metadataMap); err != nil {
        return nil, err
    }
    
    templateData, exists := metadataMap["template_processing"]
    if !exists {
        return nil, nil
    }
    
    // Convert back to ProcessingResult
    templateBytes, err := json.Marshal(templateData)
    if err != nil {
        return nil, err
    }
    
    var result ProcessingResult
    if err := json.Unmarshal(templateBytes, &result); err != nil {
        return nil, err
    }
    
    return &result, nil
}
```

### Metadata Structure Examples

**Before Template Processing:**
```json
{
  "service.name": "my-service",
  "environment": "production",
  "version": "1.0.0"
}
```

**After Template Processing:**
```json
{
  "service.name": "my-service",
  "environment": "production", 
  "version": "1.0.0",
  "template_processing": {
    "content": "Log Event from service my-service. Severity: info. Message: test log. Attributes: none.",
    "metadata": {
      "service_name": "my-service",
      "level": "info",
      "body": "test log"
    },
    "template_id": 123,
    "processed_at": "2025-01-02T15:04:05Z",
    "mode": "pre_insert"
  }
}
```

### Benefits of Using Existing Metadata Column

1. **No Schema Changes**: Zero database migrations required
2. **Backward Compatibility**: Existing queries continue to work
3. **Flexible Storage**: JSONB allows nested structures and dynamic fields
4. **Query Capability**: Can query template results using JSONB operators
5. **Space Efficient**: No additional storage overhead for unused columns

### Many-to-One Relationship Design

**Current Design**: Many-to-one relationship between `ProcessorConfig` and `ProcessorTemplate`

**Benefits**:
- **Template Reuse**: Multiple processor configurations can share the same template
- **Reduced Duplication**: Avoid creating identical templates for different processors
- **Centralized Management**: Template changes affect all processors using it
- **Flexibility**: Different processors can use the same template with different settings
- **Efficient Resource Usage**: Single template instance serves multiple processors

**Implementation**:
```go
// ProcessorConfig with shared template support
type ProcessorConfig struct {
    ID                  int64             `json:"id"`
    Name                string            `json:"name"`
    DatabaseID          int64             `json:"database_id"`
    VectorStoreID       *int64            `json:"vectorstore_id"`
    ProcessorTemplateID int64             `json:"template_id"` // Shared across multiple processors
    ProcessingMode      string            `json:"processing_mode"`
    Enabled             bool              `json:"enabled"`
    TemplateOverrides   map[string]interface{} `json:"template_overrides"` // Optional per-processor overrides
    Routing             map[string]interface{} `json:"routing"`
    Filter              map[string]interface{} `json:"filter"`
    Metadata            map[string]interface{} `json:"metadata"`
    CreatedAt           time.Time         `json:"created_at"`
    UpdatedAt           time.Time         `json:"updated_at"`
}
```

**Use Cases**:
- Multiple log processors using the same template for different environments
- Different signal types (logs, spans, metrics) sharing common template patterns
- Regional processors with identical templates but different configurations

### Querying Template Results

Since template results are stored in the existing metadata JSONB column, you can query them using PostgreSQL JSONB operators:

```sql
-- Find all logs with template processing
SELECT * FROM logs 
WHERE metadata ? 'template_processing';

-- Find logs processed with specific template ID
SELECT * FROM logs 
WHERE metadata->'template_processing'->>'template_id' = '123';

-- Find logs with template processing in pre_insert mode
SELECT * FROM logs 
WHERE metadata->'template_processing'->>'mode' = 'pre_insert';

-- Find logs with specific template content
SELECT * FROM logs 
WHERE metadata->'template_processing'->>'content' ILIKE '%error%';

-- Find logs with template metadata containing specific service
SELECT * FROM logs 
WHERE metadata->'template_processing'->'metadata'->>'service_name' = 'my-service';
```

### ProcessorConfig and ProcessorTemplate Examples

**Example 1: Processor Configuration**
```json
{
  "id": 123,
  "name": "logs-processor",
  "database_id": 1,
  "vectorstore_id": 1,
  "processor_template_id": 456,
  "processing_mode": "logs",
  "enabled": true,
  "routing": {},
  "filter": {},
  "metadata": {}
}
```

**Example 2: Processor Template with Content and Metadata**
```json
{
  "id": 456,
  "name": "logs-template",
  "content": {
    "logs": "Log Event from service {{.service_name}}. Severity: {{.level}}. Message: {{.body}}."
  },
  "template_metadata": {
    "logs": ["service_name", "level", "body", "attributes"]
  },
  "metadata": {},
  "description": "Template for log processing"
}
```

**Example 3: Complete Template Processing Flow (Many-to-One)**
```json
// Multiple ProcessorConfigs sharing the same template
// ProcessorConfig 1 (config_otel_processor table)
{
  "id": 123,
  "name": "production-logs-processor",
  "processor_template_id": 456,
  "processing_mode": "logs"
}

// ProcessorConfig 2 (config_otel_processor table)
{
  "id": 124,
  "name": "staging-logs-processor", 
  "processor_template_id": 456,  // Same template ID
  "processing_mode": "logs"
}

// Shared ProcessorTemplate (config_embedding_template table)
{
  "id": 456,
  "name": "standard-logs-template",
  "content": {
    "logs": "Log Event from service {{.service_name}}. Severity: {{.level}}."
  },
  "template_metadata": {
    "logs": ["service_name", "level"]
  }
}

// Result in metadata JSONB (same for both processors)
{
  "template_processing": {
    "content": "Log Event from service my-service. Severity: info.",
    "metadata": {
      "service_name": "my-service",
      "level": "info"
    },
    "template_id": 456,
    "processed_at": "2025-01-02T15:04:05Z",
    "mode": "pre_insert"
  }
}
```

## Template Data Preparation

### Signal Type-Specific Data Conversion

```go
// pkg/template/data_preparation.go
package template

import (
    "encoding/json"
    "time"
)

// convertLogEntryToSignalData converts LogEntry to template-compatible format
func (pm *ProcessingMiddleware) convertLogEntryToSignalData(entry *LogEntry) map[string]interface{} {
    signalData := map[string]interface{}{
        "id":                    entry.ID,
        "timestamp_ns":          entry.TimestampNS,
        "service_name":          entry.ServiceName,
        "severity_text":         entry.SeverityText,
        "severity_number":       entry.SeverityNumber,
        "trace_id":              entry.TraceID,
        "span_id":               entry.SpanID,
        "body":                  string(entry.Body),
    }
    
    // Parse JSONB fields
    if len(entry.ResourceAttributes) > 0 {
        var resourceAttrs map[string]interface{}
        if err := json.Unmarshal(entry.ResourceAttributes, &resourceAttrs); err == nil {
            signalData["resource_attributes"] = resourceAttrs
        }
    }
    
    if len(entry.Attributes) > 0 {
        var attrs map[string]interface{}
        if err := json.Unmarshal(entry.Attributes, &attrs); err == nil {
            signalData["attributes"] = attrs
        }
    }
    
    // Extract service name from resource attributes if not set
    if signalData["service_name"] == "" {
        if resourceAttrs, ok := signalData["resource_attributes"].(map[string]interface{}); ok {
            if serviceName, ok := resourceAttrs["service.name"].(string); ok {
                signalData["service_name"] = serviceName
            }
        }
    }
    
    return signalData
}
```

## Configuration and Integration

### Repository Interface Enhancement

```go
// pkg/repository/otel_logs_repository.go

// ✅ NEW: Enhanced interface with optional template support
type LogRepository interface {
    // Existing methods (unchanged)
    QueryLogs(ctx context.Context, q LogQuery) ([]LogEntry, string, bool, error)
    InsertLogs(ctx context.Context, entries []LogEntry, schema string) ([]string, error)
    
    // ✅ NEW: Optional template-aware insert
    InsertLogsWithTemplates(ctx context.Context, entries []LogEntry, schema string, config *template.Config) ([]string, error)
}

// ✅ NEW: Repository factory with optional template engine
func NewLogRepository(db *sql.DB, templateEngine *template.Engine) LogRepository {
    return &logRepository{
        db:             db,
        templateEngine: templateEngine, // Can be nil
    }
}
```

### Service Layer Integration

```go
// pkg/repository/otel_logs_service.go

// ✅ NEW: Enhanced service with template support
type LogService struct {
    repo           LogRepository
    templateEngine *template.Engine // Optional
}

func NewLogService(repo LogRepository, templateEngine *template.Engine) *LogService {
    return &LogService{
        repo:           repo,
        templateEngine: templateEngine,
    }
}

// ✅ NEW: Template-aware insert method
func (s *LogService) InsertLogsWithTemplates(
    ctx context.Context,
    entries []LogEntry,
    schema string,
    templateConfig *template.Config,
) ([]string, error) {
    
    if templateConfig != nil && s.templateEngine != nil {
        return s.repo.InsertLogsWithTemplates(ctx, entries, schema, templateConfig)
    }
    
    // Fall back to original insert
    return s.repo.InsertLogs(ctx, entries, schema)
}
```

## Performance Optimizations

### Template Engine Pooling

```go
// pkg/template/pool.go
package template

import (
    "sync"
)

// EnginePool provides efficient template engine reuse
type EnginePool struct {
    pool sync.Pool
}

func NewEnginePool() *EnginePool {
    return &EnginePool{
        pool: sync.Pool{
            New: func() interface{} {
                return NewEngine()
            },
        },
    }
}

func (ep *EnginePool) Get() *Engine {
    return ep.pool.Get().(*Engine)
}

func (ep *EnginePool) Put(engine *Engine) {
    ep.pool.Put(engine)
}
```

### Batch Processing

```go
// pkg/template/batch.go
package template

import (
    "context"
    "runtime"
    "sync"
)

// BatchProcessor processes multiple signals efficiently
type BatchProcessor struct {
    engine *Engine
    pool   *EnginePool
}

func (bp *BatchProcessor) ProcessBatch(
    ctx context.Context,
    signals []map[string]interface{},
    signalType string,
) ([]*ProcessingResult, error) {
    
    results := make([]*ProcessingResult, len(signals))
    workerCount := runtime.NumCPU()
    semaphore := make(chan struct{}, workerCount)
    
    var wg sync.WaitGroup
    for i, signal := range signals {
        wg.Add(1)
        go func(index int, sig map[string]interface{}) {
            defer wg.Done()
            semaphore <- struct{}{}
            defer func() { <-semaphore }()
            
            engine := bp.pool.Get()
            defer bp.pool.Put(engine)
            
            result, err := engine.ProcessSignal(ctx, signalType, sig)
            if err != nil {
                results[index] = &ProcessingResult{Error: err.Error()}
            } else {
                results[index] = result
            }
        }(i, signal)
    }
    
    wg.Wait()
    return results, nil
}
```

## Dual-Mode Template Processing Strategy

### Mode 1: Pre-Insert Processing (Current Implementation)
**When**: During HTTP request processing (insertion time)
**How**: Process templates in memory before database insertion
**Storage**: Template results stored in metadata during insertion
**Use Case**: Real-time template processing for immediate availability

```go
// Pre-insert flow
if templateConfig.EnablePreInsert {
    // Process templates in memory
    processedEntries := processTemplatesInMemory(entries, templateConfig)
    // Insert with template results in metadata
    return insertLogs(processedEntries)
}
```

### Mode 2: Worker Mode Processing (Future Implementation)
**When**: During embedding worker execution (similar to current embedded_id check)
**How**: Check metadata for existing template processing, process if missing
**Storage**: Template results stored in metadata during worker execution
**Use Case**: Batch processing, retroactive template application

```go
// Worker mode flow (future)
func (w *EmbeddingWorker) processSignalWithTemplates(signal map[string]interface{}) {
    // Check if template already processed
    if templateResult := getTemplateResultFromMetadata(signal); templateResult != nil {
        // Use existing template results
        return templateResult
    }
    
    // Process template from database data
    templateResult := w.templateEngine.ProcessSignal(ctx, signalType, signal)
    
    // Store template results in metadata
    storeTemplateResultsInMetadata(signal, templateResult)
    
    return templateResult
}
```

### Template Processing Check Logic (Worker Mode)

```go
// Check if template has been processed
func hasTemplateProcessing(metadata []byte, templateID int64) bool {
    var metadataMap map[string]interface{}
    if err := json.Unmarshal(metadata, &metadataMap); err != nil {
        return false
    }
    
    templateData, exists := metadataMap["template_processing"]
    if !exists {
        return false
    }
    
    templateMap, ok := templateData.(map[string]interface{})
    if !ok {
        return false
    }
    
    storedTemplateID, ok := templateMap["template_id"].(float64) // JSON numbers are float64
    return ok && int64(storedTemplateID) == templateID
}

// Get existing template results
func getTemplateResultFromMetadata(metadata []byte) *ProcessingResult {
    var metadataMap map[string]interface{}
    if err := json.Unmarshal(metadata, &metadataMap); err != nil {
        return nil
    }
    
    templateData, exists := metadataMap["template_processing"]
    if !exists {
        return nil
    }
    
    // Convert back to ProcessingResult
    templateBytes, err := json.Marshal(templateData)
    if err != nil {
        return nil
    }
    
    var result ProcessingResult
    if err := json.Unmarshal(templateBytes, &result); err != nil {
        return nil
    }
    
    return &result
}
```

## Migration Strategy

### Phase 1: Template Engine Package
1. Extract template engine to `pkg/template/`
2. Make it self-contained and independent
3. Add comprehensive tests

### Phase 2: Pre-Insert Mode Implementation
1. Add template processing middleware
2. Enhance repository interfaces (backward compatible)
3. Add template-aware insert methods

### Phase 3: HTTP Handler Enhancement
1. Add optional template configuration parsing
2. Enhance handlers with template support
3. Maintain backward compatibility

### Phase 4: Worker Mode Implementation (Future)
1. Enhance embedding workers with template processing
2. Add template result checking logic
3. Implement template processing during worker execution

### Phase 5: Performance Optimization
1. Add template engine pooling
2. Implement batch processing
3. Add performance monitoring

## Usage Examples

### Basic Usage (No Template Processing)

```bash
# Existing API call (unchanged)
curl -X POST "http://localhost:8080/api/v1/logs?schema=otel" \
  -H "Content-Type: application/json" \
  -d '[{"ServiceName": "my-service", "Body": "test log"}]'
```

### Pre-Insert Template Processing Usage

```bash
# Template processing during insertion (one-to-one relationship)
curl -X POST "http://localhost:8080/api/v1/logs?schema=otel&processor_config_id=123&enable_pre_insert=true" \
  -H "Content-Type: application/json" \
  -d '[{"ServiceName": "my-service", "Body": "test log"}]'
```

### Advanced Configuration

```bash
# Full template configuration (one-to-one relationship)
curl -X POST "http://localhost:8080/api/v1/logs?schema=otel&processor_config_id=123&enable_pre_insert=true&performance_mode=fast&store_results=true" \
  -H "Content-Type: application/json" \
  -d '[{"ServiceName": "my-service", "Body": "test log"}]'
```

### Worker Mode Template Processing (Future)

```bash
# Template processing during worker execution (one-to-one relationship)
curl -X POST "http://localhost:8080/api/v1/logs?schema=otel&processor_config_id=123&enable_worker_mode=true" \
  -H "Content-Type: application/json" \
  -d '[{"ServiceName": "my-service", "Body": "test log"}]'
```

### Dual-Mode Processing

```bash
# Both pre-insert and worker mode enabled (one-to-one relationship)
curl -X POST "http://localhost:8080/api/v1/logs?schema=otel&processor_config_id=123&enable_pre_insert=true&enable_worker_mode=true" \
  -H "Content-Type: application/json" \
  -d '[{"ServiceName": "my-service", "Body": "test log"}]'
```

## Benefits

### Minimal Impact
- ✅ Existing code paths remain unchanged
- ✅ Template processing is opt-in
- ✅ Backward compatibility maintained
- ✅ No breaking changes

### Self-Contained Design
- ✅ Template engine is independent
- ✅ Can be extracted as standalone package
- ✅ Clear separation of concerns
- ✅ Future processor mode ready

### Performance
- ✅ Zero-allocation operations
- ✅ Template engine pooling
- ✅ Batch processing support
- ✅ Efficient metadata storage

### Flexibility
- ✅ Multiple processing modes
- ✅ Configuration-driven behavior
- ✅ Graceful fallbacks
- ✅ Extensible design

## Future Extensions

### Processor Mode Integration
```go
// Future: Template processing in OpenTelemetry processor
type TemplateProcessor struct {
    engine *template.Engine
    config *template.Config
}

func (tp *TemplateProcessor) ProcessTraces(ctx context.Context, traces ptrace.Traces) error {
    // Process templates during OpenTelemetry processing pipeline
    return tp.processTemplates(ctx, traces)
}
```

### Standalone Package
```bash
# Future: Extract as standalone package
go get github.com/medxops/template-engine
```

## Error Handling and Resilience

### Template Processing Errors
```go
// Graceful error handling in template processing
func (r *logRepository) processTemplatesInMemory(ctx context.Context, entries []LogEntry, config *template.Config) []LogEntry {
    processedEntries := make([]LogEntry, len(entries))
    copy(processedEntries, entries)
    
    for i := range processedEntries {
        result, err := r.templateEngine.ProcessSignal(ctx, "logs", signalData, config.ProcessorConfigID)
        if err != nil {
            // Log error but continue processing other entries
            r.logger.Error("template processing failed", 
                "error", err, 
                "entry_id", processedEntries[i].ID,
                "template_id", config.TemplateID)
            
            // Store error information in metadata for debugging
            r.storeTemplateErrorInMetadata(&processedEntries[i], err, config.TemplateID)
            continue
        }
        
        r.storeTemplateResultsInMemory(&processedEntries[i], result)
    }
    
    return processedEntries
}

// Store template processing errors in metadata
func (r *logRepository) storeTemplateErrorInMetadata(entry *LogEntry, err error, templateID int64) {
    existingMetadata := make(map[string]interface{})
    if len(entry.Metadata) > 0 {
        json.Unmarshal(entry.Metadata, &existingMetadata)
    }
    
    existingMetadata["template_processing"] = map[string]interface{}{
        "error":        err.Error(),
        "template_id":  templateID,
        "processed_at": time.Now(),
        "mode":         "pre_insert",
        "status":       "failed",
    }
    
    if metadataBytes, err := json.Marshal(existingMetadata); err == nil {
        entry.Metadata = metadataBytes
    }
}
```

### Fallback Strategies
1. **Template Engine Unavailable**: Fall back to original insert without template processing
2. **Template Processing Errors**: Continue with original data, log errors for debugging
3. **Invalid Template Configuration**: Use default templates or skip processing
4. **Memory Constraints**: Process in smaller batches or skip processing

## Monitoring and Observability

### Template Processing Metrics
```go
// Template processing metrics
type TemplateMetrics struct {
    ProcessingCount    int64
    ProcessingErrors   int64
    ProcessingDuration time.Duration
    TemplateCacheHits  int64
    TemplateCacheMisses int64
}

// Metrics collection
func (e *Engine) recordMetrics(signalType string, duration time.Duration, err error) {
    e.metrics.ProcessingCount++
    e.metrics.ProcessingDuration += duration
    
    if err != nil {
        e.metrics.ProcessingErrors++
    }
}
```

### Health Checks
```go
// Template engine health check
func (e *Engine) HealthCheck() error {
    if e.contentEngine == nil {
        return errors.New("template engine not initialized")
    }
    
    // Test template compilation
    if err := e.contentEngine.ValidateTemplates(); err != nil {
        return fmt.Errorf("template validation failed: %w", err)
    }
    
    return nil
}
```

## Security Considerations

### Template Injection Prevention
```go
// Validate template content for security
func (e *Engine) validateTemplateSecurity(templateContent string) error {
    // Check for potentially dangerous template functions
    dangerousPatterns := []string{
        "exec", "system", "eval", "os.", "runtime.",
    }
    
    for _, pattern := range dangerousPatterns {
        if strings.Contains(templateContent, pattern) {
            return fmt.Errorf("potentially dangerous template content: %s", pattern)
        }
    }
    
    return nil
}
```

### Input Sanitization
```go
// Sanitize signal data before template processing
func sanitizeSignalData(data map[string]interface{}) map[string]interface{} {
    sanitized := make(map[string]interface{})
    
    for key, value := range data {
        switch v := value.(type) {
        case string:
            // Basic HTML/script tag removal
            sanitized[key] = sanitizeString(v)
        case map[string]interface{}:
            sanitized[key] = sanitizeSignalData(v)
        default:
            sanitized[key] = v
        }
    }
    
    return sanitized
}
```

## Testing Strategy

### Unit Tests
```go
// Template engine unit tests
func TestTemplateEngine_ProcessSignal(t *testing.T) {
    engine := NewEngine()
    
    testCases := []struct {
        name       string
        signalType string
        signalData map[string]interface{}
        expected   string
    }{
        {
            name:       "basic log processing",
            signalType: "logs",
            signalData: map[string]interface{}{
                "service_name": "test-service",
                "body":         "test log message",
            },
            expected: "Log Event from service test-service",
        },
    }
    
    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            result, err := engine.ProcessSignal(context.Background(), tc.signalType, tc.signalData)
            assert.NoError(t, err)
            assert.Contains(t, result.Content, tc.expected)
        })
    }
}
```

### Integration Tests
```go
// Integration test with actual database
func TestTemplateProcessing_Integration(t *testing.T) {
    // Test full flow: HTTP request → template processing → database insertion
    // Verify template results are stored in metadata
}
```

## Configuration Management

### Environment-Based Configuration
```go
// Template configuration from environment
type TemplateConfig struct {
    EnablePreInsert  bool   `env:"TEMPLATE_ENABLE_PRE_INSERT" envDefault:"false"`
    EnableWorkerMode bool   `env:"TEMPLATE_ENABLE_WORKER_MODE" envDefault:"true"`
    PerformanceMode  string `env:"TEMPLATE_PERFORMANCE_MODE" envDefault:"balanced"`
    MaxBatchSize     int    `env:"TEMPLATE_MAX_BATCH_SIZE" envDefault:"100"`
    CacheSize        int    `env:"TEMPLATE_CACHE_SIZE" envDefault:"1000"`
}
```

### Dynamic Configuration Updates
```go
// Hot-reload template configuration
func (e *Engine) UpdateConfig(newConfig *Config) error {
    e.mu.Lock()
    defer e.mu.Unlock()
    
    // Validate new configuration
    if err := e.validateConfig(newConfig); err != nil {
        return err
    }
    
    // Update configuration
    e.config = newConfig
    
    // Clear template cache if needed
    if e.config.ClearCacheOnUpdate {
        e.contentEngine.ClearCache()
    }
    
    return nil
}
```

## Performance Tuning

### Configuration Loading & Caching Strategy

**Critical Performance Consideration**: The `ProcessSignal` function calls `getProcessorConfig` and `getProcessorTemplate`, which would query the database. To maintain "zero database overhead" claims, a robust caching strategy is essential.

```go
// Configuration cache with TTL
type ConfigCache struct {
    processorConfigs *lru.Cache
    processorTemplates *lru.Cache
    ttl time.Duration
    mu sync.RWMutex
}

type CachedConfig struct {
    Config      interface{}
    ExpiresAt   time.Time
}

func NewConfigCache(size int, ttl time.Duration) *ConfigCache {
    return &ConfigCache{
        processorConfigs: lru.New(size),
        processorTemplates: lru.New(size),
        ttl: ttl,
    }
}

// Get processor config with caching
func (cc *ConfigCache) GetProcessorConfig(ctx context.Context, configID int64) (*ProcessorConfig, error) {
    cc.mu.RLock()
    if cached, ok := cc.processorConfigs.Get(configID); ok {
        cachedConfig := cached.(CachedConfig)
        if time.Now().Before(cachedConfig.ExpiresAt) {
            cc.mu.RUnlock()
            return cachedConfig.Config.(*ProcessorConfig), nil
        }
    }
    cc.mu.RUnlock()
    
    // Cache miss - fetch from database
    cc.mu.Lock()
    defer cc.mu.Unlock()
    
    config, err := fetchProcessorConfigFromDB(ctx, configID)
    if err != nil {
        return nil, err
    }
    
    cc.processorConfigs.Add(configID, CachedConfig{
        Config:    config,
        ExpiresAt: time.Now().Add(cc.ttl),
    })
    
    return config, nil
}

// Get processor template with caching
func (cc *ConfigCache) GetProcessorTemplate(ctx context.Context, templateID int64) (*ProcessorTemplate, error) {
    cc.mu.RLock()
    if cached, ok := cc.processorTemplates.Get(templateID); ok {
        cachedTemplate := cached.(CachedConfig)
        if time.Now().Before(cachedTemplate.ExpiresAt) {
            cc.mu.RUnlock()
            return cachedTemplate.Config.(*ProcessorTemplate), nil
        }
    }
    cc.mu.RUnlock()
    
    // Cache miss - fetch from database
    cc.mu.Lock()
    defer cc.mu.Unlock()
    
    template, err := fetchProcessorTemplateFromDB(ctx, templateID)
    if err != nil {
        return nil, err
    }
    
    cc.processorTemplates.Add(templateID, CachedConfig{
        Config:    template,
        ExpiresAt: time.Now().Add(cc.ttl),
    })
    
    return template, nil
}
```

### Caching Strategies

1. **Application Startup Loading**: Pre-load all configurations at startup
2. **On-Demand with TTL**: Load configurations on first use with time-to-live
3. **Hot-Reloading**: Background refresh of configurations
4. **Hybrid Approach**: Critical configs at startup, others on-demand

### Template Caching Strategy
```go
// LRU cache for compiled templates
type TemplateCache struct {
    cache *lru.Cache
    size  int
}

func NewTemplateCache(size int) *TemplateCache {
    cache, _ := lru.New(size)
    return &TemplateCache{
        cache: cache,
        size:  size,
    }
}

func (tc *TemplateCache) Get(key string) (*ContentTemplateEngine, bool) {
    if engine, ok := tc.cache.Get(key); ok {
        return engine.(*ContentTemplateEngine), true
    }
    return nil, false
}

func (tc *TemplateCache) Set(key string, engine *ContentTemplateEngine) {
    tc.cache.Add(key, engine)
}
```

### Memory Optimization
```go
// Memory pool for template processing
type TemplatePool struct {
    bufferPool *sync.Pool
    dataPool   *sync.Pool
}

func NewTemplatePool() *TemplatePool {
    return &TemplatePool{
        bufferPool: &sync.Pool{
            New: func() interface{} {
                return new(bytes.Buffer)
            },
        },
        dataPool: &sync.Pool{
            New: func() interface{} {
                return make(map[string]interface{})
            },
        },
    }
}
```

This design ensures minimal impact on existing business logic while providing a robust, self-contained template processing solution that can be easily integrated and eventually extracted as a standalone package. 

## Future Improvements and Optimizations

### 1. Performance Monitoring and Metrics

**Add comprehensive performance monitoring to track template processing efficiency:**

```go
// Template processing metrics for production monitoring
type TemplateProcessingMetrics struct {
    ProcessingDuration    time.Duration
    EntriesProcessed      int64
    ProcessingErrors      int64
    CacheHitRate         float64
    MemoryUsage          int64
    TemplateCompileTime  time.Duration
    JSONUnmarshalTime    time.Duration
    MetadataMergeTime    time.Duration
}

// Metrics collection in template processing
func (r *logRepository) processTemplatesInMemory(ctx context.Context, entries []LogEntry, config *template.Config) []LogEntry {
    startTime := time.Now()
    processedCount := int64(0)
    errorCount := int64(0)
    
    // Process entries with metrics collection
    for i := range processedEntries {
        entryStart := time.Now()
        
        signalData := r.convertLogEntryToTemplateData(&processedEntries[i])
        result, err := r.templateEngine.ProcessSignal(ctx, "logs", signalData, config.ProcessorConfigID)
        
        if err != nil {
            errorCount++
            r.logger.Error("template processing failed", 
                "error", err, 
                "entry_id", processedEntries[i].ID,
                "config_id", config.ProcessorConfigID)
            continue
        }
        
        r.storeTemplateResultsInMemory(&processedEntries[i], result)
        processedCount++
        
        // Record per-entry metrics
        r.metrics.RecordEntryProcessing(time.Since(entryStart))
    }
    
    // Record batch metrics
    r.metrics.RecordBatchProcessing(
        duration: time.Since(startTime),
        processed: processedCount,
        errors: errorCount,
        batchSize: len(entries),
    )
    
    return processedEntries
}
```

### 2. Batch Template Processing Optimization

**Implement batch processing for better performance with large datasets:**

```go
// Batch template processing for improved performance
type BatchTemplateProcessor struct {
    templateEngine *template.Engine
    workerPool     *sync.Pool
    maxWorkers     int
}

func (btp *BatchTemplateProcessor) ProcessBatch(ctx context.Context, entries []LogEntry, config *template.Config) []LogEntry {
    if len(entries) <= btp.maxWorkers {
        return btp.processSequentially(ctx, entries, config)
    }
    
    // Process in parallel batches
    chunkSize := len(entries) / btp.maxWorkers
    results := make([]LogEntry, len(entries))
    
    var wg sync.WaitGroup
    for i := 0; i < btp.maxWorkers; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            
            start := workerID * chunkSize
            end := start + chunkSize
            if workerID == btp.maxWorkers-1 {
                end = len(entries)
            }
            
            chunk := entries[start:end]
            processedChunk := btp.processSequentially(ctx, chunk, config)
            copy(results[start:end], processedChunk)
        }(i)
    }
    
    wg.Wait()
    return results
}
```

### 3. Template Result Validation and Sanitization

**Add validation to prevent metadata bloat and ensure data quality:**

```go
// Template result validation and sanitization
type TemplateResultValidator struct {
    maxContentSize    int
    maxMetadataSize   int
    allowedFields     map[string]bool
}

func (trv *TemplateResultValidator) ValidateAndSanitize(result *template.ProcessingResult) error {
    // Validate content size
    if len(result.Content) > trv.maxContentSize {
        result.Content = truncateString(result.Content, trv.maxContentSize)
        log.Warn("Template content truncated due to size limit", 
            "original_size", len(result.Content),
            "max_size", trv.maxContentSize)
    }
    
    // Validate metadata fields
    if result.Metadata != nil {
        sanitizedMetadata := make(map[string]interface{})
        for key, value := range result.Metadata {
            if trv.allowedFields[key] {
                sanitizedMetadata[key] = value
            } else {
                log.Warn("Filtered out disallowed metadata field", "field", key)
            }
        }
        result.Metadata = sanitizedMetadata
    }
    
    return nil
}

// Configuration validation
func validateTemplateConfig(config *template.Config) error {
    if config.ProcessorConfigID <= 0 {
        return errors.New("invalid processor config ID")
    }
    
    if config.PerformanceMode != "" && 
       config.PerformanceMode != "fast" && 
       config.PerformanceMode != "balanced" && 
       config.PerformanceMode != "complete" {
        return errors.New("invalid performance mode")
    }
    
    return nil
}
```

### 4. Memory Pool Optimization

**Use object pools for high-throughput scenarios to reduce GC pressure:**

```go
// Memory pools for template processing
type TemplateMemoryPool struct {
    metadataPool *sync.Pool
    dataPool     *sync.Pool
    bufferPool   *sync.Pool
}

func NewTemplateMemoryPool() *TemplateMemoryPool {
    return &TemplateMemoryPool{
        metadataPool: &sync.Pool{
            New: func() interface{} {
                return make(map[string]interface{})
            },
        },
        dataPool: &sync.Pool{
            New: func() interface{} {
                return make(map[string]interface{})
            },
        },
        bufferPool: &sync.Pool{
            New: func() interface{} {
                return new(bytes.Buffer)
            },
        },
    }
}

func (tmp *TemplateMemoryPool) GetMetadataMap() map[string]interface{} {
    return tmp.metadataPool.Get().(map[string]interface{})
}

func (tmp *TemplateMemoryPool) PutMetadataMap(m map[string]interface{}) {
    // Clear the map before returning to pool
    for k := range m {
        delete(m, k)
    }
    tmp.metadataPool.Put(m)
}

// Usage in template processing
func (r *logRepository) storeTemplateResultsInMemory(entry *LogEntry, result *template.ProcessingResult) {
    existingMetadata := r.memoryPool.GetMetadataMap()
    defer r.memoryPool.PutMetadataMap(existingMetadata)
    
    // Parse existing metadata
    if len(entry.Metadata) > 0 {
        if err := json.Unmarshal(entry.Metadata, &existingMetadata); err == nil {
            existingMetadata["template_processing"] = result
            if metadataBytes, err := json.Marshal(existingMetadata); err == nil {
                entry.Metadata = metadataBytes
            }
        }
    }
}
```

### 5. Template Result Caching

**Cache template results for identical inputs to improve performance:**

```go
// Template result cache for repeated patterns
type TemplateResultCache struct {
    cache *lru.Cache
    ttl   time.Duration
    mu    sync.RWMutex
}

type CachedTemplateResult struct {
    Result    *template.ProcessingResult
    ExpiresAt time.Time
}

func NewTemplateResultCache(size int, ttl time.Duration) *TemplateResultCache {
    cache, _ := lru.New(size)
    return &TemplateResultCache{
        cache: cache,
        ttl:   ttl,
    }
}

func (trc *TemplateResultCache) Get(key string) (*template.ProcessingResult, bool) {
    trc.mu.RLock()
    defer trc.mu.RUnlock()
    
    if cached, ok := trc.cache.Get(key); ok {
        cachedResult := cached.(CachedTemplateResult)
        if time.Now().Before(cachedResult.ExpiresAt) {
            return cachedResult.Result, true
        }
    }
    return nil, false
}

func (trc *TemplateResultCache) Set(key string, result *template.ProcessingResult) {
    trc.mu.Lock()
    defer trc.mu.Unlock()
    
    trc.cache.Add(key, CachedTemplateResult{
        Result:    result,
        ExpiresAt: time.Now().Add(trc.ttl),
    })
}

// Generate cache key from signal data
func generateTemplateCacheKey(signalData map[string]interface{}, configID int64) string {
    // Create a deterministic key from signal data
    keyData := map[string]interface{}{
        "config_id": configID,
        "service_name": signalData["service_name"],
        "severity_text": signalData["severity_text"],
        "body": signalData["body"],
    }
    
    keyBytes, _ := json.Marshal(keyData)
    return fmt.Sprintf("template_%x", md5.Sum(keyBytes))
}
```

### 6. Graceful Degradation for Large Batches

**Implement batch size limits and chunking for memory management:**

```go
// Batch size management for template processing
const (
    MaxTemplateBatchSize = 1000
    OptimalTemplateBatchSize = 100
)

func (r *logRepository) InsertLogsWithTemplates(ctx context.Context, entries []LogEntry, schema string, config *template.Config) ([]string, error) {
    if config != nil && config.EnablePreInsert && r.templateEngine != nil {
        if len(entries) > MaxTemplateBatchSize {
            // Process in chunks to prevent memory spikes
            return r.processTemplatesInChunks(ctx, entries, schema, config)
        }
        
        // Process normally for smaller batches
        processedEntries := r.processTemplatesInMemory(ctx, entries, config)
        return r.InsertLogs(ctx, processedEntries, schema)
    }
    
    return r.InsertLogs(ctx, entries, schema)
}

func (r *logRepository) processTemplatesInChunks(ctx context.Context, entries []LogEntry, schema string, config *template.Config) ([]string, error) {
    var allIDs []string
    
    for start := 0; start < len(entries); start += OptimalTemplateBatchSize {
        end := start + OptimalTemplateBatchSize
        if end > len(entries) {
            end = len(entries)
        }
        
        chunk := entries[start:end]
        processedChunk := r.processTemplatesInMemory(ctx, chunk, config)
        
        ids, err := r.InsertLogs(ctx, processedChunk, schema)
        if err != nil {
            return nil, fmt.Errorf("failed to insert chunk %d-%d: %w", start, end, err)
        }
        
        allIDs = append(allIDs, ids...)
        
        // Check for context cancellation between chunks
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        default:
        }
    }
    
    return allIDs, nil
}
```

### 7. Enhanced Error Handling and Policies

**Implement configurable error handling policies:**

```go
// Error handling policies for template processing
type TemplateErrorPolicy string

const (
    ErrorPolicyContinue TemplateErrorPolicy = "continue"  // Skip failed entries, continue processing
    ErrorPolicyStop     TemplateErrorPolicy = "stop"      // Stop processing on first error
    ErrorPolicyRetry    TemplateErrorPolicy = "retry"     // Retry failed entries
)

type TemplateProcessingConfig struct {
    ErrorPolicy        TemplateErrorPolicy
    MaxRetries         int
    RetryBackoff       time.Duration
    LogErrors          bool
    StoreErrorDetails  bool
}

func (r *logRepository) processTemplatesInMemory(ctx context.Context, entries []LogEntry, config *template.Config) []LogEntry {
    processedEntries := make([]LogEntry, len(entries))
    copy(processedEntries, entries)
    
    for i := range processedEntries {
        result, err := r.templateEngine.ProcessSignal(ctx, "logs", signalData, config.ProcessorConfigID)
        
        if err != nil {
            switch config.ErrorPolicy {
            case ErrorPolicyContinue:
                // Log error and continue (current behavior)
                r.logger.Error("template processing failed, continuing", "error", err)
                continue
                
            case ErrorPolicyStop:
                // Stop processing and return error
                return nil, fmt.Errorf("template processing failed at entry %d: %w", i, err)
                
            case ErrorPolicyRetry:
                // Retry with exponential backoff
                if retryResult := r.retryTemplateProcessing(ctx, signalData, config, config.MaxRetries); retryResult != nil {
                    r.storeTemplateResultsInMemory(&processedEntries[i], retryResult)
                }
            }
        } else {
            r.storeTemplateResultsInMemory(&processedEntries[i], result)
        }
    }
    
    return processedEntries
}
```

### 8. Template Engine Thread Safety and Pooling

**Ensure thread safety and implement engine pooling for high concurrency:**

```go
// Thread-safe template engine pool
type TemplateEnginePool struct {
    pool *sync.Pool
    mu   sync.RWMutex
}

func NewTemplateEnginePool() *TemplateEnginePool {
    return &TemplateEnginePool{
        pool: &sync.Pool{
            New: func() interface{} {
                return template.NewEngine()
            },
        },
    }
}

func (tep *TemplateEnginePool) Get() *template.Engine {
    return tep.pool.Get().(*template.Engine)
}

func (tep *TemplateEnginePool) Put(engine *template.Engine) {
    // Reset engine state before returning to pool
    engine.Reset()
    tep.pool.Put(engine)
}

// Thread-safe template processing
func (r *logRepository) processTemplatesInMemory(ctx context.Context, entries []LogEntry, config *template.Config) []LogEntry {
    processedEntries := make([]LogEntry, len(entries))
    copy(processedEntries, entries)
    
    // Use engine pool for thread safety
    engine := r.enginePool.Get()
    defer r.enginePool.Put(engine)
    
    for i := range processedEntries {
        signalData := r.convertLogEntryToTemplateData(&processedEntries[i])
        result, err := engine.ProcessSignal(ctx, "logs", signalData, config.ProcessorConfigID)
        
        if err != nil {
            r.logger.Error("template processing failed", "error", err)
            continue
        }
        
        r.storeTemplateResultsInMemory(&processedEntries[i], result)
    }
    
    return processedEntries
}
```

### 9. Cache Invalidation Strategy

**Implement intelligent cache invalidation beyond simple TTL:**

```go
// Enhanced cache invalidation
type CacheInvalidationStrategy interface {
    ShouldInvalidate(key string, lastAccess time.Time) bool
    Invalidate(key string)
}

type AdaptiveTTLStrategy struct {
    baseTTL     time.Duration
    accessCount map[string]int
    mu          sync.RWMutex
}

func (ats *AdaptiveTTLStrategy) ShouldInvalidate(key string, lastAccess time.Time) bool {
    ats.mu.RLock()
    count := ats.accessCount[key]
    ats.mu.RUnlock()
    
    // Extend TTL for frequently accessed items
    ttl := ats.baseTTL
    if count > 10 {
        ttl = ats.baseTTL * 2
    }
    if count > 100 {
        ttl = ats.baseTTL * 4
    }
    
    return time.Since(lastAccess) > ttl
}

// Cache invalidation on template updates
type TemplateUpdateNotifier struct {
    subscribers map[string][]chan struct{}
    mu          sync.RWMutex
}

func (tun *TemplateUpdateNotifier) Subscribe(templateID string, ch chan struct{}) {
    tun.mu.Lock()
    defer tun.mu.Unlock()
    
    tun.subscribers[templateID] = append(tun.subscribers[templateID], ch)
}

func (tun *TemplateUpdateNotifier) NotifyUpdate(templateID string) {
    tun.mu.RLock()
    defer tun.mu.RUnlock()
    
    for _, ch := range tun.subscribers[templateID] {
        select {
        case ch <- struct{}{}:
        default:
            // Channel is full, skip notification
        }
    }
}
```

### 10. Configuration Hot-Reloading

**Implement hot-reloading of template configurations:**

```go
// Hot-reloading configuration manager
type ConfigHotReloader struct {
    configPath    string
    reloadInterval time.Duration
    subscribers    []chan *template.Config
    mu            sync.RWMutex
}

func NewConfigHotReloader(configPath string, reloadInterval time.Duration) *ConfigHotReloader {
    chr := &ConfigHotReloader{
        configPath:     configPath,
        reloadInterval: reloadInterval,
        subscribers:    make([]chan *template.Config, 0),
    }
    
    go chr.watchConfig()
    return chr
}

func (chr *ConfigHotReloader) watchConfig() {
    ticker := time.NewTicker(chr.reloadInterval)
    defer ticker.Stop()
    
    lastModTime := time.Time{}
    
    for range ticker.C {
        if info, err := os.Stat(chr.configPath); err == nil {
            if info.ModTime().After(lastModTime) {
                if config := chr.loadConfig(); config != nil {
                    chr.notifySubscribers(config)
                    lastModTime = info.ModTime()
                }
            }
        }
    }
}

func (chr *ConfigHotReloader) Subscribe(ch chan *template.Config) {
    chr.mu.Lock()
    defer chr.mu.Unlock()
    
    chr.subscribers = append(chr.subscribers, ch)
}

func (chr *ConfigHotReloader) notifySubscribers(config *template.Config) {
    chr.mu.RLock()
    defer chr.mu.RUnlock()
    
    for _, ch := range chr.subscribers {
        select {
        case ch <- config:
        default:
            // Channel is full, skip notification
        }
    }
}
```

These improvements provide a comprehensive roadmap for optimizing the template processing system for production use, focusing on performance, reliability, and maintainability. 