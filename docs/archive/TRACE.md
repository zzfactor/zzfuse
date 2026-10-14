# Full Trace Vectorization: Design & Implementation Strategy

| Version | Date | Status |
|---------|------|--------|
| 3.0 | January 2025 | Design Finalized |

## 1. Overview

This document presents a comprehensive strategy for vectorizing complete application traces as single, semantically rich units. The goal is to move beyond embedding individual signals and instead capture the full context of a request—including all associated spans, logs, metrics, and events—in one vector. This approach enables superior semantic search for complex debugging and system analysis while ensuring the resulting data fits within the constraints of modern embedding models.

The core of this design is a **flexible processing architecture** that supports multiple processing modes through a unified data pipeline.

## 2. Problem Statement

Current observability platforms often embed telemetry signals individually. This approach has significant limitations:

- **Loss of Context**: Relationships between spans, logs, and metrics within the same trace are lost.
- **Ineffective Search**: It's difficult to search for complex, multi-signal patterns (e.g., "a database query that was slow and logged a specific warning").
- **Incomplete Picture**: Analyzing a single signal provides a narrow view, making it hard to understand the full user request lifecycle.

This design directly addresses these gaps by creating cohesive representations that preserve signal relationships and context.

## 3. The Key Challenge: Context Window Management

The primary technical challenge is fitting a potentially massive trace, with hundreds of signals, into a fixed model context window. With the selected **mixedbread-ai/mxbai-embed-large-v1** model providing **30,000 tokens**, we have significant flexibility for content selection while maintaining high fidelity.

### ⚠️ A Note on Token Counting

**Success hinges on accurate token counting.** The system must use the official tokenizer for the target embedding model to precisely calculate content length. Simple character or word counts are not sufficient.

### Multi-Layered Content Optimization Strategy

#### Layer 1: Processing Mode Selection
Different processing modes determine the initial content scope:
- **all_signals**: Individual signals only
- **full_trace**: Complete trace with summarized correlated signals
- **full_trace_signals**: Complete trace with expanded correlated signals
- **full_trace_signals_attributes**: Complete trace with all attributes
- **full_span**: Individual spans with summarized correlated signals
- **full_span_signals**: Individual spans with expanded correlated signals
- **full_span_signals_attributes**: Individual spans with all attributes

#### Layer 2: Content Prioritization & Selective Truncation
This layer ensures that high-signal data is always preserved while low-signal data is shortened.

**Always Keep (Full Detail):**
- **Root Span**: The top-level context of the request.
- **Error Spans**: Any span with an error status, including its full attributes and events.
- **Error Logs**: Any log with ERROR or CRITICAL severity, including the full message and stack trace.

**Prioritize (Include if Space Permits):**
- **High-Latency Spans**: Spans with outlier durations (e.g., >P95).
- **Service Boundary Spans**: Spans representing network calls (span.kind = CLIENT or SERVER).

**Truncate (Shorten Less Critical Data):**
- Shorten long attribute values: `"sql.query": "SELECT * FROM users WHERE ... [truncated]"`
- Keep event names but drop their attributes.

#### Layer 3: Statistical & Structural Summarization
When dealing with high volumes of similar, low-priority items, this layer replaces them with statistical aggregates.

**Signal Grouping**: Collapse repeating, successful signals.
```
Example: Replace 50 redis.GET spans with: "Summarized 50 successful 'redis.GET' spans (avg duration: 2ms)."
```

**Log Grouping**: Aggregate verbose logs by level.
```
Example: Replace hundreds of debug logs with: "Summarized 125 DEBUG logs."
```

**Structural Pruning**: Collapse entire successful and fast sub-trees of a trace.
```
Example: "[Sub-trace for successful payment authorization, 25 spans, 85ms]"
```

#### Layer 4: Token Budget Management (Advanced)
For maximum robustness, a programmatic approach manages a "token budget" to guarantee the final output fits.

1. **Start with Full Content**: Begin with the complete trace content for the selected processing mode.
2. **Calculate Token Count**: Use the official tokenizer to count tokens.
3. **Apply Priority Rules**: If over budget, apply priority-based content selection.
4. **Summarize Low-Priority Content**: Replace detailed content with summaries.
5. **Finalize**: Ensure the final content fits within the token limit.

## 4. Proposed Architecture: Flexible Processing Pipeline

### Core Design Principle: "Build Once, Transform Many"

The architecture follows a unified data pipeline that builds complete trace content once, then transforms it based on processing mode and context window constraints.

```
Raw Trace Data → Complete Trace Content → Processing Mode Transform → Context Window Optimization → Final Vector
```

### Processing Modes

| Processing Mode | Description | Vector Count | Content Level |
|----------------|-------------|--------------|---------------|
| **all_signals** | One vector per individual signal (log, trace, metric, profile) | N vectors (one per signal) | Individual signals only |
| **full_trace** | Complete trace with **summarized** correlated signals | 1 vector per trace | Spans + summarized logs/metrics |
| **full_trace_signals** | Complete trace with **expanded** correlated signals | 1 vector per trace | Spans + full log bodies + metrics |
| **full_trace_signals_attributes** | Complete trace with **all attributes** | 1 vector per trace | Spans + full signals + all attributes |
| **full_span** | Individual spans with **summarized** correlated signals | N vectors per trace | Span + summarized logs/metrics |
| **full_span_signals** | Individual spans with **expanded** correlated signals | N vectors per trace | Span + full log bodies + metrics |
| **full_span_signals_attributes** | Individual spans with **all attributes** | N vectors per trace | Span + full signals + all attributes |

## 5. Current Implementation Status

### ✅ Completed Infrastructure

1. **Database Schema**: The `processing_mode` field has been successfully renamed from `purpose` in the `config_otel_processor` table
2. **Embedding Worker**: Robust worker with batch, trigger, and hybrid modes already implemented
3. **Configuration System**: Runtime configuration management through database tables
4. **Vector Store Integration**: Support for multiple vector stores (PGVector, Chroma, Qdrant)
5. **Template System**: Configurable content generation with signal-specific templates

### 🔄 Implementation Strategy

