package internal

import (
	"net/http"
	"strings"
)

// RegisterConfigRoutes registers all configuration management routes
func RegisterConfigRoutes(mux *http.ServeMux, handler *ConfigHandler) {
	// Configuration management endpoints under /api/v1/config/
	// Note: Admin auth is handled internally by each handler method
	
	// Handle all config routes with a single handler that routes internally
	mux.HandleFunc("/api/v1/config/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/config/")
		path = strings.Trim(path, "/")
		
		// Handle statistics endpoint
		if path == "statistics" && r.Method == "GET" {
			handler.HandleConfigStatistics(w, r)
			return
		}
		
		// Parse path segments
		segments := strings.Split(path, "/")
		if len(segments) == 0 || segments[0] == "" {
			WriteErrorResponse(w, http.StatusBadRequest, "config type is required")
			return
		}
		
		// Route based on path structure and HTTP method
		switch {
		case len(segments) == 1:
			// /api/v1/config/{config_type}
			switch r.Method {
			case "GET":
				handler.HandleConfigList(w, r)
			case "POST":
				handler.HandleConfigCreate(w, r)
			default:
				WriteErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		case len(segments) == 2:
			// /api/v1/config/{config_type}/{id}
			switch r.Method {
			case "GET":
				handler.HandleConfigGet(w, r)
			case "PUT":
				handler.HandleConfigUpdate(w, r)
			case "DELETE":
				handler.HandleConfigDelete(w, r)
			default:
				WriteErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
			}
		default:
			WriteErrorResponse(w, http.StatusNotFound, "endpoint not found")
		}
	})
}
