# MVP — CodeValdWork

## Goal

Deliver a production-ready task management gRPC microservice with ArangoDB persistence and CodeValdCross registration.

---

## MVP Scope

The MVP delivers:
1. The `TaskManager` Go interface and its `taskManager` implementation
2. The `Task` domain model with status lifecycle enforcement
3. An ArangoDB `Backend` implementation
4. A `TaskService` gRPC service exposing all CRUD+list operations
5. CodeValdCross heartbeat registration
6. Integration tests for all five gRPC operations

---

## Task List

| Task ID | Title | Status | Depends On |
|---|---|---|---|
| MVP-WORK-001 | Library Scaffolding & Task Model | ✅ Done | — |
| MVP-WORK-002 | ArangoDB Backend | ✅ Done | MVP-WORK-001 |
| MVP-WORK-003 | gRPC Service (TaskService) | ✅ Done | MVP-WORK-001 |
| MVP-WORK-004 | CodeValdCross Registration | ✅ Done | MVP-WORK-003 |
| MVP-WORK-005 | Unit & Integration Tests | ✅ Done | MVP-WORK-001, MVP-WORK-002 |
| MVP-WORK-006 | Service-Driven Route Registration | ✅ Done | MVP-WORK-003, CROSS-007 |

*All tasks complete — see `mvp_done.md` for details.*

---

## Success Criteria

- ✅ `go build ./...` succeeds
- ✅ `go test -race ./...` all pass
- ✅ `go vet ./...` shows 0 issues
- ✅ All five `TaskService` RPCs work end-to-end with ArangoDB
- ✅ CodeValdCross registration fires on startup and repeats on heartbeat
- ✅ Invalid status transitions return `FAILED_PRECONDITION` from gRPC
- ✅ Routes (`POST /{agencyId}/tasks`, `GET /{agencyId}/tasks`) declared in `RegisterRequest` and proxied via CodeValdCross dynamic proxy

---

## Branch Naming

```
feature/WORK-001_library_scaffolding
feature/WORK-002_arangodb_backend
feature/WORK-003_grpc_service
feature/WORK-004_cross_registration
feature/WORK-005_integration_tests
```
