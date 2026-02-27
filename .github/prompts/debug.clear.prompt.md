````prompt
---
agent: agent
---

# Debug Print Removal Prompt

You are a cleanup assistant that removes debug prints that were added for troubleshooting.

## Task Identification

First, identify the current task ID from:
1. Git branch name (e.g., `feature/WORK-012_task_status` → Task ID: `WORK-012`)
2. Active file context or user mention
3. Search for TODO comments mentioning task IDs

## Debug Print Removal Guidelines

### What to Remove

Remove all debug prints with the identified task ID prefix:

#### Go
```go
// Remove lines like:
log.Printf("[WORK-012] ...")
// And their TODO comments:
// TODO: Remove debug prints for WORK-012 after issue is resolved
```

### Search Strategy

1. **Search for TODO comments** with task ID
2. **Search for log statements** with `[TASK-ID]` prefix
3. **Verify context** — ensure it's debug code, not production logging
4. **Remove cleanly** — preserve surrounding code structure

### What to Keep

**DO NOT** remove:
- Production logging (without task ID prefix)
- Error handling that logs to production systems
- Standard application startup/shutdown logs

### Execution Steps

1. **Identify Task ID** from branch name
2. **Search for debug prints** with that task ID using grep/search
3. **Review each occurrence** to confirm it's debug code
4. **Remove prints and TODO comments** while preserving code structure
5. **Verify syntax** after removal

## Search Commands

```bash
# Find all debug prints for task
grep -rn "\[WORK-012\]" . --include="*.go"

# Find TODO comments
grep -rn "TODO.*WORK-012" . --include="*.go"
```

## Validation After Removal

```bash
go build ./...         # Must succeed
go vet ./...           # Must show 0 issues
go test -v -race ./... # Must pass
```
````
