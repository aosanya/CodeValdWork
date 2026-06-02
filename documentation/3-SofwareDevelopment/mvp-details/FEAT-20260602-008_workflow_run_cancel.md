---
status: 📋 Draft (2026-06-02)
owner: CodeValdWork
co_owners: CodeValdAI, CodeValdFunctions, CodeValdCross, CodeValdWorkFrontend
estimated_effort: ~1.5 days (endpoint + status transitions + topic constants + per-service subscribers + frontend button)
source: this conversation (2026-06-02) — gap G3 in [Cross pipeline-failure-handling](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/pipeline-failure-handling.md)
depends_on: FEAT-20260602-001 (workflow_run_id propagation), FEAT-20260602-003 (status state machine)
---

# FEAT-20260602-008 — Mid-flight WorkflowRun cancel

## Problem

The 09 QA scenario routinely produces runs that the operator wants to stop
before they terminate naturally: a flapping compile loop, an LLM that's
generating obvious garbage, a developer-introduced bug that's about to land
on `main`. Today there is **no programmatic way to stop an `in_progress`
run**:

- [FEAT-20260602-004](FEAT-20260602-004_workflow_run_rollback_semantics.md)
  rollback rejects `in_progress` runs explicitly — rollback only accepts
  `failed | completed`.
- [FEAT-20260602-006](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-006_workflow_run_watchdog.md)
  watchdog only fires after `WORKFLOW_RUN_STALE_TIMEOUT` (default 30 min)
  of inactivity. A run that's *actively* misbehaving (e.g. generating
  events at full speed but in the wrong direction) is never stale and
  never times out.
- [FEAT-20260602-007](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-007_failure_pipeline_budget.md)
  budget caps recovery cost but does nothing for forward-path misbehaviour.

The operator's only current option is to kill containers — which leaves
the entity graph inconsistent and produces orphaned `AgentRun` /
`Job` rows that confuse subsequent runs.

## Principle

> **`POST /workflow-runs/{id}/cancel` flips an `in_progress` run terminal,
> cascades cancellation to every non-terminal child artifact, and quiesces
> in-flight handlers via a typed event each service subscribes to.**

Cancel is a deliberate operator action. It is **not** a rollback (artifacts
are kept; no compensating actions fire) and **not** a watchdog timeout
(no automatic detection). The run terminates as `cancelled` —
distinguishable from `failed` so closure SSE / rollback can treat it
differently if needed.

## Goal

1. New endpoint `POST /work/{agencyId}/workflow-runs/{id}/cancel` on
   CodeValdWork.
2. Two new topics: `work.run.cancelling` (transient signal) and
   `work.run.cancelled` (terminal).
3. Per-service subscribers (`codevaldai`, `codevaldfunctions`) handle
   `work.task.cancelled` by cancelling in-flight AgentRuns / Jobs.
4. WorkflowRun status enum extended with `cancelling` (transient) and
   `cancelled` (terminal).
5. A "Cancel run" button on the WorkflowRun detail page in WorkFrontend
   (separate UI FEAT; defer doc).

## Non-goals

- **Rollback after cancel.** A cancelled run leaves its artifacts in
  place (the feature branch on git, the partial Jobs, the completed
  todos). To remove them, the operator triggers FEAT-004 rollback
  *after* cancel terminates. Cancel and rollback are sequential.
- **Pause / resume.** Cancel is terminal. There is no "paused" state and
  no resume. A cancelled run is dead; restart by creating a new run.
- **Bulk cancel.** v1 cancels one run at a time. Bulk operations are an
  operator-tool concern, not a service-API concern.
- **Cancel reasoning UI.** The endpoint accepts a `reason` string; how
  the frontend collects it (free text, dropdown, etc.) is the UI FEAT's
  concern.

---

## Design

### 1. Status state machine

Extend `WorkflowRunStatus` per
[FEAT-20260602-003](FEAT-20260602-003_workflow_run_status_state_machine.md):

```
in_progress ──cancel API──> cancelling ──quiesce timeout──> cancelled
                 │                                              ▲
                 │ (already terminal — reject)                  │
                 ▼                                              │
              409 Conflict                                      │
                                                                │
in_progress → completed | failed | cancelled   (terminal states)
```

