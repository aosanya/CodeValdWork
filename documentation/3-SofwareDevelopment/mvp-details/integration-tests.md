# Phase 2 Integration Tests

Topics: `fakeDataManager` graph extensions · ArangoDB end-to-end · Event
verification

---

## MVP-WORK-016 — Unit & integration tests

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-016_phase2_tests`
**Depends on**: MVP-WORK-008 through MVP-WORK-015

### Goal

Validate the entire Phase 2 surface end-to-end. The bar is the same as
Phase 1: `go test -race ./...` passes, plus `WORK_ARANGO_ENDPOINT`-gated
integration tests pass against a real ArangoDB.

### `fakeDataManager` extensions

Phase 1 `task_test.go` defines an in-memory `fakeDataManager`. Phase 2 needs:

| Capability | Required by |
|---|---|
| Edge storage (label-keyed map: `(label, fromID, toID) → Relationship`) | WORK-009, WORK-010, WORK-011, WORK-012, WORK-014 |
| Single-hop traversal in both directions | WORK-009, WORK-011, WORK-012 |
| Label whitelist enforcement (mirrors real backend) | WORK-009 |
| Cross-agency edge rejection | WORK-009 |

`fakeDataManager` does **not** need multi-hop traversal — the production code
also doesn't use it (single-hop only per the relationships spec).

### Files to create / modify

| File | Change |
|---|---|
| `task_test.go` | Extend `fakeDataManager`; add Phase 2 unit suites alongside the existing Phase 1 ones (one suite per task) |
| `relationship_test.go` (new, optional) | If `task_test.go` exceeds 500 lines, split off the relationship/agent/group suites here |
| `storage/arangodb/arangodb_test.go` | End-to-end Phase 2 scenarios (see below); skipped when `WORK_ARANGO_ENDPOINT` is unset |
| `internal/server/server_test.go` (new if absent) | gRPC-level acceptance tests covering the new RPCs and error mapping |

> File-size watch: `task_test.go` is currently small but will grow under
> Phase 2. If it crosses 500 lines, split as noted; do **not** wait for the
> linter to flag it.

### End-to-end scenarios (ArangoDB-backed)

Each scenario starts from a clean database, registers a fresh agency, and
runs through:

1. **Schema seed** — confirm `Task`, `TaskGroup`, `Agent` collections plus
   `work_relationships` edge collection exist; confirm `work_graph` named
   graph is registered and includes the edge definition.
2. **Subtask hierarchy** — create parent + 2 children with `subtask_of`
   edges. Verify `TraverseRelationships(parent, "subtask_of", Inbound)`
   returns both children.
3. **Blocker gate** — A `blocks` B; A is `pending`; attempt B `pending →
   in_progress` and assert `ErrBlocked` with `BlockedByInfo.BlockerTaskIDs =
   [A.ID]`. Then complete A, retry, assert success.
4. **Assignment via edge** — `UpsertAgent(a1)`, `UpsertAgent(a2)`,
   `AssignTask(t, a1)`, `AssignTask(t, a2)`. After both, `t` has exactly one
   outbound `assigned_to` edge, pointing to `a2`.
5. **TaskGroup membership** — create group, add 3 tasks, list members,
   remove one, list members. `DeleteTaskGroup` removes the vertex and the
   2 remaining `member_of` edges; the 3 tasks still exist.
6. **Event verification** — drive a sequence (create → update → assign →
   transition to completed → create blocks edge) past a recording publisher
   and assert the exact list of `Event` objects emitted, in order, with
   correct payloads.

### gRPC-level acceptance tests (`internal/server`)

For each new RPC: one happy-path test plus one domain-error test that
exercises the error mapping. Use the in-memory `TaskManager` (constructed
with `fakeDataManager`) — these tests do not need a live ArangoDB.

The `BlockedByInfo` round-trip is the highest-value end-to-end check:
construct a real `*status.Status`, call `WithDetails`, send across an
in-process `grpc.ClientConn`, decode the detail on the client side.

### Acceptance criteria

- `go build ./...` succeeds.
- `go vet ./...` reports zero issues.
- `go test -race ./...` passes with `WORK_ARANGO_ENDPOINT` unset (unit tier).
- `go test -race ./...` passes with `WORK_ARANGO_ENDPOINT` set (integration
  tier).
- `golangci-lint run ./...` passes.
- Coverage on the new `task.go` / `relationship.go` / `agent.go` /
  `taskgroup.go` exceeds 80 % (no enforcement gate, but report it in the
  PR).

### Out of scope

- End-to-end through CodeValdCross — that's covered by a Cross-side
  integration suite (analogous to CROSS-IT-001..004 for the Git pipeline);
  if it doesn't exist for the Work pipeline yet, file a follow-up CROSS
  task. WORK-016 stops at the gRPC boundary.
- Performance / load tests.
- Migration tests — there is no Phase 1 → Phase 2 data migration (greenfield).
