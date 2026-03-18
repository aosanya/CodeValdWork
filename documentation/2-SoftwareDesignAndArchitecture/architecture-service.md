# CodeValdWork — Architecture: Service

> Part of [architecture.md](architecture.md)

## 1. gRPC Service

`TaskService` is defined in `proto/codevaldwork/v1/service.proto`. Handlers
are thin — they translate protobuf ↔ domain types and delegate to
`TaskManager`.

```proto
service TaskService {
    // Task CRUD (delegates to TaskManager → WorkDataManager)
    rpc CreateTask(CreateTaskRequest)         returns (CreateTaskResponse);
    rpc GetTask(GetTaskRequest)               returns (GetTaskResponse);
    rpc UpdateTask(UpdateTaskRequest)         returns (UpdateTaskResponse);
    rpc DeleteTask(DeleteTaskRequest)         returns (DeleteTaskResponse);
    rpc ListTasks(ListTasksRequest)           returns (ListTasksResponse);

    // Task relationships (delegates to WorkDataManager)
    rpc AssignTask(AssignTaskRequest)                 returns (AssignTaskResponse);
    rpc UnassignTask(UnassignTaskRequest)             returns (UnassignTaskResponse);
    rpc CreateTaskRelationship(CreateRelRequest)      returns (CreateRelResponse);
    rpc DeleteTaskRelationship(DeleteRelRequest)      returns (DeleteRelResponse);
    rpc ListTaskRelationships(ListRelRequest)         returns (ListRelResponse);
    rpc TraverseTaskGraph(TraverseRequest)            returns (TraverseResponse);

    // TaskGroup
    rpc CreateTaskGroup(CreateTaskGroupRequest)       returns (CreateTaskGroupResponse);
    rpc GetTaskGroup(GetTaskGroupRequest)             returns (GetTaskGroupResponse);
    rpc UpdateTaskGroup(UpdateTaskGroupRequest)       returns (UpdateTaskGroupResponse);
    rpc DeleteTaskGroup(DeleteTaskGroupRequest)       returns (DeleteTaskGroupResponse);
    rpc ListTaskGroups(ListTaskGroupsRequest)         returns (ListTaskGroupsResponse);

    // Agent
    rpc CreateAgent(CreateAgentRequest)               returns (CreateAgentResponse);
    rpc GetAgent(GetAgentRequest)                     returns (GetAgentResponse);
    rpc UpdateAgent(UpdateAgentRequest)               returns (UpdateAgentResponse);
    rpc DeleteAgent(DeleteAgentRequest)               returns (DeleteAgentResponse);
    rpc ListAgents(ListAgentsRequest)                 returns (ListAgentsResponse);
}
```

---

## 2. Error Mapping

| Domain Error | gRPC Code |
|---|---|
| `ErrTaskNotFound` | `codes.NotFound` |
| `ErrTaskAlreadyExists` | `codes.AlreadyExists` |
| `ErrInvalidStatusTransition` | `codes.FailedPrecondition` |
| `ErrBlockerNotTerminal` | `codes.FailedPrecondition` |
| `ErrInvalidTask` | `codes.InvalidArgument` |
| `ErrSchemaNotFound` | `codes.NotFound` |
| all others | `codes.Internal` |

---

## 3. HTTP Convenience Routes

All paths are prefixed with `/{agencyId}/work`.

### Tasks

| Method | Path | Operation |
|---|---|---|
| `POST` | `/{agencyId}/work/tasks` | `CreateTask` |
| `GET` | `/{agencyId}/work/tasks` | `ListTasks` (supports `?status=`, `?priority=`, `?assignedTo=`, `?tag=`) |
| `GET` | `/{agencyId}/work/tasks/{taskId}` | `GetTask` |
| `PUT` | `/{agencyId}/work/tasks/{taskId}` | `UpdateTask` |
| `DELETE` | `/{agencyId}/work/tasks/{taskId}` | `DeleteTask` |

### Task Status (shorthand)

| Method | Path | Operation |
|---|---|---|
| `PUT` | `/{agencyId}/work/tasks/{taskId}/status` | `UpdateTask` (status field only) |
| `PUT` | `/{agencyId}/work/tasks/{taskId}/assign` | `AssignTask` → creates `assigned_to` edge |
| `DELETE` | `/{agencyId}/work/tasks/{taskId}/assign` | `UnassignTask` → removes `assigned_to` edge |

### Task Relationships

