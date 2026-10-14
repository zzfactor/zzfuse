package internal

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository"
)

// CommonQueryParams holds the standard query parameters used across all endpoints
type CommonQueryParams struct {
	Start        *time.Time
	End          *time.Time
	ServiceName  string
	ServiceNames []string // service_in[] for multiple services
	// Wildcard service filtering
	ServiceNameLike     string
	ServiceNameRegex    string
	ServiceNamePrefix   string
	ServiceNameSuffix   string
	ServiceNameContains string
	Limit               int
	Direction           string
	DirectionResults    string
	Schema              string
	Cursor              string
	CountOnly           bool
	ValidationErrors    *ValidationErrors
}

// ParseCommonQueryParams extracts and validates common query parameters from HTTP request
func ParseCommonQueryParams(r *http.Request, defaultLimit int) *CommonQueryParams {
	qv := r.URL.Query()
	validationErrors := &ValidationErrors{}

	// Extract raw parameters
	startStr := qv.Get("start")
	endStr := qv.Get("end")
	lastStr := qv.Get("last")
	serviceName := qv.Get("service")
	serviceNames := qv["service_in"]
	// Wildcard service filtering parameters
	serviceNameLike := qv.Get("service_name_like")
	serviceNameRegex := qv.Get("service_name_regex")
	serviceNamePrefix := qv.Get("service_name_prefix")
	serviceNameSuffix := qv.Get("service_name_suffix")
	serviceNameContains := qv.Get("service_name_contains")
	limitStr := qv.Get("limit")
	direction := strings.ToLower(qv.Get("direction"))
	directionResults := strings.ToLower(qv.Get("direction_results"))
	countOnlyParam := strings.ToLower(qv.Get("count_only"))
	schema := getSchemaParam(r)
	cursor := qv.Get("cursor")

	// Set defaults
	if direction != "asc" && direction != "desc" {
		direction = "asc"
	}
	if directionResults != "asc" && directionResults != "desc" {
		directionResults = direction
	}

	// Parse and validate time parameters
	start, startErrors := ParseTimeWithValidation(startStr, "start")
	if startErrors.HasErrors() {
		validationErrors.Errors = append(validationErrors.Errors, startErrors.Errors...)
	}

	end, endErrors := ParseTimeWithValidation(endStr, "end")
	if endErrors.HasErrors() {
		validationErrors.Errors = append(validationErrors.Errors, endErrors.Errors...)
	}

	// Validate time range if both are provided
	if start != nil && end != nil {
		timeRangeErrors := ValidateTimeRange(start, end)
		if timeRangeErrors.HasErrors() {
			validationErrors.Errors = append(validationErrors.Errors, timeRangeErrors.Errors...)
		}
	}

	// Handle default time window if no start/end provided
	if start == nil && end == nil {
		window := 24 * time.Hour // Default window
		if lastStr != "" {
			if dur, err := parseCommonLastWindow(lastStr); err == nil {
				window = dur
			} else {
				validationErrors.AddError("last", "invalid last window format")
			}
		}
		te := time.Now().UTC()
		ts := te.Add(-window)
		start = &ts
		end = &te
	}

	// Parse and validate limit
	limit, limitErrors := ParseAndValidateLimit(limitStr, defaultLimit)
	if limitErrors.HasErrors() {
		validationErrors.Errors = append(validationErrors.Errors, limitErrors.Errors...)
	}

	// Parse count_only flag
	countOnly := countOnlyParam == "true" || countOnlyParam == "1"

	return &CommonQueryParams{
		Start:               start,
		End:                 end,
		ServiceName:         serviceName,
		ServiceNames:        serviceNames,
		ServiceNameLike:     serviceNameLike,
		ServiceNameRegex:    serviceNameRegex,
		ServiceNamePrefix:   serviceNamePrefix,
		ServiceNameSuffix:   serviceNameSuffix,
		ServiceNameContains: serviceNameContains,
		Limit:               limit,
		Direction:           direction,
		DirectionResults:    directionResults,
		Schema:              schema,
		Cursor:              cursor,
		CountOnly:           countOnly,
		ValidationErrors:    validationErrors,
	}
}

