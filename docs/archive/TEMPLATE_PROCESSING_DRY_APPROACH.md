# Template Processing DRY Approach

## Overview

This document outlines the DRY (Don't Repeat Yourself) approach implemented for template processing across all signal types (logs, traces, and metrics) in the trazr-otel repository.

## Problem Statement

The original implementation had significant code duplication:

1. **Template processing logic** was duplicated across repositories
2. **Data conversion** was specific to each signal type
3. **Metadata extraction and storage** was repeated
4. **Insert operations** had similar patterns but different implementations

## Solution Architecture

### 1. Shared Template Processor (`template_processor.go`)

The `TemplateProcessor` provides a unified interface for processing templates across all signal types:

```go
type TemplateProcessor struct{}

func (tp *TemplateProcessor) ProcessTemplatesAndPopulateMetadata(
    ctx context.Context,
    entries []SignalDataConverter,
    schemaName string,
    signalType string,
) ([]SignalDataConverter, error)
```

**Key Features:**
- Generic template processing for any signal type
- Automatic metadata extraction and storage
- Error handling with fallback to original entries
- Debug logging for troubleshooting

### 2. Signal Data Converter Interface (`otel_signal_converters.go`)

The `SignalDataConverter` interface provides a unified way to convert any signal type to template-compatible data:

```go
type SignalDataConverter interface {
    ToTemplateData() map[string]interface{}
    GetSignalType() string
    GetMetadata() []byte
    SetMetadata([]byte)
}
```

**Implementations:**
- `LogEntryConverter` - Converts `LogEntry` to template data
- `TraceEntryConverter` - Converts `TraceEntry` to template data  
- `MetricEntryConverter` - Converts metric entries to template data

### 3. Shared Insert Functions (`otel_repository_util.go`)

Package-level functions provide unified insert functionality with template processing:

```go
func InsertSignalEntries(
    ctx context.Context,
    db *sql.DB,
    entries []SignalDataConverter,
    schemaName string,
    signalType string,
    tableName string,
    columns []string,
    extractArgs func(SignalDataConverter) []interface{},
) ([]string, error)
```

**Specialized Functions:**
- `InsertLogEntries()` - For log entries
- `InsertTraceEntries()` - For trace entries
- `InsertMetricEntries()` - For metric entries

## Usage Examples

### For Logs

```go
// In logRepository
func (r *logRepository) InsertLogs(ctx context.Context, entries []LogEntry, schemaName string) ([]string, error) {
    return InsertLogEntries(ctx, r.db, entries, schemaName)
}
```

### For Traces

```go
// In traceRepository
func (r *traceRepository) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {
    return InsertTraceEntries(ctx, r.db, entries, schema)
}
```

### For Metrics

```go
// In metricRepository
func (r *metricRepository) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {
    return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeGaugeMetrics)
}
```

## Benefits

### 1. **Code Reuse**
- Single template processing logic for all signal types
- Shared insert operations with consistent error handling
- Unified metadata management

### 2. **Maintainability**
- Changes to template processing affect all signal types
- Consistent behavior across repositories
- Easier to add new signal types

### 3. **Consistency**
- Same template processing pipeline for all signals
- Uniform error handling and logging
- Standardized metadata structure

### 4. **Extensibility**
- Easy to add new signal types by implementing `SignalDataConverter`
- Template processing automatically works for new types
- Minimal code changes required

## Implementation Steps

### ✅ **Completed Implementation**

The DRY approach has been successfully implemented for all signal types:

#### 1. **Logs** ✅
```go
func (r *logRepository) InsertLogs(ctx context.Context, entries []LogEntry, schemaName string) ([]string, error) {
    return InsertLogEntries(ctx, r.db, entries, schemaName)
}
```

#### 2. **Traces** ✅
```go
func (r *traceRepository) InsertSpans(ctx context.Context, entries []TraceEntry, schema string) ([]string, error) {
    return InsertTraceEntries(ctx, r.db, entries, schema)
}
```

#### 3. **Metrics** ✅
```go
// All metric types now use template processing
func (r *metricRepository) InsertGaugeMetrics(ctx context.Context, entries []GaugeMetricEntry, schema string) ([]string, error) {
    return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeGaugeMetrics)
}

func (r *metricRepository) InsertSumMetrics(ctx context.Context, entries []SumMetricEntry, schema string) ([]string, error) {
    return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeSumMetrics)
}

func (r *metricRepository) InsertHistogramMetrics(ctx context.Context, entries []HistogramMetricEntry, schema string) ([]string, error) {
    return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeHistogramMetrics)
}

func (r *metricRepository) InsertSummaryMetrics(ctx context.Context, entries []SummaryMetricEntry, schema string) ([]string, error) {
    return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeSummaryMetrics)
}

func (r *metricRepository) InsertExponentialHistogramMetrics(ctx context.Context, entries []ExponentialHistogramMetricEntry, schema string) ([]string, error) {
    return InsertMetricEntries(ctx, r.db, entries, schema, SignalTypeExponentialHistogramMetrics)
}
```

#### 4. **Profiles** ✅
```go
func (r *profileRepository) InsertProfiles(ctx context.Context, entries []ProfileEntry, schema string) ([]string, error) {
    return InsertProfileEntries(ctx, r.db, entries, schema)
}
```

## Migration Guide

### ✅ **Migration Complete**

All signal types have been successfully migrated to use the DRY approach:

1. **✅ Logs** - Template processing implemented
2. **✅ Traces** - Template processing implemented  
3. **✅ Metrics** - Template processing implemented for all metric types:
   - Gauge metrics
   - Sum metrics
   - Histogram metrics
   - Summary metrics
   - Exponential histogram metrics
4. **✅ Profiles** - Template processing implemented

### For Future Signal Types

1. **Implement `SignalDataConverter`** interface
2. **Add converter function** if needed
3. **Update signal type constants** if necessary
4. **Test template processing** with new signal type

## Template Processing Flow

```
1. Signal Entry (LogEntry, TraceEntry, etc.)
   ↓
2. SignalDataConverter (LogEntryConverter, etc.)
   ↓
3. TemplateProcessor.ProcessTemplatesAndPopulateMetadata()
   ↓
4. Template Engine Rendering
   ↓
5. Metadata Extraction and Storage
   ↓
6. InsertHelper.InsertSignalEntries()
   ↓
7. Database Insert with Template Results
```

## Error Handling

The DRY approach includes robust error handling:

- **Template processing failures** fall back to original entries
- **Database errors** are properly propagated
- **Invalid signal types** are handled gracefully
- **Debug logging** helps troubleshoot issues

## Performance Considerations

- **Batch processing** maintains existing performance
- **Template caching** is preserved
- **Minimal overhead** from converter pattern
- **Efficient metadata handling** with JSON marshaling

## Future Enhancements

1. **Template validation** at converter level
2. **Performance metrics** for template processing
3. **Async template processing** for high-throughput scenarios
4. **Template result caching** to avoid reprocessing
5. **Custom metadata extractors** per signal type

## Conclusion

The DRY approach significantly reduces code duplication while maintaining flexibility and performance. It provides a solid foundation for adding new signal types and template processing features in the future. 