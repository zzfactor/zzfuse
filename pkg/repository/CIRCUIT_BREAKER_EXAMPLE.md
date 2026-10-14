# Unified Circuit Breaker Pattern Examples

## Overview

The unified circuit breaker pattern has been implemented to provide robust error handling for **all resource types** in the system. Each schema has its own circuit breaker manager that tracks failures per resource type and automatically disables failing resources temporarily.

## Resource Types Supported

- **Database**: Main DB and schema-specific connections
- **Vector Store**: Vector store clients (pgvector, chroma, etc.)
- **Embedding**: Embedding providers (SBERT, OpenAI, etc.)
- **Worker**: Processing workers per schema

## Key Features

- **Per-Resource Isolation**: Each resource has its own circuit breaker
- **Resource-Specific Settings**: Different failure thresholds and timeouts per resource type
- **Automatic Recovery**: Failed resources are automatically re-enabled after a timeout
- **Failure Tracking**: Comprehensive statistics for success/failure rates
- **Graceful Degradation**: System continues operating even when some resources fail

## Circuit Breaker States

| State | Description | Behavior |
|-------|-------------|----------|
| **Closed** | Normal operation | All operations are allowed |
| **Open** | Failure detected | All operations are blocked |
| **Half-Open** | Testing recovery | Limited operations allowed |

## Configuration

### Resource-Specific Default Settings

| Resource Type | Failure Threshold | Reset Timeout | Rationale |
|---------------|-------------------|---------------|-----------|
| **Database** | 3 failures | 60 seconds | Critical resource, conservative settings |
| **Vector Store** | 5 failures | 30 seconds | Network operations, moderate settings |
| **Embedding** | 5 failures | 45 seconds | API calls, moderate settings |
| **Worker** | 10 failures | 15 seconds | Processing operations, liberal settings |

### Custom Configuration
```go
// Create circuit breaker with custom settings
baseCB := util.NewCircuitBreakerWithConfig(10, 60*time.Second) // 10 failures, 60s timeout
```

## Usage Examples

### 1. Basic Circuit Breaker Usage

```go
// Get circuit breaker for a specific resource
circuitBreaker := serviceResources.CircuitBreakerManager.GetOrCreateCircuitBreaker(
    ResourceTypeVectorStore, "pgvector_prod", "pgvector")

// Execute operation with circuit breaker protection
err := circuitBreaker.ExecuteWithMetrics(func() error {
    // Your vector store operation here
    return vectorStore.WriteEmbedding(ctx, embedding)
})

if err != nil {
    // Handle error - circuit breaker will track this failure
    log.Error("Vector store operation failed", "error", err)
}
```

### 2. Database Circuit Breaker Usage

```go
// Get circuit breaker for database operations
dbCircuitBreaker := serviceResources.CircuitBreakerManager.GetOrCreateCircuitBreaker(
    ResourceTypeDatabase, "main_db", "postgres")

// Execute database operation with circuit breaker protection
err := dbCircuitBreaker.ExecuteWithMetrics(func() error {
    return db.QueryContext(ctx, "SELECT * FROM signals")
})

if err != nil {
    log.Error("Database operation failed", "error", err)
}
```

### 3. Embedding Circuit Breaker Usage

```go
// Get circuit breaker for embedding operations
embeddingCircuitBreaker := serviceResources.CircuitBreakerManager.GetOrCreateCircuitBreaker(
    ResourceTypeEmbedding, "sbert_model", "sbert")

// Execute embedding operation with circuit breaker protection
err := embeddingCircuitBreaker.ExecuteWithMetrics(func() error {
    return embeddingClient.EmbedText(ctx, text)
})

if err != nil {
    log.Error("Embedding operation failed", "error", err)
}
```

### 2. Checking Circuit Breaker State

```go
// Check if circuit breaker is open
if circuitBreaker.IsOpen() {
    log.Warn("Circuit breaker is open - vector store temporarily disabled")
    return
}

// Get current state
state := circuitBreaker.GetState()
switch state {
case util.CircuitBreakerClosed:
    log.Info("Circuit breaker is closed - normal operation")
case util.CircuitBreakerOpen:
    log.Warn("Circuit breaker is open - operations blocked")
case util.CircuitBreakerHalfOpen:
    log.Info("Circuit breaker is half-open - testing recovery")
}
```

