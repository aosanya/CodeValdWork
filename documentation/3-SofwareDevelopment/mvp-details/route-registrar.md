# Service-Driven Route Registration

Topics: HTTP Routing · Route Registrar · CodeValdCross Integration

---

## MVP-WORK-006 — Service-Driven Route Registration

### Overview

Move work-backed HTTP handler functions out of CodeValdCross's `internal/server/http.go`
and into `internal/clients/work/` inside CodeValdCross. Expose them through a
`Routes(orch *orchestrator.Orchestrator) []server.Route` function so that
`NewHTTPServer` never names `clients/work` directly — it mounts whatever routes
are handed to it.

This task is the CodeValdWork-side deliverable of the pattern decision made in
**CROSS-007**. It has no changes to the CodeValdWork repository itself — all
changes live in the CodeValdCross module, specifically in `internal/clients/work/`.

---

### Dependencies

- **MVP-WORK-003** ✅ (gRPC service — `TaskServiceClient` is available)
- **CROSS-006** (work task endpoints — handlers already exist in `http.go`)
- **CROSS-007** must be designed and spec'd before implementation begins
  (provides the `server.Route` type that `Routes()` returns)

---

### Acceptance Criteria

#### `internal/clients/work/routes.go` (new file in CodeValdCross repo)

- [ ] Package `work` (same package as `client.go` and `mock.go`)
- [ ] Imports only `net/http`, `internal/orchestrator`, and `internal/server`
      (no circular imports)
- [ ] Exported function:
  ```go
  // Routes returns the HTTP routes backed by CodeValdWork operations.
  // Pass the result to server.NewHTTPServer alongside routes from other
  // client packages.
  func Routes(orch *orchestrator.Orchestrator) []server.Route
  ```
- [ ] Returns exactly two routes:
  | Method | Pattern | Handler |
  |--------|---------|---------|
  | `POST` | `/{agencyId}/tasks` | `handleCreateTask` |
  | `GET`  | `/{agencyId}/tasks` | `handleListTasks`  |
- [ ] `handleCreateTask` and `handleListTasks` moved verbatim from
  `internal/server/http.go` into this file (or a companion `handlers.go`)
- [ ] Both handlers call `server.WriteJSONError` (exported helper) for error
  responses
- [ ] Local types `createTaskBody` and `workTaskResponse` and the helper
  `workTaskToResponse` move into this file (they are work-specific)
- [ ] Godoc on `Routes` and on each handler function

#### `internal/server/http.go` (modified in CodeValdCross repo)

- [ ] `handleCreateTask`, `handleListTasks`, `createTaskBody`,
  `workTaskResponse`, and `workTaskToResponse` removed
- [ ] `writeJSONError` renamed to `WriteJSONError` (exported); all call sites
  updated across `clients/git` and `clients/work` packages

---

### What Does NOT Change in CodeValdWork

This task makes no changes to the CodeValdWork repository. The proto definitions,
gRPC server, generated stubs, and `TaskService` implementation are untouched.
The `WorkClient` interface gains no new methods.

---

### Test Impact

- Existing orchestrator tests (`orchestrator/task_test.go`) are unaffected
- Any direct HTTP handler tests should be re-homed into the `work` package
  alongside the handler functions
- `go build ./...` and `go test -race ./...` in CodeValdCross must pass

---

### Branch Naming (in CodeValdCross repo)

```
feature/CROSS-007_service_driven_route_registration
```

This is a shared branch with GIT-011 — both are part of the same refactor
implemented as a single CodeValdCross feature branch.
