# BUG-20260609-001 — Drop `work.` domain prefix from published topic names (system-wide rename)

**Status:** 📋 Open
**Severity:** High — re-keys the entire CodeValdWork → consumers dispatch graph. Until this lands the new [`flows_planning.json`](../../../../CodeValdImplementations/Agencies/utility-app-builder/flows_planning.json) and scenario-12 QA cannot match real events, and the system-wide Domain-event-ownership rule is being reversed without code changes
**Owner:** CodeValdWork (primary — most publishers and the highest event volume); coordinated paired item [CodeValdAI/BUG-20260609-001](../../../../CodeValdAI/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_ai_domain_prefix.md) for `ai.*`; trigger-topic updates land in [CodeValdImplementations/Agencies/utility-app-builder/agency.json](../../../../CodeValdImplementations/Agencies/utility-app-builder/agency.json); SharedLib eventreceiver topic construction is upstream
**Estimated effort:** ~1.5 days (audit + rename + agency.json sweep + auto-memory retirement; held back by needing a single coordinated change with CodeValdAI to avoid a half-renamed event graph in production)
**Source finding:** `/document_issues` run during scenario-12 setup on 2026-06-09. Mismatch surfaced: live agency uses `work.task.assigned` / `ai.task.decompose`; the new `flows_planning.json` and scenario-12 docs use bare `task.assigned` / `task.request-decompose` / `task.todo`. User decision was to retire the domain-prefix rule and converge on the un-prefixed names

---

## Problem

CodeValdWork publishes every domain event with a `work.` prefix:

| Today's topic | Producer | New topic |
|---|---|---|
| `work.task.assigned` | task-assignment write path | `task.assigned` |
| `work.task.todo` | per-todo persistence (one event per `TaskTodo` created) | `task.todo-persisted` |
| `work.todo.dispatched` | dispatch handler that fans out TaskTodos to AI | `todo.dispatched` |
| `work.todo.completed` | TaskTodo terminal state | `todo.completed` |
| `work.task.completed` | Task terminal state (or deferred completion roll-up) | `task.completed` |
| `work.task.failed` | Task failed state | `task.failed` |
| `work.task.needs-direction` | retry-ladder escalation | `task.needs-direction` |
| `work.task.direction` | direction-given relay (when AI Failure Reviewer answers) | `task.direction` |
| `work.run.timeout` | watchdog stale-run sweep | `run.timeout` |
| `work.pipeline.requested` | start-pipeline function trigger | `pipeline.requested` |

(See [models.go](../../../models.go), [internal/](../../../internal/), and the live agency.json work plan `trigger_topic` values for the canonical list.)

The new direction — confirmed by user decision on 2026-06-09 during `/document_issues` — is to drop the `work.` / `ai.` / `git.` / `comm.` prefixes everywhere. Topic names become intent-keyed:

- `task.assigned` — entity-lifecycle: a task has been assigned
- `task.todo-persisted` — a todo has been persisted (was `work.task.todo`)
- `todo.dispatched` — a todo is being dispatched to a consumer
- `task.request-decompose` — a planner is requesting that a task be decomposed
- `task.request-split` — a planner is requesting that a task be split into child tasks

This conflicts with the now-retired `feedback_domain_event_rule.md` auto-memory ("each service only publishes its own domain"). The memory file will be retired as part of this work.

Until the rename lands:

- `flows_planning.json` step-1 trigger `task.assigned` never matches the real `work.task.assigned` publish, so no part of the planning flow can be exercised against the live agency.
- Scenario-12 QA pubsub assertions all fail (every `events?topic=task.assigned&...` query returns empty).
- WorkPlan rows in agency.json have `trigger_topic: work.task.assigned` which would also need to change.
- The system carries two incompatible naming conventions in parallel, masked only by the fact the new convention isn't wired up to anything.

## Evidence

```text
$ python3 -c "
import sys, json, urllib.request, base64
auth = 'Basic ' + base64.b64encode(b'codevald:chanuisosnnau@geme').decode()
req = urllib.request.Request('http://codevaldcross:8081/agency/utility-app-builder/work-plans', headers={'Authorization': auth})
plans = json.load(urllib.request.urlopen(req)).get('entities', [])
for code in ['planner-assigned-handler','developer-assigned-handler','work-todo-handler']:
    p = next((x for x in plans if x['properties'].get('code') == code), None)
    if p: print(f'{code}: trigger_topic = {p[\"properties\"][\"trigger_topic\"]}')
"
planner-assigned-handler: trigger_topic = work.task.assigned          ← old prefix
developer-assigned-handler: trigger_topic = ai.task.decompose         ← old prefix
work-todo-handler: trigger_topic = work.todo.dispatched               ← old prefix

$ grep -nc 'task.assigned\|task.request-decompose' \
    CodeValdImplementations/Agencies/utility-app-builder/flows_planning.json
4   # new (un-prefixed) convention
```

The disjoint between live agency (`work.*` / `ai.*`) and new `flows_planning.json` (un-prefixed) is the visible symptom. The root is that producers and trigger_topics were never updated when the new convention was chosen.

## Root cause

