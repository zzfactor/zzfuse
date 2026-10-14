# Template Metrics Extension Plan

## Overview
This document outlines the plan to extend the existing metrics infrastructure to comprehensively support template processing metrics across both embedding and pre-insert contexts.

## 📊 **Current State Analysis**

### **✅ Existing Template Metrics (in embedding.go)**
The metrics package already has some template-related metrics for embedding:

```go
// EmbeddingTemplateRendersTotal counts total template renders by status.
EmbeddingTemplateRendersTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "embedding_template_renders_total",
        Help: "Total number of template renders by status.",
    },
    []string{"status", "template_type"},
)

// EmbeddingTemplateLatency measures template render latency.
EmbeddingTemplateLatency = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "embedding_template_latency_seconds",
        Help:    "Template render latency in seconds.",
        Buckets: prometheus.DefBuckets,
    },
    []string{"template_type"},
)
```

### **🔄 Current Template Processing Usage**
- **Embedding Context**: Templates used for content generation during embedding processing
- **Pre-Insert Context**: Templates used for metadata extraction during database insertion
- **Shared Infrastructure**: Both use the same template engine but in different contexts

## 🎯 **Extension Strategy**

### **1. Create Dedicated Template Processing Metrics**

Since template processing is now used in both embedding and pre-insert contexts, we need dedicated metrics that can be used across both scenarios.

**New File**: `pkg/metrics/template.go`

```go
// TemplateProcessingTotal counts total template processing operations by context and status.
TemplateProcessingTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "template_processing_total",
        Help: "Total number of template processing operations by context and status.",
    },
    []string{"context", "status", "signal_type", "template_id"},
)

// TemplateProcessingDuration measures template processing duration by context and signal type.
TemplateProcessingDuration = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "template_processing_duration_seconds",
        Help:    "Template processing duration in seconds by context and signal type.",
        Buckets: prometheus.DefBuckets,
    },
    []string{"context", "signal_type", "template_id"},
)
```

### **2. Context-Aware Metrics**

Define constants for different processing contexts:

```go
const (
    TemplateContextPreInsert = "pre_insert"
    TemplateContextEmbedding = "embedding"
)

const (
    TemplateStatusSuccess = "success"
    TemplateStatusError   = "error"
    TemplateStatusSkipped = "skipped"
)
```

### **3. Comprehensive Metric Categories**

#### **A. Processing Metrics**
- Total operations by context and status
- Duration histograms by context and signal type
- Batch size distributions
- Throughput measurements

#### **B. Error Metrics**
- Error counts by error type and context
- Error rates by signal type
- Specific error categories (config fetch, template render, etc.)

#### **C. Performance Metrics**
- Memory usage during processing
- Cache hit/miss rates
- Active operation counts
- Feature flag status

#### **D. Content Metrics**
- Metadata extraction success rates
- Content generation success rates
- Fields extracted by type

## 🚀 **Implementation Plan**

### **Phase 1: Core Metrics Infrastructure** ✅ **COMPLETED**

1. ✅ **Create `pkg/metrics/template.go`** - New dedicated template metrics file
2. ✅ **Define comprehensive metric categories** - Processing, errors, performance, content
3. ✅ **Add context constants** - Pre-insert vs embedding contexts
4. ✅ **Register metrics in main metrics.go** - Integration with existing system

### **Phase 2: Integration with Template Processor**

#### **A. Update Template Processor**
```go
// In template_processor.go
import "github.com/medxops/trazr-otel/pkg/metrics"

func (tp *TemplateProcessor) ProcessTemplatesAndPopulateMetadata(...) {
    startTime := time.Now()
    
    // Increment active operations
    metrics.TemplateProcessingActiveOperations.WithLabelValues(
        context, signalType,
    ).Inc()
    defer metrics.TemplateProcessingActiveOperations.WithLabelValues(
        context, signalType,
    ).Dec()
    
    // Record duration
    defer func() {
        duration := time.Since(startTime).Seconds()
        metrics.TemplateProcessingDuration.WithLabelValues(
            context, signalType, templateID,
        ).Observe(duration)
    }()
    
    // Record success/error
    defer func() {
        if err != nil {
            metrics.TemplateProcessingTotal.WithLabelValues(
                context, "error", signalType, templateID,
            ).Inc()
            metrics.TemplateProcessingErrorsTotal.WithLabelValues(
                errorType, context, signalType,
            ).Inc()
        } else {
            metrics.TemplateProcessingTotal.WithLabelValues(
                context, "success", signalType, templateID,
            ).Inc()
        }
    }()
}
```

#### **B. Update Repository Insert Functions**
```go
// In otel_repository_util.go
func InsertSignalEntries(...) {
    if shouldSkipTemplateProcessing() {
        metrics.TemplateProcessingFeatureFlagEnabled.WithLabelValues("pre_insert").Set(0)
        // Skip template processing
    } else {
        metrics.TemplateProcessingFeatureFlagEnabled.WithLabelValues("pre_insert").Set(1)
        // Process templates with metrics
    }
}
```

