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

	// TaskStatusBlocked means the task has been assigned but is waiting on
	// at least one unfulfilled depends_on edge to a non-terminal source
	// task. Dispatch (work.task.assigned publish) is held until every
	// dependency reaches a terminal state, at which point the task
	// transitions back to pending and the assignment fires for real.
	TaskStatusBlocked TaskStatus = "blocked"

	// TaskStatusAwaitingDirection is a non-terminal hold state entered when
	// a task has exhausted its automatic retry budget and AI classification
	// determines that human (or AI reviewer) intervention is required.
	// The WorkflowRun transitions to paused while any task holds this status.
	// Resolved by a work.task.direction event — transitions to in_progress
	// (retry), blocked (awaiting external fix), or cancelled (abort).
	TaskStatusAwaitingDirection TaskStatus = "awaiting-direction"
)

// CanTransitionTo reports whether transitioning from the receiver status to
// next is a valid move in the task lifecycle.
//
// Allowed transitions:
//
//	pending            → in_progress, cancelled, blocked
//	blocked            → pending, cancelled
//	in_progress        → completed, failed, cancelled, awaiting-direction
//	awaiting-direction → in_progress, blocked, cancelled
//	completed          → (none — terminal)
//	failed             → (none — terminal)
//	cancelled          → (none — terminal)
func (s TaskStatus) CanTransitionTo(next TaskStatus) bool {
	switch s {
	case TaskStatusPending:
		return next == TaskStatusInProgress || next == TaskStatusCancelled || next == TaskStatusBlocked
	case TaskStatusBlocked:
		// On dependency completion the dispatch flow flips back to pending,
		// re-publishes work.task.assigned, and then UpdateTask transitions
		// to in_progress. Cancel is always allowed as an escape hatch.
		// awaiting-direction is allowed for the operator unblock path: a task
		// that was blocked by a human (mark-blocked direction) can be re-opened
		// for a new direction cycle without losing the escalation context.
		return next == TaskStatusPending || next == TaskStatusCancelled || next == TaskStatusAwaitingDirection
	case TaskStatusInProgress:
		return next == TaskStatusCompleted || next == TaskStatusFailed ||
			next == TaskStatusCancelled || next == TaskStatusAwaitingDirection
	case TaskStatusAwaitingDirection:
		// Direction received: retry (→ in_progress), external blocker noted
		// (→ blocked), or operator abort (→ cancelled).
		return next == TaskStatusInProgress || next == TaskStatusBlocked || next == TaskStatusCancelled
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

	// AssignedTo is the entity ID of the Agent currently responsible for this
	// task. Empty string means unassigned. Populated from the assigned_to graph
	// edge at read time; mutated via AssignTask / UnassignTask.
	AssignedTo string `json:"assigned_to,omitempty"`

	// WorkflowRunID denormalises the WorkflowRun anchor onto the Task row so
	// queries can filter by run-id without traversing the started_task edge.
	// Set on CreateTask when a non-empty value is supplied (which also writes
	// the started_task edge) and inherited by AssignTask from the inbound
	// event payload. Empty for tasks not produced under a WorkflowRun
	// (FEAT-20260602-002).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`

	// RecoveryRunsUsed is the number of automatic retry cycles already charged
	// against the task's recovery budget. Incremented by the failure handler
	// each time a task is re-dispatched after failing. Once it reaches the
	// max_recovery_runs threshold (default 3) the task escalates to
	// AI classification before entering awaiting-direction.
	RecoveryRunsUsed int `json:"recovery_runs_used,omitempty"`

	// BlockerNote is the human-readable note supplied by an operator when
	// choosing the mark-blocked direction option. Stored so the blocker reason
	// is visible on the task without parsing the description field.
	BlockerNote string `json:"blocker_note,omitempty"`

	// DirectionHistory is a JSON-encoded []string of past direction options
	// submitted for this task. Appended each time a work.task.direction event
	// is handled so the full recovery trajectory is visible.
	DirectionHistory string `json:"direction_history,omitempty"`
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

// TodoStatus represents the lifecycle state of a [TaskTodo].
type TodoStatus string

const (
	// TodoStatusPending is the initial state — the todo has been created and
	// is waiting to be picked up by a CodeValdAI agent via work.task.todo.
	TodoStatusPending TodoStatus = "pending"

	// TodoStatusBlocked is a holding state — the todo was created but has
	// unfulfilled depends_on entries. It will not be dispatched until every
	// predecessor todo reaches TodoStatusCompleted. If any predecessor fails,
	// all of its blocked dependents are cascade-failed.
	TodoStatusBlocked TodoStatus = "blocked"

	// TodoStatusDispatched means a CodeValdAI agent has started an AgentRun
	// for this todo (ai.task.in_progress received).
	TodoStatusDispatched TodoStatus = "dispatched"

	// TodoStatusCompleted is a terminal state — the agent finished successfully.
	TodoStatusCompleted TodoStatus = "completed"

	// TodoStatusFailed is a terminal state — the agent encountered an error.
	TodoStatusFailed TodoStatus = "failed"

	// TodoStatusSkipped is a terminal state — a direction response chose to
	// bypass this step. Treated as non-failing by maybeCompleteParentTask so
	// skipping a todo does not fail its parent task.
	TodoStatusSkipped TodoStatus = "skipped"
)

// TaskTodo is a decomposed sub-task produced when CodeValdWork receives an
// ai.task.todo event from CodeValdAI. Each TodoItem in the payload becomes
// one TaskTodo entity linked to its parent Task via a has_todo edge.
//
// When a TaskTodo is created CodeValdWork publishes work.task.todo so that
// CodeValdAI agents can pick it up via their work plans.
type TaskTodo struct {
	// ID is the entity-graph storage key — opaque to callers.
	ID string `json:"id"`

	// AgencyID is the agency that owns this todo.
	AgencyID string `json:"agency_id"`

	// Title is the short label for this sub-task.
	Title string `json:"title"`

	// Description explains what this sub-task accomplishes.
	Description string `json:"description,omitempty"`

	// Instructions is the fully self-contained agent prompt for execution.
	Instructions string `json:"instructions"`

	// Ordinality is the 1-based position of this todo within the decomposition.
	Ordinality int `json:"ordinality"`

	// CanRunParallel is true when this todo has no predecessor dependency.
	CanRunParallel bool `json:"can_run_parallel"`

	// DependsOn lists the ordinality values of todos that must complete first.
	DependsOn []int `json:"depends_on,omitempty"`

	// Status is the current lifecycle state — see [TodoStatus].
	Status TodoStatus `json:"status"`

	// ParentTaskID is the Work Task ID from which this todo was decomposed.
	ParentTaskID string `json:"parent_task_id"`

	// DecompRunID is the CodeValdAI AgentRun ID that produced this todo.
	DecompRunID string `json:"decomp_run_id,omitempty"`

	// AgentID is the CodeValdAI agent assigned to execute this todo.
	AgentID string `json:"agent_id,omitempty"`

	// Precalls is a JSON-encoded []PrecallSpec: pre-execution fetch specs
	// executed by HydrateEventContext before the LLM runs this todo.
	Precalls string `json:"precalls,omitempty"`

	// CreatedAt is the RFC 3339 timestamp when the todo was created.
	CreatedAt string `json:"created_at"`

	// UpdatedAt is the RFC 3339 timestamp of the most recent mutation.
	UpdatedAt string `json:"updated_at"`

	// WorkflowRunID denormalises the WorkflowRun anchor onto the TaskTodo
	// row. Inherited from the parent Task at creation time so the todo
	// carries the run-id its parent belongs to (FEAT-20260602-002).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskFilter constrains the results returned by [TaskManager.ListTasks].
// Zero values mean "no filter" for that field — all values match.
type TaskFilter struct {
	// Status filters tasks to the given status. Empty string matches all.
	Status TaskStatus

	// Priority filters tasks to the given priority. Empty string matches all.
	Priority TaskPriority

	// WorkflowRunID filters tasks to the given workflow_run_id when non-empty.
	// Empty matches all (FEAT-20260602-002).
	WorkflowRunID string
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

	// RepoName is the CodeValdGit repository name associated with this project.
	// Used by HydrateEventContext to scope file hydration to the correct repo.
	RepoName string `json:"repo_name,omitempty"`

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

// WorkflowRunStatus represents the lifecycle state of a [WorkflowRun].
type WorkflowRunStatus string

const (
	// WorkflowRunStatusPending is the initial state — the run has been created
	// but no Task has yet started executing.
	WorkflowRunStatusPending WorkflowRunStatus = "pending"

	// WorkflowRunStatusInProgress means at least one Task in the run is
	// executing.
	WorkflowRunStatusInProgress WorkflowRunStatus = "in_progress"

	// WorkflowRunStatusCompleted is a terminal state — every Task in the run
	// finished successfully.
	WorkflowRunStatusCompleted WorkflowRunStatus = "completed"

	// WorkflowRunStatusFailed is a terminal state — at least one Task in the
	// run failed.
	WorkflowRunStatusFailed WorkflowRunStatus = "failed"

	// WorkflowRunStatusRolledBack is a terminal state — the rollback action
	// has compensated all artifacts of this run.
	WorkflowRunStatusRolledBack WorkflowRunStatus = "rolled_back"

	// WorkflowRunStatusRollingBack is a transient state entered when the
	// rollback coordinator begins compensating cross-service artifacts.
	// Transitions to rolled_back on success or rollback_failed on partial failure.
	WorkflowRunStatusRollingBack WorkflowRunStatus = "rolling_back"

	// WorkflowRunStatusRollbackFailed is a terminal state indicating the
	// rollback coordinator encountered a partial failure. Operator intervention
	// is required; the rollback may be re-triggered after manual remediation.
	WorkflowRunStatusRollbackFailed WorkflowRunStatus = "rollback_failed"

	// WorkflowRunStatusCancelling is the transient state entered when an
	// operator-issued POST /workflow-runs/{id}/cancel quiesces in-flight
	// handlers (FEAT-20260602-008). The run remains in this state until the
	// quiesce deadline elapses and the finalization step transitions it to
	// cancelled.
	WorkflowRunStatusCancelling WorkflowRunStatus = "cancelling"

	// WorkflowRunStatusCancelled is the terminal state reached after the
	// quiesce deadline elapses. Distinct from failed so closure SSE and
	// rollback can treat operator-cancelled runs differently if needed.
	// Rollback is allowed on cancelled runs (cancel → optional rollback).
	WorkflowRunStatusCancelled WorkflowRunStatus = "cancelled"

	// WorkflowRunStatusPaused is a non-terminal hold state entered when at
	// least one Task transitions to awaiting-direction or blocked (external
	// blocker). The run resumes to in_progress once all paused tasks receive
	// direction. The run fails if a paused task is cancelled.
	WorkflowRunStatusPaused WorkflowRunStatus = "paused"
)

// CanTransitionTo reports whether moving from the current status to next is
// a valid state-machine step.
//
//	pending          → in_progress       (first task assigned)
//	in_progress      → completed         (terminal event matched)
//	in_progress      → failed            (any failure event)
//	in_progress      → paused            (a task enters awaiting-direction)
//	in_progress      → cancelling        (operator cancel issued — FEAT-008)
//	paused           → in_progress       (all awaiting-direction tasks resolved)
//	paused           → failed            (a paused task is cancelled/failed)
//	paused           → cancelling        (operator cancel while paused)
//	cancelling       → cancelled         (quiesce deadline elapsed — FEAT-008)
//	failed           → rolling_back      (explicit rollback triggered)
//	completed        → rolling_back      (explicit rollback on a completed run)
//	cancelled        → rolling_back      (rollback allowed after cancel — FEAT-008)
//	rolling_back     → rolled_back       (coordinator finished cleanly)
//	rolling_back     → rollback_failed   (coordinator hit a partial failure)
//	rollback_failed  → rolling_back      (operator re-triggers after remediation)
func (s WorkflowRunStatus) CanTransitionTo(next WorkflowRunStatus) bool {
	switch s {
	case WorkflowRunStatusPending:
		return next == WorkflowRunStatusInProgress
	case WorkflowRunStatusInProgress:
		return next == WorkflowRunStatusCompleted ||
			next == WorkflowRunStatusFailed ||
			next == WorkflowRunStatusPaused ||
			next == WorkflowRunStatusCancelling
	case WorkflowRunStatusPaused:
		return next == WorkflowRunStatusInProgress ||
			next == WorkflowRunStatusFailed ||
			next == WorkflowRunStatusCancelling
	case WorkflowRunStatusCancelling:
		return next == WorkflowRunStatusCancelled
	case WorkflowRunStatusFailed, WorkflowRunStatusCompleted, WorkflowRunStatusCancelled:
		return next == WorkflowRunStatusRollingBack
	case WorkflowRunStatusRollingBack:
		return next == WorkflowRunStatusRolledBack || next == WorkflowRunStatusRollbackFailed
	case WorkflowRunStatusRollbackFailed:
		return next == WorkflowRunStatusRollingBack
	default:
		return false // rolled_back is terminal
	}
}

// IsTerminal reports whether s is a terminal (non-recoverable) status.
// paused is explicitly non-terminal — it resolves to in_progress or failed.
func (s WorkflowRunStatus) IsTerminal() bool {
	switch s {
	case WorkflowRunStatusCompleted, WorkflowRunStatusFailed,
		WorkflowRunStatusRolledBack, WorkflowRunStatusRollbackFailed,
		WorkflowRunStatusCancelled:
		return true
	}
	return false
}

// WorkflowRun anchors the closure of a single orchestrated execution.
// Created by a producer (e.g. the next-task function) at run start; linked
// to every Task / TaskTodo it produces via started_task / started_todo edges.
type WorkflowRun struct {
	// ID is the entity-graph storage key — opaque to callers.
	ID string `json:"id"`

	// AgencyID is the agency that owns this run.
	AgencyID string `json:"agency_id"`

	// Name is a caller-supplied or server-generated human-readable label,
	// unique per agency. Acts as the correlation handle for test scripts
	// that publish a trigger event and then need to find the run that
	// resulted from it; surfaces as the headline column in the
	// `/workflow-runs` UI list.
	Name string `json:"name"`

	// Status is the current lifecycle state.
	Status WorkflowRunStatus `json:"status"`

	// TriggerEvent names the event that started the run
	// (e.g. "work.next.requested"). Empty when not known.
	TriggerEvent string `json:"trigger_event,omitempty"`

	// Initiator is an opaque caller identifier. Empty when not set.
	Initiator string `json:"initiator,omitempty"`

	// Notes is free-form human-readable context.
	Notes string `json:"notes,omitempty"`

	// AgentRunIDs are CodeValdAI AgentRun IDs linked to this run.
	// Opaque strings — CodeValdWork does not validate them.
	AgentRunIDs []string `json:"agent_run_ids,omitempty"`

	// FunctionJobIDs are CodeValdFunctions job IDs linked to this run.
	FunctionJobIDs []string `json:"function_job_ids,omitempty"`

	// BranchNames are git branch names linked to this run.
	BranchNames []string `json:"branch_names,omitempty"`

	// TerminalEvent is an optional colon-delimited condition
	// (topic:field=value:field=value) that, when matched by an inbound event,
	// transitions the run from in_progress to completed automatically.
	// Empty means the run never auto-completes — operator must POST /complete.
	TerminalEvent string `json:"terminal_event,omitempty"`

	// StartedAt is the RFC 3339 timestamp the run began execution.
	StartedAt string `json:"started_at,omitempty"`

	// CompletedAt is the RFC 3339 timestamp the run reached a terminal status.
	CompletedAt string `json:"completed_at,omitempty"`

	// CreatedAt is the RFC 3339 timestamp the run vertex was first created.
	CreatedAt string `json:"created_at"`

	// UpdatedAt is the RFC 3339 timestamp of the most recent mutation.
	UpdatedAt string `json:"updated_at"`

	// ParentWorkflowRunID references the WorkflowRun whose failure spawned
	// this child (recovery) run. Empty on top-level runs. See FEAT-20260602-007.
	ParentWorkflowRunID string `json:"parent_workflow_run_id,omitempty"`

	// RootWorkflowRunID is the top-of-chain run ID — denormalised so Cross
	// can read/increment the root counter in O(1). For top-level runs this
	// equals ID (or is empty; consumers default to ID). Carried on every
	// downstream event and artifact for one-query closure aggregation.
	RootWorkflowRunID string `json:"root_workflow_run_id,omitempty"`

	// FailurePipelineBudget is the maximum number of recovery-pipeline
	// activations allowed under this run's lineage. Lives only on the root
	// run. Resolved by start-pipeline (payload > agency > env default) and
	// frozen for the run's lifetime.
	FailurePipelineBudget int `json:"failure_pipeline_budget,omitempty"`

	// FailurePipelinesUsed counts recovery activations charged to this root
	// run. Atomically incremented by IncrementFailureBudget. Lives only on
	// the root run.
	FailurePipelinesUsed int `json:"failure_pipelines_used,omitempty"`

	// CountedChildRunIDs is the dedup set of child run IDs already charged
	// to FailurePipelinesUsed — supports idempotent retries of
	// IncrementFailureBudget.
	CountedChildRunIDs []string `json:"counted_child_run_ids,omitempty"`

	// CancelledBy is the authenticated identity of the caller who issued the
	// cancel API call (FEAT-20260602-008). Empty for runs that have never
	// been cancelled.
	CancelledBy string `json:"cancelled_by,omitempty"`

	// CancelReason is the human-readable explanation supplied by the caller
	// when cancelling the run (FEAT-20260602-008). Carried in the
	// work.run.cancelling and work.run.cancelled event payloads.
	CancelReason string `json:"cancel_reason,omitempty"`

	// CancellingUntil is the RFC 3339 timestamp marking the quiesce deadline.
	// The finalization step transitions the run from cancelling → cancelled
	// at or after this time (FEAT-20260602-008). Empty for non-cancelled runs.
	CancellingUntil string `json:"cancelling_until,omitempty"`

	// LastEventAt is the RFC 3339 timestamp of the most recent event carrying
	// this run's workflow_run_id. Initialised to created_at; bumped by Cross
	// on every observed event (best-effort). Used by the watchdog sweeper.
	LastEventAt string `json:"last_event_at,omitempty"`

	// TimeoutPublished records that work.run.timeout was published for this
	// run. The sweeper skips runs with TimeoutPublished=true to avoid
	// duplicate timeout events across restarts.
	TimeoutPublished bool `json:"timeout_published,omitempty"`

	// PausedAt, when non-empty, suspends watchdog sweeping for this run.
	// Reserved for the future operator-pause API — set it now so the AQL
	// queries and watchdog are correct before the pause endpoint ships.
	PausedAt string `json:"paused_at,omitempty"`

	// CurrentStepID is the plan code of the step currently executing. Set
	// by Cross when it dispatches the step's trigger event; cleared when the
	// step's terminal event is observed. Used by the per-step sweep pass.
	CurrentStepID string `json:"current_step_id,omitempty"`

	// CurrentStepStartedAt is the RFC 3339 timestamp when CurrentStepID
	// was set. Used with the step's step_timeout to detect stalled steps.
	CurrentStepStartedAt string `json:"current_step_started_at,omitempty"`
}

// WorkflowRunClosure is the full read returned by GetWorkflowRunClosure —
// the run anchor plus every Task / TaskTodo / Relationship in its closure
// and the IDs of foreign (cross-service) entities it referenced.
//
// Edges include every relationship whose endpoints sit in the closure,
// including edges to entities OUTSIDE the closure — the caller needs this
// to compensate cross-closure links during rollback.
type WorkflowRunClosure struct {
	Run            WorkflowRun    `json:"run"`
	Tasks          []Task         `json:"tasks"`
	Todos          []TaskTodo     `json:"todos"`
	Edges          []Relationship `json:"edges"`
	AgentRunIDs    []string       `json:"agent_run_ids,omitempty"`
	FunctionJobIDs []string       `json:"function_job_ids,omitempty"`
	BranchNames    []string       `json:"branch_names,omitempty"`
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
