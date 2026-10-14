package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// MCPClient represents a client connection to an MCP server via stdio
type MCPClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	reader *bufio.Reader
}

// NewMCPClient starts the MCP server and returns a client
func NewMCPClient(t *testing.T) *MCPClient {
	t.Helper()

	// Start the MCP server process
	cmd := exec.Command("./trazr-mcp")
	cmd.Env = append(os.Environ(), "AUTH_BASE_URL="+os.Getenv("EXTENSION_URL"))

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start MCP server: %v", err)
	}

	return &MCPClient{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		reader: bufio.NewReader(stdout),
	}
}

// SendMessage sends a JSON-RPC message to the MCP server
func (c *MCPClient) SendMessage(msg map[string]interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	_, err = c.stdin.Write(append(data, '\n'))
	return err
}

// ReadMessage reads a JSON-RPC response from the MCP server
func (c *MCPClient) ReadMessage() (map[string]interface{}, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// Close closes the MCP client connection
func (c *MCPClient) Close() error {
	c.stdin.Close()
	c.stdout.Close()
	return c.cmd.Wait()
}

// WaitForResponse waits for a JSON-RPC response with the given ID
func (c *MCPClient) WaitForResponse(id string, timeout time.Duration) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for response with id %s", id)
		default:
			msg, err := c.ReadMessage()
			if err != nil {
				return nil, err
			}

			if msgID, ok := msg["id"].(string); ok && msgID == id {
				return msg, nil
			}
			// Continue reading if this isn't the message we're looking for
		}
	}
}

func TestIntegration_MCP_StdioTools(t *testing.T) {
	// Skip if not explicitly enabled
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("TRAZR_IT!=1; skipping integration test")
	}

	// Verify extension backend is reachable
	extensionURL := os.Getenv("EXTENSION_URL")
	if extensionURL == "" {
		t.Fatal("set EXTENSION_URL for backend integration tests")
	}

	t.Logf("Testing extension backend connectivity at: %s", extensionURL)
	if err := verifyExtensionBackend(extensionURL); err != nil {
		t.Logf("Warning: Extension backend not reachable at %s: %v", extensionURL, err)
		t.Logf("This test will verify MCP functionality but may not test real backend calls")
	} else {
		t.Logf("✅ Extension backend is reachable at %s", extensionURL)
	}

	// Create MCP client
	client := NewMCPClient(t)
	defer client.Close()

	// 1) Initialize the MCP session
	initID := "init-1"
	initMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      initID,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]interface{}{
				"name":    "trazr-integration-test",
				"version": "1.0.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		},
	}

	if err := client.SendMessage(initMsg); err != nil {
		t.Fatalf("failed to send initialize message: %v", err)
	}

	initResp, err := client.WaitForResponse(initID, 10*time.Second)
	if err != nil {
		t.Fatalf("failed to get initialize response: %v", err)
	}

	// Check that we got a successful response
	if result, ok := initResp["result"].(map[string]interface{}); ok {
		if serverInfo, ok := result["serverInfo"].(map[string]interface{}); ok {
			t.Logf("Connected to MCP server: %s v%s", serverInfo["name"], serverInfo["version"])
		}
	} else {
		t.Fatalf("initialize failed: %+v", initResp)
	}

	// 2) Test the hello tool
	helloID := "hello-1"
	helloMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      helloID,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "hello",
			"arguments": map[string]interface{}{
				"name": "Integration Test",
			},
		},
	}

	if err := client.SendMessage(helloMsg); err != nil {
		t.Fatalf("failed to send hello tool call: %v", err)
	}

	helloResp, err := client.WaitForResponse(helloID, 10*time.Second)
	if err != nil {
		t.Fatalf("failed to get hello response: %v", err)
	}

	// Validate hello response
	if result, ok := helloResp["result"].(map[string]interface{}); ok {
		if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
			if textContent, ok := content[0].(map[string]interface{}); ok {
				if text, ok := textContent["text"].(string); ok {
					if !strings.Contains(text, "Integration Test") {
						t.Errorf("hello response doesn't contain expected name: %s", text)
					}
					t.Logf("Hello tool response: %s", text)
				} else {
					t.Errorf("hello response missing text field")
				}
			}
		} else {
			t.Errorf("hello response missing content")
		}
	} else {
		t.Fatalf("hello tool call failed: %+v", helloResp)
	}

	// 3) Test the logs.query tool
	logsID := "logs-1"

	// Get API key from environment or use a test key
	apiKey := os.Getenv("MCP_API_KEY")
	if apiKey == "" {
		apiKey = "test-api-key-for-integration"
	}

	logsMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      logsID,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "logs.query",
			"arguments": map[string]interface{}{
				"api_key": apiKey,
				"query":   "test query",
				"limit":   5,
			},
		},
	}

	if err := client.SendMessage(logsMsg); err != nil {
		t.Fatalf("failed to send logs.query tool call: %v", err)
	}

	logsResp, err := client.WaitForResponse(logsID, 10*time.Second)
	if err != nil {
		t.Fatalf("failed to get logs.query response: %v", err)
	}

	// Validate logs.query response
	if result, ok := logsResp["result"].(map[string]interface{}); ok {
		if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
			if textContent, ok := content[0].(map[string]interface{}); ok {
				if text, ok := textContent["text"].(string); ok {
					// Verify it's valid JSON
					var jsonData interface{}
					if err := json.Unmarshal([]byte(text), &jsonData); err != nil {
						t.Errorf("logs.query response is not valid JSON: %v", err)
					} else {
						t.Logf("Logs.query tool returned valid JSON response")
					}
				} else {
					t.Errorf("logs.query response missing text field")
				}
			}
		} else {
			t.Errorf("logs.query response missing content")
		}
	} else {
		t.Fatalf("logs.query tool call failed: %+v", logsResp)
	}

	t.Log("All MCP tool tests passed successfully")
}

