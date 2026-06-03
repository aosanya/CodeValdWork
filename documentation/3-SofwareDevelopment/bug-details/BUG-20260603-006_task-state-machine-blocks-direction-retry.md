# BUG-20260603-006 — TaskStatus.CanTransitionTo blocks `failed → in_progress`, breaking direction-driven retry

**Status:** ✅ Fixed (2026-06-03)
**Severity:** High — `handleTaskDirection` cannot retry FAILED tasks; the entire AI-Failure-Reviewer recovery loop (FEAT-20260603-003) is dead on any task that reached the `failed` terminal status
**Owner:** CodeValdWork
**Estimated effort:** ~0.25 day (state-machine rule + test)
**Source finding:** QA scenario 11 (Dir-2, 2026-06-03T21:23 UTC) — operator published `work.task.direction` `retry-with-instructions` for MVP-SF-001 (FAILED); Work logged `handleTaskDirection: retry: UpdateTask 1c320ad8-...: invalid task status transition` and the task stayed at `TASK_STATUS_FAILED`.

## Problem

`handleTaskDirection` for `retry-with-instructions` sets `task.Status = TaskStatusInProgress` and calls `UpdateTask`. The state-machine rule in [models.go `TaskStatus.CanTransitionTo`](../../../models.go) declared `failed` terminal:

```go
default:
    // completed, failed, cancelled are terminal — no further transitions.
    return false
```

This conflicts with the FEAT-20260603-003 intent: the AI Failure Reviewer should be able to choose `skip`, `retry-with-instructions`, `mark-blocked`, or `cancel` for a failing task, and Work should apply the chosen direction. For tasks that bypassed the retry ladder (legacy state, or non-todo-driven failures) the task may already be in `failed` instead of `awaiting-direction`, and the recovery path is then unreachable.

## Evidence

```
codevaldwork-1  | 2026/06/03 21:23:07 codevaldwork: NotifyEvent: ACK event_id=f96a9748-... topic=work.task.direction
codevaldwork-1  | 2026/06/03 21:23:07 codevaldwork: handleTaskDirection: task=1c320ad8-... option=retry-with-instructions
codevaldwork-1  | 2026/06/03 21:23:07 codevaldwork: handleTaskDirection: retry: UpdateTask 1c320ad8-...: invalid task status transition
```

Task remained at `TASK_STATUS_FAILED`; MVP-SF-002 (its dependent) remained blocked indefinitely.

## Root cause

`failed` was treated as a hard terminal in `TaskStatus.CanTransitionTo`. The direction-driven recovery path was added (FEAT-20260603-003) but only wired through `awaiting-direction → in_progress / blocked / cancelled` — the symmetric path from `failed` was never added.

## Fix

Add a `case TaskStatusFailed:` in `TaskStatus.CanTransitionTo` mirroring the `awaiting-direction` exits, so a direction event can recover any FAILED task. Docstring updated to reflect the new rule.

```go
case TaskStatusFailed:
    // A FAILED task is normally terminal, but a work.task.direction event
    // (from the AI Failure Reviewer or a human operator) can reopen it to
    // recover. Mirrors the awaiting-direction exits.
    return next == TaskStatusInProgress || next == TaskStatusBlocked || next == TaskStatusCancelled
```

## Verification

After the fix, replaying the same direction event flips MVP-SF-001 `failed → in_progress` and re-emits `work.task.assigned` so the AI re-decomposes:

```
codevaldwork-1  | 2026/06/03 21:28:08 codevaldwork: handleTaskDirection: retry: re-dispatched task=1c320ad8-... role=Developer
codevaldai-1    | 2026/06/03 21:28:08 codevaldai: NotifyEvent: ACK event_id=1b553578-... topic=work.task.assigned
```

## Dependencies

- Pairs with [BUG-20260603-001](BUG-20260603-001_workflow-run-status-never-advances.md): without that fix the new pipeline run stays Pending; without this fix the operator cannot reopen the FAILED task at all.
- Pairs with [BUG-20260603-007](BUG-20260603-007_maybe-complete-parent-task-counts-superseded-todos.md): even after the state-machine fix, retried tasks were still marked FAILED because stale todos from the prior decomposition were being counted.
