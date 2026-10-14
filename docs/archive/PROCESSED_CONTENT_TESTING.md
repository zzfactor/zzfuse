# Processed Content Testing Guide

## 🎯 **Overview**

This guide explains how to use the processed content testing functionality to understand what data is being collected for each processing mode. The system now stores processed content in the database for **all processing modes** (including span modes) to help with testing and analysis.

## 🏗️ **Architecture**

### **Components**

1. **ProcessedContentService**: Handles storing/retrieving processed content
2. **TraceProcessor**: Processes spans and stores content
3. **Processing Transformers**: Generate content for different modes
4. **Database Storage**: Uses `processed_content` column in traces table

### **Data Flow**

```
Span Completes → ProcessSpanAndStore() → 
├─ Generate content based on mode
├─ Store in processed_content column
└─ Retrieve for analysis
```

## 📊 **Stored Data Structure**

Each processed content entry is stored as JSON in the `processed_content` column:

```json
{
  "processing_mode": "full_span_signals",
  "span_id": "span-123",
  "trace_id": "trace-456",
  "content": "Generated content for this span...",
  "metadata": {
    "span_summary": {
      "span_name": "database-query",
      "duration_ms": 150,
      "status_code": 0,
      "service_name": "user-service"
    },
    "correlated_signals": {
      "logs": {
        "total": 3,
        "info": 2,
        "warning": 1,
        "error": 0
      },
      "metrics": {
        "total": 2,
        "gauge": 1,
        "counter": 1
      },
      "profiles": {
        "total": 0
      }
    }
  },
  "token_estimate": 1234,
  "processed_at": "2024-01-15T10:30:00Z"
}
```

## 🚀 **Usage Examples**

### **1. Basic Testing**

```go
// Initialize services
db, _ := sql.Open("postgres", "your_connection_string")
contentService := NewProcessedContentService(db)
mockDB := NewMockTraceDatabaseClient()
processor := NewTraceProcessor(mockDB, contentService)

// Test a specific span with a processing mode
ctx := context.Background()
err := processor.ProcessSpanAndStore(ctx, "trace-123", "span-1", "full_span_signals")
if err != nil {
    log.Printf("Error: %v", err)
}

// Retrieve and analyze the stored content
content, err := contentService.GetProcessedContent(ctx, "trace-123", "span-1")
if err != nil {
    log.Printf("Error: %v", err)
}

fmt.Printf("Mode: %s\n", content.Mode)
fmt.Printf("Content Length: %d characters\n", len(content.Content))
fmt.Printf("Token Estimate: %d tokens\n", estimateContentTokenCount(content.Content))
fmt.Printf("Metadata: %+v\n", content.Metadata)
```

### **2. Test All Processing Modes**

```go
modes := []string{
    "full_trace",
    "full_span", 
    "full_trace_signals",
    "full_span_signals",
}

for _, mode := range modes {
    fmt.Printf("Testing mode: %s\n", mode)
    
    // Process and store
    err := processor.ProcessSpanAndStore(ctx, traceID, spanID, mode)
    if err != nil {
        log.Printf("Error processing mode %s: %v", mode, err)
        continue
    }

    // Retrieve and display
    content, err := contentService.GetProcessedContent(ctx, traceID, spanID)
    if err != nil {
        log.Printf("Error retrieving content: %v", err)
        continue
    }

    fmt.Printf("Content: %s\n", truncateString(content.Content, 200))
    fmt.Printf("Tokens: %d\n", estimateContentTokenCount(content.Content))
    fmt.Println("---")
}
```

### **3. Analyze Collected Data**

```go
// Get all processed content for a trace
contents, err := contentService.GetAllProcessedContentForTrace(ctx, traceID)
if err != nil {
    log.Printf("Error: %v", err)
    return
}

// Analyze the data
totalTokens := 0
modeCounts := make(map[string]int)

for _, content := range contents {
    modeCounts[content.Mode]++
    tokens := estimateContentTokenCount(content.Content)
    totalTokens += tokens
    
    fmt.Printf("Mode %s: %d tokens\n", content.Mode, tokens)
}

fmt.Printf("Total tokens: %d\n", totalTokens)
fmt.Printf("Mode distribution: %+v\n", modeCounts)
```

### **4. Cleanup Test Data**

```go
// Delete processed content for cleanup
err := contentService.DeleteProcessedContent(ctx, traceID, spanID)
if err != nil {
    log.Printf("Error: %v", err)
    return
}

fmt.Printf("Deleted processed content for trace %s, span %s\n", traceID, spanID)
```

## 📈 **What to Analyze**

### **Content Analysis**
- **Content Length**: How much text is generated per mode
- **Token Estimates**: Rough token count for embedding planning
- **Content Quality**: Readability and structure of generated content

### **Metadata Analysis**
- **Signal Correlation**: How many logs/metrics are correlated per span
- **Span Information**: Duration, status, service names
- **Processing Statistics**: Timing, error rates, etc.

### **Mode Comparison**
- **Token Efficiency**: Which modes generate the most/least tokens
- **Content Richness**: Which modes include the most detailed information
- **Processing Overhead**: Time and resource usage per mode

## 🔧 **Configuration Options**

### **Token Estimation**
Currently uses a simple character-based estimation:
```go
func estimateContentTokenCount(content string) int {
    // Rough estimate: 1 token ≈ 4 characters for English text
    return len(content) / 4
}
```

### **Metadata Collection**
The system collects:
- Span-level information (name, duration, status)
- Correlated signal counts (logs, metrics, profiles)
- Processing metadata (timing, mode, etc.)

## 🎯 **Next Steps**

1. **Run Tests**: Use the examples above to test different processing modes
2. **Analyze Data**: Compare content quality and token usage across modes
3. **Optimize**: Based on analysis, optimize content generation
4. **Implement Remaining Modes**: Add the two attributes-level modes
5. **Production Integration**: Integrate with real-time span processing

## 📝 **Notes**

- This is for **testing purposes only** - in production, span modes would embed immediately
- The `processed_content` column stores NULL by default for efficiency
- Token estimation is approximate - use proper tokenizer for production
- All processing modes (including span modes) store content for analysis

This testing framework will help you understand exactly what data is being collected and how different processing modes affect content generation and token usage. 