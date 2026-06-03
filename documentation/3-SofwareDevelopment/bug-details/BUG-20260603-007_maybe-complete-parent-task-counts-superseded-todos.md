# BUG-20260603-007 — `maybeCompleteParentTask` counts superseded todos from previous decompositions

**Status:** ✅ Fixed (2026-06-03)
**Severity:** High — direction-driven retry appears to succeed (new todos all COMPLETE) but the parent task is immediately re-marked FAILED, short-circuiting the retry ladder and never escalating to `work.task.needs-direction`
**Owner:** CodeValdWork
**Estimated effort:** ~0.5 day (filter helper + unit tests)
**Source finding:** QA scenario 11 (Dir-3, 2026-06-03T21:31 UTC) — after Dir-2 reopened MVP-SF-001 with `retry-with-instructions`, the AI re-decomposed and all 8 new todos (including the previously-failing "Compile and verify branch") COMPLETED. Task immediately went back to `TASK_STATUS_FAILED` with `failureCount=0`.

## Problem

`maybeCompleteParentTask` iterates **every** todo edged off the parent task and marks the task FAILED if any are in `failed` or `blocked`. It does not scope to the latest decomposition. After a retry-with-instructions direction:

1. AI creates a fresh decomposition (8 new todos with a new `DecompRunID`).
2. New todos execute and all reach `completed`.
3. `maybeCompleteParentTask` runs.
4. It sees 8 new COMPLETED + 2 stale FAILED todos from prior decompositions.
5. `anyFailed = true` → task marked FAILED.

## Evidence

After Dir-2 retry on MVP-SF-001 (parent task 1c320ad8-...):

```
ord  created                status     title
1    2026-06-03T09:15:16Z   COMPLETED  Create branch feature/MVP-SF-001-project-scaffolding
…
7    2026-06-03T09:15:16Z   FAILED     Compile and verify branch         ← stale, pre-BUG-003-fix
8    2026-06-03T09:25:51Z   FAILED     Compile and verify branch         ← stale, pre-BUG-003-fix
…
1    2026-06-03T21:30:16Z   COMPLETED  Create feature branch              ← new decomp
…
8    2026-06-03T21:30:16Z   COMPLETED  Compile and verify branch          ← new decomp succeeds
```

23 todos: 21 COMPLETED, 2 FAILED (both from prior decompositions). The new decomposition is fully successful, but the task is FAILED.

## Root cause

Each call to `handleTaskDirection: retry-with-instructions` re-publishes `work.task.assigned`. CodeValdAI handles the assignment by calling its decomposition path, which creates a fresh set of TaskTodo entities edged off the same parent. Each TaskTodo carries a `DecompRunID` pointing to the AgentRun that produced it, but `maybeCompleteParentTask` does not look at `DecompRunID`.

## Fix

Group todos by `DecompRunID` and only evaluate the group whose latest `CreatedAt` is most recent. Todos with empty `DecompRunID` (non-AI-decomposed work) fall into a single group keyed on "" so they behave as before. Implementation added as a private `latestDecomposition([]TaskTodo) []TaskTodo` helper alongside `maybeCompleteParentTask`.

```go
todos := latestDecomposition(allTodos)
anyFailed := false
for _, todo := range todos {
    // ... existing terminal-check logic, but only over the latest group
}
```

Docstring on `maybeCompleteParentTask` updated to call out the retry-with-instructions invariant.

## Verification

After the fix, the third direction-retry on MVP-SF-001 should drive the parent task to `TASK_STATUS_COMPLETED` even though 2 stale FAILED todos still exist on the same parent.

## Dependencies

- [BUG-20260603-001](BUG-20260603-001_workflow-run-status-never-advances.md) (Work subscription gap) — must be fixed so the WorkflowRun lifecycle reflects the retry.
- [BUG-20260603-006](BUG-20260603-006_task-state-machine-blocks-direction-retry.md) (state machine `failed→in_progress`) — must be fixed so the retry can start at all.
- Together these three bugs are the prerequisite to validating the AI Failure Reviewer loop in QA scenario 11.

## Alternative considered (rejected)

Marking the prior decomposition's todos as `cancelled` or `skipped` when a retry-with-instructions direction lands. Rejected because it mutates audit history — the prior FAILED todos and their failure reasons are useful evidence for forensics. Filtering on read keeps the records intact.
