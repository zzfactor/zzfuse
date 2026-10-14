# Template System Organization Guide

## 🎯 **Current State Analysis**

Your project has **3 distinct templating systems** that serve different purposes:

1. **SQL Template System** - Database schema generation
2. **Embedding Template System** - Content generation for vector embeddings  
3. **Vector Store Template System** - DDL generation for vector stores

## 🏗️ **Recommended Folder Structure (Within Existing Packages)**

### **Option 1: Repository Package Organization (Recommended)**

```
pkg/
├── repository/
│   ├── template/                      # Template systems within repository package
│   │   ├── sql/                      # SQL template system
│   │   │   ├── engine.go             # SQLTemplateEngine
│   │   │   ├── partition.go          # PartitionTemplateProcessor
│   │   │   └── ddl.go                # DDL template functions
│   │   ├── embedding/                # Embedding template system
│   │   │   ├── engine.go             # EmbeddingTemplateEngine
│   │   │   ├── functions.go          # Template functions (categorization, etc.)
│   │   │   ├── pool.go               # TemplatePool for high-throughput
│   │   │   └── cache.go              # Template caching
│   │   ├── vectorstore/              # Vector store template system
│   │   │   ├── engine.go             # VectorStoreTemplateEngine
│   │   │   └── ddl.go                # Vector store DDL templates
│   │   └── common/                   # Shared template utilities
│   │       ├── functions.go          # Common template functions
│   │       ├── pool.go               # Shared template pool
│   │       └── cache.go              # Shared caching utilities
│   ├── embedding_worker.go           # Main embedding worker (existing)
│   ├── vectorstore_provider_pgvector.go # Vector store provider (existing)
│   └── internal/
│       └── database/
│           └── config_otel_partitions.go # SQL template processing (existing)
```

### **Option 2: Util Package Organization**

```
pkg/
├── util/
│   ├── template/                     # Template systems within util package
│   │   ├── sql/                     # SQL template system
│   │   │   ├── engine.go            # SQLTemplateEngine
│   │   │   ├── partition.go         # PartitionTemplateProcessor
│   │   │   └── ddl.go               # DDL template functions
│   │   ├── embedding/               # Embedding template system
│   │   │   ├── engine.go            # EmbeddingTemplateEngine
│   │   │   ├── functions.go         # Template functions (categorization, etc.)
│   │   │   ├── pool.go              # TemplatePool for high-throughput
│   │   │   └── cache.go             # Template caching
│   │   ├── vectorstore/             # Vector store template system
│   │   │   ├── engine.go            # VectorStoreTemplateEngine
│   │   │   └── ddl.go               # Vector store DDL templates
│   │   └── common/                  # Shared template utilities
│   │       ├── functions.go         # Common template functions
│   │       ├── pool.go              # Shared template pool
│   │       └── cache.go             # Shared caching utilities
│   ├── template.go                  # Move existing template.go here
│   ├── attributes.go                # Existing attributes file
│   └── format.go                    # Existing format file
```

## 🎯 **Recommended Implementation: Option 1 (Repository Package)**

### **1. SQL Template System**

```go
// pkg/repository/template/sql/engine.go
package sql

import (
    "fmt"
    "strings"
    "text/template"
)

// SQLTemplateEngine handles SQL DDL template processing
type SQLTemplateEngine struct {
    partitionProcessor *PartitionTemplateProcessor
}

// NewSQLTemplateEngine creates a new SQL template engine
func NewSQLTemplateEngine() *SQLTemplateEngine {
    return &SQLTemplateEngine{
        partitionProcessor: NewPartitionTemplateProcessor(),
    }
}

// ProcessPartitionTemplate processes SQL templates with partition configuration
func (e *SQLTemplateEngine) ProcessPartitionTemplate(content string, config PartitionConfig) string {
    return e.partitionProcessor.Process(content, config)
}

// ExecuteDDLTemplate executes a DDL template with parameters
func (e *SQLTemplateEngine) ExecuteDDLTemplate(templateName, content string, params interface{}) (string, error) {
    tmpl, err := template.New(templateName).Parse(content)
    if err != nil {
        return "", fmt.Errorf("failed to parse DDL template %s: %w", templateName, err)
    }
    
    var buf strings.Builder
    if err := tmpl.Execute(&buf, params); err != nil {
        return "", fmt.Errorf("failed to execute DDL template %s: %w", templateName, err)
    }
    
    return buf.String(), nil
}
```

