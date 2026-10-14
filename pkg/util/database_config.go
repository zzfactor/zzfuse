package util

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// DatabaseConfig holds configuration for database connections and queries
type DatabaseConfig struct {
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
	QueryTimeout    time.Duration `json:"query_timeout"`
}

// DefaultDatabaseConfig returns sensible defaults for database configuration
func DefaultDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		QueryTimeout:    30 * time.Second,
	}
}

// ConfigureConnectionPool configures the database connection pool with the given settings
func ConfigureConnectionPool(db *sql.DB, config *DatabaseConfig) {
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
}

// QueryMetrics tracks performance metrics for database queries
type QueryMetrics struct {
	QueryType string        `json:"query_type"`
	TableName string        `json:"table_name"`
	Duration  time.Duration `json:"duration"`
	RowCount  int           `json:"row_count"`
	Error     error         `json:"error,omitempty"`
	Timestamp time.Time     `json:"timestamp"`
}

// QueryLogger interface for structured logging of database operations
type QueryLogger interface {
	Info(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// MetricsRecorder interface for recording query performance metrics
type MetricsRecorder interface {
	RecordQueryDuration(queryType, tableName string, duration time.Duration)
	RecordQueryError(queryType, tableName string, err error)
	RecordQueryRowCount(queryType, tableName string, rowCount int)
	GetMetrics() map[string]interface{}
}

// SimpleQueryLogger provides a basic implementation of QueryLogger
type SimpleQueryLogger struct{}

func (l *SimpleQueryLogger) Info(msg string, fields ...interface{}) {
	// Basic implementation - can be enhanced with proper logging
	fmt.Printf("[INFO] %s", msg)
	if len(fields) > 0 {
		fmt.Printf(" %v", fields)
	}
	fmt.Println()
}

func (l *SimpleQueryLogger) Error(msg string, fields ...interface{}) {
	fmt.Printf("[ERROR] %s", msg)
	if len(fields) > 0 {
		fmt.Printf(" %v", fields)
	}
	fmt.Println()
}

func (l *SimpleQueryLogger) Debug(msg string, fields ...interface{}) {
	// Debug logging disabled to reduce verbosity
}

// SimpleMetricsRecorder provides a basic implementation of MetricsRecorder
type SimpleMetricsRecorder struct {
	metrics map[string]interface{}
	mutex   sync.RWMutex
}

// NewSimpleMetricsRecorder creates a new simple metrics recorder
func NewSimpleMetricsRecorder() *SimpleMetricsRecorder {
	return &SimpleMetricsRecorder{
		metrics: make(map[string]interface{}),
	}
}

func (m *SimpleMetricsRecorder) RecordQueryDuration(queryType, tableName string, duration time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := fmt.Sprintf("%s_%s_duration", queryType, tableName)
	m.metrics[key] = duration
}

func (m *SimpleMetricsRecorder) RecordQueryError(queryType, tableName string, err error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := fmt.Sprintf("%s_%s_error", queryType, tableName)
	m.metrics[key] = err.Error()
}

func (m *SimpleMetricsRecorder) RecordQueryRowCount(queryType, tableName string, rowCount int) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	key := fmt.Sprintf("%s_%s_row_count", queryType, tableName)
	m.metrics[key] = rowCount
}

func (m *SimpleMetricsRecorder) GetMetrics() map[string]interface{} {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	// Return a copy to avoid race conditions
	result := make(map[string]interface{})
	for k, v := range m.metrics {
		result[k] = v
	}
	return result
}
