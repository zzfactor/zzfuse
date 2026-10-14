package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/medxops/trazr-otel/pkg/repository"
)

// ConfigType represents the supported configuration types
type ConfigType string

const (
	ConfigTypeSchemas      ConfigType = "schemas"
	ConfigTypeProcessors   ConfigType = "processors"
	ConfigTypeEmbeddings   ConfigType = "embeddings"
	ConfigTypeTemplates    ConfigType = "templates"
	ConfigTypeVectorStores ConfigType = "vectorstores"
)

// isValidConfigType validates if the config type is supported
func isValidConfigType(configType ConfigType) bool {
	switch configType {
	case ConfigTypeSchemas, ConfigTypeProcessors, ConfigTypeEmbeddings, ConfigTypeTemplates, ConfigTypeVectorStores:
		return true
	default:
		return false
	}
}

// ConfigQueryParams holds query parameters for configuration endpoints
type ConfigQueryParams struct {
	*CommonQueryParams
	Name     string
	Enabled  *bool
	Priority *int
	Search   string
}

// ParseConfigQueryParams extracts configuration-specific query parameters
func ParseConfigQueryParams(r *http.Request) *ConfigQueryParams {
	commonParams := ParseCommonQueryParams(r, 100) // Default limit for configs
	qv := r.URL.Query()

	// Parse config-specific parameters
	name := strings.TrimSpace(qv.Get("name"))
	search := strings.TrimSpace(qv.Get("search"))

	var enabled *bool
	if enabledStr := qv.Get("enabled"); enabledStr != "" {
		if parseBool(enabledStr) {
			enabled = &[]bool{true}[0]
		} else {
			enabled = &[]bool{false}[0]
		}
	}

	var priority *int
	if priorityStr := qv.Get("priority"); priorityStr != "" {
		if p, err := strconv.Atoi(priorityStr); err == nil {
			priority = &p
		}
	}

	return &ConfigQueryParams{
		CommonQueryParams: commonParams,
		Name:              name,
		Enabled:           enabled,
		Priority:          priority,
		Search:            search,
	}
}

// ConfigResponse represents a generic configuration response
type ConfigResponse struct {
	ID          int64                  `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	CreatedAt   string                 `json:"created_at"`
	UpdatedAt   string                 `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Config      map[string]interface{} `json:"config"`
}

// ConfigListResponse represents a paginated list of configurations
type ConfigListResponse struct {
	Data       []ConfigResponse   `json:"data"`
	Pagination PaginationResponse `json:"pagination"`
}

// ConfigCreateRequest represents a request to create a new configuration
type ConfigCreateRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Config      map[string]interface{} `json:"config"`
}

