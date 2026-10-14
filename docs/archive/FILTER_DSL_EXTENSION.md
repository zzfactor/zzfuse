# Filter DSL Extension: Unified Filtering for Database Routing and Signal Processing

## Overview

Extend the existing JSON-based Filter DSL to support both:
1. **Database Filter** (`config_otel_database.filter`) - Service routing based on incoming signal attributes
2. **VectorStore Filter** (`config_otel_processor.filter`) - Signal processing filtering for embedding

## Current Filter DSL

The existing Filter DSL supports:
- **Operators**: `=`, `IN`, `NOT IN`, `LIKE`, `>`, `<`, `>=`, `<=`, `BETWEEN`
- **Fields**: All OpenTelemetry signal fields (service_name, severity_text, timestamp_ns, etc.)
- **Format**: JSON with `{"field": {"op": "operator", "value": value}}`

## Proposed Unified Filter DSL

### 1. Database Filter (Service Routing)

**Purpose**: Route incoming OpenTelemetry signals to the correct database schema.

**Supported Fields**:
```json
{
  "service.name": {"op": "=", "value": "api-service"},
  "resource_attributes.environment": {"op": "=", "value": "production"},
  "resource_attributes.region": {"op": "IN", "value": ["us-east-1", "us-west-2"]},
  "resource_attributes.version": {"op": "LIKE", "value": "v2.%"},
  "attributes.deployment": {"op": "=", "value": "blue"}
}
```

**Routing Logic**:
- **Exact Match**: All conditions must match
- **Wildcard Support**: `{"service.name": {"op": "=", "value": "*"}}` matches all services
- **Priority**: First matching filter wins

### 2. VectorStore Filter (Signal Processing)

**Purpose**: Filter which signals get processed by the embedding worker.

**Supported Fields** (existing):
```json
{
  "severity_text": {"op": "=", "value": "ERROR"},
  "service_name": {"op": "=", "value": "critical-service"},
  "timestamp_ns": {"op": ">=", "value": 1700000000000},
  "body": {"op": "LIKE", "value": "%error%"},
  "attributes.error_code": {"op": "IN", "value": ["500", "502", "503"]}
}
```

## Implementation Changes

### 1. Extend Filter Parser

```go
// New function for database routing filters
func ParseDatabaseFilter(config map[string]interface{}) (*DatabaseFilter, error) {
    // Validate that only routing-relevant fields are used
    allowedFields := []string{
        "service.name", "service_name",
        "resource_attributes.*", "attributes.*"
    }
    
    return &DatabaseFilter{
        Conditions: config,
    }, nil
}

// Enhanced existing function for signal processing filters
func ParseSignalFilter(signalType string, config map[string]interface{}) (*SignalFilter, error) {
    // Use existing ParseEmbeddingFilter logic
    return ParseEmbeddingFilter(signalType, config)
}
```

### 2. Database Filter Usage

```go
// In service_resource_manager.go
func (srm *ServiceResourceManager) GetResourcesByAttributes(attrs map[string]interface{}) (*ServiceResources, bool) {
    for _, res := range srm.pools {
        if res.DatabaseFilter.Matches(attrs) {
            return res, true
        }
    }
    return nil, false
}

// New method on DatabaseFilter
func (df *DatabaseFilter) Matches(attrs map[string]interface{}) bool {
    // Check if all filter conditions match the incoming attributes
    for field, condition := range df.Conditions {
        if !df.matchesCondition(field, condition, attrs) {
            return false
        }
    }
    return true
}
```

### 3. VectorStore Filter Usage (Existing)

```go
// In embedding_worker_batch.go (existing)
func (w *EmbeddingWorker) processSignals(signals []map[string]interface{}) {
    for _, signal := range signals {
        if w.SignalFilter.ShouldProcess(signal) {
            // Process signal for embedding
        }
    }
}
```

## Configuration Examples

### Database Filter Examples

**1. Route by Service Name**:
```json
{
  "service.name": {"op": "=", "value": "api-service"}
}
```

**2. Route by Environment**:
```json
{
  "resource_attributes.environment": {"op": "=", "value": "production"}
}
```

**3. Route by Multiple Criteria**:
```json
{
  "service.name": {"op": "=", "value": "api-service"},
  "resource_attributes.environment": {"op": "=", "value": "production"},
  "resource_attributes.region": {"op": "=", "value": "us-east-1"}
}
```

**4. Wildcard (Default Schema)**:
```json
{
  "service.name": {"op": "=", "value": "*"}
}
```

### VectorStore Filter Examples

**1. Error Logs Only**:
```json
{
  "severity_text": {"op": "=", "value": "ERROR"}
}
```

**2. Critical Service Errors**:
```json
{
  "service_name": {"op": "=", "value": "critical-service"},
  "severity_text": {"op": "=", "value": "ERROR"}
}
```

**3. Recent Errors**:
```json
{
  "severity_text": {"op": "=", "value": "ERROR"},
  "timestamp_ns": {"op": ">=", "value": 1700000000000}
}
```

## Migration Strategy

### Phase 1: Extend Parser
1. Add `ParseDatabaseFilter()` function
2. Add `DatabaseFilter` struct with `Matches()` method
3. Update service resource manager to use new filter

### Phase 2: Update Configuration
1. Update default database filter to use DSL format
2. Update default vectorstore filter to use DSL format
3. Add validation for field restrictions per filter type

### Phase 3: Documentation
1. Update `SIGNAL_FILTERS.md` to include database filter examples
2. Add examples for both filter types
3. Document migration from simple key-value to DSL format

## Benefits

1. **Consistency**: Same DSL syntax for both filter types
2. **Flexibility**: Rich filtering capabilities for both routing and processing
3. **Extensibility**: Easy to add new operators or fields
4. **Validation**: Type-safe filter validation
5. **Performance**: Optimized SQL generation for both use cases

## Backward Compatibility

- **Database Filter**: Support both old format (`{"service.name": "api"}`) and new DSL format
- **VectorStore Filter**: Already uses DSL format, no changes needed
- **Migration**: Automatic conversion of old format to new format 