`cancelling` is **transient** — it persists only between the API call
and the quiesce-deadline reached. In every other respect it behaves like
`in_progress` for queries (closure SSE still streams; tasks may still
emit events while handlers wind down).

### 2. Endpoint

```
POST /work/{agencyId}/workflow-runs/{id}/cancel
Authorization: Basic <auth>
Content-Type: application/json

{
  "reason":              "compile loop runaway",
  "quiesce_timeout_ms":  30000     // optional; default 30s
}
```

**Response 200:**

```json
{
  "workflow_run_id": "...",
  "status":          "cancelling",
  "cancelled_by":    "<authenticated user or service>",
  "reason":          "compile loop runaway",
  "quiesce_deadline": "2026-06-02T15:30:42Z"
}
```

**Error 409 Conflict** — run is not `in_progress` (already terminal, or
already `cancelling`).
**Error 404** — run does not exist.

### 3. Endpoint implementation

In [`CodeValdWork/internal/server/`](../../../CodeValdWork/internal/server/),
new file `cancel.go`:

```go
func (s *workManager) CancelWorkflowRun(ctx context.Context, req CancelRequest) (CancelResponse, error) {
    run, err := s.GetWorkflowRun(ctx, req.AgencyID, req.ID)
    if err != nil { return CancelResponse{}, err }
    if run.Status != WorkflowRunStatusInProgress {
        return CancelResponse{}, ErrCannotCancelTerminalRun
    }

    // Flip to cancelling. Idempotent — concurrent calls all see "already cancelling"
    // via the optimistic concurrency check on the status field.
    if err := s.setRunStatus(ctx, req.AgencyID, req.ID,
                              WorkflowRunStatusCancelling, req.Reason); err != nil {
        return CancelResponse{}, err
    }

    deadline := s.clock.Now().Add(req.QuiesceTimeout)

    // Publish the quiesce signal. Per-service subscribers cancel their in-flight work.
    s.publish(ctx, TopicWorkRunCancelling, RunCancellingPayload{
        WorkflowRunID:     req.ID,
        AgencyID:          req.AgencyID,
        Reason:            req.Reason,
        CancelledBy:       req.CancelledBy,
        QuiesceDeadline:   deadline,
    })

    // Cascade to non-terminal tasks. Each emits work.task.cancelled which
    // CodeValdAI / CodeValdFunctions subscribe to.
    tasks, _ := s.ListTasksByWorkflowRun(ctx, req.AgencyID, req.ID)
    for _, t := range tasks {
        if !t.IsTerminal() {
            _ = s.setTaskStatus(ctx, req.AgencyID, t.ID, TaskStatusCancelled, "run_cancelled")
            s.publish(ctx, TopicWorkTaskCancelled, TaskCancelledPayload{
                TaskID:         t.ID,
                WorkflowRunID:  req.ID,
                Reason:         "run_cancelled",
            })
        }
    }

    // Schedule the deadline transition. Goroutine; survives process restart via
    // a `cancelling_until` property on the WorkflowRun row (read on startup).
    go s.scheduleCancelFinalization(req.ID, deadline)

    return CancelResponse{
        WorkflowRunID:    req.ID,
        Status:           WorkflowRunStatusCancelling,
        CancelledBy:      req.CancelledBy,
        Reason:           req.Reason,
        QuiesceDeadline:  deadline,
    }, nil
}
```

### 4. Quiesce + finalization

After the quiesce deadline, `scheduleCancelFinalization` flips the run
to `cancelled` regardless of whether all handlers have acknowledged.
This is intentional: cancel is **best-effort** — services that don't
quiesce in time will eventually finish their in-flight work and emit
their events; those late events are ignored by the run-terminal handlers
(idempotency) but persist as audit.

