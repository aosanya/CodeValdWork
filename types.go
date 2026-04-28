package codevaldwork

import (
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

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

	// DueAt is the deadline by which the task should be completed.
	// Nil when no deadline is set.
	DueAt *time.Time

	// Tags are free-form labels associated with the task.
	// Empty slice when the task has no tags.
	Tags []string

	// EstimatedHours is the planned effort to complete the task, in hours.
	// Zero when not estimated.
	EstimatedHours float64

	// Context is the AI agent's working memory blob — long-lived state
	// preserved across turns. Optional.
	Context string

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
//
// Filtering by assignee is no longer a property filter — task assignment is
// a graph edge. Callers needing tasks-for-agent should traverse inbound
// `assigned_to` from the Agent vertex (or list all tasks and join in
// memory if the volume is small).
type TaskFilter struct {
	// Status filters tasks to the given status. Empty string matches all.
	Status TaskStatus

	// Priority filters tasks to the given priority. Empty string matches all.
	Priority TaskPriority
}

// TaskGroup is an optional container that groups related tasks (e.g. a sprint,
// a project milestone, or an epic). Tasks become members via the `member_of`
// graph edge added in MVP-WORK-012.
type TaskGroup struct {
	// ID is the unique identifier for this group within the agency.
	// Set by the backend on creation.
	ID string

	// AgencyID is the agency that owns this group.
	AgencyID string

	// Name is the short human-readable label. Required.
	Name string

	// Description provides additional context for the group. Optional.
	Description string

	// DueAt is the target completion date for the group. Optional.
	DueAt *time.Time

	// CreatedAt is the UTC timestamp when the group was first created.
	CreatedAt time.Time

	// UpdatedAt is the UTC timestamp of the most recent mutation.
	UpdatedAt time.Time
}

// taskGroupToProperties serialises a TaskGroup into the property map stored on
// its entitygraph Entity. Time fields are encoded as RFC 3339 strings.
func taskGroupToProperties(g TaskGroup) map[string]any {
	props := map[string]any{
		"name":        g.Name,
		"description": g.Description,
	}
	if g.DueAt != nil && !g.DueAt.IsZero() {
		props["dueAt"] = g.DueAt.UTC().Format(time.RFC3339Nano)
	}
	return props
}

// taskGroupFromEntity reconstructs a TaskGroup from an entitygraph Entity.
func taskGroupFromEntity(e entitygraph.Entity) TaskGroup {
	g := TaskGroup{
		ID:        e.ID,
		AgencyID:  e.AgencyID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if v, ok := e.Properties["name"].(string); ok {
		g.Name = v
	}
	if v, ok := e.Properties["description"].(string); ok {
		g.Description = v
	}
	if v, ok := e.Properties["dueAt"].(string); ok && v != "" {
		if ts, err := time.Parse(time.RFC3339Nano, v); err == nil {
			g.DueAt = &ts
		}
	}
	return g
}

