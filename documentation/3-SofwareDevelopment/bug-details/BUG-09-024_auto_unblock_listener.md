# BUG-09-024 — Auto-unblock listener for BLOCKED tasks when dependencies complete

**Status:** ✅ Fixed (2026-06-01, main)
**Severity:** Medium — operators currently have to manually re-assign every blocked task once its dependency completes; pipeline doesn't self-progress
**Owner:** CodeValdWork
**Estimated effort:** ~1 day (event subscriber + a couple of edge traversals)
**Source finding:** [`/4-QA/agencies/utility-app-builder/bugs/09-mvp-sf-pipeline-findings.md`](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/bugs/09-mvp-sf-pipeline-findings.md)

## Resolution

Implemented the inverse trigger as outlined in the fix plan below. CodeValdWork now self-subscribes to `work.task.completed` and drives the auto-unblock cascade through the dispatcher.

- New [`assignment_unblock.go`](../../../assignment_unblock.go) — `taskManager.UnblockDependents` walks every inbound `depends_on` edge from the completed task, re-evaluates each dependent's full `depends_on` set via the existing `findUnmetDependencies` helper, then flips `blocked → pending` and re-publishes both `work.task.status.changed` and `work.task.assigned` (using the cached `assigned_to` agent) for any dependent whose gating is fully satisfied. Dependents with no cached assignee are left blocked.
- [`task.go`](../../../task.go) — `UnblockDependents` added to the `TaskManager` interface so the dispatcher can invoke it without reaching into the concrete type.
- [`internal/server/event_dispatcher.go`](../../../internal/server/event_dispatcher.go) — `Dispatch` now routes `codevaldwork.TopicTaskCompleted` to a new `handleWorkTaskCompleted` that unmarshals `TaskCompletedPayload`, filters to `TerminalStatus == completed` (failed/cancelled deliberately leave dependents blocked — separate follow-up will decide cascade-fail vs release), then calls `UnblockDependents`. Self-loop receipts are safe — the `status != blocked` check inside the manager makes the handler idempotent.
- [`events.go`](../../../events.go) — `ConsumedTopics` now returns `TopicTaskCompleted` alongside `TopicTaskUpdate`, plus a comment documenting the self-subscription pattern.
- [`internal/config/config.go`](../../../internal/config/config.go) — `WORK_SUBSCRIBE_TOPICS` default extended with `work.task.completed` so the registrar advertises the self-subscription on startup.
- Tests: [`assignment_unblock_test.go`](../../../assignment_unblock_test.go) covers happy path (blocked → pending + republished events), the partial-deps case (still blocked when another dep is unmet), the no-assignee case (stays blocked, nothing to dispatch), idempotency on redelivery, and unknown-task error.

Cascade unblocks (001 completes → 002 unblocks → 002 completes → 003 unblocks) fall out of the dispatcher firing per-event — no recursion needed inside the manager.

---

## Context

[`assignment.go:AssignTask`](../../../assignment.go) (landed this session) now correctly sets `status=blocked` and suppresses `work.task.assigned` when a task has an outbound `depends_on` edge to a non-terminal source. Verified live: MVP-SF-002 → blocked when assigned while MVP-SF-001 is pending. Cascades correctly (003 → blocked because 002 is blocked).

Missing: the **inverse trigger**. When MVP-SF-001 eventually completes, something needs to:

1. Find every task with an inbound `depends_on` edge from MVP-SF-001 (tasks blocked by it).
2. For each, re-check ALL outbound `depends_on` edges.
3. If every dependency is terminal AND the task is `TaskStatusBlocked` AND it has an `assigned_to` edge: flip `blocked → pending` and publish `work.task.assigned` with the cached assignee.
4. Otherwise leave it blocked.

## Reproducer / current behaviour

```bash
# 1. Assign MVP-SF-002 while MVP-SF-001 is PENDING
curl -X PUT ${BASE}/work/${AGENCY}/tasks/${MVP_SF_002_ID}/assignee/${WORK_AGENT_ID} ...
# → MVP-SF-002.status = blocked (correct)

# 2. Run MVP-SF-001 to completion
curl -X PUT ${BASE}/work/${AGENCY}/tasks/${MVP_SF_001_ID}/assignee/${WORK_AGENT_ID} ...
# → MVP-SF-001.status = completed

# 3. Expected: MVP-SF-002 auto-flips to pending and fires work.task.assigned
# 3. Observed: MVP-SF-002 stays at status=blocked forever; operator must
#    re-PUT the assignee to trigger AssignTask again
```

## Concrete fix plan

