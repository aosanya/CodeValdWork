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
| [BUG-09-024](bug-details/BUG-09-024_auto_unblock_listener.md) | Auto-unblock listener for BLOCKED tasks when dependencies complete | Medium | 📋 Open | — |

---

## BUG-09-024 — Auto-unblock listener for BLOCKED tasks

**Status**: 📋 Open · **Severity**: Medium · **Estimated effort**: ~1 day

`AssignTask` correctly marks a dependent task as `blocked` when its source is non-terminal. The inverse trigger is missing: when the source `work.task.completed` fires, nothing flips the dependent from `blocked → pending` and re-emits `work.task.assigned`. Operators currently have to re-PUT the assignee manually to make the pipeline progress.

**Fix**: subscribe CodeValdWork to its own `work.task.completed`, walk inbound `depends_on` edges, re-check that all of a dependent's deps are terminal, then flip status and re-publish `work.task.assigned`. Guard against self-loops; handle cascade unblocks naturally per-event. Open design decision: should `failed` count as "satisfied" or cascade-fail the dependent? (Leaning cascade-fail.)

See: [bug-details/BUG-09-024_auto_unblock_listener.md](bug-details/BUG-09-024_auto_unblock_listener.md)
