package repository

import (
	"context"

	"github.com/medxops/trazr-otel/pkg/util"
)

// ServiceResourceManagerFactory defines the interface for creating ServiceResourceManager instances.
// This allows for dependency injection and easier testing by abstracting the creation logic.
//
// The default implementation is DefaultSRMFactory which calls NewServiceResourceManager().
// Tests can provide mock factories to create mock ServiceResourceManagers.
type ServiceResourceManagerFactory interface {
	// Create creates a new ServiceResourceManager instance.
	// Each call creates a fresh instance - there is no singleton pattern.
	Create(ctx context.Context, logger util.Logger) (ServiceResourceManagerInterface, error)
}

// DefaultSRMFactory is the default implementation of ServiceResourceManagerFactory.
// It creates real ServiceResourceManager instances using NewServiceResourceManager().
type DefaultSRMFactory struct{}

// Ensure DefaultSRMFactory implements the interface
var _ ServiceResourceManagerFactory = (*DefaultSRMFactory)(nil)

// Create creates a new ServiceResourceManager instance.
func (f *DefaultSRMFactory) Create(ctx context.Context, logger util.Logger) (ServiceResourceManagerInterface, error) {
	return NewServiceResourceManager(ctx, logger)
}

// NewDefaultSRMFactory creates a new DefaultSRMFactory.
func NewDefaultSRMFactory() *DefaultSRMFactory {
	return &DefaultSRMFactory{}
}
