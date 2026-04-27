# Schema Extension — TaskGroup, Agent, Richer Task Properties

Topics: Pre-delivered schema · `TypeDefinition` · `PropertyDefinition`

---

## MVP-WORK-008 — Schema extension

**Status**: 🟒 Not Started
**Branch**: `feature/WORK-008_schema_extension`
**Depends on**: —

### Goal

Bring `DefaultWorkSchema` up to the design described in
[architecture-domain.md §2-3](../../2-SoftwareDesignAndArchitecture/architecture-domain.md):

1. Extend the `Task` TypeDefinition with `dueAt` (datetime), `tags` (array),
   `estimatedHours` (number), `context` (string), `completedAt` (datetime).
2. Re-type `status` and `priority` as `option` properties whose allowed values
   are the existing `TaskStatus` and `TaskPriority` constants.
3. Add a `TaskGroup` TypeDefinition (`work_groups` collection).
4. Add an `Agent` TypeDefinition (`work_agents` collection).
5. Drop `assigned_to` from the `Task` properties — the Phase 2 design moves it
   to a graph edge ([MVP-WORK-010](agent-assignment.md)). Removing the property
   here keeps the schema honest; the field is reintroduced as an edge in
   WORK-009 / WORK-010.

The schema ID stays `work-schema-v1` for the duration of Phase 2 — entries are
additive plus the `assigned_to` removal, and there is no deployed data to
migrate.

### Files to create / modify

| File | Change |
|---|---|
| `schema.go` | Extend `DefaultWorkSchema` with the three TypeDefinitions and richer Task properties |
| `types.go` | Add `TaskGroup` and `Agent` Go structs (peers of `Task`); add `taskGroupToProperties` / `agentToProperties` / matching `*FromEntity` helpers if not generated |
| `task.go` | Update `taskToProperties` / `taskFromEntity` to read/write the new fields; remove `AssignedTo` from the property write path |
| `task_test.go` | Cover round-tripping each new property type through the existing `fakeDataManager` |

### Property-type mapping (against SharedLib `types.PropertyType`)

| Field | Type constant | Notes |
|---|---|---|
| `title` | `PropertyTypeString` | Required (unchanged) |
| `description` | `PropertyTypeString` | |
| `status` | `PropertyTypeOption` | Allowed: `pending`, `in_progress`, `completed`, `failed`, `cancelled` — declare via `Options` |
| `priority` | `PropertyTypeOption` | Allowed: `low`, `medium`, `high`, `critical` |
| `dueAt` | `PropertyTypeDateTime` | RFC 3339 |
| `tags` | `PropertyTypeArray` (element `string`) | |
| `estimatedHours` | `PropertyTypeNumber` | |
| `context` | `PropertyTypeString` | AI agent working memory blob |
| `completedAt` | `PropertyTypeDateTime` | Set by `TaskManager` on terminal status |
| `created_at`, `updated_at` | `PropertyTypeDateTime` | Re-typed from string |

> **SharedLib check**: confirm `PropertyTypeOption`, `PropertyTypeArray`,
> `PropertyTypeNumber`, `PropertyTypeDateTime` exist in
> `github.com/aosanya/CodeValdSharedLib/types`. If `Options` carrier is missing
> on `PropertyDefinition`, surface as a SharedLib extension before continuing.

### TaskGroup TypeDefinition

```
{ Name: "TaskGroup", DisplayName: "Task Group",
  PathSegment: "task-groups", EntityIDParam: "taskGroupId",
  StorageCollection: "work_groups",
  Properties: [
    { name, type=string,    required=true },
    { description, type=string },
    { dueAt, type=datetime },
  ],
}
```

### Agent TypeDefinition

```
{ Name: "Agent", DisplayName: "Agent",
  PathSegment: "agents", EntityIDParam: "agentId",
  StorageCollection: "work_agents",
  Properties: [
    { agentID, type=string, required=true }, // external agent identifier
    { displayName, type=string },
    { capability, type=string },             // "code" | "research" | "review" | ...
  ],
}
```

`agentID` is the natural key that callers (e.g. CodeValdAI) use to identify the
agent. The graph entity ID is the storage `_key` and is opaque to callers.
WORK-010 enforces the `(agencyID, agentID)` uniqueness constraint at the
`UpsertAgent` API boundary.

### Acceptance tests

- `DefaultWorkSchema().Types` length is 3 (`Task`, `TaskGroup`, `Agent`).
- Each property reports the correct `PropertyType`.
- `status` / `priority` `Options` lists match the `TaskStatus` /
  `TaskPriority` constants.
- `taskToProperties` round-trips a Task that sets `dueAt`, `tags`,
  `estimatedHours`, `context`, `completedAt` — the resulting `Task` value
  equals the input modulo `UpdatedAt`.
- `taskToProperties` no longer writes an `assigned_to` key (regression guard
  for Phase 1 behaviour).
- Unit tests pass with `go test -race ./...`.

### Out of scope

- Edge collection registration → [MVP-WORK-009](relationships.md)
- gRPC proto messages for the new fields → [MVP-WORK-013](grpc-surface.md)
- Per-agency schema seeding on `cross.agency.created` — explicitly deferred.
  Schema is still seeded once on startup via `entitygraph.SeedSchema`.
