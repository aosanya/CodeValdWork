# CodeValdWork — Architecture

## Overview

CodeValdWork is a **Go gRPC microservice** that manages the full task lifecycle
for CodeVald agencies. It is built on the `entitygraph.DataManager` /
`entitygraph.SchemaManager` interfaces from SharedLib — the same foundation as
CodeValdDT and CodeValdComm.

**TaskManager** is the public API. Internally, the `taskManager` implementation
holds a `WorkDataManager` (entity + graph operations) and a `WorkSchemaManager`
(schema storage). A pre-delivered schema is seeded per agency on first use.

---

## Architecture Documents

| Document | Contents |
|---|---|
| [architecture-domain.md](architecture-domain.md) | Entity TypeDefinitions (Task, TaskGroup, Agent), pre-delivered schema, graph relationship model, pub/sub topics |
| [architecture-storage.md](architecture-storage.md) | ArangoDB collections, document shapes, named graph, indexes |
| [architecture-service.md](architecture-service.md) | gRPC TaskService, HTTP convenience routes, Cross registration, project layout |
| [architecture-flows.md](architecture-flows.md) | CreateTask, UpdateTask (fields + status), AssignTask, CreateRelationship, SchemaSeeding, CrossTaskRequested |

---

## Key Design Decisions

| Decision | Choice |
|---|---|
| Entity-graph foundation | `WorkDataManager = entitygraph.DataManager`; `WorkSchemaManager = entitygraph.SchemaManager` |
| Pre-delivered schema | Fixed TypeDefinitions seeded on `cross.agency.created` |
| Task entity types | `Task`, `TaskGroup`, `Agent` — each in own `work_*` collection |
| Relationships | Graph edges: `assigned_to`, `blocks`, `subtask_of`, `depends_on`, `member_of` |
| Blocker enforcement | Hard gate on `pending → in_progress` if any `blocks`-inbound task is non-terminal |
| Pub/sub granularity | Separate topics: `created`, `updated`, `status.changed`, `completed`, `assigned` |
| HTTP routes | Full convenience layer mirroring CodeValdComm pattern |
| Status lifecycle | `pending → in_progress → completed/failed/cancelled`; enforced in `TaskManager` layer |

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
| Agency created | Schema seeded via `WorkSchemaManager.SetSchema` |
| Cross dispatches a task | `TaskManager.CreateTask(agencyID, task)` (via `cross.task.requested`) |
| Agent claims a task | `TaskManager.UpdateTaskStatus(agencyID, taskID, in_progress)` |
| Agent assigned explicitly | `TaskManager.AssignTask(agencyID, taskID, agentID)` → `assigned_to` edge |
| Agent completes task | `TaskManager.UpdateTaskStatus(agencyID, taskID, completed)` |
| Agent fails task | `TaskManager.UpdateTaskStatus(agencyID, taskID, failed)` |
| Operator cancels task | `TaskManager.UpdateTaskStatus(agencyID, taskID, cancelled)` |
| Task dependency declared | `TaskManager.CreateRelationship(blocks/subtask_of/depends_on)` |
| UI task list view | `TaskManager.ListTasks(agencyID, filter)` |
| UI task detail view | `TaskManager.GetTask(agencyID, taskID)` |
| UI blocker view | `TraverseGraph(taskID, blocks, inbound)` |
| UI subtask view | `TraverseGraph(taskID, subtask_of, inbound)` |
