package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// SQLDatabaseClient provides database operations for embedding-related functionality
type SQLDatabaseClient struct {
	DB     *sql.DB
	Schema string // Database schema name (e.g., "default_schema")
}

// EmbeddingReceipt represents an embedding configuration receipt
type EmbeddingReceipt struct {
	ConfigID   int64  `json:"processor_id"`
	EmbeddedID string `json:"embedded_id"`
}

// SignalMetadata represents metadata for signals including embeddings
type SignalMetadata struct {
	Embeddings []EmbeddingReceipt `json:"embeddings,omitempty"`
}

// =========================
// Core Methods
// =========================

// FetchOptions defines options for fetching unembedded signals
type FetchOptions struct {
	UseTransaction bool   // Whether to use a transaction with row locking
	OrderBy        string // "ASC" for oldest first, "DESC" for newest first
	LockMode       string // "FOR UPDATE SKIP LOCKED" or empty for no locking
}

// DefaultFetchOptions returns default fetch options for simple read-only operations
func DefaultFetchOptions() *FetchOptions {
	return &FetchOptions{
		UseTransaction: false,
		OrderBy:        "DESC", // newest first
		LockMode:       "",     // no locking
	}
}

// TransactionalFetchOptions returns fetch options for high-concurrency scenarios
func TransactionalFetchOptions() *FetchOptions {
	return &FetchOptions{
		UseTransaction: true,
		OrderBy:        "ASC", // oldest first for fair processing
		LockMode:       "FOR UPDATE SKIP LOCKED",
	}
}

// FetchUnembeddedSignalsAdvanced fetches unembedded signals from a specific table with configurable options.
// This is the advanced version used by embedding workers.
func (c *SQLDatabaseClient) FetchUnembeddedSignalsAdvanced(ctx context.Context, tableName string, batchSize int, configID int64, schema string, options *FetchOptions, requiredFields []SignalFieldConfig) ([]map[string]interface{}, *sql.Tx, error) {
	if options == nil {
		options = DefaultFetchOptions()
	}

	// Debug logging for schema parameter
	if schema == "" {
		return nil, nil, fmt.Errorf("schema parameter is empty - cannot construct table name")
	}

	fullTableName := fmt.Sprintf("%s.%s", schema, tableName)

	// Build WHERE clause - optimized for PostgreSQL 17 JSONB performance
	// Use @> operator for optimal GIN index utilization and performance
	whereClause := fmt.Sprintf("(metadata->>'embeddings' IS NULL OR NOT (metadata->'embeddings' @> '[{\"processor_id\": %d}]'))", configID)

	// Build SELECT fields from required fields or fallback to full configuration
	var selectFields string
	if len(requiredFields) > 0 {
		// Use dynamic field selection based on template requirements
		fieldNames := make([]string, len(requiredFields))
		for i, fieldConfig := range requiredFields {
			fieldNames[i] = fieldConfig.FieldName
		}
		selectFields = strings.Join(fieldNames, ", ")
	} else {
		// Fallback to full field configuration
		config, exists := signalFieldConfigs[tableName]
		if !exists {
			// Fallback for unknown signal types
			selectFields = "id, timestamp_ns, service_name, metadata"
		} else {
			fieldNames := make([]string, len(config))
			for i, fieldConfig := range config {
				fieldNames[i] = fieldConfig.FieldName
			}
			selectFields = strings.Join(fieldNames, ", ")
		}
	}

	// Build the query with optional transaction and locking
	var query string
	if options.UseTransaction {
		query = fmt.Sprintf(`
			SELECT %s
			FROM %s 
			WHERE %s
			ORDER BY timestamp_ns %s 
			LIMIT %d %s`,
			selectFields, fullTableName, whereClause, options.OrderBy, batchSize, options.LockMode)
	} else {
		query = fmt.Sprintf(`
			SELECT %s
			FROM %s 
			WHERE %s
			ORDER BY timestamp_ns %s 
			LIMIT %d`,
			selectFields, fullTableName, whereClause, options.OrderBy, batchSize)
	}

	// Execute query
	var rows *sql.Rows
	var err error
	var tx *sql.Tx

	if options.UseTransaction {
		tx, err = c.DB.BeginTx(ctx, nil)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
		}
		rows, err = tx.QueryContext(ctx, query)
	} else {
		rows, err = c.DB.QueryContext(ctx, query)
	}

	if err != nil {
		if tx != nil {
			tx.Rollback()
		}
		return nil, nil, fmt.Errorf("failed to query unembedded signals: %w", err)
	}
	defer rows.Close()

	// Scan results
	var signals []map[string]interface{}
	for rows.Next() {
		signal, err := c.scanSignalRow(rows, tableName, requiredFields)
		if err != nil {
			if tx != nil {
				tx.Rollback()
			}
			return nil, nil, fmt.Errorf("failed to scan signal row: %w", err)
		}
		signals = append(signals, signal)
	}

	if err := rows.Err(); err != nil {
		if tx != nil {
			tx.Rollback()
		}
		return nil, nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	return signals, tx, nil
}