### 4. Getting Statistics

```go
// Get statistics for a specific circuit breaker
stats := circuitBreaker.GetStats()
log.Info("Circuit breaker statistics",
    "resource_type", stats["resource_type"],
    "resource_name", stats["resource_name"],
    "success_rate", stats["success_rate"],
    "total_operations", stats["total_operations"],
    "failure_count", stats["failure_count"])

// Get statistics for all circuit breakers in a schema
allStats := serviceResources.CircuitBreakerManager.GetAllStats()
for resourceKey, resourceStats := range allStats {
    log.Info("Resource circuit breaker stats",
        "key", resourceKey,
        "stats", resourceStats)
}

// Get statistics for specific resource types
vectorStoreStats := serviceResources.CircuitBreakerManager.GetStatsByResourceType(ResourceTypeVectorStore)
databaseStats := serviceResources.CircuitBreakerManager.GetStatsByResourceType(ResourceTypeDatabase)
embeddingStats := serviceResources.CircuitBreakerManager.GetStatsByResourceType(ResourceTypeEmbedding)
```

### 4. Manual Reset

```go
// Reset a specific circuit breaker
err := serviceResources.CircuitBreakerManager.ResetCircuitBreaker(
    "pgvector_prod", "pgvector")
if err != nil {
    log.Error("Failed to reset circuit breaker", "error", err)
}
```

### 5. Service Resource Manager Integration

```go
// Get circuit breaker statistics for all schemas
stats := serviceResourceManager.GetCircuitBreakerStats()
for schemaName, schemaStats := range stats {
    log.Info("Schema circuit breaker statistics",
        "schema", schemaName,
        "stats", schemaStats)
}

// Get comprehensive health check for all resources
health := serviceResourceManager.GetResourceHealthCheck()
for schemaName, schemaHealth := range health {
    log.Info("Schema resource health",
        "schema", schemaName,
        "health", schemaHealth)
    
    // Check specific resource types
    if databaseHealth, exists := schemaHealth["database"]; exists {
        log.Info("Database health", "health", databaseHealth)
    }
    if vectorStoreHealth, exists := schemaHealth["vectorstore"]; exists {
        log.Info("Vector store health", "health", vectorStoreHealth)
    }
    if embeddingHealth, exists := schemaHealth["embedding"]; exists {
        log.Info("Embedding health", "health", embeddingHealth)
    }
}
```

## Error Handling Patterns

### 1. Graceful Degradation

```go
func processWithFallback(ctx context.Context, data []byte) error {
    // Try primary vector store
    err := primaryCircuitBreaker.ExecuteWithMetrics(func() error {
        return primaryVectorStore.Write(ctx, data)
    })
    
    if err != nil {
        log.Warn("Primary vector store failed, trying secondary", "error", err)
        
        // Try secondary vector store
        err = secondaryCircuitBreaker.ExecuteWithMetrics(func() error {
            return secondaryVectorStore.Write(ctx, data)
        })
        
        if err != nil {
            return fmt.Errorf("both vector stores failed: %w", err)
        }
    }
    
    return nil
}
```

### 2. Batch Processing with Circuit Breakers

```go
func processBatch(ctx context.Context, embeddings []Embedding) error {
    var failedEmbeddings []Embedding
    
    for _, embedding := range embeddings {
        err := circuitBreaker.ExecuteWithMetrics(func() error {
            return vectorStore.WriteEmbedding(ctx, embedding)
        })
        
        if err != nil {
            failedEmbeddings = append(failedEmbeddings, embedding)
            log.Error("Failed to write embedding", "error", err)
        }
    }
    
    // Retry failed embeddings later
    if len(failedEmbeddings) > 0 {
        log.Info("Some embeddings failed, will retry later", 
            "failed_count", len(failedEmbeddings))
    }
    
    return nil
}
```

## Monitoring and Alerting

