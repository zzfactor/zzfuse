# Embedding Content Architecture: Current State & Future Recommendations

## 1. Current State

### A. Data Flow
- **Raw OTel signals** (traces, spans, logs, metrics) are ingested.
- **Processing pipeline** extracts/transforms signals into "processed content" using Go code and templates.
- **Processed content** is stored in tables (e.g., `otel.traces`, `otel.spans_span`).
- **Vector embedding** is performed on this content, and the result is stored in a vector store table (e.g., `otel.spans_span` with a `vector` column).

### B. Table Structure
- **otel.traces**: Stores processed trace-level content (JSONB, includes spans, logs, etc.)
- **otel.spans_span**: Stores individual span content and vector embeddings
- **admin.config_embedding_provider**: Configuration for embedding models (token limits, etc.)
- **admin.config_processor_template**: Template definitions for content generation

### C. Current Issues
1. **Content truncation** due to 512-token limit (mxbai model)
2. **Inconsistent content structure** across different transformers
3. **Arbitrary content ordering** that hurts search effectiveness
4. **No pre-calculated aggregates** (log_count, span_count, etc.)
5. **Single model approach** for all use cases

## 2. Research-Based Recommendations

### A. 🏆 Top Priority (Must Implement)

#### 1. Context Injection Strategy ⭐⭐⭐⭐⭐
**Problem**: 512-token limit truncates important trace information
**Solution**: Generate global trace summary with LLM, prepend to each chunk

```go
// Pseudo-code for Context Injection
func (w *Worker) processLargeTrace(traceID string) {
    // 1. Generate global summary
    summary := w.llm.GenerateSummary(traceContent)
    
    // 2. Chunk the trace
    chunks := w.chunkTrace(traceContent)
    
    // 3. Prepend summary to each chunk
    for _, chunk := range chunks {
        enrichedChunk := fmt.Sprintf("Trace Summary: %s\n\n%s", summary, chunk)
        embedding := w.embedModel.Embed(enrichedChunk)
        w.storeVector(embedding, chunk)
    }
}
```

#### 2. Hierarchical Content Structure ⭐⭐⭐⭐⭐
**Problem**: Current content order is arbitrary and hurts search quality
**Solution**: Structured format with clear priorities

```
# SPAN CONTEXT (Most Important for Search)
Span: [name] ([id])
Trace: [id]
Service: [name]
Parent: [id] | ROOT
Kind: [type]
Duration: [time]
Status: [code] - [message]

# OPERATION DETAILS (Business Logic)
Operation: [what this span does]
Endpoint: [if HTTP]
Method: [if applicable]

# CRITICAL ATTRIBUTES (High Priority)
- [key]: [value] (most important for debugging/search)

# CONTEXT ATTRIBUTES (Medium Priority)
- [key]: [value] (environment, user context)

# TECHNICAL ATTRIBUTES (Lower Priority)
- [key]: [value] (internal details)

# CORRELATED SIGNALS (Chronological Order)
## Logs (time-ordered)
[timestamp] [level]: [message]
  - [log attributes]

## Metrics
[metric_name]: [value] [unit]
  - [metric attributes]
```

#### 3. Dual Model Architecture ⭐⭐⭐⭐⭐
**Problem**: Single model can't optimize for different use cases
**Solution**: Different models for different purposes

| Use Case | Model | Context Window | Rationale |
|----------|-------|----------------|-----------|
| **Individual Spans** | `nomic-embed-text` | 8192 tokens | Fast, efficient, good for structured data |
| **Full Traces** | `bge-m3` or `Nomic Embed Code` | 8192 tokens | Powerful, code-aware, better for complex narratives |
| **Regression Testing** | `bge-m3` | 8192 tokens | High accuracy for semantic comparison |

### B. 🎯 High Priority (Should Implement Soon)

#### 4. Pre-calculated Aggregates ⭐⭐⭐⭐
**Problem**: Expensive on-demand queries for derived fields
**Solution**: Store aggregates in database

```sql
-- Add aggregate columns to existing tables
ALTER TABLE otel.traces ADD COLUMN span_count INTEGER DEFAULT 0;
ALTER TABLE otel.traces ADD COLUMN log_count INTEGER DEFAULT 0;
ALTER TABLE otel.traces ADD COLUMN error_count INTEGER DEFAULT 0;
ALTER TABLE otel.traces ADD COLUMN total_duration_ns BIGINT DEFAULT 0;
ALTER TABLE otel.traces ADD COLUMN has_error BOOLEAN DEFAULT FALSE;
ALTER TABLE otel.traces ADD COLUMN max_duration_ns BIGINT DEFAULT 0;
```

**Update Strategy**:
```go
func (w *Worker) updateTraceAggregates(traceID string, signalType string) {
    query := `
        UPDATE otel.traces 
        SET 
            span_count = span_count + CASE WHEN $2 = 'spans' THEN 1 ELSE 0 END,
            log_count = log_count + CASE WHEN $2 = 'logs' THEN 1 ELSE 0 END,
            error_count = error_count + CASE WHEN $2 = 'logs' AND level = 'error' THEN 1 ELSE 0 END,
            has_error = CASE WHEN $2 = 'logs' AND level = 'error' THEN TRUE ELSE has_error END
        WHERE trace_id = $1
    `
    w.db.ExecContext(ctx, query, traceID, signalType)
}
```

