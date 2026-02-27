````prompt
---
agent: agent
---

# Debug a CodeValdWork Issue

## Common Failure Scenarios

### Scenario 1: `CreateTask` Returns Duplicate Error
**Symptom**: `ErrTaskAlreadyExists` returned for a new task ID
**Cause**: Task ID collision in the storage backend, or ArangoDB unique index constraint
**Check**: Verify task ID generation logic; check if `GetTask` returns a result before `CreateTask`

### Scenario 2: `UpdateTask` Status Transition Rejected
**Symptom**: `ErrInvalidStatusTransition` returned unexpectedly
**Cause**: Caller attempting a disallowed transition (e.g., `completed → in_progress`)
**Check**: Review `TaskStatus.CanTransitionTo` logic in `types.go`

### Scenario 3: `ListTasks` Returns Empty for Known Agency
**Symptom**: Tasks exist in DB but `ListTasks` returns an empty slice
**Cause**: `agencyID` filter not applied correctly in storage backend
**Check**: Log the ArangoDB query and verify the `agency_id` field match

### Scenario 4: Context Cancellation Not Respected
**Symptom**: Operation hangs after caller cancels context
**Cause**: Missing `ctx.Err()` check in long-running storage operations
**Check**: Add `ctx.Err()` check in storage backend query loops

### Scenario 5: CodeValdCross Registration Failing
**Symptom**: Service starts but never appears in CodeValdCross registry
**Cause**: `CROSS_GRPC_ADDR` not set, or CodeValdCross not reachable at startup
**Check**: Verify env var and network reachability; check registrar logs

## Debug Print Guidelines

### Prefix Format
All debug prints MUST be prefixed with: `[TASK-ID]`

### Go
```go
log.Printf("[WORK-XXX] Function called: %s with args: %+v", functionName, args)
log.Printf("[WORK-XXX] State before: %+v", state)
log.Printf("[WORK-XXX] Error in operation: %v", err)
```

### Strategic Placement

Add debug prints at:

1. **Function Entry Points** — log function name and key parameters
2. **State Changes** — before and after critical state modifications
3. **Conditional Branches** — log which branch is taken and why
4. **Error Handling** — log errors with context before returning
5. **Return Statements** — log what is being returned for complex functions

### What NOT to Debug
Avoid adding debug prints to simple getters/setters or pure value computations.
````
