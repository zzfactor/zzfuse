# Template Plus Mode Guide

## Overview

The `template_plus` processing mode extends the existing `template` mode by adding trace-level embedding functionality. When enabled, it performs all the normal template processing for individual spans, but also creates composite vectors representing entire traces when root spans are processed.

## How It Works

### Template Mode (Existing)
- Processes individual spans using templates
- Creates embeddings for each span separately
- Stores span vectors in the vector store

### Template Plus Mode (New)
- Does everything template mode does
- **Additionally**: When a root span is processed, it:
  1. Identifies if it's a root span (no `parent_span_id`)
  2. Queries the database for all child spans in the same trace
  3. Retrieves vectors for all spans in the trace
  4. Combines them using a configurable strategy (mean, max, concatenation, weighted_mean)
  5. Creates a composite vector with `span_id = "trace"`
  6. Updates the root span's `trace_embedded_id` field

## Configuration

### Database Configuration

Set the `processing_mode` field in the `config_otel_vector` table to `"template_plus"`:

```sql
UPDATE config_otel_vector 
SET processing_mode = 'template_plus' 
WHERE id = your_config_id;
```

### Vector Combination Strategy

Configure the vector combination strategy in the vector store metadata:

```sql
UPDATE config_otel_vector 
SET metadata = jsonb_set(
    metadata, 
    '{embedding_vector_combination}', 
    '"mean"'
) 
WHERE id = your_config_id;
```

Available strategies:
- `"mean"` - Average all vectors (default)
- `"max"` - Take maximum value for each dimension
- `"concatenation"` - Append all vectors end-to-end
- `"weighted_mean"` - Weighted average (currently equal weights)

## Usage Example

### 1. Enable Template Plus Mode

```sql
-- Update your vector store mapping to use template_plus mode
UPDATE config_otel_vector 
SET processing_mode = 'template_plus',
    metadata = jsonb_set(
        metadata, 
        '{embedding_vector_combination}', 
        '"mean"'
    )
WHERE name = 'your-vector-store-mapping';
```

### 2. Process Spans

When spans are processed with `template_plus` mode:

1. **Individual spans** are processed normally with templates
2. **Root spans** trigger additional trace-level processing:
   - System queries for child spans: `SELECT * FROM spans WHERE trace_id = ? AND parent_span_id = ?`
   - Retrieves vectors for all spans in the trace
   - Combines vectors using the configured strategy
   - Creates composite vector with `span_id = "trace"`
   - Updates root span's `trace_embedded_id`

### 3. Query Results

You can now query for:
- **Individual span vectors**: Normal similarity search
- **Trace-level vectors**: Search for `span_id = "trace"` or use the `trace_embedded_id` from root spans

## Error Handling

- Trace embedding failures are logged as warnings but don't stop batch processing
- Individual span processing continues normally even if trace embedding fails
- Vector quality issues (NaN, Inf, zero vectors) are automatically handled and normalized

## Performance Considerations

- Trace embedding adds minimal overhead to normal processing
- Vector combination is optimized with batch operations
- Database queries are efficient with proper indexing
- Memory usage is controlled with vector validation and normalization

## Monitoring

The system provides metrics for:
- Vector combination operations
- Trace embedding success/failure rates
- Processing time for trace-level operations
- Vector quality issues

## Compatibility

- Works with all existing vector stores (PGVector, Chroma, Pinecone, Qdrant)
- Backward compatible with existing `template` mode
- No breaking changes to existing functionality
- Minimal configuration changes required

## Troubleshooting

### Common Issues

1. **No trace embeddings created**: Check that spans have proper `trace_id` and `parent_span_id` relationships
2. **Vector combination errors**: Verify vector dimensions are consistent across spans
3. **Performance issues**: Ensure database has proper indexes on `trace_id` and `parent_span_id`

### Debug Logging

Enable debug logging to see trace embedding operations:

```go
// In your configuration
logger.SetLevel("debug")
```

This will show:
- Root span detection
- Child span queries
- Vector combination operations
- Success/failure messages 