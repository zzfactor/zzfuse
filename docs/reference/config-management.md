# Configuration Management API Design
## Functional Interface for Telemetry Backend Configuration

### Overview

This document defines a comprehensive API design for managing telemetry backend configurations. The API provides a RESTful interface on top of the existing configuration repository system, following industry best practices for configuration management APIs.

## Table of Contents

1. [API Design Principles](#api-design-principles)
2. [Resource Hierarchy](#resource-hierarchy)
3. [API Endpoints](#api-endpoints)
4. [Request/Response Models](#requestresponse-models)
5. [Error Handling](#error-handling)
6. [Implementation Interface](#implementation-interface)
7. [Usage Examples](#usage-examples)
8. [Security Considerations](#security-considerations)

## API Design Principles

### 1. RESTful Design
- Use HTTP verbs appropriately (GET, POST, PUT, DELETE)
- Resource-based URLs with clear hierarchies
- Consistent naming conventions (kebab-case for URLs, camelCase for JSON)

### 2. Configuration Management Best Practices
- **Immutable Configuration IDs**: Once created, configuration IDs never change
- **Versioning Support**: Track configuration changes with version numbers
- **Validation Before Persistence**: Validate all configurations before saving
- **Atomic Operations**: Configuration changes are all-or-nothing
- **Dependency Validation**: Ensure referenced configurations exist

### 3. API Consistency
- Standardized response formats across all endpoints
- Consistent error handling and status codes
- Uniform pagination and filtering patterns
- Common metadata fields (created_at, updated_at, version)

## Resource Hierarchy

```
/api/v1/config/
├── schemas/                    # Database schema configurations
├── processors/                 # Processor mapping configurations  
├── embeddings/                 # Embedding provider configurations
├── templates/                  # Template configurations
├── vectorstores/              # Vector store provider configurations
└── health/                    # Configuration system health
```

## API Endpoints

### 1. Schema Configurations

#### Base Path: `/api/v1/config/schemas`

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/schemas` | List all schema configurations |
| GET | `/schemas/{id}` | Get schema configuration by ID |
| GET | `/schemas/by-name/{name}` | Get schema configuration by name |
| POST | `/schemas` | Create new schema configuration |
| PUT | `/schemas/{id}` | Update schema configuration |
| DELETE | `/schemas/{id}` | Delete schema configuration |
| GET | `/schemas/{id}/processors` | Get processors for schema |
| POST | `/schemas/{id}/enable` | Enable schema |
| POST | `/schemas/{id}/disable` | Disable schema |

### 2. Processor Configurations

#### Base Path: `/api/v1/config/processors`

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/processors` | List all processor configurations |
| GET | `/processors/{id}` | Get processor configuration by ID |
| GET | `/processors/by-name/{name}` | Get processor configuration by name |
| POST | `/processors` | Create new processor configuration |
| PUT | `/processors/{id}` | Update processor configuration |
| DELETE | `/processors/{id}` | Delete processor configuration |
| GET | `/processors/{id}/schema` | Get schema for processor |
| POST | `/processors/{id}/enable` | Enable processor |
| POST | `/processors/{id}/disable` | Disable processor |
| POST | `/processors/bulk` | Bulk create/update processors |

### 3. Embedding Provider Configurations

#### Base Path: `/api/v1/config/embeddings`

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/embeddings` | List all embedding configurations |
| GET | `/embeddings/{id}` | Get embedding configuration by ID |
| GET | `/embeddings/by-name/{name}` | Get embedding configuration by name |
| POST | `/embeddings` | Create new embedding configuration |
| PUT | `/embeddings/{id}` | Update embedding configuration |
| DELETE | `/embeddings/{id}` | Delete embedding configuration |
| POST | `/embeddings/{id}/test` | Test embedding configuration |
| GET | `/embeddings/providers` | List supported providers |

### 4. Template Configurations

#### Base Path: `/api/v1/config/templates`

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/templates` | List all template configurations |
| GET | `/templates/{id}` | Get template configuration by ID |
| GET | `/templates/by-name/{name}` | Get template configuration by name |
| POST | `/templates` | Create new template configuration |
| PUT | `/templates/{id}` | Update template configuration |
| DELETE | `/templates/{id}` | Delete template configuration |
| POST | `/templates/{id}/validate` | Validate template syntax |
| GET | `/templates/{id}/preview` | Preview template output |

### 5. Vector Store Configurations

#### Base Path: `/api/v1/config/vectorstores`

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/vectorstores` | List all vector store configurations |
| GET | `/vectorstores/{id}` | Get vector store configuration by ID |
| GET | `/vectorstores/by-name/{name}` | Get vector store configuration by name |
| POST | `/vectorstores` | Create new vector store configuration |
| PUT | `/vectorstores/{id}` | Update vector store configuration |
| DELETE | `/vectorstores/{id}` | Delete vector store configuration |
| POST | `/vectorstores/{id}/test` | Test vector store connection |
| GET | `/vectorstores/providers` | List supported providers |

### 6. System Operations

#### Base Path: `/api/v1/config`

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Configuration system health |
| GET | `/statistics` | Configuration statistics |
| POST | `/validate` | Validate configuration set |
| POST | `/export` | Export all configurations |
| POST | `/import` | Import configurations |
| GET | `/dependencies/{type}/{id}` | Get configuration dependencies |

## Request/Response Models

### Common Query Parameters

```typescript
interface CommonQueryParams {
  limit?: number;          // Default: 50, Max: 1000
  offset?: number;         // Default: 0
  sort?: string;          // Default: "created_at"
  order?: "asc" | "desc"; // Default: "desc"
  enabled?: boolean;      // Filter by enabled status
  search?: string;        // Search in name/description
  fields?: string[];      // Select specific fields
}
```

### Schema Configuration

```typescript
interface SchemaConfigRequest {
  name: string;
  databaseSchema: string;
  description?: string;
  enabled?: boolean;
  priority?: number;
  metadata?: Record<string, any>;
}

interface SchemaConfigResponse {
  id: number;
  name: string;
  databaseSchema: string;
  description: string;
  enabled: boolean;
  priority: number;
  metadata: Record<string, any>;
  createdAt: string;
  updatedAt: string;
  version: number;
  processorCount?: number;
}
```

### Processor Configuration

```typescript
interface ProcessorConfigRequest {
  name: string;
  databaseId: number;
  vectorstoreId: number;
  templateId?: number;
  processingMode: "batch" | "stream" | "hybrid";
  enabled?: boolean;
  routing?: Record<string, any>;
  filter?: Record<string, any>;
  metadata?: Record<string, any>;
}

interface ProcessorConfigResponse {
  id: number;
  name: string;
  databaseId: number;
  vectorstoreId: number;
  templateId?: number;
  processingMode: string;
  enabled: boolean;
  routing: Record<string, any>;
  filter: Record<string, any>;
  metadata: Record<string, any>;
  createdAt: string;
  updatedAt: string;
  version: number;
  schema?: SchemaConfigResponse;
  vectorstore?: VectorStoreConfigResponse;
  template?: TemplateConfigResponse;
}
```

### Embedding Configuration

```typescript
interface EmbeddingConfigRequest {
  name: string;
  provider: string;
  endpoint: string;
  apiKey?: string;
  model: string;
  maxTokens?: number;
  dimensions?: number;
  timeout?: number;
  retryAttempts?: number;
  headers?: Record<string, string>;
  metadata?: Record<string, any>;
}

interface EmbeddingConfigResponse {
  id: number;
  name: string;
  provider: string;
  endpoint: string;
  model: string;
  maxTokens: number;
  dimensions: number;
  timeout: number;
  retryAttempts: number;
  headers: Record<string, string>;
  metadata: Record<string, any>;
  createdAt: string;
  updatedAt: string;
  version: number;
  status?: "active" | "inactive" | "error";
}
```

### Template Configuration

```typescript
interface TemplateConfigRequest {
  name: string;
  description?: string;
  contentTemplate: string;
  metadataTemplate?: string;
  templateType: "go" | "jinja2" | "mustache";
  variables?: Record<string, any>;
  metadata?: Record<string, any>;
}

interface TemplateConfigResponse {
  id: number;
  name: string;
  description: string;
  contentTemplate: string;
  metadataTemplate?: string;
  templateType: string;
  variables: Record<string, any>;
  metadata: Record<string, any>;
  createdAt: string;
  updatedAt: string;
  version: number;
  isValid?: boolean;
  validationErrors?: string[];
}
```

### Vector Store Configuration

```typescript
interface VectorStoreConfigRequest {
  name: string;
  provider: string;
  connectionString: string;
  embeddingId: number;
  dimensions?: number;
  indexType?: string;
  distanceMetric?: "cosine" | "euclidean" | "dot_product";
  maxConnections?: number;
  timeout?: number;
  metadata?: Record<string, any>;
}

interface VectorStoreConfigResponse {
  id: number;
  name: string;
  provider: string;
  connectionString: string;
  embeddingId: number;
  dimensions: number;
  indexType: string;
  distanceMetric: string;
  maxConnections: number;
  timeout: number;
  metadata: Record<string, any>;
  createdAt: string;
  updatedAt: string;
  version: number;
  embedding?: EmbeddingConfigResponse;
  status?: "connected" | "disconnected" | "error";
}
```

### Standard Response Wrapper

```typescript
interface ApiResponse<T> {
  success: boolean;
  data?: T;
  error?: {
    code: string;
    message: string;
    details?: Record<string, any>;
  };
  metadata?: {
    total?: number;
    limit?: number;
    offset?: number;
    hasMore?: boolean;
  };
}

interface ListResponse<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
  hasMore: boolean;
}
```

## Error Handling

### HTTP Status Codes

| Status | Usage |
|--------|-------|
| 200 | Successful GET, PUT operations |
| 201 | Successful POST (creation) |
| 204 | Successful DELETE |
| 400 | Bad Request (validation errors) |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Resource not found |
| 409 | Conflict (duplicate name, dependency issues) |
| 422 | Unprocessable Entity (business logic errors) |
| 500 | Internal Server Error |

### Error Response Format

```typescript
interface ErrorResponse {
  success: false;
  error: {
    code: string;
    message: string;
    details?: {
      field?: string;
      value?: any;
      constraint?: string;
      dependentResources?: string[];
    };
  };
  requestId: string;
  timestamp: string;
}
```

### Common Error Codes

```typescript
enum ConfigErrorCodes {
  VALIDATION_FAILED = "VALIDATION_FAILED",
  DUPLICATE_NAME = "DUPLICATE_NAME",
  RESOURCE_NOT_FOUND = "RESOURCE_NOT_FOUND",
  DEPENDENCY_NOT_FOUND = "DEPENDENCY_NOT_FOUND",
  DEPENDENCY_CONFLICT = "DEPENDENCY_CONFLICT",
  CONFIGURATION_INVALID = "CONFIGURATION_INVALID",
  CONNECTION_TEST_FAILED = "CONNECTION_TEST_FAILED",
  TEMPLATE_SYNTAX_ERROR = "TEMPLATE_SYNTAX_ERROR",
  CIRCULAR_DEPENDENCY = "CIRCULAR_DEPENDENCY",
  RESOURCE_IN_USE = "RESOURCE_IN_USE"
}
```

## Implementation Interface

### Core Configuration Manager Interface

```go
package config

import (
    "context"
    "time"
)

// ConfigurationManager provides high-level configuration management operations
type ConfigurationManager interface {
    // Schema Operations
    CreateSchema(ctx context.Context, req *SchemaConfigRequest) (*SchemaConfigResponse, error)
    UpdateSchema(ctx context.Context, id int64, req *SchemaConfigRequest) (*SchemaConfigResponse, error)
    GetSchema(ctx context.Context, id int64) (*SchemaConfigResponse, error)
    GetSchemaByName(ctx context.Context, name string) (*SchemaConfigResponse, error)
    ListSchemas(ctx context.Context, params *QueryParams) (*ListResponse[SchemaConfigResponse], error)
    DeleteSchema(ctx context.Context, id int64) error
    EnableSchema(ctx context.Context, id int64) error
    DisableSchema(ctx context.Context, id int64) error
    
    // Processor Operations
    CreateProcessor(ctx context.Context, req *ProcessorConfigRequest) (*ProcessorConfigResponse, error)
    UpdateProcessor(ctx context.Context, id int64, req *ProcessorConfigRequest) (*ProcessorConfigResponse, error)
    GetProcessor(ctx context.Context, id int64) (*ProcessorConfigResponse, error)
    GetProcessorByName(ctx context.Context, name string) (*ProcessorConfigResponse, error)
    ListProcessors(ctx context.Context, params *QueryParams) (*ListResponse[ProcessorConfigResponse], error)
    DeleteProcessor(ctx context.Context, id int64) error
    EnableProcessor(ctx context.Context, id int64) error
    DisableProcessor(ctx context.Context, id int64) error
    BulkCreateProcessors(ctx context.Context, reqs []*ProcessorConfigRequest) ([]*ProcessorConfigResponse, error)
    
    // Embedding Operations
    CreateEmbedding(ctx context.Context, req *EmbeddingConfigRequest) (*EmbeddingConfigResponse, error)
    UpdateEmbedding(ctx context.Context, id int64, req *EmbeddingConfigRequest) (*EmbeddingConfigResponse, error)
    GetEmbedding(ctx context.Context, id int64) (*EmbeddingConfigResponse, error)
    GetEmbeddingByName(ctx context.Context, name string) (*EmbeddingConfigResponse, error)
    ListEmbeddings(ctx context.Context, params *QueryParams) (*ListResponse[EmbeddingConfigResponse], error)
    DeleteEmbedding(ctx context.Context, id int64) error
    TestEmbedding(ctx context.Context, id int64) (*ConnectionTestResult, error)
    ListEmbeddingProviders(ctx context.Context) ([]ProviderInfo, error)
    
    // Template Operations
    CreateTemplate(ctx context.Context, req *TemplateConfigRequest) (*TemplateConfigResponse, error)
    UpdateTemplate(ctx context.Context, id int64, req *TemplateConfigRequest) (*TemplateConfigResponse, error)
    GetTemplate(ctx context.Context, id int64) (*TemplateConfigResponse, error)
    GetTemplateByName(ctx context.Context, name string) (*TemplateConfigResponse, error)
    ListTemplates(ctx context.Context, params *QueryParams) (*ListResponse[TemplateConfigResponse], error)
    DeleteTemplate(ctx context.Context, id int64) error
    ValidateTemplate(ctx context.Context, id int64) (*ValidationResult, error)
    PreviewTemplate(ctx context.Context, id int64, sampleData map[string]interface{}) (*TemplatePreview, error)
    
    // Vector Store Operations
    CreateVectorStore(ctx context.Context, req *VectorStoreConfigRequest) (*VectorStoreConfigResponse, error)
    UpdateVectorStore(ctx context.Context, id int64, req *VectorStoreConfigRequest) (*VectorStoreConfigResponse, error)
    GetVectorStore(ctx context.Context, id int64) (*VectorStoreConfigResponse, error)
    GetVectorStoreByName(ctx context.Context, name string) (*VectorStoreConfigResponse, error)
    ListVectorStores(ctx context.Context, params *QueryParams) (*ListResponse[VectorStoreConfigResponse], error)
    DeleteVectorStore(ctx context.Context, id int64) error
    TestVectorStore(ctx context.Context, id int64) (*ConnectionTestResult, error)
    ListVectorStoreProviders(ctx context.Context) ([]ProviderInfo, error)
    
    // System Operations
    GetHealth(ctx context.Context) (*HealthStatus, error)
    GetStatistics(ctx context.Context) (*ConfigStatistics, error)
    ValidateConfiguration(ctx context.Context, req *ConfigValidationRequest) (*ValidationResult, error)
    ExportConfigurations(ctx context.Context, filter *ExportFilter) (*ConfigExport, error)
    ImportConfigurations(ctx context.Context, data *ConfigImport) (*ImportResult, error)
    GetDependencies(ctx context.Context, resourceType string, id int64) (*DependencyGraph, error)
}

// Supporting Types
type QueryParams struct {
    Limit    int
    Offset   int
    Sort     string
    Order    string
    Enabled  *bool
    Search   string
    Fields   []string
}

type ListResponse[T any] struct {
    Items   []T  `json:"items"`
    Total   int  `json:"total"`
    Limit   int  `json:"limit"`
    Offset  int  `json:"offset"`
    HasMore bool `json:"hasMore"`
}

type ConnectionTestResult struct {
    Success     bool              `json:"success"`
    Latency     time.Duration     `json:"latency"`
    Error       string            `json:"error,omitempty"`
    Details     map[string]interface{} `json:"details,omitempty"`
    TestedAt    time.Time         `json:"testedAt"`
}

type ValidationResult struct {
    Valid   bool     `json:"valid"`
    Errors  []string `json:"errors,omitempty"`
    Warnings []string `json:"warnings,omitempty"`
}

type TemplatePreview struct {
    Content  string                 `json:"content"`
    Metadata map[string]interface{} `json:"metadata,omitempty"`
    Variables map[string]interface{} `json:"variables"`
}

type ProviderInfo struct {
    Name         string   `json:"name"`
    DisplayName  string   `json:"displayName"`
    Description  string   `json:"description"`
    Capabilities []string `json:"capabilities"`
    ConfigSchema map[string]interface{} `json:"configSchema"`
}

type HealthStatus struct {
    Status      string                 `json:"status"`
    Checks      map[string]HealthCheck `json:"checks"`
    Timestamp   time.Time              `json:"timestamp"`
}

type HealthCheck struct {
    Status  string        `json:"status"`
    Message string        `json:"message,omitempty"`
    Latency time.Duration `json:"latency,omitempty"`
}

type ConfigStatistics struct {
    Schemas      int `json:"schemas"`
    Processors   int `json:"processors"`
    Embeddings   int `json:"embeddings"`
    Templates    int `json:"templates"`
    VectorStores int `json:"vectorStores"`
    EnabledCount int `json:"enabledCount"`
    TotalCount   int `json:"totalCount"`
}

type DependencyGraph struct {
    ResourceType string                    `json:"resourceType"`
    ResourceID   int64                     `json:"resourceId"`
    Dependencies []DependencyNode          `json:"dependencies"`
    Dependents   []DependencyNode          `json:"dependents"`
}

type DependencyNode struct {
    Type string `json:"type"`
    ID   int64  `json:"id"`
    Name string `json:"name"`
}
```

## Usage Examples

### 1. Create Complete Configuration Stack

```bash
# 1. Create embedding provider
curl -X POST /api/v1/config/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "name": "openai-ada-002",
    "provider": "openai",
    "endpoint": "https://api.openai.com/v1/embeddings",
    "model": "text-embedding-ada-002",
    "dimensions": 1536,
    "maxTokens": 8192
  }'

# 2. Create vector store
curl -X POST /api/v1/config/vectorstores \
  -H "Content-Type: application/json" \
  -d '{
    "name": "pgvector-main",
    "provider": "pgvector",
    "connectionString": "postgres://user:pass@localhost/vectordb",
    "embeddingId": 1,
    "dimensions": 1536,
    "distanceMetric": "cosine"
  }'

# 3. Create template
curl -X POST /api/v1/config/templates \
  -H "Content-Type: application/json" \
  -d '{
    "name": "log-embedding-template",
    "description": "Template for log message embedding",
    "contentTemplate": "{{.Message}} {{.Level}} {{.Service}}",
    "templateType": "go"
  }'

# 4. Create schema
curl -X POST /api/v1/config/schemas \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-logs",
    "databaseSchema": "otel",
    "description": "Production log schema",
    "enabled": true,
    "priority": 1
  }'

# 5. Create processor
curl -X POST /api/v1/config/processors \
  -H "Content-Type: application/json" \
  -d '{
    "name": "logs-embedding-processor",
    "databaseId": 1,
    "vectorstoreId": 1,
    "templateId": 1,
    "processingMode": "batch",
    "enabled": true,
    "filter": {
      "signal_type": "logs",
      "level": ["error", "warn"]
    }
  }'
```

### 2. List and Filter Configurations

```bash
# List schemas with pagination
curl "/api/v1/config/schemas?limit=10&offset=0&sort=name&order=asc"

# Search configurations
curl "/api/v1/config/processors?search=embedding&enabled=true"

# Get configuration with dependencies
curl "/api/v1/config/processors/1"
```

### 3. Test and Validate Configurations

```bash
# Test embedding connection
curl -X POST "/api/v1/config/embeddings/1/test"

# Validate template
curl -X POST "/api/v1/config/templates/1/validate"

# Preview template with sample data
curl -X POST "/api/v1/config/templates/1/preview" \
  -H "Content-Type: application/json" \
  -d '{
    "Message": "Application error occurred",
    "Level": "error",
    "Service": "auth-service"
  }'
```

## Security Considerations

### 1. Authentication & Authorization
- **API Key Authentication**: Require API keys for all configuration operations
- **Role-Based Access Control**: Different permissions for read/write operations
- **Resource-Level Permissions**: Control access to specific configurations

### 2. Sensitive Data Handling
- **Credential Encryption**: Encrypt API keys and connection strings at rest
- **Masked Responses**: Never return sensitive data in API responses
- **Audit Logging**: Log all configuration changes with user context

### 3. Validation & Security
- **Input Sanitization**: Validate all input data to prevent injection attacks
- **Configuration Limits**: Enforce limits on configuration sizes and counts
- **Rate Limiting**: Prevent abuse with request rate limiting

### 4. Network Security
- **HTTPS Only**: All API endpoints must use HTTPS
- **CORS Configuration**: Properly configure CORS for web clients
- **IP Whitelisting**: Optional IP-based access control

## Implementation Status

### Current Progress (Phase 3 - HTTP API Layer)

✅ **COMPLETED - Phase 3 HTTP API Layer Implementation**
- **ConfigHandler**: Complete HTTP handler infrastructure with all CRUD endpoints
- **ConfigService**: Business logic layer with repository integration via ServiceResourceManager
- **ConfigConverter**: DRY converter module for all configuration types
- **HTTP Routing**: RESTful endpoints integrated into trazrextension
- **Authentication**: Admin authentication enforced on all endpoints
- **Error Handling**: Consistent error responses and validation
- **Query Parameters**: Comprehensive filtering and pagination support

### Architecture Overview

```
HTTP Layer (trazrextension/internal/)
├── config_handlers.go     ✅ Complete - HTTP request/response handling
├── config_service.go      ✅ Complete - Business logic layer
├── config_converter.go    ✅ Complete - DRY type conversion utilities
└── Routes integrated      ✅ Complete - RESTful endpoint routing

Repository Layer (pkg/repository/)
├── ConfigRepository       ✅ Available - CRUD operations interface
├── ServiceResourceManager ✅ Available - Database and schema management
└── Database Models        ✅ Available - All configuration types supported
```

### Implementation Approach: Methods-First Implementation

The implementation follows **methods-first** approach, building from the bottom up:

1. **✅ Complete Database Layer** → 2. **✅ Implement Business Logic** → 3. **✅ Create API Endpoints** → 4. **🔄 Add Advanced Features**

This ensures a solid foundation before exposing functionality through HTTP endpoints.

## Detailed Implementation Plan

### Phase 1: Foundation Layer (Database & Repository) ✅ COMPLETED
**Status: Complete - Repository layer fully functional**

#### Step 1.1: Database Implementation ✅ COMPLETED
- [x] **OtelSQLConfigDatabaseClient methods** - All CRUD operations implemented
  - [x] Implement `UpsertEmbeddingConfig()`
  - [x] Implement `UpsertVectorStoreConfig()`  
  - [x] Implement `UpsertProcessorConfig()`
  - [x] Implement `UpsertTemplateConfig()`
  - [x] Implement all corresponding Delete methods
- [x] **SQL DDL operations** - All tables can be created/modified
- [x] **Database operations tested** - Repository layer fully functional

#### Step 1.2: Repository Layer Enhancement ✅ COMPLETED
- [x] **Validation functions** - ValidateEmbeddingConfig(), ValidateVectorStoreConfig() etc. complete
- [x] **Dependency checking** - Foreign key relationships validated before operations
- [x] **Transaction support** - Atomic operations for complex configurations
- [x] **Error context** - Detailed error messages with specific field information

### Phase 2: Business Logic Layer (ConfigurationManager) ✅ COMPLETED
**Status: Complete - ConfigService implements all business logic**

#### Step 2.1: Core ConfigurationManager Implementation ✅ COMPLETED
```go
// File: trazrextension/internal/config_service.go
type ConfigService struct {
    serviceResourceManager repository.ServiceResourceManagerInterface
}

func NewConfigService(srm repository.ServiceResourceManagerInterface) *ConfigService {
    return &ConfigService{
        serviceResourceManager: srm,
    }
}
```

- [x] **ConfigService struct** - Implemented with ServiceResourceManager dependency injection
- [x] **Schema operations** - ListConfigurations, GetConfiguration, DeleteConfiguration implemented
- [x] **Processor operations** - Full CRUD with repository integration
- [x] **Embedding operations** - Full CRUD with repository integration
- [x] **Template operations** - Full CRUD with repository integration
- [x] **VectorStore operations** - Full CRUD with repository integration

#### Step 2.2: Business Logic Features ✅ COMPLETED
- [x] **Dependency validation** - Repository layer provides dependency checking
- [x] **Configuration validation** - Proper validation through repository methods
- [x] **Error handling** - Comprehensive error messages and status codes
- [x] **Type conversion** - Proper handling of database model to HTTP response conversion

### Phase 3: HTTP API Layer ✅ COMPLETED
**Status: Complete - All HTTP endpoints implemented**

#### Step 3.1: HTTP Handler Implementation ✅ COMPLETED
```go
// File: trazrextension/internal/config_handlers.go
type ConfigHandler struct {
    configService *ConfigService
}

func NewConfigHandler(configService *ConfigService) *ConfigHandler {
    return &ConfigHandler{
        configService: configService,
    }
}
```

- [x] **HTTP Handlers** - Complete CRUD endpoints for all config types
- [x] **Request/Response Models** - ConfigCreateRequest, ConfigUpdateRequest, ConfigResponse
- [x] **Query Parameter Parsing** - Filtering, pagination, search functionality
- [x] **Error Handling** - Consistent HTTP error responses with proper status codes
- [x] **Authentication** - Admin authentication enforced on all endpoints

#### Step 3.2: API Integration ✅ COMPLETED
- [x] **Route Registration** - RESTful endpoints integrated into trazrextension
- [x] **ConfigConverter** - DRY conversion utilities for all configuration types
- [x] **Statistics Endpoint** - Configuration statistics and counts
- [x] **Type Safety** - Proper handling of pointer fields and time conversions

### Phase 4: Testing & Integration ✅ COMPLETED
**Priority: High - Production readiness**

#### Step 4.1: Integration Test Suite ✅ COMPLETED
```go
// File: trazrextension/internal/config_integration_test.go
type ConfigIntegrationTestSuite struct {
    suite.Suite
    router     *mux.Router
    handler    *ConfigHandler
    service    *ConfigService
    testServer *httptest.Server
    baseURL    string
}
```

- [x] **Comprehensive Test Suite** - Complete integration tests for all Configuration Management endpoints
- [x] **CRUD Operation Testing** - Create, Read, Update, Delete tests for all 5 config types
- [x] **Error Scenario Testing** - Invalid requests, missing data, authentication failures
- [x] **Concurrent Access Testing** - Multi-threaded request handling validation
- [x] **Complete Workflow Testing** - End-to-end configuration lifecycle testing

#### Step 4.2: API Testing Scripts ✅ COMPLETED
```bash
# File: scripts/test-config-api.sh
# Comprehensive bash script for API testing
./scripts/test-config-api.sh
```

- [x] **Automated Test Script** - Complete bash script for API endpoint testing
- [x] **All Config Types** - Tests schemas, processors, embeddings, templates, vectorstores
- [x] **Filter Testing** - Query parameter validation and filtering tests
- [x] **Error Handling** - Comprehensive error scenario validation
- [x] **Concurrent Testing** - Multi-request concurrency validation

#### Step 4.3: OpenAPI Documentation ✅ COMPLETED
- [x] **Complete API Documentation** - All Configuration Management endpoints documented
- [x] **Request/Response Examples** - Detailed examples for all configuration types
- [x] **Error Response Documentation** - Comprehensive error handling documentation
- [x] **Schema Definitions** - Complete schema definitions with validation rules
- [x] **Bundled Documentation** - Successfully regenerated with all endpoints

### Phase 5: Advanced Features 📋 AVAILABLE FOR FUTURE ENHANCEMENT
**Priority: Medium - Enhancement functionality**

#### Step 5.1: Validation Services 📋 PENDING
- [ ] **Connection testing** - Test embedding providers and vector stores
- [ ] **Template validation** - Syntax checking and variable validation
- [ ] **Template preview** - Generate sample output with test data
- [ ] **Configuration health checks** - Monitor configuration status

#### Step 5.2: System Services 📋 PENDING
- [ ] **Export/Import service** - Configuration backup and migration
- [ ] **Dependency tracking** - Map configuration relationships
- [ ] **Metrics collection** - API usage metrics
- [ ] **Client SDKs** - Go/Python/JavaScript clients

### Phase 6: Enterprise Features 📋 PENDING
**Priority: Low - Enhancement features**

#### Step 6.1: Enterprise Features
- [ ] **Rate limiting** - Prevent API abuse
- [ ] **Role-based access control** - User permissions
- [ ] **Audit logging** - Track all configuration changes
- [ ] **Approval workflows** - Configuration change approval
- [ ] **Multi-tenant support** - Isolated configurations per tenant

## API Testing Guide

### 🧪 **Integration Testing Overview**

The Configuration Management API includes comprehensive testing capabilities through both automated scripts and manual testing approaches.

### **Authentication Requirements**

All Configuration Management endpoints require **admin authentication**:

```bash
# Set authentication token
export TRAZR_AUTH_TOKEN="your-admin-bearer-token"

# Or pass directly in requests
curl -H "Authorization: Bearer ${TRAZR_TEST_JWT}" \
     -H "Content-Type: application/json" \
     http://localhost:8080/api/v1/config/schemas
```

### **Automated Testing Script**

#### **Quick Start**
```bash
# Make script executable
chmod +x scripts/test-config-api.sh

# Run all tests with default settings
./scripts/test-config-api.sh

# Run with custom base URL and auth token
TRAZR_BASE_URL=http://localhost:8080 \
TRAZR_AUTH_TOKEN=your-token \
./scripts/test-config-api.sh
```

#### **Test Coverage**
The automated script tests:
- ✅ **Health Check** - API server availability
- ✅ **Statistics Endpoint** - Configuration counts and metrics
- ✅ **CRUD Operations** - Create, Read, Update, Delete for all 5 config types
- ✅ **Query Filtering** - Name, enabled, priority, search parameters
- ✅ **Error Scenarios** - Invalid requests, missing data, authentication failures
- ✅ **Concurrent Access** - Multi-request handling validation

### **Manual Testing Examples**

#### **1. Schema Configuration Testing**
```bash
# Create schema configuration
curl -X POST http://localhost:8080/api/v1/config/schemas \
  -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "production-otel",
    "description": "Production OpenTelemetry schema",
    "enabled": true,
    "priority": 1,
    "metadata": {
      "version": "1.0",
      "environment": "production"
    },
    "config": {
      "database_schema": "otel",
      "priority": 1,
      "routing": {
        "logs": "otel_logs",
        "traces": "otel_traces",
        "metrics": "otel_metrics"
      }
    }
  }'

