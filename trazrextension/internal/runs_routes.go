package internal

import (
	"net/http"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// RegisterRunRoutes registers all run-related endpoints
func RegisterRunRoutes(mux *http.ServeMux, runWriterService repository.RunWriterService, runQueryService repository.RunQueryService) {
	// Register runs endpoint (JWT required, method dispatch)
	mux.Handle("/api/v1/runs", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			NewRunInsertHandler(runWriterService)(w, r)
		case http.MethodGet:
			NewRunQueryV1Handler(runQueryService)(w, r)
		case http.MethodPut:
			NewRunUpdateHandler(runWriterService)(w, r)
		case http.MethodDelete:
			NewRunEndHandler(runWriterService)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
	
	// Register run fetch-by-ID endpoint (JWT required)
	mux.Handle("/api/v1/runs/", auth.UserContextMiddleware(http.HandlerFunc(NewRunGetByIDHandler(runQueryService))))
}
