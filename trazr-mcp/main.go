package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	authclient "github.com/medxops/trazr-otel/pkg/auth/client"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// helloHandler implements a simple greeting tool
func helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Printf("--> helloHandler: Received tool call with arguments: %+v", request.GetArguments())

	// Extract name from arguments if provided
	name := request.GetString("name", "World")

	message := fmt.Sprintf("Hello, %s! This is a response from the TRAZR MCP server at %s.", 
		name, time.Now().Format(time.RFC3339))

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: message,
			},
		},
	}, nil
}

// AuthService handles authentication for all MCP tools
type AuthService struct {
	tokenProvider *authclient.TokenProvider
	extensionURL  string
}

// NewAuthService creates a new authentication service
func NewAuthService() *AuthService {
	// Get auth service URL from environment
	authURL := os.Getenv("AUTH_BASE_URL")
	if authURL == "" {
		authURL = "http://localhost:18888" // Default to extension URL for auth
	}

	// Create token provider with caching and 30s refresh skew
	tokenProvider := authclient.New(authURL, nil, 30*time.Second)

	extensionURL := os.Getenv("EXTENSION_URL")
	if extensionURL == "" {
		extensionURL = "http://localhost:18888"
	}

	return &AuthService{
		tokenProvider: tokenProvider,
		extensionURL:  extensionURL,
	}
}

// GetToken exchanges an API key for a JWT token with caching
func (as *AuthService) GetToken(ctx context.Context, apiKey string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("API key is required")
	}

	token, err := as.tokenProvider.GetToken(ctx, apiKey)
	if err != nil {
		return "", fmt.Errorf("failed to exchange API key for token: %w", err)
	}

	return token, nil
}

// MakeAuthenticatedRequest makes an HTTP request with proper authentication
func (as *AuthService) MakeAuthenticatedRequest(ctx context.Context, apiKey, method, endpoint string, body io.Reader) (*http.Response, error) {
	// Get cached/fresh token
	token, err := as.GetToken(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	// Construct full URL
	fullURL := strings.TrimRight(as.extensionURL, "/") + endpoint

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication and headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "trazr-mcp/1.0.0")

	// Make request with timeout
	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

// logsQueryHandler implements the logs.query tool with real extension backend calls
func logsQueryHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Printf("--> logsQueryHandler: Received tool call with arguments: %+v", request.GetArguments())

	// Extract parameters
	apiKey := request.GetString("api_key", "")
	query := request.GetString("query", "")
	limit := request.GetInt("limit", 10)

	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}

	log.Printf("Processing logs query: %s (limit: %d)", query, limit)

	// Use shared auth service
	authService := NewAuthService()
	
	// Build query parameters
	params := url.Values{}
	if query != "" {
		if strings.Contains(query, "=") {
			queryParams, err := url.ParseQuery(query)
			if err == nil {
				for k, v := range queryParams {
					if len(v) > 0 {
						params.Set(k, v[0])
					}
				}
			}
		} else {
			params.Set("q", query)
		}
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}

	// Build endpoint with query parameters
	endpoint := "/logs"
	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	// Make authenticated request
	resp, err := authService.MakeAuthenticatedRequest(ctx, apiKey, "GET", endpoint, nil)
	if err != nil {
		log.Printf("Failed to query extension logs: %v", err)
		// Return error response instead of crashing
		errorMsg := fmt.Sprintf("Authentication or connection failed: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "%s", "query": "%s", "limit": %d}`, errorMsg, query, limit),
				},
			},
		}, nil
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read response body: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Failed to read response: %v", "query": "%s"}`, err, query),
				},
			},
		}, nil
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		log.Printf("Extension returned error status %d: %s", resp.StatusCode, string(body))
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Extension returned status %d", "response": "%s", "query": "%s"}`, 
						resp.StatusCode, string(body), query),
				},
			},
		}, nil
	}

	// Parse and return JSON response
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("Failed to parse JSON response: %v", err)
		// Return raw response if JSON parsing fails
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(body),
				},
			},
		}, nil
	}

	// Convert back to JSON string for MCP response
	jsonData, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(body), // Return original response
				},
			},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

// metricsQueryHandler demonstrates how to add additional authenticated endpoints
func metricsQueryHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Printf("--> metricsQueryHandler: Received tool call with arguments: %+v", request.GetArguments())

	// Extract parameters
	apiKey := request.GetString("api_key", "")
	query := request.GetString("query", "")
	timeRange := request.GetString("time_range", "1h")

	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}

	log.Printf("Processing metrics query: %s (time_range: %s)", query, timeRange)

	// Use shared auth service
	authService := NewAuthService()
	
	// Build query parameters
	params := url.Values{}
	if query != "" {
		params.Set("query", query)
	}
	params.Set("time_range", timeRange)

	// Build endpoint
	endpoint := "/metrics?" + params.Encode()

	// Make authenticated request
	resp, err := authService.MakeAuthenticatedRequest(ctx, apiKey, "GET", endpoint, nil)
	if err != nil {
		log.Printf("Failed to query metrics: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Failed to query metrics: %v", "query": "%s"}`, err, query),
				},
			},
		}, nil
	}
	defer resp.Body.Close()

	// Read and parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Failed to read response: %v"}`, err),
				},
			},
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Metrics endpoint returned status %d: %s", resp.StatusCode, string(body))
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Metrics endpoint returned status %d", "response": "%s"}`, resp.StatusCode, string(body)),
				},
			},
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(body), // Return raw response
				},
			},
		}, nil
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(body),
				},
			},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