// FetchUnembeddedSignalsSimple is a convenience method for backward compatibility.
// This is the simple version used by the basic interface.
func (c *SQLDatabaseClient) FetchUnembeddedSignalsSimple(ctx context.Context, tableName string, batchSize int, filterConfig map[string]interface{}, configID int64, schema string) ([]map[string]interface{}, error) {
	results, _, err := c.FetchUnembeddedSignalsAdvanced(ctx, tableName, batchSize, configID, schema, DefaultFetchOptions(), nil)
	return results, err
}

// MarkSignalsEmbeddedAdvanced marks the given signals as embedded in the specified table.
// This is the advanced version used by embedding workers.
func (c *SQLDatabaseClient) MarkSignalsEmbeddedAdvanced(ctx context.Context, tableName string, ids []interface{}, configID int64, embeddedIDs []string, schema string) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) != len(embeddedIDs) {
		return fmt.Errorf("mismatch between signal IDs (%d) and embedded IDs (%d)", len(ids), len(embeddedIDs))
	}

	fullTableName := fmt.Sprintf("%s.%s", schema, tableName)

	// Use a transaction for bulk update with PostgreSQL 17 optimized performance
	// This implements upsert logic to prevent duplicate processor_id entries
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare the statement for better performance
	// Simple append approach - duplicates will be handled by application logic
	stmt, err := tx.PrepareContext(ctx, fmt.Sprintf(`
		UPDATE %s 
		SET metadata = jsonb_set(
			COALESCE(metadata, '{}'::jsonb),
			'{embeddings}',
			COALESCE(metadata->'embeddings', '[]'::jsonb) || 
			jsonb_build_array(jsonb_build_object('processor_id', $1::bigint, 'embedded_id', $2::text))
		)
		WHERE id = $3`, fullTableName))
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Execute bulk update
	for i, id := range ids {
		_, err := stmt.ExecContext(ctx, configID, embeddedIDs[i], id)
		if err != nil {
			return fmt.Errorf("failed to mark signal %v as embedded: %w", id, err)
		}
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// =========================
// Filter Cleaning Helpers
// =========================

// =========================
// SQL and Scan Helpers
// =========================

// SignalFieldConfig defines the configuration for scanning signal fields
type SignalFieldConfig struct {
	FieldName       string // Database column name
	SignalKey       string // Key in the signal map
	IsJSONB         bool   // Whether this field should be unmarshaled as JSONB
	SpecialHandling string // Special handling type (e.g., "body", "profile_bytes")
}

// signalFieldConfigs defines the field configurations for each signal type
var signalFieldConfigs = map[string][]SignalFieldConfig{
	SignalTypeLogs: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "observed_timestamp", SignalKey: "observed_timestamp"},
		{FieldName: "trace_id", SignalKey: "trace_id"},
		{FieldName: "span_id", SignalKey: "span_id"},
		{FieldName: "flags", SignalKey: "flags"},
		{FieldName: "severity_text", SignalKey: "severity_text"},
		{FieldName: "severity_number", SignalKey: "severity_number"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "dropped_attr_count", SignalKey: "dropped_attr_count"},
		{FieldName: "body", SignalKey: "body", SpecialHandling: "body"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},
		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
	SignalTypeSpans: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "end_time", SignalKey: "end_time"},
		{FieldName: "duration", SignalKey: "duration"},
		{FieldName: "trace_id", SignalKey: "trace_id"},
		{FieldName: "span_id", SignalKey: "span_id"},
		{FieldName: "parent_span_id", SignalKey: "parent_span_id"},
		{FieldName: "trace_state", SignalKey: "trace_state"},
		{FieldName: "name", SignalKey: "name"},
		{FieldName: "span_kind", SignalKey: "span_kind"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "dropped_attr_count", SignalKey: "dropped_attr_count"},
		{FieldName: "events", SignalKey: "events", IsJSONB: true},
		{FieldName: "dropped_events_count", SignalKey: "dropped_events_count"},
		{FieldName: "links", SignalKey: "links", IsJSONB: true},
		{FieldName: "dropped_links_count", SignalKey: "dropped_links_count"},
		{FieldName: "status_code", SignalKey: "status_code"},
		{FieldName: "status_message", SignalKey: "status_message"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},

		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
	SignalTypeGaugeMetrics: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "name", SignalKey: "name"},
		{FieldName: "description", SignalKey: "description"},
		{FieldName: "unit", SignalKey: "unit"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "value", SignalKey: "value"},
		{FieldName: "flags", SignalKey: "flags"},
		{FieldName: "exemplars", SignalKey: "exemplars", IsJSONB: true},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},
		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
	SignalTypeSumMetrics: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "name", SignalKey: "name"},
		{FieldName: "description", SignalKey: "description"},
		{FieldName: "unit", SignalKey: "unit"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "start_time", SignalKey: "start_time"},
		{FieldName: "value", SignalKey: "value"},
		{FieldName: "flags", SignalKey: "flags"},
		{FieldName: "exemplars", SignalKey: "exemplars", IsJSONB: true},
		{FieldName: "aggregation_temporality", SignalKey: "aggregation_temporality"},
		{FieldName: "is_monotonic", SignalKey: "is_monotonic"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},
		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
	SignalTypeHistogramMetrics: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "name", SignalKey: "name"},
		{FieldName: "description", SignalKey: "description"},
		{FieldName: "unit", SignalKey: "unit"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "start_time", SignalKey: "start_time"},
		{FieldName: "count", SignalKey: "count"},
		{FieldName: "sum", SignalKey: "sum"},
		{FieldName: "buckets", SignalKey: "buckets", IsJSONB: true},
		{FieldName: "flags", SignalKey: "flags"},
		{FieldName: "aggregation_temporality", SignalKey: "aggregation_temporality"},
		{FieldName: "exemplars", SignalKey: "exemplars", IsJSONB: true},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},
		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
	SignalTypeSummaryMetrics: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "name", SignalKey: "name"},
		{FieldName: "description", SignalKey: "description"},
		{FieldName: "unit", SignalKey: "unit"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "start_time", SignalKey: "start_time"},
		{FieldName: "count", SignalKey: "count"},
		{FieldName: "sum", SignalKey: "sum"},
		{FieldName: "quantiles", SignalKey: "quantiles", IsJSONB: true},
		{FieldName: "flags", SignalKey: "flags"},
		{FieldName: "exemplars", SignalKey: "exemplars", IsJSONB: true},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},
		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
	SignalTypeExponentialHistogramMetrics: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "name", SignalKey: "name"},
		{FieldName: "description", SignalKey: "description"},
		{FieldName: "unit", SignalKey: "unit"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "start_time", SignalKey: "start_time"},
		{FieldName: "count", SignalKey: "count"},
		{FieldName: "sum", SignalKey: "sum"},
		{FieldName: "scale", SignalKey: "scale"},
		{FieldName: "zero_count", SignalKey: "zero_count"},
		{FieldName: "positive_offset", SignalKey: "positive_offset"},
		{FieldName: "positive_bucket_counts", SignalKey: "positive_bucket_counts", IsJSONB: true},
		{FieldName: "negative_offset", SignalKey: "negative_offset"},
		{FieldName: "negative_bucket_counts", SignalKey: "negative_bucket_counts", IsJSONB: true},
		{FieldName: "exemplars", SignalKey: "exemplars", IsJSONB: true},
		{FieldName: "flags", SignalKey: "flags"},
		{FieldName: "min", SignalKey: "min"},
		{FieldName: "max", SignalKey: "max"},
		{FieldName: "aggregation_temporality", SignalKey: "aggregation_temporality"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},
		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
	SignalTypeProfiles: {
		{FieldName: "id", SignalKey: "id"},
		{FieldName: "timestamp_ns", SignalKey: "timestamp_ns"},
		{FieldName: "timestamp", SignalKey: "timestamp"},
		{FieldName: "service_name", SignalKey: "service_name"},
		{FieldName: "profile_type", SignalKey: "profile_type"},
		{FieldName: "duration", SignalKey: "duration"},
		{FieldName: "sample_type", SignalKey: "sample_type"},
		{FieldName: "sample_unit", SignalKey: "sample_unit"},
		{FieldName: "profile_bytes", SignalKey: "profile_bytes", SpecialHandling: "profile_bytes"},
		{FieldName: "attributes", SignalKey: "attributes", IsJSONB: true},
		{FieldName: "resource_attributes", SignalKey: "resource_attributes", IsJSONB: true},
		{FieldName: "scope_name", SignalKey: "scope_name"},
		{FieldName: "scope_version", SignalKey: "scope_version"},
		{FieldName: "scope_attributes", SignalKey: "scope_attributes", IsJSONB: true},
		{FieldName: "scope_dropped_attr_count", SignalKey: "scope_dropped_attr_count"},
		{FieldName: "metadata", SignalKey: "metadata", IsJSONB: true},
		{FieldName: "resource_schema_url", SignalKey: "resource_schema_url"},
		{FieldName: "scope_schema_url", SignalKey: "scope_schema_url"},
	},
}

