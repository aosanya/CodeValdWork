# Task Decomposition — `ai.task.todo` Bridge & `TaskTodo` Entity

Topics: `ai.task.todo` consumer · `TaskTodo` entity · `work.task.todo` publisher ·
`todo_assigned_to` edge · `TodoStatus` lifecycle

---

## Overview

When a developer AI agent decomposes a task it emits an `ai.task.todo` event.
CodeValdWork consumes that event and materialises each `TodoItem` as a first-class
`TaskTodo` entity — queryable, assignable, and status-tracked in the work graph.
CodeValdWork then publishes one `work.task.todo` event per entity so CodeValdAI
agents can pick each todo up via a work plan.

CodeValdAI does **not** spawn child runs internally when it sees `ai.task.todo`.
CodeValdWork is the sole owner of the todo-to-run lifecycle.

---

## End-to-End Event Flow

```
work.task.assigned  (ParentTaskID absent — original task)
        │
        ▼ CodeValdAI: decomposition run
        │   callLLM → outputs ONLY an ```actions block
        │             with topic "ai.task.todo"
        │   dispatchActions: publish "ai.task.todo" to Cross (no internal spawn)
        │
        ▼ CodeValdWork: ai.task.todo EventReceiver
        │
        ├─ for each TodoItem in payload:
        │     CreateEntity("TaskTodo", properties…)
        │     CreateRelationship(TaskTodo ──todo_of──────────► Task)
        │     CreateRelationship(TaskTodo ──todo_assigned_to──► Agent)  [parent assignee]
        │     Publish "work.task.todo" {TodoID, AgentID, ParentTaskID,
        │                               Title, Instructions, Ordinality,
        │                               CanRunParallel, DependsOn}
        │
        ▼ CodeValdAI: work plan subscribed to "work.task.todo"
            RACIDispatcher → triggerPlanRun
            AgentRun (task_id = TodoID)
            ai.task.in_progress → CodeValdWork: TaskTodo.status → dispatched
            ai.task.completed   → CodeValdWork: TaskTodo.status → completed
            ai.task.failed      → CodeValdWork: TaskTodo.status → failed
```

---

## Graph Topology

```
Task ──has_todo──────────► TaskTodo ──todo_assigned_to──► Agent
```

Inverse edges auto-created by entitygraph on `CreateRelationship`:

```
TaskTodo ──todo_of──────────────────► Task
Agent    ──todo_assigned_tasks──────► TaskTodo
```

---

## `TaskTodo` Entity

### Schema (`schema.go`)

| Property | Type | Required | Notes |
|---|---|---|---|
| `title` | string | ✓ | Short imperative label |
| `description` | string | | What this sub-task accomplishes |
| `instructions` | string | ✓ | Fully self-contained agent prompt |
| `ordinality` | integer | ✓ | 1-based position in the decomposition |
| `can_run_parallel` | boolean | | True when no predecessor dependency |
| `depends_on` | integer[] | | Ordinality values that must complete first |
| `status` | string | | See [TodoStatus Lifecycle](#todostatus-lifecycle) |
| `parent_task_id` | string | ✓ | Work Task ID that was decomposed |
| `decomp_run_id` | string | | CodeValdAI AgentRun ID that produced this todo |
| `agent_id` | string | | CodeValdAI agent assigned to execute this todo |
| `created_at` | string | | RFC 3339 |
| `updated_at` | string | | RFC 3339 |

**Storage collection**: `work_task_todos`
**PublishEvents**: true (schema-derived CRUD events enabled)

### Relationships

| Label | Direction | Inverse | Cardinality |
|---|---|---|---|
| `todo_of` | TaskTodo → Task | `has_todo` | many-to-one |
| `todo_assigned_to` | TaskTodo → Agent | `todo_assigned_tasks` | many-to-one |

---

## Agent Assignment

Each `TaskTodo` is assigned to the parent Task's current assignee by default.

Bridge logic:
1. Traverse `Task ──assigned_to──► Agent` to get `AgentID`.
2. If no assignee on the Task, fall back to `payload.AgentID` (the CodeValdAI agent
   that ran the decomposition).
3. Upsert the `Agent` vertex (same as `UpsertAgent` in `AssignTask` flow).
4. Create `todo_assigned_to` edge from `TaskTodo → Agent`.
5. Store `agent_id` as a flat property on `TaskTodo` for payload convenience.

---

## `work.task.todo` Payload

```go
// TopicTaskTodo is published once per TaskTodo entity created from an
// ai.task.todo decomposition. CodeValdAI agents subscribe via work plans.
const TopicTaskTodo = "work.task.todo"

