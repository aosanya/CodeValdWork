// workflow_run.go — WorkflowRun lifecycle and closure traversal (FEAT-20260601-001).
//
// A WorkflowRun anchors the constellation of Tasks, TaskTodos, edges, and
// cross-service references produced by a single orchestrated execution.
// Its closure is what a future rollback feature compensates as a transaction.
package codevaldwork

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// runNameSuffixBytes is the number of random bytes that feed the
// 6-hex-char suffix in [generateRunName].
const runNameSuffixBytes = 3

// CreateWorkflowRun anchors a new orchestrated execution.
//
// If name is empty, the server generates one of the form
// `pipeline-YYYY-MM-DD-HHMMSS-<6hex>`. If name is set, leading/trailing
// whitespace is rejected (returns [ErrInvalidTask]); case is preserved.
//
// Returns [ErrWorkflowRunNameExists] when a run with the same
// (agencyID, name) pair already exists — names are immutable, so the
// caller should append a discriminator and retry.
func (m *taskManager) CreateWorkflowRun(ctx context.Context, agencyID, name, triggerEvent, initiator string) (WorkflowRun, error) {
	if name != "" && strings.TrimSpace(name) != name {
		return WorkflowRun{}, fmt.Errorf("%w: WorkflowRun.Name must not have leading/trailing whitespace", ErrInvalidTask)
	}
	now := time.Now().UTC()
	if name == "" {
		name = generateRunName(now)
	}
	// CreateEntity does not enforce the schema [UniqueKey] (that is the
	// contract of UpsertEntity). We need exact-match collision detection,
	// not merge semantics, so explicitly look up by name first.
	if existing, err := m.GetWorkflowRunByName(ctx, agencyID, name); err == nil && existing.ID != "" {
		return WorkflowRun{}, ErrWorkflowRunNameExists
	} else if err != nil && !errors.Is(err, ErrWorkflowRunNotFound) {
		return WorkflowRun{}, fmt.Errorf("CreateWorkflowRun: name precheck: %w", err)
	}
	run := WorkflowRun{
		AgencyID:     agencyID,
		Name:         name,
		Status:       WorkflowRunStatusPending,
		TriggerEvent: triggerEvent,
		Initiator:    initiator,
		StartedAt:    now.Format(time.RFC3339),
		CreatedAt:    now.Format(time.RFC3339),
		UpdatedAt:    now.Format(time.RFC3339),
	}
	created, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   agencyID,
		TypeID:     workflowRunTypeID,
		Properties: workflowRunToProperties(run),
	})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("CreateWorkflowRun: %w", err)
	}
	return workflowRunFromEntity(created), nil
}

// generateRunName builds a deterministic-looking but collision-resistant
// label of the form `pipeline-YYYY-MM-DD-HHMMSS-<6hex>`. The 6 hex chars
// come from 3 random bytes — ~16M possibilities, so collisions across
// 1k runs/day are negligible. Falls back to a UTC seconds-based suffix
// if the OS RNG is unavailable.
func generateRunName(now time.Time) string {
	var buf [runNameSuffixBytes]byte
	suffix := ""
	if _, err := rand.Read(buf[:]); err == nil {
		suffix = hex.EncodeToString(buf[:])
	} else {
		suffix = fmt.Sprintf("%06x", now.UnixNano()&0xFFFFFF)
	}
	return fmt.Sprintf("pipeline-%s-%s", now.UTC().Format("2006-01-02-150405"), suffix)
}

// GetWorkflowRun reads a single WorkflowRun entity.
func (m *taskManager) GetWorkflowRun(ctx context.Context, agencyID, runID string) (WorkflowRun, error) {
	e, err := m.dm.GetEntity(ctx, agencyID, runID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return WorkflowRun{}, ErrWorkflowRunNotFound
		}
		return WorkflowRun{}, fmt.Errorf("GetWorkflowRun: %w", err)
	}
	if e.AgencyID != agencyID || e.TypeID != workflowRunTypeID {
		return WorkflowRun{}, ErrWorkflowRunNotFound
	}
	return workflowRunFromEntity(e), nil
}

