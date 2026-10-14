// Package trazrexporter provides typed errors for common failure cases in the PostgreSQL exporter.
// These errors provide better error handling and context for debugging database operations.
package trazrexporter

import (
	"fmt"
	"time"

	"github.com/medxops/trazr-otel/pkg/repository"
)

// ConfigError represents configuration validation errors
type ConfigError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Type      string                 `json:"type"`
	Field     string                 `json:"field,omitempty"`
	Value     interface{}            `json:"value,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Cause     error                  `json:"-"`
}

// Error implements the error interface
func (e *ConfigError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s: %s (field: %s, value: %v)", e.Code, e.Message, e.Field, e.Value)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *ConfigError) Unwrap() error {
	return e.Cause
}

// WithField returns a new ConfigError with the field and value set
func (e *ConfigError) WithField(field string, value interface{}) *ConfigError {
	newErr := *e
	newErr.Field = field
	newErr.Value = value
	newErr.Timestamp = time.Now()
	return &newErr
}

// WithCause returns a new ConfigError with the underlying cause set
func (e *ConfigError) WithCause(cause error) *ConfigError {
	newErr := *e
	newErr.Cause = cause
	newErr.Timestamp = time.Now()
	return &newErr
}

// DatabaseError represents database operation errors
type DatabaseError struct {
	Code      string                 `json:"code"`
	Message   string                 `json:"message"`
	Type      string                 `json:"type"`
	Database  string                 `json:"database,omitempty"`
	Table     string                 `json:"table,omitempty"`
	Operation string                 `json:"operation,omitempty"`
	Query     string                 `json:"query,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
	Cause     error                  `json:"-"`
}

