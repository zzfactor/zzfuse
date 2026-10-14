package util

import (
	"database/sql"
	"encoding/json"

	"go.uber.org/zap"
)

// RollbackWithLog rolls back a transaction and logs any error.
// Use this instead of `_ = tx.Rollback()` to catch rollback failures.
func RollbackWithLog(tx *sql.Tx, logger *zap.Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		if logger != nil {
			logger.Warn("transaction rollback failed", zap.Error(err))
		}
	}
}

// RollbackWithLoggerInterface rolls back a transaction and logs any error using the Logger interface.
func RollbackWithLoggerInterface(tx *sql.Tx, logger Logger) {
	if tx == nil {
		return
	}
	if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
		if logger != nil {
			logger.Error("transaction rollback failed", "error", err)
		}
	}
}

// UnmarshalWithLog unmarshals JSON and logs any error.
// Returns true if successful, false otherwise.
// Use this instead of `_ = json.Unmarshal()` to catch parse failures.
func UnmarshalWithLog(data []byte, v interface{}, logger *zap.Logger, context string) bool {
	if err := json.Unmarshal(data, v); err != nil {
		if logger != nil {
			logger.Warn("JSON unmarshal failed",
				zap.String("context", context),
				zap.Error(err),
				zap.Int("data_length", len(data)))
		}
		return false
	}
	return true
}

// UnmarshalWithLoggerInterface unmarshals JSON and logs any error using the Logger interface.
func UnmarshalWithLoggerInterface(data []byte, v interface{}, logger Logger, context string) bool {
	if err := json.Unmarshal(data, v); err != nil {
		if logger != nil {
			logger.Error("JSON unmarshal failed", "context", context, "error", err, "data_length", len(data))
		}
		return false
	}
	return true
}

// CloseWithLog closes a resource and logs any error.
// Use this for io.Closer implementations where errors should be logged but not propagated.
func CloseWithLog(closer interface{ Close() error }, logger *zap.Logger, resource string) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil {
		if logger != nil {
			logger.Warn("failed to close resource",
				zap.String("resource", resource),
				zap.Error(err))
		}
	}
}

// IgnoreError wraps an error-returning operation that we intentionally ignore.
// This makes the intent explicit while still allowing the operation to execute.
// Use sparingly and only when the error truly doesn't matter.
func IgnoreError(fn func() error) {
	_ = fn()
}

// LogErrorAndContinue executes a function and logs any error without stopping execution.
// Returns true if successful, false if an error occurred.
func LogErrorAndContinue(fn func() error, logger *zap.Logger, operation string) bool {
	if err := fn(); err != nil {
		if logger != nil {
			logger.Warn("operation failed but continuing",
				zap.String("operation", operation),
				zap.Error(err))
		}
		return false
	}
	return true
}

// PublishEventWithLog publishes an event and logs any error.
// Use this instead of `_ = eventBus.Publish()`.
func PublishEventWithLog(publish func() error, logger *zap.Logger, eventType string) {
	if err := publish(); err != nil {
		if logger != nil {
			logger.Warn("event publish failed",
				zap.String("event_type", eventType),
				zap.Error(err))
		}
	}
}

// SafeUnmarshalJSONB unmarshals JSONB data into a map, returning an empty map on error.
// Logs the error if a logger is provided.
func SafeUnmarshalJSONB(data []byte, logger *zap.Logger, field string) map[string]interface{} {
	result := make(map[string]interface{})
	if len(data) == 0 {
		return result
	}
	if err := json.Unmarshal(data, &result); err != nil {
		if logger != nil {
			logger.Debug("JSONB unmarshal failed, using empty map",
				zap.String("field", field),
				zap.Error(err))
		}
	}
	return result
}

// SafeUnmarshalJSONBArray unmarshals JSONB data into a slice of maps, returning empty slice on error.
func SafeUnmarshalJSONBArray(data []byte, logger *zap.Logger, field string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0)
	if len(data) == 0 {
		return result
	}
	if err := json.Unmarshal(data, &result); err != nil {
		if logger != nil {
			logger.Debug("JSONB array unmarshal failed, using empty slice",
				zap.String("field", field),
				zap.Error(err))
		}
	}
	return result
}
