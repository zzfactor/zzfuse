# Documentation

Maintained guides describe the current repository. Source code, runtime configuration, and OpenAPI definitions take precedence over older design documents.

- [Architecture](architecture.md): modules, resource ownership, tenant boundaries, and extension points.
- [Configuration](configuration.md): runtime settings, services, and credentials.
- [Development](development.md): builds, tests, upgrades, and Collector regeneration.
- [Profiles](profiles.md): versioned storage, API behavior, and legacy compatibility.
- [Embedding and templates](embedding.md): processing modes and content generation.
- [Consolidated history](history.md): earlier decisions and implementation reports.

## Detailed references

These guides preserve earlier Trazr domain explanations and examples. Verify API examples against current handlers and OpenAPI.

- [Authentication](reference/authentication.md), [CLI authentication](reference/cli-authentication.md), and [server](reference/server.md).
- [Streaming clients](reference/streaming.md) and [WebSocket conventions](reference/websockets.md).
- [Event bus](reference/event-bus.md) and [configuration-management design](reference/config-management.md).
- [Templates](reference/templates.md), [fields](reference/template-fields.md), and [setup](reference/template-setup.md).
- [Template plus](reference/template-plus.md), [categorization](reference/template-categorization.md), and [exemplars](reference/template-exemplars.md).
- [Database DDL](../pkg/repository/internal/database/ddl/README.md).
- [Python service](../services/README.md) and [MCP server](../trazr-mcp/README.md).

Older plans, assessments, summaries, and reviews remain in the [archive](archive/README.md). Their completion claims are historical evidence, not current tasks or validation results.
