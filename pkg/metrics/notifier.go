package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// NotifierDelivered counts delivered notifications per topic.
	NotifierDelivered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifier_notifications_delivered_total",
			Help: "Total notifications delivered per topic.",
		},
		[]string{"topic"},
	)
	// NotifierDropped counts dropped notifications per topic.
	NotifierDropped = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "notifier_notifications_dropped_total",
			Help: "Total notifications dropped per topic.",
		},
		[]string{"topic"},
	)
)

func registerDBNotifierMetrics() {
	prometheus.MustRegister(NotifierDelivered, NotifierDropped)
}
