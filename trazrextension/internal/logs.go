package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/auth"
	"github.com/medxops/trazr-otel/pkg/repository"
)

type LogQueryService interface {
	QueryLogs(ctx context.Context, q LogQuery) (LogQueryResult, error)
	StreamLogs(ctx context.Context, q LogQuery) (<-chan map[string]interface{}, error)
	GenerateCursor(ctx context.Context, offset int, filters LogQuery) (LogQueryResult, error)
}

// LogQuery defines filters for log queries.
type LogQuery struct {
	TimeRange
	ServiceName string
	Severity    string
	Limit       int
	Schema      string
	Cursor      string // for cursor-based pagination
}

type LogQueryResult struct {
	Entries    []map[string]interface{} `json:"data"`
	NextCursor string                   `json:"next_cursor"`
	HasMore    bool                     `json:"has_more"`
	PrevCursor string                   `json:"prev_cursor,omitempty"`
	HasPrev    bool                     `json:"has_prev,omitempty"`
}

// LogEntryDTO is a data transfer object for incoming log entries
// with plain Go types for easy JSON unmarshalling.
type LogEntryDTO struct {
	Timestamp             string                 `json:"timestamp"`
	ServiceName           string                 `json:"service_name"`
	SeverityText          string                 `json:"severity_text,omitempty"`
	SeverityNumber        int                    `json:"severity,omitempty"`
	Body                  string                 `json:"body"`
	Attributes            map[string]interface{} `json:"attributes,omitempty"`
	TraceID               string                 `json:"trace_id,omitempty"`
	SpanID                string                 `json:"span_id,omitempty"`
	Flags                 int                    `json:"flags,omitempty"`
	ResourceAttributes    map[string]interface{} `json:"resource_attributes,omitempty"`
	Metadata              map[string]interface{} `json:"metadata,omitempty"`
	ObservedTimestamp     string                 `json:"observed_timestamp,omitempty"`
	ResourceSchemaURL     string                 `json:"resource_schema_url,omitempty"`
	ScopeName             string                 `json:"scope_name,omitempty"`
	ScopeVersion          string                 `json:"scope_version,omitempty"`
	ScopeAttributes       map[string]interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount int                    `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL        string                 `json:"scope_schema_url,omitempty"`
	DroppedAttrCount      int                    `json:"dropped_attr_count,omitempty"`
}

// ToLogEntry converts LogEntryDTO to repository.LogEntry
func (dto LogEntryDTO) ToLogEntry() (repository.LogEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repository.LogEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	// Defensive type check for Metadata
	if dto.Metadata != nil {
		if _, ok := any(dto.Metadata).(map[string]interface{}); !ok {
			return repository.LogEntry{}, fmt.Errorf("metadata must be a JSON object (map[string]interface{}), got %T", dto.Metadata)
		}
	}
	var observedTimestamp *time.Time
	if dto.ObservedTimestamp != "" {
		ot, err := time.Parse(time.RFC3339, dto.ObservedTimestamp)
		if err == nil {
			observedTimestamp = &ot
		}
	}
	attributesJSON := MarshalOrDefault(dto.Attributes, []byte("{}"))
	resourceAttrsJSON := MarshalOrDefault(dto.ResourceAttributes, []byte("{}"))
	scopeAttrsJSON := MarshalOrDefault(dto.ScopeAttributes, []byte("{}"))
	metadataJSON := MarshalOrDefault(dto.Metadata, []byte("{}"))
	bodyJSON := []byte("null")
	if dto.Body != "" {
		bodyJSON, _ = json.Marshal(dto.Body)
	}
	return repository.LogEntry{
		ID:                    0,
		TimestampNS:           t.UnixNano(),
		Timestamp:             t,
		ObservedTimestamp:     observedTimestamp,
		TraceID:               PtrFromString(dto.TraceID),
		SpanID:                PtrFromString(dto.SpanID),
		Flags:                 PtrFromInt32(int32(dto.Flags)),
		SeverityText:          PtrFromString(dto.SeverityText),
		SeverityNumber:        PtrFromInt32(int32(dto.SeverityNumber)),
		ServiceName:           dto.ServiceName,
		ResourceAttributes:    resourceAttrsJSON,
		ResourceSchemaURL:     PtrFromString(dto.ResourceSchemaURL),
		ScopeName:             PtrFromString(dto.ScopeName),
		ScopeVersion:          PtrFromString(dto.ScopeVersion),
		ScopeAttributes:       scopeAttrsJSON,
		ScopeDroppedAttrCount: PtrFromInt32(int32(dto.ScopeDroppedAttrCount)),
		ScopeSchemaURL:        PtrFromString(dto.ScopeSchemaURL),
		Attributes:            attributesJSON,
		DroppedAttrCount:      PtrFromInt32(int32(dto.DroppedAttrCount)),
		Body:                  bodyJSON,
		Metadata:              metadataJSON,
	}, nil
}

// NewLogCursorGenerationHandler creates a new handler for generating cursors
func NewLogCursorGenerationHandler(service repository.LogQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}

		// Parse offset parameter
		offsetStr := r.URL.Query().Get("offset")
		if offsetStr == "" {
			http.Error(w, "offset parameter is required", http.StatusBadRequest)
			return
		}

		offset, err := strconv.Atoi(offsetStr)
		if err != nil || offset < 0 {
			http.Error(w, "offset must be a non-negative integer", http.StatusBadRequest)
			return
		}

		// Parse filter parameters
		startTime := parseTimeOrNil(r.URL.Query().Get("timestamp_gte"))
		endTime := parseTimeOrNil(r.URL.Query().Get("timestamp_lte"))

		var timeRange TimeRange
		if startTime != nil {
			timeRange.Start = *startTime
		}
		if endTime != nil {
			timeRange.End = *endTime
		}

		filters := LogQuery{
			TimeRange:   timeRange,
			ServiceName: r.URL.Query().Get("service"),
			Severity:    r.URL.Query().Get("severity"),
			Schema:      schema,
		}

		// Convert internal LogQuery to repository LogQuery
		var repoStartTime, repoEndTime *time.Time
		if !filters.TimeRange.Start.IsZero() {
			repoStartTime = &filters.TimeRange.Start
		}
		if !filters.TimeRange.End.IsZero() {
			repoEndTime = &filters.TimeRange.End
		}

		repoQuery := repository.LogQuery{
			TimeRange: repository.TimeRange{Start: repoStartTime, End: repoEndTime},
			Severity:  filters.Severity,
		}
		// Set embedded CommonBaseFilters fields
		repoQuery.CommonBaseFilters.Schema = filters.Schema
		repoQuery.CommonBaseFilters.ServiceName = filters.ServiceName

		// Generate cursor using existing service
		result, err := service.GenerateCursor(ctx, offset, repoQuery)
		if err != nil {
			// Handle specific error types
			if strings.Contains(err.Error(), "invalid offset") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.Contains(err.Error(), "no logs found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Return success response using existing LogQueryResult
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(result)
	}
}

func NewLogGetByIDHandler(service repository.LogQueryService) http.HandlerFunc {
	return CreateGetByIDHandler("/api/v1/logs/", service.GetLogByID)
}

// NewLogInsertHandler returns an http.HandlerFunc for POST /api/v1/logs
func NewLogInsertHandler(logWriter repository.LogWriterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get user context from request context
		userCtx, err := auth.GetUserContext(r.Context())
		if err != nil {
			writeInsertError(w, http.StatusUnauthorized, "user context not found")
			return
		}

		// Get schema parameter, default to "otel" if not provided
		schema := r.URL.Query().Get("schema")
		if schema == "" {
			schema = DefaultSchema
		}

		// Check if user has access to the requested schema
		if !auth.UserHasSchemaAccess(r.Context(), schema) {
			writeInsertError(w, http.StatusForbidden, "access denied to schema: "+schema)
			return
		}
		var dtos []LogEntryDTO
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeInsertError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		if err := json.Unmarshal(body, &dtos); err != nil {
			// Try single object
			var singleDTO LogEntryDTO
			if err2 := json.Unmarshal(body, &singleDTO); err2 != nil {
				writeInsertError(w, http.StatusBadRequest, "invalid log entry format")
				return
			}
			dtos = []LogEntryDTO{singleDTO}
		}
		if len(dtos) == 0 {
			writeInsertError(w, http.StatusBadRequest, "no log entries provided")
			return
		}
		var entries []repository.LogEntry
		for _, dto := range dtos {
			entry, err := dto.ToLogEntry()
			if err != nil {
				writeInsertError(w, http.StatusBadRequest, err.Error())
				return
			}
			entries = append(entries, entry)
		}
		// Insert logs with user context available in request context
		ids, err := logWriter.InsertLogs(r.Context(), entries, schema)
		if err != nil {
			writeInsertError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Log successful insertion with user info for audit trail
		fmt.Printf("[AUDIT] User %d (%s) inserted %d log entries to schema %s\n",
			userCtx.UserID, userCtx.Email, len(ids), schema)
		writeInsertResult(w, ids)
	}
}

// LogQueryV1Handler returns an http.HandlerFunc for GET /api/v1/logs
func NewLogQueryV1Handler(service repository.LogQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		qv := r.URL.Query()

		// Get user context from request context
		userCtx, err := auth.GetUserContext(ctx)
		if err != nil {
			WriteErrorResponse(w, http.StatusUnauthorized, "user context not found")
			return
		}

		// Parse common query parameters using DRY helper
		commonParams := ParseCommonQueryParams(r, 100)

		// Check if user has access to the requested schema
		if !auth.UserHasSchemaAccess(ctx, commonParams.Schema) {
			WriteErrorResponse(w, http.StatusForbidden, "access denied to schema: "+commonParams.Schema)
			return
		}

		// Extract logs-specific parameters
		severity := qv.Get("severity") // legacy single
		severityMinStr := qv.Get("severity_min")
		severityMaxStr := qv.Get("severity_max")
		severityGroupIn := qv["severity_group_in"]
		severityTextIn := qv["severity_text_in"]

		// Use common parameters
		validationErrors := commonParams.ValidationErrors
		start := commonParams.Start
		end := commonParams.End
		countOnly := commonParams.CountOnly

		// Parse and validate severity parameters
		var severityMin, severityMax *int
		if severityMinStr != "" {
			if v, err := strconv.Atoi(severityMinStr); err == nil {
				severityMin = &v
			} else {
				validationErrors.AddError("severity_min", "severity_min must be a valid number")
			}
		}
		if severityMaxStr != "" {
			if v, err := strconv.Atoi(severityMaxStr); err == nil {
				severityMax = &v
			} else {
				validationErrors.AddError("severity_max", "severity_max must be a valid number")
			}
		}

		// Advanced filters will be handled by SetCommonAdvancedFilters helper

		// Check for validation errors before proceeding
		if validationErrors.HasErrors() {
			WriteValidationErrors(w, validationErrors)
			return
		}

		// Build query with wildcard service filtering and advanced filters
		query := repository.LogQuery{
			TimeRange: repository.TimeRange{Start: start, End: end},
			// Log-specific fields
			Severity:       severity,
			SeverityMin:    severityMin,
			SeverityMax:    severityMax,
			SeverityGroups: upperSlice(severityGroupIn),
			SeverityTextIn: severityTextIn,
		}

		// Set embedded filter fields using DRY helpers
		SetCommonBaseFilters(&query, commonParams)
		SetCommonAdvancedFilters(&query, r)

		// Debug logging
		fmt.Printf("[DEBUG] LogQuery: TimeRange={Start: %v, End: %v}, Schema=%s, Limit=%d, Direction=%s\n",
			query.TimeRange.Start, query.TimeRange.End, query.CommonBaseFilters.Schema,
			query.CommonBaseFilters.Limit, query.CommonBaseFilters.Direction)

		// Count-only mode
		if countOnly {
			// Use a type assertion to check for CountLogs support to avoid compile-time coupling
			if counter, ok := any(service).(interface {
				CountLogs(context.Context, repository.LogQuery) (int64, error)
			}); ok {
				cnt, err := counter.CountLogs(ctx, query)
				if err != nil {
					WriteErrorResponse(w, http.StatusInternalServerError, err.Error())
					return
				}
				WriteJSONResponse(w, http.StatusOK, map[string]int64{"count": cnt})
				return
			}
			WriteErrorResponse(w, http.StatusNotImplemented, "count_only not supported by this service")
			return
		}

		// Default full-result mode
		result, err := service.QueryLogs(ctx, query)
		if err != nil {
			fmt.Printf("[DEBUG] QueryLogs error: %v\n", err)
			// Sanitize database errors before exposing to users
			sanitizedError := SanitizeDatabaseError(err)
			WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
			return
		}

		// Log successful query with user info for audit trail
		fmt.Printf("[AUDIT] User %d (%s) queried %d log entries from schema %s\n",
			userCtx.UserID, userCtx.Email, len(result.Entries), commonParams.Schema)
		fmt.Printf("[DEBUG] QueryLogs result: %d entries, NextCursor=%s, HasMore=%v\n",
			len(result.Entries), result.NextCursor, result.HasMore)

		// Transform to standardized paginated response
		paginatedResponse := NewPaginatedResponse(
			result.Entries,
			result.NextCursor,
			result.PrevCursor,
			result.HasMore,
			result.HasPrev,
		)

		WriteJSONResponse(w, http.StatusOK, paginatedResponse)
	}
}
