# CodeValdWork — Active Bug Backlog

## Overview

Bugs in scope for CodeValdWork. Mirrors the `mvp.md` / `mvp_done.md` / `mvp-details/` layout used for feature work.

- **Fixed bugs**: see [`bugs_done.md`](bugs_done.md)
- **Per-bug detail**: see [`bug-details/`](bug-details/)
- **Master cross-service queue**: [`../../../CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md`](../../../CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md)

## Workflow

### Completion Process (MANDATORY)
1. Implement and validate (`go build ./...`, `go vet ./...`, `go test -race ./...`)
2. Move the bug row from this file to `bugs_done.md`
3. Update the detail file's Status header to `✅ Fixed (YYYY-MM-DD)` and cite the commit / branch
4. Strike-through + ✅ the entry on the master prioritization.md
5. Merge feature branch to main

### Status Legend
- 📋 **Open** — not yet started or in triage
- 🚀 **In Progress** — actively being worked
- ⏸️ **Blocked** — waiting on a dependency
- ✅ **Fixed** — moved to `bugs_done.md` (do not list here)

---

## Active Bugs

| Bug ID | Title | Severity | Status | Depends On |
|--------|-------|----------|--------|------------|
| ~~[BUG-20260610-002](bug-details/BUG-20260610-002_applytaskstatus-hardcoded-transitions.md)~~ | ~~`applyAITaskStatus` hardcodes transitions; the active CodeValdAgency publication's event_flows + work plans are not enforced at runtime; legacy work plans from prior imports still fire~~ | High | ✅ Fixed (2026-06-10) | — |
| ~~[BUG-20260610-001](bug-details/BUG-20260610-001_rollback-leaks-todos-and-edges.md)~~ | ~~`RollbackWorkflowRun` leaks TaskTodos and dangling `work_relationships` edges~~ | N/A | ❌ Invalid (2026-06-10) — verification check bug, not a code bug; rollback works correctly | — |
| ~~[BUG-20260609-001](bug-details/BUG-20260609-001_drop_work_domain_prefix.md)~~ | ~~Drop `work.` domain prefix from published topic names (system-wide rename; paired with CodeValdAI)~~ | High | ✅ Fixed (2026-06-09) | — |
| ~~[BUG-20260603-005](bug-details/BUG-20260603-005_task-todos-api-ignores-workflow-run-id-filter.md)~~ | ~~`GET /work/{agency}/task-todos` ignores `workflow_run_id` query param — returns empty list~~ | Medium | ✅ Fixed (2026-06-03) | — |
| ~~[BUG-20260603-004](bug-details/BUG-20260603-004_project-name-routing-case-sensitive.md)~~ | ~~Project-name URL routing is case-sensitive; display-name casing returns 404~~ | Medium | ✅ Fixed (2026-06-10) | — |
| ~~[BUG-20260603-001](bug-details/BUG-20260603-001_workflow-run-status-never-advances.md)~~ | ~~WorkflowRun status never advances past PENDING~~ | Medium | ✅ Fixed (2026-06-03) | — |

---

### BUG-20260610-002 — Active CodeValdAgency publication is not enforced at runtime; legacy work plans still fire; Work bypasses event_flows entirely

**Severity:** High — silent flow violation. Planner / non-developer AgentRuns flip the parent Task to COMPLETED without doing the work; downstream gates fire on a lie. Legacy plans from prior imports continue triggering handlers the active publication doesn't declare.
**Status:** ✅ Fixed (2026-06-10) — five-commit fix across CodeValdAgency (`af53521`, `12cb51e`, `d7bd2e2`) and CodeValdWork (`79f25dc`). Agency now projects per-workflow event_flows into queryable EventFlowStep entities, retires legacy work plans on re-import, and exposes a `LookupFlowStep` RPC. Work's `applyAITaskStatus` now defers parent-Task completion whenever the parent has any TaskTodo (decompose-mode signal); rollback now actually clears `completed_at`. Local-state heuristic is interim — the full LookupFlowStep wiring lands when CodeValdAI starts publishing `handler_code` in task.* payloads.
**Detail:** [bug-details/BUG-20260610-002](bug-details/BUG-20260610-002_applytaskstatus-hardcoded-transitions.md)

Once an agency.json is imported and promoted, the persisted CodeValdAgency state — `event_flows` entities + `work plans` under the active `AgencyPublication` — IS the runtime source of truth. The on-disk flow file is only input to the import. Today, `internal/server/event_dispatcher.go:235-244` unconditionally maps `task.started/completed/failed → IN_PROGRESS/COMPLETED/FAILED` on the parent Task, consulting nothing about the active publication's declared transitions. Live repro 2026-06-10T11:33–35Z: `MVP-SF-001` planner AgentRun completed → parent Task flipped to COMPLETED with 0 todos created. Compounding: `PromoteDraft` doesn't retire prior-publication work plans, so legacy handlers continue firing even when the active publication wouldn't declare them.

Fix path covers: (1) verify the import round-trips every declared transition; (2) `PromoteDraft` retires legacy work plans; (3) Agency exposes a `LookupFlowStep` RPC; (4) Work uses it before applying any AI-emitted transition. Secondary corruption: rollback / reset does not clear `properties.completed_at`, so the next legitimate completion event surfaces a stale 2026-06-03 timestamp.

