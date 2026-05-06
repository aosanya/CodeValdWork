// Package codevaldwork — domain entity types.
//
// This file mirrors the TypeDefinitions declared in [DefaultWorkSchema]:
//   - Task    — a unit of work assigned to an AI Agent
//   - Agent   — Work-domain projection of an AI agent
//   - Project — optional container that groups related Tasks
//   - Tag     — free-form label attached to Tasks via `has_tag` edges
//
// All domain structs use string timestamps (ISO 8601 / RFC 3339) to match
// the entitygraph property storage convention used across the CodeVald platform.
package codevaldwork

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

	// TaskPriorityHigh is used for time-sensitive tasks.
	TaskPriorityHigh TaskPriority = "high"

	// TaskPriorityCritical is reserved for tasks that require immediate attention.
	TaskPriorityCritical TaskPriority = "critical"
)

// Task is the core domain entity managed by [TaskManager].
// All timestamps are ISO 8601 strings (RFC 3339). Empty string means "not set".
type Task struct {
	// ID is the unique identifier for this task within the agency.
	// Set by the backend on creation; callers should leave it empty in
	// CreateTask requests.
	ID string `json:"id"`

	// AgencyID is the agency that owns this task.
	AgencyID string `json:"agency_id"`

	// Description provides additional context for the agent assigned
	// to this task. Optional.
	Description string `json:"description,omitempty"`

	// Status is the current lifecycle state of the task.
	// Always starts as [TaskStatusPending] on creation.
	Status TaskStatus `json:"status"`

	// Priority indicates the relative urgency of the task.
	// Defaults to [TaskPriorityMedium] when not specified.
	Priority TaskPriority `json:"priority"`

	// DueAt is the RFC 3339 deadline; empty string when no deadline is set.
	DueAt string `json:"due_at,omitempty"`

	// Tags are free-form labels associated with the task.
	Tags []string `json:"tags,omitempty"`

	// EstimatedHours is the planned effort to complete the task, in hours.
	// Zero when not estimated.
	EstimatedHours float64 `json:"estimated_hours,omitempty"`

	// Context is the AI agent's working memory blob. Optional.
	Context string `json:"context,omitempty"`

	// CreatedAt is the RFC 3339 timestamp when the task was first created.
	CreatedAt string `json:"created_at"`

	// UpdatedAt is the RFC 3339 timestamp of the most recent mutation.
	UpdatedAt string `json:"updated_at"`

	// CompletedAt is the RFC 3339 timestamp when the task reached a terminal
	// status (completed, failed, or cancelled). Empty until then.
	CompletedAt string `json:"completed_at,omitempty"`

	// Title is the short human-readable label for the task (e.g. "Farm Dashboard").
	// Distinct from Description, which carries the full implementation spec.
	Title string `json:"title,omitempty"`

	// TaskName is the project-scoped human-readable identifier auto-generated
	// by CreateTaskInProject (e.g. "MVP-001"). Empty for tasks not in a project.
	TaskName string `json:"task_name,omitempty"`

	// ProjectName is the URL-safe slug of the project this task belongs to.
	// Empty for tasks not in a project.
	ProjectName string `json:"project_name,omitempty"`

	// SeparateBranch indicates whether this task should be worked on in its own git branch.
	SeparateBranch bool `json:"separate_branch,omitempty"`

	// BranchName is the git branch to create/use for this task (e.g. "feature/SF-001_scaffolding").
	BranchName string `json:"branch_name,omitempty"`
}

// ImportResult is returned by [TaskManager.ImportProject].
type ImportResult struct {
	// Project is the newly created Project vertex.
	Project Project

	// Tasks are the Task vertices created in document order.
	Tasks []Task

	// DepsCreated is the number of depends_on edges written between tasks.
	DepsCreated int

	// TasksCreated is the number of Task vertices created (len(Tasks)).
	TasksCreated int
}

// ImportProjectJob tracks an async project-import operation started by
// [TaskManager.StartImportProject]. Status transitions:
//
//	pending → running → completed | failed | cancelled
type ImportProjectJob struct {
	// ID is the entity-graph storage key for this job.
	ID string `json:"id"`

	// AgencyID is the agency that owns this import job.
	AgencyID string `json:"agency_id"`

	// Status is the current lifecycle state ("pending", "running",
	// "completed", "failed", "cancelled").
	Status string `json:"status"`

	// ErrorMessage is set when Status is "failed".
	ErrorMessage string `json:"error_message,omitempty"`

	// ProgressSteps are in-memory log lines captured while the job goroutine
	// is running. Not persisted — only present in [GetImportProjectStatus]
	// responses while the goroutine is alive.
	ProgressSteps []string `json:"progress_steps,omitempty"`

	// TasksCreated is the number of Task vertices written. Populated once the
	// job reaches "completed".
	TasksCreated int `json:"tasks_created,omitempty"`

	// DepsCreated is the number of depends_on edges written. Populated once
	// the job reaches "completed".
	DepsCreated int `json:"deps_created,omitempty"`

	// ProjectName is the URL-safe slug of the project created by this import.
	// Populated once the job reaches "completed".
	ProjectName string `json:"project_name,omitempty"`

	// CreatedAt is the RFC 3339 timestamp when the job was first created.
	CreatedAt string `json:"created_at"`

	// UpdatedAt is the RFC 3339 timestamp of the most recent status change.
	UpdatedAt string `json:"updated_at"`
}

