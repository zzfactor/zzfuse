package repository

import "github.com/medxops/trazr-otel/pkg/event"

// EventType represents specific event types.
// This keeps business logic separate from the generic event bus.
type EventType event.EventType

const (
	// Core telemetry events
	EventTypeLogInserted     EventType = "log.inserted"
	EventTypeTraceInserted   EventType = "trace.inserted"
	EventTypeProfileInserted EventType = "profile.inserted"

	// Run events
	EventTypeRunCreated EventType = "run.created"
	EventTypeRunUpdated EventType = "run.updated"

	// Metric events (unified)
	EventTypeMetricInserted EventType = "metric.inserted"

	// User notification events
	EventTypeNotificationPublished EventType = "notification.published"

	// Specific metric type events (for backward compatibility)
	EventTypeGaugeMetricInserted                EventType = "metric.gauge.inserted"
	EventTypeSumMetricInserted                  EventType = "metric.sum.inserted"
	EventTypeHistogramMetricInserted            EventType = "metric.histogram.inserted"
	EventTypeSummaryMetricInserted              EventType = "metric.summary.inserted"
	EventTypeExponentialHistogramMetricInserted EventType = "metric.exponential_histogram.inserted"
)

// ToEventType converts EventType to the generic EventType
func (t EventType) ToEventType() event.EventType {
	return event.EventType(t)
}

// FromEventType converts generic EventType to EventType
func FromEventType(et event.EventType) EventType {
	return EventType(et)
}

// IsEvent checks if an event type is a telemetry event
func IsEvent(et event.EventType) bool {
	switch EventType(et) {
	case EventTypeLogInserted,
		EventTypeTraceInserted,
		EventTypeProfileInserted,
		EventTypeRunCreated,
		EventTypeRunUpdated,
		EventTypeMetricInserted,
		EventTypeNotificationPublished,
		EventTypeGaugeMetricInserted,
		EventTypeSumMetricInserted,
		EventTypeHistogramMetricInserted,
		EventTypeSummaryMetricInserted,
		EventTypeExponentialHistogramMetricInserted:
		return true
	default:
		return false
	}
}

// GetEventCategory returns the category of a telemetry event
func GetEventCategory(et event.EventType) string {
	switch EventType(et) {
	case EventTypeLogInserted:
		return "logs"
	case EventTypeTraceInserted:
		return "traces"
	case EventTypeProfileInserted:
		return "profiles"
	case EventTypeRunCreated,
		EventTypeRunUpdated:
		return "runs"
	case EventTypeMetricInserted,
		EventTypeGaugeMetricInserted,
		EventTypeSumMetricInserted,
		EventTypeHistogramMetricInserted,
		EventTypeSummaryMetricInserted,
		EventTypeExponentialHistogramMetricInserted:
		return "metrics"
	case EventTypeNotificationPublished:
		return "notifications"
	default:
		return "unknown"
	}
}