#### Phase 1: Core Infrastructure (Week 1)
- Implement unified trace content builder
- Add processing mode transformers
- Create content optimization pipeline
- Extend database layer with correlation methods

#### Phase 2: Processing Modes (Week 2-3)
- Implement `full_trace` and `full_span` modes
- Add `full_trace_signals` and `full_span_signals` modes
- Implement `full_trace_signals_attributes` and `full_span_signals_attributes` modes

#### Phase 3: Advanced Features (Week 4)
- Implement token counting with mxbai tokenizer
- Add priority-based content selection
- Performance optimization and testing

## 6. Unified Data Pipeline Architecture

### Core Design Principles

1. **"Build Once, Transform Many"**: Build complete trace content once, then transform it based on processing mode
2. **Resumable Processing**: Support pause/resume operations for long-running trace processing
3. **Observable Operations**: Comprehensive statistics and monitoring for all processing stages
4. **Graceful Degradation**: Handle failures and large traces without system impact

### Core Components

#### 1. Trace Content Builder
```go
type TraceContentBuilder struct {
    db          SQLDatabaseClient
    tokenizer   Tokenizer
    config      *ProcessingConfig
}

type CompleteTraceContent struct {
    TraceID      string
    ServiceName  string
    StartTime    int64
    EndTime      int64
    Duration     int64
    Spans        []SpanData
    Logs         []LogData
    Metrics      []MetricData
    Profiles     []ProfileData
    Summary      *TraceSummary
}

func (b *TraceContentBuilder) BuildCompleteContent(ctx context.Context, traceID string) (*CompleteTraceContent, error) {
    // Fetch all signals for the trace
    spans, err := b.db.FetchSpansByTraceID(ctx, traceID)
    logs, err := b.db.FetchLogsByTraceID(ctx, traceID)
    metrics, err := b.db.FetchMetricsByTraceID(ctx, traceID)
    profiles, err := b.db.FetchProfilesByTraceID(ctx, traceID)
    
    // Build complete trace content
    content := &CompleteTraceContent{
        TraceID: traceID,
        Spans:   spans,
        Logs:    logs,
        Metrics: metrics,
        Profiles: profiles,
    }
    
    // Calculate trace summary
    content.Summary = b.calculateTraceSummary(content)
    
    return content, nil
}
```

#### 2. Processing Mode Transformers
```go
type ProcessingModeTransformer interface {
    Transform(content *CompleteTraceContent) ([]ProcessedContent, error)
    GetMode() string
}

type ProcessedContent struct {
    Content     string
    Metadata    map[string]interface{}
    TokenCount  int
    Mode        string
}

// Full Trace Transformer
type FullTraceTransformer struct {
    config *ProcessingConfig
}

func (t *FullTraceTransformer) Transform(content *CompleteTraceContent) ([]ProcessedContent, error) {
    // Create single vector for entire trace with summarized signals
    template := t.config.GetTemplate("full_trace")
    
    // Summarize correlated signals
    summarizedLogs := t.summarizeLogs(content.Logs)
    summarizedMetrics := t.summarizeMetrics(content.Metrics)
    
    // Build template data
    data := map[string]interface{}{
        "trace_id": content.TraceID,
        "service_name": content.ServiceName,
        "duration_ms": content.Duration / 1000000,
        "spans": content.Spans,
        "summarized_logs": summarizedLogs,
        "summarized_metrics": summarizedMetrics,
        "summary": content.Summary,
    }
    
    // Render template
    rendered, err := template.Render(data)
    if err != nil {
        return nil, err
    }
    
    return []ProcessedContent{{
        Content: rendered,
        Metadata: data,
        Mode: "full_trace",
    }}, nil
}

// Full Span Transformer
type FullSpanTransformer struct {
    config *ProcessingConfig
}

func (t *FullSpanTransformer) Transform(content *CompleteTraceContent) ([]ProcessedContent, error) {
    var results []ProcessedContent
    template := t.config.GetTemplate("full_span")
    
    for _, span := range content.Spans {
        // Get correlated signals for this span
        correlatedLogs := t.getCorrelatedLogs(content.Logs, span)
        correlatedMetrics := t.getCorrelatedMetrics(content.Metrics, span)
        
        // Summarize correlated signals
        summarizedLogs := t.summarizeLogs(correlatedLogs)
        summarizedMetrics := t.summarizeMetrics(correlatedMetrics)
        
        // Build template data
        data := map[string]interface{}{
            "span": span,
            "trace_id": content.TraceID,
            "service_name": content.ServiceName,
            "summarized_logs": summarizedLogs,
            "summarized_metrics": summarizedMetrics,
            "trace_summary": content.Summary,
        }
        
        // Render template
        rendered, err := template.Render(data)
        if err != nil {
            return nil, err
        }
        
        results = append(results, ProcessedContent{
            Content: rendered,
            Metadata: data,
            Mode: "full_span",
        })
    }
    
    return results, nil
}
```

#### 3. Content Optimizer
```go
type ContentOptimizer struct {
    tokenizer   Tokenizer
    maxTokens   int
    priorities  []ContentPriority
}

type ContentPriority struct {
    Field       string
    Priority    float64
    Required    bool
    MaxLength   int
    Truncatable bool
}

func (o *ContentOptimizer) Optimize(content *ProcessedContent) (*ProcessedContent, error) {
    // Count tokens
    tokenCount, err := o.tokenizer.CountTokens(content.Content)
    if err != nil {
        return nil, err
    }
    
    // If within limits, return as-is
    if tokenCount <= o.maxTokens {
        content.TokenCount = tokenCount
        return content, nil
    }
    
    // Apply priority-based optimization
    optimized, err := o.applyPriorityOptimization(content)
    if err != nil {
        return nil, err
    }
    
    return optimized, nil
}

func (o *ContentOptimizer) applyPriorityOptimization(content *ProcessedContent) (*ProcessedContent, error) {
    // Parse content into sections
    sections := o.parseContentSections(content.Content)
    
    // Sort by priority
    sort.Slice(sections, func(i, j int) bool {
        return o.getPriority(sections[i]) > o.getPriority(sections[j])
    })
    
    // Build optimized content
    var optimizedContent strings.Builder
    remainingTokens := o.maxTokens
    
    for _, section := range sections {
        sectionTokens := o.tokenizer.CountTokens(section.Content)
        
        if sectionTokens <= remainingTokens {
            optimizedContent.WriteString(section.Content)
            remainingTokens -= sectionTokens
        } else if section.Required {
            // Truncate required content
            truncated := o.truncateContent(section.Content, remainingTokens)
            optimizedContent.WriteString(truncated)
            break
        } else {
            // Skip non-required content
            continue
        }
    }
    
    return &ProcessedContent{
        Content: optimizedContent.String(),
        Metadata: content.Metadata,
        TokenCount: o.maxTokens - remainingTokens,
        Mode: content.Mode,
    }, nil
}
```

