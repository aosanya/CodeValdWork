---
status: 📋 Draft (2026-06-03)
owner: CodeValdWork
scope: Workflow failure recovery — automatic retry, AI classification, human-in-the-loop direction via JSON form
source: Research session 2026-06-03; observed infinite retry loop in qa-scenario-09-vscode-20260603-090300
---

# FEAT-20260603-003 — Workflow Failure Recovery & Human-in-the-Loop Direction

## Overview

When a task step fails, the current system retries blindly until `FailurePipelineBudget` is
exhausted — there is no mechanism to pause, classify the error, or ask a human what to do next.
This feature adds a three-phase recovery ladder:

1. **Retry** — automatic retries up to N (configurable per WorkPlan)
2. **Classify** — AI decides: transient (retry again) vs. requires-human (escalate)
3. **Human direction** — system pauses, sends a JSON-described form to the human; human picks
   an option or writes a custom instruction; system resumes with that context injected

The direction form is described entirely in JSON so any frontend (web, mobile, CLI, notification
widget) can render it from the same event payload.

---

## Observed failure pattern (motivation)

From `qa-scenario-09-vscode-20260603-090300`:

```
work.task.assigned  → ai.task.started → ... todos complete ...
work.todo.completed  { title: "Compile and verify branch", status: "failed" }
work.task.status.changed  { From: "in_progress", To: "failed" }
work.task.completed  { TerminalStatus: "failed" }
work.task.assigned   ← task re-assigned immediately (automatic retry)
... same cycle repeats 3× ...
```

Root cause: the "Compile and verify branch" todo has stub instructions
(`"Compile gate — emit an empty actions block []."`) that the AI agent cannot fulfill.
The system retries with identical input and gets identical failure — no learning, no escalation.

---

## Recovery ladder

### Phase 1 — Automatic retry

When a task fails, CodeValdWork checks the task's `max_recovery_runs` property
(defaults to 3, configurable per WorkPlan). If `recovery_runs_used < max_recovery_runs`,
the task is retried by re-emitting `work.task.assigned` (existing `FailurePipelineBudget`
mechanism). Each retry increments `recovery_runs_used`.

### Phase 2 — AI classification

After the last retry is consumed, rather than immediately escalating to a human, the system
emits a `work.task.classify-failure` event. CodeValdAI receives it and publishes
`work.task.failure-classified` with one of two outcomes:

| `failure_type` | Meaning | Next step |
|---|---|---|
| `transient` | Temporary issue (network, rate limit, environment); the same plan should eventually work | Retry once more (budget exception) |
| `requires-human` | Structural problem (bad instruction, stub step, missing toolchain); the AI cannot self-recover | Escalate to human direction |

If AI classification itself fails (e.g. AI service down), default to `requires-human`.

### Phase 3 — Human direction

On `requires-human`:

1. Task status → `awaiting-direction`
2. WorkflowRun status → `paused`
3. Dependent tasks block (do not advance past `pending`)
4. CodeValdWork emits `work.task.needs-direction` carrying a JSON direction form
5. Cross routes the event; notification is sent to the human agent; frontend renders the form
6. Human submits their selection
7. CodeValdWork receives `work.task.direction` and re-dispatches the task with the human's
   instruction injected into the task description

---

## Human direction form — JSON schema

The `form` field inside `work.task.needs-direction` is a self-contained description of
the interaction. Any platform can render it without knowing CodeVald internals.

