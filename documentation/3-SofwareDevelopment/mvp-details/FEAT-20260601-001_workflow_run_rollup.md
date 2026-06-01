# FEAT-20260601-001 — WorkflowRun rollup endpoint (transactional rollback closure)

**Status:** 📋 Not Started
**Severity:** Medium — enables transactional rollback semantics for orchestrated pipelines; without it, partial-failure cleanup is a manual graph walk per service
**Owner:** CodeValdWork
**Estimated effort:** ~3–5 days (schema addition + traversal RPC + HTTP route + tests)
**Source finding:** This conversation (2026-06-01) — surfaced while wiring `next-task-selector` and discussing how the Part-G pipeline should be unwound when a single run fails partway through.

---

## Problem

When the Part-G pipeline executes one logical "run" (e.g. picking MVP-SF-001 → branch → todos → file writes → compile → merge), the resulting state is scattered across the agency graph as a constellation of entities and edges:

- the `Task` and any `TaskTodo` children,
- `assigned_to` edges to the agent,
- `depends_on` edges to / from other tasks,
- `has_todo` edges,
- the git branch reference (held outside CodeValdWork but pointed at by `branch_name`),
- compile jobs / merge jobs (CodeValdFunctions),
- AgentRuns (CodeValdAI).

There is currently **no anchor that names this set as one logical unit**, and there is no endpoint that returns the full closure. Operators wanting to roll back a failed run must hand-walk every neighbour collection.

## Goal

Introduce a first-class **`WorkflowRun`** entity in CodeValdWork that anchors the closure of a single orchestrated execution, and expose an endpoint that returns the run plus every entity and edge attached to it — enough information for the caller to reconstruct (or undo) the run as a transaction.

Why `WorkflowRun` and not `AgentRun`:

- Some pipelines are agent-free (e.g. `next-task-selector` → assign → branch create — all function-driven). `AgentRun` is CodeValdAI-specific and absent for those runs.
- `WorkflowRun` lives in CodeValdWork (the orchestrator), so it can anchor mixed AI / function / human flows uniformly.

## Non-goals

- Implementing the rollback **execution** itself. This feature returns the closure; the rollback transaction (event compensations, edge deletions, branch revert) is a follow-up feature once the closure shape is settled.
- Cross-service rollups. CodeValdWork returns its own entities and the IDs of foreign entities (git branch name, function job ID, AgentRun ID); the caller fans out to those services if needed.

---

## Design

### Option A — Add `WorkflowRun` as a graph entity (preferred)

New entity type in [`schema.go`](../../schema.go), persisted in a new collection (e.g. `work_workflow_runs`). Created at the start of a workflow (e.g. by `next-task-selector` when it assigns a task), referenced via edges to every entity it produces.

Edges introduced:

```
WorkflowRun ──started_task───────► Task              (1:N)
WorkflowRun ──started_todo───────► TaskTodo          (1:N, optional — usually reached via Task)
WorkflowRun ──linked_run─────────► AgentRun ID       (cross-service: store ID property, no edge object)
WorkflowRun ──linked_job─────────► FunctionsJob ID   (cross-service: store ID property)
WorkflowRun ──linked_branch──────► branch name       (cross-service: store name property)
```

Inverse edge:

```
Task / TaskTodo ──part_of_run────► WorkflowRun       (lookup: "which run created this?")
```

`WorkflowRun` properties: `id`, `agency_id`, `started_at`, `completed_at`, `status` (pending / in_progress / completed / failed / rolled_back), `trigger_event` (e.g. `work.next.requested`), `initiator` (caller identifier or empty), `notes`.

### Option B — Query-only (no schema change)

Skip the new entity. Add a single endpoint that, given a `task_id`, traverses every reachable edge in CodeValdWork (Task → TaskTodos, assigned_to, depends_on inbound + outbound, has_todo) and returns the closure. Cheaper to ship; loses the "this is one run" anchor for mixed pipelines and offers no path for the eventual rollback action.

**Recommendation:** Option A. The cost of the schema addition is small (one entity, three relationship definitions) and it gives the future rollback feature a place to record its compensating-action log. Option B paints us into a corner — every later rollback feature has to retrofit an anchor.

### RPC + HTTP surface

Proto addition in [`proto/codevaldwork/v1/service.proto`](../../proto/codevaldwork/v1/service.proto):

```proto
rpc GetWorkflowRun(GetWorkflowRunRequest) returns (WorkflowRunClosure);

message GetWorkflowRunRequest {
  string agency_id     = 1;
  string workflow_run_id = 2;
}

message WorkflowRunClosure {
  WorkflowRun run               = 1;
  repeated Task tasks           = 2;
  repeated TaskTodo todos       = 3;
  repeated Relationship edges   = 4;  // every edge whose from_id or to_id is in the closure
  repeated string agent_run_ids = 5;  // foreign references
  repeated string function_job_ids = 6;
  repeated string branch_names  = 7;
}
```

HTTP route (registered via `RegisterRequest`):

```
GET /work/{agencyId}/workflow-runs/{workflowRunId}
```

Optional companion lookups (Phase 2, not required for v1):

