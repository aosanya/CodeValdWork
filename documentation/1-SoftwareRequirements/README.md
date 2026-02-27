# 1 — Software Requirements

## Overview

This section captures everything **what** CodeValdWork must do and **why** — without prescribing how.

---

## Index

| Document | Description |
|---|---|
| [requirements.md](requirements.md) | Functional requirements, non-functional requirements, scope, and open questions |
| [introduction/problem-definition.md](introduction/problem-definition.md) | Problem statement and motivation |
| [introduction/high-level-features.md](introduction/high-level-features.md) | High-level capability summary |
| [introduction/stakeholders.md](introduction/stakeholders.md) | Consumers and stakeholders |

---

## Summary

CodeValdWork is the authoritative task management service for CodeValdCortex agencies. AI agents receive tasks through CodeValdWork and report progress back via status transitions.

### Core Requirements at a Glance

| FR | Requirement |
|---|---|
| FR-001 | Task creation — create a task for an agency with title, description, and priority |
| FR-002 | Task retrieval — get a single task by ID within an agency |
| FR-003 | Task update — modify mutable fields; validate status transitions |
| FR-004 | Task deletion — permanently remove a task by ID |
| FR-005 | Task listing — list tasks for an agency with optional status/priority/assignee filters |
| FR-006 | Status lifecycle — `pending → in_progress → completed | failed | cancelled` |
| FR-007 | CodeValdCross registration — register on startup and heartbeat to CodeValdCross |
| FR-008 | gRPC service — expose all task operations via the `TaskService` gRPC service |
