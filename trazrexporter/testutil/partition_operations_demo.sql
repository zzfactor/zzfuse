-- Different types of partition-aware operations

-- 1. Partition Pruning with Index Usage
EXPLAIN (ANALYZE, BUFFERS)
SELECT trace_id, span_id, timestamp 
FROM spans 
WHERE timestamp_ns BETWEEN 
    EXTRACT(EPOCH FROM CURRENT_DATE)::bigint * 1000000000 
    AND EXTRACT(EPOCH FROM CURRENT_DATE + INTERVAL '1 hour')::bigint * 1000000000
  AND trace_id = 'some-trace-id'
ORDER BY timestamp_ns;

-- 2. Partition-wise JOIN (if we had related tables)
-- This would show how PostgreSQL can join partitions independently

-- 3. Partition-wise Aggregation
SET enable_partitionwise_aggregate = on;
EXPLAIN (ANALYZE, BUFFERS)
SELECT 
    DATE_TRUNC('hour', timestamp) as hour,
    service_name,
    COUNT(*) as span_count,
    AVG(duration) as avg_duration
FROM spans 
WHERE timestamp_ns >= EXTRACT(EPOCH FROM CURRENT_DATE - INTERVAL '1 day')::bigint * 1000000000
GROUP BY DATE_TRUNC('hour', timestamp), service_name
ORDER BY hour, service_name;

-- 4. Parallel Partition Processing
SET max_parallel_workers_per_gather = 4;
SET parallel_setup_cost = 1000;
SET parallel_tuple_cost = 0.1;

EXPLAIN (ANALYZE, BUFFERS)
SELECT COUNT(*), AVG(duration)
FROM spans 
WHERE timestamp_ns >= EXTRACT(EPOCH FROM CURRENT_DATE - INTERVAL '7 days')::bigint * 1000000000;