#### 4. Processing State Manager
```go
type ProcessingState string

const (
    StatePending   ProcessingState = "pending"
    StateRunning   ProcessingState = "running"
    StatePaused    ProcessingState = "paused"
    StateCompleted ProcessingState = "completed"
    StateFailed    ProcessingState = "failed"
    StateCancelled ProcessingState = "cancelled"
)

type ProcessingJob struct {
    ID              string
    TraceID         string
    ProcessingMode  string
    State           ProcessingState
    CreatedAt       time.Time
    StartedAt       *time.Time
    PausedAt        *time.Time
    ResumedAt       *time.Time
    CompletedAt     *time.Time
    Progress        *ProcessingProgress
    Statistics      *ProcessingStatistics
    Error           *ProcessingError
    Metadata        map[string]interface{}
}

type ProcessingProgress struct {
    CurrentStep     string
    StepNumber      int
    TotalSteps      int
    ProcessedSpans  int
    TotalSpans      int
    ProcessedLogs   int
    TotalLogs       int
    ProcessedMetrics int
    TotalMetrics    int
    TokenCount      int
    EstimatedTimeRemaining time.Duration
}

type ProcessingStatistics struct {
    BuildTime       time.Duration
    TransformTime   time.Duration
    OptimizeTime    time.Duration
    TotalTime       time.Duration
    MemoryUsage     int64
    TokenCount      int
    SpanCount       int
    LogCount        int
    MetricCount     int
    ProfileCount    int
    ErrorCount      int
    WarningCount    int
}

type ProcessingError struct {
    Step        string
    Message     string
    Details     string
    Recoverable bool
    RetryCount  int
}

type ProcessingStateManager struct {
    db          SQLDatabaseClient
    cache       *sync.Map
    mutex       sync.RWMutex
}

func (m *ProcessingStateManager) CreateJob(ctx context.Context, traceID, mode string) (*ProcessingJob, error) {
    job := &ProcessingJob{
        ID:             generateJobID(),
        TraceID:        traceID,
        ProcessingMode: mode,
        State:          StatePending,
        CreatedAt:      time.Now(),
        Progress:       &ProcessingProgress{},
        Statistics:     &ProcessingStatistics{},
        Metadata:       make(map[string]interface{}),
    }
    
    err := m.db.SaveProcessingJob(ctx, job)
    if err != nil {
        return nil, err
    }
    
    m.cache.Store(job.ID, job)
    return job, nil
}

func (m *ProcessingStateManager) UpdateJobState(ctx context.Context, jobID string, state ProcessingState) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    job, exists := m.cache.Load(jobID)
    if !exists {
        return fmt.Errorf("job not found: %s", jobID)
    }
    
    processingJob := job.(*ProcessingJob)
    processingJob.State = state
    
    switch state {
    case StateRunning:
        now := time.Now()
        processingJob.StartedAt = &now
    case StatePaused:
        now := time.Now()
        processingJob.PausedAt = &now
    case StateCompleted:
        now := time.Now()
        processingJob.CompletedAt = &now
    }
    
    return m.db.UpdateProcessingJob(ctx, processingJob)
}

func (m *ProcessingStateManager) PauseJob(ctx context.Context, jobID string) error {
    return m.UpdateJobState(ctx, jobID, StatePaused)
}

func (m *ProcessingStateManager) ResumeJob(ctx context.Context, jobID string) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    job, exists := m.cache.Load(jobID)
    if !exists {
        return fmt.Errorf("job not found: %s", jobID)
    }
    
    processingJob := job.(*ProcessingJob)
    if processingJob.State != StatePaused {
        return fmt.Errorf("job is not paused: %s", processingJob.State)
    }
    
    now := time.Now()
    processingJob.ResumedAt = &now
    processingJob.State = StateRunning
    
    return m.db.UpdateProcessingJob(ctx, processingJob)
}

func (m *ProcessingStateManager) CancelJob(ctx context.Context, jobID string) error {
    return m.UpdateJobState(ctx, jobID, StateCancelled)
}

func (m *ProcessingStateManager) GetJob(ctx context.Context, jobID string) (*ProcessingJob, error) {
    // Try cache first
    if job, exists := m.cache.Load(jobID); exists {
        return job.(*ProcessingJob), nil
    }
    
    // Fallback to database
    job, err := m.db.GetProcessingJob(ctx, jobID)
    if err != nil {
        return nil, err
    }
    
    m.cache.Store(jobID, job)
    return job, nil
}

func (m *ProcessingStateManager) UpdateProgress(ctx context.Context, jobID string, progress *ProcessingProgress) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    job, exists := m.cache.Load(jobID)
    if !exists {
        return fmt.Errorf("job not found: %s", jobID)
    }
    
    processingJob := job.(*ProcessingJob)
    processingJob.Progress = progress
    
    return m.db.UpdateProcessingJobProgress(ctx, jobID, progress)
}

func (m *ProcessingStateManager) UpdateStatistics(ctx context.Context, jobID string, stats *ProcessingStatistics) error {
    m.mutex.Lock()
    defer m.mutex.Unlock()
    
    job, exists := m.cache.Load(jobID)
    if !exists {
        return fmt.Errorf("job not found: %s", jobID)
    }
    
    processingJob := job.(*ProcessingJob)
    processingJob.Statistics = stats
    
    return m.db.UpdateProcessingJobStatistics(ctx, jobID, stats)
}
```