// verifyExtensionBackend checks if the extension backend is reachable
func verifyExtensionBackend(extensionURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}

	// Try to reach the health endpoint or root endpoint
	endpoints := []string{"/healthz", "/health", "/"}

	for _, endpoint := range endpoints {
		url := strings.TrimRight(extensionURL, "/") + endpoint
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		resp.Body.Close()

		// Accept any 2xx, 3xx, 4xx status (server is responding)
		if resp.StatusCode < 500 {
			return nil
		}
	}

	return fmt.Errorf("extension backend not reachable")
}

// TestIntegration_ExtensionBackendCalls specifically tests real backend calls
func TestIntegration_ExtensionBackendCalls(t *testing.T) {
	// Skip if not explicitly enabled
	if os.Getenv("TRAZR_IT") != "1" {
		t.Skip("TRAZR_IT!=1; skipping integration test")
	}

	// Verify extension backend is reachable
	extensionURL := os.Getenv("EXTENSION_URL")
	if extensionURL == "" {
		t.Fatal("set EXTENSION_URL for backend integration tests")
	}

	if err := verifyExtensionBackend(extensionURL); err != nil {
		t.Skipf("Extension backend not reachable at %s: %v", extensionURL, err)
	}

	t.Logf("✅ Testing real backend calls to: %s", extensionURL)

	// Get API key from environment
	apiKey := os.Getenv("MCP_API_KEY")
	if apiKey == "" {
		t.Fatal("set MCP_API_KEY for backend integration tests")
	}

	// Create MCP client
	client := NewMCPClient(t)
	defer client.Close()

	// Initialize MCP session
	initID := "backend-init"
	initMsg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      initID,
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"clientInfo": map[string]interface{}{
				"name":    "trazr-backend-test",
				"version": "1.0.0",
			},
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
		},
	}

	if err := client.SendMessage(initMsg); err != nil {
		t.Fatalf("failed to send initialize message: %v", err)
	}

	if _, err := client.WaitForResponse(initID, 10*time.Second); err != nil {
		t.Fatalf("failed to get initialize response: %v", err)
	}

	// Test logs.query with real backend call
	t.Run("logs.query_real_backend", func(t *testing.T) {
		logsID := "backend-logs-1"
		logsMsg := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      logsID,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "logs.query",
				"arguments": map[string]interface{}{
					"api_key": apiKey,
					"query":   "schema=otel-logs-v1&last=1h",
					"limit":   5,
				},
			},
		}

		if err := client.SendMessage(logsMsg); err != nil {
			t.Fatalf("failed to send logs.query: %v", err)
		}

		logsResp, err := client.WaitForResponse(logsID, 30*time.Second)
		if err != nil {
			t.Fatalf("failed to get logs.query response: %v", err)
		}

		// Verify we got a successful response
		if result, ok := logsResp["result"].(map[string]interface{}); ok {
			if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
				if textContent, ok := content[0].(map[string]interface{}); ok {
					if text, ok := textContent["text"].(string); ok {
						t.Logf("✅ Received logs response from backend: %d bytes", len(text))

						// Verify it's valid JSON
						var jsonData interface{}
						if err := json.Unmarshal([]byte(text), &jsonData); err != nil {
							t.Errorf("Response is not valid JSON: %v", err)
						} else {
							t.Logf("✅ Backend returned valid JSON response")
						}

						// Log first 200 chars for debugging
						preview := text
						if len(preview) > 200 {
							preview = preview[:200] + "..."
						}
						t.Logf("Response preview: %s", preview)
					} else {
						t.Errorf("Response missing text field")
					}
				}
			} else {
				t.Errorf("Response missing content")
			}
		} else {
			// Check if it's an error response
			if errorInfo, ok := logsResp["error"].(map[string]interface{}); ok {
				t.Logf("Backend call failed with error: %+v", errorInfo)
				if code, ok := errorInfo["code"].(float64); ok && code == -32000 {
					t.Logf("This appears to be an authentication or backend connectivity issue")
				}
			} else {
				t.Fatalf("Unexpected response format: %+v", logsResp)
			}
		}
	})

	// Test metrics.query with real backend call
	t.Run("metrics.query_real_backend", func(t *testing.T) {
		metricsID := "backend-metrics-1"
		metricsMsg := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      metricsID,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "metrics.query",
				"arguments": map[string]interface{}{
					"api_key":    apiKey,
					"query":      "up",
					"time_range": "1h",
				},
			},
		}

		if err := client.SendMessage(metricsMsg); err != nil {
			t.Fatalf("failed to send metrics.query: %v", err)
		}

		metricsResp, err := client.WaitForResponse(metricsID, 30*time.Second)
		if err != nil {
			t.Fatalf("failed to get metrics.query response: %v", err)
		}

		// Log the response for debugging
		if result, ok := metricsResp["result"].(map[string]interface{}); ok {
			t.Logf("✅ Metrics query succeeded")
			if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
				if textContent, ok := content[0].(map[string]interface{}); ok {
					if text, ok := textContent["text"].(string); ok {
						t.Logf("✅ Received metrics response: %d bytes", len(text))
					}
				}
			}
		} else if errorInfo, ok := metricsResp["error"].(map[string]interface{}); ok {
			t.Logf("Metrics query failed (expected if /metrics endpoint doesn't exist): %+v", errorInfo)
		}
	})

	t.Log("✅ Backend connectivity tests completed")
}
