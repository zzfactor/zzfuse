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

	"github.com/medxops/trazr-otel/pkg/repository"
)

// MetricEntryDTO is a data transfer object for incoming metric entries
// with plain Go types for easy JSON unmarshalling.
type MetricEntryDTO struct {
	Timestamp              string                 `json:"timestamp"`
	ServiceName            string                 `json:"service_name"`
	Name                   string                 `json:"name"`
	Description            string                 `json:"description,omitempty"`
	Unit                   string                 `json:"unit,omitempty"`
	Attributes             map[string]interface{} `json:"attributes,omitempty"`
	Value                  float64                `json:"value,omitempty"`
	Count                  int64                  `json:"count,omitempty"`
	Sum                    float64                `json:"sum,omitempty"`
	Buckets                interface{}            `json:"buckets,omitempty"`
	Quantiles              interface{}            `json:"quantiles,omitempty"`
	Flags                  int                    `json:"flags,omitempty"`
	AggregationTemporality int                    `json:"aggregation_temporality,omitempty"`
	Exemplars              interface{}            `json:"exemplars,omitempty"`
	ResourceAttributes     map[string]interface{} `json:"resource_attributes,omitempty"`
	ScopeName              string                 `json:"scope_name,omitempty"`
	ScopeVersion           string                 `json:"scope_version,omitempty"`
	ScopeAttributes        map[string]interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount  int                    `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL         string                 `json:"scope_schema_url,omitempty"`
	StartTime              string                 `json:"start_time,omitempty"`
	IsMonotonic            bool                   `json:"is_monotonic,omitempty"`
	Metadata               map[string]interface{} `json:"metadata,omitempty"`
	// Exponential Histogram specific fields
	Scale                int32       `json:"scale,omitempty"`
	ZeroCount            int64       `json:"zero_count,omitempty"`
	PositiveOffset       int32       `json:"positive_offset,omitempty"`
	NegativeOffset       int32       `json:"negative_offset,omitempty"`
	Min                  float64     `json:"min,omitempty"`
	Max                  float64     `json:"max,omitempty"`
	PositiveBucketCounts interface{} `json:"positive_bucket_counts,omitempty"`
	NegativeBucketCounts interface{} `json:"negative_bucket_counts,omitempty"`
}

// MetricQuery defines filters for metric queryueries
type MetricQuery struct {
	TimeRange
	ServiceName string
	MetricType  string
	Limit       int
	Schema      string
	Cursor      string // for cursor-based pagination
}

// Helper for wildcard filtering for metrics
type metricFilterParams struct {
	ServiceName string
	MetricName  string
}

func NewMetricGetByIDHandler(service repository.MetricQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/metrics/")
		path = strings.Trim(path, "/")
		parts := strings.Split(path, "/")
		schema := getSchemaParam(r)
		if !requireSchemaAccess(w, r, schema) {
			return
		}
		if len(parts) == 1 {
			idNum, err := ParseResourceID(r, "/api/v1/metrics/")
			if err != nil {
				WriteErrorResponse(w, http.StatusBadRequest, err.Error())
				return
			}
			entry, err := service.GetMetricByID(r.Context(), idNum, schema)
			if err != nil {
				WriteErrorResponse(w, http.StatusNotFound, "not found")
				return
			}
			WriteJSONResponse(w, http.StatusOK, entry)
			return
		}
		if len(parts) == 2 {
			metricType := parts[0]
			id := parts[1]
			if id == "" || metricType == "" || strings.Contains(id, "/") {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid type or id"})
				return
			}
			// Validate numeric id (BIGSERIAL)
			idNum, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "id must be a number"})
				return
			}
			_ = idNum // validated
			entry, err := service.GetMetricByIDTyped(r.Context(), metricType, idNum, schema)
			if err != nil {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(entry)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid path"})
	}
}

// Add mapping functions for each metric type

func (dto MetricEntryDTO) ToGaugeMetricEntry() (repository.GaugeMetricEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repository.GaugeMetricEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	return repository.GaugeMetricEntry{
		ID:                    0, // Use 0 for int64 ID field
		TimestampNS:           t.UnixNano(),
		Timestamp:             t,
		ServiceName:           dto.ServiceName,
		Name:                  PtrFromString(dto.Name),
		Description:           PtrFromString(dto.Description),
		Unit:                  PtrFromString(dto.Unit),
		Attributes:            MarshalOrDefault(dto.Attributes, []byte("{}")),
		Value:                 PtrFromFloat64(dto.Value),
		Flags:                 PtrFromInt32(int32(dto.Flags)),
		Exemplars:             MarshalOrDefault(dto.Exemplars, []byte("null")),
		ResourceAttributes:    MarshalOrDefault(dto.ResourceAttributes, []byte("{}")),
		ScopeName:             PtrFromString(dto.ScopeName),
		ScopeVersion:          PtrFromString(dto.ScopeVersion),
		ScopeAttributes:       MarshalOrDefault(dto.ScopeAttributes, []byte("{}")),
		ScopeDroppedAttrCount: PtrFromInt32(int32(dto.ScopeDroppedAttrCount)),
		Metadata:              MarshalOrDefault(dto.Metadata, []byte("{}")),
		ResourceSchemaURL:     PtrFromString(dto.ScopeSchemaURL),
		ScopeSchemaURL:        PtrFromString(dto.ScopeSchemaURL),
	}, nil
}

func (dto MetricEntryDTO) ToSumMetricEntry() (repository.SumMetricEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repository.SumMetricEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	return repository.SumMetricEntry{
		ID:                     0, // Use 0 for int64 ID field
		TimestampNS:            t.UnixNano(),
		Timestamp:              t,
		ServiceName:            dto.ServiceName,
		ResourceAttributes:     MarshalOrDefault(dto.ResourceAttributes, []byte("{}")),
		ScopeName:              PtrFromString(dto.ScopeName),
		ScopeVersion:           PtrFromString(dto.ScopeVersion),
		ScopeAttributes:        MarshalOrDefault(dto.ScopeAttributes, []byte("{}")),
		ScopeDroppedAttrCount:  PtrFromInt32(int32(dto.ScopeDroppedAttrCount)),
		Name:                   PtrFromString(dto.Name),
		Description:            PtrFromString(dto.Description),
		Unit:                   PtrFromString(dto.Unit),
		Attributes:             MarshalOrDefault(dto.Attributes, []byte("{}")),
		StartTime:              parseTimeOrNil(dto.StartTime),
		Value:                  PtrFromFloat64(dto.Value),
		Flags:                  PtrFromInt32(int32(dto.Flags)),
		Exemplars:              MarshalOrDefault(dto.Exemplars, []byte("null")),
		AggregationTemporality: PtrFromInt32(int32(dto.AggregationTemporality)),
		IsMonotonic:            &dto.IsMonotonic,
		Metadata:               MarshalOrDefault(dto.Metadata, []byte("{}")),
		ResourceSchemaURL:      PtrFromString(dto.ScopeSchemaURL),
		ScopeSchemaURL:         PtrFromString(dto.ScopeSchemaURL),
	}, nil
}

func (dto MetricEntryDTO) ToHistogramMetricEntry() (repository.HistogramMetricEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repository.HistogramMetricEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	return repository.HistogramMetricEntry{
		ID:                     0, // Use 0 for int64 ID field
		TimestampNS:            t.UnixNano(),
		Timestamp:              t,
		ServiceName:            dto.ServiceName,
		Name:                   PtrFromString(dto.Name),
		Description:            PtrFromString(dto.Description),
		Unit:                   PtrFromString(dto.Unit),
		Attributes:             MarshalOrDefault(dto.Attributes, []byte("{}")),
		StartTime:              parseTimeOrNil(dto.StartTime),
		Count:                  PtrFromInt64(dto.Count),
		Sum:                    PtrFromFloat64(dto.Sum),
		Buckets:                MarshalOrDefault(dto.Buckets, []byte("null")),
		Flags:                  PtrFromInt32(int32(dto.Flags)),
		AggregationTemporality: PtrFromInt32(int32(dto.AggregationTemporality)),
		Exemplars:              MarshalOrDefault(dto.Exemplars, []byte("null")),
		ResourceAttributes:     MarshalOrDefault(dto.ResourceAttributes, []byte("{}")),
		ScopeName:              PtrFromString(dto.ScopeName),
		ScopeVersion:           PtrFromString(dto.ScopeVersion),
		ScopeAttributes:        MarshalOrDefault(dto.ScopeAttributes, []byte("{}")),
		ScopeDroppedAttrCount:  PtrFromInt32(int32(dto.ScopeDroppedAttrCount)),
		Metadata:               MarshalOrDefault(dto.Metadata, []byte("{}")),
		ResourceSchemaURL:      PtrFromString(dto.ScopeSchemaURL),
		ScopeSchemaURL:         PtrFromString(dto.ScopeSchemaURL),
	}, nil
}

func (dto MetricEntryDTO) ToSummaryMetricEntry() (repository.SummaryMetricEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repository.SummaryMetricEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	return repository.SummaryMetricEntry{
		ID:                    0, // Use 0 for int64 ID field
		TimestampNS:           t.UnixNano(),
		Timestamp:             t,
		ServiceName:           dto.ServiceName,
		Name:                  PtrFromString(dto.Name),
		Description:           PtrFromString(dto.Description),
		Unit:                  PtrFromString(dto.Unit),
		Attributes:            MarshalOrDefault(dto.Attributes, []byte("{}")),
		StartTime:             parseTimeOrNil(dto.StartTime),
		Count:                 PtrFromInt64(dto.Count),
		Sum:                   PtrFromFloat64(dto.Sum),
		Quantiles:             MarshalOrDefault(dto.Quantiles, []byte("null")),
		Flags:                 PtrFromInt32(int32(dto.Flags)),
		Exemplars:             MarshalOrDefault(dto.Exemplars, []byte("null")),
		ResourceAttributes:    MarshalOrDefault(dto.ResourceAttributes, []byte("{}")),
		ScopeName:             PtrFromString(dto.ScopeName),
		ScopeVersion:          PtrFromString(dto.ScopeVersion),
		ScopeAttributes:       MarshalOrDefault(dto.ScopeAttributes, []byte("{}")),
		ScopeDroppedAttrCount: PtrFromInt32(int32(dto.ScopeDroppedAttrCount)),
		Metadata:              MarshalOrDefault(dto.Metadata, []byte("{}")),
		ResourceSchemaURL:     PtrFromString(dto.ScopeSchemaURL),
		ScopeSchemaURL:        PtrFromString(dto.ScopeSchemaURL),
	}, nil
}

func (dto MetricEntryDTO) ToExponentialHistogramMetricEntry() (repository.ExponentialHistogramMetricEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repository.ExponentialHistogramMetricEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	return repository.ExponentialHistogramMetricEntry{
		ID:                     0, // Use 0 for int64 ID field
		TimestampNS:            t.UnixNano(),
		Timestamp:              t,
		ServiceName:            dto.ServiceName,
		ResourceAttributes:     MarshalOrDefault(dto.ResourceAttributes, []byte("{}")),
		ScopeName:              PtrFromString(dto.ScopeName),
		ScopeVersion:           PtrFromString(dto.ScopeVersion),
		ScopeAttributes:        MarshalOrDefault(dto.ScopeAttributes, []byte("{}")),
		ScopeDroppedAttrCount:  PtrFromInt32(int32(dto.ScopeDroppedAttrCount)),
		Name:                   PtrFromString(dto.Name),
		Description:            PtrFromString(dto.Description),
		Unit:                   PtrFromString(dto.Unit),
		Attributes:             MarshalOrDefault(dto.Attributes, []byte("{}")),
		StartTime:              parseTimeOrNil(dto.StartTime),
		Count:                  PtrFromInt64(dto.Count),
		Sum:                    PtrFromFloat64(dto.Sum),
		Scale:                  PtrFromInt32(dto.Scale),
		ZeroCount:              PtrFromInt64(dto.ZeroCount),
		PositiveOffset:         PtrFromInt32(dto.PositiveOffset),
		PositiveBucketCounts:   MarshalOrDefault(dto.PositiveBucketCounts, []byte("null")),
		NegativeOffset:         PtrFromInt32(dto.NegativeOffset),
		NegativeBucketCounts:   MarshalOrDefault(dto.NegativeBucketCounts, []byte("null")),
		Exemplars:              MarshalOrDefault(dto.Exemplars, []byte("null")),
		Flags:                  PtrFromInt32(int32(dto.Flags)),
		Min:                    PtrFromFloat64(dto.Min),
		Max:                    PtrFromFloat64(dto.Max),
		AggregationTemporality: PtrFromInt32(int32(dto.AggregationTemporality)),
		Metadata:               MarshalOrDefault(dto.Metadata, []byte("{}")),
		ResourceSchemaURL:      PtrFromString(dto.ScopeSchemaURL),
		ScopeSchemaURL:         PtrFromString(dto.ScopeSchemaURL),
	}, nil
}

// NewMetricInsertHandler returns an http.HandlerFunc for POST /api/v1/metrics
func NewMetricInsertHandler(metricWriter repository.MetricWriterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := r.URL.Query().Get("schema")
		if schema == "" {
			schema = DefaultSchema
		}
		if !requireSchemaAccess(w, r, schema) {
			return
		}

		metricType := r.URL.Query().Get("type")
		if metricType == "" {
			writeInsertError(w, http.StatusBadRequest, "metric type is required as a query parameter")
			return
		}
		var dtos []MetricEntryDTO
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeInsertError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		if err := json.Unmarshal(body, &dtos); err != nil {
			// Try single object
			var singleDTO MetricEntryDTO
			if err2 := json.Unmarshal(body, &singleDTO); err2 != nil {
				writeInsertError(w, http.StatusBadRequest, "invalid metric entry format")
				return
			}
			dtos = []MetricEntryDTO{singleDTO}
		}
		if len(dtos) == 0 {
			writeInsertError(w, http.StatusBadRequest, "no metric entries provided")
			return
		}
		var ids []string
		switch metricType {
		case "gauge":
			var entries []repository.GaugeMetricEntry
			for _, dto := range dtos {
				entry, err := dto.ToGaugeMetricEntry()
				if err != nil {
					writeInsertError(w, http.StatusBadRequest, err.Error())
					return
				}
				entries = append(entries, entry)
			}
			ids, err = metricWriter.InsertGaugeMetrics(r.Context(), entries, schema)
		case "sum":
			var entries []repository.SumMetricEntry
			for _, dto := range dtos {
				entry, err := dto.ToSumMetricEntry()
				if err != nil {
					writeInsertError(w, http.StatusBadRequest, err.Error())
					return
				}
				entries = append(entries, entry)
			}
			ids, err = metricWriter.InsertSumMetrics(r.Context(), entries, schema)
		case "histogram":
			var entries []repository.HistogramMetricEntry
			for _, dto := range dtos {
				entry, err := dto.ToHistogramMetricEntry()
				if err != nil {
					writeInsertError(w, http.StatusBadRequest, err.Error())
					return
				}
				entries = append(entries, entry)
			}
			ids, err = metricWriter.InsertHistogramMetrics(r.Context(), entries, schema)
		case "summary":
			var entries []repository.SummaryMetricEntry
			for _, dto := range dtos {
				entry, err := dto.ToSummaryMetricEntry()
				if err != nil {
					writeInsertError(w, http.StatusBadRequest, err.Error())
					return
				}
				entries = append(entries, entry)
			}
			ids, err = metricWriter.InsertSummaryMetrics(r.Context(), entries, schema)
		case "exponential_histogram":
			var entries []repository.ExponentialHistogramMetricEntry
			for _, dto := range dtos {
				entry, err := dto.ToExponentialHistogramMetricEntry()
				if err != nil {
					writeInsertError(w, http.StatusBadRequest, err.Error())
					return
				}
				entries = append(entries, entry)
			}
			ids, err = metricWriter.InsertExponentialHistogramMetrics(r.Context(), entries, schema)
		default:
			writeInsertError(w, http.StatusBadRequest, "unsupported metric type")
			return
		}
		if err != nil {
			writeInsertError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeInsertResult(w, ids)
	}
}

// MetricQueryV1Handler returns an http.HandlerFunc for GET /api/v1/metrics
// Supported types: gauge, sum, histogram, summary, exponential_histogram (via ?type=...)
func NewMetricQueryV1Handler(service repository.MetricQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Collect all validation errors
		validationErrors := &ValidationErrors{}

		// Parse common query parameters using DRY helper
		commonParams := ParseCommonQueryParams(r, 100)
		if !requireSchemaAccess(w, r, commonParams.Schema) {
			return
		}

		// Extract metrics-specific parameters
		metricName := r.URL.Query().Get("metric")
		metricType := r.URL.Query().Get("type")

		// Use common parameters
		validationErrors = commonParams.ValidationErrors
		start := commonParams.Start
		end := commonParams.End
		serviceName := commonParams.ServiceName
		limit := commonParams.Limit
		direction := commonParams.Direction
		direction_results := commonParams.DirectionResults
		schema := commonParams.Schema

		// Validate required metric type
		if metricType == "" {
			validationErrors.AddError("type", "metric type is required (gauge, sum, histogram, summary, exponential_histogram)")
		}

		// Check for validation errors before proceeding
		if validationErrors.HasErrors() {
			WriteValidationErrors(w, validationErrors)
			return
		}
		query := repository.MetricQuery{
			TimeRange:  repository.TimeRange{Start: start, End: end},
			MetricName: metricName,
		}
		// Set embedded CommonBaseFilters fields
		query.CommonBaseFilters.ServiceName = serviceName
		query.CommonBaseFilters.Limit = limit
		query.CommonBaseFilters.Schema = schema
		query.CommonBaseFilters.Direction = direction
		query.CommonBaseFilters.DirectionResults = direction_results
		// Handle count-only mode similar to traces handler
		countOnly := commonParams.CountOnly
		if countOnly {
			switch metricType {
			case "gauge":
				if counter, ok := any(service).(interface {
					CountMetricsGauge(context.Context, repository.MetricQuery) (int64, error)
				}); ok {
					cnt, err := counter.CountMetricsGauge(ctx, query)
					if err != nil {
						WriteErrorResponse(w, http.StatusInternalServerError, err.Error())
						return
					}
					WriteJSONResponse(w, http.StatusOK, map[string]int64{"count": cnt})
					return
				}
			case "sum":
				if counter, ok := any(service).(interface {
					CountMetricsSum(context.Context, repository.MetricQuery) (int64, error)
				}); ok {
					cnt, err := counter.CountMetricsSum(ctx, query)
					if err != nil {
						sanitizedError := SanitizeDatabaseError(err)
						WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
						return
					}
					WriteJSONResponse(w, http.StatusOK, map[string]int64{"count": cnt})
					return
				}
			case "histogram":
				if counter, ok := any(service).(interface {
					CountMetricsHistogram(context.Context, repository.MetricQuery) (int64, error)
				}); ok {
					cnt, err := counter.CountMetricsHistogram(ctx, query)
					if err != nil {
						sanitizedError := SanitizeDatabaseError(err)
						WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
						return
					}
					WriteJSONResponse(w, http.StatusOK, map[string]int64{"count": cnt})
					return
				}
			case "summary":
				if counter, ok := any(service).(interface {
					CountMetricsSummary(context.Context, repository.MetricQuery) (int64, error)
				}); ok {
					cnt, err := counter.CountMetricsSummary(ctx, query)
					if err != nil {
						sanitizedError := SanitizeDatabaseError(err)
						WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
						return
					}
					WriteJSONResponse(w, http.StatusOK, map[string]int64{"count": cnt})
					return
				}
			case "exponential_histogram":
				if counter, ok := any(service).(interface {
					CountMetricsExponentialHistogram(context.Context, repository.MetricQuery) (int64, error)
				}); ok {
					cnt, err := counter.CountMetricsExponentialHistogram(ctx, query)
					if err != nil {
						sanitizedError := SanitizeDatabaseError(err)
						WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
						return
					}
					WriteJSONResponse(w, http.StatusOK, map[string]int64{"count": cnt})
					return
				}
			}
			WriteErrorResponse(w, http.StatusNotImplemented, "count_only not supported by this service")
			return
		}

		var err error // <-- fix: declare err here
		var result repository.MetricQueryResult
		switch metricType {
		case "gauge":
			result, err = service.QueryMetricsGauge(ctx, query)
		case "sum":
			result, err = service.QueryMetricsSum(ctx, query)
		case "histogram":
			result, err = service.QueryMetricsHistogram(ctx, query)
		case "summary":
			result, err = service.QueryMetricsSummary(ctx, query)
		case "exponential_histogram":
			result, err = service.QueryMetricsExponentialHistogram(ctx, query)
		default:
			WriteErrorResponse(w, http.StatusBadRequest, "unsupported metric type")
			return
		}
		if err != nil {
			sanitizedError := SanitizeDatabaseError(err)
			WriteErrorResponse(w, http.StatusInternalServerError, sanitizedError)
			return
		}
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

// NewMetricCursorGenerationHandler creates a new handler for generating cursors for metrics
func NewMetricCursorGenerationHandler(service repository.MetricQueryService) http.HandlerFunc {
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

		filters := MetricQuery{
			TimeRange:   timeRange,
			ServiceName: r.URL.Query().Get("service"),
			MetricType:  r.URL.Query().Get("type"),
			Schema:      schema,
		}

		// Convert internal MetricQuery to repository MetricQuery
		var repoStartTime, repoEndTime *time.Time
		if !filters.TimeRange.Start.IsZero() {
			repoStartTime = &filters.TimeRange.Start
		}
		if !filters.TimeRange.End.IsZero() {
			repoEndTime = &filters.TimeRange.End
		}

		repoQuery := repository.MetricQuery{
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
			if strings.Contains(err.Error(), "no metrics found") {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Return success response using existing MetricQueryResult
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(result)
	}
}
