package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

var (
	// EventBusEventsPublishedTotal counts total events published by event type and bus type.
	EventBusEventsPublishedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "event_bus_events_published_total",
			Help: "Total number of events published by event type and bus type.",
		},
		[]string{"event_type", "bus_type"},
	)

	// EventBusEventsDeliveredTotal counts total events delivered to handlers by event type and bus type.
	EventBusEventsDeliveredTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "event_bus_events_delivered_total",
			Help: "Total number of events delivered to handlers by event type and bus type.",
		},
		[]string{"event_type", "bus_type"},
	)

	// EventBusEventsFailedTotal counts total events that failed processing by event type and bus type.
	EventBusEventsFailedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "event_bus_events_failed_total",
			Help: "Total number of events that failed processing by event type and bus type.",
		},
		[]string{"event_type", "bus_type", "error_type"},
	)

	// EventBusProcessingLatency measures event processing latency by event type and bus type.
	EventBusProcessingLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "event_bus_processing_latency_seconds",
			Help:    "Event processing latency in seconds by event type and bus type.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"event_type", "bus_type"},
	)

	// EventBusQueueSize measures current queue size by bus type.
	EventBusQueueSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_queue_size",
			Help: "Current number of events in the processing queue by bus type.",
		},
		[]string{"bus_type"},
	)

	// EventBusActiveWorkers measures current number of active workers by bus type.
	EventBusActiveWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_active_workers",
			Help: "Current number of active workers by bus type.",
		},
		[]string{"bus_type"},
	)

	// EventBusTotalWorkers measures total number of workers by bus type.
	EventBusTotalWorkers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_total_workers",
			Help: "Total number of workers by bus type.",
		},
		[]string{"bus_type"},
	)

	// EventBusHandlersRegistered measures number of registered handlers by event type and bus type.
	EventBusHandlersRegistered = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_handlers_registered",
			Help: "Number of registered handlers by event type and bus type.",
		},
		[]string{"event_type", "bus_type"},
	)

	// EventBusBatchSize measures batch processing statistics by bus type.
	EventBusBatchSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "event_bus_batch_size",
			Help:    "Batch size distribution by bus type.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000},
		},
		[]string{"bus_type"},
	)

	// EventBusBatchLatency measures batch processing latency by bus type.
	EventBusBatchLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "event_bus_batch_latency_seconds",
			Help:    "Batch processing latency in seconds by bus type.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bus_type"},
	)

	// NATS specific metrics
	EventBusNATSConnectionStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_nats_connection_status",
			Help: "NATS connection status (1 = connected, 0 = disconnected).",
		},
		[]string{"url"},
	)

	EventBusNATSMessagesPublished = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "event_bus_nats_messages_published_total",
			Help: "Total number of messages published to NATS by topic.",
		},
		[]string{"topic"},
	)

	EventBusNATSMessagesReceived = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "event_bus_nats_messages_received_total",
			Help: "Total number of messages received from NATS by topic.",
		},
		[]string{"topic"},
	)

	EventBusNATSPublishLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "event_bus_nats_publish_latency_seconds",
			Help:    "NATS publish latency in seconds by topic.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"topic"},
	)

	// EventBusNATSMessageSize measures NATS message size by topic.
	EventBusNATSMessageSize = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "event_bus_nats_message_size_bytes",
			Help:    "NATS message size in bytes by topic.",
			Buckets: []float64{64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768},
		},
		[]string{"topic"},
	)

	// Circuit Breaker Metrics
	EventBusCircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_circuit_breaker_state",
			Help: "Circuit breaker state by event type and bus type (0=closed, 1=half-open, 2=open).",
		},
		[]string{"event_type", "bus_type"},
	)

	EventBusCircuitBreakerFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "event_bus_circuit_breaker_failures_total",
			Help: "Total failures by event type and bus type that triggered circuit breaker.",
		},
		[]string{"event_type", "bus_type"},
	)

	EventBusCircuitBreakerTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "event_bus_circuit_breaker_transitions_total",
			Help: "Total circuit breaker state transitions by event type and bus type.",
		},
		[]string{"event_type", "bus_type", "from_state", "to_state"},
	)

	EventBusCircuitBreakerSuccessRate = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_circuit_breaker_success_rate",
			Help: "Success rate by event type and bus type (0.0 to 1.0).",
		},
		[]string{"event_type", "bus_type"},
	)

	EventBusCircuitBreakerLastFailureTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "event_bus_circuit_breaker_last_failure_timestamp",
			Help: "Timestamp of last failure by event type and bus type.",
		},
		[]string{"event_type", "bus_type"},
	)
)

func registerEventBusMetrics() {
	prometheus.MustRegister(
		EventBusEventsPublishedTotal,
		EventBusEventsDeliveredTotal,
		EventBusEventsFailedTotal,
		EventBusProcessingLatency,
		EventBusQueueSize,
		EventBusActiveWorkers,
		EventBusTotalWorkers,
		EventBusHandlersRegistered,
		EventBusBatchSize,
		EventBusBatchLatency,
		EventBusNATSConnectionStatus,
		EventBusNATSMessagesPublished,
		EventBusNATSMessagesReceived,
		EventBusNATSPublishLatency,
		EventBusNATSMessageSize,
		EventBusCircuitBreakerState,
		EventBusCircuitBreakerFailures,
		EventBusCircuitBreakerTransitions,
		EventBusCircuitBreakerSuccessRate,
		EventBusCircuitBreakerLastFailureTime,
	)
}

// GetEventBusMetrics returns event bus metrics in a format suitable for API responses
func GetEventBusMetrics() map[string]interface{} {
	// Note: Prometheus metrics are typically accessed via HTTP endpoint
	// For programmatic access, we'd need to implement metric value extraction
	// This is a simplified version - in practice, you might want to use
	// prometheus.Registry to get metric values programmatically

	return map[string]interface{}{
		"events_published":                  "use_prometheus_endpoint",
		"events_delivered":                  "use_prometheus_endpoint",
		"events_failed":                     "use_prometheus_endpoint",
		"processing_latency":                "use_prometheus_endpoint",
		"queue_size":                        "use_prometheus_endpoint",
		"active_workers":                    "use_prometheus_endpoint",
		"total_workers":                     "use_prometheus_endpoint",
		"handlers_registered":               "use_prometheus_endpoint",
		"batch_size":                        "use_prometheus_endpoint",
		"batch_latency":                     "use_prometheus_endpoint",
		"nats_connection_status":            "use_prometheus_endpoint",
		"nats_messages_published":           "use_prometheus_endpoint",
		"nats_messages_received":            "use_prometheus_endpoint",
		"nats_publish_latency":              "use_prometheus_endpoint",
		"nats_message_size":                 "use_prometheus_endpoint",
		"circuit_breaker_state":             "use_prometheus_endpoint",
		"circuit_breaker_failures":          "use_prometheus_endpoint",
		"circuit_breaker_transitions":       "use_prometheus_endpoint",
		"circuit_breaker_success_rate":      "use_prometheus_endpoint",
		"circuit_breaker_last_failure_time": "use_prometheus_endpoint",
	}
}
