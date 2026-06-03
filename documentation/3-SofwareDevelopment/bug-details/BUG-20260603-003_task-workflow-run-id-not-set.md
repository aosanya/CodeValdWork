# BUG-20260603-003 — Tasks created by a workflow run have `workflow_run_id` null or empty

**Status:** ✅ Fixed (2026-06-03)
**Severity:** High — rollback cannot find tasks by run ID; they survive the rollback in their completed state and must be cleaned up manually
**Owner:** CodeValdWork
**Estimated effort:** ~0.5 day (identify creation path, ensure field is stamped)
**Source finding:** Session 2026-06-03 — after rolling back run `ce0b3d84`, tasks MVP-SF-001 and MVP-SF-002 remained `completed`; DB inspection showed `workflow_run_id: null` and `workflow_run_id: ""` respectively

## Problem

When a workflow run creates or completes a Task, the `workflow_run_id` field on the task document is not set (or is set to empty string). `DeleteWorkflowRunArtifacts` filters tasks with:

```aql
FOR doc IN work_tasks
  FILTER doc.properties.workflow_run_id == @run_id
```

A `null` or `""` value never matches a real UUID, so these tasks are silently skipped. The run completes "successfully" but its tasks remain completed in the DB.

## Evidence

```json
// MVP-SF-001 after the run completed:
{ "_key": "1c320ad8-...", "properties": { "workflow_run_id": null, "status": "completed" } }

// MVP-SF-002 after the run completed:
{ "_key": "73540b47-...", "properties": { "workflow_run_id": "", "status": "completed" } }
```

The run ID was `ce0b3d84-2782-4433-a14a-138163af87de`.

## Root cause

Unknown which code path creates/updates tasks for a workflow run without stamping `workflow_run_id`. Candidates:

1. **Task creation path** — `CreateTask` or `CreateTaskInProject` does not accept or store `workflow_run_id` at creation time; the caller (CodeValdAI dispatch or a seeding script) omits it
2. **Task completion path** — when a task is marked `completed`, the handler that writes the status does not carry the run context forward and set `workflow_run_id`
3. **Import / seed path** — tasks were seeded via `import.go` which does not wire `workflow_run_id`

The empty-string case (`""`) vs null suggests two separate callers with different behaviour.

## Fix plan

1. Find every code path that transitions a Task to `completed` (or `in_progress`) in response to a workflow run event
2. Ensure each path writes `workflow_run_id` onto the task properties at that moment
3. Add a server-side guard in `UpdateTask`: if the new status is `in_progress` or `completed` and `workflow_run_id` is empty, return `INVALID_ARGUMENT`
4. Add integration test: create task → assign via run → complete via run → assert `workflow_run_id` is set

## Verification

After fix: call the API to complete a task as part of a run, then read the task back — `workflow_run_id` must equal the run ID.

## Dependencies

- Upstream of [BUG-20260603-002](BUG-20260603-002_rollback-deletes-tasks-instead-of-resetting.md) — rollback reset logic depends on finding tasks by run ID
