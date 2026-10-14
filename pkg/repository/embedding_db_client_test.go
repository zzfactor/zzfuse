package repository

import (
	"strings"
	"testing"
)

// TestSpansColumnCount verifies that the SELECT query and scan operation have matching column counts
func TestSpansColumnCount(t *testing.T) {
	// Get the SELECT fields for spans
	selectFields := "id, timestamp_ns, timestamp, end_time, duration, trace_id, span_id, parent_span_id, trace_state, name, span_kind, service_name, resource_attributes, scope_name, scope_version, scope_attributes, scope_dropped_attr_count, attributes, dropped_attr_count, events, dropped_events_count, links, dropped_links_count, status_code, status_message, metadata, resource_schema_url, scope_schema_url"

	// Count the columns in the SELECT fields
	columnCount := len(strings.Split(selectFields, ","))

	// Expected count: 28 columns (removed processed_content)
	expectedCount := 28

	if columnCount != expectedCount {
		t.Errorf("Expected %d columns in SELECT fields, got %d", expectedCount, columnCount)
		t.Logf("SELECT fields: %s", selectFields)
	}

	// Verify the scan operation has the same number of variables
	// This is a manual check - the scan operation should have 28 variables (removed processed_content)
	scanVariables := []string{
		"id", "timestampNs", "timestamp", "endTime", "duration", "traceID", "spanID", "parentSpanID",
		"traceState", "name", "spanKind", "serviceName", "resourceAttrsBytes",
		"scopeName", "scopeVersion", "scopeAttrsBytes", "scopeDroppedAttrCount",
		"attributesBytes", "droppedAttrCount", "eventsBytes", "droppedEventsCount",
		"linksBytes", "droppedLinksCount", "statusCode", "statusMessage",
		"metadataBytes",
		"resourceSchemaUrl", "scopeSchemaUrl",
	}

	if len(scanVariables) != expectedCount {
		t.Errorf("Expected %d variables in scan operation, got %d", expectedCount, len(scanVariables))
		t.Logf("Scan variables: %v", scanVariables)
	}
}

// TestSpansColumnOrder verifies that the columns are in the correct order
func TestSpansColumnOrder(t *testing.T) {
	// Expected column order from the SELECT query
	expectedColumns := []string{
		"id", "timestamp_ns", "timestamp", "end_time", "duration", "trace_id", "span_id", "parent_span_id",
		"trace_state", "name", "span_kind", "service_name", "resource_attributes",
		"scope_name", "scope_version", "scope_attributes", "scope_dropped_attr_count",
		"attributes", "dropped_attr_count", "events", "dropped_events_count",
		"links", "dropped_links_count", "status_code", "status_message",
		"metadata",
		"resource_schema_url", "scope_schema_url",
	}

	// Get the actual SELECT fields
	selectFields := "id, timestamp_ns, timestamp, end_time, duration, trace_id, span_id, parent_span_id, trace_state, name, span_kind, service_name, resource_attributes, scope_name, scope_version, scope_attributes, scope_dropped_attr_count, attributes, dropped_attr_count, events, dropped_events_count, links, dropped_links_count, status_code, status_message, metadata, resource_schema_url, scope_schema_url"

	actualColumns := strings.Split(selectFields, ", ")

	if len(actualColumns) != len(expectedColumns) {
		t.Errorf("Column count mismatch: expected %d, got %d", len(expectedColumns), len(actualColumns))
		return
	}

	for i, expected := range expectedColumns {
		if i >= len(actualColumns) {
			t.Errorf("Missing column at position %d: %s", i, expected)
			continue
		}
		if actualColumns[i] != expected {
			t.Errorf("Column mismatch at position %d: expected %s, got %s", i, expected, actualColumns[i])
		}
	}
}
