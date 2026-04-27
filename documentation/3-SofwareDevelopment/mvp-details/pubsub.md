# Pub/Sub Publishing Pipeline

Topics: `CrossPublisher` payloads · Six-topic publish hooks ·
Registrar `produces` · SharedLib extraction candidate

---

## MVP-WORK-014 — Pub/sub publishing pipeline

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-014_pubsub_pipeline`
**Depends on**: MVP-WORK-008…012

### Goal

Replace the Phase 1 `CrossPublisher.Publish(ctx, topic, agencyID)` stub with
a typed, payload-bearing publish API and wire publish hooks for **all six**
topics named in
[architecture-domain.md §6](../../2-SoftwareDesignAndArchitecture/architecture-domain.md):

| Topic | Fired when | Payload |
|---|---|---|
| `work.task.created` | Task entity created | `{taskID, agencyID, title, priority}` |
| `work.task.updated` | Mutable field changed (excluding status) | `{taskID, agencyID, changedFields[]}` |
| `work.task.status.changed` | Any status transition | `{taskID, agencyID, from, to}` |
| `work.task.completed` | Status → `completed`, `failed`, or `cancelled` | `{taskID, agencyID, terminalStatus, completedAt}` |
| `work.task.assigned` | `assigned_to` edge created or replaced | `{taskID, agencyID, agentID}` |
| `work.relationship.created` | Any edge created | `{fromID, toID, label, agencyID}` |

### Phase 1 starting point

| Already in place | Gap |
|---|---|
| `CrossPublisher` interface ([`task.go:59`](../../../task.go#L59)) | Carries only `(topic, agencyID)` — no payload |
| `Registrar.Publish` log-only stub ([`registrar.go:85`](../../../internal/registrar/registrar.go#L85)) | TODO points at a non-existent `OrchestratorService.Publish` RPC (CROSS-007 reference is stale) |
| Publishes on `work.task.created` / `.updated` / `.completed` ([`task.go:115,167,169`](../../../task.go#L115)) | Misses `.status.changed`, `.assigned`, `.relationship.created` |
| `produces` declared in registrar: 3 topics | Missing 3 topics |

### 🚩 SharedLib extraction candidate — flagged before starting

A typed event-bus client is **not** CodeValdWork-specific. CodeValdAgency,
CodeValdComm, CodeValdDT all need to publish lifecycle events on the same
mechanism. Per the SharedLib extraction rule in `start-prioritized.prompt.md`:

> Whenever you encounter infrastructure code that is — or could soon be —
> used by more than one service, **stop and flag it explicitly** … Never
> silently copy code across services.

**Action before WORK-014 implementation begins**: open a SharedLib design
note proposing one of the following, and reach alignment with the user:

1. **`SharedLib/eventbus`** package — defines a `Publisher` interface, an
   `Event` struct (topic + agencyID + typed payload via `proto.Message` or
   `json.RawMessage`), and a Cross-RPC implementation. Each service imports
   it and supplies its own typed payload helpers.
2. **Status quo + per-service adapter** — keep `CrossPublisher` per-service,
   accept that it will be near-duplicated across CodeValdWork / Comm / DT.
   Choose this only if the user explicitly prefers it.

The `mvp.md` task row stays as MVP-WORK-014 either way; if option 1 is taken,
WORK-014 absorbs a SharedLib dependency and gets one new line in its scope:
"add `github.com/aosanya/CodeValdSharedLib/eventbus` to `go.mod`".

### Cross-side dependency

`CrossPublisher` ultimately wants to call a `OrchestratorService.Publish` RPC
on CodeValdCross — which **does not exist today**. This is a cross-repo
dependency.

**Recommended sequencing**:

1. Land WORK-014 with a logging-only implementation **plus** the typed
   payload contract (so callers commit to the payload shape now).
2. Open a CROSS-XXX task to add the gRPC `Publish` RPC + the matching client
   wiring on CodeValdCross's side.
3. Once CROSS-XXX lands, replace the log-only body in `Registrar.Publish`
   with a real RPC call. No CodeValdWork API changes needed at that point.

This split keeps WORK-014 unblocked. The Cross dependency is recorded in
the task's "Depends On" cell as a **soft** dependency (✅ won't block PR
merge, but the publish pipeline isn't observable end-to-end until the Cross
RPC ships).

### Files to create / modify

| File | Change |
|---|---|
| `task.go` | Replace `CrossPublisher.Publish(ctx, topic, agencyID)` with `Publish(ctx, Event)`; update all six call sites |
| `events.go` (new) | `Event` struct, six typed payload structs (`TaskCreatedPayload`, …), helpers to construct each |
| `task.go` (`taskManager`) | Wire publish on `UpdateTask` (status.changed when `from != to`), `AssignTask` (assigned), `CreateRelationship` (relationship.created); keep existing publishes for created / updated / completed |
| `internal/registrar/registrar.go` | Extend `produces` to all six topics; update `Publish` to accept the new signature; **continue to log only** until the Cross RPC exists |
| `task_test.go` | `recordingPublisher` test double; assert each transition publishes the expected `Event` exactly once |

### `Event` shape (proposed)

```go
type Event struct {
    Topic     string    // "work.task.created", etc.
    AgencyID  string
    Timestamp time.Time
    Payload   any       // one of the typed payload structs below
}

type TaskCreatedPayload  struct { TaskID, Title string; Priority TaskPriority }
type TaskUpdatedPayload  struct { TaskID string; ChangedFields []string }
type TaskStatusChangedPayload struct { TaskID string; From, To TaskStatus }
type TaskCompletedPayload struct { TaskID string; TerminalStatus TaskStatus; CompletedAt time.Time }
type TaskAssignedPayload struct { TaskID, AgentID string }
type RelationshipCreatedPayload struct { FromID, ToID, Label string }
```

If the SharedLib extraction goes ahead, these types move into
`SharedLib/eventbus` (or stay local with a `proto.Message` interop, depending
on the extracted contract).

### Acceptance tests

- Each TaskManager mutation that **should** publish does so exactly once.
- Each that **should not** publish stays silent (e.g. failed-validation paths
  must not emit).
- `UpdateTask` with no status change publishes `work.task.updated`, not
  `work.task.status.changed`.
- `UpdateTask` with status change publishes **both** `work.task.status.changed`
  and (if terminal) `work.task.completed` — order: status.changed → completed.
- `AssignTask` replacing an existing assignment publishes `work.task.assigned`
  once (not twice for delete-then-create).
- `CreateRelationship` publishes `work.relationship.created`.
- `produces` reported to Cross in `RegisterRequest` contains all six topics.

### Out of scope

- Per-agency schema seeding on `cross.agency.created` — explicitly deferred.
- Consuming events (`cross.task.requested`, `cross.agency.created`) — Phase
  1 already declares these as `consumes` but does not subscribe; consumption
  is a separate task, not in Phase 2.
- Replay / dead-letter / retry semantics — best-effort delivery is fine for
  MVP.
