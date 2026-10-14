package repository

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// ResourceHealthStatus represents the health and status of a managed resource.
type ResourceHealthStatus struct {
	Name      string
	Type      string // e.g., "db", "embedding", "vectorstore"
	Healthy   bool
	LastError string
	LastCheck time.Time
}

// ResourceMetrics holds counters and gauges for a managed resource.
type ResourceMetrics struct {
	RequestCount   int64
	SuccessCount   int64
	FailureCount   int64
	TotalLatencyMs int64
	LastError      string
	LastUpdated    time.Time
}

// ServiceResourceObservability provides observability hooks for ServiceResourceManager.
type ServiceResourceObservability struct {
	tracer  trace.Tracer
	meter   metric.Meter
	metrics map[string]*ResourceMetrics // keyed by resource name
	mu      sync.RWMutex

	healthStatuses []ResourceHealthStatus // latest health check results
}

// NewServiceResourceObservability creates a new observability struct with OpenTelemetry integration.
func NewServiceResourceObservability() *ServiceResourceObservability {
	return &ServiceResourceObservability{
		tracer:  otel.Tracer("trazr-otel.serviceresource"),
		meter:   otel.Meter("trazr-otel.serviceresource"),
		metrics: make(map[string]*ResourceMetrics),
	}
}

// RecordOperation records an operation for a resource, updating metrics and emitting a span.
func (o *ServiceResourceObservability) RecordOperation(ctx context.Context, resourceName, resourceType, operation string, start time.Time, err error) {
	latency := time.Since(start).Milliseconds()
	status := err == nil

	// Update metrics
	o.mu.Lock()
	m, ok := o.metrics[resourceName]
	if !ok {
		m = &ResourceMetrics{}
		o.metrics[resourceName] = m
	}
	m.RequestCount++
	m.TotalLatencyMs += latency
	m.LastUpdated = time.Now()
	if status {
		m.SuccessCount++
	} else {
		m.FailureCount++
		m.LastError = err.Error()
	}
	o.mu.Unlock()

	// Emit OpenTelemetry span
	_, span := o.tracer.Start(ctx, operation,
		trace.WithAttributes(
			attribute.String("resource.name", resourceName),
			attribute.String("resource.type", resourceType),
			attribute.Bool("success", status),
			attribute.Int64("latency_ms", latency),
		),
	)
	if err != nil {
		span.RecordError(err)
	}
	span.End()
}

// GetResourceMetrics returns a snapshot of metrics for all resources.
func (o *ServiceResourceObservability) GetResourceMetrics() map[string]ResourceMetrics {
	o.mu.RLock()
	defer o.mu.RUnlock()
	result := make(map[string]ResourceMetrics, len(o.metrics))
	for k, v := range o.metrics {
		result[k] = *v
	}
	return result
}

// SetResourceHealth updates the latest health check results.
func (o *ServiceResourceObservability) SetResourceHealth(statuses []ResourceHealthStatus) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.healthStatuses = statuses
}

// GetResourceHealth returns the latest health check results.
func (o *ServiceResourceObservability) GetResourceHealth() []ResourceHealthStatus {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.healthStatuses
}