type TaskTodoPayload struct {
    TodoID         string
    ParentTaskID   string
    DecompRunID    string
    AgentID        string
    Title          string
    Instructions   string
    Ordinality     int
    CanRunParallel bool
    DependsOn      []int
}
```

Carries all fields needed for CodeValdAI's dispatcher to trigger a run with no
second lookup — mirrors the shape of `TaskAssignedPayload`.

---

## TodoStatus Lifecycle

```
pending → dispatched → completed
                    ↘ failed
```

| Status | Set when |
|---|---|
| `pending` | `TaskTodo` entity created |
| `dispatched` | CodeValdWork receives `ai.task.in_progress` with `TaskID == TodoID` |
| `completed` | CodeValdWork receives `ai.task.completed` with `TaskID == TodoID` |
| `failed` | CodeValdWork receives `ai.task.failed` with `TaskID == TodoID` |

---

## Relationship Constants (`relationship.go`)

```go
// RelLabelHasTodo links a Task to its decomposed TaskTodo entries (one-to-many).
RelLabelHasTodo = "has_todo"

// RelLabelTodoAssignedTo links a TaskTodo to the Agent responsible for it.
// Separate from RelLabelAssignedTo (Task → Agent) to keep endpoint validation clean.
RelLabelTodoAssignedTo = "todo_assigned_to"
```

Both are registered in `relationshipEndpointTypes`:

```go
RelLabelHasTodo:        {fromType: taskTypeID,     toType: taskTodoTypeID},
RelLabelTodoAssignedTo: {fromType: taskTodoTypeID, toType: agentTypeID},
```

---

## Consumer Registration

CodeValdWork must subscribe to `ai.task.todo` in its registrar `consumes` list:

```go
[]string{ // consumes
    "ai.task.in_progress",
    "ai.task.completed",
    "ai.task.failed",
    "ai.task.todo",   // ← bridges decomposition into TaskTodo entities
}
```

---

## Files to Create / Modify

| File | Change |
|---|---|
| `schema.go` | `TaskTodo` TypeDefinition; `has_todo` on `Task`; schema `Version: 2` |
| `relationship.go` | `RelLabelHasTodo`, `RelLabelTodoAssignedTo`; update `relationshipEndpointTypes` and `notFoundForType` |
| `models.go` | `TaskTodo` struct; `TodoStatus` type + constants |
| `errors.go` | `ErrTaskTodoNotFound` sentinel |
| `events.go` | `TopicTaskTodo`, `TaskTodoPayload`; add to `AllTopics()` |
| `task.go` | `taskTodoTypeID` constant |
| `internal/eventhandler/ai_task_todo.go` (new) | Consume `ai.task.todo`; create `TaskTodo` entities; publish `work.task.todo` per item |

---

## Acceptance Tests

| Test | Expected |
|---|---|
| Receive `ai.task.todo` with 3 items | 3 `TaskTodo` entities in `work_task_todos` |
| Each `TaskTodo` | `todo_of` edge to parent Task; `todo_assigned_to` edge to parent's agent |
| Agent fallback | No parent assignee → use `payload.AgentID` |
| 3 `work.task.todo` events published | One per `TaskTodo`, payload contains `TodoID` + `Instructions` |
| Parent Task status | Unchanged after todo creation |
| `ai.task.in_progress` with `TaskID = TodoID` | `TaskTodo.status → dispatched` |
| `ai.task.completed` with `TaskID = TodoID` | `TaskTodo.status → completed` |
| `ai.task.failed` with `TaskID = TodoID` | `TaskTodo.status → failed` |
| `ai.task.in_progress` with `TaskID = Task ID` | Parent `Task` status → `in_progress` (existing bridge — unaffected) |
