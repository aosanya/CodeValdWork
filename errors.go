package codevaldwork

import "errors"

// ErrTaskNotFound is returned when a task does not exist for the given
// agencyID and taskID combination.
var ErrTaskNotFound = errors.New("task not found")

// ErrTaskAlreadyExists is returned by [TaskManager.CreateTask] when a task
// with the same ID already exists for the given agency.
var ErrTaskAlreadyExists = errors.New("task already exists")

// ErrInvalidStatusTransition is returned by [TaskManager.UpdateTask] when
// the requested status change is not a valid transition from the current
// status. See [TaskStatus.CanTransitionTo] for the allowed transition table.
var ErrInvalidStatusTransition = errors.New("invalid task status transition")

// ErrInvalidTask is returned when a task is missing required fields (e.g.
// empty Title or missing AgencyID on creation).
var ErrInvalidTask = errors.New("invalid task: missing required fields")

// ErrAgentNotFound is returned when an Agent vertex does not exist for the
// given agencyID and entity ID.
var ErrAgentNotFound = errors.New("agent not found")

// ErrAgentAlreadyExists is reserved for callers that want to distinguish the
// create branch of [TaskManager.UpsertAgent] from the merge branch. The
// upsert path itself does not return this error.
var ErrAgentAlreadyExists = errors.New("agent already exists")

// ErrProjectNotFound is returned when a Project vertex does not exist
// for the given agencyID and entity ID.
var ErrProjectNotFound = errors.New("project not found")

// ErrTagNotFound is returned when a Tag vertex does not exist for the
// given agencyID and entity ID.
var ErrTagNotFound = errors.New("tag not found")

// ErrTaskTodoNotFound is returned when a TaskTodo vertex does not exist
// for the given agencyID and entity ID.
var ErrTaskTodoNotFound = errors.New("task todo not found")

// ErrWorkflowRunNotFound is returned when a WorkflowRun vertex does not
// exist for the given agencyID and entity ID.
var ErrWorkflowRunNotFound = errors.New("workflow run not found")

// ErrInvalidRunStatusTransition is returned by [TaskManager.UpdateWorkflowRunStatus]
// when the requested transition is not permitted from the current status.
// Terminal states (completed, failed, rolled_back) cannot be re-transitioned;
// failed cannot transition to completed; only in_progress → * is allowed after pending.
var ErrInvalidRunStatusTransition = errors.New("invalid workflow run status transition")

// ErrWorkflowRunNameExists is returned by [TaskManager.CreateWorkflowRun]
// when a run with the same (agencyID, name) pair already exists. The caller
// should append a discriminator and retry, or treat the existing run as
// idempotent — names are immutable once created.
var ErrWorkflowRunNameExists = errors.New("workflow run name already exists")

// ErrWorkflowRunMismatch is returned by [TaskManager.AssignTask] when the
// caller supplies a workflow_run_id that differs from the one already
// recorded on the Task. A task belonging to two runs breaks the rollback
// invariant — callers must instead create a fresh task under the new run
// (FEAT-20260602-002 — reject reassignment policy).
var ErrWorkflowRunMismatch = errors.New("task already belongs to a different workflow run")

// ErrProjectAlreadyExists is returned by [TaskManager.CreateProject]
// when a Project with the same ID already exists in the agency.
var ErrProjectAlreadyExists = errors.New("project already exists")

// ErrInvalidRelationship is returned by [TaskManager.CreateRelationship]
// when the (label, fromType, toType) triple is not in the Work edge-label
// whitelist, when the endpoints are in different agencies, or when an
// endpoint's type does not match the label's declared From/To types.
var ErrInvalidRelationship = errors.New("invalid relationship")

// ErrRelationshipNotFound is returned by [TaskManager.DeleteRelationship]
// when no edge matches the given (fromID, toID, label) triple in the
// agency.
var ErrRelationshipNotFound = errors.New("relationship not found")

// ErrInvalidImport is returned by [TaskManager.ImportProject] when the
// supplied document cannot be parsed.
var ErrInvalidImport = errors.New("invalid import: markdown document could not be parsed")

