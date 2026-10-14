package util

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// QueryPlanAnalysis provides detailed analysis of query execution plans
type QueryPlanAnalysis struct {
	QueryType     string                 `json:"query_type"`
	TableName     string                 `json:"table_name"`
	Query         string                 `json:"query"`
	ExecutionTime time.Duration          `json:"execution_time"`
	PlanningTime  time.Duration          `json:"planning_time"`
	TotalCost     float64                `json:"total_cost"`
	RowsReturned  int                    `json:"rows_returned"`
	IndexUsage    []string               `json:"index_usage"`
	ScanType      string                 `json:"scan_type"`
	Timestamp     time.Time              `json:"timestamp"`
	RawPlan       map[string]interface{} `json:"raw_plan"`
}

// QueryPlanMonitor tracks query plan performance over time
type QueryPlanMonitor struct {
	analyses   []QueryPlanAnalysis
	maxHistory int
	mutex      sync.RWMutex
}

// NewQueryPlanMonitor creates a new query plan monitor
func NewQueryPlanMonitor(maxHistory int) *QueryPlanMonitor {
	return &QueryPlanMonitor{
		analyses:   make([]QueryPlanAnalysis, 0),
		maxHistory: maxHistory,
	}
}

// RecordAnalysis records a query plan analysis
func (m *QueryPlanMonitor) RecordAnalysis(analysis QueryPlanAnalysis) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.analyses = append(m.analyses, analysis)

	// Keep only the most recent analyses
	if len(m.analyses) > m.maxHistory {
		m.analyses = m.analyses[1:]
	}
}

// GetAnalyses returns all analyses for a given query type and table
func (m *QueryPlanMonitor) GetAnalyses(queryType, tableName string) []QueryPlanAnalysis {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	var result []QueryPlanAnalysis
	for _, analysis := range m.analyses {
		if analysis.QueryType == queryType && analysis.TableName == tableName {
			result = append(result, analysis)
		}
	}
	return result
}

// AnalyzeQueryPlan performs comprehensive analysis of a query plan
func AnalyzeQueryPlan(ctx context.Context, db *sql.DB, queryType, tableName, query string, args ...interface{}) (*QueryPlanAnalysis, error) {
	// Create the EXPLAIN query
	explainQuery := fmt.Sprintf("EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) %s", query)

	// Execute the EXPLAIN query
	rows, err := db.QueryContext(ctx, explainQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute EXPLAIN query: %w", err)
	}
	defer rows.Close()

	var planJSON string
	if rows.Next() {
		if err := rows.Scan(&planJSON); err != nil {
			return nil, fmt.Errorf("failed to scan EXPLAIN result: %w", err)
		}
	}

	// Parse the JSON plan
	var planData []map[string]interface{}
	if err := json.Unmarshal([]byte(planJSON), &planData); err != nil {
		return nil, fmt.Errorf("failed to parse query plan JSON: %w", err)
	}

	if len(planData) == 0 {
		return nil, fmt.Errorf("no plan data returned")
	}

	plan := planData[0]

	// Extract key metrics from the plan
	analysis := &QueryPlanAnalysis{
		QueryType: queryType,
		TableName: tableName,
		Query:     query,
		Timestamp: time.Now(),
		RawPlan:   plan,
	}

	// Extract execution time
	if executionTime, ok := plan["Execution Time"].(float64); ok {
		analysis.ExecutionTime = time.Duration(executionTime * float64(time.Millisecond))
	}

	// Extract planning time
	if planningTime, ok := plan["Planning Time"].(float64); ok {
		analysis.PlanningTime = time.Duration(planningTime * float64(time.Millisecond))
	}

	// Extract total cost
	if totalCost, ok := plan["Total Cost"].(float64); ok {
		analysis.TotalCost = totalCost
	}

	// Extract rows returned
	if actualRows, ok := plan["Actual Rows"].(float64); ok {
		analysis.RowsReturned = int(actualRows)
	}

	// Extract scan type and index usage
	if nodeType, ok := plan["Node Type"].(string); ok {
		analysis.ScanType = nodeType
	}

	// Extract index usage from the plan
	analysis.IndexUsage = extractIndexUsage(plan)

	return analysis, nil
}

