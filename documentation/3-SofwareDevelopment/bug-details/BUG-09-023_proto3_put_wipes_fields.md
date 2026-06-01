# BUG-09-023 — proto3 replace-all on PUT task silently wipes omitted fields

**Status:** ✅ Fixed (2026-06-01, main)
**Severity:** High — operator/agent partial updates destroy unsent fields with no warning
**Owner:** CodeValdWork
**Estimated effort:** ~1 day (proto schema + handler + a couple of consumers)
**Source finding:** [`/4-QA/agencies/utility-app-builder/bugs/09-mvp-sf-pipeline-findings.md`](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/bugs/09-mvp-sf-pipeline-findings.md)

## Resolution

Implemented Option A (FieldMask) at the gRPC server boundary, leaving the manager interface unchanged so internal callers keep their existing GET-then-PUT pattern unmodified.

- `proto/codevaldwork/v1/service.proto` — added `google.protobuf.FieldMask update_mask = 4;` to `UpdateTaskRequest`.
- `internal/server/server.go:UpdateTask` — when `update_mask` is set, the handler reads the current task, copies only the listed fields from the incoming `Task` onto the current value (via `applyTaskMask`), and writes that merged task. When the mask is empty, the legacy replace-all path runs with a deprecation log so call sites can be migrated.
- Side fixes uncovered by the work:
  - `protoToTask` was missing `SeparateBranch` / `BranchName` — added.
  - `proto/codevaldwork/v1/codevaldwork.proto` had a stale `reserved "assigned_to";` that collided with the live `assigned_to = 20` field and blocked `make proto`; removed (the field-number reservation `reserved 7;` is unchanged).
  - `TASK_STATUS_BLOCKED = 6` was added to the proto enum now that codegen runs again — the Go side already used the matching `"blocked"` constant.
- Tests: `internal/server/update_task_mask_test.go` covers the reproducer (`TestUpdateTask_WithMask_PreservesUnlistedFields`), the explicit-clear case, the legacy fallback, and unknown-path tolerance.

Pending follow-ups (separate issues, intentionally out of scope here):
- Migrate every internal `UpdateTask` caller to send an explicit `update_mask` and then delete the deprecation fallback.
- Apply the same FieldMask pattern to `UpdateAgent` (CodeValdWork) and `UpdateRole` / `UpdateGoal` / `UpdateWorkflow` (CodeValdAgency).

---

## Reproducer

```bash
# 1. Get the current task
curl -s ${BASE}/work/${AGENCY}/tasks/${MVP_SF_001_ID} -u "$CV_AUTH" | jq .task
# task.title: "Project Scaffolding"   task.description: "flutter create sharedfarms; ..."

# 2. PUT to clear ONLY branch_name
curl -s -X PUT ${BASE}/work/${AGENCY}/tasks/${MVP_SF_001_ID} \
  -u "$CV_AUTH" -H "Content-Type: application/json" \
  -d '{ "task": { "id": "'$MVP_SF_001_ID'", "status": "TASK_STATUS_IN_PROGRESS", "branch_name": "" } }'

# 3. GET again
curl -s ${BASE}/work/${AGENCY}/tasks/${MVP_SF_001_ID} -u "$CV_AUTH" | jq .task
# task.title: ""        <-- WIPED
# task.description: ""  <-- WIPED
# task.taskName: null   <-- WIPED
```

The task becomes unusable: the AI receives `Title: ""` and `Description: ""` in the assigned event and outputs prose-of-confusion instead of actions (verified live: 4362-char free-form `<think>` block from DeepSeek complaining "Title and Description are empty, possibly this is a generic task assignment").

## Root cause

`UpdateTaskRequest.task` in [`proto/codevaldwork/v1/service.proto`](../../../proto/codevaldwork/v1/service.proto) is a full `Task` message. proto3 has no way to distinguish "field not sent" from "field set to zero value", so the handler in [`task_impl_task.go:UpdateTask`](../../../task_impl_task.go) receives empty strings for everything the caller didn't include and writes all of them back via `taskToProperties(task)`.

This is a general proto3 footgun. [gRPC FieldMask](https://protobuf.dev/reference/protobuf/google.protobuf/#field-mask) is the canonical fix.

## Concrete fix plan

### Option A — Add a `field_mask` to UpdateTaskRequest (recommended, ~6h)

1. Add `google.protobuf.FieldMask update_mask = 4;` to `UpdateTaskRequest` in [`service.proto`](../../../proto/codevaldwork/v1/service.proto). Run `make proto` to regenerate.
2. In [`task_impl_task.go:UpdateTask`](../../../task_impl_task.go), instead of building a full property map from the incoming `task`, walk `update_mask.paths` and only set the matching properties on the existing entity. Fields not in the mask are left untouched.
3. **Backwards compat:** when `update_mask` is empty, fall back to the old replace-all behaviour so existing callers don't break immediately. Add a deprecation log line.
4. Update the assignee endpoint in `assignment.go` (it does its own status changes — make sure they go through a fielded path, not via the full Task message).
5. Migrate every internal caller of `UpdateTask` to set `update_mask`. Touchpoints found this session: `task_impl_task.go:maybeCompleteParentTask`, the `setTaskStatus` helper in [`assignment.go`](../../../assignment.go) (already uses `GetEntity` + `UpdateEntity` directly to dodge this gap — good template).
6. Remove the deprecation fallback once all internal callers are migrated.

### Option B — Add `PATCH` routes for the common partial updates (~3h)

1. `PATCH /work/{agencyId}/tasks/{taskId}/branch_name` with body `{ "branch_name": "..." }`.
2. Same for `/status` (already exists at the assignee level).
3. Frontend + scripts use the PATCH endpoints; reserve PUT for full replacements.
4. Lower-risk than option A but doesn't fix the general footgun for future fields.

## Verification once fixed

- Reproducer above: after step 2, GET shows `title`, `description`, `taskName` unchanged from step 1.
- Add a Work-1 verdict in /09 docs: "after a partial PUT, every field NOT in the update payload retains its prior value (verified via diff against a pre-PUT snapshot)."

## Affected APIs found this session

Any RPC that takes a full entity message in an `Update*Request` is suspect. Audit list:
- `UpdateTask` (this bug)
- `UpdateAgent` (CodeValdWork) — same pattern
- `UpdateRole` / `UpdateGoal` / `UpdateWorkflow` (CodeValdAgency) — same pattern, but agency entities are read-only after publish so lower risk
- `UpdateBranch` (CodeValdGit) — does not exist; merge/delete are the only mutators

## Workaround for current operators

Until fixed, **always GET → modify → PUT the full task** when updating any field:

```bash
GET=$(curl -s ${BASE}/work/${AGENCY}/tasks/${TID} -u "$CV_AUTH")
echo "$GET" | jq '.task | .branch_name = ""' | curl -X PUT ${BASE}/work/${AGENCY}/tasks/${TID} -d @-
```

Never PUT a sparse `{ task: { id, status, branch_name: "" } }` body — the omitted fields will be lost.

## Dependencies on other gaps

- Required by BUG-09-020 phase 2 (the dispatcher needs to add `expected_writes` to a todo mid-flight without wiping other fields).
- Required by BUG-09-024 (the unblock listener must flip `status: blocked → pending` without touching anything else).
