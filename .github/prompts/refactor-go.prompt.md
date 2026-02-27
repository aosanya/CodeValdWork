````prompt
---
agent: agent
---

# Go File Modularization Prompt

You are a Go refactoring expert that helps split large files into smaller, focused packages while following Go best practices and clean architecture principles.

## When to Use This Prompt

- Go file exceeds 400 lines
- File contains multiple unrelated concerns
- Repository structure violates `.github/instructions/rules.instructions.md`
- File has poor separation of concerns

## Go Package Organization Strategy

### Standard Project Structure (CodeValdWork)

```
codevaldwork/           ← root package (library entry point)
├── task.go             # TaskManager interface + implementation
├── errors.go           # Exported error types
├── types.go            # Task, TaskStatus, TaskPriority, TaskFilter value types
├── storage/
│   └── arangodb/       # ArangoDB Backend implementation
│       ├── arangodb.go # Main implementation
│       └── arangodb_test.go
└── internal/
    ├── grpcserver/     # gRPC handler wrapping TaskManager
    └── registrar/      # CodeValdCross heartbeat registrar
```

### File Size Limits (ENFORCED)

- **Any file**: Max 500 lines (hard limit)
- **Functions**: Max 50 lines (prefer 20-30)

## Refactoring Strategies

### Strategy 1: Split by Domain (Recommended)

When `storage/arangodb/arangodb.go` grows beyond 500 lines, split by collection concern:

```
storage/arangodb/
├── tasks.go      # Task CRUD operations
├── queries.go    # Complex AQL queries, filtering
└── config.go     # Connection and configuration
```

### Strategy 2: Split by Responsibility

When `internal/grpcserver/server.go` grows, split by service method group:

```
internal/grpcserver/
├── server.go        # Server struct, New constructor
├── lifecycle.go     # CreateTask, DeleteTask handlers
├── query.go         # GetTask, ListTasks handlers
└── errors.go        # gRPC error mapping
```

## Refactoring Checklist

Before refactoring:
- [ ] Check current file sizes with `wc -l *.go`
- [ ] Identify which concerns belong in which file
- [ ] Ensure all packages compile after split: `go build ./...`
- [ ] Run tests after split: `go test -v -race ./...`
````