// extractIndexUsage recursively extracts index usage information from the query plan
func extractIndexUsage(plan map[string]interface{}) []string {
	var indexes []string

	// Check for index name in current node
	if indexName, ok := plan["Index Name"].(string); ok && indexName != "" {
		indexes = append(indexes, indexName)
	}

	// Check for relation name (table being scanned)
	if relationName, ok := plan["Relation Name"].(string); ok && relationName != "" {
		// This indicates a table scan, which might be using an index
		if nodeType, ok := plan["Node Type"].(string); ok && strings.Contains(nodeType, "Index") {
			indexes = append(indexes, fmt.Sprintf("%s_scan", relationName))
		}
	}

	// Recursively check child nodes
	if plans, ok := plan["Plans"].([]interface{}); ok {
		for _, childPlan := range plans {
			if childMap, ok := childPlan.(map[string]interface{}); ok {
				childIndexes := extractIndexUsage(childMap)
				indexes = append(indexes, childIndexes...)
			}
		}
	}

	return indexes
}

// ValidateQueryPlans ensures that TimescaleDB is using indexes effectively for exemplar queries
func ValidateQueryPlans(ctx context.Context, db *sql.DB, monitor *QueryPlanMonitor) error {
	// Example validation queries - these would be customized based on your specific needs
	validationQueries := []struct {
		queryType string
		tableName string
		query     string
	}{
		{
			queryType: "exemplar_span_lookup",
			tableName: "gauge_metrics",
			query:     "SELECT * FROM gauge_metrics WHERE exemplars @> $1::jsonb LIMIT 1",
		},
		{
			queryType: "log_span_lookup",
			tableName: "logs",
			query:     "SELECT * FROM logs WHERE span_id = $1 LIMIT 1",
		},
	}

	for _, vq := range validationQueries {
		analysis, err := AnalyzeQueryPlan(ctx, db, vq.queryType, vq.tableName, vq.query, `[{"span_id": "test"}]`)
		if err != nil {
			return fmt.Errorf("failed to analyze query plan for %s: %w", vq.queryType, err)
		}

		// Record the analysis
		monitor.RecordAnalysis(*analysis)

		// Validate that the query is using indexes effectively
		if len(analysis.IndexUsage) == 0 {
			return fmt.Errorf("query %s is not using any indexes", vq.queryType)
		}

		// Check for expensive operations
		if analysis.TotalCost > 1000 {
			return fmt.Errorf("query %s has high cost (%f), consider optimizing", vq.queryType, analysis.TotalCost)
		}
	}

	return nil
}

// MonitorQueryPerformance tracks query performance over time and alerts on regressions
func MonitorQueryPerformance(ctx context.Context, db *sql.DB, monitor *QueryPlanMonitor) error {
	// Get recent analyses for comparison
	recentAnalyses := monitor.GetAnalyses("", "") // Get all analyses

	if len(recentAnalyses) < 2 {
		return nil // Need at least 2 analyses to detect regressions
	}

	// Group analyses by query type and table
	queryPerformance := make(map[string][]QueryPlanAnalysis)
	for _, analysis := range recentAnalyses {
		key := fmt.Sprintf("%s_%s", analysis.QueryType, analysis.TableName)
		queryPerformance[key] = append(queryPerformance[key], analysis)
	}

	// Check for performance regressions
	for queryKey, analyses := range queryPerformance {
		if len(analyses) < 2 {
			continue
		}

		// Sort by timestamp
		latest := analyses[len(analyses)-1]
		previous := analyses[len(analyses)-2]

		// Check for significant performance degradation
		executionTimeIncrease := latest.ExecutionTime - previous.ExecutionTime
		if executionTimeIncrease > 100*time.Millisecond {
			return fmt.Errorf("performance regression detected for %s: execution time increased by %v",
				queryKey, executionTimeIncrease)
		}

		// Check for cost increase
		costIncrease := latest.TotalCost - previous.TotalCost
		if costIncrease > 100 {
			return fmt.Errorf("cost regression detected for %s: cost increased by %f",
				queryKey, costIncrease)
		}
	}

	return nil
}