#### 5. Statistics Collector
```go
type StatisticsCollector struct {
    db          SQLDatabaseClient
    metrics     *ProcessingMetrics
    mutex       sync.RWMutex
}

type ProcessingMetrics struct {
    TotalJobsProcessed    int64
    SuccessfulJobs        int64
    FailedJobs            int64
    CancelledJobs         int64
    AverageProcessingTime time.Duration
    TotalProcessingTime   time.Duration
    AverageTokenCount     int
    TotalTokenCount       int64
    AverageSpanCount      int
    TotalSpanCount        int64
    AverageLogCount       int
    TotalLogCount         int64
    AverageMetricCount    int
    TotalMetricCount      int64
    MemoryUsage           int64
    PeakMemoryUsage       int64
    ProcessingModeStats   map[string]*ModeStatistics
}

type ModeStatistics struct {
    JobsProcessed     int64
    AverageTime       time.Duration
    AverageTokens     int
    SuccessRate       float64
    ErrorRate         float64
}

func (c *StatisticsCollector) RecordJobCompletion(ctx context.Context, job *ProcessingJob) error {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    // Update global statistics
    c.metrics.TotalJobsProcessed++
    c.metrics.TotalProcessingTime += job.Statistics.TotalTime
    c.metrics.AverageProcessingTime = c.metrics.TotalProcessingTime / time.Duration(c.metrics.TotalJobsProcessed)
    
    c.metrics.TotalTokenCount += int64(job.Statistics.TokenCount)
    c.metrics.AverageTokenCount = int(c.metrics.TotalTokenCount / c.metrics.TotalJobsProcessed)
    
    c.metrics.TotalSpanCount += int64(job.Statistics.SpanCount)
    c.metrics.AverageSpanCount = int(c.metrics.TotalSpanCount / c.metrics.TotalJobsProcessed)
    
    c.metrics.TotalLogCount += int64(job.Statistics.LogCount)
    c.metrics.AverageLogCount = int(c.metrics.TotalLogCount / c.metrics.TotalJobsProcessed)
    
    c.metrics.TotalMetricCount += int64(job.Statistics.MetricCount)
    c.metrics.AverageMetricCount = int(c.metrics.TotalMetricCount / c.metrics.TotalJobsProcessed)
    
    // Update mode-specific statistics
    if c.metrics.ProcessingModeStats == nil {
        c.metrics.ProcessingModeStats = make(map[string]*ModeStatistics)
    }
    
    modeStats, exists := c.metrics.ProcessingModeStats[job.ProcessingMode]
    if !exists {
        modeStats = &ModeStatistics{}
        c.metrics.ProcessingModeStats[job.ProcessingMode] = modeStats
    }
    
    modeStats.JobsProcessed++
    modeStats.AverageTime = (modeStats.AverageTime*time.Duration(modeStats.JobsProcessed-1) + job.Statistics.TotalTime) / time.Duration(modeStats.JobsProcessed)
    modeStats.AverageTokens = (modeStats.AverageTokens*(int(modeStats.JobsProcessed)-1) + job.Statistics.TokenCount) / int(modeStats.JobsProcessed)
    
    // Update success/error rates
    switch job.State {
    case StateCompleted:
        c.metrics.SuccessfulJobs++
        modeStats.SuccessRate = float64(c.metrics.SuccessfulJobs) / float64(c.metrics.TotalJobsProcessed)
    case StateFailed:
        c.metrics.FailedJobs++
        modeStats.ErrorRate = float64(c.metrics.FailedJobs) / float64(c.metrics.TotalJobsProcessed)
    case StateCancelled:
        c.metrics.CancelledJobs++
    }
    
    // Update memory usage
    if job.Statistics.MemoryUsage > c.metrics.PeakMemoryUsage {
        c.metrics.PeakMemoryUsage = job.Statistics.MemoryUsage
    }
    c.metrics.MemoryUsage = job.Statistics.MemoryUsage
    
    return c.db.SaveProcessingStatistics(ctx, c.metrics)
}

func (c *StatisticsCollector) GetStatistics(ctx context.Context) (*ProcessingMetrics, error) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    return c.metrics, nil
}

func (c *StatisticsCollector) GetModeStatistics(ctx context.Context, mode string) (*ModeStatistics, error) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    if stats, exists := c.metrics.ProcessingModeStats[mode]; exists {
        return stats, nil
    }
    
    return nil, fmt.Errorf("no statistics for mode: %s", mode)
}
```