### Step 1 — Wire a `work.task.completed` listener inside CodeValdWork

CodeValdWork already self-subscribes to events (its NotifyEvent gRPC handler is used for `ai.task.completed`). Add a handler for `work.task.completed`:

1. In [`internal/server/event_dispatcher.go`](../../../internal/server/event_dispatcher.go), add a case:
   ```go
   case codevaldwork.TopicTaskCompleted:
       go s.handleTaskCompleted(ctx, req.GetPayload())
   ```
2. Implement `handleTaskCompleted` in a new file `assignment_unblock.go` next to `assignment.go`:
   ```go
   func (m *taskManager) UnblockDependents(ctx context.Context, agencyID, completedTaskID string) error {
       edges, err := m.TraverseRelationships(ctx, agencyID, completedTaskID, RelLabelDependsOn, DirectionInbound)
       if err != nil { return err }
       for _, e := range edges {
           dependentID := e.FromID
           dependent, err := m.GetTask(ctx, agencyID, dependentID)
           if err != nil { continue }
           if dependent.Status != TaskStatusBlocked { continue }
           unmet, err := m.findUnmetDependencies(ctx, agencyID, dependentID)
           if err != nil { continue }
           if len(unmet) > 0 { continue }
           assignedEdges, err := m.TraverseRelationships(ctx, agencyID, dependentID, RelLabelAssignedTo, DirectionOutbound)
           if err != nil || len(assignedEdges) == 0 { continue }
           agentID := assignedEdges[0].ToID
           if err := m.setTaskStatus(ctx, agencyID, dependentID, TaskStatusPending); err != nil { continue }
           m.publish(ctx, TopicTaskStatusChanged, agencyID, TaskStatusChangedPayload{
               TaskID: dependentID, From: TaskStatusBlocked, To: TaskStatusPending,
           })
           agent, _ := m.GetAgent(ctx, agencyID, agentID)
           m.publish(ctx, TopicTaskAssigned, agencyID, TaskAssignedPayload{
               TaskID: dependentID, AgentID: agentID,
               RoleName: agent.RoleName, TaskCode: dependent.TaskName,
               Title: dependent.Title, Description: dependent.Description,
           })
       }
       return nil
   }
   ```

### Step 2 — Reuse `findUnmetDependencies` and `setTaskStatus`

Both helpers exist in `assignment.go` as file-private. Either:
- Move both to package level, or
- Co-locate the unblock logic in `assignment.go`.

### Step 3 — Subscribe at startup

CodeValdWork must register as a consumer of `work.task.completed` with Cross. Check [`internal/registrar/registrar.go`](../../../internal/registrar/registrar.go) — it likely has a `Consumes` list. Add `TopicTaskCompleted`.

**Self-loop caution:** CodeValdWork publishes `work.task.completed` too. Mitigations:
- `handleTaskCompleted` is idempotent (the `status != blocked` check is a no-op for already-pending tasks).
- Add a check: ignore the event if `payload.Source == "codevaldwork"`.

## Edge cases to handle

1. **Cascade unblocks** — 001 completes → 002 unblocks → fires assigned → 002 eventually completes → 003 unblocks. The dispatcher fires per-event; this happens naturally.
2. **Failed dependencies** — if 001 completes with `status=failed`, should 002 unblock or cascade-fail? Current `findUnmetDependencies` uses `isTerminalStatus` which treats `failed` as satisfied. Decision needed: treat only `completed` as "satisfied"; `failed`/`cancelled` cascade-fail the dependent. File a sub-task.
3. **Blocked but no assignee** — leave blocked; nothing to dispatch.
4. **PubSub redelivery** — idempotency check prevents double-publish.

## Verification once fixed

```bash
# Setup: deps wired (gittesting.json import sets up 002 → depends_on 001)
# 1. Assign 002 → blocked
# 2. Assign 001 → runs, eventually completed
# 3. Wait ~5s
# 4. Check 002 — should be pending or in_progress, not blocked
# 5. Check pubsub for a second work.task.assigned for 002
```

Add a Work-7 step to /09: "assign a task that depends on a running one; observe it lands in `blocked`; after the dependency completes, observe within 10s the dependent flips to `pending`/`in_progress` and gets a fresh `work.task.assigned` event published with the cached assignee."

## Dependencies on other gaps

- Implementation depends on BUG-09-023 (proto3 replace-all) being fixed. The unblock path uses the `setTaskStatus` helper that works around that gap; a proper FieldMask makes it cleaner.
- Doesn't unblock BUG-09-020 but multiplies its impact: with auto-unblock working, more tasks run in sequence and the flush race becomes the dominant failure mode.