// Error implements the error interface
func (e *DatabaseError) Error() string {
	if e.Table != "" && e.Operation != "" {
		return fmt.Sprintf("%s: %s (operation: %s, table: %s)", e.Code, e.Message, e.Operation, e.Table)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *DatabaseError) Unwrap() error {
	return e.Cause
}

// WithOperation returns a new DatabaseError with operation details set
func (e *DatabaseError) WithOperation(operation, table string) *DatabaseError {
	newErr := *e
	newErr.Operation = operation
	newErr.Table = table
	newErr.Timestamp = time.Now()
	return &newErr
}

// WithQuery returns a new DatabaseError with the query set
func (e *DatabaseError) WithQuery(query string) *DatabaseError {
	newErr := *e
	newErr.Query = query
	newErr.Timestamp = time.Now()
	return &newErr
}

// ExportError represents data export/processing errors
type ExportError struct {
	Code       string                 `json:"code"`
	Message    string                 `json:"message"`
	Type       string                 `json:"type"`
	SignalType string                 `json:"signal_type,omitempty"`
	RecordID   string                 `json:"record_id,omitempty"`
	BatchSize  int                    `json:"batch_size,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
	Cause      error                  `json:"-"`
}

// Error implements the error interface
func (e *ExportError) Error() string {
	if e.SignalType != "" {
		return fmt.Sprintf("%s: %s (signal_type: %s)", e.Code, e.Message, e.SignalType)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error
func (e *ExportError) Unwrap() error {
	return e.Cause
}

// WithSignal returns a new ExportError with signal details set
func (e *ExportError) WithSignal(signalType string, recordID string) *ExportError {
	newErr := *e
	newErr.SignalType = signalType
	newErr.RecordID = recordID
	newErr.Timestamp = time.Now()
	return &newErr
}

// Common error constructors for typical failure scenarios

// NewEndpointValidationError creates a new error for endpoint validation failures
func NewEndpointValidationError() *ConfigError {
	return &ConfigError{
		Code:      "ENDPOINT_VALIDATION_FAILED",
		Message:   "endpoint or host must be specified",
		Type:      "validation_failed",
		Field:     "endpoint",
		Timestamp: time.Now(),
	}
}

// NewPortValidationError creates a new error for port validation failures
func NewPortValidationError(port int) *ConfigError {
	return &ConfigError{
		Code:      "PORT_VALIDATION_FAILED",
		Message:   "port must be between 1 and 65535",
		Type:      "validation_failed",
		Field:     "port",
		Value:     port,
		Timestamp: time.Now(),
	}
}

// NewDSNBuildError creates a new error for DSN building failures
func NewDSNBuildError(cause error) *ConfigError {
	return &ConfigError{
		Code:      "DSN_BUILD_FAILED",
		Message:   "failed to build database connection string",
		Type:      "configuration_error",
		Timestamp: time.Now(),
		Cause:     cause,
	}
}

// NewInvalidEndpointFormatError creates a new error for invalid endpoint format
func NewInvalidEndpointFormatError(endpoint string, cause error) *ConfigError {
	return &ConfigError{
		Code:      "INVALID_ENDPOINT_FORMAT",
		Message:   "invalid endpoint format",
		Type:      "validation_failed",
		Field:     "endpoint",
		Value:     endpoint,
		Timestamp: time.Now(),
		Cause:     cause,
	}
}

// NewSchemeValidationError creates a new error for invalid URL scheme
func NewSchemeValidationError(scheme string) *ConfigError {
	return &ConfigError{
		Code:      "INVALID_URL_SCHEME",
		Message:   "invalid endpoint format: must use postgres:// or postgresql:// scheme",
		Type:      "validation_failed",
		Field:     "scheme",
		Value:     scheme,
		Timestamp: time.Now(),
	}
}

// NewDatabaseConnectionError creates a new error for database connection failures
func NewDatabaseConnectionError(database string, cause error) *DatabaseError {
	return &DatabaseError{
		Code:      "CONNECTION_FAILED",
		Message:   "failed to connect to PostgreSQL database",
		Type:      "connection_failed",
		Database:  database,
		Timestamp: time.Now(),
		Cause:     cause,
	}
}

// NewTableCreationError creates a new error for table creation failures
func NewTableCreationError(tableName string, cause error) *DatabaseError {
	return &DatabaseError{
		Code:      "TABLE_CREATION_FAILED",
		Message:   "failed to create table",
		Type:      "ddl_operation_failed",
		Table:     tableName,
		Operation: "CREATE TABLE",
		Timestamp: time.Now(),
		Cause:     cause,
	}
}

// NewPartitionCreationError creates a new error for partition creation failures
func NewPartitionCreationError(tableName string, cause error) *DatabaseError {
	return &DatabaseError{
		Code:      "PARTITION_CREATION_FAILED",
		Message:   "failed to create table partitions",
		Type:      "ddl_operation_failed",
		Table:     tableName,
		Operation: "CREATE PARTITION",
		Timestamp: time.Now(),
		Cause:     cause,
	}
}

// NewDataInsertError creates a new error for data insertion failures
func NewDataInsertError(tableName string, batchSize int, cause error) *DatabaseError {
	return &DatabaseError{
		Code:      "DATA_INSERT_FAILED",
		Message:   "failed to insert data into table",
		Type:      "dml_operation_failed",
		Table:     tableName,
		Operation: "INSERT",
		Details:   map[string]interface{}{"batch_size": batchSize},
		Timestamp: time.Now(),
		Cause:     cause,
	}
}

// NewMetricProcessingError creates a new error for metric processing failures
func NewMetricProcessingError(metricType string, cause error) *ExportError {
	return &ExportError{
		Code:       "METRIC_PROCESSING_FAILED",
		Message:    "failed to process metric data",
		Type:       "data_processing_failed",
		SignalType: metricType,
		Timestamp:  time.Now(),
		Cause:      cause,
	}
}

// NewLogProcessingError creates a new error for log processing failures
func NewLogProcessingError(recordID string, cause error) *ExportError {
	return &ExportError{
		Code:       "LOG_PROCESSING_FAILED",
		Message:    "failed to process log record",
		Type:       "data_processing_failed",
		SignalType: repository.SignalTypeLogs,
		RecordID:   recordID,
		Timestamp:  time.Now(),
		Cause:      cause,
	}
}

// NewTraceProcessingError creates a new error for trace processing failures
func NewTraceProcessingError(spanID string, cause error) *ExportError {
	return &ExportError{
		Code:       "TRACE_PROCESSING_FAILED",
		Message:    "failed to process trace span",
		Type:       "data_processing_failed",
		SignalType: repository.SignalTypeSpans,
		RecordID:   spanID,
		Timestamp:  time.Now(),
		Cause:      cause,
	}
}

// Error type checking functions

// IsConfigError checks if an error is a ConfigError
func IsConfigError(err error) bool {
	_, ok := err.(*ConfigError)
	return ok
}

// IsDatabaseError checks if an error is a DatabaseError
func IsDatabaseError(err error) bool {
	_, ok := err.(*DatabaseError)
	return ok
}

// IsExportError checks if an error is an ExportError
func IsExportError(err error) bool {
	_, ok := err.(*ExportError)
	return ok
}