// ListWorkflowRuns returns every WorkflowRun in the agency, sorted newest first
// by created_at. Returns an empty slice (not an error) when none exist.
//
// When name is non-empty, the result is filtered to runs whose Name field
// matches exactly — at most one row given the schema [UniqueKey] on name.
func (m *taskManager) ListWorkflowRuns(ctx context.Context, agencyID, name string) ([]WorkflowRun, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: agencyID,
		TypeID:   workflowRunTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("ListWorkflowRuns: %w", err)
	}
	out := make([]WorkflowRun, 0, len(entities))
	for _, e := range entities {
		r := workflowRunFromEntity(e)
		if name != "" && r.Name != name {
			continue
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// GetWorkflowRunByName looks up a single run by its unique (agency, name)
// pair. Returns [ErrWorkflowRunNotFound] when no match exists.
func (m *taskManager) GetWorkflowRunByName(ctx context.Context, agencyID, name string) (WorkflowRun, error) {
	if name == "" {
		return WorkflowRun{}, fmt.Errorf("%w: WorkflowRun.Name is required", ErrInvalidTask)
	}
	runs, err := m.ListWorkflowRuns(ctx, agencyID, name)
	if err != nil {
		return WorkflowRun{}, err
	}
	if len(runs) == 0 {
		return WorkflowRun{}, ErrWorkflowRunNotFound
	}
	return runs[0], nil
}

// LinkTaskToRun writes the started_task edge from the run to a task.
func (m *taskManager) LinkTaskToRun(ctx context.Context, agencyID, runID, taskID string) error {
	if _, err := m.GetWorkflowRun(ctx, agencyID, runID); err != nil {
		return err
	}
	if _, err := m.GetTask(ctx, agencyID, taskID); err != nil {
		return err
	}
	_, err := m.CreateRelationship(ctx, agencyID, Relationship{
		Label:  RelLabelStartedTask,
		FromID: runID,
		ToID:   taskID,
	})
	return err
}

// LinkTodoToRun writes the started_todo edge from the run to a todo.
func (m *taskManager) LinkTodoToRun(ctx context.Context, agencyID, runID, todoID string) error {
	if _, err := m.GetWorkflowRun(ctx, agencyID, runID); err != nil {
		return err
	}
	if _, err := m.GetTaskTodo(ctx, agencyID, todoID); err != nil {
		return err
	}
	_, err := m.CreateRelationship(ctx, agencyID, Relationship{
		Label:  RelLabelStartedTodo,
		FromID: runID,
		ToID:   todoID,
	})
	return err
}

// GetWorkflowRunClosure walks the started_task / has_todo / assigned_to /
// depends_on edges reachable from the run and returns the full set of
// entities and edges encountered. Edges whose endpoints land outside the
// closure are still included — the rollback feature needs them to plan
// compensating actions on neighbours.
func (m *taskManager) GetWorkflowRunClosure(ctx context.Context, agencyID, runID string) (WorkflowRunClosure, error) {
	run, err := m.GetWorkflowRun(ctx, agencyID, runID)
	if err != nil {
		return WorkflowRunClosure{}, err
	}

	closure := WorkflowRunClosure{
		Run:            run,
		AgentRunIDs:    append([]string(nil), run.AgentRunIDs...),
		FunctionJobIDs: append([]string(nil), run.FunctionJobIDs...),
		BranchNames:    append([]string(nil), run.BranchNames...),
	}

	taskIDs := map[string]struct{}{}
	todoIDs := map[string]struct{}{}
	edgeKeys := map[string]struct{}{}

	addEdge := func(rel Relationship) {
		if rel.ID == "" {
			rel.ID = rel.FromID + "→" + rel.Label + "→" + rel.ToID
		}
		if _, seen := edgeKeys[rel.ID]; seen {
			return
		}
		edgeKeys[rel.ID] = struct{}{}
		closure.Edges = append(closure.Edges, rel)
	}

	// Step 1 — started_task edges from the run.
	startedTaskEdges, err := m.TraverseRelationships(ctx, agencyID, runID, RelLabelStartedTask, DirectionOutbound)
	if err != nil {
		return WorkflowRunClosure{}, fmt.Errorf("GetWorkflowRunClosure: started_task: %w", err)
	}
	for _, e := range startedTaskEdges {
		addEdge(e)
		taskIDs[e.ToID] = struct{}{}
	}

	// Step 2 — started_todo edges from the run (some producers link todos
	// directly without going through their parent task).
	startedTodoEdges, err := m.TraverseRelationships(ctx, agencyID, runID, RelLabelStartedTodo, DirectionOutbound)
	if err != nil {
		return WorkflowRunClosure{}, fmt.Errorf("GetWorkflowRunClosure: started_todo: %w", err)
	}
	for _, e := range startedTodoEdges {
		addEdge(e)
		todoIDs[e.ToID] = struct{}{}
	}

	// Step 3 — for each task, walk has_todo, assigned_to, and depends_on
	// (both directions). Tag and member_of edges are not part of the
	// rollback closure but might be added in a later revision.
	for taskID := range taskIDs {
		todoEdges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelHasTodo, DirectionOutbound)
		if err == nil {
			for _, e := range todoEdges {
				addEdge(e)
				todoIDs[e.ToID] = struct{}{}
			}
		}
		assignedEdges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelAssignedTo, DirectionOutbound)
		if err == nil {
			for _, e := range assignedEdges {
				addEdge(e)
			}
		}
		dependsOut, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelDependsOn, DirectionOutbound)
		if err == nil {
			for _, e := range dependsOut {
				addEdge(e)
			}
		}
		dependsIn, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelDependsOn, DirectionInbound)
		if err == nil {
			for _, e := range dependsIn {
				addEdge(e)
			}
		}
	}

	// Step 4 — resolve entities. Tasks first, sorted by created_at for
	// stable output, then todos. Missing entities are skipped (defensive —
	// a deleted task should not fail the closure read).
	tasks := make([]Task, 0, len(taskIDs))
	for id := range taskIDs {
		t, err := m.GetTask(ctx, agencyID, id)
		if err != nil {
			continue
		}
		tasks = append(tasks, t)
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].CreatedAt < tasks[j].CreatedAt })
	closure.Tasks = tasks

	todos := make([]TaskTodo, 0, len(todoIDs))
	for id := range todoIDs {
		td, err := m.GetTaskTodo(ctx, agencyID, id)
		if err != nil {
			continue
		}
		todos = append(todos, td)
	}
	sort.Slice(todos, func(i, j int) bool {
		if todos[i].ParentTaskID != todos[j].ParentTaskID {
			return todos[i].ParentTaskID < todos[j].ParentTaskID
		}
		return todos[i].Ordinality < todos[j].Ordinality
	})
	closure.Todos = todos

	return closure, nil
}

