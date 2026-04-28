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

// ErrTaskGroupNotFound is returned when a TaskGroup vertex does not exist
// for the given agencyID and entity ID.
var ErrTaskGroupNotFound = errors.New("task group not found")

// ErrInvalidRelationship is returned by [TaskManager.CreateRelationship]
// when the (label, fromType, toType) triple is not in the Work edge-label
// whitelist, when the endpoints are in different agencies, or when an
// endpoint's type does not match the label's declared From/To types.
var ErrInvalidRelationship = errors.New("invalid relationship")

// ErrRelationshipNotFound is returned by [TaskManager.DeleteRelationship]
// when no edge matches the given (fromID, toID, label) triple in the
// agency.
var ErrRelationshipNotFound = errors.New("relationship not found")

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