#### 6. Enhanced Unified Processing Pipeline
```go
type UnifiedProcessingPipeline struct {
    builder     *TraceContentBuilder
    transformers map[string]ProcessingModeTransformer
    optimizer   *ContentOptimizer
    stateManager *ProcessingStateManager
    statsCollector *StatisticsCollector
    logger      *log.Logger
}

func (p *UnifiedProcessingPipeline) ProcessTraceAsync(ctx context.Context, traceID string, mode string) (*ProcessingJob, error) {
    // Create processing job
    job, err := p.stateManager.CreateJob(ctx, traceID, mode)
    if err != nil {
        return nil, fmt.Errorf("failed to create job: %w", err)
    }
    
    // Start async processing
    go func() {
        p.processTraceWithState(ctx, job)
    }()
    
    return job, nil
}

func (p *UnifiedProcessingPipeline) ProcessTrace(ctx context.Context, traceID string, mode string) ([]ProcessedContent, error) {
    // Create processing job
    job, err := p.stateManager.CreateJob(ctx, traceID, mode)
    if err != nil {
        return nil, fmt.Errorf("failed to create job: %w", err)
    }
    
    // Process synchronously
    return p.processTraceWithState(ctx, job)
}

func (p *UnifiedProcessingPipeline) processTraceWithState(ctx context.Context, job *ProcessingJob) ([]ProcessedContent, error) {
    startTime := time.Now()
    var result []ProcessedContent
    
    defer func() {
        // Update final statistics
        job.Statistics.TotalTime = time.Since(startTime)
        p.statsCollector.RecordJobCompletion(ctx, job)
    }()
    
    // Check for cancellation
    if ctx.Err() != nil {
        p.stateManager.UpdateJobState(ctx, job.ID, StateCancelled)
        return nil, ctx.Err()
    }
    
    // Step 1: Update state and progress
    p.stateManager.UpdateJobState(ctx, job.ID, StateRunning)
    p.stateManager.UpdateProgress(ctx, job.ID, &ProcessingProgress{
        CurrentStep: "Building trace content",
        StepNumber:  1,
        TotalSteps:  4,
    })
    
    // Step 1: Build complete trace content
    buildStart := time.Now()
    content, err := p.builder.BuildCompleteContent(ctx, job.TraceID)
    if err != nil {
        job.Error = &ProcessingError{
            Step:        "build",
            Message:     "Failed to build trace content",
            Details:     err.Error(),
            Recoverable: false,
        }
        p.stateManager.UpdateJobState(ctx, job.ID, StateFailed)
        return nil, fmt.Errorf("failed to build trace content: %w", err)
    }
    job.Statistics.BuildTime = time.Since(buildStart)
    
    // Update progress
    p.stateManager.UpdateProgress(ctx, job.ID, &ProcessingProgress{
        CurrentStep:     "Transforming content",
        StepNumber:      2,
        TotalSteps:      4,
        TotalSpans:      len(content.Spans),
        TotalLogs:       len(content.Logs),
        TotalMetrics:    len(content.Metrics),
    })
    
    // Check for pause/cancellation
    if ctx.Err() != nil {
        p.stateManager.UpdateJobState(ctx, job.ID, StateCancelled)
        return nil, ctx.Err()
    }
    
    // Step 2: Transform based on processing mode
    transformStart := time.Now()
    transformer, exists := p.transformers[job.ProcessingMode]
    if !exists {
        job.Error = &ProcessingError{
            Step:        "transform",
            Message:     "Unknown processing mode",
            Details:     fmt.Sprintf("Mode: %s", job.ProcessingMode),
            Recoverable: false,
        }
        p.stateManager.UpdateJobState(ctx, job.ID, StateFailed)
        return nil, fmt.Errorf("unknown processing mode: %s", job.ProcessingMode)
    }
    
    processed, err := transformer.Transform(content)
    if err != nil {
        job.Error = &ProcessingError{
            Step:        "transform",
            Message:     "Failed to transform content",
            Details:     err.Error(),
            Recoverable: false,
        }
        p.stateManager.UpdateJobState(ctx, job.ID, StateFailed)
        return nil, fmt.Errorf("failed to transform content: %w", err)
    }
    job.Statistics.TransformTime = time.Since(transformStart)
    
    // Update progress
    p.stateManager.UpdateProgress(ctx, job.ID, &ProcessingProgress{
        CurrentStep:     "Optimizing content",
        StepNumber:      3,
        TotalSteps:      4,
        ProcessedSpans:  len(content.Spans),
        TotalSpans:      len(content.Spans),
        ProcessedLogs:   len(content.Logs),
        TotalLogs:       len(content.Logs),
        ProcessedMetrics: len(content.Metrics),
        TotalMetrics:    len(content.Metrics),
    })
    
    // Check for pause/cancellation
    if ctx.Err() != nil {
        p.stateManager.UpdateJobState(ctx, job.ID, StateCancelled)
        return nil, ctx.Err()
    }
    
    // Step 3: Optimize for context window
    optimizeStart := time.Now()
    var optimized []ProcessedContent
    for i, pc := range processed {
        // Check for pause/cancellation between optimizations
        if ctx.Err() != nil {
            p.stateManager.UpdateJobState(ctx, job.ID, StateCancelled)
            return nil, ctx.Err()
        }
        
        opt, err := p.optimizer.Optimize(&pc)
        if err != nil {
            job.Error = &ProcessingError{
                Step:        "optimize",
                Message:     "Failed to optimize content",
                Details:     fmt.Sprintf("Item %d: %s", i, err.Error()),
                Recoverable: true,
                RetryCount:  1,
            }
            p.stateManager.UpdateJobState(ctx, job.ID, StateFailed)
            return nil, fmt.Errorf("failed to optimize content item %d: %w", i, err)
        }
        optimized = append(optimized, *opt)
        
        // Update token count
        job.Statistics.TokenCount += opt.TokenCount
    }
    job.Statistics.OptimizeTime = time.Since(optimizeStart)
    
    // Update final progress
    p.stateManager.UpdateProgress(ctx, job.ID, &ProcessingProgress{
        CurrentStep:     "Completed",
        StepNumber:      4,
        TotalSteps:      4,
        ProcessedSpans:  len(content.Spans),
        TotalSpans:      len(content.Spans),
        ProcessedLogs:   len(content.Logs),
        TotalLogs:       len(content.Logs),
        ProcessedMetrics: len(content.Metrics),
        TotalMetrics:    len(content.Metrics),
        TokenCount:      job.Statistics.TokenCount,
    })
    
    // Update statistics
    job.Statistics.SpanCount = len(content.Spans)
    job.Statistics.LogCount = len(content.Logs)
    job.Statistics.MetricCount = len(content.Metrics)
    job.Statistics.ProfileCount = len(content.Profiles)
    
    // Count errors and warnings
    for _, span := range content.Spans {
        if span.StatusCode == "ERROR" {
            job.Statistics.ErrorCount++
        }
    }
    
    for _, log := range content.Logs {
        if log.Level == "WARN" {
            job.Statistics.WarningCount++
        } else if log.Level == "ERROR" {
            job.Statistics.ErrorCount++
        }
    }
    
    p.stateManager.UpdateStatistics(ctx, job.ID, job.Statistics)
    p.stateManager.UpdateJobState(ctx, job.ID, StateCompleted)
    
    return optimized, nil
}

// Control methods
func (p *UnifiedProcessingPipeline) PauseJob(ctx context.Context, jobID string) error {
    return p.stateManager.PauseJob(ctx, jobID)
}

func (p *UnifiedProcessingPipeline) ResumeJob(ctx context.Context, jobID string) error {
    return p.stateManager.ResumeJob(ctx, jobID)
}

func (p *UnifiedProcessingPipeline) CancelJob(ctx context.Context, jobID string) error {
    return p.stateManager.CancelJob(ctx, jobID)
}

func (p *UnifiedProcessingPipeline) GetJobStatus(ctx context.Context, jobID string) (*ProcessingJob, error) {
    return p.stateManager.GetJob(ctx, jobID)
}

func (p *UnifiedProcessingPipeline) GetStatistics(ctx context.Context) (*ProcessingMetrics, error) {
    return p.statsCollector.GetStatistics(ctx)
}

func (p *UnifiedProcessingPipeline) GetModeStatistics(ctx context.Context, mode string) (*ModeStatistics, error) {
    return p.statsCollector.GetModeStatistics(ctx, mode)
}
```

