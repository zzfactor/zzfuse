package repository

import (
	"context"

	"github.com/medxops/trazr-otel/pkg/event"
)

// TrazrServiceProvider is implemented by TrazrExtension to provide access to
// shared services for exporters. This follows the OTel Collector idiom where
// extensions provide services to other components via Host.GetExtensions().
//
// Usage in exporters:
//
//	func (e *logsExporter) start(ctx context.Context, host component.Host) error {
//	    provider, err := GetTrazrServiceProvider(host)
//	    if err != nil {
//	        return fmt.Errorf("trazr extension not found: %w", err)
//	    }
//	    if err := provider.WaitForReady(ctx); err != nil {
//	        return fmt.Errorf("trazr services not ready: %w", err)
//	    }
//	    e.srm, err = provider.GetServiceResourceManager()
//	    return err
//	}
type TrazrServiceProvider interface {
	// GetServiceResourceManager returns the shared ServiceResourceManager instance.
	// This should only be called after WaitForReady returns successfully.
	GetServiceResourceManager() (ServiceResourceManagerInterface, error)

	// WaitForReady blocks until the TrazrExtension has fully initialized its services.
	// Returns an error if the context is cancelled or times out before services are ready.
	WaitForReady(ctx context.Context) error

	// IsReady returns true if the TrazrExtension has fully initialized its services.
	IsReady() bool
}

// EventBusProvider breaks the circular dependency between SchemaManager and
// ServiceResourceManager. Instead of SchemaManager holding a reference to
// ServiceResourceManager, it holds an EventBusProvider which can be implemented
// by ServiceResourceManager or any other component that provides event bus access.
type EventBusProvider interface {
	// GetEventBus returns the event bus interface for event-driven architecture.
	GetEventBus() (event.EventBusInterface, error)
}

// Ensure ServiceResourceManager implements EventBusProvider
var _ EventBusProvider = (*ServiceResourceManager)(nil)

// Ensure ServiceResourceManager implements WorkerManagerDependencies
var _ WorkerManagerDependencies = (*ServiceResourceManager)(nil)
