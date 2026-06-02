---
status: 📋 Draft (2026-06-02)
owner: CodeValdWork
scope: task & todo failure events and WorkflowRun terminal-state transitions
source: gap analysis of `/4-QA/agencies/utility-app-builder/09`
---

# Task & Todo Failure Modes

CodeValdWork owns the **Task ↔ TaskTodo ↔ WorkflowRun** lifecycle. This doc
catalogues the failure events Work emits, the terminal-state transitions Work
is responsible for, and the field contracts that recovery pipelines (per
[FEAT-20260602-005](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md))
must satisfy when they synthesize Work's success events.

The orchestration overview lives in
[CodeValdCross — pipeline-failure-handling](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/pipeline-failure-handling.md).

---

## Failure events Work emits

| Event | When emitted | `payload` fields |
|---|---|---|
| `work.task.assignment_failed` | Assignment to a missing agent or one with empty `role_name`. | `task_id`, `agent_id`, `reason` ∈ {`agent_missing`, `missing_role`, `agent_deleted`} |
| `work.task.failed` | Task is flipped to `FAILED`. Emitted by Work itself or in response to `work.task.fail`. | `task_id`, `workflow_run_id`, `reason` |
| `work.todo.failed` | Todo is flipped to `FAILED`. Wraps `ai.run.failed` for todo runs. | `todo_id`, `parent_task_id`, `workflow_run_id`, `reason` |
| `work.todo.cascade_cancelled` | Todo cancelled because a predecessor in its `depends_on` chain failed. | `todo_id`, `parent_task_id`, `cancelled_due_to_predecessor_id`, `workflow_run_id` |
| `work.run.failed` | WorkflowRun reached `failed` terminal state. | `workflow_run_id`, `reason`, `failing_task_ids` |
| `work.run.timeout` | Watchdog detected stale run (G1 in centerpiece). Same shape as `failed` with `reason: stale`. | `workflow_run_id` |
| `work.run.cancelled` | Operator cancelled mid-flight (G3). | `workflow_run_id`, `cancelled_by` |

---

## Field contracts for synthesized success events

Recovery pipelines must produce these events with these fields when
synthesizing a Work-owned success. **Must produce** fields are read by
downstream handlers; **may differ** fields are informational.

### `work.task.completed`

Listened for by: `compile-on-todo-completed` (indirectly via
`work.todo.completed`), the closure SSE aggregator,
`maybeCompleteWorkflowRun`.

- **Must produce:** `task_id`, `workflow_run_id`
- **May differ:** `completed_at`, `final_status`, `summary`

### `work.todo.completed`

Listened for by: `compile-on-todo-completed`.

- **Must produce:** `todo_id`, `parent_task_id`, `todo_type`, `workflow_run_id`
- **May differ:** `run_count`, `completed_at`, `output_excerpt`

### `work.task.assigned`

Listened for by: `developer-assigned-handler` (CodeValdAI),
`work-todo-handler` (CodeValdAI).

- **Must produce:** `task_id`, `agent_id`, `role_name`, `task_name`, `title`,
  `description`, `workflow_run_id`
- **May differ:** `assigned_at`

> Recovery pipelines that synthesize `work.task.assigned` must pull the agent
> via the upsert PUT (per 09 setup §10b) so `role_name` is never empty.

---

## Failure-pipeline triggers Work depends on

Per FEAT-005, every Work step declares a `failure_event` and a recovery
pipeline. The defaults for the utility-app-builder agency:

| Plan / step | `failure_event` | Default recovery |
|---|---|---|
| `developer-assigned-handler` (decomp dispatch) | `ai.run.failed` (decomp) | `decomp-solving-problem` |
| `work-todo-handler` (impl dispatch) | `ai.run.failed` (todo) | `impl-solving-problem` |
| Task-level (parent failure) | `work.task.failed` | `default-failure-pipeline` (publishes `work.run.failed`) |
| Todo-level (single-todo failure) | `work.todo.failed` | (none — bubbles to task-level via `maybeCompleteParentTask`) |

When `work.task.fail` is published by an upstream service (e.g. CodeValdFunctions
when `compile-fix max_runs` is exhausted), Work subscribes and flips the task
to `FAILED`. This emits `work.task.failed`, which then routes to the task's
declared failure pipeline.

---

## Terminal closure rules (`maybeCompleteWorkflowRun`)

CodeValdWork is the sole producer of `work.run.{completed,failed,cancelled}`.
Implementation (proposed; not yet built):

```go
// internal/server/run_lifecycle.go (new file)

// Called from every task-terminal handler. Idempotent.
func (m *workManager) maybeCompleteWorkflowRun(ctx context.Context, runID string) {
    run, err := m.GetWorkflowRun(ctx, runID)
    if err != nil || run.IsTerminal() { return }

    tasks, _ := m.ListTasksByWorkflowRun(ctx, runID)
    if !allTerminal(tasks) { return }

    switch {
    case allCompleted(tasks):
        m.setRunStatus(runID, WorkflowRunStatusCompleted)
        m.publish(TopicWorkRunCompleted, runID)
    case anyFailed(tasks):
        m.setRunStatus(runID, WorkflowRunStatusFailed)
        m.publish(TopicWorkRunFailed, runID, reason)
    case allCancelled(tasks):
        m.setRunStatus(runID, WorkflowRunStatusCancelled)
        m.publish(TopicWorkRunCancelled, runID)
    }
}
```

Call sites:

- `task_impl_task.go:maybeCompleteParentTask` after flipping a parent task
  to `COMPLETED`
- new `setTaskStatus(FAILED)` hook (today an internal helper in
  `assignment.go`; needs to be package-level — see [BUG-09-024 step 2 refactor](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/bugs/09-mvp-sf-pipeline-findings.md))
