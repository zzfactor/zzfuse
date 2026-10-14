package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// DBQueryTotal counts total DB queries by operation and status.
	DBQueryTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "db_query_total",
			Help: "Total number of database queries by operation and status.",
		},
		[]string{"operation", "status"},
	)
	// DBQueryLatency measures DB query latency by operation.
	DBQueryLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_latency_seconds",
			Help:    "Database query latency in seconds by operation.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)
)

func registerDBMetrics() {
	prometheus.MustRegister(DBQueryTotal, DBQueryLatency)
}
