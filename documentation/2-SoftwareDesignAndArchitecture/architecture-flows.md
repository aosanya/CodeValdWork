# CodeValdWork — Architecture: Flows

> Part of [architecture.md](architecture.md)

## Error Types

| Error | gRPC Code | When |
|---|---|---|
| `ErrTaskNotFound` | `codes.NotFound` | Task does not exist |
| `ErrTaskAlreadyExists` | `codes.AlreadyExists` | Task ID already present for agency |
| `ErrInvalidStatusTransition` | `codes.FailedPrecondition` | Requested status not reachable from current |
| `ErrBlockerNotTerminal` | `codes.FailedPrecondition` | Transition to `in_progress` blocked by non-terminal blocker task |
| `ErrInvalidTask` | `codes.InvalidArgument` | Required fields missing (e.g. empty title) |
| `ErrSchemaNotFound` | `codes.NotFound` | Agency schema not seeded |

---

## Flow 1: CreateTask (POST /{agencyId}/work/tasks)

**Inputs:** `agencyID`, `title`, `description?`, `priority?`, `dueAt?`, `tags?`, `estimatedHours?`, `context?`

```
1. Validate inputs — title must not be empty
2. workSchemaManager.GetSchema(ctx, agencyID)
   → ErrSchemaNotFound if no schema seeded for agency
3. Resolve TypeDefinition for "Task" → StorageCollection = "work_tasks"
4. workDataManager.CreateEntity(ctx, CreateEntityRequest{
       AgencyID: agencyID,
       TypeID:   "Task",
       Properties: {
           title, description, status: "pending",
           priority: priority || "medium",
           dueAt, tags, estimatedHours, context,
       },
   })
   → ErrInvalidEntity on validation failure
   → returns task entity with generated ID
5. bus.Publish(ctx, Message{
       ID:      uuid.New().String(),
       Topic:   "work.task.created",
       Payload: { taskID: task.ID, agencyID, title, priority },
       Source:  "codevaldwork",
   })
6. Return task entity
```

---

## Flow 2: UpdateTask — Field Change (PUT /{agencyId}/work/tasks/{taskId})

**Inputs:** `agencyID`, `taskID`, mutable fields (not status)

```
1. Validate inputs
2. workDataManager.GetEntity(ctx, agencyID, taskID)
   → ErrTaskNotFound if task does not exist
3. workDataManager.UpdateEntity(ctx, UpdateEntityRequest{
       AgencyID:   agencyID,
       EntityID:   taskID,
       Properties: { changedFields... },
   })
4. bus.Publish(ctx, Message{
       Topic:   "work.task.updated",
       Payload: { taskID, agencyID, changedFields: [list of changed field names] },
       Source:  "codevaldwork",
   })
5. Return updated task entity
```

---

## Flow 3: UpdateTask — Status Transition (PUT /{agencyId}/work/tasks/{taskId}/status)

**Inputs:** `agencyID`, `taskID`, `newStatus`

```
1. Validate inputs
2. workDataManager.GetEntity(ctx, agencyID, taskID)
   → ErrTaskNotFound if task does not exist
   → read current status from entity properties
3. Validate transition: current.Status.CanTransitionTo(newStatus)
   → ErrInvalidStatusTransition if not valid
4. If newStatus == "in_progress":
   a. workDataManager.TraverseGraph(ctx, TraverseGraphRequest{
          AgencyID:   agencyID,
          StartID:    taskID,
          Label:      "blocks",
          Direction:  inbound,
          Depth:      1,
      })
      → returns list of tasks that block this one
   b. For each blocker: check blocker.status is terminal
      (completed | failed | cancelled)
      → ErrBlockerNotTerminal if any blocker is non-terminal
5. If newStatus is terminal (completed | failed | cancelled):
   set completedAt = time.Now().UTC()
6. workDataManager.UpdateEntity(ctx, UpdateEntityRequest{
       AgencyID:   agencyID,
       EntityID:   taskID,
       Properties: { status: newStatus, completedAt? },
   })
7. bus.Publish(ctx, Message{
       Topic:   "work.task.status.changed",
       Payload: { taskID, agencyID, from: current.Status, to: newStatus },
       Source:  "codevaldwork",
   })
8. If newStatus is terminal:
   bus.Publish(ctx, Message{
       Topic:   "work.task.completed",
       Payload: { taskID, agencyID, terminalStatus: newStatus, completedAt },
       Source:  "codevaldwork",
   })
9. Return updated task entity
```

