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
