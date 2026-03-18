# CodeValdWork — Architecture: Domain Model

> Part of [architecture.md](architecture.md)

## 1. Entity-Graph Foundation

CodeValdWork adopts the same entity-graph foundation as CodeValdDT and
CodeValdComm. The `TaskManager` interface is a thin orchestration layer
built on top of `entitygraph.DataManager` and `entitygraph.SchemaManager`
from SharedLib.

```go
// WorkDataManager is the entity-graph data interface for CodeValdWork.
// It is a type alias — no additional methods.
type WorkDataManager = entitygraph.DataManager

// WorkSchemaManager is the schema interface for CodeValdWork.
// It is a type alias — no additional methods.
type WorkSchemaManager = entitygraph.SchemaManager
```

`TaskManager` remains the public API of the service. Internally, its
`taskManager` implementation holds a `WorkDataManager` and a
`WorkSchemaManager` rather than a raw `Backend`.

---

## 2. Pre-Delivered Schema

CodeValdWork ships with a **fixed, built-in schema** — agencies do not author
TypeDefinitions. On receipt of `cross.agency.created`, CodeValdWork calls
`WorkSchemaManager.SetSchema(agencyID, defaultWorkSchema)` if no schema
exists for that agency. This is idempotent.

The schema is a package-level constant defined in `schema.go`.

---

## 3. Entity TypeDefinitions

### Task

```
TypeDefinition{
    Name:              "Task",
    DisplayName:       "Task",
    StorageCollection: "work_tasks",
    Immutable:         false,
    Properties: [
        { Name: "title",            Type: string,   Required: true  },
        { Name: "description",      Type: string,   Required: false },
        { Name: "status",           Type: option,   Required: false }, // "pending" | "in_progress" | "completed" | "failed" | "cancelled"
        { Name: "priority",         Type: option,   Required: false }, // "low" | "medium" | "high" | "critical"
        { Name: "dueAt",            Type: datetime, Required: false },
        { Name: "tags",             Type: array,    Required: false }, // []string
        { Name: "estimatedHours",   Type: number,   Required: false },
        { Name: "context",          Type: string,   Required: false }, // AI agent working memory blob
        { Name: "completedAt",      Type: datetime, Required: false }, // set on terminal status
    ],
}
```

`status` and `priority` are kept as properties on the Task entity rather than
separate TypeDefinitions. Status transitions are enforced by `TaskManager`
before delegating to `WorkDataManager.UpdateEntity`.

---

### TaskGroup

A `TaskGroup` is an optional container that groups related tasks (e.g. a
sprint, a project milestone, or an epic). Tasks are linked to a group via the
`member_of` edge.

```
TypeDefinition{
    Name:              "TaskGroup",
    DisplayName:       "Task Group",
    StorageCollection: "work_groups",
    Immutable:         false,
    Properties: [
        { Name: "name",        Type: string,   Required: true  },
        { Name: "description", Type: string,   Required: false },
        { Name: "dueAt",       Type: datetime, Required: false },
    ],
}
```

---

### Agent

An `Agent` entity is the Work-domain projection of an AI agent. It becomes
a graph vertex so that `assigned_to` edges can be first-class graph
relationships rather than plain string fields on the Task document.

```
TypeDefinition{
    Name:              "Agent",
    DisplayName:       "Agent",
    StorageCollection: "work_agents",
    Immutable:         false,
    Properties: [
        { Name: "agentID",     Type: string, Required: true  }, // external agent identifier
        { Name: "displayName", Type: string, Required: false },
        { Name: "capability",  Type: string, Required: false }, // e.g. "code", "research", "review"
    ],
}
```

At most one Agent per `(agencyID, agentID)` pair.

---

## 4. Graph Relationship Model

All edges are stored in the `work_relationships` **edge collection**.
The named graph `work_graph` spans all Work vertex collections.

| Relationship | From | To | Edge Properties | Semantics |
|---|---|---|---|---|
| `assigned_to` | Task | Agent | `assignedAt`, `assignedBy` | Task is owned by an Agent |
| `blocks` | Task | Task | `createdAt`, `reason` | Source task must reach terminal status before target may start |
| `subtask_of` | Task | Task | `createdAt` | Source is a child task of the target |
| `depends_on` | Task | Task | `createdAt`, `reason` | Source requires output from target (soft dependency; no enforcement) |
| `member_of` | Task | TaskGroup | `addedAt` | Task belongs to a TaskGroup |

### Dependency Semantics

| Edge | Enforcement | Blocking? |
|---|---|---|
| `blocks` | Hard — `TaskManager.CreateTask` / `UpdateTask` checks if blocker is terminal before allowing `in_progress` | Yes |
| `depends_on` | Soft — informational only; no status gate | No |
| `subtask_of` | Structural — parent task shows aggregate child progress | No |
| `assigned_to` | Replaces `AssignedTo string` field — `TaskManager.AssignTask` creates/replaces this edge | — |

### Graph Traversal Patterns

```
Task assignment:
  Task --[assigned_to]--> Agent

Blockers for a task:
  Task <--[blocks]-- Task  (inbound: which tasks are blocked by me?)
  Task --[blocks]--> Task  (outbound: which tasks am I blocking?)

Subtask hierarchy (depth=n):
  ParentTask <--[subtask_of]-- ChildTask  (inbound)

Task group members:
  TaskGroup <--[member_of]-- Task  (inbound)

Dependency chain:
  Task --[depends_on]--> Task  (outbound: what do I depend on?)
```

---

## 5. Status State Machine

The status lifecycle is enforced by `TaskManager.UpdateTask` before the
entity-graph write. This logic lives in the orchestration layer — not in the
storage backend.

```
                ┌──────────────────────────┐
                ▼                          │
           [pending] ──────────────→ [cancelled]
                │
                ▼
         [in_progress] ──────────→ [cancelled]
           │         │
           ▼         ▼
      [completed]  [failed]
```

Terminal states: `completed`, `failed`, `cancelled` — no further transitions.

**Blocker gate**: a Task in `pending` state cannot transition to `in_progress`
if any `blocks`-inbound Task (i.e. a task that blocks this one) is still
non-terminal. `TaskManager.UpdateTask` traverses the `blocks` edges before
allowing this transition.

---

## 6. Pub/Sub Events

| Topic | Published when | Payload |
|---|---|---|
| `work.task.created` | Task entity created | `{taskID, agencyID, title, priority}` |
| `work.task.updated` | Any mutable field changed (except status) | `{taskID, agencyID, changedFields: []}` |
| `work.task.status.changed` | Status transitions (any) | `{taskID, agencyID, from, to}` |
| `work.task.completed` | Status → `completed`, `failed`, or `cancelled` | `{taskID, agencyID, terminalStatus, completedAt}` |
| `work.task.assigned` | `assigned_to` edge created or replaced | `{taskID, agencyID, agentID}` |
| `work.relationship.created` | Any relationship edge created | `{fromID, toID, label, agencyID}` |
