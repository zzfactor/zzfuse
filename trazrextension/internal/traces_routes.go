package internal

import (
	"net/http"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// RegisterTraceRoutes registers all trace/span-related endpoints
func RegisterTraceRoutes(mux *http.ServeMux, traceWriterService repository.TraceWriterService, traceQueryService repository.TraceQueryService) {
	// Register spans endpoint (JWT required, method dispatch)
	mux.Handle("/api/v1/spans", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			NewTraceInsertHandler(traceWriterService)(w, r)
		case http.MethodGet:
			NewTraceQueryV1Handler(traceQueryService)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// Register span fetch-by-ID endpoint (JWT required)
	mux.Handle("/api/v1/spans/", auth.UserContextMiddleware(http.HandlerFunc(NewSpanGetByIDHandler(traceQueryService))))
	
	// Register trace cursor generation endpoint (JWT required)
	mux.Handle("/api/v1/traces/cursor", auth.UserContextMiddleware(http.HandlerFunc(NewTraceCursorGenerationHandler(traceQueryService))))
}
