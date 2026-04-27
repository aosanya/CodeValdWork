# TaskGroup CRUD & `member_of` Edge

Topics: `TaskGroup` entity · `member_of` edge · Aggregate child progress

---

## MVP-WORK-012 — TaskGroup CRUD + `member_of` enforcement

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-012_task_group`
**Depends on**: MVP-WORK-008, MVP-WORK-009

### Goal

Add CRUD for the `TaskGroup` entity (sprints / milestones / epics) and the
`member_of` edge that links Tasks to a Group. The `TaskGroup` TypeDefinition
itself ships in [MVP-WORK-008](schema.md); this task wires up the Go API and
the edge enforcement specific to `member_of`.

### Files to create / modify

| File | Change |
|---|---|
| `taskgroup.go` (new) | `TaskGroup` struct + `taskGroupToProperties` / `*FromEntity` helpers |
| `task.go` | Add `CreateTaskGroup`, `GetTaskGroup`, `UpdateTaskGroup`, `DeleteTaskGroup`, `ListTaskGroups`, `AddTaskToGroup`, `RemoveTaskFromGroup`, `ListTasksInGroup`, `ListGroupsForTask` |
| `errors.go` | Add `ErrTaskGroupNotFound`, `ErrTaskGroupAlreadyExists` |
| `task_test.go` | Cover CRUD + membership flows on `fakeDataManager` |
| `storage/arangodb/arangodb_test.go` | End-to-end through the real backend |

### TaskManager additions

```go
type TaskManager interface {
    // ... existing methods ...

    CreateTaskGroup(ctx context.Context, agencyID string, g TaskGroup) (TaskGroup, error)
    GetTaskGroup(ctx context.Context, agencyID, groupID string) (TaskGroup, error)
    UpdateTaskGroup(ctx context.Context, agencyID string, g TaskGroup) (TaskGroup, error)
    DeleteTaskGroup(ctx context.Context, agencyID, groupID string) error
    ListTaskGroups(ctx context.Context, agencyID string) ([]TaskGroup, error)

    AddTaskToGroup(ctx context.Context, agencyID, taskID, groupID string) error
    RemoveTaskFromGroup(ctx context.Context, agencyID, taskID, groupID string) error
    ListTasksInGroup(ctx context.Context, agencyID, groupID string) ([]Task, error)
    ListGroupsForTask(ctx context.Context, agencyID, taskID string) ([]TaskGroup, error)
}
```

### `member_of` edge semantics

- Direction: **Task → TaskGroup**.
- A Task may belong to **many** Groups (no cardinality cap at this layer; the
  product can introduce one later if needed).
- `AddTaskToGroup` is idempotent — re-adding a Task already in the Group
  returns no error.
- `DeleteTaskGroup` removes the Group vertex **and** all inbound `member_of`
  edges — Tasks are not deleted, they just lose the membership.

### Aggregate child progress

The architecture mentions "parent task shows aggregate child progress" for
`subtask_of` — that's a **read concern** for UI consumers, not a write
concern. WORK-012 does not implement an aggregation API. Consumers compose
`ListTasksInGroup` (or `TraverseRelationships(..., "subtask_of", Inbound)`)
with their own progress calculation.

### Acceptance tests

- `CreateTaskGroup` with empty `Name` → `ErrInvalidTask` (re-use; or introduce
  `ErrInvalidTaskGroup` — choose at PR time, document the choice).
- `CreateTaskGroup` with duplicate ID → `ErrTaskGroupAlreadyExists`.
- `GetTaskGroup` for unknown ID → `ErrTaskGroupNotFound`.
- `AddTaskToGroup` then `ListTasksInGroup` → returns the Task.
- `AddTaskToGroup` twice → no error; only one `member_of` edge exists.
- `RemoveTaskFromGroup` then `ListTasksInGroup` → empty.
- `DeleteTaskGroup` removes vertex + all inbound `member_of` edges; the
  member Tasks themselves still exist (verify via `GetTask`).
- `member_of` from a `Task` to a non-`TaskGroup` target type → rejected by
  the WORK-009 edge-label whitelist (regression guard).

### Out of scope

- Group-of-Groups (Sprints containing Sprints) — not in the architecture.
- Group-level status / progress aggregation — read-side concern for UI.
- gRPC TaskGroup RPCs → [MVP-WORK-013](grpc-surface.md).
