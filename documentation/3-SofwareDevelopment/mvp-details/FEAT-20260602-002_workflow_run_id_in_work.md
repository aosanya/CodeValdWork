# FEAT-20260602-002 — `workflow_run_id` propagation in CodeValdWork

**Status:** 📋 Not Started
**Severity:** High — sibling of the umbrella; Tasks and TaskTodos are the first artifacts on the chain, so if Work doesn't propagate the ID, every downstream service inherits an empty value
**Owner:** CodeValdWork
**Estimated effort:** ~2 days (schema + proto + handlers + list filter + event emission + integration tests)
**Source finding:** This conversation (2026-06-02) — sibling of [umbrella FEAT-20260602-001 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md)

---

## Problem

CodeValdWork is the first downstream service in every pipeline (Functions's `start-pipeline` creates the `WorkflowRun`, then `next-task-selector` runs and assigns the first Task in CodeValdWork). If `workflow_run_id` is not a first-class field on Task and TaskTodo, the chain breaks here — no amount of work in AI, Functions, Git, or Comm can recover the link.

## Goal

Make `workflow_run_id` a first-class typed field on:

- `Task` entity (collection `work_tasks`)
- `TaskTodo` entity (collection `work_task_todos`)
- Every `work.task.*` event payload (`work.task.created`, `work.task.assigned`, `work.task.completed`, `work.task.failed`)
- Every `work.todo.*` event payload (`work.todo.created`, `work.todo.dispatched`, `work.todo.completed`, `work.todo.failed`)
- `work.next.requested` (existing topic, gains the field)

Plus a query filter: `GET /work/{agencyId}/tasks?workflow_run_id=X`, `GET /work/{agencyId}/todos?workflow_run_id=X`.

## Non-goals

- Backfilling existing rows. They keep `workflow_run_id = ""`. `make reset-db` wipes them in dev.
- Treating Tasks-without-run as orphans. Behaviour for `workflow_run_id = ""` is unchanged from today.

---

## Design

### Schema changes

In [`schema.go`](../../schema.go):

```go
// Task TypeDefinition properties
{Name: "workflow_run_id", Type: types.PropertyTypeString},

// TaskTodo TypeDefinition properties
{Name: "workflow_run_id", Type: types.PropertyTypeString},
```

The `started_task` / `started_todo` edges already link `WorkflowRun → Task / TaskTodo` ([relationship.go:91-101](../../relationship.go#L91-L101)). The new property is **denormalisation for query performance** — the umbrella's chain-through rule says every artifact must carry it, and querying by property is cheaper than walking edges across collections.

### Model changes

`Task` and `TaskTodo` in [`models.go`](../../models.go) gain `WorkflowRunID string \`json:"workflow_run_id"\``.

### Proto changes

In `proto/codevaldwork/v1/`:

- `Task` message: add `string workflow_run_id = N;`
- `TaskTodo` message: add `string workflow_run_id = N;`
- `CreateTaskRequest` accepts `string workflow_run_id` (optional).
- `AssignTaskRequest` accepts `string workflow_run_id` (overrides if set; otherwise keeps existing value).
- `ListTasksRequest` accepts `string workflow_run_id` as a filter.

Regenerate via `make proto`.

### Event payload changes

Every event payload struct in [`internal/pubsub/`](../../internal/pubsub/) (or wherever the publish helpers live) gains:

```go
type TaskAssignedEvent struct {
    WorkflowRunID string `json:"workflow_run_id"`
    // ...existing fields
}
```

Identical addition on:

- `TaskCreatedEvent`, `TaskAssignedEvent`, `TaskCompletedEvent`, `TaskFailedEvent`
- `TodoCreatedEvent`, `TodoDispatchedEvent`, `TodoCompletedEvent`, `TodoFailedEvent`
- `NextRequestedEvent` (the `work.next.requested` payload that `start-pipeline` emits and `next-task-selector` consumes)

### Chain-through behaviour

Per the umbrella's [§4 chain-through rule](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md):

- When `CreateTask` is called with `workflow_run_id`, persist on the row AND `LinkTaskToRun` (the existing edge — keep both for now, the edge supports closure traversal, the property supports fast filter).
- When `AssignTask` is called and the inbound event has `workflow_run_id` but the existing Task row's `workflow_run_id` is empty, set it (a Task being assigned via `next-task-selector` inherits the run-id from the inbound event).
- When `CreateTaskTodo` is called as part of AI decomposition (`ai.run.completed` → CodeValdWork creates todos), copy `workflow_run_id` from the parent Task onto each new TaskTodo.
- When any `work.*` event is published, set `workflow_run_id` from the entity being acted on.

### List filter

`ListTasks(ListTasksRequest{WorkflowRunID: "X"})` — if set, return only tasks where the property matches. Existing filters (status, assigned_to) stack with this one (AND semantics).

Same for `ListTaskTodos`.

### `next-task` selector update

The function in [`/CodeValdFunctions/functions/next-task`](../../../../CodeValdFunctions/functions/next-task) currently picks the lowest-named task and assigns it. After this FEAT, it reads `workflow_run_id` from the inbound `work.next.requested` event and passes it through in the assign call — so the assigned task inherits the run-id.

---

## Implementation plan

### Phase 1 — Schema + proto (0.5 day)

1. Add property to `schema.go` for Task and TaskTodo.
2. Add field to `models.go`.
3. Add proto field to `Task`, `TaskTodo`, `CreateTaskRequest`, `AssignTaskRequest`, `ListTasksRequest`, `ListTaskTodosRequest`.
4. `make proto`.

### Phase 2 — Manager + handlers (1 day)

1. Update `CreateTask`, `AssignTask`, `CreateTaskTodo` in [`task.go`](../../task.go) and decomposition path.
2. Update event publish helpers — every emit reads `workflow_run_id` from the entity.
3. Add list filters.

### Phase 3 — Tests (0.5 day)

- Unit: create task with run-id → persists; without → empty; list filter returns only matching.
- Integration: full chain — pipeline created → task created with run-id → assign event carries it → todo creates inherit → completed event carries it.

### Phase 4 — Cleanup (parallel)

The existing `started_task` / `started_todo` edges remain — they support the [closure traversal](../../workflow_run.go) for `GetWorkflowRunClosure`. The new property is additive; no migration needed.

---

## Verification

- `go test -race -count=1 ./...` clean.
- Integration test publishes `work.pipeline.requested` → `WorkflowRun` created → `work.next.requested` carries run-id → `work.task.assigned` carries run-id → `work.todo.created` (after decomposition) carries run-id.
- `GET /work/utility-app-builder/tasks?workflow_run_id=$RUN` returns exactly the tasks the run produced.

---

## Open design questions

1. **Reassignment policy.** If a Task already has `workflow_run_id = X` and `AssignTask` arrives with `workflow_run_id = Y`, do we (a) error, (b) overwrite, or (c) ignore the new value? Recommend (a) — a task belonging to two runs breaks the rollback invariant.
2. **Decomposition split.** AI decomposition produces N todos from one task; all inherit the task's `workflow_run_id`. What if the AI decides to also create a *sibling task* (not a todo of the original)? That sibling joins the same run by default — confirm in the AI sibling FEAT.

---

## Dependencies

- Part of umbrella: [FEAT-20260602-001 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md).
- Blocked by: [FEAT-20260602-001 (start-pipeline)](../../../../CodeValdFunctions/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_start_pipeline_function.md) — without `start-pipeline` no inbound event carries a run-id to propagate.
- Builds on: [FEAT-20260601-001 (WorkflowRun entity)](FEAT-20260601-001_workflow_run_rollup.md).
- Pairs with: [AI sibling FEAT](../../../../CodeValdAI/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_in_ai.md), [Functions sibling FEAT](../../../../CodeValdFunctions/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-002_workflow_run_id_in_functions.md), [Git sibling FEAT](../../../../CodeValdGit/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_in_git.md), [Comm sibling FEAT](../../../../CodeValdComm/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_in_comm.md).