// HasValidationErrors returns true if there are any validation errors
func (p *CommonQueryParams) HasValidationErrors() bool {
	return p.ValidationErrors.HasErrors()
}

// parseCommonLastWindow parses shorthand durations like 15m, 1h, 2d (renamed to avoid conflicts)
func parseCommonLastWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration format")
	}
	unit := s[len(s)-1]
	val := s[:len(s)-1]

	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid duration value")
	}

	switch unit {
	case 's':
		return time.Duration(n) * time.Second, nil
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration unit")
	}
}

// ParseResourceID extracts and validates a numeric ID from a URL path
// Returns the parsed ID and any validation error
func ParseResourceID(r *http.Request, pathPrefix string) (int64, error) {
	id := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if id == "" || strings.Contains(id, "/") {
		return 0, fmt.Errorf("invalid id")
	}

	idNum, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("id must be a number")
	}

	return idNum, nil
}

// SetCommonBaseFilters sets common base filter fields on any query struct with embedded CommonBaseFilters
func SetCommonBaseFilters(query interface{}, params *CommonQueryParams) {
	switch q := query.(type) {
	case *repository.ProfileQuery:
		q.CommonBaseFilters.ServiceName = params.ServiceName
		q.CommonBaseFilters.ServiceNames = params.ServiceNames
		q.CommonBaseFilters.ServiceNameLike = params.ServiceNameLike
		q.CommonBaseFilters.ServiceNameRegex = params.ServiceNameRegex
		q.CommonBaseFilters.ServiceNamePrefix = params.ServiceNamePrefix
		q.CommonBaseFilters.ServiceNameSuffix = params.ServiceNameSuffix
		q.CommonBaseFilters.ServiceNameContains = params.ServiceNameContains
		q.CommonBaseFilters.Limit = params.Limit
		q.CommonBaseFilters.Schema = params.Schema
		q.CommonBaseFilters.Direction = params.Direction
		q.CommonBaseFilters.DirectionResults = params.DirectionResults
		q.CommonBaseFilters.Cursor = params.Cursor
	case *repository.LogQuery:
		q.CommonBaseFilters.ServiceName = params.ServiceName
		q.CommonBaseFilters.ServiceNames = params.ServiceNames
		q.CommonBaseFilters.ServiceNameLike = params.ServiceNameLike
		q.CommonBaseFilters.ServiceNameRegex = params.ServiceNameRegex
		q.CommonBaseFilters.ServiceNamePrefix = params.ServiceNamePrefix
		q.CommonBaseFilters.ServiceNameSuffix = params.ServiceNameSuffix
		q.CommonBaseFilters.ServiceNameContains = params.ServiceNameContains
		q.CommonBaseFilters.Limit = params.Limit
		q.CommonBaseFilters.Schema = params.Schema
		q.CommonBaseFilters.Direction = params.Direction
		q.CommonBaseFilters.DirectionResults = params.DirectionResults
		q.CommonBaseFilters.Cursor = params.Cursor
	case *repository.MetricQuery:
		q.CommonBaseFilters.ServiceName = params.ServiceName
		q.CommonBaseFilters.ServiceNames = params.ServiceNames
		q.CommonBaseFilters.ServiceNameLike = params.ServiceNameLike
		q.CommonBaseFilters.ServiceNameRegex = params.ServiceNameRegex
		q.CommonBaseFilters.ServiceNamePrefix = params.ServiceNamePrefix
		q.CommonBaseFilters.ServiceNameSuffix = params.ServiceNameSuffix
		q.CommonBaseFilters.ServiceNameContains = params.ServiceNameContains
		q.CommonBaseFilters.Limit = params.Limit
		q.CommonBaseFilters.Schema = params.Schema
		q.CommonBaseFilters.Direction = params.Direction
		q.CommonBaseFilters.DirectionResults = params.DirectionResults
		q.CommonBaseFilters.Cursor = params.Cursor
	case *repository.TraceQuery:
		q.CommonBaseFilters.ServiceName = params.ServiceName
		q.CommonBaseFilters.ServiceNames = params.ServiceNames
		q.CommonBaseFilters.ServiceNameLike = params.ServiceNameLike
		q.CommonBaseFilters.ServiceNameRegex = params.ServiceNameRegex
		q.CommonBaseFilters.ServiceNamePrefix = params.ServiceNamePrefix
		q.CommonBaseFilters.ServiceNameSuffix = params.ServiceNameSuffix
		q.CommonBaseFilters.ServiceNameContains = params.ServiceNameContains
		q.CommonBaseFilters.Limit = params.Limit
		q.CommonBaseFilters.Schema = params.Schema
		q.CommonBaseFilters.Direction = params.Direction
		q.CommonBaseFilters.DirectionResults = params.DirectionResults
		q.CommonBaseFilters.Cursor = params.Cursor
	}
}

