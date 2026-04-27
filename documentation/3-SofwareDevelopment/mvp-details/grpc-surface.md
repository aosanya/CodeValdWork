# gRPC Surface Expansion & HTTP Convenience Routes

Topics: `proto/codevaldwork/v1/` · `internal/server/server.go` ·
`registrar.workRoutes` · CodeValdCross dynamic proxy

---

## MVP-WORK-013 — gRPC surface expansion

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-013_grpc_surface`
**Depends on**: MVP-WORK-010, MVP-WORK-011, MVP-WORK-012

### Goal

Expose every Phase 2 `TaskManager` capability as a gRPC RPC on `TaskService`,
extend the existing message types for the new Task properties, and add error
mapping for `ErrBlocked` / `ErrInvalidRelationship` / `ErrTaskGroupNotFound`
/ `ErrAgentNotFound`.

### Files to create / modify

| File | Change |
|---|---|
| `proto/codevaldwork/v1/codevaldwork.proto` | Add `Task` fields (`due_at`, `tags`, `estimated_hours`, `context`, `completed_at`); drop `assigned_to` (now an edge); add `Agent`, `TaskGroup`, `Relationship` messages; add `Direction` enum |
| `proto/codevaldwork/v1/service.proto` | New RPCs (see table below) |
| `proto/codevaldwork/v1/errors.proto` | Add `BlockedByInfo` (from WORK-011), `InvalidRelationshipInfo` |
| `gen/go/codevaldwork/v1/` | Regenerate via `buf generate` |
| `internal/server/server.go` | Implement the new RPC handlers |
| `internal/server/errors.go` | Map new domain errors to gRPC codes (see table) |

### New RPCs on `TaskService`

| RPC | Domain method | Notes |
|---|---|---|
| `AssignTask(AssignTaskRequest)` | `AssignTask` | request: agency_id, task_id, agent_id |
| `UnassignTask(UnassignTaskRequest)` | `UnassignTask` | request: agency_id, task_id |
| `UpsertAgent(UpsertAgentRequest)` | `UpsertAgent` | returns the Agent (with server-assigned vertex ID) |
| `GetAgent(GetAgentRequest)` | `GetAgent` | by `(agency_id, agent_id)` natural key |
| `ListAgents(ListAgentsRequest)` | `ListAgents` | |
| `CreateTaskGroup(CreateTaskGroupRequest)` | `CreateTaskGroup` | |
| `GetTaskGroup(GetTaskGroupRequest)` | `GetTaskGroup` | |
| `UpdateTaskGroup(UpdateTaskGroupRequest)` | `UpdateTaskGroup` | |
| `DeleteTaskGroup(DeleteTaskGroupRequest)` | `DeleteTaskGroup` | |
| `ListTaskGroups(ListTaskGroupsRequest)` | `ListTaskGroups` | |
| `AddTaskToGroup(AddTaskToGroupRequest)` | `AddTaskToGroup` | |
| `RemoveTaskFromGroup(RemoveTaskFromGroupRequest)` | `RemoveTaskFromGroup` | |
| `ListTasksInGroup(ListTasksInGroupRequest)` | `ListTasksInGroup` | |
| `ListGroupsForTask(ListGroupsForTaskRequest)` | `ListGroupsForTask` | |
| `CreateRelationship(CreateRelationshipRequest)` | `CreateRelationship` | request: agency_id, from_id, to_id, label, properties |
| `DeleteRelationship(DeleteRelationshipRequest)` | `DeleteRelationship` | |
| `TraverseRelationships(TraverseRelationshipsRequest)` | `TraverseRelationships` | request: agency_id, vertex_id, label, direction |

The five Phase 1 RPCs (`CreateTask`, `GetTask`, `UpdateTask`, `DeleteTask`,
`ListTasks`) are unchanged in shape — only the `Task` message gains fields
and loses `assigned_to`.

### Error mapping additions

| Domain error | gRPC code | Detail proto |
|---|---|---|
| `ErrBlocked` | `FAILED_PRECONDITION` | `BlockedByInfo` |
| `ErrInvalidRelationship` | `INVALID_ARGUMENT` | `InvalidRelationshipInfo` |
| `ErrRelationshipNotFound` | `NOT_FOUND` | — |
| `ErrTaskGroupNotFound` | `NOT_FOUND` | — |
| `ErrTaskGroupAlreadyExists` | `ALREADY_EXISTS` | — |
| `ErrAgentNotFound` | `NOT_FOUND` | — |
| `ErrAgentAlreadyExists` | `ALREADY_EXISTS` | — |

### Acceptance tests

- `buf lint` passes; `buf generate` produces no diff after commit.
- All existing Phase 1 RPC tests still pass against the regenerated stubs
  (no behavioural regression on Task CRUD).
- Each new RPC has at least one happy-path test and one domain-error test.
- `BlockedByInfo` round-trips end-to-end on a real client (decode the detail
  from the `*status.Status`).

### Out of scope

- HTTP routes → MVP-WORK-015 (below).
- Streaming RPCs — not in the architecture for Phase 2.

---

## MVP-WORK-015 — HTTP convenience routes

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-015_http_routes`
**Depends on**: MVP-WORK-013

