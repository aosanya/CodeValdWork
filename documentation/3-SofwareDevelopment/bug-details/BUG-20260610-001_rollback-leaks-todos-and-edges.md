# BUG-20260610-001 — `RollbackWorkflowRun` leaks TaskTodos and dangling relationship edges

**Status:** ❌ Invalid (2026-06-10) — both findings were false positives from a buggy verification check in `/dev-rollback-workflow`. The CodeValdWork rollback implementation is correct.
**Severity:** N/A
**Owner:** CodeValdWork
**Source finding:** Session 2026-06-10 — operator ran `/dev-rollback-workflow` against all 7 workflow runs in `codevald_demo`; verification reported leftover TaskTodos and dangling edges. Live reproduction against the same running binary (`codevald-local/codevaldwork:local` built 2026-06-10T10:56Z) shows the rollback is correct.

## Why the original report was wrong

The original "leak" report was produced by two verification queries that each made a wrong assumption about the rollback contract:

### Wrong assumption 1 — that TaskTodos should disappear from the raw collection

`DeleteWorkflowRunArtifacts` calls `m.dm.DeleteEntity` on each TaskTodo. The `entitygraph` `DeleteEntity` implementation is **soft-delete only** — it flips `doc.deleted = true` and records `doc.deleted_at`, but never hard-removes the document (`CodeValdSharedLib/entitygraph/arangodb/entities.go:120`). All read paths (`ListEntities`, `GetEntity`, `UpsertEntity`) filter `doc.deleted != true`, so the application never sees soft-deleted rows.

The verification check used:

```aql
FOR doc IN work_task_todos FILTER doc.properties.workflow_run_id == @r RETURN doc._key
```

which **did not filter `doc.deleted`** — so it correctly returned soft-deleted todos as if they were still live. The 36 "leaked" todos across 3 runs were all `deleted=true`. The right check is:

```aql
FOR doc IN work_task_todos
  FILTER doc.deleted != true
    AND doc.properties.workflow_run_id == @r
  RETURN doc._key
```

### Wrong assumption 2 — that Tasks are deleted (so edges become dangling)

[BUG-20260603-002](BUG-20260603-002_rollback-deletes-tasks-instead-of-resetting.md) landed in commit `f48740a` (2026-06-03). Since that commit Tasks are **reset** during rollback (`status → pending`, clear `workflow_run_id` + `completed_at`), not deleted. Only the `started_task` edge is removed; all structural edges (`member_of`, `has_tag`, `depends_on`, `blocks`) are preserved on purpose, because the Task is a long-lived work item that belongs to the project regardless of which run touched it.

The verification check enumerated every edge whose `_from` or `_to` pointed at the BEFORE-snapshot task keys and flagged them as "dangling." Those edges were not dangling — they were the Task's structural relationships, correctly retained because the Task was reset (not deleted).

## Live reproduction (2026-06-10) — current binary works correctly

Direct AQL setup + gRPC trigger against the running binary:

1. Insert a `failed` WorkflowRun, one Task with `workflow_run_id`, and two TaskTodos with `workflow_run_id`.
2. Call `RollbackWorkflowRun` via grpcurl.
3. Observe:
   - Run document: `status = rolled_back` ✓
   - Task document: `status = pending`, `workflow_run_id = ""`, `deleted = false` ✓ (correctly reset, not deleted)
   - Both TaskTodo documents: `deleted = true` ✓ (correctly soft-deleted)

Unit test `TestDeleteWorkflowRunArtifacts_DeletesTaskTodosForRun` added in this session (`workflow_run_rollback_test.go`) further confirms the in-memory logic.

## Lessons captured

1. `/dev-rollback-workflow` Step 3 verification (Checks 3 + 4) was rewritten to honour the actual rollback contract: filter `doc.deleted != true` for todos; do not flag edges on reset Tasks as dangling.
2. Future `entitygraph`-aware DB checks must always filter `doc.deleted != true` — raw `FOR doc IN <collection>` returns soft-deleted rows.

## Collateral damage from this session's cleanup sweep

Before discovering the verification bug, the skill's cleanup sweep hard-deleted 36 (already-soft-deleted) TaskTodos and 31 structural edges from the 3 reset Tasks. The Tasks themselves are intact and reset to `pending`; only some of their structural edges are missing. Re-running the affected workflows will regenerate any edges the operator needs.
