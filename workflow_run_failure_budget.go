// workflow_run_failure_budget.go — FEAT-20260602-007 failure-pipeline budget.
//
// A WorkflowRun's failure_pipeline_budget caps the number of recovery
// pipeline activations Cross will dispatch under the run's lineage. The
// counter lives on the root run; child (recovery) runs reference their
// root via root_workflow_run_id.
//
// start-pipeline calls [SetFailureBudget] once at run-create time to lock
// in the resolved budget (payload > agency > env default). Cross's failure
// dispatch calls [IncrementFailureBudget] before creating each child run;
// the response's `exhausted` flag tells Cross to skip the dispatch and
// fail the run instead.
package codevaldwork

import (
	"context"
	"errors"
	"fmt"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// SetFailureBudget locks the failure-pipeline budget on a root WorkflowRun.
// Refuses to overwrite a non-zero value (the budget is frozen for the
// lifetime of the run) and refuses to act on non-root runs.
//
// Returns [ErrWorkflowRunNotFound], [ErrNotRootWorkflowRun], or
// [ErrFailureBudgetAlreadySet].
func (m *taskManager) SetFailureBudget(ctx context.Context, agencyID, runID string, budget int) (WorkflowRun, error) {
	run, err := m.GetWorkflowRun(ctx, agencyID, runID)
	if err != nil {
		return WorkflowRun{}, err
	}
	if run.ParentWorkflowRunID != "" {
		return WorkflowRun{}, fmt.Errorf("%w: run %s has parent %s", ErrNotRootWorkflowRun, runID, run.ParentWorkflowRunID)
	}
	if run.FailurePipelineBudget != 0 {
		return WorkflowRun{}, fmt.Errorf("%w: run %s has budget %d", ErrFailureBudgetAlreadySet, runID, run.FailurePipelineBudget)
	}
	updated, err := m.dm.UpdateEntity(ctx, agencyID, runID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"failure_pipeline_budget": budget,
			// Root runs default their root pointer to their own id so
			// downstream services can treat root_workflow_run_id uniformly.
			"root_workflow_run_id": runID,
		},
	})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("SetFailureBudget: %w", err)
	}
	return workflowRunFromEntity(updated), nil
}

// IncrementFailureBudget atomically increments failure_pipelines_used on
// the root run identified by rootRunID. The child_run_id is recorded in
// counted_child_run_ids; subsequent calls with the same child_run_id are
// idempotent (they return the current counter state without incrementing).
//
// The returned `exhausted` flag is true iff `used > budget` after the
// increment. Cross MUST skip the recovery dispatch on `exhausted == true`
// and publish work.run.failed { reason: failure_budget_exhausted } for
// the root run.
//
// Returns [ErrWorkflowRunNotFound] or [ErrNotRootWorkflowRun].
func (m *taskManager) IncrementFailureBudget(ctx context.Context, agencyID, rootRunID, childRunID string) (used, budget int, exhausted bool, err error) {
	run, getErr := m.GetWorkflowRun(ctx, agencyID, rootRunID)
	if getErr != nil {
		return 0, 0, false, getErr
	}
	if run.ParentWorkflowRunID != "" {
		return 0, 0, false, fmt.Errorf("%w: run %s has parent %s", ErrNotRootWorkflowRun, rootRunID, run.ParentWorkflowRunID)
	}

	// Idempotency: if this child has already been counted, return the
	// current state without mutating.
	for _, id := range run.CountedChildRunIDs {
		if id == childRunID {
			return run.FailurePipelinesUsed, run.FailurePipelineBudget, exhaustedAt(run.FailurePipelinesUsed, run.FailurePipelineBudget), nil
		}
	}

	newUsed := run.FailurePipelinesUsed + 1
	newCounted := append(append([]string(nil), run.CountedChildRunIDs...), childRunID)

	updated, updateErr := m.dm.UpdateEntity(ctx, agencyID, rootRunID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"failure_pipelines_used": newUsed,
			"counted_child_run_ids":  newCounted,
		},
	})
	if updateErr != nil {
		// Fail closed at the caller (Cross); see FEAT-007 §3.
		return 0, 0, false, fmt.Errorf("IncrementFailureBudget: %w", updateErr)
	}
	result := workflowRunFromEntity(updated)
	return result.FailurePipelinesUsed, result.FailurePipelineBudget, exhaustedAt(result.FailurePipelinesUsed, result.FailurePipelineBudget), nil
}

// exhaustedAt reports whether the post-increment counter exceeds the cap.
// A zero budget means "unconfigured" — the gate is open until the budget
// is set, so exhaustion never trips. Once a positive budget is set,
// `used > budget` trips the gate (strict inequality matches the spec's
// "this dispatch was the one over the line" contract).
func exhaustedAt(used, budget int) bool {
	if budget <= 0 {
		return false
	}
	return used > budget
}

// CreateRecoveryWorkflowRun mints a child WorkflowRun spawned by a parent
// run's failure. The child carries parent_workflow_run_id and
// root_workflow_run_id but no budget of its own — Cross always reads and
// increments the root's counter via [IncrementFailureBudget].
//
// Returns [ErrWorkflowRunNotFound] if parentRunID does not exist, or
// [ErrWorkflowRunNameExists] on (agencyID, name) collision.
func (m *taskManager) CreateRecoveryWorkflowRun(ctx context.Context, agencyID, name, triggerEvent, initiator, parentRunID, rootRunID string) (WorkflowRun, error) {
	if parentRunID == "" {
		return WorkflowRun{}, fmt.Errorf("%w: parent_workflow_run_id is required for a recovery run", ErrInvalidTask)
	}
	if rootRunID == "" {
		// Inherit from parent when caller didn't pre-resolve it.
		parent, err := m.GetWorkflowRun(ctx, agencyID, parentRunID)
		if err != nil {
			return WorkflowRun{}, err
		}
		rootRunID = parent.RootWorkflowRunID
		if rootRunID == "" {
			rootRunID = parent.ID
		}
	}
	run, err := m.CreateWorkflowRun(ctx, agencyID, name, triggerEvent, initiator)
	if err != nil {
		return WorkflowRun{}, err
	}
	updated, err := m.dm.UpdateEntity(ctx, agencyID, run.ID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"parent_workflow_run_id": parentRunID,
			"root_workflow_run_id":   rootRunID,
		},
	})
	if err != nil {
		return WorkflowRun{}, fmt.Errorf("CreateRecoveryWorkflowRun: stamp parentage: %w", err)
	}
	return workflowRunFromEntity(updated), nil
}

// resolveRootRunID returns the run's stored root_workflow_run_id, falling
// back to the run's own ID for top-level runs. Internal helper for callers
// that need to address the root without an extra round-trip.
func resolveRootRunID(run WorkflowRun) string {
	if run.RootWorkflowRunID != "" {
		return run.RootWorkflowRunID
	}
	return run.ID
}

// isFailureBudgetError reports whether err originated from this file's
// sentinel set. Exposed for tests; the gRPC layer maps these to
// FailedPrecondition / NotFound as appropriate.
func isFailureBudgetError(err error) bool {
	return errors.Is(err, ErrFailureBudgetAlreadySet) || errors.Is(err, ErrNotRootWorkflowRun)
}
