package codevaldwork

import "time"

// TaskStatus represents the lifecycle state of a [Task].
type TaskStatus string

const (
	// TaskStatusPending is the initial state of every new task.
	// The task is waiting to be picked up by an agent.
	TaskStatusPending TaskStatus = "pending"

	// TaskStatusInProgress means an agent has claimed and is actively
	// working on the task.
	TaskStatusInProgress TaskStatus = "in_progress"

	// TaskStatusCompleted is a terminal state — the agent finished
	// the task successfully.
	TaskStatusCompleted TaskStatus = "completed"

	// TaskStatusFailed is a terminal state — the agent encountered an
	// unrecoverable error and could not complete the task.
	TaskStatusFailed TaskStatus = "failed"

	// TaskStatusCancelled is a terminal state — the task was abandoned
	// before completion, either by the agent or by an operator.
	TaskStatusCancelled TaskStatus = "cancelled"
)

// CanTransitionTo reports whether transitioning from the receiver status to
// next is a valid move in the task lifecycle.
//
// Allowed transitions:
//
//	pending     → in_progress, cancelled
//	in_progress → completed, failed, cancelled
//	completed   → (none — terminal)
//	failed      → (none — terminal)
//	cancelled   → (none — terminal)
func (s TaskStatus) CanTransitionTo(next TaskStatus) bool {
	switch s {
	case TaskStatusPending:
		return next == TaskStatusInProgress || next == TaskStatusCancelled
	case TaskStatusInProgress:
		return next == TaskStatusCompleted || next == TaskStatusFailed || next == TaskStatusCancelled
	default:
		// completed, failed, cancelled are terminal — no further transitions.
		return false
	}
}

// TaskPriority expresses the relative urgency of a [Task].
type TaskPriority string

const (
	// TaskPriorityLow is used for background or non-urgent tasks.
	TaskPriorityLow TaskPriority = "low"

	// TaskPriorityMedium is the default priority for new tasks.
	TaskPriorityMedium TaskPriority = "medium"

	// TaskPriorityHigh is used for time-sensitive tasks that should be
	// picked up before lower-priority work.
	TaskPriorityHigh TaskPriority = "high"

	// TaskPriorityCritical is reserved for tasks that require immediate
	// attention — e.g. blocking another agency or active incident.
	TaskPriorityCritical TaskPriority = "critical"
)

// Task is the core domain entity managed by [TaskManager].
// All fields are plain Go values — no methods, no database tags.
type Task struct {
	// ID is the unique identifier for this task within the agency.
	// Set by the backend on creation; callers should leave it empty in
	// CreateTask requests.
	ID string

	// AgencyID is the agency that owns this task. Set by the backend
	// from the agencyID parameter; callers do not need to set it.
	AgencyID string

	// Title is a short human-readable summary of the task.
	// Required — CreateTask returns [ErrInvalidTask] when empty.
	Title string

	// Description provides additional context for the agent assigned
	// to this task. Optional.
	Description string

	// Status is the current lifecycle state of the task.
	// Always starts as [TaskStatusPending] on creation.
	Status TaskStatus

	// Priority indicates the relative urgency of the task.
	// Defaults to [TaskPriorityMedium] when not specified.
	Priority TaskPriority

	// AssignedTo holds the ID of the agent currently responsible for
	// this task. Empty when the task is unassigned (status: pending).
	AssignedTo string

	// CreatedAt is the UTC timestamp when the task was first created.
	// Set by the backend; immutable after creation.
	CreatedAt time.Time

	// UpdatedAt is the UTC timestamp of the most recent mutation.
	// Updated by the backend on every successful UpdateTask call.
	UpdatedAt time.Time

	// CompletedAt is the UTC timestamp when the task reached a terminal
	// status (completed, failed, or cancelled). Nil until then.
	CompletedAt *time.Time
}

// TaskFilter constrains the results returned by [TaskManager.ListTasks].
// Zero values mean "no filter" for that field — all values match.
type TaskFilter struct {
	// Status filters tasks to the given status. Empty string matches all.
	Status TaskStatus

	// Priority filters tasks to the given priority. Empty string matches all.
	Priority TaskPriority

	// AssignedTo filters tasks assigned to a specific agent ID.
	// Empty string matches all (including unassigned tasks).
	AssignedTo string
}
