# MVP — CodeValdWork

## Goal

Deliver a production-ready task management gRPC microservice with ArangoDB persistence and CodeValdCross registration.

---

## Status

Phases 1, 2 (entity-graph completion), and the CodeValdSharedLib migration are
shipped. See [mvp_done.md](mvp_done.md) for the full record. Only Phase 2
integration tests remain.

---

## Outstanding Work

| Task ID | Title | Status | Depends On |
|---|---|---|---|
| MVP-WORK-016 | Unit & integration tests — `fakeDataManager` updated for graph edges; ArangoDB end-to-end scenarios covering subtasks, blockers (gate), assignment via edge, TaskGroup membership, and verification of all six events published | 🚀 In Progress | MVP-WORK-008…015 (done) |

See [mvp-details/integration-tests.md](mvp-details/integration-tests.md) for the test plan.

---

## Success Criteria

- ✅ `go build ./...` succeeds
- ✅ `go test -race ./...` all pass
- ✅ `go vet ./...` shows 0 issues
- ✅ All `TaskService` RPCs work end-to-end with ArangoDB
- ✅ CodeValdCross registration fires on startup and repeats on heartbeat
- ✅ Invalid status transitions return `FAILED_PRECONDITION` from gRPC
- ✅ HTTP routes declared in `RegisterRequest` and proxied via CodeValdCross dynamic proxy
- ⏳ All six pub/sub events verified end-to-end in integration tests (WORK-016)

---

## Branch Naming

```
feature/WORK-016_phase2_tests
```
