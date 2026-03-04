# Architecture

## Core Design Decisions

### 1. Single Interface: `TaskManager`

Unlike CodeValdGit which has two interfaces (`RepoManager` and `Repo`), CodeValdWork exposes one:

```go
type TaskManager interface {
    CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)
    GetTask(ctx context.Context, agencyID, taskID string) (Task, error)
    UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error)
    DeleteTask(ctx context.Context, agencyID, taskID string) error
    ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error)
}
```

All operations are stateless per call — `agencyID` is passed every time, not stored in a manager instance.

### 2. Backend Interface

```go
type Backend interface {
    CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)
    GetTask(ctx context.Context, agencyID, taskID string) (Task, error)
    UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error)
    DeleteTask(ctx context.Context, agencyID, taskID string) error
    ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error)
}
```

The `taskManager` (root package, unexported) wraps a `Backend` and adds:
- Validation (`ErrInvalidTask` on missing title)
- Status transition enforcement (`ErrInvalidStatusTransition`)

The `Backend` implementation (ArangoDB) handles raw persistence only.

### 3. Status State Machine

```
                ┌──────────────────────────┐
                ▼                          │
           [pending] ──────────────→ [cancelled]
                │
                ▼
         [in_progress] ──────────→ [cancelled]
           │         │
           ▼         ▼
      [completed]  [failed]
```

Terminal states: `completed`, `failed`, `cancelled` — no further transitions allowed.

Enforcement: `TaskStatus.CanTransitionTo(next)` in `types.go`. Called by `taskManager.UpdateTask` before delegating to the backend.

### 4. ArangoDB Storage Model

Tasks are stored as documents in an ArangoDB collection named `work_tasks`.

**Document structure:**

```json
{
  "_key": "task-abc-001",
  "agency_id": "agency-xyz",
  "title": "Research topic X",
  "description": "...",
  "status": "pending",
  "priority": "medium",
  "assigned_to": "",
  "created_at": "2026-02-27T10:00:00Z",
  "updated_at": "2026-02-27T10:00:00Z",
  "completed_at": null
}
```

`_key` is the task ID. The `agency_id` field enables multi-agency isolation within a single collection. Queries always filter by `agency_id`.

### 5. gRPC Service

`TaskService` is defined in `proto/codevaldwork/v1/service.proto`. All five CRUD + list operations are exposed as unary RPCs.

Domain errors map to gRPC status codes in `internal/grpcserver/errors.go`:

| Domain Error | gRPC Code |
|---|---|
| `ErrTaskNotFound` | `NOT_FOUND` |
| `ErrTaskAlreadyExists` | `ALREADY_EXISTS` |
| `ErrInvalidStatusTransition` | `FAILED_PRECONDITION` |
| `ErrInvalidTask` | `INVALID_ARGUMENT` |
| all others | `INTERNAL` |

### 6. CodeValdCross Registration

On startup, `internal/registrar.Registrar` dials CodeValdCross and sends:

```json
{
  "service_name": "codevaldwork",
  "addr":         ":50053",
  "produces":     ["work.task.created", "work.task.updated", "work.task.completed"],
  "consumes":     ["cross.task.requested", "cross.agency.created"]
}
```

Heartbeats repeat at `CROSS_PING_INTERVAL` (default 10s). Failures are logged and retried — never fatal.

---

## 7. CodeValdSharedLib Dependency

CodeValdWork imports `github.com/aosanya/CodeValdSharedLib` for:

| SharedLib package | Replaces |
|---|---|
| `registrar` | `internal/registrar/registrar.go` (identical struct; service-specific metadata passed as constructor args) |
| `serverutil` | `envOrDefault`, `parseDuration` helpers and gRPC server setup block in `cmd/server/main.go` |
| `arangoutil` | ArangoDB `http.NewConnection` / auth / database bootstrap in `storage/arangodb/arangodb.go` |
| `gen/go/codevaldcross/v1` | Local copy of generated Cross stubs in `gen/go/codevaldcross/v1/` |

> **Principle**: Any infrastructure code used by more than one service lives in
> SharedLib. CodeValdWork retains only domain logic, domain errors, gRPC
> handlers, and storage collection schemas.

See task MVP-WORK-007 in [mvp.md](../3-SofwareDevelopment/mvp.md) for migration scope.

---

## Integration with CodeValdCortex

| CodeValdCortex Event | CodeValdWork Call |
|---|---|
| Agency receives new work order | `TaskManager.CreateTask(agencyID, task)` |
| Agent claims a task | `TaskManager.UpdateTask(agencyID, task{Status: in_progress, AssignedTo: agentID})` |
| Agent completes task | `TaskManager.UpdateTask(agencyID, task{Status: completed})` |
| Agent fails task | `TaskManager.UpdateTask(agencyID, task{Status: failed})` |
| Operator cancels task | `TaskManager.UpdateTask(agencyID, task{Status: cancelled})` |
| UI task list view | `TaskManager.ListTasks(agencyID, filter)` |
| UI task detail view | `TaskManager.GetTask(agencyID, taskID)` |
