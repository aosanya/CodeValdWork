# FEAT-20260605-003 — Reviewer component (AI-driven AcceptanceCriteria evaluation)

**Status:** 📋 Not Started
**Severity:** Medium — enables the review gate; without this the WorkPlan review step stalls
**Owner:** CodeValdWork
**Estimated effort:** ~3 days (event handler + Work API calls + AI integration + tests)
**Source finding:** Research session 2026-06-05 — task decomposition improvement discussion
**Depends on:**
- [FEAT-20260605-001 (CodeValdWork schema v4)](FEAT-20260605-001_deliverable_acceptance_criteria_schema.md)
- [FEAT-20260605-002 (WorkPlan review step)](../../../CodeValdAgency/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260605-002_workplan_review_step.md)

---

## Problem

`AcceptanceCriteria` entities carry `result` and `result_notes` fields that must be written after a task completes. There is no component today that reads the linked `Deliverable` artifacts and criteria for a task, evaluates them, and writes the result back. Without this, the WorkPlan review gate (FEAT-20260605-002) has no signal to act on.

## Goal

Build an AI-driven reviewer that:
1. Subscribes to `work.task.completed` (or the `review_trigger_topic` declared in the WorkPlan step).
2. Fetches all `AcceptanceCriteria` and `Deliverable` entities linked to the completed task.
3. Evaluates each criterion against the associated deliverables using an LLM call.
4. Writes `result` (`"passed"` | `"failed"` | `"skipped"` | `"blocked"`) and `result_notes` back onto each `AcceptanceCriteria` via the Work API.
5. Emits `work.review.passed` if all criteria pass, or `work.review.failed` otherwise.

Human review and functional review are future variations of the same flow; the event contract is identical.

## Non-goals

- Human review UI — deferred.
- Functional review (CI-based) — deferred.
- Backfilling existing tasks — reviewer only runs on new completions.

---

## Design

### Event subscription

The reviewer subscribes to the topic declared in the WorkPlan's `review_trigger_topic` (default: `work.task.completed`). The event payload carries `task_id`, `agency_id`, and `workflow_run_id`.

### Evaluation flow

```
work.task.completed
  → fetch AcceptanceCriteria (GET /work/{agency}/todos/{taskId}/acceptance-criteria or graph traversal)
  → fetch Deliverables (GET /work/{agency}/todos/{taskId}/deliverables)
  → for each criterion:
      LLM call: "Given these deliverables, does this criterion pass?"
      → result = "passed" | "failed" | "skipped" | "blocked"
      → result_notes = LLM explanation
      → PUT /work/{agency}/acceptance-criteria/{criterionId}  { result, result_notes }
  → if all result == "passed":
      publish work.review.passed { task_id, workflow_run_id, agency_id }
    else:
      publish work.review.failed { task_id, workflow_run_id, agency_id, failed_criteria: [...] }
```

### Result values

| Value | Meaning |
|---|---|
| `"passed"` | Criterion is satisfied by the evidence in Deliverables |
| `"failed"` | Criterion is not satisfied; task must be retried or redirected |
| `"skipped"` | Criterion was not applicable for this run (e.g. no relevant deliverable) |
| `"blocked"` | Evaluation could not proceed (missing deliverable, LLM error, etc.) |

### Reviewer ownership

Owned by **CodeValdWork** as an event handler / subscriber within the Work service boundary. The LLM call uses CodeValdAI's agent execution infrastructure via Cross events — the reviewer does not dial CodeValdAI directly.

---

## Files to create / modify

| File | Change |
|---|---|
| `internal/reviewer/` (new package) | `reviewer.go` — event handler; fetches criteria + deliverables, calls LLM, writes results, emits review outcome |
| `internal/app/` | Wire the reviewer subscriber on startup |
| `internal/reviewer/reviewer_test.go` | Unit tests with mock Work client and mock LLM |

---

## Acceptance tests

- Given a completed task with 2 passing and 1 failing criterion, the reviewer writes the correct `result` to each `AcceptanceCriteria` and emits `work.review.failed`.
- Given a completed task where all criteria pass, the reviewer emits `work.review.passed`.
- If a task has no `AcceptanceCriteria`, the reviewer emits `work.review.passed` immediately (vacuous truth).
- A `"blocked"` result on any criterion causes `work.review.failed` (not a pass).
- `go test -race ./internal/reviewer/` passes.

---

## Open gaps

- Which LLM model / agent type powers the reviewer (configured per-agency or hardcoded default)?
- Should `work.review.failed` carry full criterion detail for the direction form, or just the count?
- Retry behaviour when `result = "failed"` — does the WorkPlan re-assign the task or escalate to human direction?