```json
{
  "title": "Task needs your direction",
  "description": "The step '{{todo_title}}' has failed {{failure_count}} time(s).",
  "failure_output": "{{last_failure_reason}}",
  "question": "How should the agent proceed?",
  "options": [
    {
      "id": "retry-with-instructions",
      "label": "Retry with new instructions",
      "description": "Tell the agent what to do differently.",
      "requires_input": true,
      "input_label": "Instructions",
      "input_placeholder": "e.g. The compile step is a stub — skip it and mark as completed.",
      "suggestions": ["{{ai_suggestion_1}}", "{{ai_suggestion_2}}", "{{ai_suggestion_3}}"]
    },
    {
      "id": "skip",
      "label": "Skip this step",
      "description": "Mark the failed todo as skipped and continue.",
      "requires_input": false
    },
    {
      "id": "mark-blocked",
      "label": "Mark task as blocked",
      "description": "Pause indefinitely — note what is blocking.",
      "requires_input": true,
      "input_label": "Blocker",
      "input_placeholder": "e.g. Flutter toolchain not installed in CI."
    },
    {
      "id": "cancel",
      "label": "Cancel task",
      "description": "Terminate this task and fail the workflow run cleanly.",
      "requires_input": false
    }
  ],
  "allow_freetext": true,
  "freetext_label": "Other — write your own direction"
}
```

**Key invariants:**
- `options` is AI-generated per failure; the list is not hardcoded in the platform
- `suggestions` inside `retry-with-instructions` are the AI's recommended instructions,
  presented as quick-fill text the human can accept or override
- `allow_freetext: true` always appears so the human can write anything the AI didn't anticipate
- The rendered form **must** show `failure_output` so the human sees the actual error before deciding

---

## New events

### `work.task.needs-direction`

Emitted by CodeValdWork when a task exhausts retries and classification returns `requires-human`.

```json
{
  "task_id": "string",
  "workflow_run_id": "string",
  "agency_id": "string",
  "failure_count": 3,
  "last_failure_reason": "string",
  "form": { ... }
}
```

### `work.task.failure-classified`

Emitted by CodeValdAI in response to `work.task.classify-failure`.

```json
{
  "task_id": "string",
  "workflow_run_id": "string",
  "failure_type": "transient | requires-human",
  "reasoning": "string"
}
```

### `work.task.direction`

Emitted by the frontend/notification handler after the human submits the form.
CodeValdWork subscribes and re-dispatches the task.

```json
{
  "task_id": "string",
  "workflow_run_id": "string",
  "selected_option": "retry-with-instructions | skip | mark-blocked | cancel",
  "instructions": "string",
  "directed_by": "agent_id | human_user_id",
  "directed_at": "RFC3339"
}
```

### `work.task.classify-failure`

Emitted by CodeValdWork to trigger AI classification. CodeValdAI subscribes.

```json
{
  "task_id": "string",
  "workflow_run_id": "string",
  "failure_count": 3,
  "last_failure_reason": "string",
  "task_description": "string",
  "failed_todo_title": "string",
  "failed_todo_instructions": "string"
}
```

---

## New statuses

### Task

Extend the existing `TaskStatus` enum in `models.go`:

| New value | Meaning |
|---|---|
| `awaiting-direction` | Retries exhausted; waiting for a human to submit the direction form |
| `blocked` | Human chose `mark-blocked`; paused until unblocked |

### WorkflowRun

Extend `WorkflowRunStatus` in `models.go`:

| New value | Meaning |
|---|---|
| `paused` | At least one task is `awaiting-direction` or `blocked`; run cannot complete |

---

## State transitions

```
Task:
  in_progress → failed         (existing — todo fails, parent task fails)
  failed → awaiting-direction  (new — after N retries + classification = requires-human)
  awaiting-direction → in_progress  (new — human submits direction)
  awaiting-direction → blocked      (new — human chooses mark-blocked)
  awaiting-direction → cancelled    (new — human chooses cancel)

WorkflowRun:
  in_progress → paused         (new — any task moves to awaiting-direction or blocked)
  paused → in_progress         (new — all paused tasks receive direction / are unblocked)
  paused → failed              (new — human cancels a task in awaiting-direction)
```

---

## Direction handling — what happens after submission

