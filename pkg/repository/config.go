package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	dbconfig "github.com/medxops/trazr-otel/pkg/repository/internal/database"
	"github.com/medxops/trazr-otel/pkg/util"
)

// Constants for configuration defaults and timeouts
const (
	// Database operation timeouts
	DatabaseTimeout = 5 * time.Second

	DefaultSchema       = "otel"
	DefaultConfigSchema = "admin"
)

// ConfigManagers holds all the configuration managers needed for service initialization
type ConfigManagers struct {
	Config      dbconfig.SchemaConfigInterface
	VectorStore dbconfig.VectorStoreConfigInterface
	Embedding   dbconfig.EmbeddingConfigInterface
	Template    dbconfig.EmbeddingConfigTemplateInterface
	Processor   dbconfig.ProcessorConfigInterface
}

// initializeConfigManagers creates and initializes all configuration managers
func LoadConfig(ctx context.Context, db *sql.DB, logger util.Logger) (*ConfigManagers, error) {
	// Ensure config schema exists to store config data
	if err := dbconfig.EnsureSchemaExists(ctx, db, DefaultConfigSchema); err != nil {
		return nil, fmt.Errorf("failed to create schema: %w", err)
	}

	// Create configuration tables and defaults first
	config, err := dbconfig.NewDatabaseSchemaConfig(ctx, db, logger, DefaultSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema config manager: %w", err)
	}

	// Create vector store config manager
	vsConfigMgr, err := dbconfig.NewVectorStoreConfigManager(ctx, db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create vector store config manager: %w", err)
	}

	// Create embedding config manager
	embConfigMgr, err := dbconfig.NewEmbeddingConfigRepository(ctx, db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding config manager: %w", err)
	}

	// Create template manager
	templateRepo, err := dbconfig.NewTemplateConfigRepo(ctx, db, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create template config manager: %w", err)
	}

	// Create processor config manager
	processorMgr, err := dbconfig.NewProcessorConfigRepo(ctx, db, logger, DefaultSchema)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository config manager: %w", err)
	}

	return &ConfigManagers{
		Config:      config,
		VectorStore: vsConfigMgr,
		Embedding:   embConfigMgr,
		Template:    templateRepo,
		Processor:   processorMgr,
	}, nil
}