```go
func (s *workManager) scheduleCancelFinalization(runID string, deadline time.Time) {
    select {
    case <-time.After(time.Until(deadline)):
    case <-s.shutdown:
        return
    }
    // Re-read; if someone else already finalized, no-op.
    run, _ := s.GetWorkflowRun(...)
    if run.Status != WorkflowRunStatusCancelling { return }

    _ = s.setRunStatus(..., WorkflowRunStatusCancelled, run.CancelReason)
    s.publish(TopicWorkRunCancelled, RunCancelledPayload{
        WorkflowRunID:  runID,
        Reason:         run.CancelReason,
        CancelledBy:    run.CancelledBy,
        QuiesceCompleted: !anyHandlerStillRunning(),  // best-effort metric
    })
}
```

Crash resilience: on startup, CodeValdWork queries
`status=cancelling AND cancelling_until < now`, finalizes them
immediately, and queries `status=cancelling AND cancelling_until > now`,
re-schedules them.

### 5. Per-service subscribers — what cancel cascades to

#### CodeValdAI — `work.task.cancelled` handler

For every AgentRun where `task_id` matches the cancelled task and the
run's status is non-terminal:

- Flip the `AgentRun.status` to `cancelled` (new terminal state on
  AgentRun if not already present per
  [ai-run-failure-modes AR-5](ai-run-failure-modes.md)).
- Publish `ai.run.failed { reason: cancelled }` for downstream
  visibility.
- Best-effort cancel of the underlying LLM call (close the streaming
  HTTP connection). If the LLM call is non-streaming, let it finish but
  discard the output.

#### CodeValdFunctions — `work.task.cancelled` handler

For every Job where `task_id` matches and the job is `pending` or
`running`:

- Flip `Job.status = cancelled`.
- Publish `functions.job.failed { reason: cancelled }`.
- Best-effort process kill of the running binary. Functions binaries
  that hold long-lived state (a `git clone` in progress) should respect
  `SIGTERM` and clean up; document this in the function-author guide.

#### CodeValdCross — failure-dispatch suppression

Per [FEAT-20260602-005](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md),
the recovery-pipeline dispatcher inspects every incoming `*.failed`
event's `reason`. **If `reason == cancelled`, the dispatcher skips
the recovery lookup** — cancellation is its own terminal path; we do
not want recovery pipelines firing for every cascaded cancel.

This rule is in FEAT-005 already; this FEAT confirms the contract.

### 6. Idempotency

- Repeat `POST .../cancel` calls on an already-`cancelling` run return
  the existing cancellation envelope without re-firing events. Implement
  via the optimistic concurrency check on the status transition.
- Cancelling a run that became terminal between the read and the write
  returns `409 Conflict` with the now-current status. Operator sees
  "already terminal — no action taken."

### 7. Authorization

The endpoint requires Basic auth like every other Work mutating endpoint.
v1 has no per-run ACL — any authenticated caller can cancel any run.
Per-run ACL is a future hardening once CodeValdOrg lands fully.

### 8. Reason taxonomy

The `reason` string is free-form for v1 to keep the endpoint simple. The
WorkFrontend cancel dialog should offer a small dropdown of common
reasons (`compile loop`, `wrong direction`, `infra issue`, `other →
free text`) but the API does not enforce values. Move to a typed enum
once the empirical distribution stabilises.

---

## Closure SSE & rollback interaction

- **Closure SSE** ([FEAT-20260602-003](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-003_workflow_run_closure_sse_aggregation.md))
  — the `head` event for a cancelled run carries
  `status: "cancelled"`, `cancelled_by`, `cancel_reason`. Per-service
  sections list whatever artifacts existed at cancel time (some Jobs
  may show `cancelled` status).
- **Rollback** (FEAT-004) — accepts `cancelled` runs the same way it
  accepts `failed`. So the operator flow is: `cancel` → review the
  cancelled state → optionally `rollback` to remove the artifacts.

---

## Migration