### **2. Embedding Template System**

```go
// pkg/repository/template/embedding/engine.go
package embedding

import (
    "text/template"
    "github.com/Masterminds/sprig/v3"
    "github.com/valyala/fasttemplate"
)

// EmbeddingTemplateEngine provides high-performance template rendering for embeddings
type EmbeddingTemplateEngine struct {
    complexTemplates map[string]*template.Template
    simpleTemplates  map[string]*fasttemplate.Template
    pool             *TemplatePool
    funcMap          template.FuncMap
}

// NewEmbeddingTemplateEngine creates a new template engine
func NewEmbeddingTemplateEngine() *EmbeddingTemplateEngine {
    return &EmbeddingTemplateEngine{
        complexTemplates: make(map[string]*template.Template),
        simpleTemplates:  make(map[string]*fasttemplate.Template),
        pool:             NewTemplatePool(),
        funcMap:          NewEmbeddingFuncMap(),
    }
}

// RegisterTemplate registers a template with the engine
func (e *EmbeddingTemplateEngine) RegisterTemplate(name, content string) error {
    if isSimpleTemplate(content) {
        e.simpleTemplates[name] = fasttemplate.New(content, "{{", "}}")
        return nil
    }
    
    tmpl, err := template.New(name).Funcs(e.funcMap).Parse(content)
    if err != nil {
        return err
    }
    e.complexTemplates[name] = tmpl
    return nil
}

// Render renders a template using the most efficient method
func (e *EmbeddingTemplateEngine) Render(templateName string, data map[string]interface{}) (string, error) {
    // Try simple template first (fastest)
    if simpleTmpl, exists := e.simpleTemplates[templateName]; exists {
        return simpleTmpl.ExecuteString(data), nil
    }
    
    // Fall back to complex template with pool
    if complexTmpl, exists := e.complexTemplates[templateName]; exists {
        return e.pool.RenderToString(complexTmpl, data)
    }
    
    return "", fmt.Errorf("template not found: %s", templateName)
}
```

### **3. Vector Store Template System**

```go
// pkg/repository/template/vectorstore/engine.go
package vectorstore

import (
    "text/template"
    "bytes"
)

// VectorStoreTemplateEngine handles vector store DDL template processing
type VectorStoreTemplateEngine struct {
    templates map[string]*template.Template
}

// NewVectorStoreTemplateEngine creates a new vector store template engine
func NewVectorStoreTemplateEngine() *VectorStoreTemplateEngine {
    return &VectorStoreTemplateEngine{
        templates: make(map[string]*template.Template),
    }
}

// RegisterDDLTemplate registers a DDL template
func (e *VectorStoreTemplateEngine) RegisterDDLTemplate(name, content string) error {
    tmpl, err := template.New(name).Parse(content)
    if err != nil {
        return err
    }
    e.templates[name] = tmpl
    return nil
}

// ExecuteDDLTemplate executes a DDL template with parameters
func (e *VectorStoreTemplateEngine) ExecuteDDLTemplate(templateName string, params interface{}) (string, error) {
    tmpl, exists := e.templates[templateName]
    if !exists {
        return "", fmt.Errorf("DDL template not found: %s", templateName)
    }
    
    var buf bytes.Buffer
    if err := tmpl.Execute(&buf, params); err != nil {
        return "", fmt.Errorf("failed to execute DDL template %s: %w", templateName, err)
    }
    
    return buf.String(), nil
}
```