---

### BUG-20260610-001 — `RollbackWorkflowRun` leaks TaskTodos and dangling relationship edges

**Severity:** N/A
**Status:** ❌ Invalid (2026-06-10) — verification check bug in `/dev-rollback-workflow`, not a code bug

Live reproduction against the current binary confirmed: TaskTodos are correctly soft-deleted (`deleted=true`); Tasks are correctly reset to `pending` (not deleted), so their structural edges are correctly preserved. The original "leak" report came from two verification queries that did not honour the rollback contract (didn't filter `doc.deleted`, assumed tasks were deleted). The skill's Checks 3 + 4 have been rewritten and a unit test added.

See [bug-details/BUG-20260610-001](bug-details/BUG-20260610-001_rollback-leaks-todos-and-edges.md) for the full post-mortem.

---

### BUG-20260603-005 — `GET /work/{agency}/task-todos` ignores `workflow_run_id` query param

**Severity:** Medium — WorkflowRun UI "Todos" tab shows empty; QA scripts cannot filter todos by run without direct ArangoDB access
**Status:** 📋 Open

`GET /work/{agency}/task-todos?workflow_run_id=<id>` returns `{"todos":[]}` even when matching todos exist in ArangoDB. Verified in QA scenario 09 (2026-06-03): 15 todos with `workflow_run_id=c4821356-...` in `work_task_todos` collection; API returned 0 for both filtered and unfiltered calls. Root cause: the `ListTaskTodos` handler likely uses the wrong ArangoDB property path (`doc.workflow_run_id` instead of `doc.properties.workflow_run_id`) or does not wire the query param into the AQL filter.

See [bug-details/BUG-20260603-005](bug-details/BUG-20260603-005_task-todos-api-ignores-workflow-run-id-filter.md) for full fix plan.

---

### BUG-20260603-004 — Project-name URL routing is case-sensitive; display-name casing returns 404

**Severity:** Medium
**Status:** ✅ Fixed (2026-06-10, main `c749787`)

`GET /projects/{projectName}/tasks` (and all project-scoped routes) performs an exact-match ArangoDB lookup on `projectName`. Slugs are stored lowercase (`sharedfarms`) but callers deriving the URL from the display name (`SharedFarms`) receive 404. CodeValdWorkFrontend renders "Failed to load project." when the QA doc URL used the display-name casing.

**Fix:** Apply `strings.ToLower()` to `projectName` in the request handler (or at the query layer) in `internal/server/project_server.go` before passing it to ArangoDB.

See [bug-details/BUG-20260603-004](bug-details/BUG-20260603-004_project-name-routing-case-sensitive.md) for full fix plan and workaround.

---

### BUG-20260603-001 — WorkflowRun status never advances past PENDING

**Severity:** Medium
**Status:** 📋 Open

WorkflowRun is created at `PENDING` and never transitions to `IN_PROGRESS`, `COMPLETED`, or `FAILED` during normal pipeline execution. The cancel flow (FEAT-20260602-008) already handles `CANCELLING → CANCELLED`; the other three transitions are missing.

**Root cause:** The event dispatcher does not hook `work.task.assigned` / `work.task.completed` / `work.task.failed` to look up the task's parent WorkflowRun and advance its status.

**Fix:** Add three dispatcher hooks in `internal/server/event_dispatcher.go`:
1. `work.task.assigned` → flip run PENDING → IN_PROGRESS (first assignment only).
2. `work.task.completed` → check all run tasks; flip IN_PROGRESS → COMPLETED when all are terminal.
3. `work.task.failed` → flip IN_PROGRESS → FAILED immediately.

See [bug-details/BUG-20260603-001_workflow-run-status-never-advances.md](bug-details/BUG-20260603-001_workflow-run-status-never-advances.md) for full fix plan.

---

### BUG-20260603-002 — RollbackWorkflowRun hard-deletes Tasks instead of resetting to pending

**Severity:** High
**Status:** 📋 Open
**Depends on:** BUG-20260603-003

`DeleteWorkflowRunArtifacts` issues hard deletes on every Task anchored to the run ID. Tasks are long-lived project work items and must not be deleted on rollback — only their status should be reset to `pending` and `workflow_run_id` cleared. TaskTodos (ephemeral per-run decomposition artifacts) should continue to be deleted.

See [bug-details/BUG-20260603-002](bug-details/BUG-20260603-002_rollback-deletes-tasks-instead-of-resetting.md) for full fix plan.

---

### BUG-20260603-003 — Tasks completed by a workflow run have `workflow_run_id` null or empty

**Severity:** High
**Status:** 📋 Open

Tasks transitioned to `completed` (or `in_progress`) as part of a workflow run do not have `workflow_run_id` stamped onto the task document. The rollback filter (`FILTER doc.properties.workflow_run_id == @run_id`) silently skips these tasks, leaving them in their completed state after rollback.

See [bug-details/BUG-20260603-003](bug-details/BUG-20260603-003_task-workflow-run-id-not-set.md) for full fix plan.
