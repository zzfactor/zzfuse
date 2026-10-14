package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/medxops/trazr-otel/pkg/util"
)

const (
	DefaultSchema = "otel"
)

// InsertResult is the response for successful inserts.
type InsertResult struct {
	IDs    []int64 `json:"ids"`
	Status string  `json:"status"`
}

// ErrorResponse is the response for errors.
type ErrorResponse struct {
	Error string `json:"error"`
}

// Pointer helpers for nullable fields - delegate to shared utilities
// PtrFromString returns nil for empty strings, otherwise a pointer to the string
func PtrFromString(s string) *string { return util.NonEmptyStringPtr(s) }

// PtrFromInt32 returns a pointer to the int32 value
func PtrFromInt32(i int32) *int32 { return util.Ptr(i) }

// PtrFromInt64 returns a pointer to the int64 value
func PtrFromInt64(i int64) *int64 { return util.Ptr(i) }

// PtrFromFloat64 returns a pointer to the float64 value
func PtrFromFloat64(f float64) *float64 { return util.Ptr(f) }

// DRY helpers for marshaling and parsing
func MarshalOrDefault(v interface{}, def []byte) []byte {
	if v == nil {
		return def
	}
	b, err := json.Marshal(v)
	if err != nil {
		return def
	}
	return b
}

// Cursor functions are now provided by pkg/repository/database_util.go
// Use ParseCursor and EncodeCursor from the repository package for consistency

// TimeRange represents a time window for queries.
type TimeRange struct {
	Start time.Time
	End   time.Time
}

// getSchemaParam returns the schema query parameter or a sane default
func getSchemaParam(r *http.Request) string {
	s := r.URL.Query().Get("schema")
	if s == "" {
		return DefaultSchema // Default schema when none specified
	}
	return s
}

func parseTimeOrNil(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}

func writeInsertResult(w http.ResponseWriter, ids []string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Convert to []int64 to reflect BIGSERIAL IDs in API responses
	converted := make([]int64, 0, len(ids))
	for _, s := range ids {
		if s == "" {
			continue
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(ErrorResponse{Error: fmt.Sprintf("invalid numeric id: %s", s)})
			return
		}
		converted = append(converted, v)
	}
	_ = json.NewEncoder(w).Encode(InsertResult{IDs: converted, Status: "inserted"})
}

func writeInsertError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: msg})
}
func upperSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, strings.ToUpper(s))
	}
	return out
}

