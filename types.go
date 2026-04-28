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

	// AssignedTo holds the ID of the agent currently responsible for
	// this task. Empty when the task is unassigned.
	//
	// Deprecated: Phase 2 moves task assignment to an `assigned_to` graph
	// edge between Task and Agent vertices. The field is retained until
	// MVP-WORK-010 lands so the gRPC surface keeps compiling, but the
	// storage layer no longer persists it — round-tripping a non-empty
	// value through CreateTask/GetTask returns "".
	AssignedTo string

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
type TaskFilter struct {
	// Status filters tasks to the given status. Empty string matches all.
	Status TaskStatus

	// Priority filters tasks to the given priority. Empty string matches all.
	Priority TaskPriority

	// AssignedTo filters tasks assigned to a specific agent ID.
	// Empty string matches all (including unassigned tasks).
	//
	// Deprecated: assignment moves to a graph edge in MVP-WORK-010; this
	// filter becomes inert once the property is no longer persisted. The
	// field is retained here so callers still compile.
	AssignedTo string
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

// Agent is the Work-domain projection of an AI agent. Each Agent becomes a
// graph vertex so that `assigned_to` edges (added in MVP-WORK-010) are
// first-class graph relationships rather than string fields on the Task
// document.
//
// Uniqueness — at most one Agent per (AgencyID, AgentID) — is enforced by
// [TaskManager.UpsertAgent] (added in MVP-WORK-010).
type Agent struct {
	// ID is the entity-graph storage key — opaque to callers.
	ID string

	// AgencyID is the agency this agent serves.
	AgencyID string

	// AgentID is the external agent identifier (e.g. a CodeValdAI agent ID).
	// Required and unique within an agency.
	AgentID string

	// DisplayName is a human-readable label for the agent. Optional.
	DisplayName string

	// Capability is the agent's primary capability (e.g. "code", "research",
	// "review"). Optional.
	Capability string

	// CreatedAt is the UTC timestamp when the agent was first registered.
	CreatedAt time.Time

	// UpdatedAt is the UTC timestamp of the most recent upsert.
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

// agentToProperties serialises an Agent into the property map stored on its
// entitygraph Entity.
func agentToProperties(a Agent) map[string]any {
	return map[string]any{
		"agentID":     a.AgentID,
		"displayName": a.DisplayName,
		"capability":  a.Capability,
	}
}

// agentFromEntity reconstructs an Agent from an entitygraph Entity.
func agentFromEntity(e entitygraph.Entity) Agent {
	a := Agent{
		ID:        e.ID,
		AgencyID:  e.AgencyID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if v, ok := e.Properties["agentID"].(string); ok {
		a.AgentID = v
	}
	if v, ok := e.Properties["displayName"].(string); ok {
		a.DisplayName = v
	}
	if v, ok := e.Properties["capability"].(string); ok {
		a.Capability = v
	}
	return a
}
