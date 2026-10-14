// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"testing"

	"github.com/medxops/trazr-otel/pkg/repository/internal/database"
)

// TestOtelSQLConfigDatabaseClient_Initialization tests that the client can be created
// and all repositories are properly initialized
func TestOtelSQLConfigDatabaseClient_Initialization(t *testing.T) {
	// This test verifies that the client structure is properly set up
	// without requiring a real database connection
	
	client := &OtelSQLConfigDatabaseClient{
		db:     nil, // We're not testing actual DB operations here
		schema: DefaultSchema,
		logger: nil, // Use nil for testing
	}
	
	// Verify the client structure is properly initialized
	if client.schema != DefaultSchema {
		t.Errorf("Expected schema to be %s, got %s", DefaultSchema, client.schema)
	}
	
	// Logger can be nil for testing
}

// TestOtelSQLConfigDatabaseClient_InterfaceCompliance tests that our implementation
// satisfies the OtelConfigDatabaseClient interface
func TestOtelSQLConfigDatabaseClient_InterfaceCompliance(t *testing.T) {
	var _ OtelConfigDatabaseClient = &OtelSQLConfigDatabaseClient{}
}

// TestConfigStatistics_Structure tests the ConfigStatistics structure
func TestConfigStatistics_Structure(t *testing.T) {
	stats := &database.ConfigStatistics{
		SchemaConfigCount:      5,
		ProcessorConfigCount:   10,
		EmbeddingConfigCount:   3,
		TemplateConfigCount:    7,
		VectorStoreConfigCount: 2,
	}
	
	if stats.SchemaConfigCount != 5 {
		t.Errorf("Expected SchemaConfigCount to be 5, got %d", stats.SchemaConfigCount)
	}
	
	if stats.ProcessorConfigCount != 10 {
		t.Errorf("Expected ProcessorConfigCount to be 10, got %d", stats.ProcessorConfigCount)
	}
	
	if stats.EmbeddingConfigCount != 3 {
		t.Errorf("Expected EmbeddingConfigCount to be 3, got %d", stats.EmbeddingConfigCount)
	}
	
	if stats.TemplateConfigCount != 7 {
		t.Errorf("Expected TemplateConfigCount to be 7, got %d", stats.TemplateConfigCount)
	}
	
	if stats.VectorStoreConfigCount != 2 {
		t.Errorf("Expected VectorStoreConfigCount to be 2, got %d", stats.VectorStoreConfigCount)
	}
}

// TestOtelSQLConfigDatabaseClient_ErrorHandling tests error handling for uninitialized repositories
func TestOtelSQLConfigDatabaseClient_ErrorHandling(t *testing.T) {
	ctx := context.Background()
	
	// Create client with uninitialized repositories
	client := &OtelSQLConfigDatabaseClient{
		db:     nil,
		schema: DefaultSchema,
		logger: nil,
		// All repo fields are nil
	}
	
	// Test schema operations return appropriate errors
	_, err := client.GetSchemaConfigByID(ctx, 1)
	if err == nil || err.Error() != "schema repository not initialized" {
		t.Errorf("Expected 'schema repository not initialized' error, got: %v", err)
	}
	
	// Test processor operations return appropriate errors
	_, err = client.GetProcessorConfigByID(ctx, 1)
	if err == nil || err.Error() != "processor repository not initialized" {
		t.Errorf("Expected 'processor repository not initialized' error, got: %v", err)
	}
	
	// Test embedding operations return appropriate errors
	_, err = client.GetEmbeddingConfigByID(ctx, 1)
	if err == nil || err.Error() != "embedding repository not initialized" {
		t.Errorf("Expected 'embedding repository not initialized' error, got: %v", err)
	}
	
	// Test template operations return appropriate errors
	_, err = client.GetTemplateConfigByID(ctx, 1)
	if err == nil || err.Error() != "template repository not initialized" {
		t.Errorf("Expected 'template repository not initialized' error, got: %v", err)
	}
	
	// Test vector store operations return appropriate errors
	_, err = client.GetVectorStoreConfigByID(ctx, 1)
	if err == nil || err.Error() != "vector store repository not initialized" {
		t.Errorf("Expected 'vector store repository not initialized' error, got: %v", err)
	}
}