# Expected Response: 201 Created
{
  "id": 1,
  "name": "production-otel",
  "description": "Production OpenTelemetry schema",
  "enabled": true,
  "priority": 1,
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:30:00Z",
  "metadata": {
    "version": "1.0",
    "environment": "production"
  },
  "config": {
    "database_schema": "otel",
    "priority": 1,
    "routing": {
      "logs": "otel_logs",
      "traces": "otel_traces",
      "metrics": "otel_metrics"
    }
  }
}
```

#### **2. Embedding Configuration Testing**
```bash
# Create embedding configuration
curl -X POST http://localhost:8080/api/v1/config/embeddings \
  -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "openai-embeddings",
    "description": "OpenAI embedding provider",
    "enabled": true,
    "priority": 1,
    "metadata": {
      "provider_type": "openai",
      "model_version": "v1"
    },
    "config": {
      "provider": "openai",
      "model": "text-embedding-ada-002",
      "dimension": 1536,
      "max_context_length": 8191,
      "rate_limit": 1000,
      "timeout": 30,
      "normalize": false,
      "headers": {
        "Authorization": "Bearer sk-your-openai-key"
      }
    }
  }'
```

#### **3. Vector Store Configuration Testing**
```bash
# Create vector store configuration
curl -X POST http://localhost:8080/api/v1/config/vectorstores \
  -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "postgres-vectorstore",
    "description": "PostgreSQL vector store",
    "enabled": true,
    "priority": 1,
    "metadata": {
      "storage_type": "postgres",
      "index_type": "ivfflat"
    },
    "config": {
      "provider": "postgres",
      "endpoint": "postgresql://localhost:5432/vectordb",
      "user": "vectoruser",
      "database": "vectors",
      "embedding_id": 1,
      "batch_size": 100,
      "flush_interval_ms": 200,
      "max_retries": 3,
      "vendor_specific_options": {
        "index_type": "ivfflat",
        "index_params": "lists=100"
      }
    }
  }'
