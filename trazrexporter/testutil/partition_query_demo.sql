-- Demo: How partition pruning works

-- 1. Query that should hit only one partition (current month)
EXPLAIN (ANALYZE, BUFFERS) 
SELECT COUNT(*) 
FROM logs 
WHERE timestamp_ns >= EXTRACT(EPOCH FROM DATE_TRUNC('month', CURRENT_DATE))::bigint * 1000000000
  AND timestamp_ns < EXTRACT(EPOCH FROM DATE_TRUNC('month', CURRENT_DATE) + INTERVAL '1 month')::bigint * 1000000000;

-- 2. Query that should hit multiple partitions (last 3 months)
EXPLAIN (ANALYZE, BUFFERS)
SELECT COUNT(*)
FROM logs 
WHERE timestamp_ns >= EXTRACT(EPOCH FROM DATE_TRUNC('month', CURRENT_DATE) - INTERVAL '2 months')::bigint * 1000000000;

-- 3. Query without partition key (should scan all partitions)
EXPLAIN (ANALYZE, BUFFERS)
SELECT COUNT(*)
FROM logs 
WHERE body LIKE '%error%';

-- 4. Show partition constraints
SELECT 
    schemaname,
    tablename,
    pg_get_expr(c.conbin, c.conrelid) as constraint_definition
FROM pg_constraint c
JOIN pg_tables t ON c.conrelid = t.tablename::regclass
WHERE t.tablename LIKE 'logs_%'
  AND c.contype = 'c'  -- Check constraints
ORDER BY tablename;
