# Agent Vertex & AssignTask Edge

Topics: `Agent` entity · `assigned_to` graph edge · `UpsertAgent` · `AssignTask`

---

## MVP-WORK-010 — Agent vertex + `assigned_to` edge

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-010_agent_assignment`
**Depends on**: MVP-WORK-008, MVP-WORK-009

### Goal

Replace the Phase 1 `Task.AssignedTo` string field with a graph edge from the
Task vertex to a first-class `Agent` vertex. The `Agent` TypeDefinition lands
in [MVP-WORK-008](schema.md); this task wires up the Go API and storage edge.

This is a **breaking schema change** — tolerated under the greenfield
assumption (no deployed data). Phase 1 callers that set `Task.AssignedTo` need
to migrate to `AssignTask` after this task lands.

### Files to create / modify

| File | Change |
|---|---|
| `agent.go` (new) | `Agent` struct (Go peer of the TypeDefinition); `agentToProperties` / `agentFromEntity` helpers |
| `task.go` | Add `UpsertAgent`, `GetAgent`, `ListAgents`, `AssignTask`, `UnassignTask` to `TaskManager`; remove `AssignedTo` writes from `taskToProperties` |
| `types.go` | Remove `Task.AssignedTo` (or deprecate as ignored on read for one cycle, then delete — pick one and document) |
| `errors.go` | Add `ErrAgentNotFound`, `ErrAgentAlreadyExists` |
| `task_test.go` | Round-trip Agent CRUD and `AssignTask` through `fakeDataManager` |
| `storage/arangodb/arangodb_test.go` | End-to-end via real backend |

> **Decision needed at implementation time**: keep or delete
> `Task.AssignedTo`. The architecture is unambiguous (it's an edge); leaving a
> deprecated field invites callers to set it. Recommend **delete** since there
> is no deployed data — but flag at PR time.

### TaskManager additions

```go
type TaskManager interface {
    // ... existing methods ...

    UpsertAgent(ctx context.Context, agencyID string, agent Agent) (Agent, error)
    GetAgent(ctx context.Context, agencyID, agentID string) (Agent, error)
    ListAgents(ctx context.Context, agencyID string) ([]Agent, error)

    AssignTask(ctx context.Context, agencyID, taskID, agentID string) error
    UnassignTask(ctx context.Context, agencyID, taskID string) error
}
```

### `UpsertAgent` semantics

- Looks up existing Agent by `(agencyID, agentID)` (the natural key).
- If absent, creates a new Agent vertex and returns it.
- If present, replaces `displayName` and `capability` and returns the updated
  Agent.
- `agentID` is immutable post-create — attempting to change it via
  `UpsertAgent` is a no-op for that field.

`(agencyID, agentID)` uniqueness is enforced by an `AQL` lookup; if the
SharedLib `entitygraph` interface gains a "find by property" capability, swap
to that. Surface as a SharedLib opportunity at implementation time.

### `AssignTask` semantics

- Validates Task exists (`ErrTaskNotFound`).
- Validates Agent exists (`ErrAgentNotFound`).
- Removes any existing outbound `assigned_to` edge from the Task (a Task has
  at most one assignee).
- Creates a new `assigned_to` edge: `Task --[assigned_to{assignedAt: now}]--> Agent`.
- Publishes `work.task.assigned` once [MVP-WORK-014](pubsub.md) lands.

`UnassignTask` is the inverse — deletes the outbound `assigned_to` edge if
one exists. Idempotent.

### Read path

`GetTask` and `ListTasks` no longer return an `AssignedTo` field. Callers
that need the assignee call:

```go
edges, _ := tm.TraverseRelationships(ctx, agencyID, taskID, "assigned_to", DirectionOutbound)
// edges has 0 or 1 entries — agent ID is in edges[0].ToID
```

If a Task-with-assignee read is common enough to justify a helper, expose
`GetTaskWithAssignee(ctx, agencyID, taskID) (Task, *Agent, error)` — but
**only** add it if a real caller appears during integration testing
(WORK-016). Don't speculate.

### Acceptance tests

- `UpsertAgent` twice with the same `agentID` — second call returns the
  updated Agent; only one `Agent` vertex exists for that `(agencyID, agentID)`.
- `AssignTask` to an unknown agent — returns `ErrAgentNotFound`.
- `AssignTask` to a known agent — `assigned_to` edge exists; previous edge
  (if any) is gone.
- Re-assigning an already-assigned Task replaces the edge; only one outbound
  `assigned_to` edge remains.
- `UnassignTask` on an unassigned Task — no error (idempotent).
- Cross-agency assignment (Task in agency A, Agent in agency B) — returns
  `ErrInvalidRelationship` (covered by WORK-009 whitelist; assert here too).
- `Task` returned by `GetTask` does **not** carry an `AssignedTo` string field.

### Out of scope

- Mass-reassign / bulk operations.
- Agent capability filtering / matching to Task — Phase 1 leaves agent
  selection to the caller.
- Publishing `work.task.assigned` — wired in [MVP-WORK-014](pubsub.md).