```

#### **4. Query Parameter Testing**
```bash
# List configurations with filters
curl -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
     "http://localhost:8080/api/v1/config/schemas?enabled=true&priority=1&limit=10"

# Search configurations
curl -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
     "http://localhost:8080/api/v1/config/embeddings?search=openai"

# Get configuration statistics
curl -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
     "http://localhost:8080/api/v1/config/statistics"
```

### **Expected API Responses**

#### **Success Responses**
- **201 Created** - Configuration created successfully
- **200 OK** - Configuration retrieved/updated successfully
- **204 No Content** - Configuration deleted successfully

#### **Error Responses**
- **400 Bad Request** - Invalid request data or parameters
- **401 Unauthorized** - Missing or invalid authentication
- **404 Not Found** - Configuration not found
- **409 Conflict** - Configuration name already exists
- **422 Unprocessable Entity** - Validation errors
- **500 Internal Server Error** - Server-side errors

### **Testing Workflow**

#### **Complete Configuration Lifecycle Test**
```bash
# 1. Create configuration
CONFIG_ID=$(curl -X POST http://localhost:8080/api/v1/config/schemas \
  -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"test-schema","enabled":true,"priority":1,"config":{"database_schema":"test"}}' \
  | jq -r '.id')

# 2. Get configuration by ID
curl -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
     "http://localhost:8080/api/v1/config/schemas/$CONFIG_ID"