## 🔧 **Naming Conventions**

### **1. Folder Names**
- **SQL Templates**: `pkg/repository/template/sql`
- **Embedding Templates**: `pkg/repository/template/embedding`
- **Vector Store Templates**: `pkg/repository/template/vectorstore`
- **Shared Utilities**: `pkg/repository/template/common`

### **2. Type Names**
- **Engines**: `SQLTemplateEngine`, `EmbeddingTemplateEngine`, `VectorStoreTemplateEngine`
- **Processors**: `PartitionTemplateProcessor`, `ContentTemplateProcessor`
- **Pools**: `SQLTemplatePool`, `EmbeddingTemplatePool`
- **Caches**: `SQLTemplateCache`, `EmbeddingTemplateCache`

### **3. Function Names**
- **SQL**: `ProcessPartitionTemplate`, `ExecuteDDLTemplate`
- **Embedding**: `RenderTemplate`, `RegisterTemplate`, `CategorizeLogCount`
- **Vector Store**: `ExecuteDDLTemplate`, `RegisterDDLTemplate`

### **4. Interface Names**
```go
// Common interfaces for all template systems
type TemplateEngine interface {
    RegisterTemplate(name, content string) error
    ExecuteTemplate(name string, data interface{}) (string, error)
}

type TemplateProcessor interface {
    Process(content string, config interface{}) string
}

type TemplatePool interface {
    Get() interface{}
    Put(item interface{})
}
```

## 📊 **Migration Strategy**

### **Phase 1: Create New Folder Structure**
```bash
# Create new folder structure within existing packages
mkdir -p pkg/repository/template/{sql,embedding,vectorstore,common}

# Move existing template code to new folders
mv pkg/util/template.go pkg/repository/template/embedding/engine.go
# Keep vectorstore_provider_pgvector.go in place but extract template logic
# Keep config_otel_partitions.go in place but extract template logic
```

### **Phase 2: Update Imports**
```go
// Update imports in existing files
import (
    "github.com/medxops/trazr-otel/pkg/repository/template/embedding"
    "github.com/medxops/trazr-otel/pkg/repository/template/sql"
    "github.com/medxops/trazr-otel/pkg/repository/template/vectorstore"
)
```

### **Phase 3: Refactor Existing Code**
```go
// Before: Direct template usage in embedding worker
func (w *EmbeddingWorker) initTemplateCache() {
    w.TemplateCache = make(map[string]*template.Template)
    funcMap := util.EmbeddingTemplateFuncMap()
    // ... existing code
}

// After: Use dedicated template engine
func (w *EmbeddingWorker) initTemplateEngine() {
    w.templateEngine = embedding.NewEmbeddingTemplateEngine()
    for typ, tmplStr := range w.TemplateContent {
        w.templateEngine.RegisterTemplate(typ, tmplStr)
    }
}
```

## 🎯 **Benefits of This Organization**

### **1. Clear Separation of Concerns**
- **SQL Templates**: Database schema generation only
- **Embedding Templates**: Content generation for vector embeddings
- **Vector Store Templates**: Vector store DDL generation

### **2. Consistent Naming Conventions**
- All template engines follow the same naming pattern
- Clear distinction between different template types
- Easy to understand and maintain

### **3. Reusable Components**
- Shared template pool and caching utilities
- Common template functions across systems
- Consistent error handling and logging

### **4. Performance Optimization**
- Each system can be optimized independently
- Embedding templates get the most optimization (dual engine)
- SQL templates remain simple and fast

### **5. Easy Testing**
- Each template system can be tested independently
- Clear interfaces for mocking
- Isolated test suites

## 🚀 **Implementation Priority**

1. **High Priority**: Create new folder structure within existing packages
2. **Medium Priority**: Migrate template system
3. **Low Priority**: Migrate SQL and vector store template systems

This organization keeps everything within your existing package structure while providing clear separation and consistent naming conventions. 