#### 5. Template-Driven Processing ⭐⭐⭐⭐
**Problem**: Hard-coded content generation is inflexible
**Solution**: Leverage existing template system

```json
{
  "trace_summary": "Trace for {{.operation}} {{if .has_error}}failed{{else}}completed{{end}} in {{.total_duration}}. Involves {{.span_count}} spans across {{.service_count}} services. {{if .error_count}}Contains {{.error_count}} errors.{{end}}",
  "span_detail": "Span: {{.name}} ({{.span_id}})\nTrace: {{.trace_id}}\nService: {{.service_name}}\nOperation: {{.operation}}\nDuration: {{.duration}}\nStatus: {{.status_code}} - {{.status_message}}\n{{if .critical_attributes}}Critical Attributes:\n{{range .critical_attributes}}- {{.key}}: {{.value}}\n{{end}}{{end}}",
  "error_focus": "ERROR in {{.service_name}}: {{.name}}\nStatus: {{.status_code}} - {{.status_message}}\nOperation: {{.operation}}\n{{if .error_details}}Error Details: {{.error_details}}{{end}}"
}
```

#### 6. Semantic Text Diff for Regression ⭐⭐⭐⭐
**Problem**: Simple vector similarity lacks interpretability
**Solution**: Structured diff with semantic awareness

```go
type SemanticDiff struct {
    TraceID     string
    Changes     []Change
    Severity    string // "low", "medium", "high", "critical"
    Summary     string
}

type Change struct {
    Type        string // "attribute_added", "attribute_changed", "span_added", "span_removed"
    Location    string // "span_id" or "attribute_path"
    OldValue    interface{}
    NewValue    interface{}
    Impact      string // "performance", "error", "business_logic"
}
```

### C. 🔧 Medium Priority (Nice to Have)

#### 7. Hierarchical Embeddings ⭐⭐⭐
**Problem**: Flat embeddings don't capture trace structure
**Solution**: Tree-aware embedding architecture

```go
type HierarchicalEmbedding struct {
    SpanID      string
    ContentVec  []float32  // Standard content embedding
    HierarchicalVec []float32  // Combined with children
    Children    []*HierarchicalEmbedding
}

func (h *HierarchicalEmbedding) Compute() {
    // 1. Get content embedding
    h.ContentVec = embedModel.Embed(h.content)
    
    // 2. Aggregate children
    if len(h.Children) > 0 {
        childVecs := make([][]float32, len(h.Children))
        for i, child := range h.Children {
            childVecs[i] = child.Compute()
        }
        aggregated := aggregate(childVecs) // mean, max, or attention
        
        // 3. Combine with content
        h.HierarchicalVec = combine(h.ContentVec, aggregated)
    } else {
        h.HierarchicalVec = h.ContentVec
    }
    
    return h.HierarchicalVec
}
```

#### 8. Metadata-Aware Indexing ⭐⭐⭐
**Problem**: Generic vector search doesn't scale for multi-tenant
**Solution**: HNSW with filtered search support

```go
type VectorStoreConfig struct {
    IndexType       string // "HNSW", "IVF"
    Dimensions      int
    Metric          string // "cosine", "euclidean"
    FilterFields    []string // ["customer_id", "service_name", "has_error"]
    IndexParams     map[string]interface{}
}
```

## 3. Implementation Phases

### Phase 1: Foundation (Week 1) 🚀
**Goal**: Fix immediate issues and establish baseline

1. **Fix Content Structure**
   - Implement hierarchical content format
   - Update all transformers to use consistent structure
   - Add priority-based content ordering

2. **Add Pre-calculated Aggregates**
   - Add aggregate columns to database tables
   - Implement incremental update logic
   - Update template system to use aggregates

3. **Switch to Better Model**
   - Replace mxbai with nomic-embed-text for spans
   - Update embedding provider configuration
   - Test performance and accuracy improvements

### Phase 2: Advanced Features (Week 2) 🎯
**Goal**: Implement Context Injection and template system

1. **Context Injection Strategy**
   - Integrate LLM for trace summarization
   - Implement chunking with summary prepending
   - Add configuration for different trace sizes

2. **Template System Enhancement**
   - Create use-case-specific templates
   - Add template validation and testing
   - Implement template versioning

3. **Semantic Diff Implementation**
   - Build semantic diff engine
   - Add regression testing endpoints
   - Create baseline management system

### Phase 3: Optimization (Week 3+) 🔧
**Goal**: Advanced features and performance optimization

1. **Hierarchical Embeddings** (if needed)
   - Implement tree-aware embedding architecture
   - Add hierarchical comparison for regression testing
   - Optimize for large trace trees

2. **Performance Optimization**
   - Implement metadata-aware indexing
   - Add caching layer for frequent queries
   - Optimize batch processing

3. **Advanced Analytics**
   - Add anomaly detection
   - Implement trend analysis
   - Create automated alerting

## 4. Technical Specifications

