package internal

import (
	"encoding/json"
	"net/http"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

// SchemaList represents the response for the schemas endpoint
type SchemaList struct {
	Schemas []Schema `json:"schemas"`
	Total   int      `json:"total"`
}

// Schema represents a database schema configuration for API response
type Schema struct {
	ID          int                    `json:"id"`
	Name        string                 `json:"name"`
	Schema      string                 `json:"schema"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	Routing     map[string]interface{} `json:"routing"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
}

// NewSchemasHandler creates a new HTTP handler for the schemas endpoint
func NewSchemasHandler(rsMgr repository.ServiceResourceManagerInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			WriteErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if _, err := auth.GetUserContext(r.Context()); err != nil {
			WriteErrorResponse(w, http.StatusUnauthorized, "user context not found")
			return
		}
		// Get schema configurations from the service resource manager
		// We need to access the internal schemaConfigs field
		srm, ok := rsMgr.(*repository.ServiceResourceManager)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid service resource manager type"})
			return
		}

		// Get schema configurations using the public routing schema interface
		routingSchema, err := srm.GetRoutingSchema()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to get routing schema"})
			return
		}
		schemaConfigs, err := routingSchema.GetAllEnabled(r.Context())
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to get schema configurations"})
			return
		}

		// Convert to API response format
		schemas := make([]Schema, 0)
		for _, config := range schemaConfigs {
			if !auth.UserHasSchemaAccess(r.Context(), config.DatabaseSchema) {
				continue
			}
			schema := Schema{
				ID:          int(config.ID),
				Name:        config.Name,
				Schema:      config.DatabaseSchema,
				Enabled:     config.Enabled,
				Priority:    config.Priority,
				Routing:     config.Routing,
				Description: config.Description,
				Metadata:    config.Metadata,
				CreatedAt:   config.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				UpdatedAt:   config.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			schemas = append(schemas, schema)
		}

		// Create response
		response := SchemaList{
			Schemas: schemas,
			Total:   len(schemas),
		}

		// Return JSON response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to encode response: " + err.Error()})
			return
		}
	}
}