// UpdateWorkflowRunStatus transitions a WorkflowRun to a new lifecycle status.
// Allowed transitions: pending→in_progress, in_progress→completed,
// in_progress→failed, failed→rolled_back. All others return
// [ErrInvalidRunStatusTransition].
func (m *taskManager) UpdateWorkflowRunStatus(ctx context.Context, agencyID, runID string, newStatus WorkflowRunStatus, reason string) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, agencyID, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if !run.Status.CanTransitionTo(newStatus) {
		return WorkflowRun{}, fmt.Errorf("%w: %s → %s", ErrInvalidRunStatusTransition, run.Status, newStatus)
	}
	now := time.Now().UTC()
	run.Status = newStatus
	run.UpdatedAt = now.Format(time.RFC3339)
	if newStatus == WorkflowRunStatusInProgress && run.StartedAt == "" {
		run.StartedAt = run.UpdatedAt
	}
	if newStatus == WorkflowRunStatusCompleted || newStatus == WorkflowRunStatusFailed || newStatus == WorkflowRunStatusRolledBack {
		run.CompletedAt = run.UpdatedAt
	}

	props := workflowRunToProperties(run)
	if reason != "" {
		props["failure_reason"] = reason
	}
	updated, err := m.dm.UpdateEntity(ctx, agencyID, runID, entitygraph.UpdateEntityRequest{Properties: props})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("UpdateWorkflowRunStatus: %w", err)
	}
	result := workflowRunFromEntity(updated)

	m.publishRunStatusEvent(ctx, agencyID, result, now, reason)
	return result, nil
}