// tracesQueryHandler demonstrates another authenticated endpoint
func tracesQueryHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Printf("--> tracesQueryHandler: Received tool call with arguments: %+v", request.GetArguments())

	apiKey := request.GetString("api_key", "")
	traceID := request.GetString("trace_id", "")
	serviceFilter := request.GetString("service", "")

	if apiKey == "" {
		return nil, fmt.Errorf("api_key is required")
	}

	authService := NewAuthService()
	
	// Build endpoint based on parameters
	var endpoint string
	if traceID != "" {
		endpoint = fmt.Sprintf("/traces/%s", traceID)
	} else {
		params := url.Values{}
		if serviceFilter != "" {
			params.Set("service", serviceFilter)
		}
		endpoint = "/traces"
		if len(params) > 0 {
			endpoint += "?" + params.Encode()
		}
	}

	resp, err := authService.MakeAuthenticatedRequest(ctx, apiKey, "GET", endpoint, nil)
	if err != nil {
		log.Printf("Failed to query traces: %v", err)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Failed to query traces: %v", "trace_id": "%s", "service": "%s"}`, err, traceID, serviceFilter),
				},
			},
		}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Failed to read response: %v"}`, err),
				},
			},
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Traces endpoint returned status %d: %s", resp.StatusCode, string(body))
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf(`{"error": "Traces endpoint returned status %d", "response": "%s"}`, resp.StatusCode, string(body)),
				},
			},
		}, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(body), // Return raw response
				},
			},
		}, nil
	}

	jsonData, err := json.Marshal(result)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: string(body),
				},
			},
		}, nil
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: string(jsonData),
			},
		},
	}, nil
}

func main() {
	// Create MCP server
	mcpServer := server.NewMCPServer(
		"trazr-mcp",
		"1.0.0",
		server.WithToolCapabilities(true),
		server.WithLogging(),
	)

	// Register tools
	mcpServer.AddTool(mcp.NewTool("hello",
		mcp.WithDescription("A simple greeting tool"),
		mcp.WithString("name",
			mcp.Description("Name to greet"),
		),
	), helloHandler)

	mcpServer.AddTool(mcp.NewTool("logs.query",
		mcp.WithDescription("Query logs from the TRAZR system"),
		mcp.WithString("api_key",
			mcp.Description("API key for authentication"),
			mcp.Required(),
		),
		mcp.WithString("query",
			mcp.Description("Log query string"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of logs to return"),
			mcp.DefaultNumber(10),
		),
	), logsQueryHandler)

	// Example: Additional authenticated endpoints
	mcpServer.AddTool(mcp.NewTool("metrics.query",
		mcp.WithDescription("Query metrics from the TRAZR system"),
		mcp.WithString("api_key",
			mcp.Description("API key for authentication"),
			mcp.Required(),
		),
		mcp.WithString("query",
			mcp.Description("Metrics query (e.g., PromQL)"),
		),
		mcp.WithString("time_range",
			mcp.Description("Time range for metrics (e.g., 1h, 24h)"),
			mcp.DefaultString("1h"),
		),
	), metricsQueryHandler)

	mcpServer.AddTool(mcp.NewTool("traces.query",
		mcp.WithDescription("Query traces from the TRAZR system"),
		mcp.WithString("api_key",
			mcp.Description("API key for authentication"),
			mcp.Required(),
		),
		mcp.WithString("trace_id",
			mcp.Description("Specific trace ID to retrieve"),
		),
		mcp.WithString("service",
			mcp.Description("Filter traces by service name"),
		),
	), tracesQueryHandler)

	log.Printf("TRAZR MCP server starting...")
	log.Printf("Available tools: hello, logs.query, metrics.query, traces.query")

	// Use stdio transport (standard for MCP)
	transport := "stdio"
	if envTransport := os.Getenv("MCP_TRANSPORT"); envTransport != "" {
		transport = envTransport
	}

	switch transport {
	case "stdio":
		log.Printf("Starting MCP server with stdio transport...")
		if err := server.ServeStdio(mcpServer); err != nil {
			log.Printf("MCP server stdio error: %v", err)
			log.Fatalf("failed to serve stdio: %v", err)
		}
		log.Printf("MCP server stdio finished")
	default:
		log.Fatalf("unsupported transport: %s", transport)
	}
	
	log.Printf("MCP server exiting")
}
