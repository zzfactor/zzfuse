package mocks

import (
	"context"
	"database/sql"

	"github.com/medxops/trazr-otel/pkg/event"
	"github.com/medxops/trazr-otel/pkg/repository"
	"github.com/medxops/trazr-otel/pkg/repository/template/content"
)

// MockServiceResourceManager is a mock implementation of ServiceResourceManagerInterface for testing.
type MockServiceResourceManager struct {
	ShutdownFunc          func() error
	IsRunningFunc         func() bool
	GetDatabaseFunc       func() (*sql.DB, error)
	GetEventBusFunc       func() (event.EventBusInterface, error)
	GetVectorStoreFunc    func(serviceName string) (repository.VectorStoreClient, error)
	GetRoutingSchemaFunc  func() (repository.DatabaseRoutingSchemaInterface, error)
	GetSchemaFunc         func(serviceName, serviceNamespace string) (string, bool, error)
	GetTemplateEngineFunc func(templateID int64) (*content.ContentTemplateEngine, error)
	GetLogWriterFunc      func() (repository.LogWriterService, error)
	GetMetricWriterFunc   func() (repository.MetricWriterService, error)
	GetTraceWriterFunc    func() (repository.TraceWriterService, error)
	GetProfileWriterFunc  func() (repository.ProfileWriterService, error)
	GetLogQueryFunc       func() (repository.LogQueryService, error)
	GetMetricQueryFunc    func() (repository.MetricQueryService, error)
	GetTraceQueryFunc     func() (repository.TraceQueryService, error)
	GetProfileQueryFunc   func() (repository.ProfileQueryService, error)
	GetRunWriterFunc      func() (repository.RunWriterService, error)
	GetRunQueryFunc       func() (repository.RunQueryService, error)
	PoolsFunc             func() map[string]*repository.ServiceResources
}

// Ensure MockServiceResourceManager implements the interface
var _ repository.ServiceResourceManagerInterface = (*MockServiceResourceManager)(nil)

func (m *MockServiceResourceManager) Shutdown() error {
	if m.ShutdownFunc != nil {
		return m.ShutdownFunc()
	}
	return nil
}

func (m *MockServiceResourceManager) IsRunning() bool {
	if m.IsRunningFunc != nil {
		return m.IsRunningFunc()
	}
	return true
}

func (m *MockServiceResourceManager) GetDatabase() (*sql.DB, error) {
	if m.GetDatabaseFunc != nil {
		return m.GetDatabaseFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetEventBus() (event.EventBusInterface, error) {
	if m.GetEventBusFunc != nil {
		return m.GetEventBusFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetVectorStore(serviceName string) (repository.VectorStoreClient, error) {
	if m.GetVectorStoreFunc != nil {
		return m.GetVectorStoreFunc(serviceName)
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetRoutingSchema() (repository.DatabaseRoutingSchemaInterface, error) {
	if m.GetRoutingSchemaFunc != nil {
		return m.GetRoutingSchemaFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetSchema(serviceName, serviceNamespace string) (string, bool, error) {
	if m.GetSchemaFunc != nil {
		return m.GetSchemaFunc(serviceName, serviceNamespace)
	}
	return "default", true, nil
}

func (m *MockServiceResourceManager) GetTemplateEngine(templateID int64) (*content.ContentTemplateEngine, error) {
	if m.GetTemplateEngineFunc != nil {
		return m.GetTemplateEngineFunc(templateID)
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetLogWriter() (repository.LogWriterService, error) {
	if m.GetLogWriterFunc != nil {
		return m.GetLogWriterFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetMetricWriter() (repository.MetricWriterService, error) {
	if m.GetMetricWriterFunc != nil {
		return m.GetMetricWriterFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetTraceWriter() (repository.TraceWriterService, error) {
	if m.GetTraceWriterFunc != nil {
		return m.GetTraceWriterFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetProfileWriter() (repository.ProfileWriterService, error) {
	if m.GetProfileWriterFunc != nil {
		return m.GetProfileWriterFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetLogQuery() (repository.LogQueryService, error) {
	if m.GetLogQueryFunc != nil {
		return m.GetLogQueryFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetMetricQuery() (repository.MetricQueryService, error) {
	if m.GetMetricQueryFunc != nil {
		return m.GetMetricQueryFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetTraceQuery() (repository.TraceQueryService, error) {
	if m.GetTraceQueryFunc != nil {
		return m.GetTraceQueryFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetProfileQuery() (repository.ProfileQueryService, error) {
	if m.GetProfileQueryFunc != nil {
		return m.GetProfileQueryFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetRunWriter() (repository.RunWriterService, error) {
	if m.GetRunWriterFunc != nil {
		return m.GetRunWriterFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) GetRunQuery() (repository.RunQueryService, error) {
	if m.GetRunQueryFunc != nil {
		return m.GetRunQueryFunc()
	}
	return nil, nil
}

func (m *MockServiceResourceManager) Pools() map[string]*repository.ServiceResources {
	if m.PoolsFunc != nil {
		return m.PoolsFunc()
	}
	return make(map[string]*repository.ServiceResources)
}

// MockTrazrServiceProvider is a mock implementation of TrazrServiceProvider for testing.
type MockTrazrServiceProvider struct {
	GetServiceResourceManagerFunc func() (repository.ServiceResourceManagerInterface, error)
	WaitForReadyFunc              func(ctx context.Context) error
	IsReadyFunc                   func() bool
}

// Ensure MockTrazrServiceProvider implements the interface
var _ repository.TrazrServiceProvider = (*MockTrazrServiceProvider)(nil)

func (m *MockTrazrServiceProvider) GetServiceResourceManager() (repository.ServiceResourceManagerInterface, error) {
	if m.GetServiceResourceManagerFunc != nil {
		return m.GetServiceResourceManagerFunc()
	}
	return &MockServiceResourceManager{}, nil
}

func (m *MockTrazrServiceProvider) WaitForReady(ctx context.Context) error {
	if m.WaitForReadyFunc != nil {
		return m.WaitForReadyFunc(ctx)
	}
	return nil
}

func (m *MockTrazrServiceProvider) IsReady() bool {
	if m.IsReadyFunc != nil {
		return m.IsReadyFunc()
	}
	return true
}

// MockEventBusProvider is a mock implementation of EventBusProvider for testing.
type MockEventBusProvider struct {
	GetEventBusFunc func() (event.EventBusInterface, error)
}

// Ensure MockEventBusProvider implements the interface
var _ repository.EventBusProvider = (*MockEventBusProvider)(nil)

func (m *MockEventBusProvider) GetEventBus() (event.EventBusInterface, error) {
	if m.GetEventBusFunc != nil {
		return m.GetEventBusFunc()
	}
	return nil, nil
}
