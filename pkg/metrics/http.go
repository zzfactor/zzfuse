package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// HTTPRequestsTotal counts total HTTP requests by method and code.
	HTTPRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests by method and response code.",
		},
		[]string{"method", "code"},
	)
	// HTTPRequestDuration measures HTTP request duration by method and code.
	HTTPRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds by method and response code.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "code"},
	)
)

func registerHTTPMetrics() {
	prometheus.MustRegister(HTTPRequestsTotal, HTTPRequestDuration)
}
