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
| [BUG-09-023](bug-details/BUG-09-023_proto3_put_wipes_fields.md) | proto3 replace-all on PUT task silently wipes omitted fields | High | 📋 Open | — |
| [BUG-09-024](bug-details/BUG-09-024_auto_unblock_listener.md) | Auto-unblock listener for BLOCKED tasks when dependencies complete | Medium | 📋 Open | BUG-09-023 (cleaner with FieldMask, not strictly required) |

---

## BUG-09-023 — proto3 replace-all on PUT task wipes omitted fields

**Status**: 📋 Open · **Severity**: High · **Estimated effort**: ~1 day

`UpdateTaskRequest.task` is a full `Task` message; proto3 cannot distinguish "not sent" from "zero value". Any sparse PUT (e.g. `{ id, status, branch_name }`) writes empty strings back over `title`, `description`, `taskName`. Live evidence: AI received `Title: ""` and emitted a 4362-char `<think>` block of confusion instead of an actions block.

**Recommended fix**: add `google.protobuf.FieldMask update_mask` to `UpdateTaskRequest`; walk the mask in `task_impl_task.go:UpdateTask` instead of replacing the full property map. Keep replace-all behaviour when the mask is empty (deprecation period), then migrate internal callers and remove the fallback. Audit `UpdateAgent` (same pattern) and `UpdateRole`/`UpdateGoal`/`UpdateWorkflow` in CodeValdAgency.

**Workaround**: always GET → modify → PUT the full task body. Never send sparse PUTs.

See: [bug-details/BUG-09-023_proto3_put_wipes_fields.md](bug-details/BUG-09-023_proto3_put_wipes_fields.md)

---

## BUG-09-024 — Auto-unblock listener for BLOCKED tasks

**Status**: 📋 Open · **Severity**: Medium · **Estimated effort**: ~1 day

`AssignTask` correctly marks a dependent task as `blocked` when its source is non-terminal. The inverse trigger is missing: when the source `work.task.completed` fires, nothing flips the dependent from `blocked → pending` and re-emits `work.task.assigned`. Operators currently have to re-PUT the assignee manually to make the pipeline progress.

**Fix**: subscribe CodeValdWork to its own `work.task.completed`, walk inbound `depends_on` edges, re-check that all of a dependent's deps are terminal, then flip status and re-publish `work.task.assigned`. Guard against self-loops; handle cascade unblocks naturally per-event. Open design decision: should `failed` count as "satisfied" or cascade-fail the dependent? (Leaning cascade-fail.)

See: [bug-details/BUG-09-024_auto_unblock_listener.md](bug-details/BUG-09-024_auto_unblock_listener.md)
