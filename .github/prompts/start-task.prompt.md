````prompt
---
agent: agent
---

# Start New Task

> ⚠️ **Before starting a new task**, run `CodeValdWork/.github/prompts/finish-task.prompt.md` to ensure any in-progress task is properly completed and merged first.

Follow the **mandatory task startup process** for CodeValdWork tasks:

## Task Startup Process (MANDATORY)

1. **Select the next task**
   - Check `documentation/3-SofwareDevelopment/mvp.md` for the task list and current status
   - Check `documentation/3-SofwareDevelopment/mvp-details/` for detailed specs per topic
   - Check `documentation/1-SoftwareRequirements/requirements.md` for unimplemented functional requirements
   - Prefer foundational tasks (e.g., `TaskManager` scaffold) before dependent ones

2. **Read the specification**
   - Re-read the relevant FR(s) in `documentation/1-SoftwareRequirements/requirements.md`
   - Re-read the corresponding section in `documentation/2-SoftwareDesignAndArchitecture/architecture.md`
   - Read the task spec in `documentation/3-SofwareDevelopment/mvp-details/{topic-file}.md`
   - Understand how the task fits into the `TaskManager` interface

3. **Create feature branch from `main`**
   ```bash
   cd /workspaces/CodeValdWork
   git checkout main
   git pull origin main
   git checkout -b feature/WORK-XXX_description
   ```
   Branch naming: `feature/WORK-XXX_description` (lowercase with underscores)

4. **Read project guidelines**
   - Review `.github/instructions/rules.instructions.md`
   - Key rules: interface-first, context propagation, no hardcoded storage, godoc on all exports

5. **Create a todo list**
   - Break the task into actionable steps
   - Use the manage_todo_list tool to track progress
   - Mark items in-progress and completed as you go

## Pre-Implementation Checklist

Before starting:
- [ ] Relevant FRs and architecture sections re-read
- [ ] Feature branch created: `feature/WORK-XXX_description`
- [ ] Existing files checked — no duplicate types in `types.go` or `errors.go`
- [ ] Understood which file(s) to modify (`task.go`, `errors.go`, `types.go`, `storage/arangodb/`, `internal/grpcserver/`)
- [ ] Todo list created for this task

## Development Standards

- **No hardcoded storage** — inject via `Backend` interface
- **Every exported symbol** must have a godoc comment
- **Every exported method** takes `context.Context` as the first argument
- **Errors** must be typed sentinel values — not raw `errors.New` strings scattered across files

## Git Workflow

```bash
# Create feature branch
git checkout -b feature/WORK-XXX_description

# Regular commits during development
git add .
git commit -m "WORK-XXX: Descriptive message"

# Build validation before merge
go build ./...           # must succeed
go test -v -race ./...   # must pass
go vet ./...             # must show 0 issues
golangci-lint run ./...  # must pass

# Merge when complete
git checkout main
git merge feature/WORK-XXX_description --no-ff
git branch -d feature/WORK-XXX_description
```
````
