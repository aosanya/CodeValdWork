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
| [BUG-20260603-001](bug-details/BUG-20260603-001_workflow-run-status-never-advances.md) | WorkflowRun status never advances past PENDING | Medium | 📋 Open | — |

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
