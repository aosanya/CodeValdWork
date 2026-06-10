# BUG-20260610-002 — `applyAITaskStatus` hardcodes Task transitions; CodeValdAgency event_flows + work plans are not enforced at runtime; legacy handlers still fire

**Status:** 📋 Open
**Severity:** High — silent flow violation. Planner / non-developer AgentRuns satisfy the parent Task without doing the actual work; downstream gates (review, completion cascade, dependent unblock) fire on a lie. Legacy work plans from earlier imports continue triggering even when the active publication doesn't declare them.
**Owner:** CodeValdWork (primary); CodeValdAgency (cross-service — owns the imported flow data and must expose it for runtime enforcement)
**Estimated effort:** ~2–3 days. Phase 1 narrow fix in Work; Phase 2 cross-service: Agency exposes a "step lookup by handler + topic + active publication" RPC; Phase 3: prune / disable work plans not present in the active publication.
**Source finding:** Session 2026-06-10, scenario [12 — utility-app-builder Planning Flow](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/12/). Assigned `MVP-SF-001` (Task ID `1c320ad8-829b-45d0-947b-8b0eb203adc4`) → planner-assigned-handler ran → planner AgentRun (`16f2f87f-…`) completed → CodeValdWork transitioned the parent Task PENDING → IN_PROGRESS → COMPLETED with **no decompose todos, no split children, no review, no actual work product**.

## Architectural framing

The on-disk flow files (e.g. [`flows_feature-development.json`](../../../../CodeValdImplementations/Agencies/utility-app-builder/flows_feature-development.json)) are **input** to the agency import — they are NOT the runtime source of truth. Once an agency.json is imported and promoted (via `POST /agency/{id}/import`), the persisted state in CodeValdAgency becomes the single source of truth at runtime:

- `event_flows` entities — every flow step (trigger, consumer, handler, declared `emits_topics`, declared status transitions, on-error transitions).
- `work plans` entities — the (`trigger_topic`, `payload_condition`, `handler_service`, `agent_code`) bindings that map an event onto a handler.
- The **active `AgencyPublication`** — defines which version of the above is currently authoritative.

The flow file on disk and the runtime entities diverge whenever the import is incomplete, the import drops a field, or a prior publication's plans are not retired when a new publication is promoted. CodeValdWork is currently unaware of any of this — it applies status transitions on its own internal hardcoded mapping with zero consultation of the active publication's `event_flows`.

## Problem

For `utility-app-builder`, the active publication's `event_flows` (imported from `flows_feature-development.json`) declares **every** Task state transition explicitly:

- Step 1 — `task.assigned` entry, status stays `pending`.
- Step 1.1 — planner-assigned-handler evaluates and emits `task.request-decompose` **or** `task.request-split`. The parent Task remains `pending`.
- Step 1.1.1.1 — on split path, CodeValdWork transitions parent to `split`.
- Decompose path — parent transitions to `completed` only via `maybeCompleteParentTask` once every decomposed `TaskTodo` is terminal.
- Split path — parent transitions to `completed` only when every child subtask reaches a terminal state (subtask-aggregation rule).

Nothing in those event_flows says "any AgentRun completing flips the parent Task to COMPLETED". Yet that is exactly what happens, because Work bypasses the publication entirely.

A secondary problem rides on top: **work plans from earlier imports are not retired** when a new publication is promoted. They remain `enabled=true` and continue triggering handlers that the current publication's event_flows don't declare, so even if Work later starts consulting event_flows, stale handlers will still fire and produce spurious transitions.

## Evidence

Live PubSub trace (utility-app-builder, 2026-06-10T11:33–11:35Z, single Work-1 assignment):

```
task.assigned    TaskID=1c320ad8 RoleName=Developer AgentID=e9494417  ← planner entry
task.started     TaskID=1c320ad8 RunID=ff1326b3 AgentID=503c84a3      ← planner AgentRun starts
task.failed      RunID=ff1326b3 Reason="huggingface: read stream: context canceled"
task.classify-failure failure_count=3
task.started     TaskID=1c320ad8 RunID=16f2f87f                       ← retry
task.completed   TaskID=1c320ad8 RunID=16f2f87f                       ← planner AgentRun completed
task.status.changed TaskID=1c320ad8 From=in_progress To=completed     ← Work flipped parent Task
task.completed   TaskID=1c320ad8 TerminalStatus=completed CompletedAt=2026-06-03T09:26:35Z
review.passed    task_id=1c320ad8
```

After the dust settled:

```
$ curl /work/utility-app-builder/tasks/1c320ad8-…/todos
TaskTodo entities for MVP-SF-001: 0          ← never decomposed
$ curl /pubsub/utility-app-builder/events?topic=task.todo
task.todo events: 0                          ← no work produced
$ curl /work/utility-app-builder/tasks/1c320ad8-…
"status": "TASK_STATUS_COMPLETED"            ← but parent flipped to done
```

The planner emitted `task.request-decompose` (2 of them across retries) and `task.request-split` (1) — those are the legitimate flow outputs — but the parent Task should still be `pending` waiting on the developer fan-out. Instead the planner's own AgentRun completion cascaded into a parent-Task completion.

## Root cause

[`CodeValdWork/internal/server/event_dispatcher.go:235-244`](../../../internal/server/event_dispatcher.go) — `applyAITaskStatus`:

```go
func (d *TaskEventDispatcher) applyAITaskStatus(ctx context.Context, topic string, p aiTaskPayload) {
    var nextStatus codevaldwork.TaskStatus
    switch topic {
    case topicTaskStarted:   nextStatus = codevaldwork.TaskStatusInProgress
    case topicTaskCompleted: nextStatus = codevaldwork.TaskStatusCompleted
    case topicTaskFailed:    nextStatus = codevaldwork.TaskStatusFailed
    }
    ...
}
```

The mapping is unconditional. The dispatcher does not consult:

1. Which agency-flow step produced the AgentRun (planner step? developer decompose step? split-handler? final assembly?).
2. Whether the flow declares a parent-Task transition at this point at all.
3. Whether the AgentRun was the **last** run anchored to the parent Task (the decompose path explicitly delegates parent completion to `maybeCompleteParentTask`, which gates on all child todos terminal).

`handleAITaskStatus` does have an "AI-originated payloads carry RunID" guard ([line 215](../../../internal/server/event_dispatcher.go#L215)) to suppress Work's own self-receipts, but does not differentiate AgentRun kinds.

## Fix plan

**Phase 1 — Verify the import is complete; only the active publication's plans run.**

Before any code change in Work, audit the import side:

- Confirm every flow step in the source flow file produces a corresponding `event_flow` entity under the active `AgencyPublication`. Re-import [`flows_feature-development.json`](../../../../CodeValdImplementations/Agencies/utility-app-builder/flows_feature-development.json) and diff source-step-count vs imported-entity-count. The current import path (CodeValdAgency `import_server.go`) needs to round-trip declared status transitions and on-error mappings, not just (trigger, handler) tuples.
- List every `enabled=true` work plan under the agency and reject any whose handler/topic is not declared in the active publication's event_flows. These are legacy plans from earlier imports — they must be marked `enabled=false` (or moved to a retired collection) when a new publication is promoted.
- Add a CodeValdAgency RPC `LookupFlowStep(agencyID, publicationVersion, handler_code, source_topic) → FlowStep` so downstream services (Work, AI) can answer "what does the active publication say should happen at this step?"

**Phase 2 — CodeValdWork enforces the active publication.**

`applyAITaskStatus` must not apply a status transition unless the active publication's event_flows declare one for the (originating AgentRun's flow step, observed topic). The AgentRun row already carries enough context — it was kicked off by a work plan whose `handler_service`/`agent_code`/source topic are all known — so Work can resolve the step via the new `LookupFlowStep` RPC.

For each flow step that *is* declared to terminate the parent, the existing named completers handle it correctly today:

- Decompose path → `maybeCompleteParentTask` (gates on all todos terminal).
- Split path → subtask-aggregation rule.

`applyAITaskStatus` should drop its hardcoded switch and either (a) no-op when the step declares no transition, or (b) call into the named completer the step declares.

**Phase 3 — Clear stale `completed_at` on Task reset.**

Secondary corruption observed: on `/dev-rollback-workflow`'s self-heal reset (or any future Phase-1 reset), `properties.completed_at` is not cleared. When the Task legitimately completes later, the event surfaces a stale `2026-06-03T09:26:35Z` timestamp on a 2026-06-10 run.

Either zero `completed_at` in the reset writer, or compute `completed_at = max(properties.completed_at, transition_time)` in `UpdateTask` and overwrite when transitioning out of a terminal status.

**Phase 4 — Promotion lifecycle: retire previous-publication work plans.**

CodeValdAgency's `PromoteDraft` should mark every `enabled=true` work plan that is not present in the new publication as `enabled=false` (or move to a `retired_work_plans` collection). Today's behaviour leaves all historical work plans live, so an agency that has been re-imported N times has N-1 generations of overlapping handlers competing for the same trigger.

## Verification

After fix, repro Work-1 in scenario 12 against a clean MVP-SF-001:

1. Re-import [`agency.json`](../../../../CodeValdImplementations/Agencies/utility-app-builder/agency.json) (auto_promote=true). Confirm only the active publication's work plans are `enabled=true`; any legacy plans not present in the latest import are `enabled=false`.
2. Diff source flow steps vs imported `event_flows` entities — count must match per workflow.
3. Assign MVP-SF-001 to developer-01.
4. Wait for the planner AgentRun to complete.
5. Parent Task **must remain `pending`** (or `in_progress` if a flow step explicitly declares it) — not `completed`.
6. `task.completed (TerminalStatus=completed)` for the parent Task is **not** emitted at this point.
7. Parent only transitions when either (a) all decomposed todos terminal, or (b) all split children terminal.
8. `completedAt` on the eventual legitimate completion matches today's wall clock, not 2026-06-03.

## Dependencies

- [BUG-20260610-001](BUG-20260610-001_rollback-leaks-todos-and-edges.md) — closed/invalid, but the `completed_at` secondary finding here is a direct neighbour and should land in the same fix series.
- Cross-service: depends on CodeValdAgency exposing a `LookupFlowStep` RPC (or equivalent query) so Work can resolve "this AgentRun came from step X of the active publication". Today's import (FEAT-20260609-002) persists the flows, but the lookup affordance for downstream services is missing.
- Cross-service: depends on CodeValdAgency's `PromoteDraft` retiring previous-publication work plans.
