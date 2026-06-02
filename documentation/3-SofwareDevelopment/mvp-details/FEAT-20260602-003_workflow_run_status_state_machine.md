# FEAT-20260602-003 — `WorkflowRun` status state machine

**Status:** 📋 Not Started — design (Open design questions still being worked through)
**Severity:** Medium — without state machine transitions, every run displays as `pending` forever in the UI; status is the primary signal an operator uses to decide "is this still running, did it succeed, do I need to roll it back"
**Owner:** CodeValdWork (entity owner)
**Estimated effort:** ~2 days (event subscription wiring + transition handler + terminal-event configuration + tests)
**Source finding:** This conversation (2026-06-02) — surfaced while researching the WorkflowRun lifecycle; `CreateWorkflowRun` writes `pending` but nothing ever advances the state

---

## Problem

The `WorkflowRun` entity has five lifecycle states defined in [`models.go:383-405`](../../models.go#L383-L405):

```
pending → in_progress → completed | failed | rolled_back
```

But there is no code that writes any transition. Newly-created runs sit at `pending` forever. As a result:

- The `/workflow-runs` UI shows every row as `pending` regardless of actual pipeline state.
- Closure aggregation ([FEAT-20260602-003 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-003_workflow_run_closure_sse_aggregation.md)) has no way to know "the run is done, stop polling."
- Rollback has no terminal-state contract to react to.

## Goal

Make CodeValdWork the **authoritative writer** of `WorkflowRun.status` by subscribing to a curated set of inbound events and transitioning the row on signal.

Transitions:

| From | To | On |
|---|---|---|
| `pending` | `in_progress` | First `work.task.assigned` for a Task whose `workflow_run_id` matches |
| `in_progress` | `completed` | Declared `terminal_event` matches (see below) |
| `in_progress` | `failed` | Any `work.task.failed`, `functions.job.failed`, `git.merge.failed`, `ai.run.failed` with matching `workflow_run_id` |
| `in_progress` | `rolled_back` | Explicit `POST /workflow-runs/{id}/rollback` (see [FEAT-20260602-004 (rollback)](FEAT-20260602-004_workflow_run_rollback_semantics.md)) |

## Non-goals

- The rollback action itself — only the state transition into `rolled_back`. The compensation logic is owned by [FEAT-20260602-004](FEAT-20260602-004_workflow_run_rollback_semantics.md).
- Re-deriving status from closure traversal on each read. The status is a stored property updated by transitions; it does not need to be recomputed.

---

## Design

### Authoritative writer

CodeValdWork owns the `WorkflowRun` entity and therefore owns the writes. It subscribes to a curated set of topics on the bus and applies transitions. Cross is **not** the writer — it stays a transport.

### Subscription contract

In `internal/registrar/registrar.go`, add to Consumes:

```
work.task.assigned
work.task.failed
functions.job.failed
ai.run.failed
git.merge.failed
```

Plus a configurable `terminal_event` per run (see below).

### Terminal event configuration

A run knows it's done when a specific event happens. For Part-G of scenario 09, that's `functions.job.completed` with `function_name=merge-flutter-branch` and `status=ok`. For other pipelines it'll be different.

Encode the terminal event as a new optional property on `WorkflowRun`:

```go
// schema.go — WorkflowRun PropertyDefinitions
{Name: "terminal_event", Type: types.PropertyTypeString},
//   e.g. "functions.job.completed:function_name=merge-flutter-branch:status=ok"
```

If `terminal_event` is empty, the run never auto-completes — operator must close it manually via `POST /workflow-runs/{id}/complete`. Recommended for ad-hoc/CLI flows where the terminal event isn't predictable.

The `start-pipeline` function ([FEAT-20260602-001](../../../../CodeValdFunctions/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_start_pipeline_function.md)) accepts `terminal_event` in the `work.pipeline.requested` payload and passes it through.

### Transition algorithm

On every inbound event with `workflow_run_id = X`:

```go
run, _ := mgr.GetWorkflowRun(ctx, agencyID, X)
switch run.Status {
case pending:
    if event.topic == "work.task.assigned" { transition(in_progress) }
case in_progress:
    if matchesTerminalEvent(event, run.TerminalEvent) { transition(completed) }
    if isFailureTopic(event.topic) { transition(failed) }
}
```

`matchesTerminalEvent` parses the colon-separated qualifier (`topic:field=value:field=value`) against the event topic + payload. Same matching semantics as the existing `payload_condition` on work-plans ([scenario-09/00-setup Step 11b](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/09/00-setup.md)).

`failed` → `completed` is **not** allowed (failure is sticky). `failed` → `rolled_back` is allowed (rollback after failure).

### Failure aggregation

A single failure event flips the run to `failed`. Subsequent events on the same run are observed for closure-building but don't change status. Closure may still grow (e.g. the diagnostic AgentRun fires after the failure event) — that's fine; those artifacts are part of the closure.

### Idempotency

Status writes use an `If-Match` style check (`update only if status is currently X`) to prevent races between concurrent inbound events. ArangoDB supports this via revision check on update.

### Status emission

When status changes, emit a domain event:

```
work.run.in_progress  {workflow_run_id, started_at}
work.run.completed    {workflow_run_id, completed_at, duration_ms}
work.run.failed       {workflow_run_id, failed_at, failure_reason}
work.run.rolled_back  {workflow_run_id, rolled_back_at}
```

UI's `LiveProgressBanner` and closure SSE endpoint consume these.

---

## Implementation plan

### Phase 1 — Schema (~0.25 day)

1. Add `terminal_event` property to `WorkflowRun` in [`schema.go`](../../schema.go).
2. Add `TerminalEvent` to `WorkflowRun` in [`models.go`](../../models.go).

### Phase 2 — Subscription + transition logic (~1 day)

1. Subscribe to the topics listed above in `internal/registrar/registrar.go`.
2. New file `internal/server/workflow_run_status.go` with the transition algorithm.
3. Emit `work.run.*` status events.
4. Wire into the existing event-receiver path.

### Phase 3 — Tests (~0.5 day)

- Unit: each transition (pending→in_progress, in_progress→completed via terminal event, in_progress→failed).
- Integration: full scenario 09 → assert run ends at `completed`.
- Negative: out-of-order events, double-failure, terminal-event arriving on a `failed` run.

---

## Verification

- `go test -race -count=1 ./...` clean.
- Scenario 09 end-to-end: run status visible in UI as `in_progress` during execution, transitions to `completed` after merge.
- Forced failure scenario: trigger a `functions.job.failed` for compile → run status flips to `failed`.

---

## Open design questions

1. **Terminal event must-match.** What if `terminal_event` is set but never fires (e.g. the merge step is skipped)? The run stays `in_progress` indefinitely. Recommend: add a timeout property (`max_duration_ms`); after expiry, run flips to `failed` with reason `timeout`. Configurable per run.
2. **Soft-completed.** A run where every linked Task is `completed` but no `terminal_event` was declared — should we auto-derive completion from "all tasks terminal"? Recommend yes as a fallback when `terminal_event` is empty.
3. **Status of the run vs status of the closure.** A run can be `completed` while a comm message is still being delivered asynchronously. The closure aggregator should not depend on run.status to know when to stop streaming — it should stop when all per-service sections have finished (per [FEAT-20260602-003 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-003_workflow_run_closure_sse_aggregation.md)).

---

## Dependencies

- Blocked by: [umbrella FEAT-20260602-001](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md) — transitions key off `workflow_run_id` in inbound events.
- Pairs with: [SSE aggregation FEAT-20260602-003 in Cross](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-003_workflow_run_closure_sse_aggregation.md) — that endpoint emits status changes via the `work.run.*` events emitted here.
- Builds on: [FEAT-20260601-001 (WorkflowRun entity)](FEAT-20260601-001_workflow_run_rollup.md).
