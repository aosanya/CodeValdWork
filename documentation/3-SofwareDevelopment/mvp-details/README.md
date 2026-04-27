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

---

## Task Status Legend

| Icon | Meaning |
|---|---|
| 🟒 | Not Started |
| 🚀 | In Progress |
| ⏸️ | Blocked |
| ✅ | Done |
