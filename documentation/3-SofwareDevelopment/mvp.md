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
| FEAT-20260602-002 | `workflow_run_id` on Task / TaskTodo + every `work.*` event payload (Work sibling of the [Cross umbrella](../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md)) | 🚀 In Progress | FEAT-20260601-001, ~~FEAT-20260602-001~~ ✅ in Functions (start-pipeline) |
| FEAT-20260602-003 | `WorkflowRun` status state machine — Work becomes the authoritative writer; transitions on `work.task.assigned`, `*.failed`, declared `terminal_event` | 📋 Not Started | FEAT-20260602-002 |
| FEAT-20260602-004 | Rollback / compensation semantics — design-only doc capturing the per-service contract for `POST /workflow-runs/{id}/rollback` | 📋 Design only | FEAT-20260602-002, FEAT-20260602-003 |
| FEAT-20260601-001 | `WorkflowRun` rollup endpoint — new entity in `schema.go` that anchors one orchestrated run; `GET /work/{agency}/workflow-runs/{id}` returns the run with every entity + edge in its closure, enough for a future transactional rollback | 📋 Not Started | — |
| MVP-WORK-016 | Unit & integration tests — `fakeDataManager` updated for graph edges; ArangoDB end-to-end scenarios covering subtasks, blockers (gate), assignment via edge, TaskGroup membership, and verification of all six events published | 🚀 In Progress | MVP-WORK-008…015 (done) |

See [mvp-details/integration-tests.md](mvp-details/integration-tests.md) for the test plan, [mvp-details/FEAT-20260601-001_workflow_run_rollup.md](mvp-details/FEAT-20260601-001_workflow_run_rollup.md) for the rollup endpoint design, and the FEAT-20260602-* set for the wider workflow-run expansion (anchored by the [Cross umbrella](../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-001_workflow_run_id_propagation_umbrella.md)).

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
