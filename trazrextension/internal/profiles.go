package internal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"time"

	repo "github.com/medxops/trazr-otel/pkg/repository"
)

// [BETA] InsertResult is the response for successful profile inserts.
type ProfileInsertResult struct {
	IDs    []int64 `json:"ids"`
	Status string  `json:"status"`
}

// [BETA] ErrorResponse is the response for profile insert/query errors.
type ProfileErrorResponse struct {
	Error string `json:"error"`
}

// ProfileEntryDTO is a data transfer object for incoming profile entries
// with plain Go types for easy JSON unmarshalling.
type ProfileEntryDTO struct {
	ProfileStorageVersion int16                  `json:"profile_storage_version,omitempty"`
	ProfileEncoding       string                 `json:"profile_encoding,omitempty"`
	ProfileSchemaVersion  *string                `json:"profile_schema_version,omitempty"`
	ServiceNamespace      string                 `json:"service_namespace,omitempty"`
	ServiceInstanceID     string                 `json:"service_instance_id,omitempty"`
	Timestamp             string                 `json:"timestamp"`
	ServiceName           string                 `json:"service_name"`
	ProfileType           string                 `json:"profile_type"`
	DurationNS            int64                  `json:"duration_ns,omitempty"`
	SampleType            string                 `json:"sample_type,omitempty"`
	SampleUnit            string                 `json:"sample_unit,omitempty"`
	ProfileBytes          string                 `json:"profile_bytes"` // base64 encoded
	Attributes            map[string]interface{} `json:"attributes,omitempty"`
	ResourceAttributes    map[string]interface{} `json:"resource_attributes,omitempty"`
	ResourceSchemaURL     string                 `json:"resource_schema_url,omitempty"`
	ScopeName             string                 `json:"scope_name,omitempty"`
	ScopeVersion          string                 `json:"scope_version,omitempty"`
	ScopeAttributes       map[string]interface{} `json:"scope_attributes,omitempty"`
	ScopeDroppedAttrCount int32                  `json:"scope_dropped_attr_count,omitempty"`
	ScopeSchemaURL        string                 `json:"scope_schema_url,omitempty"`
	Metadata              map[string]interface{} `json:"metadata,omitempty"`
}

// ToProfileEntry converts ProfileEntryDTO to repository.ProfileEntry
func (dto ProfileEntryDTO) ToProfileEntry() (repo.ProfileEntry, error) {
	t, err := time.Parse(time.RFC3339, dto.Timestamp)
	if err != nil {
		return repo.ProfileEntry{}, fmt.Errorf("invalid timestamp: %w", err)
	}
	if t.Before(time.Unix(0, math.MinInt64)) || t.After(time.Unix(0, math.MaxInt64)) {
		return repo.ProfileEntry{}, fmt.Errorf("timestamp outside signed nanosecond range")
	}
	attributesJSON := MarshalOrDefault(dto.Attributes, []byte("{}"))
	resourceAttrsJSON := MarshalOrDefault(dto.ResourceAttributes, []byte("{}"))
	scopeAttrsJSON := MarshalOrDefault(dto.ScopeAttributes, []byte("{}"))
	metadataJSON := MarshalOrDefault(dto.Metadata, []byte("{}"))
	profileBytes := []byte{}
	if dto.ProfileBytes != "" {
		var decodeErr error
		profileBytes, decodeErr = base64.StdEncoding.DecodeString(dto.ProfileBytes)
		if decodeErr != nil {
			return repo.ProfileEntry{}, fmt.Errorf("invalid base64 ProfileBytes: %w", decodeErr)
		}
	}
	declared := repo.ProfileEntry{ProfileBytes: profileBytes, ProfileStorageVersion: dto.ProfileStorageVersion, ProfileEncoding: dto.ProfileEncoding, ProfileSchemaVersion: dto.ProfileSchemaVersion}
	if err := repo.ValidateProfileEntry(declared); err != nil {
		return repo.ProfileEntry{}, err
	}
	if dto.ProfileStorageVersion != 0 {
		pd, err := repo.DecodeStoredProfile(declared)
		if err != nil {
			return repo.ProfileEntry{}, err
		}
		entry, err := repo.ProfileEntryFromProfiles(pd, t)
		if err != nil {
			return repo.ProfileEntry{}, err
		}
		// Retain the exact accepted wire bytes; searchable summaries derive from it.
		entry.ProfileBytes = profileBytes
		entry.Metadata = metadataJSON
		return entry, nil
	}
	return repo.ProfileEntry{
		ID:               0,
		TimestampNS:      t.UnixNano(),
		Timestamp:        t,
		ServiceName:      dto.ServiceName,
		ServiceNamespace: PtrFromString(dto.ServiceNamespace), ServiceInstanceID: PtrFromString(dto.ServiceInstanceID), ProfileEncoding: repo.ProfileEncodingLegacy,
		ProfileType:           dto.ProfileType,
		DurationNS:            dto.DurationNS,
		SampleType:            PtrFromString(dto.SampleType),
		SampleUnit:            PtrFromString(dto.SampleUnit),
		ProfileBytes:          profileBytes,
		Attributes:            attributesJSON,
		ResourceAttributes:    resourceAttrsJSON,
		ResourceSchemaURL:     PtrFromString(dto.ResourceSchemaURL),
		ScopeName:             PtrFromString(dto.ScopeName),
		ScopeVersion:          PtrFromString(dto.ScopeVersion),
		ScopeAttributes:       scopeAttrsJSON,
		ScopeDroppedAttrCount: PtrFromInt32(dto.ScopeDroppedAttrCount),
		ScopeSchemaURL:        PtrFromString(dto.ScopeSchemaURL),
		Metadata:              metadataJSON,
	}, nil
}

