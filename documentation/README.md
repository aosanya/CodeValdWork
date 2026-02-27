# CodeValdWork — Documentation

## Overview

**CodeValdWork** is a Go library and gRPC microservice that provides task lifecycle management for [CodeValdCortex](../CodeValdCortex/README.md) — the enterprise multi-agent AI orchestration platform.

It is the authoritative service for creating, tracking, and transitioning AI agent **Tasks**.

---

## Documentation Index

| Document | Description |
|---|---|
| [1-SoftwareRequirements/](1-SoftwareRequirements/README.md) | What the service must do — scope, FR, NFR, introduction |
| [2-SoftwareDesignAndArchitecture/](2-SoftwareDesignAndArchitecture/README.md) | Design decisions, storage model, TaskManager interface |
| [3-SofwareDevelopment/](3-SofwareDevelopment/README.md) | MVP task list, implementation details per topic |
| [4-QA/](4-QA/README.md) | Testing strategy, acceptance criteria, QA standards |

### Key Files

| File | Description |
|---|---|
| [1-SoftwareRequirements/requirements.md](1-SoftwareRequirements/requirements.md) | Functional requirements, NFR, resolved open questions |
| [2-SoftwareDesignAndArchitecture/architecture.md](2-SoftwareDesignAndArchitecture/architecture.md) | Core design decisions, data model, API interface |
| [3-SofwareDevelopment/mvp.md](3-SofwareDevelopment/mvp.md) | MVP task list and status |
| [3-SofwareDevelopment/mvp-details/](3-SofwareDevelopment/mvp-details/README.md) | Per-topic task specifications |

---

## Quick Summary

- **Language**: Go
- **Transport**: gRPC + protobuf
- **Storage**: ArangoDB
- **Consumer**: CodeValdCortex (via gRPC); also registered with CodeValdCross
- **Core entity**: `Task` — assigned to AI agents, progresses through a defined lifecycle
- **Lifecycle**: `pending → in_progress → completed | failed | cancelled`
