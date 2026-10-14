# Dynamic Resource-Aware Template System Architecture

## Executive Summary

This document outlines a comprehensive refactoring plan to implement dynamic, resource-aware template registration and resolution. The proposed architecture enables templates to be dynamically registered based on resource attributes, providing unprecedented flexibility for telemetry data processing while maintaining high performance and backward compatibility.

## Current Architecture Limitations

### 1. **Static Template Loading**
- Templates loaded once at startup from `admin.config_processor_template`
- No runtime template registration or updates
- Global template scope across all services and environments

### 2. **No Resource-Aware Routing**
- All workers use identical templates regardless of resource attributes
- No service-specific, environment-specific, or custom attribute-based template selection
- Limited flexibility for multi-tenant or multi-environment deployments

### 3. **Performance Bottlenecks**
- Template resolution doesn't leverage resource attribute patterns
- No caching of resource-to-template mappings
- Inefficient template lookup for high-throughput scenarios

## Proposed Architecture

### 1. **Core Components**

#### **A. Dynamic Template Registry**
```go
type DynamicTemplateRegistry struct {
    templates      map[string]*TemplateConfig
    resourceIndex  *ResourceAttributeIndex
    cache          *TemplateResolutionCache
    mutex          sync.RWMutex
    metrics        *TemplateRegistryMetrics
}

type TemplateConfig struct {
    ID              string
    Name            string
    Content         map[string]string
    Metadata        map[string][]string
    ResourceMatcher *ResourceMatcher
    Priority        int
    Active          bool
    CreatedAt       time.Time
    UpdatedAt       time.Time
}

type ResourceMatcher struct {
    ServiceName        *string
    ServiceVersion     *string
    Environment        *string
    CustomAttributes   map[string]interface{}
    MatchStrategy      MatchStrategy // EXACT, PATTERN, WILDCARD
}
```

#### **B. Resource Attribute Index**
```go
type ResourceAttributeIndex struct {
    serviceIndex    map[string][]*TemplateConfig
    envIndex        map[string][]*TemplateConfig
    customIndex     map[string][]*TemplateConfig
    priorityQueue   *PriorityQueue[*TemplateConfig]
}

type MatchStrategy int

const (
    MatchExact MatchStrategy = iota
    MatchPattern
    MatchWildcard
    MatchRegex
)
```

#### **C. Template Resolution Cache**
```go
type TemplateResolutionCache struct {
    cache    map[string]*CachedResolution
    ttl      time.Duration
    maxSize  int
    mutex    sync.RWMutex
}

type CachedResolution struct {
    Template     *TemplateConfig
    ResourceKey  string
    ExpiresAt    time.Time
    HitCount     int64
}
```

### 2. **Database Schema Design**

#### **A. Resource Template Mapping Table**
```sql
CREATE TABLE admin.resource_template_mapping (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    
    -- Resource matching criteria
    resource_pattern JSONB NOT NULL,
    match_strategy VARCHAR(50) DEFAULT 'exact',
    
    -- Template content
    template_content JSONB NOT NULL,
    template_metadata JSONB,
    
    -- Control fields
    priority INTEGER DEFAULT 0,
    active BOOLEAN DEFAULT true,
    
    -- Audit fields
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    created_by VARCHAR(255),
    version INTEGER DEFAULT 1,
    
    -- Constraints
    CONSTRAINT valid_resource_pattern CHECK (jsonb_typeof(resource_pattern) = 'object'),
    CONSTRAINT valid_template_content CHECK (jsonb_typeof(template_content) = 'object'),
    CONSTRAINT priority_range CHECK (priority >= 0 AND priority <= 1000)
);

-- Indexes for efficient querying
CREATE INDEX idx_resource_template_active ON admin.resource_template_mapping(active) WHERE active = true;
CREATE INDEX idx_resource_template_priority ON admin.resource_template_mapping(priority DESC);
CREATE INDEX idx_resource_template_pattern ON admin.resource_template_mapping USING GIN (resource_pattern);
CREATE INDEX idx_resource_template_strategy ON admin.resource_template_mapping(match_strategy);
```

#### **B. Template Versioning Table**
```sql
CREATE TABLE admin.template_versions (
    id SERIAL PRIMARY KEY,
    template_id INTEGER REFERENCES admin.resource_template_mapping(id),
    version INTEGER NOT NULL,
    content JSONB NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    created_by VARCHAR(255),
    
    UNIQUE(template_id, version)
);
```

### 3. **Template Resolution Algorithm**