// ErrImportJobNotFound is returned by [TaskManager.GetImportProjectStatus]
// and [TaskManager.CancelImportProject] when no job with the given ID exists.
var ErrImportJobNotFound = errors.New("import job not found")

// ErrImportJobNotCancellable is returned by [TaskManager.CancelImportProject]
// when the job has already reached a terminal state (completed, failed, or
// cancelled).
var ErrImportJobNotCancellable = errors.New("import job is not cancellable")

// ErrRollbackConflict is returned by [TaskManager.RollbackWorkflowRun] when
// the run is already in the rolling_back transient state (double-rollback).
// The caller should wait for the current rollback to complete or reach
// rollback_failed before re-triggering.
var ErrRollbackConflict = errors.New("workflow run rollback already in progress")

// ErrForeignRunDependency is returned by [TaskManager.RollbackWorkflowRun]
// when a Task inside the run closure has a depends_on edge pointing to a Task
// that belongs to a different WorkflowRun. Deleting the Task would break the
// other run's dependency graph. The caller must roll back the dependent run
// first, then re-trigger this rollback.
var ErrForeignRunDependency = errors.New("run closure contains a task depended on by another workflow run")

// ErrFailureBudgetAlreadySet is returned by [TaskManager.SetFailureBudget]
// when the run's failure_pipeline_budget has already been set to a non-zero
// value. The budget is frozen for the lifetime of the run; resetting it
// would invalidate previously-charged increments (FEAT-20260602-007).
var ErrFailureBudgetAlreadySet = errors.New("workflow run failure budget already set")

// ErrNotRootWorkflowRun is returned by [TaskManager.SetFailureBudget] and
// [TaskManager.IncrementFailureBudget] when the target run has a non-empty
// parent_workflow_run_id. The failure-pipeline budget lives only on the
// root run — child runs must defer to their root (FEAT-20260602-007).
var ErrNotRootWorkflowRun = errors.New("workflow run is not a root run; budget operations must target the root")

// ErrCannotCancelTerminalRun is returned by [TaskManager.CancelWorkflowRun]
// when the target run is not in the in_progress status. Cancel is only valid
// for actively running runs (FEAT-20260602-008). Already-terminal runs
// (completed, failed, cancelled, rolled_back, rollback_failed) and runs
// already in a transient state (rolling_back) are rejected with this error.
// An already-cancelling run is NOT an error — the call is idempotent and
// returns the existing cancellation envelope.
var ErrCannotCancelTerminalRun = errors.New("workflow run is not in_progress; cancel rejected")

// ErrBlocked is the sentinel returned by [TaskManager.UpdateTask] when a
// pending → in_progress transition is rejected because the task has one or
// more non-terminal `blocks`-inbound predecessors. Match it with
// [errors.Is]; to extract the blocker IDs, use [errors.As] against
// [*BlockedError].
var ErrBlocked = errors.New("task is blocked by non-terminal blockers")

// BlockedError carries the IDs of the still-non-terminal predecessor tasks
// that prevented a pending → in_progress transition. The error wraps
// [ErrBlocked] for sentinel matching:
//
//	if errors.Is(err, codevaldwork.ErrBlocked) { ... }
//	var be *codevaldwork.BlockedError
//	if errors.As(err, &be) { use(be.BlockerTaskIDs) }
//
// The gRPC layer maps this to codes.FailedPrecondition and packs the IDs into
// a [BlockedByInfo] status detail.
type BlockedError struct {
	// BlockerTaskIDs lists the IDs of non-terminal blocker tasks. Order
	// matches the traversal order returned by the underlying graph and is
	// not otherwise meaningful.
	BlockerTaskIDs []string
}

// Error implements the error interface.
func (e *BlockedError) Error() string {
	return ErrBlocked.Error()
}

// Is reports whether target is [ErrBlocked] so [errors.Is] callers can match
// the sentinel without unwrapping.
func (e *BlockedError) Is(target error) bool {
	return target == ErrBlocked
}
