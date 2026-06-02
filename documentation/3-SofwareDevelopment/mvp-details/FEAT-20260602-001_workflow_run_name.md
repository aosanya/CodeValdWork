# FEAT-20260602-001 — `WorkflowRun.name` optional/generated field

**Status:** 📋 Not Started
**Severity:** Medium — without a name, the only way to find a specific run is by ID (which the caller doesn't know until after creation), making correlation under parallel runs fragile and the UI list unreadable
**Owner:** CodeValdWork
**Estimated effort:** ~0.5 day (schema property + name generator + uniqueness check + list filter)
**Source finding:** This conversation (2026-06-02) — surfaced while designing the `start-pipeline` entry function; the test step needs a deterministic correlation handle to find the run it just created without polling

---

## Problem

The `WorkflowRun` entity ([FEAT-20260601-001](FEAT-20260601-001_workflow_run_rollup.md)) carries `trigger_event` and `initiator`, but no human-readable label. Two practical consequences:

1. **Correlation.** When the QA test publishes `work.pipeline.requested` and waits for the run to be created (asynchronously by `start-pipeline`), it cannot find *its* run unless it either (a) polls `/workflow-runs` newest-first (race-prone under parallel runs) or (b) listens on SSE for the confirmation event. Both are heavier than just `GET /workflow-runs?name=qa-scenario-09-...`.
2. **UI readability.** The `/agencies/{id}/workflow-runs` list page shows `trigger_event` (e.g. `work.next.requested`) and `initiator` (often empty), but no caller-supplied label. Multiple runs in one day all look the same.

## Goal

Add an optional `name` field to `WorkflowRun`:

- **Caller may supply.** If `name` is set in `CreateWorkflowRun`, persist as-is.
- **Server generates** when omitted. Generator format: `pipeline-YYYY-MM-DD-HHMMSS-<6hex>` (date + time + 6 random hex chars). Example: `pipeline-2026-06-02-150412-a3f1c2`.
- **Unique per agency.** `CreateWorkflowRun` returns `ALREADY_EXISTS` if a row with the same `(agency_id, name)` exists. Caller appends a discriminator and retries.
- **Filterable.** `GET /work/{agencyId}/workflow-runs?name=foo` returns exact match (single row when uniqueness is enforced).

## Non-goals

- Renaming after creation. The name is immutable — like an idempotency key.
- Search-by-substring. v1 is exact-match only; the UI can do client-side filter.

---

## Design

### Schema change

In [`schema.go`](../../schema.go), under the `WorkflowRun` `TypeDefinition` (lines 378–432), add a new `PropertyDefinition`:

```go
// name is a caller-supplied or server-generated human-readable label,
// unique per agency. Used as a correlation handle by test scripts and
// surfaces as the headline column in the WorkflowRuns UI list.
{Name: "name", Type: types.PropertyTypeString},
```

### Model field

In [`models.go`](../../models.go), `WorkflowRun` struct gains `Name string \`json:"name"\``.

### Manager

In [`workflow_run.go`](../../workflow_run.go), `CreateWorkflowRun`:

```go
func (m *taskManager) CreateWorkflowRun(ctx context.Context, agencyID, name, triggerEvent, initiator string) (WorkflowRun, error) {
    if name == "" {
        name = m.generateRunName(time.Now())
    }
    if existing, err := m.findRunByName(ctx, agencyID, name); err == nil && existing.ID != "" {
        return WorkflowRun{}, fmt.Errorf("CreateWorkflowRun: %w", ErrWorkflowRunNameExists)
    }
    // ...rest unchanged
}

func (m *taskManager) generateRunName(now time.Time) string {
    suffix := randHex(6) // 6 hex chars
    return fmt.Sprintf("pipeline-%s-%s", now.UTC().Format("2006-01-02-150405"), suffix)
}
```

### Errors

Add `ErrWorkflowRunNameExists` sentinel to [`errors.go`](../../errors.go). Maps to gRPC `ALREADY_EXISTS` and HTTP `409`.

### Proto

In `proto/codevaldwork/v1/`, `CreateWorkflowRunRequest` gains `string name = 4;`. `WorkflowRun` message gains `string name = N;`. Regenerate via `make proto`.

### HTTP

`POST /work/{agencyId}/workflow-runs` accepts `name` in body. `GET /work/{agencyId}/workflow-runs?name=foo` filters. List response continues to sort by `created_at DESC` (latest first); when `name` filter is set, returns at most one row.

### UI impact

The `/agencies/{id}/workflow-runs` list page ([agencies.$agencyId.workflow-runs._index.tsx](../../../../CodeValdWorkFrontend/app/routes/agencies.$agencyId.workflow-runs._index.tsx)) adds a `Name` column **before** Status. The existing `Started`, `Status`, `Trigger event`, `Initiator` columns stay. The link cell switches to wrap the name (so the name is the click target).

---

## Implementation plan

### Phase 1 — Backend (0.5 day)

1. Add `name` property to `WorkflowRun` in [`schema.go`](../../schema.go).
2. Add `Name string` to `WorkflowRun` in [`models.go`](../../models.go); update `workflowRunToProperties` / `workflowRunFromEntity`.
3. Add name-generator helper in [`workflow_run.go`](../../workflow_run.go).
4. Add uniqueness check in `CreateWorkflowRun`; add `ErrWorkflowRunNameExists` sentinel.
5. Add `name` to the `CreateWorkflowRunRequest` and `WorkflowRun` proto messages; `make proto`.
6. Update HTTP handler to accept `name` filter on list.
7. Unit + integration tests (rerun [workflow_run_test.go](../../workflow_run_test.go) with new fixtures).

### Phase 2 — Frontend (~1 hour, lives in WorkFrontend FEAT)

Owned by [WorkFrontend FEAT-20260602-001](../../../../CodeValdWorkFrontend/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_detail_progressive_render.md). Adds the `Name` column to the list view.

---

## Verification

- `go test -race -count=1 ./...` clean.
- `curl -X POST .../workflow-runs -d '{"trigger_event":"x","initiator":"y"}'` returns a row with a non-empty `name` matching the generator format.
- `curl -X POST .../workflow-runs -d '{"name":"my-run","...":"..."}'` returns the row with `name: my-run`.
- A second POST with the same `name` returns 409.
- `GET .../workflow-runs?name=my-run` returns exactly one row.
- `/agencies/.../workflow-runs` UI renders the new column.

---

## Open design questions

1. **Generator length.** 6 hex chars = 16M possibilities; collision probability after 1k runs/day is negligible. Recommend 6. If 4 collides more than expected in practice, expand.
2. **Trailing whitespace / case.** Reject leading/trailing whitespace; preserve case as-is. Don't normalize to lowercase.

---

## Dependencies

- Builds on: [FEAT-20260601-001 (WorkflowRun entity)](FEAT-20260601-001_workflow_run_rollup.md).
- Required by: [start-pipeline (CodeValdFunctions FEAT-20260602-001)](../../../../CodeValdFunctions/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_start_pipeline_function.md) — passes `name` through from inbound event.
- Pairs with: [WorkFrontend FEAT-20260602-001 (Name column on list)](../../../../CodeValdWorkFrontend/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_detail_progressive_render.md).