#### **A. Resolution Strategy**
```go
func (r *DynamicTemplateRegistry) ResolveTemplate(
    signalType string,
    resourceAttrs map[string]interface{},
) (*TemplateConfig, error) {
    
    // 1. Check cache first
    if cached := r.cache.Get(resourceAttrs); cached != nil {
        return cached.Template, nil
    }
    
    // 2. Build resolution candidates
    candidates := r.buildResolutionCandidates(resourceAttrs)
    
    // 3. Apply resolution rules
    template := r.applyResolutionRules(candidates, signalType)
    
    // 4. Cache result
    r.cache.Set(resourceAttrs, template)
    
    return template, nil
}

func (r *DynamicTemplateRegistry) buildResolutionCandidates(
    resourceAttrs map[string]interface{},
) []*TemplateConfig {
    
    candidates := make([]*TemplateConfig, 0)
    
    // Exact matches (highest priority)
    if exact := r.findExactMatches(resourceAttrs); len(exact) > 0 {
        candidates = append(candidates, exact...)
    }
    
    // Pattern matches
    if patterns := r.findPatternMatches(resourceAttrs); len(patterns) > 0 {
        candidates = append(candidates, patterns...)
    }
    
    // Wildcard matches
    if wildcards := r.findWildcardMatches(resourceAttrs); len(wildcards) > 0 {
        candidates = append(candidates, wildcards...)
    }
    
    return candidates
}
```

#### **B. Resolution Rules**
```go
type ResolutionRule struct {
    Name        string
    Priority    int
    Condition   func(*TemplateConfig, map[string]interface{}) bool
    Weight      float64
}

var DefaultResolutionRules = []ResolutionRule{
    {
        Name:     "ExactServiceMatch",
        Priority: 100,
        Condition: func(tc *TemplateConfig, attrs map[string]interface{}) bool {
            return tc.ResourceMatcher.ServiceName != nil &&
                   *tc.ResourceMatcher.ServiceName == attrs["service.name"]
        },
        Weight: 1.0,
    },
    {
        Name:     "EnvironmentMatch",
        Priority: 80,
        Condition: func(tc *TemplateConfig, attrs map[string]interface{}) bool {
            return tc.ResourceMatcher.Environment != nil &&
                   *tc.ResourceMatcher.Environment == attrs["environment"]
        },
        Weight: 0.8,
    },
    {
        Name:     "CustomAttributeMatch",
        Priority: 60,
        Condition: func(tc *TemplateConfig, attrs map[string]interface{}) bool {
            return r.matchCustomAttributes(tc.ResourceMatcher.CustomAttributes, attrs)
        },
        Weight: 0.6,
    },
}
```

### 4. **Enhanced Template Engine**

#### **A. Resource-Aware Template Engine**
```go
type ResourceAwareTemplateEngine struct {
    baseEngine    *ContentTemplateEngine
    registry      *DynamicTemplateRegistry
    fallbackCache map[string]*template.Template
    metrics       *TemplateEngineMetrics
}

func (e *ResourceAwareTemplateEngine) RenderWithResource(
    signalType string,
    data map[string]interface{},
    resourceAttrs map[string]interface{},
) (string, error) {
    
    start := time.Now()
    defer func() {
        e.metrics.RecordRenderDuration(signalType, time.Since(start))
    }()
    
    // 1. Resolve template based on resource attributes
    templateConfig, err := e.registry.ResolveTemplate(signalType, resourceAttrs)
    if err != nil {
        e.metrics.RecordResolutionError(signalType, err)
        return e.renderFallback(signalType, data)
    }
    
    // 2. Get template content
    content, exists := templateConfig.Content[signalType]
    if !exists {
        e.metrics.RecordMissingTemplate(signalType, templateConfig.ID)
        return e.renderFallback(signalType, data)
    }
    
    // 3. Render with resolved template
    result, err := e.baseEngine.RenderTemplate(content, data)
    if err != nil {
        e.metrics.RecordRenderError(signalType, err)
        return e.renderFallback(signalType, data)
    }
    
    e.metrics.RecordSuccessfulRender(signalType, templateConfig.ID)
    return result, nil
}
```

#### **B. Template Metrics and Monitoring**
```go
type TemplateEngineMetrics struct {
    renderDuration    *prometheus.HistogramVec
    resolutionErrors  *prometheus.CounterVec
    renderErrors      *prometheus.CounterVec
    cacheHits         *prometheus.CounterVec
    cacheMisses       *prometheus.CounterVec
    templateUsage     *prometheus.CounterVec
}

func (m *TemplateEngineMetrics) RecordRenderDuration(signalType string, duration time.Duration) {
    m.renderDuration.WithLabelValues(signalType).Observe(duration.Seconds())
}

func (m *TemplateEngineMetrics) RecordSuccessfulRender(signalType, templateID string) {
    m.templateUsage.WithLabelValues(signalType, templateID).Inc()
}
```