### A. Content Structure Schema

```go
type SpanContent struct {
    // Level 1: Primary Identifiers
    SpanID        string `json:"span_id"`
    TraceID       string `json:"trace_id"`
    ParentID      string `json:"parent_id"`
    ServiceName   string `json:"service_name"`
    SpanName      string `json:"span_name"`
    
    // Level 2: Execution Summary
    StatusCode    string `json:"status_code"`
    StatusMessage string `json:"status_message"`
    Duration      string `json:"duration"`
    SpanKind      string `json:"span_kind"`
    
    // Level 3: Operation Details
    Operation     string `json:"operation"`
    Endpoint      string `json:"endpoint,omitempty"`
    Method        string `json:"method,omitempty"`
    
    // Level 4: Attributes (prioritized)
    CriticalAttrs map[string]interface{} `json:"critical_attributes"`
    ContextAttrs  map[string]interface{} `json:"context_attributes"`
    TechnicalAttrs map[string]interface{} `json:"technical_attributes"`
    
    // Level 5: Correlated Signals
    Logs          []LogEntry     `json:"logs,omitempty"`
    Metrics       []MetricEntry  `json:"metrics,omitempty"`
    Events        []EventEntry   `json:"events,omitempty"`
    
    // Pre-calculated Aggregates
    LogCount      int  `json:"log_count"`
    ErrorCount    int  `json:"error_count"`
    HasError      bool `json:"has_error"`
}
```

### B. Template System

```go
type TemplateEngine struct {
    templates map[string]*template.Template
    funcs     template.FuncMap
}

func (te *TemplateEngine) RegisterTemplate(name, content string) error {
    tmpl, err := template.New(name).Funcs(te.funcs).Parse(content)
    if err != nil {
        return err
    }
    te.templates[name] = tmpl
    return nil
}

func (te *TemplateEngine) Render(templateName string, data interface{}) (string, error) {
    tmpl, exists := te.templates[templateName]
    if !exists {
        return "", fmt.Errorf("template %s not found", templateName)
    }
    
    var buf strings.Builder
    err := tmpl.Execute(&buf, data)
    return buf.String(), err
}
```

### C. Context Injection Implementation

```go
type ContextInjector struct {
    llmClient    LLMClient
    chunkSize    int
    overlapSize  int
}

func (ci *ContextInjector) ProcessTrace(traceContent string) ([]string, error) {
    // 1. Generate summary
    summary, err := ci.llmClient.GenerateSummary(traceContent)
    if err != nil {
        return nil, err
    }
    
    // 2. Chunk content
    chunks := ci.chunkContent(traceContent)
    
    // 3. Inject context
    enrichedChunks := make([]string, len(chunks))
    for i, chunk := range chunks {
        enrichedChunks[i] = ci.injectContext(summary, chunk)
    }
    
    return enrichedChunks, nil
}

func (ci *ContextInjector) injectContext(summary, chunk string) string {
    return fmt.Sprintf("Trace Summary: %s\n\n%s", summary, chunk)
}
```

## 5. Best Practices Checklist

### Content Generation ✅
- [ ] Use hierarchical structure with clear priorities
- [ ] Include pre-calculated aggregates in templates
- [ ] Implement consistent formatting across all transformers
- [ ] Add truncation indicators for long content
- [ ] Use semantic field names in templates

### Performance ✅
- [ ] Pre-calculate aggregates to avoid N+1 queries
- [ ] Use appropriate embedding models for different use cases
- [ ] Implement efficient chunking strategies
- [ ] Add caching for frequently accessed data
- [ ] Use batch processing for large datasets

### Maintainability ✅
- [ ] Use template-driven content generation
- [ ] Implement versioning for templates
- [ ] Add comprehensive logging and monitoring
- [ ] Create automated tests for content generation
- [ ] Document template syntax and available fields

### Scalability ✅
- [ ] Use metadata-aware vector indexing
- [ ] Implement efficient filtering strategies
- [ ] Design for multi-tenant isolation
- [ ] Plan for horizontal scaling
- [ ] Monitor and optimize query performance

## 6. Success Metrics

### Content Quality
- **Search Relevance**: Improved cosine similarity scores for relevant queries
- **Content Completeness**: Reduced truncation rates
- **Template Coverage**: Percentage of content generated via templates

### Performance
- **Query Latency**: < 100ms for typical searches
- **Processing Throughput**: > 1000 spans/second
- **Storage Efficiency**: Optimized vector storage usage

### Maintainability
- **Template Reuse**: > 80% of content via templates
- **Configuration Changes**: < 5 minutes for content format changes
- **Error Rates**: < 1% processing failures

## 7. Risk Mitigation

### Technical Risks
- **LLM Dependency**: Implement fallback summarization strategies
- **Model Performance**: A/B test different embedding models
- **Data Volume**: Implement efficient chunking and batching

### Operational Risks
- **Template Complexity**: Start with simple templates, evolve gradually
- **Performance Impact**: Monitor and optimize incrementally
- **Data Consistency**: Implement validation and rollback mechanisms

---

*This document should be updated as implementation progresses and new insights are gained from the research and testing phases.* 