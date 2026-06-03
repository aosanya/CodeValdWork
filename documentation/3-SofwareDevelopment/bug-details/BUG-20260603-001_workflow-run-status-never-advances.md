# BUG-20260603-001 — WorkflowRun status stays PENDING throughout pipeline execution

**Status:** ✅ Fixed (2026-06-03)
**Severity:** Medium — the WorkflowRun record exists and the pipeline executes, but every run shows Pending indefinitely in the UI; operators cannot distinguish an active run from a stalled one
**Owner:** CodeValdWork
**Estimated effort:** ~0.5 day (two subscriber hooks + a state-machine helper)
**Source finding:** QA scenario 09 run (2026-06-03) — `ce0b3d84-2782-4433-a14a-138163af87de` showed Pending 7 minutes after tasks were assigned and an AI run completed. Re-surfaced 2026-06-03 evening via QA scenario 11 setup: `pipeline-2026-06-03-193315-132b73` (id `2e870cda-...`) stuck Pending forever despite `work.pipeline.started` firing — the handler had been implemented but Work wasn't subscribed to the topic.

## Resolution

Fix landed in two pieces:

1. **Handler implemented earlier** — `internal/server/run_status_handler.go` consumes `work.task.assigned`, `work.pipeline.started`, `work.task.failed`, `functions.job.failed`, etc. and drives the lifecycle transitions (PENDING → IN_PROGRESS → COMPLETED / FAILED).
2. **Subscription list completed 2026-06-03 evening** — `internal/config/config.go` was using a hand-maintained `WORK_SUBSCRIBE_TOPICS` env default that was missing five topics that `events.go ConsumedTopics()` listed (`work.pipeline.started`, `work.run.timeout`, `work.task.timeout`, `work.task.failure-classified`, plus `ai.task.*` / `git.file.written` that were referenced from the dispatcher but not in either list). Fix replaces the hand-maintained default with `strings.Join(codevaldwork.ConsumedTopics(), ",")` so there is a single source of truth, and adds the missing topics to `ConsumedTopics()` so the dispatcher's switch cases are matched by the registry's subscription set.

**Verification:** Cross's `/services/registry?agencyId=utility-app-builder` for `codevaldwork` now lists 20 consumed topics (was 16), including `work.pipeline.started`. New pipeline runs created via `start-pipeline` flip PENDING → IN_PROGRESS within seconds of the `work.pipeline.started` event, even when no task is assigned by `next-task` (dependents blocked, empty queue, etc.).

## Problem

`CreateWorkflowRun` sets `status = WORKFLOW_RUN_STATUS_PENDING` and nothing ever advances it. The WorkflowRun status never reflects what is actually happening: tasks are assigned, AI runs, todos execute, and tasks complete — the run stays Pending throughout.

Expected lifecycle:

| Trigger | Transition |
|---|---|
| First task in the run is assigned (`work.task.assigned`) | PENDING → IN_PROGRESS |
| All tasks terminal (`COMPLETED` or `CANCELLED`) and at least one `COMPLETED` | IN_PROGRESS → COMPLETED |
| Any task in the run reaches `FAILED` | IN_PROGRESS → FAILED |
| `CancelWorkflowRun` called | any → CANCELLING → CANCELLED |

The cancel transition is already implemented. The other three are missing.

## Evidence

```bash
# Run created at 07:02:28, checked at 07:09:xx (7 min after tasks assigned + AI run finished)
curl -s "${BASE}/work/utility-app-builder/workflow-runs/ce0b3d84-..." -u "$CV_AUTH" \
  | python3 -c "import sys,json; r=json.load(sys.stdin)['run']; print(r['status'])"
# WORKFLOW_RUN_STATUS_PENDING

# WorkflowRun in WorkFrontend shows "● Pending" badge with no transition
```

## Root cause

CodeValdWork's event dispatcher (`internal/server/event_dispatcher.go`) handles `work.task.assigned`, `work.task.completed`, and `work.task.failed` for other purposes but does not look up the task's WorkflowRun membership or update the run's status.

`CreateWorkflowRun` in `internal/server/workflow_run_server.go` writes the run at `PENDING` and returns immediately. No post-creation hook sets up a state-machine listener.

## Fix plan

### Step 1 — PENDING → IN_PROGRESS on first task assign

In `internal/server/event_dispatcher.go`, extend the `work.task.assigned` handler (or add a new one) to:

1. Deserialise the `TaskAssignedPayload` and extract `TaskID` + `AgencyID`.
2. Look up the task's WorkflowRun edge: `GET /work/{agency}/tasks/{taskId}/workflow-run`.
3. If a run exists and `run.status == PENDING`: call `setWorkflowRunStatus(ctx, agencyID, runID, IN_PROGRESS)`.
4. Publish `work.run.in_progress` (existing topic) carrying `{workflow_run_id, agency_id}`.

Idempotency: a second `work.task.assigned` for the same run is a no-op because the guard checks current status.

### Step 2 — IN_PROGRESS → COMPLETED / FAILED on task terminal

Extend the `work.task.completed` / `work.task.failed` handlers:

1. Fetch the task's WorkflowRun edge.
2. If no run, return — task is not part of a pipeline.
3. Query all tasks whose `workflow-run` edge points to this run.
4. If any task is `FAILED`: transition run → `FAILED`, publish `work.run.failed`.
5. Else if all tasks are terminal (COMPLETED or CANCELLED): transition run → `COMPLETED`, publish `work.run.completed`.
6. Otherwise: leave at IN_PROGRESS (more tasks pending).

### Step 3 — `setWorkflowRunStatus` helper

Add a private helper in `workflow_run_server.go`:

```go
func (s *Server) setWorkflowRunStatus(ctx context.Context, agencyID, runID string, status WorkflowRunStatus) error {
    run, err := s.manager.GetWorkflowRun(ctx, agencyID, runID)
    if err != nil { return err }
    if run.Status == status { return nil } // idempotent
    return s.manager.UpdateWorkflowRunStatus(ctx, agencyID, runID, status)
}
```

`UpdateWorkflowRunStatus` may already exist on the manager (check `workflow_run.go`); if not, add it using the same FieldMask pattern as the existing `UpdateTask`.

### Step 4 — Subscribe to required topics

Verify `internal/registrar/registrar.go` Consumes list includes:
- `work.task.assigned` — needed to detect first assignment (Step 1)
- `work.task.completed` — already present for auto-unblock (BUG-09-024)
- `work.task.failed` — add if missing

## Verification

```bash
# 1. Publish work.pipeline.requested → observe WorkflowRun created at PENDING
# 2. Publish work.next.requested → observe next-task assigns a task
# 3. Within 5s: GET workflow-run → status should be IN_PROGRESS
# 4. Complete all tasks in the run
# 5. GET workflow-run → status should be COMPLETED
# 6. WorkFrontend /workflow-runs page should show "In Progress" then "Completed" badge
```

## Dependencies

- Touches the same dispatcher extension point as [BUG-09-024](BUG-09-024_auto_unblock_listener.md) (auto-unblock). No hard dependency — can land independently.
- The WorkflowRun `cancel` flow (FEAT-20260602-008) already handles `CANCELLING → CANCELLED`; this bug adds the other three transitions.
