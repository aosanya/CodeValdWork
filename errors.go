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
