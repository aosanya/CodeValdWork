# Graph Relationships & Blocker Enforcement

Topics: `work_relationships` edge collection · Edge-label whitelist ·
`CreateRelationship` / `DeleteRelationship` / `TraverseRelationships` ·
Hard blocker gate

---

## MVP-WORK-009 — Graph relationship API

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-009_graph_relationships`
**Depends on**: MVP-WORK-008

### Goal

Stand up the `work_relationships` edge collection plus the relationship API
on `TaskManager`, so that subsequent tasks (`AssignTask`, blocker enforcement,
TaskGroup membership) have a single, validated path to mutate edges.

The named graph `work_graph` spans all Work vertex collections (`work_tasks`,
`work_groups`, `work_agents`) and uses `work_relationships` as its edge
definition.

### Edge-label whitelist

| Label | From type | To type | Edge properties |
|---|---|---|---|
| `assigned_to` | `Task` | `Agent` | `assignedAt`, `assignedBy` |
| `blocks` | `Task` | `Task` | `createdAt`, `reason` |
| `subtask_of` | `Task` | `Task` | `createdAt` |
| `depends_on` | `Task` | `Task` | `createdAt`, `reason` |
| `member_of` | `Task` | `TaskGroup` | `addedAt` |

Any `(label, fromType, toType)` triple outside this table is rejected with
`ErrInvalidRelationship` (`INVALID_ARGUMENT` at the gRPC layer).

### Files to create / modify

| File | Change |
|---|---|
| `relationship.go` (new) | `Relationship` struct, label constants, `validateRelationshipLabel` against the whitelist |
| `task.go` | Extend `TaskManager` with `CreateRelationship`, `DeleteRelationship`, `TraverseRelationships` |
| `errors.go` | Add `ErrInvalidRelationship`, `ErrRelationshipNotFound` |
| `storage/arangodb/arangodb.go` | Register `work_relationships` edge collection; ensure the `work_graph` named graph contains the edge definition |
| `task_test.go` | Round-trip relationships through `fakeDataManager` |
| `storage/arangodb/arangodb_test.go` | Edge collection persistence + traversal tests |

### TaskManager additions

```go
type TaskManager interface {
    // ... Phase 1 methods ...

    CreateRelationship(ctx context.Context, agencyID string, rel Relationship) (Relationship, error)
    DeleteRelationship(ctx context.Context, agencyID, fromID, toID, label string) error
    TraverseRelationships(ctx context.Context, agencyID, vertexID, label string, dir Direction) ([]Relationship, error)
}

type Direction int
const (
    DirectionInbound  Direction = iota // edges pointing AT vertexID
    DirectionOutbound                  // edges pointing FROM vertexID
)
```

`TraverseRelationships` is a **single-hop** read — multi-hop traversal stays in
the graph layer (use `entitygraph.DataManager.TraverseGraph` directly when
needed). MVP-WORK-009 does not expose a multi-hop API.

### Behaviour

- `CreateRelationship` — validates label, looks up both endpoints (must exist
  in agency, types must match the whitelist), creates the edge with
  `createdAt = time.Now().UTC()` and any caller-supplied edge properties.
- `DeleteRelationship` — removes a single edge; returns
  `ErrRelationshipNotFound` if absent.
- Re-creating an existing `(from, to, label)` edge is **idempotent** — returns
  the existing edge, does not error. (Phase 1 `assigned_to` is the only place
  this matters; WORK-010 relies on it for "replace" semantics.)
- All methods publish `work.relationship.created` / `.deleted` via the
  `CrossPublisher` once [MVP-WORK-014](pubsub.md) lands. WORK-009 leaves a
  stub that no-ops if `publisher == nil`.

### Acceptance tests

- Create each whitelisted label between valid endpoints — succeeds.
- Create with a non-whitelisted `(label, fromType, toType)` triple — returns
  `ErrInvalidRelationship`.
- Create with a missing endpoint — returns `ErrTaskNotFound` (Task) /
  `ErrAgentNotFound` (Agent) / `ErrTaskGroupNotFound` (TaskGroup).
- Cross-agency edge create (from agency A vertex, to agency B vertex) — returns
  `ErrInvalidRelationship`.
- Re-create an existing edge — no error, returned `Relationship` equals the
  original.
- Traverse `blocks` outbound from a task with two `blocks` edges — returns
  both targets; order unspecified.
- ArangoDB integration: edges land in `work_relationships`; `work_graph`
  named graph traversal returns the same set as `TraverseRelationships`.

### Out of scope

- `assigned_to` upsert/replace semantics → [MVP-WORK-010](agent-assignment.md)
- `member_of` target-type enforcement → [MVP-WORK-012](task-group.md)
- Hard blocker gate on `pending → in_progress` → MVP-WORK-011 (below)

---

## MVP-WORK-011 — Hard blocker enforcement

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-011_blocker_enforcement`
**Depends on**: MVP-WORK-009

### Goal

Enforce the blocker gate described in
[architecture-domain.md §5](../../2-SoftwareDesignAndArchitecture/architecture-domain.md):

> A Task in `pending` state cannot transition to `in_progress` if any
> `blocks`-inbound Task (i.e. a task that blocks this one) is still
> non-terminal.

### Files to create / modify

| File | Change |
|---|---|
| `errors.go` | Add `ErrBlocked` |
| `task.go` | In `UpdateTask`, when transitioning `pending → in_progress`, traverse `blocks` inbound and verify all sources are terminal |
| `proto/codevaldwork/v1/errors.proto` | New `BlockedByInfo` detail with `repeated string blocker_task_ids` |
| `internal/server/errors.go` | Map `ErrBlocked` → `FAILED_PRECONDITION` carrying a `BlockedByInfo` detail |
| `task_test.go` | Cover the gate with `fakeDataManager` |

### Algorithm

```
when UpdateTask transitions current.Status=pending → next=in_progress:
    blockers := TraverseRelationships(ctx, agencyID, taskID, "blocks", Inbound)
    nonTerminal := []string{}
    for each edge in blockers:
        sourceTask := GetTask(ctx, agencyID, edge.FromID)
        if !isTerminalStatus(sourceTask.Status):
            nonTerminal = append(nonTerminal, sourceTask.ID)
    if len(nonTerminal) > 0:
        return ErrBlocked with detail.BlockerTaskIDs = nonTerminal
```

`isTerminalStatus` already exists in `types.go` (returns true for `completed`,
`failed`, `cancelled`).

### Behaviour notes

- Only `pending → in_progress` is gated. `pending → cancelled` is allowed
  unconditionally (cancelling a blocked task should not require the blockers
  to clear).
- `in_progress → completed/failed/cancelled` is unaffected — once a task is
  running, blockers no longer apply.
- `depends_on` is **not** gated — it is a soft dependency by spec.

### Acceptance tests

- Task A `blocks` Task B; A is `pending`; B `pending → in_progress` returns
  `ErrBlocked` with `BlockedByInfo.BlockerTaskIDs = [A.ID]`.
- Same setup, A `completed`; B `pending → in_progress` succeeds.
- Same setup, A `cancelled`; B `pending → in_progress` succeeds (terminal).
- Two blockers, one terminal one not → still returns `ErrBlocked` with only
  the non-terminal ID.
- B `pending → cancelled` succeeds even when A is `pending`.
- gRPC error round-trips: `FAILED_PRECONDITION` with `BlockedByInfo` decodable
  on the client.
