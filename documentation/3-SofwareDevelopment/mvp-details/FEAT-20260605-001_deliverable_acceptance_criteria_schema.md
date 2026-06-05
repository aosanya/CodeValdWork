# FEAT-20260605-001 — Deliverable + AcceptanceCriteria entity types (schema v4)

**Status:** 📋 Not Started
**Severity:** High — foundational schema change; FEAT-20260605-002 and FEAT-20260605-003 depend on this landing first
**Owner:** CodeValdWork
**Estimated effort:** ~1 day (schema + relationship constants + models + tests)
**Source finding:** Research session 2026-06-05 — task decomposition improvement discussion

---

## Problem

Tasks and TaskTodos have no first-class way to declare **what they must produce** (deliverables) or **how correctness is verified** (acceptance criteria). Without these, the AI agent has no structured contract to work toward and no machine-readable signal that the work is actually done to spec.

## Goal

Add two new entity types to `DefaultWorkSchema` and bump the schema version to v4:

- **`Deliverable`** — a specification of what a Task or TaskTodo must produce. Pure definition; no runtime state.
- **`AcceptanceCriteria`** — a verifiable condition that must be satisfied before the owning Task/TaskTodo is considered done. Carries a runtime `result` written by a reviewer (FEAT-20260605-003).

## Non-goals

- Gate logic in CodeValdWork — the gate lives in the WorkPlan (FEAT-20260605-002). Work is passive.
- A `criteria_type` field on `AcceptanceCriteria` — deferred; add when usage pattern is clearer.
- Reviewer logic — that is FEAT-20260605-003.

---

## Design

### New entity: `Deliverable`

Storage collection: `work_deliverables`

| Field | Type | Notes |
|---|---|---|
| `title` | string | Short label, e.g. "Passing unit tests for TaskTodo creation" |
| `description` | string | Fuller spec of what must be produced |
| `deliverable_type` | string | e.g. `"code"`, `"document"`, `"artifact"`, `"test_output"` |
| `parent_id` | string | Denormalised owner ID (Task or TaskTodo). Mirrors `parent_task_id` on TaskTodo |
| `ordinality` | integer | 1-based ordering within the owning entity's deliverables |
| `workflow_run_id` | string | Inherited from parent at creation time |
| `created_at` | string | RFC 3339 |
| `updated_at` | string | RFC 3339 |

No `status` field — lifecycle state lives on the parent Task/TaskTodo, not the deliverable.

### New entity: `AcceptanceCriteria`

Storage collection: `work_acceptance_criteria`

| Field | Type | Notes |
|---|---|---|
| `title` | string | Short label, e.g. "All unit tests pass with race detector" |
| `description` | string | The full verifiable condition |
| `parent_id` | string | Denormalised owner ID (Task or TaskTodo) |
| `ordinality` | integer | 1-based ordering within the owning entity's criteria |
| `workflow_run_id` | string | Inherited from parent at creation time |
| `result` | string | Runtime outcome: `"passed"`, `"failed"`, `"skipped"`, `"blocked"`. Empty until reviewer runs |
| `result_notes` | string | Free-form explanation written by the reviewer alongside `result` |
| `created_at` | string | RFC 3339 |
| `updated_at` | string | RFC 3339 |

### New relationship label constants (`relationship.go`)

```go
// RelLabelHasDeliverable links a Task or TaskTodo to a Deliverable it must produce.
RelLabelHasDeliverable = "has_deliverable"

// RelLabelHasAcceptanceCriteria links a Task or TaskTodo to a verifiable AcceptanceCriteria.
RelLabelHasAcceptanceCriteria = "has_acceptance_criteria"
```

Inverses (on the entity side): `deliverable_of`, `criteria_of`.

### Graph topology additions

```
Task    ──has_deliverable──────────► Deliverable
Task    ──has_acceptance_criteria──► AcceptanceCriteria
TaskTodo──has_deliverable──────────► Deliverable
TaskTodo──has_acceptance_criteria──► AcceptanceCriteria
```

All edges stored in `work_relationships` (existing edge collection).

### Schema version bump

```go
ID:      "work-schema-v1",
Version: 4,
Tag:     "v4",
```

---

## Files to create / modify

| File | Change |
|---|---|
| `schema.go` | Add `Deliverable` and `AcceptanceCriteria` TypeDefinitions; add `has_deliverable` and `has_acceptance_criteria` relationships to `Task` and `TaskTodo`; bump `Version` to 4, `Tag` to `"v4"` |
| `relationship.go` | Add `RelLabelHasDeliverable` and `RelLabelHasAcceptanceCriteria` constants |
| `models.go` | Add `Deliverable` and `AcceptanceCriteria` Go value types |

---

## Acceptance tests

- `DefaultWorkSchema().Version` equals 4.
- `DefaultWorkSchema().Types` includes `Deliverable` (collection `work_deliverables`) and `AcceptanceCriteria` (collection `work_acceptance_criteria`).
- `Task` TypeDefinition contains both `has_deliverable` and `has_acceptance_criteria` relationships.
- `TaskTodo` TypeDefinition contains both `has_deliverable` and `has_acceptance_criteria` relationships.
- `Deliverable` TypeDefinition has all eight declared property names.
- `AcceptanceCriteria` TypeDefinition has all nine declared property names including `result` and `result_notes`.
- `go build ./...` succeeds.
- `go test -race ./...` passes.

---

## Dependencies

- No upstream blockers — this is the base schema change.
- FEAT-20260605-002 (WorkPlan review step) depends on this.
- FEAT-20260605-003 (Reviewer component) depends on this.
