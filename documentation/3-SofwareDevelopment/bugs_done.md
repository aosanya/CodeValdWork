# CodeValdWork — Fixed Bugs

Bugs marked Fixed are removed from `bugs.md` and recorded here with their resolution date and the commit / branch that landed the fix.

| Bug ID | Title | Severity | Fixed Date | Commit / Branch | Detail |
|--------|-------|----------|------------|-----------------|--------|
| BUG-20260603-005 | `GET /work/{agency}/task-todos` ignores `workflow_run_id` query param — returns empty list | Medium | 2026-06-03 | main (bcbdf28) | [bug-details/BUG-20260603-005](bug-details/BUG-20260603-005_task-todos-api-ignores-workflow-run-id-filter.md) |
| BUG-20260603-003 | Tasks completed by a workflow run have workflow_run_id null or empty | High | 2026-06-03 | main | [bug-details/BUG-20260603-003](bug-details/BUG-20260603-003_task-workflow-run-id-not-set.md) |
| BUG-20260603-002 | RollbackWorkflowRun hard-deletes Tasks instead of resetting to pending | High | 2026-06-03 | main | [bug-details/BUG-20260603-002](bug-details/BUG-20260603-002_rollback-deletes-tasks-instead-of-resetting.md) |
| BUG-09-023 | proto3 replace-all on PUT task silently wipes omitted fields | High | 2026-06-01 | main | [bug-details/BUG-09-023_proto3_put_wipes_fields.md](bug-details/BUG-09-023_proto3_put_wipes_fields.md) |
| BUG-09-024 | Auto-unblock listener for BLOCKED tasks when dependencies complete | Medium | 2026-06-01 | main | [bug-details/BUG-09-024_auto_unblock_listener.md](bug-details/BUG-09-024_auto_unblock_listener.md) |
