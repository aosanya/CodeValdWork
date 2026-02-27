# Task Management — Implementation Details

## MVP-WORK-001 — Library Scaffolding & Task Model

**Status**: 🔲 Not Started  
**Branch**: `feature/WORK-001_library_scaffolding`

### Goal

Scaffold the Go module with the `TaskManager` interface, `Task` domain type, status lifecycle, and exported errors.

### Files to Create/Modify

| File | Purpose |
|---|---|
| `go.mod` | Module declaration (`github.com/aosanya/CodeValdWork`) |
| `task.go` | `TaskManager` interface, `Backend` interface, `taskManager` implementation |
| `types.go` | `Task`, `TaskStatus`, `TaskPriority`, `TaskFilter`, `CanTransitionTo` |
| `errors.go` | `ErrTaskNotFound`, `ErrTaskAlreadyExists`, `ErrInvalidStatusTransition`, `ErrInvalidTask` |

### Acceptance Tests

- `CreateTask` with empty `Title` returns `ErrInvalidTask`
- `UpdateTask` with `pending → completed` returns `ErrInvalidStatusTransition`
- `UpdateTask` with `pending → in_progress` succeeds
- `UpdateTask` with `in_progress → completed` succeeds
- `UpdateTask` with `completed → pending` returns `ErrInvalidStatusTransition`
- `NewTaskManager(nil)` returns an error

---

## MVP-WORK-002 — ArangoDB Backend

**Status**: 🔲 Not Started  
**Branch**: `feature/WORK-002_arangodb_backend`

### Goal

Implement `codevaldwork.Backend` backed by ArangoDB. Tasks are stored in the `tasks` collection, keyed by task ID, scoped by `agency_id`.

### Files to Create/Modify

| File | Purpose |
|---|---|
| `storage/arangodb/arangodb.go` | `ArangoBackend` implementing `codevaldwork.Backend` |
| `storage/arangodb/arangodb_test.go` | Integration tests (skip when `WORK_ARANGO_ENDPOINT` not set) |

### Key Behaviours

- `CreateDocument` with `_key = task.ID` — returns `ErrTaskAlreadyExists` on conflict
- `ReadDocument` + agency ownership check — returns `ErrTaskNotFound` if key missing or wrong agency
- `UpdateDocument` — full document replace with refreshed `updated_at`
- `RemoveDocument` after ownership check
- AQL query with optional `status`, `priority`, `assigned_to` filters

### Acceptance Tests

- Create a task and read it back — all fields match
- Create two tasks for same agency and list both
- Create tasks for two agencies — `ListTasks` for agency A does not return agency B tasks
- Delete a task — subsequent `GetTask` returns `ErrTaskNotFound`
- Get a non-existent task — returns `ErrTaskNotFound`

---

## MVP-WORK-003 — gRPC Service (TaskService)

**Status**: 🔲 Not Started  
**Branch**: `feature/WORK-003_grpc_service`

### Goal

Generate proto stubs and implement the `TaskService` gRPC handler in `internal/grpcserver/`.

### Files to Create/Modify

| File | Purpose |
|---|---|
| `proto/codevaldwork/v1/service.proto` | RPC definitions |
| `proto/codevaldwork/v1/codevaldwork.proto` | `Task`, `TaskFilter` message types |
| `proto/codevaldwork/v1/errors.proto` | `InvalidStatusTransitionInfo` detail |
| `internal/grpcserver/server.go` | Handler implementations |
| `internal/grpcserver/errors.go` | Domain error → gRPC status code mapping |
| `cmd/server/main.go` | Binary wiring |

### Error Mapping

| Domain Error | gRPC Code |
|---|---|
| `ErrTaskNotFound` | `NOT_FOUND` |
| `ErrTaskAlreadyExists` | `ALREADY_EXISTS` |
| `ErrInvalidStatusTransition` | `FAILED_PRECONDITION` |
| `ErrInvalidTask` | `INVALID_ARGUMENT` |

### Acceptance Tests

- `CreateTask` RPC returns `ALREADY_EXISTS` when task ID is duplicate
- `UpdateTask` RPC returns `FAILED_PRECONDITION` on invalid transition
- `DeleteTask` RPC returns `NOT_FOUND` for unknown task
- `ListTasks` RPC with status filter only returns matching tasks

---

## MVP-WORK-004 — CodeValdCross Registration

**Status**: 🔲 Not Started  
**Branch**: `feature/WORK-004_cross_registration`

### Goal

Implement `internal/registrar` to send startup registration and periodic heartbeats to CodeValdCross.

### Files to Create/Modify

| File | Purpose |
|---|---|
| `internal/registrar/registrar.go` | `Registrar` struct, `New`, `Run`, `Close`, `ping` |
| `proto/codevaldcross/v1/registration.proto` | `OrchestratorService.Register` RPC |

### Topics Declared

| Direction | Topic |
|---|---|
| Produces | `work.task.created` |
| Produces | `work.task.updated` |
| Produces | `work.task.completed` |
| Consumes | `cross.task.requested` |
| Consumes | `cross.agency.created` |

### Acceptance Tests

- When `CROSS_GRPC_ADDR` is unset, server starts without error and skips registration
- When `CROSS_GRPC_ADDR` is set but unreachable, server continues running (non-fatal)
- Registrar sends heartbeat at configured interval

---

## MVP-WORK-005 — Integration Tests

**Status**: 🔲 Not Started  
**Branch**: `feature/WORK-005_integration_tests`

### Goal

End-to-end tests covering the full gRPC + ArangoDB stack using a real ArangoDB instance. Tests skip when `WORK_ARANGO_ENDPOINT` is not set.

### Test Matrix

- Create → Get round-trip
- Create → Update (valid transitions)
- Create → Update (invalid transition → `FAILED_PRECONDITION`)
- Create → Delete → Get (`NOT_FOUND`)
- Create multiple → List with filter
- Create with duplicate ID → `ALREADY_EXISTS`
