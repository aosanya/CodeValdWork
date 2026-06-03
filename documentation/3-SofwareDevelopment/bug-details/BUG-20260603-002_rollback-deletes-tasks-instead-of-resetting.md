# BUG-20260603-002 — `RollbackWorkflowRun` hard-deletes Tasks instead of resetting them to `pending`

**Status:** 📋 Open
**Severity:** High — rollback is destructive and data-lossy; tasks created or completed by a workflow run are permanently deleted rather than returned to their pre-run state
**Owner:** CodeValdWork
**Estimated effort:** ~1 day (replace delete logic with status reset + clear workflow_run_id)
**Source finding:** Session 2026-06-03 — operator ran `/dev-rollback-workflow ce0b3d84` on a shared-farms pipeline; MVP-SF-001 (Project Scaffolding) and MVP-SF-002 (Auth) were deleted from the DB instead of being reset to `pending`

## Problem

`DeleteWorkflowRunArtifacts` (called by `RollbackWorkflowRun`) issues hard deletes on every Task and TaskTodo anchored to the run ID. The correct rollback semantics are:

| Entity | Rollback action |
|---|---|
| Task created by this run | Reset `status → pending`, clear `workflow_run_id`, clear `completed_at` |
| TaskTodo created by this run | Delete (todos are ephemeral decomposition artifacts) |
| Task that existed before the run | Reset `status → pending`, clear `workflow_run_id` |

Deleting tasks causes:
1. Permanent data loss — the task definitions, descriptions, tags, and dependency edges are gone
2. Graph corruption — edges from other tasks (`depends_on`, `member_of`) become dangling
3. The frontend task list drops entries that should still exist as `pending` work items

## Evidence

```
# Before rollback — 11 tasks, 2 completed (MVP-SF-001, MVP-SF-002)
# After RollbackWorkflowRun — 9 tasks, both completed tasks gone

# Operator had to manually restore via direct AQL insert:
db.work_tasks.insert({ _key: "1c320ad8-...", type_id: "Task", ... status: "pending" })
db.work_tasks.insert({ _key: "73540b47-...", type_id: "Task", ... status: "pending" })
# + 21 relationship edges had to be recreated
```

Additionally, the two tasks had `workflow_run_id: null` and `workflow_run_id: ""` respectively — a separate bug ([BUG-20260603-003](#)) that caused `DeleteWorkflowRunArtifacts` to miss them entirely on the first rollback attempt. A second manual cleanup then deleted them incorrectly.

## Root cause

`workflow_run_rollback.go` — `DeleteWorkflowRunArtifacts` queries tasks by `workflow_run_id` and calls `dm.DeleteEntity` on each match. This is intentionally destructive; the function was modelled on "undo artifact creation" rather than "reset to pre-run state".

The key design error: Tasks are long-lived work items that belong to the project regardless of which run executed them. Only TaskTodos are truly ephemeral per-run artifacts.

## Fix plan

**Phase 1 — Reset Tasks instead of deleting them**

In `DeleteWorkflowRunArtifacts`, replace the Task delete loop with a status reset:

```go
// For each Task anchored to this run:
task.Status = TaskStatusPending
task.CompletedAt = ""
task.WorkflowRunID = ""
task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
m.dm.UpdateEntity(ctx, agencyID, task.ID, updateReq)
// publish work.task.rolled_back event
```

**Phase 2 — Keep deleting TaskTodos**

TaskTodos are decomposition artifacts (created fresh each run). Hard delete of todos is correct and should be retained.

**Phase 3 — Handle tasks with null/empty workflow_run_id**

See [BUG-20260603-003](BUG-20260603-003_task-workflow-run-id-not-set.md) — tasks whose `workflow_run_id` is null/empty are invisible to the rollback query. Fix the root cause there so all tasks can be found by run ID.

## Verification

After fix:
1. Create tasks, assign to a workflow run, complete them
2. Call `RollbackWorkflowRun`
3. Tasks must still exist in the DB with `status = pending` and `workflow_run_id = ""`
4. TaskTodos for the run must be deleted
5. No relationship edges should be dropped

## Dependencies

- [BUG-20260603-003](BUG-20260603-003_task-workflow-run-id-not-set.md) — `workflow_run_id` not populated on tasks; must be fixed so rollback can locate all tasks for a given run
