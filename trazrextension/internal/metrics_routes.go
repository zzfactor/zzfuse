package internal

import (
	"net/http"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// RegisterMetricRoutes registers all metric-related endpoints
func RegisterMetricRoutes(mux *http.ServeMux, metricWriterService repository.MetricWriterService, metricQueryService repository.MetricQueryService) {
	// Register metrics endpoint (JWT required, method dispatch)
	mux.Handle("/api/v1/metrics", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			NewMetricInsertHandler(metricWriterService)(w, r)
		case http.MethodGet:
			NewMetricQueryV1Handler(metricQueryService)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// Register metric fetch-by-ID endpoint (JWT required)
	mux.Handle("/api/v1/metrics/", auth.UserContextMiddleware(http.HandlerFunc(NewMetricGetByIDHandler(metricQueryService))))
	
	// Register metric cursor generation endpoint (JWT required)
	mux.Handle("/api/v1/metrics/cursor", auth.UserContextMiddleware(http.HandlerFunc(NewMetricCursorGenerationHandler(metricQueryService))))
}