The endpoint is additive — no breaking changes. Old WorkflowRun rows
with `status=in_progress` from before the FEAT lands work normally
(the new endpoint just hadn't been available before).

Two-phase rollout:

1. **Phase 1** — Server-side: endpoint, topics, status enum, per-service
   subscribers. WorkFrontend continues to have no UI button — operators
   use `curl` or a script.
2. **Phase 2** — UI: cancel button on the run detail page. Includes a
   confirm modal showing the count of non-terminal tasks that will be
   cascaded.

---

## Open design questions

1. **Default quiesce timeout.** 30 s is enough for an LLM streaming
   connection to close and a `flutter analyze` subprocess to die.
   Probably too short for a long-running CodeValdGit clone. Should
   each service publish its expected quiesce time on registration so
   the cancel endpoint picks `max(observed)` as the default? Defer;
   30 s default + per-request override is enough for v1.

2. **What if quiesce never completes?** A binary that ignores `SIGTERM`
   keeps producing events past the deadline. Those events are accepted
   (idempotency on terminal-run handlers no-ops them) but the underlying
   compute keeps burning. Mitigation: per-service hard `SIGKILL` after
   `2 × quiesce_timeout`. Out of scope here; flag as platform-hardening.

3. **Cancelling a run mid-recovery.** If the parent run is `cancelling`
   while a child recovery WorkflowRun (FEAT-007) is `in_progress`, the
   child should cascade-cancel too. Solution: the cancel endpoint also
   finds all child runs (`parent_workflow_run_id == this`, recursive)
   and cancels each. Implementation detail — list children, fire cancel
   per child, then proceed.

4. **Cancel attribution.** `cancelled_by` should distinguish "operator
   <name>" vs. "automated rule X". Today the endpoint records the auth
   identity; future automated cancellers (e.g. a budget watcher that
   auto-cancels runs costing >$N) need a service identity. Out of
   scope; the field accepts any string.

5. **Should `work.run.cancelled` be routed through FEAT-005?** No —
   like `cancelled` is its own terminal path with no recovery semantics.
   The FEAT-005 dispatcher's `reason == cancelled` skip rule handles
   this.

---

## Verification (when implemented)

Scenario 09 additions (`09-work-99-failure-paths.md`):

1. Start a Part-G pipeline run; let it reach the AI decomposition step.
2. While the AgentRun is in `running`, call
   `POST /work/utility-app-builder/workflow-runs/<id>/cancel`.
3. Assert response `200`, status `cancelling`, `quiesce_deadline` ~30 s
   in the future.
4. Within 1 s, assert:
   - `work.run.cancelling` published once
   - Every non-terminal task has `status=cancelled` and a
     `work.task.cancelled` event
   - The AgentRun has `status=cancelled` and an `ai.run.failed {reason:
     cancelled}` event
5. Within the quiesce window, assert no new recovery pipelines fire
   (FEAT-005 skip rule).
6. After the quiesce deadline, assert:
   - `work.run.cancelled` published once
   - `WorkflowRun.status = cancelled`
7. Cross-check: closure SSE returns the cancelled state and all
   artifacts present at cancel time.
8. Cross-check: `POST .../rollback` now succeeds on the cancelled run
   (was 409 while `cancelling`).

Negative tests:

- Cancel an already-completed run → 409.
- Cancel a non-existent run → 404.
- Two concurrent cancel calls → first succeeds, second returns the
  already-cancelling envelope.

---

## Related work

- [Cross — pipeline-failure-handling §G3](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/pipeline-failure-handling.md)
- [Cross — FEAT-20260602-005 — failure pipelines via synthesized success events](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md)
  — confirms the `reason == cancelled` skip rule
- [Cross — FEAT-20260602-006 — workflow_run watchdog](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-006_workflow_run_watchdog.md)
  — orthogonal; watchdog is automatic, cancel is operator-initiated
- [Cross — FEAT-20260602-007 — failure-pipeline budget](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-007_failure_pipeline_budget.md)
  — child runs cascade-cancel via question 3 above
- [FEAT-20260602-003 — workflow_run status state machine](FEAT-20260602-003_workflow_run_status_state_machine.md)
  — `cancelling`, `cancelled` join the state enum
- [FEAT-20260602-004 — rollback semantics](FEAT-20260602-004_workflow_run_rollback_semantics.md)
  — operator-flow: cancel → review → rollback
- [task-failure-modes TF-6](task-failure-modes.md)
  — the task-side hook this FEAT implements
- [AI — ai-run-failure-modes AR-5](../../../../CodeValdAI/documentation/3-SofwareDevelopment/mvp-details/ai-run-failure-modes.md)
  — AgentRun cancellation handling
