package internal

import (
	"net/http"
)

// HTTP request size limits
const (
	DefaultMaxRequestSize = 10 << 20 // 10MB for general requests
	MaxTraceRequestSize   = 50 << 20 // 50MB for trace data
	MaxLogRequestSize     = 20 << 20 // 20MB for log data
	MaxMetricRequestSize  = 20 << 20 // 20MB for metric data
	MaxAPIRequestSize     = 1 << 20  // 1MB for API calls
)

// WithMaxBodySize wraps an http.Handler with request body size limiting
func WithMaxBodySize(next http.Handler, maxBytes int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next.ServeHTTP(w, r)
	})
}

// WithMaxBodySizeFunc wraps an http.HandlerFunc with request body size limiting
func WithMaxBodySizeFunc(next http.HandlerFunc, maxBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		}
		next(w, r)
	}
}

// LimitedBodyMiddleware returns middleware that limits request body size
func LimitedBodyMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return WithMaxBodySize(next, maxBytes)
	}
}

// APIRequestLimiter limits API request bodies to 1MB
func APIRequestLimiter(next http.HandlerFunc) http.HandlerFunc {
	return WithMaxBodySizeFunc(next, MaxAPIRequestSize)
}

// TraceRequestLimiter limits trace request bodies to 50MB
func TraceRequestLimiter(next http.HandlerFunc) http.HandlerFunc {
	return WithMaxBodySizeFunc(next, MaxTraceRequestSize)
}

// LogRequestLimiter limits log request bodies to 20MB
func LogRequestLimiter(next http.HandlerFunc) http.HandlerFunc {
	return WithMaxBodySizeFunc(next, MaxLogRequestSize)
}

// MetricRequestLimiter limits metric request bodies to 20MB
func MetricRequestLimiter(next http.HandlerFunc) http.HandlerFunc {
	return WithMaxBodySizeFunc(next, MaxMetricRequestSize)
}