| Method | Path | Operation |
|---|---|---|
| `POST` | `/{agencyId}/work/tasks/{taskId}/relationships` | `CreateTaskRelationship` (body: `{label, targetTaskId, reason?}`) |
| `GET` | `/{agencyId}/work/tasks/{taskId}/relationships` | `ListTaskRelationships` (supports `?label=blocks`) |
| `DELETE` | `/{agencyId}/work/tasks/{taskId}/relationships/{relId}` | `DeleteTaskRelationship` |
| `GET` | `/{agencyId}/work/tasks/{taskId}/blockers` | Traverse `blocks` inbound — tasks blocking this one |
| `GET` | `/{agencyId}/work/tasks/{taskId}/blocking` | Traverse `blocks` outbound — tasks this one is blocking |
| `GET` | `/{agencyId}/work/tasks/{taskId}/subtasks` | Traverse `subtask_of` inbound — child tasks |
| `GET` | `/{agencyId}/work/tasks/{taskId}/dependencies` | Traverse `depends_on` outbound — tasks this depends on |

### TaskGroups

| Method | Path | Operation |
|---|---|---|
| `POST` | `/{agencyId}/work/groups` | `CreateTaskGroup` |
| `GET` | `/{agencyId}/work/groups` | `ListTaskGroups` |
| `GET` | `/{agencyId}/work/groups/{groupId}` | `GetTaskGroup` |
| `PUT` | `/{agencyId}/work/groups/{groupId}` | `UpdateTaskGroup` |
| `DELETE` | `/{agencyId}/work/groups/{groupId}` | `DeleteTaskGroup` |
| `POST` | `/{agencyId}/work/groups/{groupId}/tasks` | `CreateTaskRelationship(member_of)` |
| `GET` | `/{agencyId}/work/groups/{groupId}/tasks` | Traverse `member_of` inbound |
| `DELETE` | `/{agencyId}/work/groups/{groupId}/tasks/{taskId}` | `DeleteTaskRelationship(member_of)` |

### Agents

| Method | Path | Operation |
|---|---|---|
| `POST` | `/{agencyId}/work/agents` | `CreateAgent` |
| `GET` | `/{agencyId}/work/agents` | `ListAgents` (supports `?capability=`) |
| `GET` | `/{agencyId}/work/agents/{agentId}` | `GetAgent` |
| `PUT` | `/{agencyId}/work/agents/{agentId}` | `UpdateAgent` |
| `DELETE` | `/{agencyId}/work/agents/{agentId}` | `DeleteAgent` |
| `GET` | `/{agencyId}/work/agents/{agentId}/tasks` | Traverse `assigned_to` inbound — tasks assigned to agent |

---

## 4. CodeValdCross Registration

CodeValdWork registers with CodeValdCross on startup and sends heartbeats
every `CROSS_PING_INTERVAL` (default 10s).

```go
RegisterRequest{
    ServiceName: "codevaldwork",
    Addr:        ":50053",
    Produces: []string{
        "work.task.created",
        "work.task.updated",
        "work.task.status.changed",
        "work.task.completed",
        "work.task.assigned",
        "work.relationship.created",
    },
    Consumes: []string{
        "cross.task.requested",   // Cross dispatching a new task
        "cross.agency.created",   // Seed default schema for new agency
    },
    Routes: workRoutes(),  // all HTTP convenience routes above
}
```

When `cross.agency.created` is received, CodeValdWork calls
`WorkSchemaManager.SetSchema(agencyID, defaultWorkSchema)` if no schema
exists for that agency.

When `cross.task.requested` is received, CodeValdWork calls
`TaskManager.CreateTask` with the payload from the event.

---

## 5. Project Layout

```
CodeValdWork/
├── cmd/
│   └── server/
│       └── main.go             # Dependency wiring only
├── errors.go                   # ErrTaskNotFound, ErrInvalidStatusTransition, ErrBlockerNotTerminal, etc.
├── task.go                     # TaskManager interface + taskManager implementation
├── types.go                    # Task, TaskGroup, Agent, TaskStatus, TaskPriority, TaskFilter
├── schema.go                   # defaultWorkSchema — pre-delivered TypeDefinitions
├── go.mod
├── internal/
│   ├── grpcserver/
│   │   ├── server.go           # gRPC TaskService handlers
│   │   └── errors.go           # mapError — domain error → gRPC status
│   ├── httphandler/
│   │   └── handler.go          # HTTP convenience route handlers
│   ├── config/
│   │   └── config.go           # Config struct + loader
│   └── registrar/              # (delegated to SharedLib registrar package)
├── storage/
│   └── arangodb/
│       └── arangodb.go         # ArangoDB Backend implementation
└── proto/
    └── codevaldwork/
        └── v1/
            ├── service.proto
            └── task.proto
```
