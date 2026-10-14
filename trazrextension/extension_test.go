//go:build integration
// +build integration

package trazrextension

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/medxops/trazr-otel/trazrextension/internal"
	"github.com/stretchr/testify/require"
)

// Integration test for the real API at http://localhost:18888
// Requires the API and DB to be running, and a valid JWT (set via TEST_JWT env var)
const defaultAPIBaseURL = "http://localhost:18888"

func getTestJWT(t *testing.T) string {
	t.Helper()
	token := os.Getenv("TEST_JWT")
	if token == "" {
		t.Fatal("set TEST_JWT to a valid integration token")
	}
	if !strings.HasPrefix(token, "Bearer ") {
		token = "Bearer " + token
	}
	return token
}

func getAPIBaseURL() string {
	url := os.Getenv("TEST_API_BASE_URL")
	if url == "" {
		return ""
	}
	return url
}

func TestReadJWT(t *testing.T) {
	requireExtensionIntegration(t)
	jwt := getTestJWT(t)
	if jwt == "" || jwt == "Bearer test-token" {
		t.Error("JWT token is empty or using the default placeholder. Set TEST_JWT env var to a valid token.")
	}
}

func TestAPIIntegration(t *testing.T) {
	requireExtensionIntegration(t)
	type scenario struct {
		Name          string
		Method        string
		Endpoint      string
		Payload       interface{}
		QueryParams   map[string]string
		ExpectStatus  int
		ExpectIDs     []string
		ExpectEntries int
		ExpectError   bool
		WithJWT       bool                                    // <-- new field
		PostCheck     func(t *testing.T, apiBase, jwt string) // <-- new field
	}

	// Use a fixed RFC3339 timestamp for all inserts to ensure partitioning works
	fixedTimestamp := "2025-07-10T14:04:10Z"
	fixedEndTime := "2025-07-10T14:14:10Z" // 10 minutes after start

	testCases := []scenario{
		{
			Name:     "Insert single metric (sum)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=sum",
			Payload: map[string]interface{}{
				"Timestamp":   fixedTimestamp,
				"ServiceName": "api",
				"Name":        "cpu_sum",
				"Value":       2.34,
				"Metadata":    map[string]interface{}{},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:     "Insert single metric (histogram)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=histogram",
			Payload: map[string]interface{}{
				"Timestamp":   fixedTimestamp,
				"ServiceName": "api",
				"Name":        "latency_hist",
				"Count":       1,
				"Sum":         10.0,
				"Buckets": map[string]interface{}{
					"bucket_counts":   []int{1, 2, 3},
					"explicit_bounds": []float64{0.1, 1, 10},
				},
				"Metadata": map[string]interface{}{},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:     "Insert single metric (summary)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=summary",
			Payload: map[string]interface{}{
				"Timestamp":   fixedTimestamp,
				"ServiceName": "api",
				"Name":        "response_summary",
				"Count":       1,
				"Sum":         5.0,
				"Quantiles":   map[string]float64{"0.5": 2.5, "0.9": 4.5},
				"Metadata":    map[string]interface{}{},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:     "Insert single metric (exponential_histogram)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=exponential_histogram",
			Payload: map[string]interface{}{
				"Timestamp":   fixedTimestamp,
				"ServiceName": "api",
				"Name":        "exp_hist",
				"Count":       1,
				"Sum":         7.0,
				"Buckets": map[string]interface{}{
					"scale":                  1,
					"zero_count":             0,
					"positive_offset":        0,
					"positive_bucket_counts": []int{1, 2, 3},
					"negative_offset":        0,
					"negative_bucket_counts": []int{0, 0, 0},
				},
				"StartTime": fixedTimestamp,
				"Metadata":  map[string]interface{}{},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:     "Insert single log (full model)",
			Method:   "POST",
			Endpoint: "/api/v1/logs",
			Payload: map[string]interface{}{
				"Timestamp":          fixedTimestamp,
				"ServiceName":        "api",
				"Body":               "test log",
				"SeverityText":       "INFO",
				"SeverityNumber":     9,
				"TraceID":            "abc123",
				"SpanID":             "def456",
				"Attributes":         map[string]interface{}{"env": "prod", "user": "alice"},
				"ResourceAttributes": map[string]interface{}{"host": "server1"},
				"Metadata":           map[string]interface{}{"custom": "meta"},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
			// Add a post-insert check for metadata
			PostCheck: func(t *testing.T, apiBase, jwt string) {
				// Query the log
				req, _ := http.NewRequest("GET", apiBase+"/api/v1/logs?service=api&limit=1", nil)
				req.Header.Set("Authorization", jwt)
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				defer resp.Body.Close()
				var result struct {
					Entries []map[string]interface{} `json:"Entries"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&result)
				require.NotEmpty(t, result.Entries)
				entry := result.Entries[0]
				metadata, ok := entry["metadata"].(map[string]interface{})
				require.True(t, ok, "metadata should be a JSON object, not array or null")
				require.Equal(t, "meta", metadata["custom"])
			},
		},
		{
			Name:          "Query logs",
			Method:        "GET",
			Endpoint:      "/api/v1/logs",
			QueryParams:   map[string]string{"service": "api", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 1,
		},
		{
			Name:     "Insert single trace (full model)",
			Method:   "POST",
			Endpoint: "/api/v1/spans",
			Payload: map[string]interface{}{
				"Timestamp":          fixedTimestamp,
				"EndTime":            fixedEndTime,
				"TraceID":            "abc",
				"SpanID":             "def",
				"Name":               "span",
				"ServiceName":        "api",
				"ParentSpanID":       "parent1",
				"SpanKind":           "server",
				"Attributes":         map[string]interface{}{"http.method": "GET"},
				"ResourceAttributes": map[string]interface{}{"host": "server1"},
				"Metadata":           map[string]interface{}{"custom": "meta"},
				"Events":             []interface{}{},
				"Links":              []interface{}{},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:          "Query spans",
			Method:        "GET",
			Endpoint:      "/api/v1/spans",
			QueryParams:   map[string]string{"service": "api", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 1,
		},
		{
			Name:     "Insert single metric (gauge, full model)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=gauge",
			Payload: map[string]interface{}{
				"Timestamp":          fixedTimestamp,
				"ServiceName":        "api",
				"Name":               "cpu",
				"Value":              1.23,
				"Unit":               "percent",
				"Description":        "CPU usage",
				"Attributes":         map[string]interface{}{"core": "0"},
				"ResourceAttributes": map[string]interface{}{"host": "server1"},
				"Metadata":           map[string]interface{}{"custom": "meta"},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:          "Query metrics",
			Method:        "GET",
			Endpoint:      "/api/v1/metrics",
			QueryParams:   map[string]string{"service": "api", "type": "gauge", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 1,
		},
		{
			Name:     "Insert single profile (full model) [BETA]",
			Method:   "POST",
			Endpoint: "/api/v1/profiles",
			Payload: map[string]interface{}{
				"Timestamp":          fixedTimestamp,
				"ServiceName":        "api",
				"ProfileType":        "cpu",
				"ProfileBytes":       "dGVzdA==",
				"DurationNS":         1000000,
				"SampleType":         "cpu",
				"SampleUnit":         "nanoseconds",
				"Attributes":         map[string]interface{}{"env": "prod"},
				"ResourceAttributes": map[string]interface{}{"host": "server1"},
				"Metadata":           map[string]interface{}{"custom": "meta"},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:          "Query profiles [BETA]",
			Method:        "GET",
			Endpoint:      "/api/v1/profiles",
			QueryParams:   map[string]string{"service": "api", "limit": "10", "start": "2025-07-10T00:00:00Z", "end": "2025-07-10T23:59:59Z"},
			ExpectStatus:  200,
			ExpectEntries: 1,
		},
		// {
		// 	Name:     "Insert batch logs",
		// 	Method:   "POST",
		// 	Endpoint: "/api/v1/logs",
		// 	Payload: []map[string]interface{}{
		// 		{"Timestamp": fixedTimestamp, "ServiceName": "api", "Body": "log1", "Metadata": map[string]interface{}{}},
		// 		{"Timestamp": fixedTimestamp, "ServiceName": "api", "Body": "log2", "Metadata": map[string]interface{}{}},
		// 	},
		// 	ExpectStatus: 200,
		// 	ExpectIDs:    []string{"any", "any"},
		// },
		// {
		// 	Name:          "Query batch logs",
		// 	Method:        "GET",
		// 	Endpoint:      "/api/v1/logs",
		// 	QueryParams:   map[string]string{"service": "api", "limit": "10"},
		// 	ExpectStatus:  200,
		// 	ExpectEntries: 2,
		// },
		{
			Name:     "Insert batch spans",
			Method:   "POST",
			Endpoint: "/api/v1/spans",
			Payload: []map[string]interface{}{
				{"Timestamp": fixedTimestamp, "EndTime": fixedEndTime, "TraceID": "abc1", "SpanID": "def1", "Name": "span1", "ServiceName": "api", "Events": []interface{}{}, "Links": []interface{}{}, "Metadata": map[string]interface{}{}},
				{"Timestamp": fixedTimestamp, "EndTime": fixedEndTime, "TraceID": "abc2", "SpanID": "def2", "Name": "span2", "ServiceName": "api", "Events": []interface{}{}, "Links": []interface{}{}, "Metadata": map[string]interface{}{}},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any", "any"},
		},
		{
			Name:          "Query batch spans",
			Method:        "GET",
			Endpoint:      "/api/v1/spans",
			QueryParams:   map[string]string{"service": "api", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 2,
		},
		{
			Name:     "Insert batch metrics (sum)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=sum",
			Payload: []map[string]interface{}{
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "cpu", "Value": 1.23, "Count": 2, "Sum": 2.46, "Metadata": map[string]interface{}{}},
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "mem", "Value": 2.34, "Count": 3, "Sum": 7.02, "Metadata": map[string]interface{}{}},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any", "any"},
		},
		{
			Name:          "Query batch metrics (sum)",
			Method:        "GET",
			Endpoint:      "/api/v1/metrics",
			QueryParams:   map[string]string{"service": "api", "type": "sum", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 2,
		},
		{
			Name:     "Insert batch profiles [BETA]",
			Method:   "POST",
			Endpoint: "/api/v1/profiles",
			Payload: []map[string]interface{}{
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "ProfileType": "cpu", "ProfileBytes": "dGVzdA==", "Metadata": map[string]interface{}{}},
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "ProfileType": "heap", "ProfileBytes": "dGVzdA==", "Metadata": map[string]interface{}{}},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any", "any"},
		},
		{
			Name:          "Query batch profiles [BETA]",
			Method:        "GET",
			Endpoint:      "/api/v1/profiles",
			QueryParams:   map[string]string{"service": "api", "limit": "10", "start": "2025-07-10T00:00:00Z", "end": "2025-07-10T23:59:59Z"},
			ExpectStatus:  200,
			ExpectEntries: 2,
		},
		// Error scenarios
		{
			Name:         "Insert log missing required field",
			Method:       "POST",
			Endpoint:     "/api/v1/logs",
			Payload:      map[string]interface{}{"ServiceName": "api", "Body": "no timestamp"},
			ExpectStatus: 400,
			ExpectError:  true,
		},
		{
			Name:         "Insert invalid JSON",
			Method:       "POST",
			Endpoint:     "/api/v1/logs",
			Payload:      "{invalid json}",
			ExpectStatus: 400,
			ExpectError:  true,
		},
		{
			Name:          "Query logs with no results",
			Method:        "GET",
			Endpoint:      "/api/v1/logs",
			QueryParams:   map[string]string{"service": "doesnotexist", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 0,
		},
		// Pagination scenario
		{
			Name:          "Pagination logs (limit=1)",
			Method:        "GET",
			Endpoint:      "/api/v1/logs",
			QueryParams:   map[string]string{"service": "api", "limit": "1"},
			ExpectStatus:  200,
			ExpectEntries: 1,
		},
		// Filtering scenario
		{
			Name:         "Insert log with severity",
			Method:       "POST",
			Endpoint:     "/api/v1/logs",
			Payload:      map[string]interface{}{"Timestamp": fixedTimestamp, "ServiceName": "api", "Body": "warn log", "SeverityText": "WARN", "Metadata": map[string]interface{}{}},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any"},
		},
		{
			Name:          "Query logs by severity",
			Method:        "GET",
			Endpoint:      "/api/v1/logs",
			QueryParams:   map[string]string{"service": "api", "severity": "WARN", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 1,
		},
		// Auth failure
		{
			Name:         "Insert log with missing JWT",
			Method:       "POST",
			Endpoint:     "/api/v1/logs",
			Payload:      map[string]interface{}{"Timestamp": fixedTimestamp, "ServiceName": "api", "Body": "no jwt"},
			ExpectStatus: 401,
			ExpectError:  true,
			WithJWT:      false, // <-- do not set JWT for this test
		},
		// Batch metrics: histogram
		{
			Name:     "Insert batch metrics (histogram)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=histogram",
			Payload: []map[string]interface{}{
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "latency", "Count": 2, "Sum": 10.0, "Buckets": []float64{1, 2, 3}, "Metadata": map[string]interface{}{}},
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "latency", "Count": 3, "Sum": 15.0, "Buckets": []float64{2, 3, 4}, "Metadata": map[string]interface{}{}},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any", "any"},
		},
		{
			Name:          "Query batch metrics (histogram)",
			Method:        "GET",
			Endpoint:      "/api/v1/metrics",
			QueryParams:   map[string]string{"service": "api", "type": "histogram", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 2,
		},
		// Batch metrics: summary
		{
			Name:     "Insert batch metrics (summary)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=summary",
			Payload: []map[string]interface{}{
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "response", "Count": 2, "Sum": 10.0, "Quantiles": map[string]float64{"0.5": 5.0, "0.9": 9.0}, "Metadata": map[string]interface{}{}},
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "response", "Count": 3, "Sum": 15.0, "Quantiles": map[string]float64{"0.5": 7.0, "0.9": 13.0}, "Metadata": map[string]interface{}{}},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any", "any"},
		},
		{
			Name:          "Query batch metrics (summary)",
			Method:        "GET",
			Endpoint:      "/api/v1/metrics",
			QueryParams:   map[string]string{"service": "api", "type": "summary", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 2,
		},
		// Batch metrics: exponential_histogram
		{
			Name:     "Insert batch metrics (exponential_histogram)",
			Method:   "POST",
			Endpoint: "/api/v1/metrics?type=exponential_histogram",
			Payload: []map[string]interface{}{
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "exp", "Count": 2, "Sum": 10.0, "Buckets": []float64{1, 2, 3}, "Metadata": map[string]interface{}{}, "StartTime": fixedTimestamp},
				{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "exp", "Count": 3, "Sum": 15.0, "Buckets": []float64{2, 3, 4}, "Metadata": map[string]interface{}{}, "StartTime": fixedTimestamp},
			},
			ExpectStatus: 200,
			ExpectIDs:    []string{"any", "any"},
		},
		{
			Name:          "Query batch metrics (exponential_histogram)",
			Method:        "GET",
			Endpoint:      "/api/v1/metrics",
			QueryParams:   map[string]string{"service": "api", "type": "exponential_histogram", "limit": "10"},
			ExpectStatus:  200,
			ExpectEntries: 2,
		},
		// Negative tests for profiles
		{
			Name:         "Insert profile missing required field",
			Method:       "POST",
			Endpoint:     "/api/v1/profiles",
			Payload:      map[string]interface{}{"ServiceName": "api", "ProfileType": "cpu"},
			ExpectStatus: 400,
			ExpectError:  true,
		},
		// Negative tests for spans
		{
			Name:         "Insert trace missing required field",
			Method:       "POST",
			Endpoint:     "/api/v1/spans",
			Payload:      map[string]interface{}{"SpanID": "def", "Name": "span", "ServiceName": "api"},
			ExpectStatus: 400,
			ExpectError:  true,
		},
		// Negative tests for metrics
		{
			Name:         "Insert metric missing required field",
			Method:       "POST",
			Endpoint:     "/api/v1/metrics?type=gauge",
			Payload:      map[string]interface{}{"ServiceName": "api", "Name": "cpu"},
			ExpectStatus: 400,
			ExpectError:  true,
		},
		{
			Name:         "Insert metric with invalid type",
			Method:       "POST",
			Endpoint:     "/api/v1/metrics?type=invalidtype",
			Payload:      map[string]interface{}{"Timestamp": fixedTimestamp, "ServiceName": "api", "Name": "cpu", "Value": 1.23},
			ExpectStatus: 400,
			ExpectError:  true,
		},
		// --- VectorStore Search Scenarios ---
		{
			Name:         "VectorStore search valid",
			Method:       "POST",
			Endpoint:     "/api/v1/vectorstore/search",
			Payload:      map[string]interface{}{"query": "test log", "top_k": 2, "store_id": internal.DefaultSchema},
			ExpectStatus: 200,
			WithJWT:      true,
		},
		{
			Name:         "VectorStore search missing JWT",
			Method:       "POST",
			Endpoint:     "/api/v1/vectorstore/search",
			Payload:      map[string]interface{}{"query": "test", "top_k": 2, "store_id": internal.DefaultSchema},
			ExpectStatus: 401,
			WithJWT:      false,
		},
		{
			Name:         "VectorStore search invalid store_id",
			Method:       "POST",
			Endpoint:     "/api/v1/vectorstore/search",
			Payload:      map[string]interface{}{"query": "test", "top_k": 2, "store_id": "doesnotexist"},
			ExpectStatus: 500,
			WithJWT:      true,
		},
	}

	apiBase := getAPIBaseURL()
	jwt := getTestJWT(t)

	for _, tc := range testCases {
		t.Run(tc.Name, func(t *testing.T) {
			url := apiBase + tc.Endpoint
			if len(tc.QueryParams) > 0 {
				first := true
				for k, v := range tc.QueryParams {
					if first {
						url += "?"
						first = false
					} else {
						url += "&"
					}
					url += fmt.Sprintf("%s=%s", k, v)
				}
			}
			var req *http.Request
			var bodyBytes []byte
			if tc.Method == "POST" {
				bodyBytes, _ = json.Marshal(tc.Payload)
				req, _ = http.NewRequest(tc.Method, url, bytes.NewReader(bodyBytes))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req, _ = http.NewRequest(tc.Method, url, nil)
			}
			if tc.WithJWT || (tc.WithJWT == false && tc.Name != "Insert log with missing JWT") {
				req.Header.Set("Authorization", jwt)
			}
			t.Logf("Request: %s %s", tc.Method, url)
			t.Logf("Headers: %v", req.Header)
			if tc.Method == "POST" {
				t.Logf("Body: %s", string(bodyBytes))
			}
			resp, err := http.DefaultClient.Do(req)
			defer func() {
				if resp != nil {
					resp.Body.Close()
				}
			}()
			require.NoError(t, err)
			if resp.StatusCode != tc.ExpectStatus {
				respBody, _ := io.ReadAll(resp.Body)
				t.Logf("Response status: %d", resp.StatusCode)
				t.Logf("Response body: %s", string(respBody))
			}
			require.Equal(t, tc.ExpectStatus, resp.StatusCode)
			if tc.ExpectStatus == 200 && tc.Method == "POST" {
				var result struct {
					IDs    []string `json:"IDs"`
					Status string   `json:"Status"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&result)
				require.NotEmpty(t, result.IDs)
			}
			if tc.ExpectStatus == 200 && tc.Method == "GET" && tc.ExpectEntries > 0 {
				var result struct {
					Entries []interface{} `json:"Entries"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&result)
				require.GreaterOrEqual(t, len(result.Entries), tc.ExpectEntries)
			}
			// Add more assertions for error, batch, pagination, etc.
			if tc.Name == "VectorStore search valid" {
				// Retry loop for async embedding
				var lastRespBody []byte
				for i := 0; i < 10; i++ {
					resp, err := http.Post(apiBase+tc.Endpoint, "application/json", bytes.NewReader(bodyBytes))
					if err != nil {
						t.Fatalf("POST failed: %v", err)
					}
					lastRespBody, _ = io.ReadAll(resp.Body)
					resp.Body.Close()
					var result struct{ Results []interface{} }
					_ = json.Unmarshal(lastRespBody, &result)
					if len(result.Results) > 0 {
						break
					}
					time.Sleep(200 * time.Millisecond)
				}
				var result struct{ Results []interface{} }
				_ = json.Unmarshal(lastRespBody, &result)
				if len(result.Results) == 0 {
					t.Fatalf("VectorStore search returned no results after retries")
				}
				return
			}
			if tc.PostCheck != nil {
				tc.PostCheck(t, apiBase, jwt)
			}
		})
	}
}

// --- WebSocket Streaming Test Template ---

func TestLogStreaming(t *testing.T) {
	requireExtensionIntegration(t)
	jwt := getTestJWT(t)
	serviceName := "api-log-stream-test"

	wsURL, err := url.Parse(wsURLFromHTTP(getAPIBaseURL()))
	require.NoError(t, err)
	wsURL.Path = "/api/v1/stream/logs"
	wsURL.RawQuery = "wait_for_established=1"
	header := make(http.Header)
	header.Set("Authorization", jwt)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), header)
	require.NoError(t, err, "WebSocket dial failed")
	defer conn.Close()

	// Insert the log AFTER opening the WebSocket
	apiBase := getAPIBaseURL()
	payload := map[string]interface{}{"Timestamp": "2025-07-10T14:04:10Z", "ServiceName": serviceName, "Body": "streamed log", "Metadata": map[string]interface{}{}}
	bodyBytes, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", apiBase+"/api/v1/logs", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", jwt)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "Failed to insert log")
	defer resp.Body.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err, "WebSocket read failed")
	require.Contains(t, string(msg), "streamed log")
	require.Contains(t, string(msg), serviceName)
}
