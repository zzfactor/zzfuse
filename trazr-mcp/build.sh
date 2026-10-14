#!/bin/bash
# Build optimized MCP server binary for fast startup

set -e

echo "Building optimized TRAZR MCP server..."

# Build with optimizations for fast startup
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
    -ldflags="-s -w" \
    -trimpath \
    -o trazr-mcp-darwin-arm64 \
    main.go

echo "Built: trazr-mcp-darwin-arm64"
echo "Size: $(du -h trazr-mcp-darwin-arm64 | cut -f1)"

# Make executable
chmod +x trazr-mcp-darwin-arm64

echo "Ready for Claude Desktop configuration!"
