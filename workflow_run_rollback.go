// workflow_run_rollback.go — WorkflowRun rollback coordinator and artifact cleanup
// (FEAT-20260602-004).
//
// RollbackWorkflowRun orchestrates the compensation sequence: it calls each
// cross-service compensation leg (Git, AI, Comm, Functions) via the injected
// RollbackClients, then runs CodeValdWork's own artifact cleanup.
//
// DeleteWorkflowRunArtifacts is the CodeValdWork leg: it resets every Task
// anchored by the run ID to pending (clearing workflow_run_id and completed_at,
// keeping all other task fields and non-run edges intact), hard-deletes every
// TaskTodo anchored to the run, and emits work.task.rolled_back per affected Task.
package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// RollbackWorkflowRun implements [TaskManager.RollbackWorkflowRun].
// Sequence: rolling_back → compensate cross-service artifacts → compensate own artifacts → rolled_back (or rollback_failed).
func (m *taskManager) RollbackWorkflowRun(ctx context.Context, agencyID, runID, reason string) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, agencyID, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.Status == WorkflowRunStatusRollingBack {
		return WorkflowRun{}, ErrRollbackConflict
	}
	if !run.Status.CanTransitionTo(WorkflowRunStatusRollingBack) {
		return WorkflowRun{}, fmt.Errorf("%w: %s → rolling_back", ErrInvalidRunStatusTransition, run.Status)
	}

	// Step 1 — acquire the rolling_back lock.
	rollingRun, err := m.UpdateWorkflowRunStatus(ctx, agencyID, runID, WorkflowRunStatusRollingBack, reason)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("RollbackWorkflowRun: acquire: %w", err)
	}

	// Step 2 — cross-service compensation (event-driven; each service handles its own
	// compensation when it receives the work.run.rolling_back event published above).
	// Direct service-to-service calls are NOT used — all cross-service communication
	// flows through CodeValdCross events. No stubs needed here.

	// Step 3 — CodeValdWork's own artifact cleanup.
	if err := m.DeleteWorkflowRunArtifacts(ctx, agencyID, runID); err != nil {
		var rollbackErr error
		if errors.Is(err, ErrForeignRunDependency) {
			rollbackErr = err
		} else {
			rollbackErr = fmt.Errorf("delete artifacts: %w", err)
		}
		failedRun, ferr := m.UpdateWorkflowRunStatus(ctx, agencyID, runID, WorkflowRunStatusRollbackFailed, rollbackErr.Error())
		if ferr != nil {
			slog.ErrorContext(ctx, "RollbackWorkflowRun: failed to set rollback_failed status", "run_id", runID, "err", ferr)
			return rollingRun, rollbackErr
		}
		return failedRun, rollbackErr
	}

	// Step 4 — finalize.
	finalRun, err := m.UpdateWorkflowRunStatus(ctx, agencyID, runID, WorkflowRunStatusRolledBack, reason)
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("RollbackWorkflowRun: finalize: %w", err)
	}
	return finalRun, nil
}

