````instructions
# CodeValdWork — AI Agent Development Instructions

## Project Overview

**CodeValdWork** is a **Go library and gRPC microservice** that provides task management for [CodeValdCortex](../CodeValdCortex/README.md) — the enterprise multi-agent AI orchestration platform.

It is the authoritative service for creating, tracking, and transitioning AI agent **Tasks**. CodeValdCross routes task lifecycle events to and from CodeValdWork, and agents consume tasks assigned to them.

**Core Concept**: Each Agency in CodeValdCortex has tasks managed by CodeValdWork. Tasks represent units of work assigned to AI agents. CodeValdWork manages their lifecycle — creation, status transitions, assignment, and completion.

---

## Library Architecture

### 1. Single-Interface Design

The library exposes one top-level interface:

```go
// TaskManager — full task lifecycle management.
type TaskManager interface {
    CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)
    GetTask(ctx context.Context, agencyID, taskID string) (Task, error)
    UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error)
    DeleteTask(ctx context.Context, agencyID, taskID string) error
    ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error)
}
```

### 2. Task Lifecycle

```
pending → in_progress → completed
    │                  → failed
    └──────────────→ cancelled
```

- Tasks start as `pending` on creation
- Agents pick tasks up and move them to `in_progress`
- Tasks complete, fail, or are cancelled
- Status transitions are validated — invalid transitions return `ErrInvalidStatusTransition`

### 3. Storage Backends

The `Backend` interface is the injection point:

| Backend | Package | Purpose |
|---|---|---|
| ArangoDB | `storage/arangodb/` | Production — task documents in ArangoDB collection |

> The caller (CodeValdCortex) passes the chosen `Backend` when calling `NewTaskManager`. **CodeValdWork itself is backend-agnostic.**

---

## Project Structure

```
/workspaces/CodeValdWork/
├── documentation/
│   ├── README.md
│   ├── 1-SoftwareRequirements/
│   │   ├── README.md
│   │   ├── requirements.md
│   │   └── introduction/
│   │       ├── problem-definition.md
│   │       ├── high-level-features.md
│   │       └── stakeholders.md
│   ├── 2-SoftwareDesignAndArchitecture/
│   │   ├── README.md
│   │   └── architecture.md
│   ├── 3-SofwareDevelopment/
│   │   ├── README.md
│   │   ├── mvp.md
│   │   └── mvp-details/
│   │       ├── README.md
│   │       └── task-management.md
│   └── 4-QA/
│       └── README.md
├── .github/
│   ├── copilot-instructions.md
│   ├── instructions/
│   │   └── rules.instructions.md
│   ├── prompts/
│   └── workflows/
│       └── ci.yml
├── go.mod
├── task.go            # TaskManager interface + implementation
├── errors.go          # ErrTaskNotFound, ErrTaskAlreadyExists, ErrInvalidStatusTransition
├── types.go           # Task, TaskStatus, TaskPriority, TaskFilter value types
├── proto/
│   ├── codevaldwork/v1/
│   │   ├── codevaldwork.proto  # Task message definitions
│   │   ├── service.proto       # TaskService gRPC service
│   │   └── errors.proto        # Structured error detail messages
│   └── codevaldcross/v1/
│       └── registration.proto  # OrchestratorService for registration heartbeats
├── gen/go/                     # buf-generated Go stubs (do not edit)
├── storage/
│   └── arangodb/               # ArangoDB Backend implementation
├── internal/
│   ├── grpcserver/             # gRPC handler that wraps TaskManager
│   └── registrar/              # CodeValdCross heartbeat registrar
└── cmd/
    └── server/                 # Binary entry point
```

---

## Developer Workflows

### Build & Test Commands

```bash
# Run all tests with race detector
go test -v -race ./...

# Build check
go build ./...

# Static analysis
go vet ./...

# Format code
go fmt ./...

# Lint
golangci-lint run ./...

# Regenerate proto stubs
buf generate
```

### Task Management Workflow

```bash
# 1. Create feature branch from main
git checkout -b feature/WORK-XXX_description

# 2. Implement changes

# 3. Build validation before merge
go build ./...
go vet ./...
go test -v -race ./...

# 4. Merge when complete
git checkout main
git merge feature/WORK-XXX_description --no-ff
git branch -d feature/WORK-XXX_description
```

---

## Technology Stack

| Component | Choice | Rationale |
|---|---|---|
| Language | Go 1.21+ | Matches CodeValdCortex; native concurrency |
| Transport | gRPC + protobuf | Consistent with all CodeVald services |
| Storage | ArangoDB | Consistent with CodeValdCortex persistence layer |
| Registration | CodeValdCross gRPC | Service discovery and pub/sub routing |

---

## CodeValdCross Registration

CodeValdWork registers with CodeValdCross on startup and sends periodic heartbeats.

| Topic | Direction | Description |
|---|---|---|
| `work.task.created` | Produces | New task created for an agency |
| `work.task.updated` | Produces | Task status or assignment changed |
| `work.task.completed` | Produces | Task reached terminal status |
| `cross.task.requested` | Consumes | Cortex requesting task creation |
| `cross.agency.created` | Consumes | New agency registered — seed task backlog |

---

## When in Doubt

1. **Check documentation first**: `documentation/1-SoftwareRequirements/requirements.md` and `documentation/2-SoftwareDesignAndArchitecture/architecture.md`
2. **Interface before implementation**: define the interface, write tests against the interface, then implement
3. **Inject dependencies**: storage backends are always caller-provided
4. **Write tests for every exported function** — aim for >80% coverage; use table-driven tests
````