| `selected_option` | System action |
|---|---|
| `retry-with-instructions` | Append `instructions` to task description. Set task → `in_progress`. Re-emit `work.task.assigned`. |
| `skip` | Mark the specific `TaskTodo` as `skipped`. Re-evaluate parent task completion (may complete). |
| `mark-blocked` | Set task → `blocked`. Store blocker note on task. WorkflowRun stays `paused`. |
| `cancel` | Set task → `cancelled`. Emit `work.task.cancelled`. `maybeCompleteWorkflowRun` evaluates → likely `failed`. |

---

## Configuration

| Property | Location | Default | Meaning |
|---|---|---|---|
| `max_recovery_runs` | WorkPlan schema | `3` | Retry budget before classification |
| `classification_timeout_s` | WorkPlan schema | `60` | Seconds to wait for AI classification before defaulting to `requires-human` |
| `direction_timeout_s` | WorkPlan schema | `0` (no timeout) | If set, auto-cancels a task that has been `awaiting-direction` too long |

---

## Schema changes

`WorkPlan` — new properties:

```go
{Name: "max_recovery_runs",        Type: types.PropertyTypeInt},
{Name: "classification_timeout_s", Type: types.PropertyTypeInt},
{Name: "direction_timeout_s",      Type: types.PropertyTypeInt},
```

`Task` — new properties:

```go
{Name: "recovery_runs_used", Type: types.PropertyTypeInt},
{Name: "blocker_note",       Type: types.PropertyTypeString},
{Name: "direction_history",  Type: types.PropertyTypeString}, // JSON array of past directions
```

`WorkflowRun` — add `paused` to the existing `terminal_event` state machine (non-terminal).

---

## Implementation plan

| Phase | Work | Effort |
|---|---|---|
| 1 — Schema | Add new statuses and properties above | 0.5 day |
| 2 — Retry ladder | Wire `recovery_runs_used` increment on task failure; gate re-assignment on budget | 0.5 day |
| 3 — Classification | Emit `work.task.classify-failure`; subscribe to `work.task.failure-classified`; default transient/requires-human | 1 day |
| 4 — Direction form | Build `FormDefinition` struct; AI generates `options` + `suggestions` payload; emit `work.task.needs-direction` | 1 day |
| 5 — Direction handler | Subscribe to `work.task.direction`; route by `selected_option`; re-dispatch task | 1 day |
| 6 — WorkflowRun pause | Add `paused` status; transition on task → `awaiting-direction`; un-pause on all tasks resolved | 0.5 day |
| 7 — Tests | Unit: each transition; integration: full retry-ladder scenario | 1 day |

---

## Open gaps

- **`FormDefinition` struct**: needs a canonical Go struct in CodeValdWork (or SharedLib?) so CodeValdAI can build valid form payloads without copy-pasting field names
- **Notification delivery**: `work.task.needs-direction` must route to the correct human; mechanism TBD (email, Slack, in-app)
- **`skip` implementation**: "skip a specific todo" requires a new `TaskTodo.status = skipped` path and re-running `maybeCompleteParentTask`
- **`blocked` unblock trigger**: what event/endpoint un-blocks a `blocked` task?
- **Stub compile gate bug**: the `"Compile gate — emit an empty actions block []."` instruction is meaningless — CodeValdAI always fails it. Fix: the AI should treat `actions == []` as no-op success ([BUG-20260603-003 AI](../../../../CodeValdCross/documentation/3-SofwareDevelopment/prioritization.md)) — or remove stub compile-gate todos from QA scenarios entirely

---

## Related work

- [task-failure-modes.md](task-failure-modes.md) — TF-N catalogue; `maybeCompleteWorkflowRun`
- [FEAT-20260602-003 — WorkflowRun status state machine](FEAT-20260602-003_workflow_run_status_state_machine.md)
- [FEAT-20260602-008 — WorkflowRun cancel](FEAT-20260602-008_workflow_run_cancel.md)
- [Cross — pipeline-failure-handling](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/pipeline-failure-handling.md)
- [Cross — FEAT-20260602-005 — failure pipelines via synthesized success](../../../../CodeValdCross/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260602-005_failure_pipelines_synthesized_success.md)