### Goal

Mirror the CodeValdComm pattern: declare an HTTP route per Phase 2 RPC in
`registrar.workRoutes()` so that CodeValdCross's dynamic proxy serves them
without any Cross-side code change. Phase 1 already declares two static
routes (`POST/GET /work/{agencyId}/tasks`); WORK-006 added schema-driven
entity routes via `schemaroutes.RoutesFromSchema`.

### Route table

All routes are mounted under `/work/`. Path bindings populate the request
proto fields named in the right-hand column.

| Method | Pattern | Capability | gRPC method | Path bindings |
|---|---|---|---|---|
| `PUT` | `/work/{agencyId}/tasks/{taskId}/assignee/{agentId}` | `assign_task` | `AssignTask` | `agencyId→agency_id`, `taskId→task_id`, `agentId→agent_id` |
| `DELETE` | `/work/{agencyId}/tasks/{taskId}/assignee` | `unassign_task` | `UnassignTask` | `agencyId→agency_id`, `taskId→task_id` |
| `PUT` | `/work/{agencyId}/agents/{agentId}` | `upsert_agent` | `UpsertAgent` | `agencyId→agency_id`, `agentId→agent.agent_id` |
| `GET` | `/work/{agencyId}/agents/{agentId}` | `get_agent` | `GetAgent` | |
| `GET` | `/work/{agencyId}/agents` | `list_agents` | `ListAgents` | |
| `POST` | `/work/{agencyId}/task-groups` | `create_task_group` | `CreateTaskGroup` | |
| `GET` | `/work/{agencyId}/task-groups/{taskGroupId}` | `get_task_group` | `GetTaskGroup` | |
| `PUT` | `/work/{agencyId}/task-groups/{taskGroupId}` | `update_task_group` | `UpdateTaskGroup` | |
| `DELETE` | `/work/{agencyId}/task-groups/{taskGroupId}` | `delete_task_group` | `DeleteTaskGroup` | |
| `GET` | `/work/{agencyId}/task-groups` | `list_task_groups` | `ListTaskGroups` | |
| `PUT` | `/work/{agencyId}/task-groups/{taskGroupId}/tasks/{taskId}` | `add_task_to_group` | `AddTaskToGroup` | |
| `DELETE` | `/work/{agencyId}/task-groups/{taskGroupId}/tasks/{taskId}` | `remove_task_from_group` | `RemoveTaskFromGroup` | |
| `GET` | `/work/{agencyId}/task-groups/{taskGroupId}/tasks` | `list_tasks_in_group` | `ListTasksInGroup` | |
| `GET` | `/work/{agencyId}/tasks/{taskId}/groups` | `list_groups_for_task` | `ListGroupsForTask` | |
| `POST` | `/work/{agencyId}/relationships` | `create_relationship` | `CreateRelationship` | |
| `DELETE` | `/work/{agencyId}/relationships/{label}/from/{fromId}/to/{toId}` | `delete_relationship` | `DeleteRelationship` | |
| `GET` | `/work/{agencyId}/vertices/{vertexId}/relationships/{label}` | `traverse_relationships` | `TraverseRelationships` | direction is a query param |

`IsWrite: true` on every mutation route. Read routes omit the flag.

### Files to modify

| File | Change |
|---|---|
| `internal/registrar/registrar.go` | Extend `workRoutes()` with the table above |
| `internal/registrar/registrar_test.go` (new if not present) | Snapshot test that the registered route count and capabilities match the table |

### Acceptance tests

- Cross dynamic-proxy integration: a request to each new route (mocked Cross
  handler) reaches the right gRPC method with the right path bindings.
- `RegisterRequest` round-trips with all routes — no Cross-side change is
  required.
- Snapshot test passes after the table change.

### Out of scope

- Cross-side handlers — none required; the dynamic proxy handles all routes.
- Body validation beyond what the gRPC handler already does.
