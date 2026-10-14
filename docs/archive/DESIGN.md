# Vector Store Test Factory Refactor

## Motivation

As the codebase supports multiple vector store backends (e.g., PGVector, Qdrant), it is essential that E2E tests are backend-agnostic, extensible, and maintainable. The previous approach tightly coupled test logic to specific storage implementations (e.g., direct SQL connections), making it difficult to add new backends or ensure test isolation.

## Goals
- Decouple test logic from backend-specific initialization and teardown.
- Enable easy addition of new vector store backends (e.g., Qdrant, Pinecone) without modifying test logic.
- Ensure each test is isolated and can be run in parallel.
- Encapsulate all backend-specific setup/teardown in a factory abstraction.

## Refactor Steps

### 1. Define the Factory Interface

Create an interface that abstracts backend initialization and cleanup for tests:

```go
// testutil/vectorstore_factory.go
package testutil

import (
    "testing"
    "github.com/medxops/trazr-otel/pkg/repository"
)

type VectorStoreFactory interface {
    // Open returns a VectorStore, a cleanup function, and an error.
    Open(t *testing.T) (vectorstore.VectorStore, func(), error)
    // Optionally, add a method for schema management if needed.
    SyncSchema(t *testing.T, ...interface{}) error
}
```

---

### 2. Implement a Factory for the Local/PGVector Backend

Create a concrete factory for the local/pgvector backend:

```go
// testutil/pgvector_factory.go
package testutil

import (
    "database/sql"
    "testing"
    "github.com/medxops/trazr-otel/pkg/repository"
    "go.uber.org/zap"
)

type PGVectorFactory struct {
    DSN            string
    TableName      string
    EmbeddingDim   int
    VectorStorePromotedColumns map[string]string
}

func (f *PGVectorFactory) Open(t *testing.T) (vectorstore.VectorStore, func(), error) {
    db, err := sql.Open("pgx", f.DSN)
    if err != nil {
        return nil, nil, err
    }
    cleanup := func() { db.Close() }
    store, err := vectorstore.NewLocalVectorStore(
        db,
        f.TableName,
        f.VectorStorePromotedColumns,
        0, 0, 0,
        zap.NewNop(),
    )
    if err != nil {
        cleanup()
        return nil, nil, err
    }
    return store, cleanup, nil
}

func (f *PGVectorFactory) SyncSchema(t *testing.T, args ...interface{}) error {
    db, ok := args[0].(*sql.DB)
    if !ok {
        return nil // or return an error
    }
    schemaManager := vectorstore.DefaultSchemaManager{}
    return schemaManager.SyncSchema(db, f.TableName, f.EmbeddingDim, f.VectorStorePromotedColumns)
}
```

---

### 3. Refactor the Test to Use the Factory

Update E2E tests to use the factory abstraction:

```go
// In e2e_vectorstore_test.go
import "github.com/medxops/trazr-otel/trazrexporter/testutil"

factories := []struct {
    name    string
    factory testutil.VectorStoreFactory
}{
    {
        name: "local",
        factory: &testutil.PGVectorFactory{
            DSN:            "postgres://trazr:trazrpass@localhost:5432/testdb?sslmode=disable",
            TableName:      "embeddings",
            EmbeddingDim:   1024,
            VectorStorePromotedColumns: map[string]string{...},
        },
    },
    // Add more factories for other backends (Qdrant, etc.)
}

for _, backend := range factories {
    for _, tc := range embeddingProviders {
        t.Run(backend.name+"/"+tc.name, func(t *testing.T) {
            store, cleanup, err := backend.factory.Open(t)
            if err != nil { t.Fatalf("failed to create vector store: %v", err) }
            defer cleanup()
            // ... rest of test logic using store ...
        })
    }
}
```

---

### 4. Add More Factories for Other Backends

For Qdrant or other backends, implement a similar factory struct that knows how to initialize and clean up its resources. Example:

```go
type QdrantFactory struct {
    // Qdrant-specific config fields
}

func (f *QdrantFactory) Open(t *testing.T) (vectorstore.VectorStore, func(), error) {
    // Initialize Qdrant client, return store and cleanup
}
```

---

### 5. Remove Backend-Specific Logic from Test Body

All backend-specific setup/teardown should now be in the factory. The test body should only use the `VectorStore` interface.

---

## Benefits
- **Decoupling:** Test logic is backend-agnostic and only interacts with the `VectorStore` interface.
- **Extensibility:** Adding a new backend requires only a new factory, not changes to test logic.
- **Isolation:** Each test gets its own backend instance and resources, improving reliability and parallelism.
- **Maintainability:** Backend-specific details are encapsulated, making the codebase easier to maintain and extend.

---

## Next Steps
- Implement the factory interface and concrete factories for all supported backends.
- Refactor all E2E and integration tests to use the factory pattern.
- Add documentation and examples for adding new backends and factories. 