---

## Flow 4: AssignTask (PUT /{agencyId}/work/tasks/{taskId}/assign)

**Inputs:** `agencyID`, `taskID`, `agentID`, `assignedBy`

```
1. Validate inputs
2. workDataManager.GetEntity(ctx, agencyID, taskID)
   → ErrTaskNotFound if task does not exist
3. workDataManager.GetEntity(ctx, agencyID, agentID)
   → ErrTaskNotFound (agent not found) if agent does not exist
4. Check if existing assigned_to edge exists for taskID
   - if yes: DeleteRelationship(existing edge) — one-to-one assignment
5. workDataManager.CreateRelationship(ctx, CreateRelationshipRequest{
       AgencyID:  agencyID,
       Label:     "assigned_to",
       FromID:    taskID,
       FromType:  "work_tasks",
       ToID:      agentID,
       ToType:    "work_agents",
       Properties: { assignedAt: time.Now().UTC(), assignedBy },
   })
6. bus.Publish(ctx, Message{
       Topic:   "work.task.assigned",
       Payload: { taskID, agencyID, agentID },
       Source:  "codevaldwork",
   })
7. Return relationship document
```

---

## Flow 5: CreateTaskRelationship (POST /{agencyId}/work/tasks/{taskId}/relationships)

**Inputs:** `agencyID`, `fromTaskID`, `label` (`blocks` | `subtask_of` | `depends_on`), `toTaskID`, `reason?`

```
1. Validate inputs — label must be one of the allowed values
2. workDataManager.GetEntity(ctx, agencyID, fromTaskID)
   → ErrTaskNotFound if source task does not exist
3. workDataManager.GetEntity(ctx, agencyID, toTaskID)
   → ErrTaskNotFound if target task does not exist
4. Guard against self-referential edges: fromTaskID != toTaskID
5. workDataManager.CreateRelationship(ctx, CreateRelationshipRequest{
       AgencyID:  agencyID,
       Label:     label,
       FromID:    fromTaskID,
       FromType:  "work_tasks",
       ToID:      toTaskID,
       ToType:    "work_tasks",
       Properties: { createdAt: time.Now().UTC(), reason? },
   })
6. bus.Publish(ctx, Message{
       Topic:   "work.relationship.created",
       Payload: { fromID: fromTaskID, toID: toTaskID, label, agencyID },
       Source:  "codevaldwork",
   })
7. Return relationship document
```

---

## Flow 6: AddTaskToGroup (POST /{agencyId}/work/groups/{groupId}/tasks)

**Inputs:** `agencyID`, `taskID`, `groupID`

```
1. Validate inputs
2. workDataManager.GetEntity(ctx, agencyID, taskID)
   → ErrTaskNotFound if task does not exist
3. workDataManager.GetEntity(ctx, agencyID, groupID)
   → ErrTaskNotFound if group does not exist
4. Check if member_of edge already exists — idempotent if so
5. workDataManager.CreateRelationship(ctx, CreateRelationshipRequest{
       AgencyID:  agencyID,
       Label:     "member_of",
       FromID:    taskID,
       FromType:  "work_tasks",
       ToID:      groupID,
       ToType:    "work_groups",
       Properties: { addedAt: time.Now().UTC() },
   })
6. Return relationship document
```

No pub/sub event for group membership — informational only.

---

## Flow 7: SchemaSeeding (on cross.agency.created)

**Trigger:** `cross.agency.created` pub/sub event from CodeValdCross

```
1. Extract agencyID from event payload
2. workSchemaManager.GetSchema(ctx, agencyID)
   - if schema exists: return (idempotent — do nothing)
   - if ErrSchemaNotFound: proceed to step 3
3. workSchemaManager.SetSchema(ctx, agencyID, defaultWorkSchema)
   - defaultWorkSchema is the package-level constant in schema.go
   - Contains TypeDefinitions for Task, TaskGroup, Agent
4. Log "codevaldwork: seeded default schema for agency %s"
```

---

## Flow 8: CrossTaskRequested (on cross.task.requested)

**Trigger:** `cross.task.requested` pub/sub event from CodeValdCross

```
1. Extract agencyID, title, description, priority from event payload
2. Execute Flow 1 (CreateTask) with extracted fields
   → Errors are logged; not re-published (the Cross event is not acknowledged)
```