### Signal Correlation Strategy

Based on OpenTelemetry best practices, we use a multi-layered correlation approach:

#### 1. Primary Correlation: Trace Context
```go
// Correlate by trace_id and span_id
func (c *SQLDatabaseClient) FetchCorrelatedSignals(ctx context.Context, spanID, traceID string) (*CorrelatedSignals, error) {
    // Get span timing
    span, err := c.fetchSpanByID(ctx, spanID)
    if err != nil {
        return nil, err
    }
    
    // Fetch signals with trace context
    logs, err := c.fetchLogsByTraceAndSpan(ctx, traceID, spanID)
    metrics, err := c.fetchMetricsByTraceAndSpan(ctx, traceID, spanID)
    
    return &CorrelatedSignals{Logs: logs, Metrics: metrics}, nil
}
```

#### 2. Secondary Correlation: Time Window
```go
// Configurable time window around span
func (c *SQLDatabaseClient) FetchSignalsByTimeWindow(ctx context.Context, traceID string, startTime, endTime int64) (*CorrelatedSignals, error) {
    timeWindow := endTime - startTime
    buffer := timeWindow * 10 / 100 // 10% buffer
    
    windowStart := startTime - buffer
    windowEnd := endTime + buffer
    
    logs, err := c.fetchLogsByTraceAndTime(ctx, traceID, windowStart, windowEnd)
    metrics, err := c.fetchMetricsByTraceAndTime(ctx, traceID, windowStart, windowEnd)
    
    return &CorrelatedSignals{Logs: logs, Metrics: metrics}, nil
}
```

### Summarization Strategy

#### Log Summarization
```go
type LogSummary struct {
    TotalCount int
    ByLevel    map[string]int
    ErrorCount int
    WarnCount  int
    InfoCount  int
    DebugCount int
}

func summarizeLogs(logs []LogData) *LogSummary {
    summary := &LogSummary{
        ByLevel: make(map[string]int),
    }
    
    for _, log := range logs {
        summary.TotalCount++
        level := strings.ToUpper(log.Level)
        summary.ByLevel[level]++
        
        switch level {
        case "ERROR":
            summary.ErrorCount++
        case "WARN":
            summary.WarnCount++
        case "INFO":
            summary.InfoCount++
        case "DEBUG":
            summary.DebugCount++
        }
    }
    
    return summary
}

// Template format: "5 logs (2 WARN, 1 ERROR, 2 INFO)"
func formatLogSummary(summary *LogSummary) string {
    if summary.TotalCount == 0 {
        return "0 logs"
    }
    
    var parts []string
    if summary.ErrorCount > 0 {
        parts = append(parts, fmt.Sprintf("%d ERROR", summary.ErrorCount))
    }
    if summary.WarnCount > 0 {
        parts = append(parts, fmt.Sprintf("%d WARN", summary.WarnCount))
    }
    if summary.InfoCount > 0 {
        parts = append(parts, fmt.Sprintf("%d INFO", summary.InfoCount))
    }
    if summary.DebugCount > 0 {
        parts = append(parts, fmt.Sprintf("%d DEBUG", summary.DebugCount))
    }
    
    return fmt.Sprintf("%d logs (%s)", summary.TotalCount, strings.Join(parts, ", "))
}
```

## 7. Database Schema Extensions

### Job Management Tables
```sql
-- Processing jobs table
CREATE TABLE processing_jobs (
    id VARCHAR(50) PRIMARY KEY,
    trace_id VARCHAR(50) NOT NULL,
    processing_mode VARCHAR(50) NOT NULL,
    state VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    started_at TIMESTAMP,
    paused_at TIMESTAMP,
    resumed_at TIMESTAMP,
    completed_at TIMESTAMP,
    progress JSONB DEFAULT '{}',
    statistics JSONB DEFAULT '{}',
    error JSONB DEFAULT '{}',
    metadata JSONB DEFAULT '{}',
    INDEX idx_trace_id (trace_id),
    INDEX idx_state (state),
    INDEX idx_processing_mode (processing_mode),
    INDEX idx_created_at (created_at)
);

-- Processing statistics table
CREATE TABLE processing_statistics (
    id SERIAL PRIMARY KEY,
    total_jobs_processed BIGINT DEFAULT 0,
    successful_jobs BIGINT DEFAULT 0,
    failed_jobs BIGINT DEFAULT 0,
    cancelled_jobs BIGINT DEFAULT 0,
    average_processing_time_ms BIGINT DEFAULT 0,
    total_processing_time_ms BIGINT DEFAULT 0,
    average_token_count INTEGER DEFAULT 0,
    total_token_count BIGINT DEFAULT 0,
    average_span_count INTEGER DEFAULT 0,
    total_span_count BIGINT DEFAULT 0,
    average_log_count INTEGER DEFAULT 0,
    total_log_count BIGINT DEFAULT 0,
    average_metric_count INTEGER DEFAULT 0,
    total_metric_count BIGINT DEFAULT 0,
    memory_usage_bytes BIGINT DEFAULT 0,
    peak_memory_usage_bytes BIGINT DEFAULT 0,
    processing_mode_stats JSONB DEFAULT '{}',
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Mode-specific statistics table
CREATE TABLE processing_mode_statistics (
    id SERIAL PRIMARY KEY,
    processing_mode VARCHAR(50) NOT NULL,
    jobs_processed BIGINT DEFAULT 0,
    average_time_ms BIGINT DEFAULT 0,
    average_tokens INTEGER DEFAULT 0,
    success_rate DECIMAL(5,4) DEFAULT 0,
    error_rate DECIMAL(5,4) DEFAULT 0,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(processing_mode)
);
```

