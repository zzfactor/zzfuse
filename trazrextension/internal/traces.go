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

// API response structs with JSON-friendly types

// Remove the local TraceEntry struct definition.
// Use map[string]interface{} for trace entries.
// Update all references and type usages accordingly.
type TraceQuery struct {
	TimeRange
	ServiceName string
	TraceID     string
	Limit       int
	Schema      string
	Cursor      string // for cursor-based pagination
}

type TraceQueryResult struct {
	Entries    []map[string]interface{} `json:"data"`
	NextCursor string                   `json:"next_cursor"`
	HasMore    bool                     `json:"has_more"`
	PrevCursor string                   `json:"prev_cursor,omitempty"`
	HasPrev    bool                     `json:"has_prev,omitempty"`
}

// TraceEntryDTO is a data transfer object for incoming trace entries
// with plain Go types for easy JSON unmarshalling.
type TraceEntryDTO struct {
	Timestamp             string                 `json:"timestamp"`
	TraceID               string                 `json:"trace_id"`
	SpanID                string                 `json:"span_id"`
	ParentSpanID          string                 `json:"parent_span_id,omitempty"`
	TraceState            string                 `json:"trace_state,omitempty"`
	Name                  string                 `json:"name"`
	SpanKind              string                 `json:"span_kind,omitempty"`
	ServiceName           string                 `json:"service_name"`
	ResourceAttributes    map[string]interface{} `json:"resource_attributes,omitempty"`
	ResourceSchemaURL     string                 `json:"resource_schema_url,omitempty"`
	ScopeName             string                 `json:"scope_name,omitempty"`
	ScopeVersion          string                 `json:"scope_version,omitempty"`
	ScopeAttributes       map[string]interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount int                    `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL        string                 `json:"scope_schema_url,omitempty"`
	Attributes            map[string]interface{} `json:"attributes,omitempty"`
	DroppedAttrCount      int                    `json:"dropped_attr_count,omitempty"`
	Events                []interface{}          `json:"events,omitempty"`
	DroppedEventsCount    int                    `json:"dropped_events_count,omitempty"`
	Links                 []interface{}          `json:"links,omitempty"`
	DroppedLinksCount     int                    `json:"dropped_links_count,omitempty"`
	StatusCode            int                    `json:"status_code,omitempty"`
	StatusMessage         string                 `json:"status_message,omitempty"`
	StartTime             string                 `json:"start_time,omitempty"`
	EndTime               string                 `json:"end_time,omitempty"`
	DurationNS            int64                  `json:"duration_ns,omitempty"`
	Metadata              map[string]interface{} `json:"metadata,omitempty"`
	ProcessedContent      map[string]interface{} `json:"processed_content,omitempty"`
}
type TraceQueryService interface {
	QuerySpans(ctx context.Context, q TraceQuery) (TraceQueryResult, error)
	StreamSpans(ctx context.Context, q TraceQuery) (<-chan map[string]interface{}, error)
	GenerateCursor(ctx context.Context, offset int, filters TraceQuery) (TraceQueryResult, error)
}

func NewSpanGetByIDHandler(service repository.TraceQueryService) http.HandlerFunc {
	return CreateGetByIDHandler("/api/v1/spans/", service.GetSpanByID)
}

// ToTraceEntry converts TraceEntryDTO to repository.TraceEntry
func (dto TraceEntryDTO) ToTraceEntry() (repository.TraceEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repository.TraceEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	startTime := t
	if dto.StartTime != "" {
		startTime, _ = time.Parse(time.RFC3339, dto.StartTime)
	}
	endTime := t
	if dto.EndTime != "" {
		endTime, _ = time.Parse(time.RFC3339, dto.EndTime)
	}
	attributesJSON := MarshalOrDefault(dto.Attributes, []byte("{}"))
	resourceAttrsJSON := MarshalOrDefault(dto.ResourceAttributes, []byte("{}"))
	scopeAttrsJSON := MarshalOrDefault(dto.ScopeAttributes, []byte("{}"))
	metadataJSON := MarshalOrDefault(dto.Metadata, []byte("{}"))
	eventsJSON := MarshalOrDefault(dto.Events, []byte("[]"))
	linksJSON := MarshalOrDefault(dto.Links, []byte("[]"))
	return repository.TraceEntry{
		ID:                    0, // Use 0 for int64 ID field
		TimestampNS:           t.UnixNano(),
		Timestamp:             t,
		TraceID:               dto.TraceID,
		SpanID:                dto.SpanID,
		ParentSpanID:          PtrFromString(dto.ParentSpanID),
		TraceState:            PtrFromString(dto.TraceState),
		Name:                  dto.Name,
		SpanKind:              dto.SpanKind,
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
		Events:                eventsJSON,
		DroppedEventsCount:    PtrFromInt32(int32(dto.DroppedEventsCount)),
		Links:                 linksJSON,
		DroppedLinksCount:     PtrFromInt32(int32(dto.DroppedLinksCount)),
		StatusCode:            PtrFromInt32(int32(dto.StatusCode)),
		StatusMessage:         PtrFromString(dto.StatusMessage),
		StartTime:             startTime,
		EndTime:               endTime,
		DurationNS:            dto.DurationNS,
		Metadata:              metadataJSON,
	}, nil
}

// NewTraceInsertHandler returns an http.HandlerFunc for POST /api/v1/spans
func NewTraceInsertHandler(traceWriter repository.TraceWriterService) http.HandlerFunc {
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

		var dtos []TraceEntryDTO
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeInsertError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		if err := json.Unmarshal(body, &dtos); err != nil {
			// Try single object
			var singleDTO TraceEntryDTO
			if err2 := json.Unmarshal(body, &singleDTO); err2 != nil {
				writeInsertError(w, http.StatusBadRequest, "invalid trace entry format")
				return
			}
			dtos = []TraceEntryDTO{singleDTO}
		}
		if len(dtos) == 0 {
			writeInsertError(w, http.StatusBadRequest, "no trace entries provided")
			return
		}
		var entries []repository.TraceEntry
		for _, dto := range dtos {
			entry, err := dto.ToTraceEntry()
			if err != nil {
				writeInsertError(w, http.StatusBadRequest, err.Error())
				return
			}
			entries = append(entries, entry)
		}
		// Insert spans with user context available in request context
		ids, err := traceWriter.InsertSpans(r.Context(), entries, schema)
		if err != nil {
			writeInsertError(w, http.StatusInternalServerError, err.Error())
			return
		}

		// Log successful insertion with user info for audit trail
		fmt.Printf("[AUDIT] User %d (%s) inserted %d span entries to schema %s\n",
			userCtx.UserID, userCtx.Email, len(ids), schema)
		writeInsertResult(w, ids)
	}
}

// TraceQueryV1Handler returns an http.HandlerFunc for GET /api/v1/spans
// NewTraceCursorGenerationHandler creates a new handler for generating cursors for traces
func NewTraceCursorGenerationHandler(service repository.TraceQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		filters := TraceQuery{
			TimeRange:   timeRange,
			ServiceName: r.URL.Query().Get("service"),
			Schema:      schema,
		}

		// Convert internal TraceQuery to repository TraceQuery
		var repoStartTime, repoEndTime *time.Time
		if !filters.TimeRange.Start.IsZero() {
			repoStartTime = &filters.TimeRange.Start
		}
		if !filters.TimeRange.End.IsZero() {
			repoEndTime = &filters.TimeRange.End
		}

		repoQuery := repository.TraceQuery{
			TimeRange: repository.TimeRange{Start: repoStartTime, End: repoEndTime},
		}
		// Set embedded CommonBaseFilters fields
		repoQuery.CommonBaseFilters.ServiceName = filters.ServiceName
		repoQuery.CommonBaseFilters.Schema = filters.Schema

		// Generate cursor using existing service
		result, err := service.GenerateCursor(r.Context(), offset, repoQuery)
		if err != nil {
			// Handle specific error types
			if strings.Contains(err.Error(), "invalid offset") {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.Contains(err.Error(), "no spans found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
			return
		}

		// Return success response using existing TraceQueryResult
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(result)
	}
}

func NewTraceQueryV1Handler(service repository.TraceQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get user context from request context
		userCtx, userErr := auth.GetUserContext(ctx)
		if userErr != nil {
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

		// Extract traces-specific parameters
		traceID := r.URL.Query().Get("trace_id")

		// Use common parameters
		validationErrors := commonParams.ValidationErrors
		start := commonParams.Start
		end := commonParams.End
		serviceName := commonParams.ServiceName
		limit := commonParams.Limit
		direction := commonParams.Direction
		direction_results := commonParams.DirectionResults
		schema := commonParams.Schema

		// Check for validation errors before proceeding
		if validationErrors.HasErrors() {
			WriteValidationErrors(w, validationErrors)
			return
		}
		q := repository.TraceQuery{
			TimeRange: repository.TimeRange{Start: start, End: end},
			TraceID:   traceID,
		}
		// Set embedded CommonBaseFilters fields
		q.CommonBaseFilters.ServiceName = serviceName
		q.CommonBaseFilters.Limit = limit
		q.CommonBaseFilters.Schema = schema
		q.CommonBaseFilters.Direction = direction
		q.CommonBaseFilters.DirectionResults = direction_results
		// Handle count-only mode similar to logs
		countOnly := commonParams.CountOnly
		if countOnly {
			if counter, ok := any(service).(interface {
				CountSpans(context.Context, repository.TraceQuery) (int64, error)
			}); ok {
				cnt, err := counter.CountSpans(ctx, q)
				if err != nil {
					sanitizedError := SanitizeDatabaseError(err)
					WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
					return
				}
				WriteJSONResponse(w, http.StatusOK, map[string]int64{"count": cnt})
				return
			}
			WriteErrorResponse(w, http.StatusNotImplemented, "count_only not supported by this service")
			return
		}

		var err error
		result, err := service.QuerySpans(ctx, q)
		if err != nil {
			sanitizedError := SanitizeDatabaseError(err)
			WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
			return
		}

		// Log successful query with user info for audit trail
		fmt.Printf("[AUDIT] User %d (%s) queried %d span entries from schema %s\n",
			userCtx.UserID, userCtx.Email, len(result.Entries), commonParams.Schema)

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

// Helper for wildcard filtering for spans
func matchTraceFilter(entry repository.TraceEntry, service, traceID, spanID, spanName string) bool {
	if service != "" && entry.ServiceName != service {
		return false
	}
	if traceID != "" && entry.TraceID != traceID {
		return false
	}
	if spanID != "" && entry.SpanID != spanID {
		return false
	}
	if spanName != "" && entry.Name != spanName {
		return false
	}
	return true
}
