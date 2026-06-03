# MVP Details — Index

## Overview

This directory contains per-topic implementation specifications for the
CodeValdWork MVP tasks. Phase 1 (Task lifecycle) is complete; Phase 2
(entity-graph completion) is in planning.

---

## Phase 1 — Task Lifecycle (Complete)

| File | Tasks | Description |
|---|---|---|
| [task-management.md](task-management.md) | MVP-WORK-001…005 | Phase 1 summary + pointers to current code |
| [route-registrar.md](route-registrar.md) | MVP-WORK-006 | Service-driven route registration for CodeValdCross |

---

## Phase 2 — Entity-Graph Completion

| File | Tasks | Description |
|---|---|---|
| [schema.md](schema.md) | MVP-WORK-008 | `TaskGroup` + `Agent` TypeDefinitions; richer `Task` properties |
| [relationships.md](relationships.md) | MVP-WORK-009, MVP-WORK-011 | `work_relationships` edge collection + relationship API; hard blocker enforcement on `pending → in_progress` |
| [agent-assignment.md](agent-assignment.md) | MVP-WORK-010 | `Agent` vertex + `assigned_to` edge replacing the Phase 1 string field |
| [task-group.md](task-group.md) | MVP-WORK-012 | TaskGroup CRUD + `member_of` edge enforcement |
| [grpc-surface.md](grpc-surface.md) | MVP-WORK-013, MVP-WORK-015 | gRPC RPC expansion + HTTP convenience routes |
| [pubsub.md](pubsub.md) | MVP-WORK-014 | Six-topic publishing pipeline; SharedLib `eventbus` extraction candidate |
| [integration-tests.md](integration-tests.md) | MVP-WORK-016 | Phase 2 unit + ArangoDB end-to-end coverage |
| [task-decomposition.md](task-decomposition.md) | — | `ai.todo.created` bridge → `TaskTodo` entity → `work.todo.dispatched` publisher; todo assignment and status lifecycle |
| [task-failure-modes.md](task-failure-modes.md) | — | Task/todo failure events, terminal WorkflowRun closure rules, field contracts for synthesized-success recovery pipelines per [Cross FEAT-20260602-005](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md) |

---

## WorkflowRun Expansion (2026-06-02)

| File | FEAT | Status | Description |
|---|---|---|---|
| [FEAT-20260601-001_workflow_run_rollup.md](FEAT-20260601-001_workflow_run_rollup.md) | FEAT-20260601-001 | — | WorkflowRun entity + closure rollup |
| [FEAT-20260602-001_workflow_run_name.md](FEAT-20260602-001_workflow_run_name.md) | FEAT-20260602-001 | — | Unique WorkflowRun name |
| [FEAT-20260602-002_workflow_run_id_in_work.md](FEAT-20260602-002_workflow_run_id_in_work.md) | FEAT-20260602-002 | — | `workflow_run_id` propagation through Work entities |
| [FEAT-20260602-003_workflow_run_status_state_machine.md](FEAT-20260602-003_workflow_run_status_state_machine.md) | FEAT-20260602-003 | — | Run status state machine driven by inbound events |
| [FEAT-20260602-004_workflow_run_rollback_semantics.md](FEAT-20260602-004_workflow_run_rollback_semantics.md) | FEAT-20260602-004 | ✅ Phase 1+3 / ⏸️ Phase 2 | Rollback coordinator + CodeValdWork artifact cleanup leg; cross-service legs still stubs |
| [FEAT-20260602-008_workflow_run_cancel.md](FEAT-20260602-008_workflow_run_cancel.md) | FEAT-20260602-008 | 📋 Draft | Mid-flight cancel endpoint — `POST /workflow-runs/{id}/cancel`; `cancelling` transient + `cancelled` terminal states; quiesce signal cascades to AI/Functions; complements rollback (which only accepts terminal runs) |
| [FEAT-20260603-003_workflow_failure_recovery.md](FEAT-20260603-003_workflow_failure_recovery.md) | FEAT-20260603-003 | 📋 Draft | Failure recovery ladder: N retries → AI classifies transient/requires-human → JSON direction form → human picks option → task resumes; adds `awaiting-direction` + `blocked` task statuses and `paused` WorkflowRun status |

---

## Task Status Legend

| Icon | Meaning |
|---|---|
| 🟒 | Not Started |
| 🚀 | In Progress |
| ⏸️ | Blocked |
| ✅ | Done |
