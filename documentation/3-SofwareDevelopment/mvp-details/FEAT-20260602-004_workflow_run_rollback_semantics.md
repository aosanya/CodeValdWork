# FEAT-20260602-004 — `WorkflowRun` rollback / compensation semantics (design-only)

**Status:** 📋 Design only — no implementation planned in this wave; design captured to preserve rationale and keep the contract explicit for the umbrella FEATs that depend on it
**Severity:** Medium — the "workflow_run_id is a transaction handle" justification rests on rollback being feasible; if we lock in wide propagation without a rollback design, we lose the load-bearing reason for the work
**Owner:** CodeValdWork (lead) — cross-service collaboration with AI, Functions, Git, Comm
**Estimated effort:** ~1 day to finalize this design doc; ~5–10 days to implement once the wide-propagation FEATs ship (out of scope here)
**Source finding:** This conversation (2026-06-02) — user motivation: *"everything needs a workflowrun id, so that we have an object like a transaction that we can use to reverse all the changes"*

---

## Problem

`WorkflowRun` is described as the transaction handle for rollback. But "reverse all the changes" is a phrase that hides at least three different semantics, and they have very different operational consequences:

1. **Hard delete:** every artifact created during the run is deleted (Tasks, TaskTodos, AgentRuns, Jobs, Branches, MergeRequests, Messages). The run row itself is marked `rolled_back`. Git branches are deleted; merged commits are reverted.
2. **Soft delete:** artifacts are flagged `status: rolled_back` (or equivalent terminal state). Git branches stay; the rollback is an audit-trail flag, not a literal undo.
3. **Compensate:** every artifact emits a compensating event. Other services react to the compensation. The system reaches a state "as if the run never happened" via *forward* progress, not backward deletion.

This FEAT picks one (provisionally) and documents the per-service contract.

## Goal

Capture the chosen rollback semantics with enough specificity that:

- The wide-propagation FEATs ([umbrella](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md) and siblings) can claim "future rollback is unblocked."
- An operator reading the closure UI knows what "Roll back" would actually do.
- Per-service teams know what API they'll eventually need to expose.

## Non-goals

- **No implementation in this wave.** This is a design-only doc.
- No UI for the rollback action.
- No automated rollback on failure — rollback stays explicit (operator-triggered).

---

## Provisional design

### Chosen semantics: **hybrid hard-delete + compensate**

Per-service rules:

| Service | Action on rollback |
|---|---|
| **CodeValdWork** | Hard delete Tasks + TaskTodos + edges (`started_task`, `started_todo`, `has_todo`, `assigned_to`, `depends_on` where the task is inside the closure). Emit `work.task.rolled_back` events for observability. |
| **CodeValdAI** | Cancel in-flight AgentRuns (status → `cancelled`). Keep completed AgentRuns as audit (status → `rolled_back`, frozen). Don't delete LLM artifacts — they're useful for debugging. |
| **CodeValdFunctions** | Cancel in-flight Jobs. Mark completed Jobs as `rolled_back` (audit). Don't delete function outputs (e.g. compile logs). |
| **CodeValdGit** | Revert merged commits via a compensating merge commit on `main`. Hard-delete unmerged feature branches. Mark MergeRequests as `rolled_back`. |
| **CodeValdComm** | Send a "this pipeline was rolled back" follow-up message into the same conversation. Don't delete prior messages (audit + recipients may have already read them). |

### Trigger

```
POST /work/{agencyId}/workflow-runs/{id}/rollback
{
  "reason": "merge introduced regression"
}
```

CodeValdWork validates the run is `failed` or `completed` (not `in_progress` — rollback during execution is a separate "cancel" action), then orchestrates the per-service compensations via gRPC.

### Orchestration

CodeValdWork acts as the rollback coordinator. Sequence:

1. **Acquire** — set `WorkflowRun.status = rolling_back` (new transient state — add to enum). Reject double-rollback.
2. **Quiesce** — publish `work.run.rolling_back` to halt any in-flight handlers; wait briefly for them to check the run status.
3. **Compensate in reverse order** — Comm message first (mention rollback to recipients), then Git (revert/delete), then Functions (cancel pending), then AI (cancel pending), then Work (delete entities).
4. **Finalize** — set `WorkflowRun.status = rolled_back`; emit `work.run.rolled_back`.

If any step fails, the run goes to `rollback_failed` and operator intervention is required. Idempotent retry: the operator can re-trigger the rollback; each per-service step checks its own state and skips no-ops.

### Per-service implementation

Each service exposes:

```
DELETE /<service>/{agencyId}/by-workflow-run/{id}
```

The aggregate semantics differ by service per the table above. CodeValdWork calls these in the orchestrated order.

### Failure modes

- **Partial rollback** — Git revert succeeded, Functions cancel failed. Status goes to `rollback_failed`; operator runs the per-service DELETE manually for the failed leg.
- **No-op rollback** — a run that produced nothing (e.g. `start-pipeline` succeeded, `next-task` found no task, pipeline ended). Status flips to `rolled_back` immediately; per-service DELETEs are no-ops because no artifacts have the run-id.
- **Cross-run interference** — a Task in this run depends on a Task in another run. Deleting the Task here would break the other run's depends-on edge. Resolution: rollback fails with `409: foreign_run_dependency`; operator must roll back the dependent run first.

---

## Implementation plan (deferred)

When this is picked up:

### Phase 1 — Status + endpoint stubs (~1 day)

- Add `rolling_back` and `rollback_failed` to the WorkflowRun status enum.
- Add `POST .../rollback` endpoint that just toggles status (no actual work).

### Phase 2 — Per-service DELETE endpoints (~2 days × 5 services, in parallel)

- Each service adds `DELETE /<service>/{a}/by-workflow-run/{id}` implementing its rule from the table.

### Phase 3 — Coordinator (~2 days)

- CodeValdWork orchestrates the per-service calls in sequence; handles partial failure.

### Phase 4 — Tests (~1 day)

- Full scenario 09 → rollback → assert all artifacts deleted/cancelled per the table.

---

## Verification (when implemented)

- Run scenario 09 to completion. Then `POST .../rollback`. Closure SSE returns empty sections (no work-tasks, no jobs, etc.). Git branch is deleted; merge is reverted on main. Comm conversation has a follow-up message.

---

## Open design questions

1. **Cancel-vs-rollback for `in_progress` runs.** Should `POST /rollback` on an in-progress run be allowed (cancel + rollback in one shot), or should we require `POST /cancel` first? Recommend separate verbs for clarity.
2. **Auditability after delete.** Hard-deleting Tasks loses audit history. Should we soft-delete in CodeValdWork instead (status → `rolled_back`)? Trade-off: closure UI for rolled-back runs would still render rows. Recommend hard-delete + a separate `audit_log` collection that captures the run + per-step actions before delete.
3. **Git revert authorship.** The compensating revert commit needs an author. Recommend: `CodeVald Rollback <rollback@codevald.local>` with the original run-id in the commit body.
4. **Comm notification opt-in.** Some pipelines shouldn't notify on rollback (silent rollback of a failed scenario test). Recommend a `silent: true` flag in the rollback request body.

---

## Dependencies

- Blocked by: [umbrella FEAT-20260602-001](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md) — rollback iterates over artifacts by `workflow_run_id`.
- Pairs with: [Status SM FEAT-20260602-003](FEAT-20260602-003_workflow_run_status_state_machine.md) — adds the `rolling_back` and `rollback_failed` states.
- Builds on: [FEAT-20260601-001 (WorkflowRun entity)](FEAT-20260601-001_workflow_run_rollup.md).