// SetCommonAdvancedFilters sets advanced filter fields on any query struct with embedded CommonAdvancedFilters
func SetCommonAdvancedFilters(query interface{}, r *http.Request) {
	qv := r.URL.Query()

	// Parse deepObject-style maps using existing helper functions
	attrEq := ExtractDeepObjectStringMap(qv, "attr.eq[")
	attrIn := ExtractDeepObjectCSVMap(qv, "attr.in[")
	attrExists := ExtractDeepObjectBoolMap(qv, "attr.exists[")
	resAttrEq := ExtractDeepObjectStringMap(qv, "res_attr.eq[")
	resAttrIn := ExtractDeepObjectCSVMap(qv, "res_attr.in[")
	resAttrExists := ExtractDeepObjectBoolMap(qv, "res_attr.exists[")

	switch q := query.(type) {
	case *repository.LogQuery:
		q.CommonAdvancedFilters.AttrEq = attrEq
		q.CommonAdvancedFilters.AttrIn = attrIn
		q.CommonAdvancedFilters.AttrExists = attrExists
		q.CommonAdvancedFilters.ResAttrEq = resAttrEq
		q.CommonAdvancedFilters.ResAttrIn = resAttrIn
		q.CommonAdvancedFilters.ResAttrExists = resAttrExists
	case *repository.MetricQuery:
		q.CommonAdvancedFilters.AttrEq = attrEq
		q.CommonAdvancedFilters.AttrIn = attrIn
		q.CommonAdvancedFilters.AttrExists = attrExists
		q.CommonAdvancedFilters.ResAttrEq = resAttrEq
		q.CommonAdvancedFilters.ResAttrIn = resAttrIn
		q.CommonAdvancedFilters.ResAttrExists = resAttrExists
	case *repository.TraceQuery:
		q.CommonAdvancedFilters.AttrEq = attrEq
		q.CommonAdvancedFilters.AttrIn = attrIn
		q.CommonAdvancedFilters.AttrExists = attrExists
		q.CommonAdvancedFilters.ResAttrEq = resAttrEq
		q.CommonAdvancedFilters.ResAttrIn = resAttrIn
		q.CommonAdvancedFilters.ResAttrExists = resAttrExists
	}
}

// CreateGetByIDHandler creates a common GetByID handler pattern
func CreateGetByIDHandler[T any](
	pathPrefix string,
	getByIDFunc func(ctx context.Context, id int64, schema string) (T, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idNum, err := ParseResourceID(r, pathPrefix)
		if err != nil {
			WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}

		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		entry, err := getByIDFunc(r.Context(), idNum, schema)
		if err != nil {
			WriteErrorResponse(w, http.StatusNotFound, "not found")
			return
		}

		WriteJSONResponse(w, http.StatusOK, entry)
	}
}
