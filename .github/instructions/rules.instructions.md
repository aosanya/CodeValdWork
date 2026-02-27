---
applyTo: '**'
---

# CodeValdWork — Code Structure Rules

## Library Design Principles

CodeValdWork is a **Go library and gRPC service** — not a web application. These rules reflect that:

- **No HTTP handlers, no web framework, no templating engine**
- **Callers inject dependencies** — storage backends are never hardcoded
- **Exported API surface is minimal** — expose only what consumers need

---

## Interface-First Design

**Always define interfaces before concrete types.**

```go
// ✅ CORRECT — interface in root package, consumed by CodeValdCortex
type TaskManager interface {
    CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)
    GetTask(ctx context.Context, agencyID, taskID string) (Task, error)
    // ...
}

// ❌ WRONG — leaking a concrete type to callers
type WorkTaskManager struct {
    db *arangodb.Client
}
```

**File layout — one primary concern per file:**

```
task.go      → TaskManager interface + implementation
errors.go    → all exported error types
types.go     → Task, TaskStatus, TaskPriority value types
```

---

## Task Status Rules

**ALL task status transitions must be validated.**

```go
// ✅ CORRECT — validate status transition
if !task.Status.CanTransitionTo(newStatus) {
    return ErrInvalidStatusTransition
}

// ❌ WRONG — blindly overwriting status
task.Status = newStatus
```

---

## Error Handling Rules

**All exported errors must be typed and structured:**

```go
// errors.go

var ErrTaskNotFound       = errors.New("task not found")
var ErrTaskAlreadyExists  = errors.New("task already exists")
var ErrInvalidStatus      = errors.New("invalid task status")
```

- **Never use `log.Fatal`** in library code — return errors to caller
- **Never panic** in exported functions
- **Wrap errors with context**: `fmt.Errorf("CreateTask %s: %w", agencyID, err)`

---

## Context Rules

**Every exported method must accept `context.Context` as the first argument.**

```go
// ✅ CORRECT
func (m *taskManager) CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)

// ❌ WRONG
func (m *taskManager) CreateTask(agencyID string, task Task) (Task, error)
```

---

## Godoc Rules

**Every exported type, function, interface, and method must have a godoc comment.**

- **Package comment** on the primary file of every package
- **Examples** in `_test.go` files for non-obvious API usage patterns

---

## File Size and Complexity Limits

- **Max file size**: 500 lines (hard limit)
- **Max function length**: 50 lines (prefer 20-30)
- **One primary concern per file**

---

## Naming Conventions

```go
// ✅ CORRECT — singular package names, noun-only interfaces, Err prefix for errors
package codevaldwork

type TaskManager interface{}
type Task struct{}
var ErrTaskNotFound = errors.New("task not found")

// ❌ WRONG
type ITaskManager interface{}
type TaskManagerInterface interface{}
var TaskNotFoundError = errors.New("task not found")
```

---

## Storage Backend Rules

The `Backend` interface is the injection point. The caller constructs the desired
`Backend` implementation and passes it to `NewTaskManager`. The root package never
imports any storage driver directly.

```go
// ✅ CORRECT — Backend injected by caller
b, _ := arangodb.NewArangoBackend(arangodb.Config{...})
mgr, _ := codevaldwork.NewTaskManager(b)

// ❌ WRONG — hardcoded backend inside library
func NewTaskManager() TaskManager {
    return &taskManager{db: arangoConnect()}
}
```

---

## Concurrency Rules

- **Read operations** (`GetTask`, `ListTasks`) must be **safe to call concurrently**
- **Write operations** (`CreateTask`, `UpdateTask`, `DeleteTask`) are isolated per agency
- Avoid shared mutable state in `TaskManager` implementations