// publishRunStatusEvent fires the appropriate work.run.* event after a status transition.
func (m *taskManager) publishRunStatusEvent(ctx context.Context, agencyID string, run WorkflowRun, now time.Time, reason string) {
	if m.publisher == nil {
		return
	}
	var topic string
	var payload any
	switch run.Status {
	case WorkflowRunStatusInProgress:
		topic = TopicRunInProgress
		payload = WorkflowRunInProgressPayload{WorkflowRunID: run.ID, StartedAt: run.StartedAt}
	case WorkflowRunStatusCompleted:
		durationMs := int64(0)
		if run.StartedAt != "" {
			if started, err := time.Parse(time.RFC3339, run.StartedAt); err == nil {
				durationMs = now.UnixMilli() - started.UnixMilli()
			}
		}
		topic = TopicRunCompleted
		payload = WorkflowRunCompletedPayload{WorkflowRunID: run.ID, CompletedAt: run.CompletedAt, DurationMs: durationMs}
	case WorkflowRunStatusFailed:
		topic = TopicRunFailed
		payload = WorkflowRunFailedPayload{WorkflowRunID: run.ID, FailedAt: run.CompletedAt, FailureReason: reason}
	case WorkflowRunStatusRolledBack:
		topic = TopicRunRolledBack
		payload = WorkflowRunRolledBackPayload{WorkflowRunID: run.ID, RolledBackAt: run.CompletedAt}
	default:
		return
	}
	eventbus.SafePublish(ctx, m.publisher, eventbus.Event{
		Topic:    topic,
		AgencyID: agencyID,
		Payload:  payload,
	})
}

// workflowRunToProperties serialises a WorkflowRun for storage.
func workflowRunToProperties(r WorkflowRun) map[string]any {
	props := map[string]any{
		"name":           r.Name,
		"status":         string(r.Status),
		"trigger_event":  r.TriggerEvent,
		"initiator":      r.Initiator,
		"notes":          r.Notes,
		"terminal_event": r.TerminalEvent,
		"started_at":     r.StartedAt,
		"completed_at":   r.CompletedAt,
		"created_at":     r.CreatedAt,
		"updated_at":     r.UpdatedAt,
	}
	if len(r.AgentRunIDs) > 0 {
		props["agent_run_ids"] = append([]string(nil), r.AgentRunIDs...)
	}
	if len(r.FunctionJobIDs) > 0 {
		props["function_job_ids"] = append([]string(nil), r.FunctionJobIDs...)
	}
	if len(r.BranchNames) > 0 {
		props["branch_names"] = append([]string(nil), r.BranchNames...)
	}
	return props
}

// workflowRunFromEntity reconstructs a WorkflowRun from an entitygraph Entity.
func workflowRunFromEntity(e entitygraph.Entity) WorkflowRun {
	r := WorkflowRun{
		ID:            e.ID,
		AgencyID:      e.AgencyID,
		Name:          entitygraph.StringProp(e.Properties, "name"),
		Status:        WorkflowRunStatus(entitygraph.StringProp(e.Properties, "status")),
		TriggerEvent:  entitygraph.StringProp(e.Properties, "trigger_event"),
		Initiator:     entitygraph.StringProp(e.Properties, "initiator"),
		Notes:         entitygraph.StringProp(e.Properties, "notes"),
		TerminalEvent: entitygraph.StringProp(e.Properties, "terminal_event"),
		StartedAt:     entitygraph.StringProp(e.Properties, "started_at"),
		CompletedAt:   entitygraph.StringProp(e.Properties, "completed_at"),
		CreatedAt:     entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:     entitygraph.StringProp(e.Properties, "updated_at"),
	}
	r.AgentRunIDs = stringSliceProp(e.Properties, "agent_run_ids")
	r.FunctionJobIDs = stringSliceProp(e.Properties, "function_job_ids")
	r.BranchNames = stringSliceProp(e.Properties, "branch_names")
	return r
}

// stringSliceProp accepts both native []string (in-memory fakeDataManager)
// and the JSON-decoded []any form (ArangoDB backend).
func stringSliceProp(props map[string]any, key string) []string {
	v, ok := props[key]
	if !ok {
		return nil
	}
	switch xs := v.(type) {
	case []string:
		return append([]string(nil), xs...)
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
