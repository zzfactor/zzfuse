package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var traceQueryContractSequence atomic.Int64

// The schema comes from production DDL, independently of the repository's SELECT
// strings. This catches physical/API field-name drift without an external DB.
type traceQueryContractDriver struct {
	columns map[string]bool
	values  map[string]driver.Value
}

func (d traceQueryContractDriver) Open(string) (driver.Conn, error) {
	return traceQueryContractConnection{d}, nil
}

type traceQueryContractConnection struct{ traceQueryContractDriver }

func (traceQueryContractConnection) Prepare(string) (driver.Stmt, error) {
	return nil, fmt.Errorf("unexpected prepared query")
}
func (traceQueryContractConnection) Close() error { return nil }
func (traceQueryContractConnection) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("unexpected transaction")
}
func (c traceQueryContractConnection) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	selection, _, found := strings.Cut(strings.TrimPrefix(query, "SELECT "), " FROM ")
	if !found {
		return nil, fmt.Errorf("expected span SELECT query")
	}
	rows := &traceQueryContractRows{}
	for _, selected := range strings.Split(selection, ",") {
		fields := strings.Fields(selected)
		if len(fields) == 0 || !c.columns[fields[0]] {
			return nil, fmt.Errorf("selected column absent from current spans DDL: %s", strings.TrimSpace(selected))
		}
		column := fields[0]
		if len(fields) == 3 && strings.EqualFold(fields[1], "AS") {
			column = fields[2]
		}
		rows.columns = append(rows.columns, column)
		rows.values = append(rows.values, c.values[fields[0]])
	}
	return rows, nil
}

type traceQueryContractRows struct {
	columns []string
	values  []driver.Value
	done    bool
}

func (r *traceQueryContractRows) Columns() []string { return r.columns }
func (*traceQueryContractRows) Close() error        { return nil }
func (r *traceQueryContractRows) Next(values []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(values, r.values)
	r.done = true
	return nil
}

func TestTraceQueriesUseCurrentSpanDDLAndPreserveDurationAPI(t *testing.T) {
	ddl, err := os.ReadFile("internal/database/ddl/spans.sql")
	require.NoError(t, err)
	columns := make(map[string]bool)
	for _, match := range regexp.MustCompile(`(?m)^\s+([a-z][a-z0-9_]*)\s+(?:BIGSERIAL|BIGINT|TIMESTAMP|TEXT|INTEGER|JSONB)\b`).FindAllStringSubmatch(string(ddl), -1) {
		columns[match[1]] = true
	}
	require.True(t, columns["duration"])
	require.False(t, columns["duration_ns"])
	start := time.Unix(1700000000, 0).UTC()
	const duration = int64(1234567000)
	values := map[string]driver.Value{
		"id": int64(7), "timestamp_ns": start.UnixNano(), "timestamp": start,
		"end_time": start.Add(time.Duration(duration)), "duration": duration,
		"trace_id": "trace-test", "span_id": "span-test", "name": "span query contract",
		"span_kind": "SERVER", "flags": int64(1), "service_name": "test-service",
		"service_namespace": "test-namespace", "service_instance_id": "test-instance",
		"resource_attributes": []byte(`{"service.name":"test-service"}`),
		"scope_attributes":    []byte(`{}`), "attributes": []byte(`{}`),
		"events": []byte(`[]`), "links": []byte(`[]`), "metadata": []byte(`{}`),
	}
	name := fmt.Sprintf("trace-query-contract-%d", traceQueryContractSequence.Add(1))
	sql.Register(name, traceQueryContractDriver{columns: columns, values: values})
	db, err := sql.Open(name, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repository := NewTraceRepository(db)
	query := TraceQuery{}
	query.Schema = "test_schema"
	entries, _, _, _, _, err := repository.QuerySpans(context.Background(), query)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	byID, err := repository.GetSpanByID(context.Background(), 7, "test_schema")
	require.NoError(t, err)
	for _, entry := range []TraceEntry{entries[0], byID} {
		require.Equal(t, duration, entry.DurationNS)
		require.Equal(t, start, entry.StartTime)
		require.Equal(t, "trace-test", entry.TraceID)
		require.Equal(t, int32(1), *entry.Flags)
		require.Equal(t, "test-namespace", *entry.ServiceNamespace)
		require.Equal(t, "test-instance", *entry.ServiceInstanceID)
		apiResponse := toTraceEntryResponse(entry)
		require.Equal(t, start.Format(time.RFC3339Nano), apiResponse.StartTime)
		require.Equal(t, start.Add(time.Duration(duration)).Format(time.RFC3339Nano), apiResponse.EndTime)
		encoded, err := json.Marshal(apiResponse)
		require.NoError(t, err)
		var response map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(encoded, &response))
		require.JSONEq(t, "1234567000", string(response["duration_ns"]))
		require.NotContains(t, response, "duration")
	}
}