### 1. Circuit Breaker State Monitoring

```go
// Monitor circuit breaker states
func monitorCircuitBreakers() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := serviceResourceManager.GetCircuitBreakerStats()
        
        for schemaName, schemaStats := range stats {
            for vectorStoreKey, vectorStoreStats := range schemaStats {
                state := vectorStoreStats["state"].(util.CircuitBreakerState)
                successRate := vectorStoreStats["success_rate"].(float64)
                
                // Alert on open circuit breakers
                if state == util.CircuitBreakerOpen {
                    log.Error("Circuit breaker is open",
                        "schema", schemaName,
                        "vectorstore", vectorStoreKey)
                }
                
                // Alert on low success rates
                if successRate < 0.8 {
                    log.Warn("Low success rate detected",
                        "schema", schemaName,
                        "vectorstore", vectorStoreKey,
                        "success_rate", successRate)
                }
            }
        }
    }
}
```

### 2. Metrics Integration

```go
// Example metrics that can be collected
type CircuitBreakerMetrics struct {
    State         int     // 0=closed, 1=half-open, 2=open
    SuccessRate   float64 // 0.0 to 1.0
    FailureCount  int64
    SuccessCount  int64
    TotalOperations int64
}

// Export metrics for monitoring systems
func exportCircuitBreakerMetrics() map[string]CircuitBreakerMetrics {
    stats := serviceResourceManager.GetCircuitBreakerStats()
    metrics := make(map[string]CircuitBreakerMetrics)
    
    for schemaName, schemaStats := range stats {
        for vectorStoreKey, vectorStoreStats := range schemaStats {
            key := fmt.Sprintf("%s_%s", schemaName, vectorStoreKey)
            metrics[key] = CircuitBreakerMetrics{
                State:           vectorStoreStats["state"].(int),
                SuccessRate:     vectorStoreStats["success_rate"].(float64),
                FailureCount:    vectorStoreStats["failure_count"].(int64),
                SuccessCount:    vectorStoreStats["success_count"].(int64),
                TotalOperations: vectorStoreStats["total_operations"].(int64),
            }
        }
    }
    
    return metrics
}
```

## Best Practices

### 1. Appropriate Failure Thresholds
- **High-traffic systems**: Use higher thresholds (10-20 failures)
- **Low-traffic systems**: Use lower thresholds (3-5 failures)
- **Critical systems**: Use conservative thresholds (2-3 failures)

### 2. Reset Timeout Considerations
- **Network issues**: Use shorter timeouts (15-30 seconds)
- **Service issues**: Use longer timeouts (60-300 seconds)
- **Database issues**: Use medium timeouts (30-60 seconds)

### 3. Monitoring and Alerting
- Monitor circuit breaker states continuously
- Alert on open circuit breakers
- Track success rates and failure patterns
- Set up automatic recovery notifications

### 4. Testing Circuit Breakers
```go
// Test circuit breaker behavior
func testCircuitBreaker() {
    circuitBreaker := NewVectorStoreCircuitBreakerManager(logger)
    
    // Simulate failures
    for i := 0; i < 6; i++ {
        err := circuitBreaker.ExecuteWithMetrics(func() error {
            return fmt.Errorf("simulated failure %d", i)
        })
        log.Info("Simulated failure", "attempt", i, "error", err)
    }
    
    // Circuit breaker should now be open
    if circuitBreaker.IsOpen() {
        log.Info("Circuit breaker opened as expected")
    }
}
```

## Troubleshooting

### Common Issues

1. **Circuit breaker not opening**: Check failure threshold configuration
2. **Circuit breaker not closing**: Check reset timeout configuration
3. **High false positives**: Increase failure threshold
4. **Slow recovery**: Decrease reset timeout

### Debug Information

```go
// Get detailed debug information
stats := circuitBreaker.GetStats()
log.Info("Circuit breaker debug info",
    "state", stats["state"],
    "failure_count", stats["failure_count"],
    "success_count", stats["success_count"],
    "last_failure_time", stats["last_failure_time"],
    "last_success_time", stats["last_success_time"])
``` 