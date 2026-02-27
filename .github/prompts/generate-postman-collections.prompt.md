````prompt
---
agent: agent
---

# Research gRPC / ArangoDB / Go API Usage

This prompt guides research into specific Go libraries to resolve implementation questions for CodeValdWork.

## Objective

Find accurate, version-specific API usage for a given operation. CodeValdWork uses:
- **gRPC** (`google.golang.org/grpc`)
- **protobuf** (`google.golang.org/protobuf`)
- **ArangoDB Go driver** (`github.com/arangodb/go-driver`)

## Steps

### 1. Identify the Operation

State clearly what you need to accomplish:
- e.g., "Create an ArangoDB document and return the inserted Task"
- e.g., "Map a Go error to a gRPC status code"
- e.g., "Implement a gRPC health check endpoint"

### 2. Check Documentation

**Primary sources (in order)**:
1. [gRPC Go docs](https://pkg.go.dev/google.golang.org/grpc) — authoritative API reference
2. [ArangoDB Go driver docs](https://pkg.go.dev/github.com/arangodb/go-driver) — collection CRUD API
3. [protobuf Go docs](https://pkg.go.dev/google.golang.org/protobuf) — message encoding

### 3. ArangoDB Patterns

```go
// Create a document
meta, err := col.CreateDocument(ctx, taskDoc)

// Read a document by key
var doc TaskDocument
meta, err := col.ReadDocument(ctx, key, &doc)

// Update a document
meta, err := col.UpdateDocument(ctx, key, patch)

// Delete a document
meta, err := col.RemoveDocument(ctx, key)

// Query with AQL
cursor, err := db.Query(ctx, "FOR t IN tasks FILTER t.agency_id == @agency RETURN t",
    map[string]interface{}{"agency": agencyID})
```

### 4. gRPC Error Mapping

```go
import "google.golang.org/grpc/codes"
import "google.golang.org/grpc/status"

// Map domain errors to gRPC status codes
switch {
case errors.Is(err, codevaldwork.ErrTaskNotFound):
    return nil, status.Error(codes.NotFound, err.Error())
case errors.Is(err, codevaldwork.ErrTaskAlreadyExists):
    return nil, status.Error(codes.AlreadyExists, err.Error())
default:
    return nil, status.Error(codes.Internal, err.Error())
}
```

### 5. Testing Pattern

Use mock backends for fast unit tests:

```go
type mockBackend struct {
    tasks map[string]Task
}

func (m *mockBackend) CreateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
    // ...
}
```
````
