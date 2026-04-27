# Task Management — Phase 1 (Complete)

Topics: Task lifecycle · Status state machine · ArangoDB persistence

---

## Status

Phase 1 (MVP-WORK-001 through MVP-WORK-005) shipped on 2026-02-27. All five
tasks are recorded in [`mvp_done.md`](../mvp_done.md).

The Phase 1 deliverable is a Task lifecycle backed by `entitygraph.DataManager`
in a single `work_tasks` collection: CRUD, status state machine, agency
ownership checks, and gRPC `TaskService` (five RPCs).

---

## What Phase 1 Delivered

| Concern | Location |
|---|---|
| `TaskManager` interface + `taskManager` impl | [`task.go`](../../../task.go) |
| `Task`, `TaskStatus`, `TaskPriority`, `TaskFilter`, `CanTransitionTo` | [`types.go`](../../../types.go) |
| Domain errors (`ErrTaskNotFound`, `ErrTaskAlreadyExists`, `ErrInvalidStatusTransition`, `ErrInvalidTask`) | [`errors.go`](../../../errors.go) |
| Pre-delivered schema (`Task` TypeDefinition only) | [`schema.go`](../../../schema.go) |
| ArangoDB `entitygraph.DataManager` adapter | [`storage/arangodb/arangodb.go`](../../../storage/arangodb/arangodb.go) |
| gRPC `TaskService` (Create/Get/Update/Delete/List) | [`internal/server/server.go`](../../../internal/server/server.go) |
| Domain → gRPC error mapping | [`internal/server/errors.go`](../../../internal/server/errors.go) |
| Cross registration + minimal `CrossPublisher` (logs only) | [`internal/registrar/registrar.go`](../../../internal/registrar/registrar.go) |
| Unit tests (with `fakeDataManager`) | [`task_test.go`](../../../task_test.go) |
| ArangoDB integration tests | [`storage/arangodb/arangodb_test.go`](../../../storage/arangodb/arangodb_test.go) |

---

## Phase 1 Limitations (Closed by Phase 2)

| Limitation | Phase 2 task |
|---|---|
| `Task` has only flat string properties — no `dueAt` (datetime), `tags` (array), `estimatedHours` (number), `context`, `completedAt` | [MVP-WORK-008](schema.md) |
| No `TaskGroup` or `Agent` entity types | [MVP-WORK-008](schema.md), [MVP-WORK-010](agent-assignment.md), [MVP-WORK-012](task-group.md) |
| No graph edges — `assigned_to` is a string field, not a Task → Agent edge | [MVP-WORK-009](relationships.md), [MVP-WORK-010](agent-assignment.md) |
| Status state machine has no blocker gate — `pending → in_progress` always allowed | [MVP-WORK-011](relationships.md) |
| `CrossPublisher.Publish(ctx, topic, agencyID)` carries no payload, logs only, fires for 3 of 6 architecture-listed topics | [MVP-WORK-014](pubsub.md) |
| HTTP convenience layer is two routes only (`POST/GET /work/{agencyId}/tasks`) | [MVP-WORK-015](grpc-surface.md) |

For the Phase 2 task list, see [`mvp.md`](../mvp.md). For per-topic specs, see
the other files in this directory.