// TaskFilter constrains the results returned by [TaskManager.ListTasks].
// Zero values mean "no filter" for that field — all values match.
type TaskFilter struct {
	// Status filters tasks to the given status. Empty string matches all.
	Status TaskStatus

	// Priority filters tasks to the given priority. Empty string matches all.
	Priority TaskPriority
}

// Agent is the Work-domain projection of an AI agent. Each Agent becomes a
// graph vertex so that `assigned_to` edges are first-class graph relationships
// rather than string fields on the Task document.
//
// Uniqueness — at most one Agent per (AgencyID, AgentID) — is enforced at the
// schema level via UniqueKey: ["agent_id"]. UpsertAgent relies on this for
// find-or-create semantics.
type Agent struct {
	// ID is the entity-graph storage key — opaque to callers.
	ID string `json:"id"`

	// AgencyID is the agency this agent serves.
	AgencyID string `json:"agency_id"`

	// AgentID is the external agent identifier (e.g. a CodeValdAI agent ID).
	// Required and unique within an agency.
	AgentID string `json:"agent_id"`

	// DisplayName is a human-readable label for the agent. Optional.
	DisplayName string `json:"display_name,omitempty"`

	// Capability is the agent's primary capability (e.g. "code", "research",
	// "review"). Optional.
	Capability string `json:"capability,omitempty"`

	// RoleName is the role this agent fulfils within the agency
	// (e.g. "domain-expert", "human-code-reviewer"). Used as a stable
	// filter key in event payload_condition rules. Optional.
	RoleName string `json:"role_name,omitempty"`

	// CreatedAt is the RFC 3339 timestamp when the agent was first registered.
	CreatedAt string `json:"created_at"`

	// UpdatedAt is the RFC 3339 timestamp of the most recent upsert.
	UpdatedAt string `json:"updated_at"`
}

// Project is an optional container that groups related tasks (e.g. a sprint,
// a milestone, or an epic). Tasks become members via the `member_of`
// graph edge — many-to-many; a Task may belong to multiple Projects.
type Project struct {
	// ID is the unique identifier for this project within the agency.
	ID string `json:"id"`

	// AgencyID is the agency that owns this project.
	AgencyID string `json:"agency_id"`

	// Name is the short human-readable label. Required.
	Name string `json:"name"`

	// ProjectName is the URL-safe slug derived from Name: lowercase with
	// spaces replaced by underscores (e.g. "My Sprint" → "my_sprint").
	ProjectName string `json:"project_name"`

	// Description provides additional context for the project. Optional.
	Description string `json:"description,omitempty"`

	// GithubRepo is the canonical GitHub repository, e.g. "owner/name".
	// Optional.
	GithubRepo string `json:"github_repo,omitempty"`

	// TaskPrefix is prepended to the auto-generated task name counter when
	// tasks are created via CreateTaskInProject (e.g. "MVP-" → "MVP-001").
	// If empty, defaults to "<project_name>-" at creation time.
	TaskPrefix string `json:"task_prefix,omitempty"`

	// CreatedAt is the RFC 3339 timestamp when the project was first created.
	CreatedAt string `json:"created_at"`

	// UpdatedAt is the RFC 3339 timestamp of the most recent mutation.
	UpdatedAt string `json:"updated_at"`
}

// Tag is a free-form label that can be attached to Tasks via `has_tag` graph
// edges. Tags are unique by name within an agency (UniqueKey: ["name"]).
type Tag struct {
	// ID is the entity-graph storage key — opaque to callers.
	ID string `json:"id"`

	// AgencyID is the agency that owns this tag.
	AgencyID string `json:"agency_id"`

	// Name is the unique label text (e.g. "setup", "auth"). Required.
	Name string `json:"name"`

	// Color is an optional hex/CSS color hint for UI rendering.
	Color string `json:"color,omitempty"`

	// Description provides additional context for the tag. Optional.
	Description string `json:"description,omitempty"`

	// CreatedAt is the RFC 3339 timestamp when the tag was first created.
	CreatedAt string `json:"created_at"`

	// UpdatedAt is the RFC 3339 timestamp of the most recent mutation.
	UpdatedAt string `json:"updated_at"`
}
