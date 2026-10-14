package internal

import (
	"net/http"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// RegisterLogRoutes registers all log-related endpoints
func RegisterLogRoutes(mux *http.ServeMux, logWriterService repository.LogWriterService, logQueryService repository.LogQueryService) {
	// Register logs endpoint (JWT required, method dispatch)
	mux.Handle("/api/v1/logs", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			NewLogInsertHandler(logWriterService)(w, r)
		case http.MethodGet:
			NewLogQueryV1Handler(logQueryService)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// Register log fetch-by-ID endpoint (JWT required)
	mux.Handle("/api/v1/logs/", auth.UserContextMiddleware(http.HandlerFunc(NewLogGetByIDHandler(logQueryService))))
	
	// Register log cursor generation endpoint (JWT required)
	mux.Handle("/api/v1/logs/cursor", auth.UserContextMiddleware(http.HandlerFunc(NewLogCursorGenerationHandler(logQueryService))))
}