- the watchdog-handler in the Cross sweeper (G1)
- the cancel handler (TF-6 below)

---

## TF-N — Work-specific failure modes (and which recovery pipeline owns them)

### TF-1 — Assignment to a missing / role-less agent

**Trigger:** `AssignTask` validates agent exists AND has non-empty
`role_name`. If not, emit `work.task.assignment_failed`. Today: silent
(09 setup §10b workaround re-PUTs the agent).

**Recovery:** the task's `on_failure_pipeline` runs. Default is
`default-failure-pipeline` (publishes `work.run.failed`).

### TF-2 — `IN_PROGRESS` task with no AI run

**Trigger:** `work.task.assigned` fired, but `ai.run.created` never
followed. Detected by the watchdog (G1). Emits `work.run.timeout` →
CodeValdWork flips affected tasks to `FAILED` → recovery pipeline fires.

### TF-3 — `maybeCompleteParentTask` never fires

**Trigger:** todos complete out of order or a completion event is lost.
Parent stays `IN_PROGRESS`. Fix: add `pending_todo_count` property to
`Task` schema, maintained transactionally in `assignment.go`. Trigger
parent completion when count == 0.

### TF-4 — Dependent-todo cascade on todo failure

**Trigger:** todo #1 fails; todo #2 has `depends_on: [1]`. Today the
dispatcher releases #2 because `failed` is terminal. Fix: rename
`isTerminalStatus` → `isCompletedStatus` and only release dependents on
predecessor `completed`. On predecessor `failed` / `cancelled`, mark
dependent `cancelled` and emit `work.todo.cascade_cancelled`.

Same rule applies to the dependent-task auto-unblock listener
([BUG-09-024](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/bugs/09-mvp-sf-pipeline-findings.md)).

### TF-5 — `compile-fix max_runs` exhausted (legacy path)

**Trigger:** CodeValdFunctions publishes `work.task.fail` when the third
`compile-fix` todo has run.

**Post FEAT-005:** the `compile-on-todo-completed` plan's
`compile-solving-problem` recovery pipeline encapsulates the entire fix
loop. `work.task.fail` is no longer needed; if the recovery pipeline
gives up, it triggers its own failure pipeline (`compile-escalate-to-operator`)
which publishes `work.run.failed`.

The current code path stays in place during migration: Work subscribes to
`work.task.fail` and flips the task to `FAILED` so legacy plans keep
working until the migration completes.

### TF-6 — Operator-triggered cancel of an in-flight run

**Trigger:** `POST /work/{a}/workflow-runs/{id}/cancel`. Validates
`run.status == in_progress`. Flips to `cancelling`. Publishes
`work.run.cancelling`. For every non-terminal task in the run, flips to
`cancelled` and emits `work.task.cancelled`. The AI/Functions subscribers
handle this by cancelling in-flight runs/jobs (see per-service docs).
After T seconds (default 30), flips run to `cancelled`. Emits
`work.run.cancelled`.

Distinct from rollback: cancel stops the pipeline mid-run without
reverting artefacts. Rollback runs only after a terminal state.

### TF-7 — Agent deleted / renamed mid-run

**Trigger:** assignment publish always re-reads the agent. If missing,
emit `work.task.assignment_failed { reason: agent_deleted }`. Alternative:
forbid agent delete while any non-terminal task is assigned to it (return
409). Simpler; lower blast radius.

---

## QA additions

A new file `/4-QA/agencies/utility-app-builder/09/09-work-99-failure-paths.md`
exercises each TF-N:

| Step | Failure forced | Expected recovery |
|---|---|---|
| 99-1 | Assign to a role-less agent | `default-failure-pipeline` → `work.run.failed`, reason=`missing_role` |
| 99-2 | Kill the LLM provider mid-decomp | watchdog → `work.run.timeout`, then run terminates `failed` |
| 99-3 | Force todo #1 to fail | TF-4 cascade-cancel + `work.run.failed` |
| 99-4 | 3 × `compile-fix` exhaust max_runs | `compile-solving-problem` then `compile-escalate-to-operator` → `work.run.failed` |
| 99-5 | Operator cancels mid-run | TF-6 cancel endpoint → `work.run.cancelled` |
| 99-6 | Delete agent during in-flight task | TF-7 → `work.run.failed`, reason=`agent_deleted` |

Each step uses the same PubSub catch-all dump pattern as the existing 09
steps so future debugging has the full evidence.

---

## Open follow-ups

- `pending_todo_count` schema migration — add property to `Task` schema in
  [schema.md](schema.md).
- Decision: should `BLOCKED → CANCELLED` cascade fire automatically when a
  predecessor fails, or wait for the operator? Default proposal: auto-cancel
  with `reason: predecessor_failed`.
- `setTaskStatus` helper currently lives file-private in `assignment.go`;
  move package-level so `run_lifecycle.go` can call it (same refactor flagged
  in BUG-09-024 step 2).
- Idempotency key on `POST .../cancel` to avoid double-cancel races if the UI
  button is double-clicked.

---

## Related work

- [Cross — pipeline-failure-handling](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/pipeline-failure-handling.md)
- [Cross — FEAT-20260602-005 — failure pipelines via synthesized success events](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md)
- [FEAT-20260602-003 — workflow_run status state machine](FEAT-20260602-003_workflow_run_status_state_machine.md)
- [FEAT-20260602-004 — workflow_run rollback semantics](FEAT-20260602-004_workflow_run_rollback_semantics.md)
- [BUG-09-024 — auto-unblock listener](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/bugs/09-mvp-sf-pipeline-findings.md)
- [schema.md](schema.md) — where `pending_todo_count` will be added
