# 2 — Software Design & Architecture

## Overview

This section captures the **how** — design decisions, data models, component architecture, and technical constraints for CodeValdWork.

---

## Index

| Document | Description |
|---|---|
| [architecture.md](architecture.md) | Core design decisions, storage model, TaskManager interface, CodeValdCortex integration |

---

## Key Design Decisions at a Glance

| Decision | Choice | Rationale |
|---|---|---|
| Transport | gRPC + protobuf | Consistent with all CodeVald services |
| Storage | ArangoDB | Consistent with CodeValdCortex persistence layer; survives container restarts |
| Interface | Single `TaskManager` | Task management is a single cohesive concern |
| Backend injection | `Backend` interface | Caller provides the storage backend — library is backend-agnostic |
| Status validation | In `taskManager.UpdateTask` | Centralised — backend never sees invalid transitions |
| Service registration | CodeValdCross heartbeat | Standard service discovery pattern across the CodeVald platform |

---

## Component Architecture

```
github.com/aosanya/CodeValdWork    ← root package (library entry point)
├── task.go                         # TaskManager interface + implementation + Backend interface
├── errors.go                       # ErrTaskNotFound, ErrTaskAlreadyExists, ErrInvalidStatusTransition
├── types.go                        # Task, TaskStatus, TaskPriority, TaskFilter value types
├── storage/
│   └── arangodb/                   # ArangoDB Backend implementation
│       └── arangodb.go
├── internal/
│   ├── grpcserver/                 # gRPC handler wrapping TaskManager
│   │   ├── server.go
│   │   └── errors.go               # Domain error → gRPC status code mapping
│   └── registrar/                  # CodeValdCross heartbeat registrar
│       └── registrar.go
└── cmd/
    └── server/                     # Binary entry point
        └── main.go
```