// Helpers to parse deepObject maps from query keys like "attr.eq[key]"
func ExtractDeepObjectStringMap(qs url.Values, prefix string) map[string]string {
	m := make(map[string]string)
	for k, vals := range qs {
		if strings.HasPrefix(k, prefix) && strings.HasSuffix(k, "]") {
			key := k[len(prefix) : len(k)-1]
			if key == "" || len(vals) == 0 {
				continue
			}
			m[key] = vals[0]
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func ExtractDeepObjectCSVMap(qs url.Values, prefix string) map[string][]string {
	m := make(map[string][]string)
	for k, vals := range qs {
		if strings.HasPrefix(k, prefix) && strings.HasSuffix(k, "]") {
			key := k[len(prefix) : len(k)-1]
			if key == "" || len(vals) == 0 {
				continue
			}
			parts := strings.Split(vals[0], ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, p)
				}
			}
			if len(out) > 0 {
				m[key] = out
			}
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func ExtractDeepObjectBoolMap(qs url.Values, prefix string) map[string]bool {
	m := make(map[string]bool)
	for k, vals := range qs {
		if strings.HasPrefix(k, prefix) && strings.HasSuffix(k, "]") {
			key := k[len(prefix) : len(k)-1]
			if key == "" || len(vals) == 0 {
				continue
			}
			m[key] = strings.EqualFold(vals[0], "true")
		}
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// parseBool returns true if s parses as a true boolean (true, 1, t, etc.)
func parseBool(s string) bool {
	b, err := strconv.ParseBool(s)
	return err == nil && b
}

// Validation constants
const (
	MaxLimit     = 1000
	MaxOffset    = 100000
	MaxTimeRange = 30 * 24 * time.Hour // 30 days
)

// ValidationError represents a validation error with a specific field and message
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors represents multiple validation errors
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

// AddError adds a validation error to the collection
func (ve *ValidationErrors) AddError(field, message string) {
	ve.Errors = append(ve.Errors, ValidationError{Field: field, Message: message})
}

// HasErrors returns true if there are validation errors
func (ve *ValidationErrors) HasErrors() bool {
	return len(ve.Errors) > 0
}

// ValidateTimeRange validates that start time is before end time and within reasonable bounds
func ValidateTimeRange(start, end *time.Time) *ValidationErrors {
	errors := &ValidationErrors{}

	if start != nil && end != nil {
		if start.After(*end) {
			errors.AddError("time_range", "start time must be before end time")
		}

		duration := end.Sub(*start)
		if duration > MaxTimeRange {
			errors.AddError("time_range", fmt.Sprintf("time range cannot exceed %v", MaxTimeRange))
		}
	}

	return errors
}

// ValidateLimit validates and normalizes the limit parameter
func ValidateLimit(limit int) (int, *ValidationErrors) {
	errors := &ValidationErrors{}

	if limit <= 0 {
		errors.AddError("limit", "limit must be greater than 0")
		return 100, errors
	}

	if limit > MaxLimit {
		errors.AddError("limit", fmt.Sprintf("limit cannot exceed %d", MaxLimit))
		return MaxLimit, errors
	}

	return limit, errors
}

// ValidateOffset validates the offset parameter
func ValidateOffset(offset int) *ValidationErrors {
	errors := &ValidationErrors{}

	if offset < 0 {
		errors.AddError("offset", "offset cannot be negative")
	}

	if offset > MaxOffset {
		errors.AddError("offset", fmt.Sprintf("offset cannot exceed %d", MaxOffset))
	}

	return errors
}

// ParseAndValidateLimit parses limit from string and validates it
func ParseAndValidateLimit(limitStr string, defaultLimit int) (int, *ValidationErrors) {
	errors := &ValidationErrors{}

	if limitStr == "" {
		return defaultLimit, errors
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		errors.AddError("limit", "limit must be a valid number")
		return defaultLimit, errors
	}

	validatedLimit, limitErrors := ValidateLimit(limit)
	if limitErrors.HasErrors() {
		errors.Errors = append(errors.Errors, limitErrors.Errors...)
	}

	return validatedLimit, errors
}

// ParseAndValidateOffset parses offset from string and validates it
func ParseAndValidateOffset(offsetStr string) (int, *ValidationErrors) {
	errors := &ValidationErrors{}

	if offsetStr == "" {
		return 0, errors
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		errors.AddError("offset", "offset must be a valid number")
		return 0, errors
	}

	offsetErrors := ValidateOffset(offset)
	if offsetErrors.HasErrors() {
		errors.Errors = append(errors.Errors, offsetErrors.Errors...)
	}

	return offset, errors
}

// WriteValidationErrors writes validation errors to the HTTP response
func WriteValidationErrors(w http.ResponseWriter, errors *ValidationErrors) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(errors)
}

// WriteErrorResponse writes a standardized error response
func WriteErrorResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

// WriteJSONResponse writes a JSON response with proper headers
func WriteJSONResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// SanitizeDatabaseError sanitizes database errors to hide internal details from users
func SanitizeDatabaseError(err error) string {
	if err == nil {
		return "internal server error"
	}

	errStr := err.Error()

	// Common database error patterns to sanitize
	if strings.Contains(errStr, "relation") && strings.Contains(errStr, "does not exist") {
		return "requested data source is not available"
	}
	if strings.Contains(errStr, "SQLSTATE") {
		return "database query failed"
	}
	if strings.Contains(errStr, "connection") && (strings.Contains(errStr, "refused") || strings.Contains(errStr, "timeout")) {
		return "database connection unavailable"
	}
	if strings.Contains(errStr, "syntax error") {
		return "invalid query parameters"
	}
	if strings.Contains(errStr, "permission denied") {
		return "access denied"
	}

	// For any other database errors, return a generic message
	return "query execution failed"
}

// ParseTimeWithValidation parses time string and returns validation errors if any
func ParseTimeWithValidation(timeStr, fieldName string) (*time.Time, *ValidationErrors) {
	errors := &ValidationErrors{}

	if timeStr == "" {
		return nil, errors
	}

	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		errors.AddError(fieldName, fmt.Sprintf("invalid %s time format, expected RFC3339", fieldName))
		return nil, errors
	}

	return &t, errors
}

// ValidateRequiredField checks if a required field is present
func ValidateRequiredField(value, fieldName string) *ValidationErrors {
	errors := &ValidationErrors{}

	if strings.TrimSpace(value) == "" {
		errors.AddError(fieldName, fmt.Sprintf("%s is required", fieldName))
	}

	return errors
}

// ValidateEnumField checks if a field value is in the allowed enum values
func ValidateEnumField(value, fieldName string, allowedValues []string) *ValidationErrors {
	errors := &ValidationErrors{}

	if value == "" {
		return errors
	}

	found := false
	for _, allowed := range allowedValues {
		if strings.EqualFold(value, allowed) {
			found = true
			break
		}
	}

	if !found {
		errors.AddError(fieldName, fmt.Sprintf("%s must be one of: %s", fieldName, strings.Join(allowedValues, ", ")))
	}

	return errors
}
