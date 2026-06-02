// workflow_run_rollback.go — WorkflowRun rollback coordinator and artifact cleanup
// (FEAT-20260602-004).
//
// RollbackWorkflowRun orchestrates the compensation sequence across CodeValdWork's
// own entities. Cross-service legs (AI, Functions, Git, Comm) are stubbed here and
// will be wired when each service implements DELETE /by-workflow-run/{id}.
//
// DeleteWorkflowRunArtifacts is the CodeValdWork leg of the per-service DELETE
// contract: it hard-deletes every Task and TaskTodo anchored by the run ID,
// removes their incident edges, and emits work.task.rolled_back per deleted Task.
package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// RollbackWorkflowRun implements [TaskManager.RollbackWorkflowRun].
// Sequence: rolling_back → compensate artifacts → rolled_back (or rollback_failed).
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

	// Step 2 — cross-service compensation stubs (to be wired once each service
	// exposes DELETE /by-workflow-run/{id}).
	m.stubCompensateComm(ctx, agencyID, runID, reason)
	m.stubCompensateGit(ctx, agencyID, runID)
	m.stubCompensateFunctions(ctx, agencyID, runID)
	m.stubCompensateAI(ctx, agencyID, runID)

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

	// Delete Tasks: remove all incident edges first, then soft-delete the entity.
	for _, task := range tasks {
		m.deleteEntityEdges(ctx, agencyID, task.ID)
		if err := m.dm.DeleteEntity(ctx, agencyID, task.ID); err != nil {
			slog.ErrorContext(ctx, "DeleteWorkflowRunArtifacts: delete task", "task_id", task.ID, "err", err)
		}
		m.publishTaskRolledBack(ctx, agencyID, task.ID, runID)
	}

	// Delete TaskTodos anchored to this run.
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

// ListTaskTodos returns all TaskTodos whose workflow_run_id matches runID.
func (m *taskManager) ListTaskTodos(ctx context.Context, agencyID, runID string) ([]TaskTodo, error) {
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
		if todo.WorkflowRunID == runID {
			out = append(out, todo)
		}
	}
	return out, nil
}

// publishTaskRolledBack emits work.task.rolled_back for observability after a Task is deleted.
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

// ── Cross-service compensation stubs ──────────────────────────────────────────
// Each stub will be replaced with a real gRPC call once the target service
// implements DELETE /by-workflow-run/{id} (FEAT-20260602-004 Phase 2).

func (m *taskManager) stubCompensateComm(ctx context.Context, agencyID, runID, reason string) {
	slog.InfoContext(ctx, "RollbackWorkflowRun: comm compensation stub (not yet implemented)",
		"agency_id", agencyID, "run_id", runID)
}

func (m *taskManager) stubCompensateGit(ctx context.Context, agencyID, runID string) {
	slog.InfoContext(ctx, "RollbackWorkflowRun: git compensation stub (not yet implemented)",
		"agency_id", agencyID, "run_id", runID)
}

func (m *taskManager) stubCompensateFunctions(ctx context.Context, agencyID, runID string) {
	slog.InfoContext(ctx, "RollbackWorkflowRun: functions compensation stub (not yet implemented)",
		"agency_id", agencyID, "run_id", runID)
}

func (m *taskManager) stubCompensateAI(ctx context.Context, agencyID, runID string) {
	slog.InfoContext(ctx, "RollbackWorkflowRun: ai compensation stub (not yet implemented)",
		"agency_id", agencyID, "run_id", runID)
}