// unmarshalJSONB safely unmarshals JSONB data
func unmarshalJSONB(data interface{}) interface{} {
	if data == nil {
		return nil
	}
	if bb, ok := data.([]byte); ok && len(bb) > 0 {
		var v interface{}
		if err := json.Unmarshal(bb, &v); err == nil {
			return v
		}
		return string(bb)
	}
	return data
}

// handleSpecialField processes fields that need special handling
func handleSpecialField(fieldName string, data interface{}) interface{} {
	switch fieldName {
	case "body":
		if data != nil {
			if b, ok := data.([]byte); ok && len(b) > 0 {
				var body interface{}
				if err := json.Unmarshal(b, &body); err != nil {
					return string(b) // fallback: raw string if not valid JSON
				}
				return body
			}
		}
		return data
	case "profile_bytes":
		// Profile bytes are typically binary data, return as-is
		return data
	default:
		return data
	}
}

func (c *SQLDatabaseClient) scanSignalRow(rows *sql.Rows, tableName string, requiredFields []SignalFieldConfig) (map[string]interface{}, error) {
	signal := make(map[string]interface{})

	// Use required fields if provided, otherwise fallback to full config
	config := requiredFields
	if len(config) == 0 {
		if fullConfig, exists := signalFieldConfigs[tableName]; exists {
			config = fullConfig
		} else {
			// Fallback for unknown signal types - preserve original business logic
			var id, timestampNs, serviceName interface{}
			var metadataJSON sql.NullString
			if err := rows.Scan(&id, &timestampNs, &serviceName, &metadataJSON); err != nil {
				return nil, err
			}
			signal["id"] = id
			signal["timestamp_ns"] = timestampNs
			signal["service_name"] = serviceName

			// Parse existing metadata if present (preserve original business logic)
			if metadataJSON.Valid {
				var metadata SignalMetadata
				if err := json.Unmarshal([]byte(metadataJSON.String), &metadata); err == nil {
					signal["existing_metadata"] = metadata
				}
			}

			signal["signal_type"] = tableName
			return signal, nil
		}
	}

	// Create slice of interface{} for scanning
	scanArgs := make([]interface{}, len(config))
	for i := range scanArgs {
		scanArgs[i] = new(interface{})
	}

	// Scan the row
	if err := rows.Scan(scanArgs...); err != nil {
		return nil, err
	}

	// Process each field according to its configuration
	for i, fieldConfig := range config {
		value := *(scanArgs[i].(*interface{}))

		// Apply special handling if needed
		if fieldConfig.SpecialHandling != "" {
			value = handleSpecialField(fieldConfig.SpecialHandling, value)
		}

		// Unmarshal JSONB fields
		if fieldConfig.IsJSONB {
			value = unmarshalJSONB(value)
		}

		// Store in signal map
		signal[fieldConfig.SignalKey] = value
	}

	// Inject signal_type for template selection
	signal["signal_type"] = tableName
	return signal, nil
}