// [BETA] NewProfileInsertHandler returns an http.HandlerFunc for POST /api/v1/profiles
func NewProfileInsertHandler(profileWriter repo.ProfileWriterService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		schema := r.URL.Query().Get("schema")
		if schema == "" {
			schema = DefaultSchema
		}
		if !requireSchemaAccess(w, r, schema) {
			return
		}

		var dtos []ProfileEntryDTO
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeProfileError(w, http.StatusBadRequest, "failed to read body")
			return
		}
		if err := json.Unmarshal(body, &dtos); err != nil {
			// Try single object
			var singleDTO ProfileEntryDTO
			if err2 := json.Unmarshal(body, &singleDTO); err2 != nil {
				writeProfileError(w, http.StatusBadRequest, "invalid profile entry format")
				return
			}
			dtos = []ProfileEntryDTO{singleDTO}
		}
		if len(dtos) == 0 {
			writeProfileError(w, http.StatusBadRequest, "no profile entries provided")
			return
		}
		var entries []repo.ProfileEntry
		for _, dto := range dtos {
			entry, err := dto.ToProfileEntry()
			if err != nil {
				writeProfileError(w, http.StatusBadRequest, err.Error())
				return
			}
			entries = append(entries, entry)
		}
		ids, err := profileWriter.InsertProfiles(r.Context(), entries, schema)
		if err != nil {
			writeProfileError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeProfileInsertResult(w, ids)
	}
}

// [BETA] NewProfileQueryHandler creates a new handler for querying profiles
func NewProfileQueryHandler(profileQuery repo.ProfileQueryService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Use DRY helper for common query parameter parsing
		commonParams := ParseCommonQueryParams(r, 100)
		if !requireSchemaAccess(w, r, commonParams.Schema) {
			return
		}

		// Check for validation errors before proceeding
		if commonParams.ValidationErrors.HasErrors() {
			WriteValidationErrors(w, commonParams.ValidationErrors)
			return
		}

		// Parse profile-specific parameters
		qv := r.URL.Query()
		profileType := qv.Get("profile_type")

		// Create query object with time range and profile-specific filters
		query := repo.ProfileQuery{
			TimeRange:   repo.TimeRange{Start: commonParams.Start, End: commonParams.End},
			ProfileType: profileType,
		}

		// Set embedded CommonBaseFilters using DRY helper
		SetCommonBaseFilters(&query, commonParams)

		// Set embedded CommonAdvancedFilters using DRY helper
		SetCommonAdvancedFilters(&query, r)

		result, err := profileQuery.QueryProfiles(r.Context(), query)
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

func writeProfileInsertResult(w http.ResponseWriter, ids []string) {
	converted := make([]int64, 0, len(ids))
	for _, s := range ids {
		if s == "" {
			continue
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			WriteErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("invalid numeric id: %s", s))
			return
		}
		converted = append(converted, v)
	}
	WriteJSONResponse(w, http.StatusOK, ProfileInsertResult{IDs: converted, Status: "inserted"})
}

func writeProfileError(w http.ResponseWriter, status int, msg string) {
	WriteErrorResponse(w, status, msg)
}
