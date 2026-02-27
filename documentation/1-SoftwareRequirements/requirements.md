# Software Requirements

## Scope

CodeValdWork is a **Go library and gRPC microservice** that provides task lifecycle management for CodeValdCortex AI agencies. It replaces any ad-hoc task tracking in CodeValdCortex with a dedicated, versioned service.

---

## Functional Requirements

### FR-001 — Task Creation

- A caller can create a task for a named agency by providing a `title` (required), optional `description`, and optional `priority`.
- The service assigns a unique `ID`, sets the initial status to `pending`, and records `created_at` and `updated_at` timestamps.
- **Error**: returns `ErrTaskAlreadyExists` if a task with the same ID already exists.
- **Error**: returns `ErrInvalidTask` if `title` is empty.

### FR-002 — Task Retrieval

- A caller can retrieve a task by its `agencyID` and `taskID`.
- **Error**: returns `ErrTaskNotFound` if no matching task exists.

### FR-003 — Task Update

- A caller can update mutable fields: `title`, `description`, `priority`, `assigned_to`, and `status`.
- The service validates the status transition before applying the update.
- `updated_at` is refreshed on every successful update.
- When the new status is a terminal state (`completed`, `failed`, `cancelled`), `completed_at` is set.
- **Error**: returns `ErrTaskNotFound` if the task does not exist.
- **Error**: returns `ErrInvalidStatusTransition` for disallowed transitions.

### FR-004 — Task Deletion

- A caller can permanently delete a task by its `agencyID` and `taskID`.
- **Error**: returns `ErrTaskNotFound` if the task does not exist.

### FR-005 — Task Listing

- A caller can list all tasks for an agency.
- Optional filters: `status`, `priority`, `assigned_to`.
- Returns an empty list (not an error) when no tasks match the filter.

### FR-006 — Status Lifecycle

The valid transitions are:

```
pending     → in_progress
pending     → cancelled
in_progress → completed
in_progress → failed
in_progress → cancelled
```

All other transitions are rejected with `ErrInvalidStatusTransition`.

### FR-007 — CodeValdCross Registration

- On startup, CodeValdWork calls `OrchestratorService.Register` on CodeValdCross.
- It repeats the call at a configurable interval (heartbeat).
- Registration includes the pub/sub topics it produces and consumes.
- If `CROSS_GRPC_ADDR` is not set, registration is silently skipped.

### FR-008 — gRPC Service

- All task operations are exposed via the `TaskService` gRPC service defined in `proto/codevaldwork/v1/service.proto`.
- The service includes a gRPC health endpoint (`grpc.health.v1`).

---

## Non-Functional Requirements

| NFR | Requirement |
|---|---|
| NFR-001 | Thread safety — all `TaskManager` methods must be safe for concurrent use |
| NFR-002 | Backend agnosticism — the root package must not import ArangoDB drivers |
| NFR-003 | Context propagation — every exported method accepts `context.Context` as first arg |
| NFR-004 | Structured errors — all exported errors are typed sentinel values |
| NFR-005 | Test coverage — ≥80% coverage on exported functions; table-driven tests |
| NFR-006 | File size — max 500 lines per file; max 50 lines per function |

---

## Open Questions

_None at this time._
