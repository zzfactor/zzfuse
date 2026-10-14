package trazrextension

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"
)

// Schema represents a database schema configuration
type Schema struct {
	ID          int                    `json:"id"`
	Name        *string                `json:"name,omitempty"`
	Schema      string                 `json:"schema"`
	Enabled     bool                   `json:"enabled"`
	Routing     map[string]interface{} `json:"routing"`
	Filter      map[string]interface{} `json:"filter"`
	Description *string                `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// SchemaList represents the response for the schemas endpoint
type SchemaList struct {
	Schemas []Schema `json:"schemas"`
	Total   int      `json:"total"`
}

// NewSchemasHandler creates a new HTTP handler for the schemas endpoint
func NewSchemasHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Query all schemas from the config_otel_database table
		// We need to use a default schema to query the config table
		query := `
			SELECT 
				id, 
				name, 
				schema, 
				enabled, 
				routing, 
				filter, 
				description, 
				metadata, 
				created_at, 
				updated_at
			FROM config_otel_database 
			ORDER BY id ASC
		`

		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			http.Error(w, "failed to query schemas: "+err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		var schemas []Schema
		for rows.Next() {
			var schema Schema
			var routingJSON, filterJSON, metadataJSON []byte
			var name, description sql.NullString

			err := rows.Scan(
				&schema.ID,
				&name,
				&schema.Schema,
				&schema.Enabled,
				&routingJSON,
				&filterJSON,
				&description,
				&metadataJSON,
				&schema.CreatedAt,
				&schema.UpdatedAt,
			)
			if err != nil {
				http.Error(w, "failed to scan schema row: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Handle nullable string fields
			if name.Valid {
				schema.Name = &name.String
			}
			if description.Valid {
				schema.Description = &description.String
			}

			// Parse JSONB fields
			if err := json.Unmarshal(routingJSON, &schema.Routing); err != nil {
				schema.Routing = make(map[string]interface{})
			}
			if err := json.Unmarshal(filterJSON, &schema.Filter); err != nil {
				schema.Filter = make(map[string]interface{})
			}
			if err := json.Unmarshal(metadataJSON, &schema.Metadata); err != nil {
				schema.Metadata = make(map[string]interface{})
			}

			schemas = append(schemas, schema)
		}

		if err = rows.Err(); err != nil {
			http.Error(w, "error iterating schema rows: "+err.Error(), http.StatusInternalServerError)
			return
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
			http.Error(w, "failed to encode response: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