### **Phase 3: Embedding Integration**

#### **A. Update Embedding Worker**
```go
// In embedding_worker.go
func (w *EmbeddingWorker) processBatch(...) {
    // Record batch size
    metrics.TemplateProcessingBatchSize.WithLabelValues(
        "embedding", signalType,
    ).Observe(float64(len(entries)))
    
    // Process templates with existing metrics
    // Template processing metrics will be recorded by template processor
}
```

#### **B. Update Existing Embedding Metrics**
```go
// In embedding.go - keep existing metrics for backward compatibility
// Add new template metrics for comprehensive coverage
```

### **Phase 4: Monitoring and Alerting**

#### **A. Key Metrics to Monitor**
1. **Template Processing Success Rate**
   ```promql
   rate(template_processing_total{status="success"}[5m]) / 
   rate(template_processing_total[5m])
   ```

2. **Template Processing Latency**
   ```promql
   histogram_quantile(0.95, rate(template_processing_duration_seconds_bucket[5m]))
   ```

3. **Template Processing Error Rate**
   ```promql
   rate(template_processing_errors_total[5m])
   ```

4. **Feature Flag Status**
   ```promql
   template_processing_feature_flag_enabled
   ```

#### **B. Alerting Rules**
```yaml
groups:
  - name: template_processing
    rules:
      - alert: TemplateProcessingHighErrorRate
        expr: rate(template_processing_errors_total[5m]) > 0.1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High template processing error rate"
          
      - alert: TemplateProcessingHighLatency
        expr: histogram_quantile(0.95, rate(template_processing_duration_seconds_bucket[5m])) > 1.0
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High template processing latency"
```

## 📈 **Expected Benefits**

### **1. Comprehensive Observability**
- **Context Separation**: Clear distinction between embedding and pre-insert template processing
- **Performance Tracking**: Detailed latency and throughput measurements
- **Error Visibility**: Granular error tracking by type and context
- **Resource Monitoring**: Memory usage and cache performance

### **2. Operational Benefits**
- **Feature Flag Monitoring**: Track when template processing is enabled/disabled
- **Performance Optimization**: Identify bottlenecks in template processing
- **Error Debugging**: Quickly identify and resolve template processing issues
- **Capacity Planning**: Understand resource requirements for template processing

### **3. Business Intelligence**
- **Success Rates**: Track template processing success across different contexts
- **Content Generation**: Monitor metadata extraction and content generation rates
- **Signal Type Analysis**: Understand template processing patterns by signal type

## 🔧 **Implementation Details**

### **1. Metric Naming Convention**
- **Prefix**: `template_processing_`
- **Context**: `pre_insert`, `embedding`
- **Signal Types**: `logs`, `traces`, `metrics`, `profiles`
- **Status**: `success`, `error`, `skipped`

### **2. Label Strategy**
- **Required Labels**: `context`, `signal_type`
- **Optional Labels**: `template_id`, `error_type`, `memory_type`
- **Performance Labels**: `batch_size`, `duration_bucket`

### **3. Bucket Configuration**
```go
// Duration buckets for template processing
Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0}

// Batch size buckets
Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000}
```

## 🚨 **Migration Strategy**

### **1. Backward Compatibility**
- **Keep Existing Metrics**: Maintain `EmbeddingTemplateRendersTotal` and `EmbeddingTemplateLatency`
- **Gradual Migration**: Add new metrics alongside existing ones
- **Deprecation Timeline**: Mark old metrics as deprecated after 6 months

### **2. Feature Flag Integration**
- **Environment Variable**: `SKIP_TEMPLATE_PROCESSING=true`
- **Metric Tracking**: Record feature flag status in metrics
- **Operational Control**: Enable/disable template processing via metrics

### **3. Rollout Plan**
1. **Week 1**: Deploy new metrics infrastructure
2. **Week 2**: Integrate with template processor
3. **Week 3**: Integrate with repository functions
4. **Week 4**: Set up monitoring and alerting
5. **Week 5**: Validate and optimize

## 🏁 **Conclusion**

This extension plan provides comprehensive metrics coverage for template processing across both embedding and pre-insert contexts. The implementation maintains backward compatibility while adding detailed observability capabilities.

**Key Benefits**:
- ✅ **Unified Metrics**: Single source of truth for template processing metrics
- ✅ **Context Awareness**: Clear separation between different processing contexts
- ✅ **Performance Monitoring**: Detailed latency and throughput tracking
- ✅ **Error Visibility**: Granular error tracking and debugging
- ✅ **Operational Control**: Feature flag monitoring and control

**Next Steps**:
1. Complete Phase 1 implementation (✅ Done)
2. Integrate metrics with template processor
3. Add monitoring and alerting rules
4. Validate in production environment 