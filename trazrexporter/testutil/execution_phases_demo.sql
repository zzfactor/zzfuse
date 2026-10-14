-- Demonstration of PostgreSQL partition execution phases

-- Phase 1: Show partition pruning in action
SET enable_partition_pruning = on;  -- Default, but explicit
SET constraint_exclusion = partition;  -- Default for partitioned tables

-- This query should demonstrate partition pruning
EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
SELECT 
    COUNT(*) as log_count,
    MIN(timestamp) as earliest,
    MAX(timestamp) as latest
FROM logs 
WHERE timestamp_ns >= EXTRACT(EPOCH FROM CURRENT_DATE - INTERVAL '1 day')::bigint * 1000000000;

-- Phase 2: Show what happens when partition pruning is disabled
SET enable_partition_pruning = off;

EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)
SELECT COUNT(*) 
FROM logs 
WHERE timestamp_ns >= EXTRACT(EPOCH FROM CURRENT_DATE - INTERVAL '1 day')::bigint * 1000000000;

-- Reset
SET enable_partition_pruning = on;

-- Phase 3: Show partition-wise aggregation
SET enable_partitionwise_aggregate = on;

EXPLAIN (ANALYZE, BUFFERS)
SELECT 
    DATE_TRUNC('hour', timestamp) as hour,
    COUNT(*) as log_count
FROM logs 
WHERE timestamp_ns >= EXTRACT(EPOCH FROM CURRENT_DATE - INTERVAL '1 day')::bigint * 1000000000
GROUP BY DATE_TRUNC('hour', timestamp)
ORDER BY hour;
