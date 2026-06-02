// workflow_run_cancel.go — Mid-flight WorkflowRun cancel (FEAT-20260602-008).
//
// CancelWorkflowRun handles the synchronous half of the cancel flow: it flips
// the run to the transient cancelling state, persists the cancellation
// envelope (cancelled_by, cancel_reason, cancelling_until), cascades
// work.task.cancelled to every non-terminal Task anchored by the run, and
// publishes work.run.cancelling.
//
// FinalizeWorkflowRunCancellation is the deferred half: after the quiesce
// deadline elapses (driven by the gRPC handler's goroutine), it transitions
// the run from cancelling → cancelled and publishes work.run.cancelled.
// Both halves are idempotent and safe to call multiple times.
package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// CancelWorkflowRun implements [TaskManager.CancelWorkflowRun]. See the
// interface for the contract.
func (m *taskManager) CancelWorkflowRun(ctx context.Context, agencyID, runID, reason, cancelledBy string, quiesceDeadline time.Time) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, agencyID, runID)
	if err != nil {
		return WorkflowRun{}, err
	}

	// Idempotency: a repeated cancel on an already-cancelling run returns the
	// stored envelope without shifting the deadline or re-firing events.
	if run.Status == WorkflowRunStatusCancelling {
		return run, nil
	}

	if run.Status != WorkflowRunStatusInProgress {
		return WorkflowRun{}, fmt.Errorf("%w: status=%s", ErrCannotCancelTerminalRun, run.Status)
	}

	now := time.Now().UTC()
	run.Status = WorkflowRunStatusCancelling
	run.UpdatedAt = now.Format(time.RFC3339)
	run.CancelledBy = cancelledBy
	run.CancelReason = reason
	run.CancellingUntil = quiesceDeadline.UTC().Format(time.RFC3339)

	updated, err := m.dm.UpdateEntity(ctx, agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: workflowRunToProperties(run),
	})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("CancelWorkflowRun: %w", err)
	}
	result := workflowRunFromEntity(updated)

	// Cascade: flip every non-terminal Task anchored by the run to cancelled
	// and emit work.task.cancelled per task. Failures cascading individual
	// tasks are logged but do not abort the cancel — the run-level signal is
	// the authoritative quiesce trigger; per-service subscribers handle each
	// task cancellation idempotently.
	m.cascadeTaskCancellation(ctx, agencyID, runID, reason)

	// Publish the run-level quiesce signal.
	m.publishRunStatusEvent(ctx, agencyID, result, now, reason)
	return result, nil
}

// FinalizeWorkflowRunCancellation implements
// [TaskManager.FinalizeWorkflowRunCancellation]. See the interface for the
// contract.
func (m *taskManager) FinalizeWorkflowRunCancellation(ctx context.Context, agencyID, runID string) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, agencyID, runID)
	if err != nil {
		return WorkflowRun{}, err
	}

	// Idempotency: if someone else already finalized (or the run was never
	// cancelling), return the current state without effect.
	if run.Status != WorkflowRunStatusCancelling {
		return run, nil
	}

	now := time.Now().UTC()
	run.Status = WorkflowRunStatusCancelled
	run.UpdatedAt = now.Format(time.RFC3339)
	run.CompletedAt = run.UpdatedAt

	updated, err := m.dm.UpdateEntity(ctx, agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: workflowRunToProperties(run),
	})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("FinalizeWorkflowRunCancellation: %w", err)
	}
	result := workflowRunFromEntity(updated)

	m.publishRunStatusEvent(ctx, agencyID, result, now, result.CancelReason)
	return result, nil
}

// cascadeTaskCancellation flips every non-terminal Task whose workflow_run_id
// matches runID to cancelled and emits work.task.cancelled per affected task.
// Best-effort: per-task failures are logged and do not abort the cascade.
func (m *taskManager) cascadeTaskCancellation(ctx context.Context, agencyID, runID, reason string) {
	tasks, err := m.ListTasks(ctx, agencyID, TaskFilter{WorkflowRunID: runID})
	if err != nil {
		slog.ErrorContext(ctx, "CancelWorkflowRun: list tasks for cascade", "run_id", runID, "err", err)
		return
	}
	for _, t := range tasks {
		if isTerminalStatus(t.Status) {
			continue
		}
		if err := m.cancelTask(ctx, agencyID, t, reason); err != nil {
			slog.ErrorContext(ctx, "CancelWorkflowRun: cancel task", "task_id", t.ID, "run_id", runID, "err", err)
		}
	}
}

// cancelTask transitions a single Task to cancelled and publishes
// work.task.cancelled. Returns nil even if the state-machine rejects the
// transition (handled by the caller); a real persistence error is surfaced.
func (m *taskManager) cancelTask(ctx context.Context, agencyID string, task Task, reason string) error {
	if !task.Status.CanTransitionTo(TaskStatusCancelled) {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	task.Status = TaskStatusCancelled
	task.UpdatedAt = now
	if task.CompletedAt == "" {
		task.CompletedAt = now
	}
	_, err := m.dm.UpdateEntity(ctx, agencyID, task.ID, entitygraph.UpdateEntityRequest{
		Properties: taskToProperties(task),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("cancelTask: %w", err)
	}
	m.publish(ctx, TopicTaskCancelled, agencyID, TaskCancelledPayload{
		TaskID:        task.ID,
		WorkflowRunID: task.WorkflowRunID,
		Reason:        reason,
	})
	return nil
}
