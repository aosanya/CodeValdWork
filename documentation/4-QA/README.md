# 4 — QA

## Overview

This section covers testing strategy, acceptance criteria, and quality assurance for CodeValdWork.

---

## Index

| Document | Description |
|---|---|
| [Integration Tests](#integration-tests) | ArangoDB backend integration test guide |
| [Test Matrix](#test-matrix) | Per-task acceptance criteria |

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
task_test.go                    ← TaskManager lifecycle + status transition tests
storage/
  arangodb/
    arangodb_test.go            ← ArangoDB backend integration tests
internal/
  grpcserver/
    server_test.go              ← gRPC handler unit tests (mock backend)
```

Integration tests that require an external ArangoDB instance must use `t.Skip()` when `WORK_ARANGO_ENDPOINT` is not set.

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

---

## Integration Tests

### Overview

The file `storage/arangodb/arangodb_test.go` contains the full integration test
suite for the ArangoDB backend. Tests cover the complete task lifecycle against a
real ArangoDB instance, including CRUD operations, status updates, agency
isolation, and filter queries.

All tests **skip automatically** when `WORK_ARANGO_ENDPOINT` is not set in the
environment — they never fail in CI environments without ArangoDB.

### Environment Setup

Copy `.env.example` to `.env` and fill in your ArangoDB connection details:

```bash
cp .env.example .env
```

The minimum required variables for integration tests:

| Variable | Default | Description |
|---|---|---|
| `WORK_ARANGO_ENDPOINT` | _(none — skip)_ | ArangoDB HTTP endpoint (e.g. `http://localhost:8529`) |
| `WORK_ARANGO_USER` | `root` | ArangoDB username |
| `WORK_ARANGO_PASSWORD` | _(empty)_ | ArangoDB password |

Example `.env` for local development (also matches the committed `.env` values):

```bash
WORK_ARANGO_ENDPOINT=http://host.docker.internal:8529
WORK_ARANGO_DATABASE=codevaldwork
WORK_ARANGO_USER=root
WORK_ARANGO_PASSWORD=rootpassword
```

> **Note**: The integration tests create a **unique database per test run**
> (`test_{TestName}_{timestamp}`) to guarantee isolation. Databases are not
> cleaned up automatically — re-run or drop them manually if disk space
> becomes a concern.

### Running Integration Tests

```bash
# Load .env and run only the ArangoDB integration tests
make test-arango

# Or set the variable inline
WORK_ARANGO_ENDPOINT=http://localhost:8529 go test -v -race ./storage/arangodb/

# Run against the dev container's ArangoDB (host.docker.internal)
WORK_ARANGO_ENDPOINT=http://host.docker.internal:8529 \
WORK_ARANGO_USER=root \
WORK_ARANGO_PASSWORD=rootpassword \
go test -v -race ./storage/arangodb/
```

### Test Coverage

| Test | What it covers |
|---|---|
| `TestArangoDB_CreateGet_RoundTrip` | Create task → read back; all fields match, status starts as `pending` |
| `TestArangoDB_CreateUpdate_ValidTransition` | Update status and `assigned_to`; confirms field persistence |
| `TestArangoDB_DeleteThenGet_NotFound` | Delete task → `ErrTaskNotFound` on subsequent get |
| `TestArangoDB_GetNonExistent_NotFound` | Get unknown ID → `ErrTaskNotFound` |
| `TestArangoDB_DuplicateCreate_AlreadyExists` | Create with duplicate ID → `ErrTaskAlreadyExists` |
| `TestArangoDB_ListTasks_SameAgency` | Create 3 tasks → list returns all 3 |
| `TestArangoDB_ListTasks_AgencyIsolation` | Agency A tasks never appear in agency B list |
| `TestArangoDB_ListTasks_FilterByStatus` | Status filter returns only matching tasks |
| `TestArangoDB_ListTasks_EmptyResult` | Empty agency returns `[]Task{}` not `nil` |

### Skip Behaviour

When `WORK_ARANGO_ENDPOINT` is unset, all integration tests produce:

```
--- SKIP: TestArangoDB_CreateGet_RoundTrip (0.00s)
    arangodb_test.go:XX: WORK_ARANGO_ENDPOINT not set — skipping ArangoDB integration test
```

This means `make test` (unit tests) always passes in any environment without
requiring a database.

---

## Test Matrix

See the `### Acceptance Tests` section of each task in
[../3-SofwareDevelopment/mvp-details/task-management.md](../3-SofwareDevelopment/mvp-details/task-management.md)
for the full acceptance criteria per MVP task.