### 5. **API Design for Dynamic Registration**

#### **A. REST API Endpoints**
```go
// Template Management API
type TemplateAPI struct {
    registry *DynamicTemplateRegistry
    metrics  *TemplateAPIMetrics
}

// Register new template
POST /api/v1/templates
{
    "name": "payment-service-templates",
    "description": "Templates for payment service operations",
    "resource_pattern": {
        "service.name": "payment-service",
        "environment": "production"
    },
    "match_strategy": "exact",
    "template_content": {
        "spans": "Payment operation {{.name}} took {{.duration_ms}}ms. Amount: {{.attributes.amount}}"
    },
    "priority": 100,
    "active": true
}

// Update existing template
PUT /api/v1/templates/{id}
{
    "template_content": {
        "spans": "Updated payment template: {{.name}} - {{.duration_ms}}ms"
    },
    "version": 2
}

// List templates with filtering
GET /api/v1/templates?service=payment-service&environment=production&active=true

// Delete template
DELETE /api/v1/templates/{id}
```

#### **B. Template Validation**
```go
type TemplateValidator struct {
    syntaxChecker    *TemplateSyntaxChecker
    variableChecker  *TemplateVariableChecker
    performanceChecker *TemplatePerformanceChecker
}

func (v *TemplateValidator) ValidateTemplate(config *TemplateConfig) []ValidationError {
    var errors []ValidationError
    
    // Syntax validation
    if syntaxErrs := v.syntaxChecker.Check(config.Content); len(syntaxErrs) > 0 {
        errors = append(errors, syntaxErrs...)
    }
    
    // Variable validation
    if varErrs := v.variableChecker.Check(config.Content, config.Metadata); len(varErrs) > 0 {
        errors = append(errors, varErrs...)
    }
    
    // Performance validation
    if perfErrs := v.performanceChecker.Check(config.Content); len(perfErrs) > 0 {
        errors = append(errors, perfErrs...)
    }
    
    return errors
}
```

### 6. **Performance Optimizations**

#### **A. Multi-Level Caching**
```go
type MultiLevelCache struct {
    l1Cache *LRUCache // In-memory cache for hot templates
    l2Cache *RedisCache // Distributed cache for shared templates
    l3Cache *DatabaseCache // Database cache for cold templates
}

func (c *MultiLevelCache) Get(key string) (*TemplateConfig, error) {
    // L1: In-memory cache
    if template := c.l1Cache.Get(key); template != nil {
        return template, nil
    }
    
    // L2: Redis cache
    if template := c.l2Cache.Get(key); template != nil {
        c.l1Cache.Set(key, template) // Populate L1
        return template, nil
    }
    
    // L3: Database cache
    if template := c.l3Cache.Get(key); template != nil {
        c.l2Cache.Set(key, template) // Populate L2
        c.l1Cache.Set(key, template) // Populate L1
        return template, nil
    }
    
    return nil, ErrTemplateNotFound
}
```

#### **B. Template Precompilation**
```go
type TemplatePrecompiler struct {
    compiler *TemplateCompiler
    pool     *sync.Pool
}

func (p *TemplatePrecompiler) PrecompileTemplates(templates []*TemplateConfig) error {
    for _, template := range templates {
        for signalType, content := range template.Content {
            compiled, err := p.compiler.Compile(content)
            if err != nil {
                return fmt.Errorf("failed to compile template %s: %w", signalType, err)
            }
            template.CompiledTemplates[signalType] = compiled
        }
    }
    return nil
}
```

### 7. **Integration with Existing System**

#### **A. Embedding Worker Integration**
```go
func (w *EmbeddingWorker) extractTextFromTraceWithProcessingMode(sig map[string]interface{}) string {
    // ... existing code ...
    
    // Use resource-aware template engine
    if w.resourceAwareEngine != nil {
        resourceAttrs := extractResourceAttributes(sig)
        return w.resourceAwareEngine.RenderWithResource(
            "spans", 
            templateData, 
            resourceAttrs,
        )
    }
    
    // Fallback to existing template system
    return w.extractTextWithFallback(sig)
}

func extractResourceAttributes(sig map[string]interface{}) map[string]interface{} {
    if resource, ok := sig["resource"].(map[string]interface{}); ok {
        return resource
    }
    return make(map[string]interface{})
}
```

