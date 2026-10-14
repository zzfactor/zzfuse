# Processing Mode Configuration Examples

## Overview

Processing mode can now be configured at the schema level using the `metadata` field in the `config_otel_database` table. This provides consistency across all vector stores in a schema.

## Configuration Examples

### 1. Set Processing Mode in Database Metadata

```sql
-- Update existing database schema to use batch processing mode
UPDATE config_otel_database 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb), 
    '{processing_mode}', 
    '"batch"'
)
WHERE name = 'your-schema-name';

-- Update to use trigger processing mode
UPDATE config_otel_database 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb), 
    '{processing_mode}', 
    '"trigger"'
)
WHERE name = 'your-schema-name';

-- Update to use hybrid processing mode
UPDATE config_otel_database 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb), 
    '{processing_mode}', 
    '"hybrid"'
)
WHERE name = 'your-schema-name';
```

### 2. Check Current Processing Mode

```sql
-- View current processing mode for all schemas
SELECT 
    name, 
    metadata->>'processing_mode' as processing_mode,
    CASE 
        WHEN metadata->>'processing_mode' IS NULL THEN 'hybrid (default)'
        ELSE metadata->>'processing_mode'
    END as effective_mode
FROM config_otel_database 
WHERE enabled = true;
```

### 3. Complete Metadata Configuration Example

```sql
-- Set comprehensive metadata including processing mode
UPDATE config_otel_database 
SET metadata = '{
    "processing_mode": "batch",
    "batch_size": 100,
    "batch_interval": "30s",
    "description": "Production schema with batch processing"
}'::jsonb
WHERE name = 'production-schema';
```

## Processing Mode Options

| Mode | Description | Use Case |
|------|-------------|----------|
| `batch` | Process signals in batches | High throughput, bulk processing |
| `trigger` | Process signals immediately | Low latency, real-time processing |
| `hybrid` | Combine batch and trigger | Balanced approach (default) |

## Backward Compatibility

- **Existing configurations**: Continue to work with vector store-level processing mode
- **New configurations**: Can use schema-level processing mode via metadata
- **Fallback logic**: If no schema-level mode is set, falls back to vector store mode
- **Default**: "hybrid" mode if no processing mode is specified

## Migration Path

### Step 1: Check Current Configuration
```sql
SELECT 
    name,
    metadata->>'processing_mode' as schema_mode,
    'vector_store_mode' as vector_store_mode
FROM config_otel_database 
WHERE enabled = true;
```

### Step 2: Set Schema-Level Processing Mode
```sql
-- Example: Set all production schemas to batch mode
UPDATE config_otel_database 
SET metadata = jsonb_set(
    COALESCE(metadata, '{}'::jsonb), 
    '{processing_mode}', 
    '"batch"'
)
WHERE name LIKE '%prod%' AND enabled = true;
```

### Step 3: Verify Configuration
```sql
-- Verify the changes
SELECT 
    name, 
    metadata->>'processing_mode' as processing_mode
FROM config_otel_database 
WHERE enabled = true;
```

## Benefits

1. **Consistency**: All vector stores in a schema use the same processing mode
2. **Simplified Management**: One configuration per schema instead of per vector store
3. **Flexibility**: Can still override at vector store level if needed
4. **Backward Compatible**: Existing configurations continue to work
5. **No Schema Changes**: Uses existing metadata JSONB column 