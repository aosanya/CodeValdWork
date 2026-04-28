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

## Phase 1 — Task Lifecycle (Complete)

| Task ID | Title | Status | Depends On |
|---|---|---|---|
| MVP-WORK-001 | Library Scaffolding & Task Model | ✅ Done | — |
| MVP-WORK-002 | ArangoDB Backend | ✅ Done | MVP-WORK-001 |
| MVP-WORK-003 | gRPC Service (TaskService) | ✅ Done | MVP-WORK-001 |
| MVP-WORK-004 | CodeValdCross Registration | ✅ Done | MVP-WORK-003 |
| MVP-WORK-005 | Unit & Integration Tests | ✅ Done | MVP-WORK-001, MVP-WORK-002 |
| MVP-WORK-006 | Service-Driven Route Registration | ✅ Done | MVP-WORK-003, CROSS-007 |

*All Phase 1 tasks complete — see `mvp_done.md` for details.*

---

## Phase 2 — Entity-Graph Completion

Phase 1 delivered Task CRUD on top of `entitygraph.DataManager`. The
`architecture-domain.md` design adds two more entity types (`TaskGroup`,
`Agent`), five graph edges, hard blocker enforcement, richer Task properties,
and a complete pub/sub publishing pipeline. Phase 2 closes that gap.

> **Greenfield assumption** — CodeValdWork has not been deployed. Phase 2 is
> free to make breaking schema changes (e.g. converting `assigned_to` from a
> string property into a graph edge) without a migration path.

| Task ID | Title | Status | Depends On |
|---|---|---|---|
| MVP-WORK-008 | Schema extension — `TaskGroup` + `Agent` TypeDefinitions; richer Task properties (`dueAt`, `tags`, `estimatedHours`, `context`, `completedAt`); `option` typing for `status`/`priority` | ✅ Complete | — |
| MVP-WORK-009 | Graph relationship API — `work_relationships` edge collection, `work_graph` named graph, edge-label whitelist (`blocks`, `subtask_of`, `depends_on`, `member_of`, `assigned_to`), `CreateRelationship` / `DeleteRelationship` / `TraverseRelationships` on `TaskManager` | ✅ Complete | MVP-WORK-008 |
| MVP-WORK-010 | Agent vertex + `assigned_to` edge — drop `assigned_to` string property, add `UpsertAgent` and `AssignTask(agencyID, taskID, agentID)` creating the edge | ✅ Complete | MVP-WORK-008, MVP-WORK-009 |
| MVP-WORK-011 | Hard blocker enforcement — `pending → in_progress` traverses inbound `blocks`; new `ErrBlocked` (`FAILED_PRECONDITION`) with `BlockedByInfo` proto detail | ✅ Complete | MVP-WORK-009 |
| MVP-WORK-012 | TaskGroup CRUD + `member_of` edge enforcement (target-type whitelist) | ✅ Complete | MVP-WORK-008, MVP-WORK-009 |
| MVP-WORK-013 | gRPC surface expansion — new RPCs (`AssignTask`, `CreateRelationship`, `DeleteRelationship`, traversal RPCs, `TaskGroup` CRUD, `UpsertAgent`); new proto messages; error mapping for `ErrBlocked` | ✅ Complete | MVP-WORK-010, MVP-WORK-011, MVP-WORK-012 |
| MVP-WORK-014 | Pub/sub publishing pipeline — extend `CrossPublisher` to carry typed payloads; publish hooks for all six topics (`work.task.created`, `work.task.updated`, `work.task.status.changed`, `work.task.completed`, `work.task.assigned`, `work.relationship.created`); update registrar `produces` list | ✅ Complete | MVP-WORK-008…012 |
| MVP-WORK-015 | HTTP convenience routes — mirror CodeValdComm pattern; routes for Task, TaskGroup, Agent, relationships declared in registrar `RegisterRequest` for Cross dynamic proxy | 🟒 Not Started | MVP-WORK-013 |
| MVP-WORK-016 | Unit & integration tests — `fakeDataManager` updated for graph edges; ArangoDB end-to-end scenarios covering subtasks, blockers (gate), assignment via edge, TaskGroup membership, and verification of all six events published | 🟒 Not Started | MVP-WORK-008…015 |

See per-topic specs:

- [mvp-details/schema.md](mvp-details/schema.md) — WORK-008
- [mvp-details/relationships.md](mvp-details/relationships.md) — WORK-009, WORK-011
- [mvp-details/agent-assignment.md](mvp-details/agent-assignment.md) — WORK-010
- [mvp-details/task-group.md](mvp-details/task-group.md) — WORK-012
- [mvp-details/grpc-surface.md](mvp-details/grpc-surface.md) — WORK-013, WORK-015
- [mvp-details/pubsub.md](mvp-details/pubsub.md) — WORK-014
- [mvp-details/integration-tests.md](mvp-details/integration-tests.md) — WORK-016

---

## P3: CodeValdSharedLib Migration

| Task ID | Title | Status | Depends On |
|---|---|---|---|
| MVP-WORK-007 | Migrate shared infrastructure to CodeValdSharedLib | ✅ Done | SHAREDLIB-003, SHAREDLIB-004, SHAREDLIB-005, SHAREDLIB-006 |

**MVP-WORK-007 scope**:
- Replace `internal/registrar/` with `github.com/aosanya/CodeValdSharedLib/registrar` (caller passes `"codevaldwork"`, its topics, and its `declaredRoutes`).
- Replace `envOrDefault` / `parseDuration` helpers in `cmd/server/main.go` with `serverutil.EnvOrDefault` / `serverutil.ParseDurationString`.
- Replace the gRPC server setup block in `cmd/server/main.go` with `serverutil.NewGRPCServer()` + `serverutil.RunWithGracefulShutdown()`.
- Replace the ArangoDB `http.NewConnection` / auth / database bootstrap in `storage/arangodb/arangodb.go` with `arangoutil.Connect(ctx, cfg)`.
- Replace the local copy of `gen/go/codevaldcross/v1/` with the SharedLib copy; update `internal/registrar/` import paths.
- Remove `internal/registrar/` package entirely.
- Update `go.mod` with `require github.com/aosanya/CodeValdSharedLib` and a `replace ../CodeValdSharedLib` directive.

See [CodeValdSharedLib mvp.md](../../../CodeValdSharedLib/documentation/3-SofwareDevelopment/mvp.md) for the full SharedLib task breakdown.

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

Phase 1:

```
feature/WORK-001_library_scaffolding
feature/WORK-002_arangodb_backend
feature/WORK-003_grpc_service
feature/WORK-004_cross_registration
feature/WORK-005_integration_tests
```

Phase 2:

```
feature/WORK-008_schema_extension
feature/WORK-009_graph_relationships
feature/WORK-010_agent_assignment
feature/WORK-011_blocker_enforcement
feature/WORK-012_task_group
feature/WORK-013_grpc_surface
feature/WORK-014_pubsub_pipeline
feature/WORK-015_http_routes
feature/WORK-016_phase2_tests
```
