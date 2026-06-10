# CodeValdWork — Fixed Bugs

Bugs marked Fixed are removed from `bugs.md` and recorded here with their resolution date and the commit / branch that landed the fix.

| Bug ID | Title | Severity | Fixed Date | Commit / Branch | Detail |
|--------|-------|----------|------------|-----------------|--------|
| BUG-20260603-004 | Project-name URL routing was case-sensitive; display-name casing returned 404. `GetProjectByName` now normalizes the lookup key through `toSlug`, matching the slug stored at `CreateProject` time | Medium | 2026-06-10 | main (c749787) | [bug-details/BUG-20260603-004](bug-details/BUG-20260603-004_project-name-routing-case-sensitive.md) |
| BUG-20260609-001 | Drop `work.` domain prefix from published topic names (system-wide rename — SharedLib + Work + Implementations + Cross + AI + Functions + Agency) | High | 2026-06-09 | main (7d676fa) | [bug-details/BUG-20260609-001](bug-details/BUG-20260609-001_drop_work_domain_prefix.md) |
| BUG-20260603-007 | `maybeCompleteParentTask` counts superseded todos from previous decompositions — direction-driven retry can never complete | High | 2026-06-03 | main | [bug-details/BUG-20260603-007](bug-details/BUG-20260603-007_maybe-complete-parent-task-counts-superseded-todos.md) |
| BUG-20260603-006 | TaskStatus state machine blocks `failed → in_progress`, breaking direction-driven retry | High | 2026-06-03 | main | [bug-details/BUG-20260603-006](bug-details/BUG-20260603-006_task-state-machine-blocks-direction-retry.md) |
| BUG-20260603-001 | WorkflowRun status never advances past PENDING (handler existed but Work wasn't subscribed to `work.pipeline.started`; fixed by routing config default through `events.go ConsumedTopics()`) | Medium | 2026-06-03 | main | [bug-details/BUG-20260603-001](bug-details/BUG-20260603-001_workflow-run-status-never-advances.md) |
| BUG-20260603-005 | `GET /work/{agency}/task-todos` ignores `workflow_run_id` query param — returns empty list | Medium | 2026-06-03 | main (bcbdf28) | [bug-details/BUG-20260603-005](bug-details/BUG-20260603-005_task-todos-api-ignores-workflow-run-id-filter.md) |
| BUG-20260603-003 | Tasks completed by a workflow run have workflow_run_id null or empty | High | 2026-06-03 | main | [bug-details/BUG-20260603-003](bug-details/BUG-20260603-003_task-workflow-run-id-not-set.md) |
| BUG-20260603-002 | RollbackWorkflowRun hard-deletes Tasks instead of resetting to pending | High | 2026-06-03 | main | [bug-details/BUG-20260603-002](bug-details/BUG-20260603-002_rollback-deletes-tasks-instead-of-resetting.md) |
| BUG-09-023 | proto3 replace-all on PUT task silently wipes omitted fields | High | 2026-06-01 | main | [bug-details/BUG-09-023_proto3_put_wipes_fields.md](bug-details/BUG-09-023_proto3_put_wipes_fields.md) |
| BUG-09-024 | Auto-unblock listener for BLOCKED tasks when dependencies complete | Medium | 2026-06-01 | main | [bug-details/BUG-09-024_auto_unblock_listener.md](bug-details/BUG-09-024_auto_unblock_listener.md) |