```
GET /work/{agencyId}/workflow-runs                          — list runs by agency
GET /work/{agencyId}/tasks/{taskId}/workflow-runs           — runs that touched a task
```

### Edge inclusion rule

The endpoint returns **all edges** whose endpoints sit in the closure — including edges to entities outside the closure (e.g. a task in the closure depending on a task outside it). This matters for the rollback transaction: it needs to know "this run added an inbound `depends_on` edge to TaskX outside the closure" in order to compensate.

---

## Implementation plan

### Phase 1 — Schema

1. Add `TypeWorkflowRun` to [`schema.go`](../../schema.go) `TypeDefinitions` with `StorageCollection: "work_workflow_runs"`. Properties per the Design section.
2. Add the new `RelLabel*` constants for `started_task`, `started_todo`, `part_of_run` (the inverse).
3. Add the `RelationshipDefinitions` entries (the existing pattern in `schema.go` covers symmetric pair declarations).
4. Add a `WorkflowRun` value type to [`models.go`](../../models.go).
5. Add `ErrWorkflowRunNotFound` sentinel to [`errors.go`](../../errors.go).

### Phase 2 — Manager

1. New file `workflow_run.go` next to `assignment.go` with:
   - `CreateWorkflowRun(ctx, agencyID, triggerEvent, initiator) (*WorkflowRun, error)`
   - `LinkTaskToRun(ctx, agencyID, runID, taskID) error` — writes `started_task` + inverse `part_of_run`.
   - `LinkTodoToRun`, `LinkAgentRun`, `LinkFunctionJob`, `LinkBranch` (the last three update properties; no graph edges to foreign entity IDs).
   - `GetWorkflowRunClosure(ctx, agencyID, runID) (*WorkflowRunClosure, error)` — the read.
2. Closure traversal:
   - Start at the run node; walk `started_task` to get tasks.
   - For each task, walk `has_todo`, `assigned_to`, `depends_on` (both directions).
   - Collect every visited edge.
   - Property lookups for foreign IDs.
3. Self-publish a domain event when a run completes / fails (`work.run.completed`, `work.run.failed`). Out of scope for the rollback feature itself but cheap to wire while we're here.

### Phase 3 — Wiring producers

`next-task` function ([`/CodeValdFunctions/functions/next-task`](../../../../CodeValdFunctions/functions/next-task)) becomes a `WorkflowRun` producer: on success it first creates a `WorkflowRun` via the new RPC, then `LinkTaskToRun(runID, pickedTaskID)`, and embeds `workflow_run_id` in the PUT-assign payload so downstream handlers can chain through.

Optional: any handler that creates Tasks (e.g. `developer-assigned-handler` decomposition) reads `workflow_run_id` from event context and calls `LinkTodoToRun` for each TaskTodo it creates. If absent, the run anchor is just less complete — graceful degradation.

### Phase 4 — Tests

- Unit: `workflow_run_test.go` covers create, link variants, closure traversal happy path (1 task + 3 todos + 2 deps), partial closure (run that links a task in another run), idempotency (re-linking the same task is a no-op).
- Integration: end-to-end with ArangoDB — create run, link entities, GET closure, assert every produced edge is present.

---

## Verification

- `go build ./...` clean
- `go vet ./...` clean
- `go test -race -count=1 ./...` clean
- Integration: pick MVP-SF-001 via `next-task` → GET `/work/utility-app-builder/workflow-runs/{runID}` → response contains the task, its todos, the agent-assign edge, the `depends_on` outbound edges, and the branch name string.

---

## Open design questions

1. **Run creation point.** Should `WorkflowRun` be created by `next-task-selector` (the first WorkPlan), or by the very first event that hits CodeValdWork in the run (e.g. `work.task.assigned`)? Latter is more uniform but couples to the entrypoint less explicitly.
2. **Trigger event identity.** A run is one logical execution; how do we tell "is this the same run as the prior `work.next.requested`?" if the user fires the trigger twice? Proposal: idempotency key on the publish payload — the function checks for an existing in-progress run before creating a new one. Document in the v1 spec.
3. **Foreign reference policy.** Store AgentRun and FunctionJob references as opaque strings (the design above) or introduce typed graph edges with cross-service ID format? Opaque keeps services decoupled; typed lets the graph store guarantee referential integrity. Recommendation: opaque for v1, revisit if rollback execution needs typed edges.
4. **Failed-run retention.** Rolled-back runs — keep `WorkflowRun` for audit, or hard-delete to keep the graph clean? Likely keep + flag (`status: rolled_back`), but confirm before Phase 1 lands.

## Dependencies

None blocking. Lands cleanly alongside existing schema; no breaking changes to existing entities. Phase 3 (producer wiring) depends on Phase 1+2 only.

## Future follow-ups

- Rollback action — given a `WorkflowRun`, execute compensating events for every entity in the closure (delete TaskTodos, unset branch on Task, revert git branch via CodeValdGit, fail AgentRuns, etc.). Separate feature; this endpoint is its prerequisite.
- Frontend visualisation — a "Run Detail" page in CodeValdWorkFrontend that renders the closure as a graph.
