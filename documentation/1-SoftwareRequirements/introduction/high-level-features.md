# High-Level Features

## Feature Overview

| Feature | Description |
|---|---|
| Task CRUD | Create, read, update, and delete tasks scoped to an agency |
| Status Lifecycle | Enforced state machine: `pending → in_progress → completed/failed/cancelled` |
| Filtering | List tasks by status, priority, or assigned agent |
| gRPC API | All operations via the `TaskService` gRPC service |
| Health Check | gRPC health endpoint for load balancers and orchestration platforms |
| CodeValdCross Registration | Heartbeat-based service registration and pub/sub contract announcement |
| ArangoDB Backend | Persistent, container-restart-safe task storage |

## Core Entity: Task

```
Task {
    ID          string        // server-assigned
    AgencyID    string        // scoped to an agency
    Title       string        // required
    Description string        // optional
    Status      TaskStatus    // pending | in_progress | completed | failed | cancelled
    Priority    TaskPriority  // low | medium | high | critical
    AssignedTo  string        // agent ID (empty = unassigned)
    CreatedAt   time.Time
    UpdatedAt   time.Time
    CompletedAt *time.Time    // set when terminal state is reached
}
```