The Domain event ownership rule ("each service publishes only its own domain") prefixed every emitted topic with the publisher's service domain. It made graph-traversal trivial — given a topic, the producer is known by inspection — but cluttered every topic name with redundant information when the topic itself already carries intent (`task.assigned` *is* about a task; the producer is whoever owns task lifecycle).

The user reversed the rule on 2026-06-09. The rename has not yet been applied to the publishers, the trigger_topics, the QA docs, or the auto-memory.

## Fix plan (phased)

### Phase 1 — SharedLib eventreceiver (upstream)

[`github.com/aosanya/CodeValdSharedLib/eventreceiver`](../../../) constructs topic names from the type's StorageCollection + operation. Today the construction prepends the service domain. Phase-1 change:

- Drop the `<service-domain>.` prefix from auto-constructed topic names.
- Add a backward-compat shim: emit *both* the old (`work.task.assigned`) and the new (`task.assigned`) topics for two weeks, gated on an env flag. Required so we can land Phase 2 (CodeValdWork code rename) and Phase 3 (agency.json trigger_topic rename) independently without a flag-day.

### Phase 2 — CodeValdWork rename

Audit every Publish / emit call site. Files to touch (non-exhaustive — Phase 1 should be merged first so test fixtures continue passing):

- `internal/pubsub/` — topic constants
- `internal/dispatch/` — topic mapping
- `internal/task/` — assign, complete, fail paths
- `internal/todo/` — dispatch, complete paths
- `internal/workflow_run/` — watchdog, timeout, cancel
- `internal/pipeline/` — start-pipeline trigger

Replace the constants per the table in the Problem section. Update unit tests and event-receiver assertions.

### Phase 3 — agency.json trigger_topics

For every utility-app-builder WorkPlan in [agency.json](../../../../CodeValdImplementations/Agencies/utility-app-builder/agency.json) whose `trigger_topic` is in the rename table, update to the new value:

- `planner-assigned-handler`: `work.task.assigned` → `task.assigned`
- `developer-assigned-handler`: `ai.task.decompose` → `task.request-decompose` (companion CodeValdAI bug)
- `work-todo-handler`: `work.todo.dispatched` → `todo.dispatched`
- (audit the rest)

Reimport via the new auto-promote path ([FEAT-20260609-003 — CodeValdAgency](../../../../CodeValdAgency/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260609-003_auto_draft_on_import.md)). Update [`flows_planning.json`](../../../../CodeValdImplementations/Agencies/utility-app-builder/flows_planning.json) to match the canonical names (no changes likely — it already uses un-prefixed names, just confirm trigger/emit pairs).

### Phase 4 — Scenario-12 QA

The QA docs already use un-prefixed names — once Phases 1–3 land, the pubsub assertions in [12/work-01..03](../../../../CodeValdCross/documentation/4-QA/agencies/utility-app-builder/12/) will match real events for the first time. No QA edits required, but a smoke pass to confirm is part of verification.

### Phase 5 — Retire the auto-memory

Remove or rewrite [`feedback_domain_event_rule.md`](~/.claude/projects/-workspaces-CodeVald-AIProject-CodeValdAgency/memory/feedback_domain_event_rule.md). Add a replacement memory describing the new intent-keyed naming convention.

### Phase 6 — Remove the SharedLib backward-compat shim

Two weeks after Phase 2 ships in prod, turn off the dual-emit flag from Phase 1. Audit Cross for any consumer still subscribed to the legacy topic names.

## Verification

- [ ] All unit tests in CodeValdWork pass with the new topic constants.
- [ ] Scenario-12 QA pubsub assertions match real events (`?topic=task.assigned&...` returns the just-assigned task) on a live deploy.
- [ ] No consumer is still subscribed to `work.*` topics after the dual-emit window closes (audit via `GET /services/registry?agencyId=...`).
- [ ] `feedback_domain_event_rule.md` retired; replacement memory describes intent-keyed naming.
- [ ] Companion AI rename ([CodeValdAI/BUG-20260609-001](../../../../CodeValdAI/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_ai_domain_prefix.md)) shipped in parallel.

## Dependencies

- **Hard depends on** SharedLib Phase 1 (dual-emit shim) — otherwise the rename has to be a flag-day across multiple services.
- **Paired with** [CodeValdAI/BUG-20260609-001](../../../../CodeValdAI/documentation/3-SofwareDevelopment/bug-details/BUG-20260609-001_drop_ai_domain_prefix.md) — the `ai.task.*` family must rename in the same release window.
- **Soft depends on** [CodeValdAgency/FEAT-20260609-003](../../../../CodeValdAgency/documentation/3-SofwareDevelopment/mvp-details/FEAT-20260609-003_auto_draft_on_import.md) (auto-draft on import) — without it, Phase 3 needs manual draft-promote ceremony for every utility-app-builder reimport during the rollout.

## Risks

- Half-renamed graph: if Phase 2 ships before Phase 3, every CodeValdWork publish goes to a new topic but the WorkPlan trigger_topics still listen to the old one — entire planning flow halts. SharedLib dual-emit (Phase 1) is the safety net; do NOT skip it.
- Frontend hard-coded topics: any frontend file that hard-codes `work.task.*` strings (for stream subscriptions or status banners) needs a sweep too — file a follow-up if found.
- External consumers (if any) subscribed via Cross from outside the CodeVald stack — survey before turning off the dual-emit shim in Phase 6.