// DeleteWorkflowRunArtifacts implements [TaskManager.DeleteWorkflowRunArtifacts].
// Tasks are reset to pending (not deleted). TaskTodos are hard-deleted.
func (m *taskManager) DeleteWorkflowRunArtifacts(ctx context.Context, agencyID, runID string) error {
	if _, err := m.GetWorkflowRun(ctx, agencyID, runID); err != nil {
		return err
	}

	tasks, err := m.ListTasks(ctx, agencyID, TaskFilter{WorkflowRunID: runID})
	if err != nil {
		return fmt.Errorf("DeleteWorkflowRunArtifacts: list tasks: %w", err)
	}

	// Guard: check for Tasks in this run that are depended on by Tasks in OTHER runs.
	for _, task := range tasks {
		inbound, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
			AgencyID: agencyID,
			ToID:     task.ID,
			Name:     RelLabelDependsOn,
		})
		if err != nil {
			return fmt.Errorf("DeleteWorkflowRunArtifacts: list inbound deps for %s: %w", task.ID, err)
		}
		for _, rel := range inbound {
			fromTask, err := m.GetTask(ctx, agencyID, rel.FromID)
			if err != nil {
				continue // missing task — not a blocker
			}
			if fromTask.WorkflowRunID != "" && fromTask.WorkflowRunID != runID {
				return fmt.Errorf("%w: task %s is depended on by task %s (run %s)",
					ErrForeignRunDependency, task.ID, fromTask.ID, fromTask.WorkflowRunID)
			}
		}
	}

	// Reset Tasks: status → pending, clear workflow_run_id + completed_at, remove started_task edge.
	// All other task fields and edges (member_of, has_tag, blocks, depends_on) are preserved.
	//
	// BUG-20260610-002 Phase 3 — pass the reset properties directly to UpdateEntity
	// rather than re-serialising via taskToProperties. The latter omits
	// `completed_at` when its Go value is empty (see task_impl_converters.go),
	// so an in-place `taskToProperties(task)` after `task.CompletedAt = ""`
	// silently kept the stale completed_at in storage; the next legitimate
	// completion event then surfaced a stale timestamp (e.g. a 2026-06-03
	// CompletedAt on a 2026-06-10 run).
	now := time.Now().UTC().Format(time.RFC3339)
	for _, task := range tasks {
		m.removeStartedTaskEdge(ctx, agencyID, task.ID)
		if _, err := m.dm.UpdateEntity(ctx, agencyID, task.ID, entitygraph.UpdateEntityRequest{
			Properties: map[string]any{
				"status":          string(TaskStatusPending),
				"workflow_run_id": "",
				"completed_at":    "",
				"updated_at":      now,
			},
		}); err != nil {
			slog.ErrorContext(ctx, "DeleteWorkflowRunArtifacts: reset task", "task_id", task.ID, "err", err)
		}
		m.publishTaskRolledBack(ctx, agencyID, task.ID, runID)
	}

	// Delete TaskTodos anchored to this run (ephemeral decomposition artifacts).
	todos, err := m.ListTaskTodos(ctx, agencyID, runID)
	if err != nil {
		return fmt.Errorf("DeleteWorkflowRunArtifacts: list todos: %w", err)
	}
	for _, todo := range todos {
		m.deleteEntityEdges(ctx, agencyID, todo.ID)
		if err := m.dm.DeleteEntity(ctx, agencyID, todo.ID); err != nil {
			slog.ErrorContext(ctx, "DeleteWorkflowRunArtifacts: delete todo", "todo_id", todo.ID, "err", err)
		}
	}

	return nil
}

// removeStartedTaskEdge removes all inbound started_task edges pointing to taskID.
// There is normally exactly one (from the owning run), but we remove all to stay clean on re-invocation.
func (m *taskManager) removeStartedTaskEdge(ctx context.Context, agencyID, taskID string) {
	rels, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: agencyID,
		ToID:     taskID,
		Name:     RelLabelStartedTask,
	})
	if err != nil {
		slog.ErrorContext(ctx, "removeStartedTaskEdge: list", "task_id", taskID, "err", err)
		return
	}
	for _, rel := range rels {
		if err := m.dm.DeleteRelationship(ctx, agencyID, rel.ID); err != nil {
			slog.ErrorContext(ctx, "removeStartedTaskEdge: delete", "rel_id", rel.ID, "err", err)
		}
	}
}

// deleteEntityEdges removes all incident (from + to) relationships for an entity.
// Errors are logged but do not abort — a missing edge on a to-be-deleted entity is harmless.
func (m *taskManager) deleteEntityEdges(ctx context.Context, agencyID, entityID string) {
	fromRels, _ := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{AgencyID: agencyID, FromID: entityID})
	toRels, _ := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{AgencyID: agencyID, ToID: entityID})
	for _, rel := range append(fromRels, toRels...) {
		if err := m.dm.DeleteRelationship(ctx, agencyID, rel.ID); err != nil {
			slog.ErrorContext(ctx, "deleteEntityEdges: delete relationship", "rel_id", rel.ID, "err", err)
		}
	}
}

// ListTaskTodos returns TaskTodos for the agency, optionally filtered by
// workflowRunID. When workflowRunID is empty all todos for the agency are
// returned (BUG-20260603-005).
func (m *taskManager) ListTaskTodos(ctx context.Context, agencyID, workflowRunID string) ([]TaskTodo, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: agencyID,
		TypeID:   taskTodoTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("ListTaskTodos: %w", err)
	}
	var out []TaskTodo
	for _, e := range entities {
		todo := taskTodoFromEntity(e)
		if workflowRunID == "" || todo.WorkflowRunID == workflowRunID {
			out = append(out, todo)
		}
	}
	return out, nil
}

// publishTaskRolledBack emits work.task.rolled_back for observability after a Task is rolled back.
func (m *taskManager) publishTaskRolledBack(ctx context.Context, agencyID, taskID, runID string) {
	if m.publisher == nil {
		return
	}
	eventbus.SafePublish(ctx, m.publisher, eventbus.Event{
		Topic:    TopicTaskRolledBack,
		AgencyID: agencyID,
		Payload:  TaskRolledBackPayload{TaskID: taskID, WorkflowRunID: runID},
	})
}

