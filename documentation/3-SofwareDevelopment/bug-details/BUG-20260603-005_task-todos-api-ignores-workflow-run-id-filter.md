# BUG-20260603-005 (Work) — `GET /work/{agency}/task-todos` ignores `workflow_run_id` query param

**Status:** 📋 Open
**Severity:** Medium — the WorkflowRun UI "Todos" tab likely shows an empty list even when todos exist; QA scripts cannot filter todos by run without direct ArangoDB access
**Owner:** CodeValdWork
**Estimated effort:** S — add `workflow_run_id` to the `ListTaskTodos` query handler
**Source finding:** QA scenario 09 run 2026-06-03 — `GET /work/utility-app-builder/task-todos?workflow_run_id=c4821356-...` returned `{"todos":[]}` even though 15 todos with that `workflow_run_id` existed in ArangoDB (`work_task_todos` collection)

## Problem

`GET /work/{agency}/task-todos` is the endpoint used by the WorkflowRun UI and QA scripts to list todos associated with a specific pipeline run. The endpoint accepts a `workflow_run_id` query parameter but does not apply it as a filter: all calls return an empty list regardless of parameter value (or possibly all todos without filtering — in this run, the unfiltered call also returned 0).

Direct ArangoDB query confirmed 15 todos in `work_task_todos` for run `c4821356-5c8e-4c98-bcc0-9c211a36a7fe`:
- 13 with `status=completed`
- 2 with `status=failed`

The API returned `{"todos":[]}` for both the filtered and unfiltered calls.

## Evidence

```bash
# API call (returns empty)
curl -s http://codevaldcross:8081/work/utility-app-builder/task-todos?workflow_run_id=c4821356-...
→ {"todos":[]}

# API call without filter (also returns empty)
curl -s http://codevaldcross:8081/work/utility-app-builder/task-todos
→ {"todos":[]}  (total:0)

# ArangoDB direct query (returns 15)
FOR doc IN work_task_todos
  FILTER doc.properties.workflow_run_id == "c4821356-5c8e-4c98-bcc0-9c211a36a7fe"
  RETURN {status: doc.properties.status, ...}
→ 15 documents (13 completed, 2 failed)
```

## Root cause

Two possible causes (requires code inspection to confirm which applies):

1. **Unfiltered endpoint returns empty by design or bug** — `ListTaskTodos` in `internal/server/` may have a missing or incorrect AQL query that always returns 0 results (e.g. wrong collection name, wrong property path).
2. **Filter parameter not wired** — `workflow_run_id` is accepted as a query param by Cross's route registration but is not injected into the AQL filter clause in the server handler.

The property path in ArangoDB is `doc.properties.workflow_run_id` (snake_case, nested under `properties`). If the query uses `doc.workflow_run_id` or `doc.properties.workflowRunId` (camelCase), 0 results is expected.

## Fix plan

1. Locate `ListTaskTodos` handler in `internal/server/` (likely `task_todos_server.go` or similar).
2. Confirm the AQL query hits the correct collection (`work_task_todos`) with the correct property path (`doc.properties.workflow_run_id`).
3. Ensure `workflow_run_id` from the request is used as a WHERE condition when non-empty.
4. Ensure the unfiltered call (no `workflow_run_id`) returns all todos for the agency (not empty).

## Verification

```bash
# Seed: create a todo with workflow_run_id="test-run-001"
# Call:
GET /work/{agency}/task-todos?workflow_run_id=test-run-001
# Expect: todo appears in response

GET /work/{agency}/task-todos
# Expect: all todos for agency appear (including the seeded one)
```

Integration test: add to `internal/server/integration_test.go` — create task + todo with `workflow_run_id`, call `ListTaskTodos` with filter, assert count > 0.

## Dependencies

None. Isolated to CodeValdWork `ListTaskTodos` handler and AQL query.
