package internal

import (
	"net/http"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// RegisterProfileRoutes registers all profile-related endpoints
func RegisterProfileRoutes(mux *http.ServeMux, profileWriterService repository.ProfileWriterService, profileQueryService repository.ProfileQueryService) {
	// Register [BETA] profiles endpoint (JWT required, method dispatch)
	mux.Handle("/api/v1/profiles", auth.UserContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			NewProfileInsertHandler(profileWriterService)(w, r)
		case http.MethodGet:
			NewProfileQueryHandler(profileQueryService)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})))
}