#### **B. Backward Compatibility Layer**
```go
type CompatibilityLayer struct {
    legacyEngine    *ContentTemplateEngine
    resourceEngine  *ResourceAwareTemplateEngine
    config          *CompatibilityConfig
}

func (c *CompatibilityLayer) Render(
    signalType string,
    data map[string]interface{},
    resourceAttrs map[string]interface{},
) (string, error) {
    
    // Check if resource-aware templates are enabled
    if c.config.EnableResourceAwareTemplates {
        return c.resourceEngine.RenderWithResource(signalType, data, resourceAttrs)
    }
    
    // Fallback to legacy system
    return c.legacyEngine.Render(signalType, data)
}
```

### 8. **Migration Strategy**

#### **Phase 1: Foundation (Weeks 1-2)**
- Implement `DynamicTemplateRegistry` core components
- Create database schema for resource template mapping
- Build basic template resolution algorithm
- Add comprehensive unit tests

#### **Phase 2: Integration (Weeks 3-4)**
- Integrate with existing `EmbeddingWorker`
- Implement `ResourceAwareTemplateEngine`
- Add template validation and error handling
- Create monitoring and metrics

#### **Phase 3: API and Management (Weeks 5-6)**
- Implement REST API for template management
- Add template versioning and rollback capabilities
- Create admin interface for template management
- Add template performance monitoring

#### **Phase 4: Optimization (Weeks 7-8)**
- Implement multi-level caching
- Add template precompilation
- Optimize resolution algorithms
- Performance testing and tuning

### 9. **Benefits and Impact**

#### **A. Flexibility**
- Service-specific templates based on resource attributes
- Environment-specific customization (dev, staging, prod)
- Dynamic template updates without application restart
- Support for complex resource attribute patterns

#### **B. Performance**
- Intelligent caching of template resolutions
- Precompiled templates for faster rendering
- Optimized resource attribute matching
- Reduced template lookup overhead

#### **C. Scalability**
- Support for thousands of resource-specific templates
- Distributed caching for multi-instance deployments
- Efficient database indexing for template queries
- Horizontal scaling capabilities

#### **D. Maintainability**
- Clear separation of concerns
- Comprehensive monitoring and metrics
- Template validation and error handling
- Version control and rollback capabilities

### 10. **Risk Mitigation**

#### **A. Backward Compatibility**
- Maintain existing template system alongside new system
- Gradual migration path for existing templates
- Fallback mechanisms for template resolution failures

#### **B. Performance Risks**
- Comprehensive performance testing
- Monitoring and alerting for template resolution times
- Circuit breakers for template registry failures

#### **C. Data Consistency**
- Transactional template updates
- Template validation before registration
- Rollback capabilities for failed template updates

### 11. **Example Use Cases**

#### **A. Service-Specific Templates**
```json
{
    "name": "payment-service-spans",
    "resource_pattern": {
        "service.name": "payment-service",
        "environment": "production"
    },
    "template_content": {
        "spans": "Payment {{.name}} processed in {{.duration_ms}}ms. Amount: ${{.attributes.amount}}. Status: {{.status_code}}"
    }
}
```

#### **B. Environment-Specific Templates**
```json
{
    "name": "dev-environment-logs",
    "resource_pattern": {
        "environment": "development"
    },
    "template_content": {
        "logs": "DEV: {{.service_name}} - {{.level}}: {{.message}} [{{.timestamp}}]"
    }
}
```

#### **C. Custom Attribute Templates**
```json
{
    "name": "high-value-transactions",
    "resource_pattern": {
        "service.name": "payment-service",
        "attributes.transaction_type": "high_value"
    },
    "template_content": {
        "spans": "HIGH VALUE: {{.name}} - Amount: ${{.attributes.amount}} - Duration: {{.duration_ms}}ms"
    }
}
```

## Conclusion

This architecture provides a robust, scalable, and flexible foundation for dynamic resource-aware template management. The proposed design addresses current limitations while maintaining high performance and backward compatibility. The phased implementation approach ensures minimal disruption to existing systems while delivering incremental value.

The system will enable unprecedented customization of telemetry data processing based on resource attributes, making it ideal for multi-tenant, multi-environment deployments with diverse template requirements.

### **Next Steps**

1. **Review and Approval**: Stakeholder review of the proposed architecture
2. **Detailed Design**: Create detailed technical specifications for each component
3. **Proof of Concept**: Implement core components to validate the approach
4. **Implementation Planning**: Create detailed implementation timeline and resource allocation
5. **Testing Strategy**: Define comprehensive testing approach including performance benchmarks

### **Success Metrics**

- **Performance**: Template resolution time < 1ms for 95% of requests
- **Scalability**: Support for 10,000+ resource-specific templates
- **Reliability**: 99.9% uptime for template resolution service
- **Flexibility**: Support for 5+ different resource attribute matching strategies
- **Backward Compatibility**: Zero downtime migration from existing template system 