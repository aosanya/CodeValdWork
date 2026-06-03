// workflow_run_watchdog.go — stale-run detection helpers (FEAT-20260602-006).
//
// This file adds the query helpers used by the CodeValdCross watchdog sweeper
// and the handlers that process work.run.timeout and work.task.timeout events.
package codevaldwork

import (
	"context"
	"errors"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// ListWorkflowRunsStaleSince returns all non-terminal, unpaused WorkflowRuns
// whose last_event_at is before cutoff and whose timeout_published is false.
// Used by the Cross watchdog sweeper to find runs to time out.
func (m *taskManager) ListWorkflowRunsStaleSince(ctx context.Context, agencyID string, cutoff time.Time) ([]WorkflowRun, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: agencyID,
		TypeID:   workflowRunTypeID,
	})
	if err != nil {
		return nil, err
	}
	cutoffStr := cutoff.UTC().Format(time.RFC3339)
	var out []WorkflowRun
	for _, e := range entities {
		r := workflowRunFromEntity(e)
		if r.Status.IsTerminal() {
			continue
		}
		if r.PausedAt != "" {
			continue
		}
		if r.TimeoutPublished {
			continue
		}
		if r.LastEventAt == "" || r.LastEventAt >= cutoffStr {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// ListWorkflowRunsStepStaleSince returns non-terminal, unpaused WorkflowRuns
// that have a current_step_id set and current_step_started_at before cutoff.
// Used by the Cross watchdog sweeper to find stalled per-step executions.
func (m *taskManager) ListWorkflowRunsStepStaleSince(ctx context.Context, agencyID string, cutoff time.Time) ([]WorkflowRun, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: agencyID,
		TypeID:   workflowRunTypeID,
	})
	if err != nil {
		return nil, err
	}
	cutoffStr := cutoff.UTC().Format(time.RFC3339)
	var out []WorkflowRun
	for _, e := range entities {
		r := workflowRunFromEntity(e)
		if r.Status.IsTerminal() {
			continue
		}
		if r.PausedAt != "" {
			continue
		}
		if r.CurrentStepID == "" {
			continue
		}
		if r.CurrentStepStartedAt == "" || r.CurrentStepStartedAt >= cutoffStr {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// MarkTimeoutPublished sets timeout_published=true so the sweeper skips the run on
// subsequent ticks. Called before publishing work.run.timeout for idempotency.
func (m *taskManager) MarkTimeoutPublished(ctx context.Context, agencyID, runID string) error {
	_, err := m.dm.UpdateEntity(ctx, agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{"timeout_published": true},
	})
	if err != nil && errors.Is(err, entitygraph.ErrEntityNotFound) {
		return nil
	}
	return err
}

// HandleRunTimeout processes a work.run.timeout event: flips the run to failed
// (if not already terminal) and cascades to non-terminal tasks.
func (m *taskManager) HandleRunTimeout(ctx context.Context, agencyID, runID string) error {
	run, err := m.GetWorkflowRun(ctx, agencyID, runID)
	if err != nil {
		if errors.Is(err, ErrWorkflowRunNotFound) {
			return nil
		}
		return err
	}
	if run.Status.IsTerminal() {
		return nil
	}

	if _, err := m.UpdateWorkflowRunStatus(ctx, agencyID, runID, WorkflowRunStatusFailed, "stale"); err != nil {
		return err
	}

	tasks, _ := m.ListTasksForRun(ctx, agencyID, runID)
	for _, t := range tasks {
		if t.Status == TaskStatusCompleted || t.Status == TaskStatusFailed ||
			t.Status == TaskStatusCancelled {
			continue
		}
		t.Status = TaskStatusFailed
		if _, err := m.UpdateTask(ctx, agencyID, t); err != nil {
			continue
		}
		if m.publisher != nil {
			eventbus.SafePublish(ctx, m.publisher, eventbus.Event{
				Topic:    TopicTaskFailed,
				AgencyID: agencyID,
				Payload: TaskFailedPayload{
					TaskID:        t.ID,
					Reason:        "run_timeout",
					WorkflowRunID: runID,
				},
			})
		}
	}
	return nil
}

// HandleTaskTimeout processes a work.task.timeout event: flips the task to failed.
func (m *taskManager) HandleTaskTimeout(ctx context.Context, agencyID, taskOrTodoID string, runID string) error {
	task, err := m.GetTask(ctx, agencyID, taskOrTodoID)
	if err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil
		}
		return err
	}
	if task.Status == TaskStatusCompleted || task.Status == TaskStatusFailed ||
		task.Status == TaskStatusCancelled {
		return nil
	}
	task.Status = TaskStatusFailed
	if _, err := m.UpdateTask(ctx, agencyID, task); err != nil {
		return err
	}
	if m.publisher != nil {
		eventbus.SafePublish(ctx, m.publisher, eventbus.Event{
			Topic:    TopicTaskFailed,
			AgencyID: agencyID,
			Payload: TaskFailedPayload{
				TaskID:        taskOrTodoID,
				Reason:        "step_timeout",
				WorkflowRunID: runID,
			},
		})
	}
	return nil
}

// ListTasksForRun returns every Task linked to runID.
func (m *taskManager) ListTasksForRun(ctx context.Context, agencyID, runID string) ([]Task, error) {
	return m.ListTasks(ctx, agencyID, TaskFilter{WorkflowRunID: runID})
}