### Database Methods
```go
// Processing job methods
func (c *SQLDatabaseClient) SaveProcessingJob(ctx context.Context, job *ProcessingJob) error
func (c *SQLDatabaseClient) UpdateProcessingJob(ctx context.Context, job *ProcessingJob) error
func (c *SQLDatabaseClient) GetProcessingJob(ctx context.Context, jobID string) (*ProcessingJob, error)
func (c *SQLDatabaseClient) UpdateProcessingJobProgress(ctx context.Context, jobID string, progress *ProcessingProgress) error
func (c *SQLDatabaseClient) UpdateProcessingJobStatistics(ctx context.Context, jobID string, stats *ProcessingStatistics) error

// Statistics methods
func (c *SQLDatabaseClient) SaveProcessingStatistics(ctx context.Context, metrics *ProcessingMetrics) error
func (c *SQLDatabaseClient) GetProcessingStatistics(ctx context.Context) (*ProcessingMetrics, error)
func (c *SQLDatabaseClient) UpdateModeStatistics(ctx context.Context, mode string, stats *ModeStatistics) error
```

## 8. Configuration Structure

### Database Table Configuration
```sql
-- Main processing mode
processing_mode VARCHAR(50) DEFAULT 'all_signals'

-- Advanced configuration
correlation_config JSONB DEFAULT '{}',
content_optimization JSONB DEFAULT '{}',
template_config JSONB DEFAULT '{}'
```

### Metadata Configuration
```json
{
  "embedding_mode": "hybrid",
  "embedding_batch_interval": 5,
  "embedding_trigger_debounce_delay": 2,
  
  "correlation_config": {
    "time_window_ms": 5000,
    "use_span_context": true,
    "fallback_to_trace": true
  },
  
  "content_optimization": {
    "max_tokens": 30000,
    "enable_token_counting": true,
    "priority_rules": [
      {"field": "root_spans", "priority": 1.0, "required": true},
      {"field": "error_spans", "priority": 0.9, "required": true},
      {"field": "error_logs", "priority": 0.9, "required": true},
      {"field": "span_events", "priority": 0.8, "truncatable": true},
      {"field": "attributes", "priority": 0.6, "truncatable": true, "max_length": 500},
      {"field": "regular_logs", "priority": 0.4, "truncatable": true, "max_length": 200}
    ]
  },
  
  "template_config": {
    "full_trace": "custom_full_trace_template",
    "full_span": "custom_full_span_template",
    "attribute_filter": ["http.method", "db.system", "service.name"]
  }
}
```

## 8. Template Examples

### Full Trace Template (Summarized)
```
Trace {{.trace_id}} from {{.service_name}}:
Duration: {{.duration_ms}}ms, Spans: {{.span_count}}, Logs: {{.log_count}}, Metrics: {{.metric_count}}
{{if .has_errors}}Contains {{.error_count}} error spans and {{.error_log_count}} error logs{{end}}

{{range .spans}}{{.name}} ({{.span_id}}) -> {{if .parent_span_id}}{{.parent_span_id}}{{else}}ROOT{{end}}
  Kind: {{.span_kind}}, Duration: {{.duration_ms}}ms, Status: {{.status_code}}
  {{if .attributes}}Attributes: {{.attributes | formatMap}}{{end}}
  {{if .correlated_signals}}Correlated: {{.correlated_signals | formatSummary}}{{end}}
{{end}}

Signal Summary:
Logs: {{.summarized_logs | formatLogSummary}}
Metrics: {{.summarized_metrics | formatMetricSummary}}
```

### Full Span Template (Summarized)
```
Span: {{.span.name}} ({{.span.span_id}}) in service {{.service_name}}
Status: {{.span.status_code}}
Duration: {{.span.duration_ms}}ms, Kind: {{.span.span_kind}}
{{if .span.parent_span_id}}Parent: {{.span.parent_span_id}}{{end}}
Trace: {{.trace_id}}

{{if .span.attributes}}Attributes: {{.span.attributes | formatMap}}{{end}}
{{if .span.events}}Events: {{range .span.events}}{{.name}}; {{end}}{{end}}

Correlated Signals:
{{.summarized_logs | formatLogSummary}}
{{.summarized_metrics | formatMetricSummary}}

Trace Context: {{.trace_summary.total_spans}} spans, {{.trace_summary.duration_ms}}ms total
{{if .trace_summary.has_errors}}Contains errors{{end}}
```

### Full Trace Signals Template (Expanded)
```
Trace {{.trace_id}} from {{.service_name}}:
Duration: {{.duration_ms}}ms, Spans: {{.span_count}}, Logs: {{.log_count}}, Metrics: {{.metric_count}}
{{if .has_errors}}Contains {{.error_count}} error spans and {{.error_log_count}} error logs{{end}}

{{range .spans}}{{.name}} ({{.span_id}}) -> {{if .parent_span_id}}{{.parent_span_id}}{{else}}ROOT{{end}}
  Kind: {{.span_kind}}, Duration: {{.duration_ms}}ms, Status: {{.status_code}}
  {{if .attributes}}Attributes: {{.attributes | formatMap}}{{end}}
{{end}}

LOGS:
{{range .logs}}{{.level}}: {{.message}}
  {{if .attributes}}Attributes: {{.attributes | formatMap}}{{end}}
{{end}}

METRICS:
{{range .metrics}}{{.name}}: {{.value}} {{.unit}} ({{.type}})
  {{if .attributes}}Attributes: {{.attributes | formatMap}}{{end}}
{{end}}
```

## 9. API Endpoints for Job Control and Statistics

