# 4 — QA

## Overview

This section covers testing strategy, acceptance criteria, and quality assurance for CodeValdWork.

---

## Testing Standards

All contributions must satisfy:

| Check | Command | Requirement |
|---|---|---|
| Build | `go build ./...` | Must succeed — no compilation errors |
| Unit tests | `go test -v -race ./...` | All tests green; no data races |
| Static analysis | `go vet ./...` | 0 issues |
| Linting | `golangci-lint run ./...` | Must pass |
| Coverage | `go test -coverprofile=coverage.out ./...` | Target ≥ 80% on exported functions |

---

## Test Structure Convention

Tests live alongside source files using Go's standard `_test.go` convention:

```
task_test.go                    ← TaskManager + status transition tests
storage/
  arangodb/
    arangodb_test.go            ← ArangoDB backend integration tests
internal/
  grpcserver/
    server_test.go              ← gRPC handler unit tests (mock backend)
```

Integration tests that require an external ArangoDB instance must use `t.Skip()` when `WORK_ARANGO_ENDPOINT` is not set.

---

## Acceptance Criteria per Task

See the `### Acceptance Tests` section of each task in [../3-SofwareDevelopment/mvp-details/task-management.md](../3-SofwareDevelopment/mvp-details/task-management.md) for the full test matrix per MVP task.

---

## Running Tests

```bash
# Unit tests only (no external services required)
make test

# ArangoDB integration tests (requires WORK_ARANGO_ENDPOINT)
make test-arango

# Full test suite with coverage report
make cover
```
