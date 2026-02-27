# MVP Done — Completed Tasks

Completed tasks are removed from `mvp.md` and recorded here with their completion date.

| Task ID | Title | Completion Date | Branch | Notes |
|---------|-------|-----------------|--------|-------|
| MVP-WORK-001 | Library Scaffolding & Task Model | 2026-02-27 | — | `task.go`, `types.go`, `errors.go`; `TaskManager` interface + `taskManager` impl; status state machine in `types.go` |
| MVP-WORK-002 | ArangoDB Backend | 2026-02-27 | — | `storage/arangodb/arangodb.go`; `work_tasks` collection; all five CRUD + list operations; `arangodb_test.go` |
| MVP-WORK-003 | gRPC Service (TaskService) | 2026-02-27 | — | `proto/codevaldwork/v1/service.proto`; generated stubs in `gen/go/codevaldwork/v1/`; `internal/grpcserver/server.go` implements all five RPCs; `internal/grpcserver/errors.go` maps domain errors to gRPC codes |
| MVP-WORK-004 | CodeValdCross Registration | 2026-02-27 | — | `internal/registrar/registrar.go`; heartbeats to Cross on startup and every 10 s; sends `produces`, `consumes`, `addr`, and `routes` in each `RegisterRequest` |
| MVP-WORK-005 | Unit & Integration Tests | 2026-02-27 | — | `task_test.go` — comprehensive unit tests with `fakeBackend` covering CreateTask, GetTask, UpdateTask, DeleteTask, ListTasks, status-transition enforcement; `storage/arangodb/arangodb_test.go` — ArangoDB backend integration tests |
| MVP-WORK-006 | Service-Driven Route Registration | 2026-02-27 | — | Routes `POST /{agencyId}/tasks` (`create_task`) and `GET /{agencyId}/tasks` (`list_tasks`) declared in `registrar.go` with `GrpcMethod` + `PathBindings`; CodeValdCross dynamic proxy handles forwarding with no Cross code changes |
