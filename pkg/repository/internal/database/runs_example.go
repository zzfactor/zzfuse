package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"
)

// ExampleRunMetadataUsage demonstrates how to use the structured metadata functionality
func ExampleRunMetadataUsage() {
	// This is a demonstration of how to use the structured metadata
	// In a real application, you would have a database connection
	fmt.Println("Example: Using Structured Run Metadata")
	fmt.Println("=====================================")

	// 1. Create structured metadata
	metadata := &RunMetadata{
		SpanCount:       1000,
		SpanEventCount:  500,
		LogCount:        2000,
		LogInfoCount:    1500,
		LogWarningCount: 300,
		LogDebugCount:   150,
		LogErrorCount:   50,
		SpanErrorCount:  25,
	}

	// 2. Create a run with structured metadata
	scopeName := "my-scope"
	run := NewRunWithStructuredMetadata(
		"my-service",
		&scopeName,
		"processing-run",
		time.Now().UnixNano(),
		nil, // endTimestampNs
		nil, // traceID
		nil, // processorID
		nil, // runType
		metadata,
	)

	// 3. Access structured metadata
	structuredMetadata := run.GetStructuredMetadata()
	fmt.Printf("Initial Span Count: %d\n", structuredMetadata.SpanCount)
	fmt.Printf("Initial Log Error Count: %d\n", structuredMetadata.LogErrorCount)

	// 4. Update metadata
	structuredMetadata.SpanCount += 50
	structuredMetadata.LogErrorCount += 5
	run.SetStructuredMetadata(structuredMetadata)

	// 5. Verify updates
	updatedMetadata := run.GetStructuredMetadata()
	fmt.Printf("Updated Span Count: %d\n", updatedMetadata.SpanCount)
	fmt.Printf("Updated Log Error Count: %d\n", updatedMetadata.LogErrorCount)

	// 6. Convert to map for database storage
	dbMetadata := structuredMetadata.ToMap()
	fmt.Printf("Database metadata keys: %v\n", getKeys(dbMetadata))
}

// ExampleRepositoryUsage demonstrates how to use the repository with structured metadata
func ExampleRepositoryUsage(ctx context.Context, db *sql.DB, schema string) {
	// Create repository
	repo, err := NewRunRepository(ctx, db, schema)
	if err != nil {
		panic(fmt.Sprintf("Failed to create repository: %v", err))
	}

	// 1. Create a run with structured metadata
	metadata := &RunMetadata{
		SpanCount:     100,
		LogCount:      200,
		LogErrorCount: 5,
	}

	scopeName := "example-scope"
	runID, err := repo.CreateWithStructuredMetadata(
		ctx,
		"example-service",
		&scopeName,
		"example-run",
		time.Now().UnixNano(),
		nil, // endTimestampNs
		nil, // traceID
		nil, // processorID
		nil, // runType
		metadata,
	)
	if err != nil {
		log.Printf("Failed to create run: %v", err)
		return
	}

	fmt.Printf("Created run with ID: %d\n", runID)

	// 2. Retrieve and display structured metadata
	retrievedMetadata, err := repo.GetStructuredMetadata(ctx, runID)
	if err != nil {
		log.Printf("Failed to get structured metadata: %v", err)
		return
	}

	fmt.Printf("Retrieved metadata - Spans: %d, Logs: %d, Errors: %d\n",
		retrievedMetadata.SpanCount,
		retrievedMetadata.LogCount,
		retrievedMetadata.LogErrorCount,
	)

	// 3. Update specific metadata fields
	err = repo.UpdateMetadataField(ctx, runID, "SPAN_COUNT", 150)
	if err != nil {
		log.Printf("Failed to update SPAN_COUNT: %v", err)
		return
	}

	// 4. Increment metadata fields
	err = repo.IncrementMetadataField(ctx, runID, "LOG_ERROR_COUNT", 3)
	if err != nil {
		log.Printf("Failed to increment LOG_ERROR_COUNT: %v", err)
		return
	}

	// 5. Retrieve updated metadata
	updatedMetadata, err := repo.GetStructuredMetadata(ctx, runID)
	if err != nil {
		log.Printf("Failed to get updated metadata: %v", err)
		return
	}

	fmt.Printf("Updated metadata - Spans: %d, Logs: %d, Errors: %d\n",
		updatedMetadata.SpanCount,
		updatedMetadata.LogCount,
		updatedMetadata.LogErrorCount,
	)

	// 6. Update entire structured metadata
	newMetadata := &RunMetadata{
		SpanCount:       200,
		SpanEventCount:  100,
		LogCount:        300,
		LogInfoCount:    250,
		LogWarningCount: 30,
		LogDebugCount:   15,
		LogErrorCount:   5,
		SpanErrorCount:  10,
	}

	err = repo.UpdateStructuredMetadata(ctx, runID, newMetadata)
	if err != nil {
		log.Printf("Failed to update structured metadata: %v", err)
		return
	}

	fmt.Println("Successfully updated entire structured metadata")
}

// Helper function to get keys from a map
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ExampleIncrementalUpdates demonstrates how to incrementally update metadata
func ExampleIncrementalUpdates(ctx context.Context, repo *RunRepository, runID int64) {
	fmt.Println("Example: Incremental Metadata Updates")
	fmt.Println("====================================")

	// Increment various counters
	counters := map[string]int64{
		"SPAN_COUNT":        10,
		"SPAN_EVENT_COUNT":  5,
		"LOG_COUNT":         20,
		"LOG_INFO_COUNT":    15,
		"LOG_WARNING_COUNT": 3,
		"LOG_DEBUG_COUNT":   2,
		"LOG_ERROR_COUNT":   1,
		"SPAN_ERROR_COUNT":  1,
	}

	for field, increment := range counters {
		err := repo.IncrementMetadataField(ctx, runID, field, increment)
		if err != nil {
			log.Printf("Failed to increment %s: %v", field, err)
			continue
		}
		fmt.Printf("Incremented %s by %d\n", field, increment)
	}

	// Retrieve final metadata
	finalMetadata, err := repo.GetStructuredMetadata(ctx, runID)
	if err != nil {
		log.Printf("Failed to get final metadata: %v", err)
		return
	}

	fmt.Printf("Final metadata:\n")
	fmt.Printf("  Spans: %d (events: %d, errors: %d)\n",
		finalMetadata.SpanCount, finalMetadata.SpanEventCount, finalMetadata.SpanErrorCount)
	fmt.Printf("  Logs: %d (info: %d, warning: %d, debug: %d, error: %d)\n",
		finalMetadata.LogCount, finalMetadata.LogInfoCount, finalMetadata.LogWarningCount,
		finalMetadata.LogDebugCount, finalMetadata.LogErrorCount)
}