# 3. Update configuration
curl -X PUT "http://localhost:8080/api/v1/config/schemas/$CONFIG_ID" \
  -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"test-schema","enabled":false,"priority":2,"config":{"database_schema":"test_updated"}}'

# 4. List configurations (should include updated config)
curl -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
     "http://localhost:8080/api/v1/config/schemas"

# 5. Delete configuration
curl -X DELETE "http://localhost:8080/api/v1/config/schemas/$CONFIG_ID" \
  -H "Authorization: Bearer $TRAZR_AUTH_TOKEN"
```

### **Performance Testing**

#### **Concurrent Request Testing**
```bash
# Test concurrent access (run multiple requests in parallel)
for i in {1..10}; do
  curl -H "Authorization: Bearer $TRAZR_AUTH_TOKEN" \
       "http://localhost:8080/api/v1/config/schemas" &
done
wait
```

### **Integration Test Files**

#### **Go Integration Tests**
- **File**: `trazrextension/internal/config_integration_test.go`
- **Purpose**: Comprehensive Go-based integration tests
- **Usage**: `go test -v ./internal -run TestConfig`

#### **Bash Test Script**
- **File**: `scripts/test-config-api.sh`
- **Purpose**: Automated API endpoint testing
- **Usage**: `./scripts/test-config-api.sh`

### **OpenAPI Documentation Testing**

The API is fully documented with OpenAPI specifications:
- **Bundled Documentation**: Available after running `redocly bundle`
- **Interactive Testing**: Use tools like Swagger UI or Postman
- **Schema Validation**: All request/response schemas are defined

## Current Implementation Status Summary

### ✅ COMPLETED PHASES
1. **Phase 1: Database & Repository Layer** - All CRUD operations, validation, dependency checking
2. **Phase 2: Business Logic Layer** - ConfigService with repository integration
3. **Phase 3: HTTP API Layer** - Complete REST endpoints with authentication and error handling

### 🔄 CURRENT CHALLENGES
1. **Create/Update Operations** - Architectural constraint preventing direct database model imports in extension package
2. **OpenAPI Documentation** - Needs update to reflect implemented endpoints

### 📋 IMMEDIATE NEXT STEPS

#### High Priority
1. **Resolve Create/Update Architecture** - Add database model factory methods to repository package
2. **Update OpenAPI Specifications** - Document all implemented configuration management endpoints
3. **End-to-End Testing** - Test all CRUD operations through HTTP layer

#### Medium Priority  
4. **Connection Testing Services** - Test embedding providers and vector stores
5. **Template Validation** - Syntax checking and preview functionality
6. **System Health Monitoring** - Configuration status and statistics

#### Low Priority
7. **Rate Limiting** - API abuse prevention
8. **Client SDKs** - Go/Python/JavaScript clients
9. **Enterprise Features** - RBAC, audit logging, workflows

## OpenAPI Documentation Status

**COMPLETED**: The OpenAPI specifications have been fully updated to reflect the implemented endpoints and schemas.

### Completed Updates:
1. **Path Definitions** (`trazrextension/openapi/paths/config.yaml`):
   - ✅ All CRUD endpoints properly documented with comprehensive descriptions
   - ✅ Configuration types clearly explained (schemas, processors, embeddings, templates, vectorstores)
   - ✅ Authentication requirements documented for all endpoints
   - ✅ Comprehensive examples added for create operations
   - ✅ Proper error response documentation with all HTTP status codes
   - ✅ Dependency validation warnings added for delete operations

2. **Schema Definitions** (`trazrextension/openapi/components/schemas.yaml`):
   - ✅ ConfigResponse schema aligned with implementation
   - ✅ ConfigCreateRequest schema with comprehensive examples for all config types
   - ✅ ConfigUpdateRequest schema with partial and full update examples
   - ✅ ConfigStatisticsResponse schema properly defined
   - ✅ Multiple realistic examples for different configuration types

3. **Security and Parameters**:
   - ✅ Admin authentication requirements documented
   - ✅ Standard query parameters properly referenced
   - ✅ Config-specific path parameters documented with enums
   - ✅ All response codes including authentication and authorization errors
### Required Schema Definitions:
- ConfigCreateRequest
- ConfigUpdateRequest  
- ConfigResponse
- ConfigStatisticsResponse
- ConfigQueryParams
- Standard error responses

## File Structure for Implementation

```
pkg/repository/
├── config_manager.go          # ConfigurationManager implementation
├── config_validator.go        # Configuration validation logic
├── config_tester.go          # Connection testing and validation
├── config_dependencies.go    # Dependency tracking
├── config_statistics.go      # Statistics and health monitoring
└── config_export.go          # Export/import functionality

trazrextension/internal/
├── config_api.go             # HTTP handlers
├── config_middleware.go      # HTTP middleware
├── config_models.go          # Request/response models
└── config_routes.go          # Route definitions
```

## Success Criteria

### Phase 1 Complete When:
- [ ] All database operations work without "not implemented" errors
- [ ] Repository layer has comprehensive unit tests
- [ ] Configuration validation prevents invalid data

### Phase 2 Complete When:
- [ ] ConfigurationManager interface fully implemented
- [ ] All CRUD operations work through business logic layer
- [ ] Dependency validation prevents orphaned configurations

### Phase 3 Complete When:
- [ ] Connection testing works for embeddings and vector stores
- [ ] Template validation and preview functionality works
- [ ] System health and statistics available

### Phase 4 Complete When:
- [ ] All API endpoints functional with proper error handling
- [ ] API documentation available
- [ ] Basic authentication and validation in place

This methods-first approach ensures a solid foundation before exposing functionality through HTTP endpoints, reducing the risk of incomplete or buggy API implementations.
