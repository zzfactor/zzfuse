# TRAZR MCP Server

This service implements a Model Context Protocol (MCP) server for the TRAZR platform using [mark3labs/mcp-go](https://github.com/mark3labs/mcp-go).

## Features
- Exposes tools and resources to LLMs and agents via MCP (HTTP/SSE or STDIO)
- Example tool: `hello` (greets a user by name)
- Easily extensible: add your own tools/resources in `main.go` or via packages
- TLS/HTTPS support, API key authentication, rate limiting, and graceful shutdown
- **Unified configuration**: supports command-line flags, environment variables, and YAML/JSON config files (via [Viper](https://github.com/spf13/viper))

## Configuration Precedence

TRAZR MCP uses a unified configuration system (via Viper) that supports:
- **Command-line flags** (highest precedence)
- **Environment variables**
- **Config file** (`--config` flag, YAML or JSON)
- **Default values** (lowest precedence)

**Precedence order:**
1. Command-line flags (e.g. `--addr=:8082`)
2. Environment variables (e.g. `ADDR=":8081"`)
3. Config file (YAML/JSON)
4. Built-in defaults

This is the recommended MCP/cloud-native best practice for flexible, 12-factor configuration.

## Flags, Environment Variables, and Config File

| Flag           | Description                                              |
|----------------|----------------------------------------------------------|
| --config       | Path to YAML/JSON config file (optional)                |
| --sse          | Enable HTTP/SSE mode (default: STDIO)                   |
| --addr         | Address to listen on (default: :8080)                   |
| --tls          | Enable HTTPS (TLS)                                      |
| --tls-cert     | Path to TLS certificate file                            |
| --tls-key      | Path to TLS private key file                            |
| --api-key      | API key for HTTP/HTTPS endpoints (or set API_KEY env)   |
| --rate-limit   | Requests/sec per client IP (default: 10)                |
| --rate-burst   | Burst size for rate limiting (default: 20)              |

You can set any config value via **flag**, **environment variable** (e.g. `ADDR`, `API_KEY`), or **config file**. Flags always override env/config file.

### Example config.yaml
```yaml
addr: ":9000"
sse: true
tls: false
api-key: "mysecret"
rate-limit: 20
rate-burst: 40
```

### Example Usage

#### Using a config file
```
./trazr-mcp --config config.yaml
```

#### Overriding with environment variables
```
ADDR=":8081" API_KEY=anotherkey ./trazr-mcp --config config.yaml
```

#### Overriding with command-line flags
```
./trazr-mcp --config config.yaml --addr=:8082 --api-key=flagkey
```

**Result:** The value from the highest-precedence source is used for each setting.

### Docker Compose
Add this service to your `docker-compose.yml`:
```yaml
  mcpserver:
    build:
      context: .
      dockerfile: trazr-mcp/Dockerfile
    ports:
      - "8080:8080"
    environment:
      - API_KEY=mysecret
      - ADDR=:8080
    volumes:
      - ./config.yaml:/app/config.yaml
    command: ["--config", "config.yaml"]
```

## Security & Best Practices
- **Always use HTTPS in production.**
- **Set a strong API key for all deployments.**
- **Rate limiting is enabled by default.**
- The server supports graceful shutdown on SIGINT/SIGTERM.
- For advanced auth (JWT/OAuth2), see `main.go` for extension points.
- **Configuration precedence** (flags > env > config file > defaults) is enforced for all settings.

## Observability

- **Prometheus metrics** are available at `/metrics` for monitoring request rates, durations, and tool usage.
- **Distributed tracing** is enabled via OpenTelemetry; configure the collector endpoint with `--otel-endpoint` or the `otel-endpoint` config value.

## Vector Store Backends

Supported backends: `local`, `pgvector`, `qdrant`, and more (see code for latest list).
Configure the backend in your config file:

```yaml
vector_store_provider: "local"
vector_store_table: "embeddings"
```

## Adding Tools

To add a new tool, edit `main.go` and use the `mcp.NewTool` and `s.AddTool` pattern:

```go
myTool := mcp.NewTool("mytool", mcp.WithDescription("My custom tool"))
s.AddTool(myTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    // Tool logic here
    return mcp.NewToolResultText("Hello from mytool!"), nil
})
```

## Testing

Run all tests with:
```
go test ./...
```

## Production Deployment

- Use the provided health (`/healthz`) and readiness (`/readyz`) endpoints for Kubernetes probes.
- Set resource limits and requests in your deployment manifests for reliability.
- Use the Helm chart (if available) for easy Kubernetes deployment.
- Always enable HTTPS and set a strong API key in production.

## Health & Readiness Endpoints

The MCP server exposes standard endpoints for health and readiness checks:

| Endpoint    | ProcessingMode         | Response Example                          |
|-------------|----------------|-------------------------------------------|
| `/healthz`  | Liveness probe  | `{ "status": "ok" }` (always 200 OK)     |
| `/readyz`   | Readiness probe | `{ "status": "ready" }` (200 OK if ready)<br>`{ "status": "unhealthy", "error": "..." }` (503 if not ready) |

- `/healthz` is suitable for Kubernetes liveness probes and always returns 200 OK.
- `/readyz` checks the vector store's health (if supported) and returns 200 OK if healthy, 503 otherwise.
- Both endpoints are instrumented with Prometheus metrics and OpenTelemetry tracing for observability.

**Example Kubernetes probe config:**
```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

## License
Apache 2.0. See the [repository license](../LICENSE).