// ConfigUpdateRequest represents a request to update an existing configuration
type ConfigUpdateRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Enabled     *bool                  `json:"enabled,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// ConfigStatisticsResponse represents configuration statistics
type ConfigStatisticsResponse struct {
	SchemaConfigs      int64 `json:"schema_configs"`
	ProcessorConfigs   int64 `json:"processor_configs"`
	EmbeddingConfigs   int64 `json:"embedding_configs"`
	TemplateConfigs    int64 `json:"template_configs"`
	VectorStoreConfigs int64 `json:"vectorstore_configs"`
	TotalConfigs       int64 `json:"total_configs"`
}

// ConfigHandler handles configuration management endpoints
type ConfigHandler struct {
	configService *ConfigService
	converter     *ConfigConverter
}

// NewConfigHandler creates a new configuration handler
func NewConfigHandler(rsMgr repository.ServiceResourceManagerInterface) *ConfigHandler {
	return &ConfigHandler{
		configService: NewConfigService(rsMgr),
		converter:     NewConfigConverter(),
	}
}

// HandleConfigList handles GET /api/v1/config/{config_type}
func (h *ConfigHandler) HandleConfigList(w http.ResponseWriter, r *http.Request) {
	// Require admin authentication
	if _, ok := requireAdminAuth(w, r); !ok {
		return
	}

	// Extract config type from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid config type path")
		return
	}
	configType := ConfigType(pathParts[3])

	// Validate config type
	if !isValidConfigType(configType) {
		WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid config type: %s", configType))
		return
	}

	// Parse query parameters
	params := ParseConfigQueryParams(r)
	if params.HasValidationErrors() {
		WriteJSONResponse(w, http.StatusBadRequest, params.ValidationErrors)
		return
	}

	ctx := r.Context()

	// Get configurations from service layer
	configs, err := h.configService.ListConfigurations(ctx, string(configType))
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to retrieve configurations: %v", err))
		return
	}

	// Convert service responses to HTTP responses
	var configResponses []ConfigResponse
	for _, config := range configs {
		configResponse := ConfigResponse{
			ID:          config.ID,
			Name:        config.Name,
			Enabled:     config.Enabled,
			Description: config.Description,
			CreatedAt:   config.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:   config.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		configResponses = append(configResponses, configResponse)
	}

	// Apply filtering
	filtered := h.filterConfigs(configResponses, params, configType)

	// Apply pagination (simplified)
	start := 0
	end := len(filtered)
	if params.Limit > 0 && params.Limit < len(filtered) {
		end = params.Limit
	}

	paginatedConfigs := filtered[start:end]

	response := ConfigListResponse{
		Data: paginatedConfigs,
		Pagination: PaginationResponse{
			HasNext: end < len(filtered),
			HasPrev: false,
		},
	}

	WriteJSONResponse(w, http.StatusOK, response)
}

// HandleConfigGet handles GET /api/v1/config/{config_type}/{id}
func (h *ConfigHandler) HandleConfigGet(w http.ResponseWriter, r *http.Request) {
	// Require admin authentication
	if _, ok := requireAdminAuth(w, r); !ok {
		return
	}

	// Extract config type and ID from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 5 {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid config path")
		return
	}
	configType := ConfigType(pathParts[3])
	idStr := pathParts[4]

	// Validate config type
	if !isValidConfigType(configType) {
		WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid config type: %s", configType))
		return
	}

	// Parse ID
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid configuration ID")
		return
	}

	ctx := r.Context()

	// Get configuration from service layer
	config, err := h.configService.GetConfiguration(ctx, string(configType), id)
	if err != nil {
		WriteErrorResponse(w, http.StatusNotFound, fmt.Sprintf("configuration not found: %v", err))
		return
	}

	// Convert service response to HTTP response
	configResponse := &ConfigResponse{
		ID:          config.ID,
		Name:        config.Name,
		Enabled:     config.Enabled,
		Description: config.Description,
		CreatedAt:   config.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   config.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	WriteJSONResponse(w, http.StatusOK, configResponse)
}

// HandleConfigCreate handles POST /api/v1/config/{config_type}
func (h *ConfigHandler) HandleConfigCreate(w http.ResponseWriter, r *http.Request) {
	// Require admin authentication
	if _, ok := requireAdminAuth(w, r); !ok {
		return
	}

	// Extract config type from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 4 {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid config type path")
		return
	}
	configType := ConfigType(pathParts[3])

	// Validate config type
	if !isValidConfigType(configType) {
		WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid config type: %s", configType))
		return
	}

	// Parse request body
	var req ConfigCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}

	ctx := r.Context()

	// Create service request
	serviceReq := &ConfigCreateServiceRequest{
		Name:        req.Name,
		Type:        string(configType),
		Enabled:     req.Enabled,
		Description: req.Description,
		Priority:    req.Priority,
		Metadata:    req.Metadata,
		Config:      req.Config,
	}

	// Create configuration through service layer
	created, err := h.configService.CreateConfiguration(ctx, serviceReq)
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("failed to create configuration: %v", err))
		return
	}

	// Convert service response to HTTP response
	configResponse := &ConfigResponse{
		ID:          created.ID,
		Name:        created.Name,
		Enabled:     created.Enabled,
		Description: created.Description,
		Priority:    created.Priority,
		CreatedAt:   created.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   created.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Metadata:    created.Metadata,
		Config:      created.Config,
	}

	WriteJSONResponse(w, http.StatusCreated, configResponse)
}

// HandleConfigUpdate handles PUT /api/v1/config/{config_type}/{id}
func (h *ConfigHandler) HandleConfigUpdate(w http.ResponseWriter, r *http.Request) {
	// Require admin authentication
	if _, ok := requireAdminAuth(w, r); !ok {
		return
	}

	// Extract config type and ID from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 5 {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid config path")
		return
	}
	configType := ConfigType(pathParts[3])
	idStr := pathParts[4]

	// Validate config type
	if !isValidConfigType(configType) {
		WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid config type: %s", configType))
		return
	}

	// Parse ID
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid configuration ID")
		return
	}

	// Parse request body
	var req ConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields
	if req.Name == nil || *req.Name == "" {
		WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}

	ctx := r.Context()

	// Create service request with proper pointer handling
	name := ""
	if req.Name != nil {
		name = *req.Name
	}
	description := ""
	if req.Description != nil {
		description = *req.Description
	}
	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	priority := 0
	if req.Priority != nil {
		priority = *req.Priority
	}
	
	serviceReq := &ConfigCreateServiceRequest{
		Name:        name,
		Type:        string(configType),
		Enabled:     enabled,
		Description: description,
		Priority:    priority,
		Metadata:    req.Metadata,
		Config:      req.Config,
	}

	// Update configuration through service layer
	updated, err := h.configService.UpdateConfiguration(ctx, string(configType), id, serviceReq)
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("failed to update configuration: %v", err))
		return
	}

	// Convert service response to HTTP response
	configResponse := &ConfigResponse{
		ID:          updated.ID,
		Name:        updated.Name,
		Enabled:     updated.Enabled,
		Description: updated.Description,
		Priority:    updated.Priority,
		CreatedAt:   updated.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   updated.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Metadata:    updated.Metadata,
		Config:      updated.Config,
	}

	WriteJSONResponse(w, http.StatusOK, configResponse)
}

// HandleConfigDelete handles DELETE /api/v1/config/{config_type}/{id}
func (h *ConfigHandler) HandleConfigDelete(w http.ResponseWriter, r *http.Request) {
	// Require admin authentication
	if _, ok := requireAdminAuth(w, r); !ok {
		return
	}

	// Extract config type and ID from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 5 {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid config path")
		return
	}
	configType := ConfigType(pathParts[3])
	idStr := pathParts[4]

	// Validate config type
	if !isValidConfigType(configType) {
		WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("invalid config type: %s", configType))
		return
	}

	// Parse ID
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		WriteErrorResponse(w, http.StatusBadRequest, "invalid configuration ID")
		return
	}

	ctx := r.Context()

	// Delete configuration through service layer
	err = h.configService.DeleteConfiguration(ctx, string(configType), id)
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to delete configuration: %v", err))
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleConfigStatistics handles GET /api/v1/config/statistics
func (h *ConfigHandler) HandleConfigStatistics(w http.ResponseWriter, r *http.Request) {
	// Require admin authentication
	if _, ok := requireAdminAuth(w, r); !ok {
		return
	}

	// Get configuration statistics from service layer
	stats, err := h.configService.GetConfigurationStatistics(r.Context())
	if err != nil {
		WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("failed to get statistics: %v", err))
		return
	}

	response := ConfigStatisticsResponse{
		SchemaConfigs:      stats.SchemaConfigs,
		ProcessorConfigs:   stats.ProcessorConfigs,
		EmbeddingConfigs:   stats.EmbeddingConfigs,
		TemplateConfigs:    stats.TemplateConfigs,
		VectorStoreConfigs: stats.VectorStoreConfigs,
		TotalConfigs:       stats.TotalConfigs,
	}

	WriteJSONResponse(w, http.StatusOK, response)
}


// filterConfigs applies filtering based on query parameters
func (h *ConfigHandler) filterConfigs(configs []ConfigResponse, params *ConfigQueryParams, configType ConfigType) []ConfigResponse {
	if params.Name == "" && params.Enabled == nil && params.Priority == nil && params.Search == "" {
		return configs
	}

	var filtered []ConfigResponse
	for _, config := range configs {
		// Filter by name
		if params.Name != "" && !strings.Contains(strings.ToLower(config.Name), strings.ToLower(params.Name)) {
			continue
		}

		// Filter by enabled status
		if params.Enabled != nil && config.Enabled != *params.Enabled {
			continue
		}

		// Filter by priority (only applicable for schema configs)
		if params.Priority != nil && configType == ConfigTypeSchemas && config.Priority != *params.Priority {
			continue
		}

		// Filter by search term (name or description)
		if params.Search != "" {
			searchTerm := strings.ToLower(params.Search)
			if !strings.Contains(strings.ToLower(config.Name), searchTerm) &&
				!strings.Contains(strings.ToLower(config.Description), searchTerm) {
				continue
			}
		}

		filtered = append(filtered, config)
	}

	return filtered
}
