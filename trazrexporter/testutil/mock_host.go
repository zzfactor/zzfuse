package testutil

import (
	"context"

	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/pkg/repository/mocks"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// MockHost is a mock implementation of component.Host for testing exporters.
// It provides a TrazrServiceProvider via GetExtensions().
type MockHost struct {
	extensions map[component.ID]component.Component
}

// Ensure MockHost implements component.Host
var _ component.Host = (*MockHost)(nil)

// NewMockHost creates a new MockHost with a TrazrServiceProvider extension.
func NewMockHost(provider repository.TrazrServiceProvider) *MockHost {
	h := &MockHost{
		extensions: make(map[component.ID]component.Component),
	}
	if provider != nil {
		// Add the provider as an extension
		h.extensions[component.MustNewID("trazr")] = &mockExtensionWrapper{provider: provider}
	}
	return h
}

// NewMockHostWithSRM creates a new MockHost with a MockTrazrServiceProvider
// that returns the given ServiceResourceManager.
func NewMockHostWithSRM(srm repository.ServiceResourceManagerInterface) *MockHost {
	provider := &mocks.MockTrazrServiceProvider{
		GetServiceResourceManagerFunc: func() (repository.ServiceResourceManagerInterface, error) {
			return srm, nil
		},
		WaitForReadyFunc: func(ctx context.Context) error {
			return nil
		},
		IsReadyFunc: func() bool {
			return true
		},
	}
	return NewMockHost(provider)
}

// GetExtensions returns the extensions map.
func (h *MockHost) GetExtensions() map[component.ID]component.Component {
	return h.extensions
}

// mockExtensionWrapper wraps a TrazrServiceProvider to satisfy the component.Component interface.
type mockExtensionWrapper struct {
	provider repository.TrazrServiceProvider
}

// Ensure mockExtensionWrapper implements extension.Extension
var _ extension.Extension = (*mockExtensionWrapper)(nil)

func (w *mockExtensionWrapper) Start(ctx context.Context, host component.Host) error {
	return nil
}

func (w *mockExtensionWrapper) Shutdown(ctx context.Context) error {
	return nil
}

// GetServiceResourceManager delegates to the wrapped provider.
func (w *mockExtensionWrapper) GetServiceResourceManager() (repository.ServiceResourceManagerInterface, error) {
	return w.provider.GetServiceResourceManager()
}

// WaitForReady delegates to the wrapped provider.
func (w *mockExtensionWrapper) WaitForReady(ctx context.Context) error {
	return w.provider.WaitForReady(ctx)
}

// IsReady delegates to the wrapped provider.
func (w *mockExtensionWrapper) IsReady() bool {
	return w.provider.IsReady()
}