### Job Management Endpoints
```go
// Start async processing
POST /api/v1/traces/{traceID}/process
{
    "processing_mode": "full_trace",
    "async": true
}
Response: {
    "job_id": "job_123",
    "status": "pending",
    "created_at": "2025-01-23T10:00:00Z"
}

// Get job status
GET /api/v1/jobs/{jobID}
Response: {
    "id": "job_123",
    "trace_id": "trace_456",
    "processing_mode": "full_trace",
    "state": "running",
    "progress": {
        "current_step": "Transforming content",
        "step_number": 2,
        "total_steps": 4,
        "processed_spans": 50,
        "total_spans": 100
    },
    "statistics": {
        "build_time_ms": 1500,
        "transform_time_ms": 800,
        "total_time_ms": 2300
    },
    "created_at": "2025-01-23T10:00:00Z",
    "started_at": "2025-01-23T10:00:01Z"
}

// Pause job
POST /api/v1/jobs/{jobID}/pause
Response: {
    "job_id": "job_123",
    "state": "paused",
    "paused_at": "2025-01-23T10:00:30Z"
}

// Resume job
POST /api/v1/jobs/{jobID}/resume
Response: {
    "job_id": "job_123",
    "state": "running",
    "resumed_at": "2025-01-23T10:01:00Z"
}

// Cancel job
POST /api/v1/jobs/{jobID}/cancel
Response: {
    "job_id": "job_123",
    "state": "cancelled",
    "cancelled_at": "2025-01-23T10:01:30Z"
}

// List jobs
GET /api/v1/jobs?state=running&processing_mode=full_trace&limit=50&offset=0
Response: {
    "jobs": [
        {
            "id": "job_123",
            "trace_id": "trace_456",
            "processing_mode": "full_trace",
            "state": "running",
            "created_at": "2025-01-23T10:00:00Z"
        }
    ],
    "total": 150,
    "limit": 50,
    "offset": 0
}
```

### Statistics Endpoints
```go
// Get overall statistics
GET /api/v1/statistics
Response: {
    "total_jobs_processed": 1500,
    "successful_jobs": 1420,
    "failed_jobs": 60,
    "cancelled_jobs": 20,
    "average_processing_time_ms": 2500,
    "total_processing_time_ms": 3750000,
    "average_token_count": 8500,
    "total_token_count": 12750000,
    "average_span_count": 45,
    "total_span_count": 67500,
    "average_log_count": 120,
    "total_log_count": 180000,
    "average_metric_count": 30,
    "total_metric_count": 45000,
    "memory_usage_bytes": 1073741824,
    "peak_memory_usage_bytes": 2147483648,
    "processing_mode_stats": {
        "full_trace": {
            "jobs_processed": 500,
            "average_time_ms": 3000,
            "average_tokens": 12000,
            "success_rate": 0.95,
            "error_rate": 0.05
        },
        "full_span": {
            "jobs_processed": 1000,
            "average_time_ms": 2000,
            "average_tokens": 5000,
            "success_rate": 0.98,
            "error_rate": 0.02
        }
    }
}

// Get mode-specific statistics
GET /api/v1/statistics/modes/{processingMode}
Response: {
    "processing_mode": "full_trace",
    "jobs_processed": 500,
    "average_time_ms": 3000,
    "average_tokens": 12000,
    "success_rate": 0.95,
    "error_rate": 0.05,
    "recent_jobs": [
        {
            "id": "job_123",
            "trace_id": "trace_456",
            "state": "completed",
            "processing_time_ms": 2800,
            "token_count": 11500,
            "completed_at": "2025-01-23T10:00:00Z"
        }
    ]
}
```

## 10. Implementation Plan

### Phase 1: Core Infrastructure (Week 1)
1. **Trace Content Builder**: Implement unified trace content building
2. **Processing Mode Transformers**: Create transformers for each mode
3. **Content Optimizer**: Implement token counting and optimization
4. **Database Extensions**: Add correlation and signal fetching methods
5. **Processing State Manager**: Implement job state management and persistence
6. **Statistics Collector**: Implement comprehensive statistics tracking

### Phase 2: Processing Modes (Week 2-3)
1. **Summarized Modes**: Implement `full_trace` and `full_span`
2. **Expanded Modes**: Implement `full_trace_signals` and `full_span_signals`
3. **Attribute Modes**: Implement `full_trace_signals_attributes` and `full_span_signals_attributes`
4. **Enhanced Pipeline**: Integrate state management and statistics collection

### Phase 3: Advanced Features (Week 4)
1. **Token Counting**: Integrate mxbai tokenizer
2. **Priority System**: Implement content prioritization
3. **Performance Optimization**: Optimize database queries and content processing
4. **API Endpoints**: Implement job control and statistics endpoints
5. **Testing**: Comprehensive testing across all modes and job management features

## 11. Benefits and Trade-offs

### Benefits
- **Unified Architecture**: Single pipeline handles all processing modes
- **Flexible Content**: Easy to add new processing modes
- **Efficient Processing**: Build once, transform many
- **Context Preservation**: Maintains signal relationships
- **Scalable**: Handles large traces with intelligent optimization
- **Resumable Processing**: Support for pause/resume operations on long-running jobs
- **Comprehensive Monitoring**: Detailed statistics and progress tracking
- **Operational Control**: Full job lifecycle management with async processing
- **Observability**: Complete visibility into processing performance and bottlenecks

### Trade-offs
- **Complexity**: More complex than individual signal processing
- **Memory Usage**: Requires more memory for complete trace content and job state
- **Processing Time**: More time to build and transform content
- **Configuration**: More configuration options to manage
- **Database Overhead**: Additional tables and queries for job management
- **State Management**: Need to handle job state persistence and recovery

## 11. Success Criteria

- **Enhanced Search**: Successfully find complex patterns across multiple signals
- **Context Preservation**: Maintain relationships between spans, logs, and metrics
- **Performance**: Maintain or improve current embedding performance
- **Flexibility**: Support all processing modes with unified architecture
- **Scalability**: Handle traces with hundreds of signals efficiently

This implementation plan provides a clear roadmap for building a flexible, scalable trace processing system that follows OpenTelemetry best practices while providing the enhanced search capabilities needed for modern observability. 