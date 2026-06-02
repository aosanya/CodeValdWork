// Package codevaldwork provides task lifecycle management for CodeValdCortex
// agencies. It exposes [TaskManager] — the single interface for creating,
// reading, updating, deleting, and listing tasks assigned to AI agents.
//
// Storage is delegated to a [github.com/aosanya/CodeValdSharedLib/entitygraph.DataManager],
// so Tasks live in the agency-scoped graph alongside every other CodeVald entity
// type. Use storage/arangodb.NewBackend to construct a DataManager and pass it
// to [NewTaskManager].
//
// Implementation is split across focused files:
//   - task_impl_task.go       — CreateTask, GetTask, UpdateTask, DeleteTask, ListTasks
//   - task_impl_import.go     — ImportProject + async job infrastructure
//   - task_impl_converters.go — entity↔domain converters
//   - project.go              — Project CRUD + membership edges
//   - assignment.go           — AssignTask / UnassignTask
//   - agent.go                — UpsertAgent, GetAgent, ListAgents
//   - relationship.go         — CreateRelationship, DeleteRelationship, TraverseRelationships
package codevaldwork

import (
	"context"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// taskTypeID is the TypeDefinition.Name used for Task entities in the schema.
const taskTypeID = "Task"

// projectTypeID is the TypeDefinition.Name used for Project entities.
const projectTypeID = "Project"

// agentTypeID is the TypeDefinition.Name used for Agent entities.
const agentTypeID = "Agent"

// tagTypeID is the TypeDefinition.Name used for Tag entities.
const tagTypeID = "Tag"

// taskTodoTypeID is the TypeDefinition.Name used for TaskTodo entities.
const taskTodoTypeID = "TaskTodo"

// workflowRunTypeID is the TypeDefinition.Name used for WorkflowRun entities —
// the anchor that names every Task / TaskTodo in a single orchestrated run
// (FEAT-20260601-001).
const workflowRunTypeID = "WorkflowRun"

// TaskManager is the primary interface for task lifecycle management.
// All operations are scoped to the manager's agencyID, fixed at construction.
//
// Implementations must be safe for concurrent use.
type TaskManager interface {
	// CreateTask creates a new task for the agency.
	// The task is assigned a server-generated ID and starts in [TaskStatusPending].
	CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)

	// GetTask retrieves a single task by its ID within the given agency.
	// Returns [ErrTaskNotFound] if no matching task exists.
	GetTask(ctx context.Context, agencyID, taskID string) (Task, error)

	// UpdateTask replaces the mutable fields of an existing task.
	// Status transitions are validated — returns [ErrInvalidStatusTransition]
	// if the new status is not reachable from the current status.
	// Returns [ErrTaskNotFound] if the task does not exist.
	UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error)

	// DeleteTask soft-deletes a task from the agency graph.
	// Returns [ErrTaskNotFound] if the task does not exist.
	DeleteTask(ctx context.Context, agencyID, taskID string) error

	// ListTasks returns all non-deleted tasks for the given agency that match
	// the filter. Returns an empty slice (not an error) when no tasks match.
	ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error)

	// CreateRelationship creates a directed edge between two Work vertices.
	// rel.Label must be one of the RelLabel* constants and the (FromID, ToID)
	// vertex types must match the label's whitelist entry, otherwise
	// [ErrInvalidRelationship] is returned.
	//
	// Both endpoints must already exist in the same agency. A missing endpoint
	// returns [ErrTaskNotFound], [ErrAgentNotFound], or [ErrProjectNotFound]
	// depending on the label's expected vertex type.
	//
	// Re-creating an existing (FromID, ToID, Label) edge is idempotent — the
	// existing edge is returned with no error and no second edge is written.
	CreateRelationship(ctx context.Context, agencyID string, rel Relationship) (Relationship, error)

	// DeleteRelationship removes a single edge identified by the
	// (fromID, toID, label) triple within the agency. Returns
	// [ErrRelationshipNotFound] if no such edge exists.
	DeleteRelationship(ctx context.Context, agencyID, fromID, toID, label string) error

	// TraverseRelationships returns the single-hop edges incident on
	// vertexID with the given label and direction.
	//
	// Returns an empty slice (not an error) when no edges match.
	TraverseRelationships(ctx context.Context, agencyID, vertexID, label string, dir Direction) ([]Relationship, error)

	// UpsertAgent creates or merges an Agent vertex keyed by the
	// (agencyID, agent.AgentID) natural key. On the merge branch, display_name
	// and capability are updated; agent_id is immutable.
	UpsertAgent(ctx context.Context, agencyID string, agent Agent) (Agent, error)

	// GetAgent retrieves a single Agent by either its entity UUID or its
	// AgentID slug (e.g. "developer-01"). UUID lookup is tried first; on
	// NotFound it falls back to a slug match. Returns [ErrAgentNotFound] if
	// neither form resolves.
	GetAgent(ctx context.Context, agencyID, idOrSlug string) (Agent, error)

	// GetAgentByAgentID retrieves an Agent by its external slug
	// (the same `agent_id` UpsertAgent uses as the natural key). Returns
	// [ErrAgentNotFound] when no Agent in the agency has the slug.
	GetAgentByAgentID(ctx context.Context, agencyID, agentIDSlug string) (Agent, error)

	// ListAgents returns all non-deleted Agents in the agency. Returns an
	// empty slice (not an error) when none exist.
	ListAgents(ctx context.Context, agencyID string) ([]Agent, error)

	// AssignTask sets the assignee of a Task by writing the `assigned_to`
	// edge (Task → Agent). Replaces any prior assignee — a Task has at
	// most one outbound `assigned_to` edge.
	//
	// workflowRunID propagates the WorkflowRun anchor from the inbound event
	// payload onto the Task per the FEAT-20260602-002 chain-through rule.
	// Semantics when non-empty:
	//   • If the Task's stored WorkflowRunID is empty, it is set to the new
	//     value AND the started_task edge from run→task is written.
	//   • If the stored WorkflowRunID equals the new value, it is a no-op.
	//   • If the stored WorkflowRunID differs, returns [ErrWorkflowRunMismatch]
	//     — a task belonging to two runs breaks the rollback invariant.
	// Empty workflowRunID is permitted and preserves the existing value.
	//
	// Returns [ErrTaskNotFound] or [ErrAgentNotFound] when the respective
	// vertex is missing.
	AssignTask(ctx context.Context, agencyID, taskID, agentID, workflowRunID string) error

	// UnassignTask removes any outbound `assigned_to` edge from the Task.
	// Idempotent — returns nil whether or not an edge was present.
	UnassignTask(ctx context.Context, agencyID, taskID string) error

	// UnblockDependents transitions every Task with an inbound `depends_on`
	// edge from completedTaskID out of [TaskStatusBlocked] when all of its
	// outbound `depends_on` edges now point to terminal Tasks, and re-fires
	// [TopicTaskAssigned] for the cached assignee so CodeValdAI sees the
	// dispatch it previously missed. Tasks without an `assigned_to` edge
	// are left blocked (nothing to dispatch). Idempotent — invoking it on
	// the same completedTaskID a second time is a no-op once each dependent
	// has already moved out of blocked. Per-dependent failures are logged
	// and other dependents continue.
	UnblockDependents(ctx context.Context, agencyID, completedTaskID string) error

	// CreateProject creates a new Project vertex. Returns
	// [ErrInvalidTask] when Name is empty and [ErrProjectAlreadyExists]
	// when the underlying store reports a duplicate.
	CreateProject(ctx context.Context, agencyID string, p Project) (Project, error)

	// GetProject retrieves a single Project by entity ID. Returns
	// [ErrProjectNotFound] if no matching project exists.
	GetProject(ctx context.Context, agencyID, projectID string) (Project, error)

	// GetProjectByName retrieves a single Project by its slug (project_name).
	// Returns [ErrProjectNotFound] if no project with that slug exists.
	GetProjectByName(ctx context.Context, agencyID, projectName string) (Project, error)

	// UpdateProject replaces the mutable fields of an existing Project.
	// Returns [ErrProjectNotFound] if the project does not exist.
	UpdateProject(ctx context.Context, agencyID string, p Project) (Project, error)

	// DeleteProject soft-deletes the Project vertex AND removes every
	// inbound `member_of` edge so member Tasks lose the membership.
	// Member Tasks themselves are not deleted.
	DeleteProject(ctx context.Context, agencyID, projectID string) error

	// ListProjects returns all non-deleted Projects in the agency.
	ListProjects(ctx context.Context, agencyID string) ([]Project, error)

	// AddTaskToProject writes the `member_of` edge from taskID to projectID.
	// Idempotent — returns nil whether or not the edge already existed.
	AddTaskToProject(ctx context.Context, agencyID, taskID, projectID string) error

	// RemoveTaskFromProject removes the `member_of` edge from taskID to
	// projectID. Returns [ErrRelationshipNotFound] if no membership existed.
	RemoveTaskFromProject(ctx context.Context, agencyID, taskID, projectID string) error

	// ListTasksInProject returns the Tasks belonging to the given project via
	// inbound `member_of` edges.
	ListTasksInProject(ctx context.Context, agencyID, projectID string) ([]Task, error)

	// ListProjectsForTask returns the Projects the given Task belongs to
	// via outbound `member_of` edges.
	ListProjectsForTask(ctx context.Context, agencyID, taskID string) ([]Project, error)

	// GetTaskByName retrieves a task by its project-scoped name (task_name)
	// within a project (project_name). Returns [ErrTaskNotFound] if no task
	// with that name exists in the project.
	GetTaskByName(ctx context.Context, agencyID, projectName, taskName string) (Task, error)

	// CreateTaskInProject creates a task, auto-generates its task_name from the
	// project's task_prefix, and writes the member_of edge in one atomic sequence.
	// Returns [ErrProjectNotFound] if the project does not exist.
	CreateTaskInProject(ctx context.Context, agencyID, projectName string, task Task) (Task, error)

	// ImportProject parses a JSON import document and creates a Project,
	// Tasks, member_of edges, and depends_on edges in a single call.
	// Returns [ErrInvalidImport] when the document is malformed.
	ImportProject(ctx context.Context, agencyID, document string) (ImportResult, error)

	// StartImportProject begins an async import of a JSON project document.
	// Returns immediately with an [ImportProjectJob] whose ID can be passed to
	// [GetImportProjectStatus] to poll for progress.
	// Returns [ErrInvalidImport] when the document cannot be parsed.
	StartImportProject(ctx context.Context, agencyID, document string) (ImportProjectJob, error)

	// GetImportProjectStatus returns the current state of an async import job.
	// Returns [ErrImportJobNotFound] if no job with the given ID exists for
	// this agency.
	GetImportProjectStatus(ctx context.Context, agencyID, jobID string) (ImportProjectJob, error)

	// CancelImportProject cancels a pending or running import job. Returns
	// [ErrImportJobNotFound] if the job does not exist, or
	// [ErrImportJobNotCancellable] if it has already reached a terminal state.
	CancelImportProject(ctx context.Context, agencyID, jobID string) error

	// CreateTaskTodo creates a new TaskTodo entity for a decomposed sub-task.
	// Required fields: Title, Instructions, ParentTaskID. Returns [ErrInvalidTask]
	// if any required field is empty.
	// Todos with non-empty DependsOn are created with [TodoStatusBlocked] and
	// are NOT dispatched — call [DispatchTaskTodo] once their predecessors complete.
	// Todos with no dependencies start as [TodoStatusPending]; callers should
	// immediately call [DispatchTaskTodo] for them.
	CreateTaskTodo(ctx context.Context, agencyID string, todo TaskTodo) (TaskTodo, error)

	// DispatchTaskTodo publishes [TopicTodoDispatched] for an existing todo,
	// making it available to CodeValdAI agents via their work plans.
	// If the todo is currently [TodoStatusBlocked], it is first transitioned to
	// [TodoStatusPending] before the event is published.
	// Returns [ErrTaskTodoNotFound] if the todo does not exist.
	DispatchTaskTodo(ctx context.Context, agencyID, todoID string) error

	// GetTaskTodo retrieves a single TaskTodo by its entity ID within the given agency.
	// Returns [ErrTaskTodoNotFound] if no matching todo exists.
	GetTaskTodo(ctx context.Context, agencyID, todoID string) (TaskTodo, error)

	// UpdateTaskTodoStatus transitions a TaskTodo to a new [TodoStatus].
	// Returns [ErrTaskTodoNotFound] if the todo does not exist.
	UpdateTaskTodoStatus(ctx context.Context, agencyID, todoID string, status TodoStatus) (TaskTodo, error)

	// CreateWorkflowRun anchors a new orchestrated execution. The returned
	// run is in [WorkflowRunStatusPending]; producers transition it to
	// in_progress / completed / failed via UpdateWorkflowRunStatus.
	//
	// name may be empty — the server generates a unique label of the form
	// `pipeline-YYYY-MM-DD-HHMMSS-<6hex>` in that case. When name is set,
	// it must not have leading/trailing whitespace and must not collide
	// with an existing run in the same agency (otherwise
	// [ErrWorkflowRunNameExists]).
	CreateWorkflowRun(ctx context.Context, agencyID, name, triggerEvent, initiator string) (WorkflowRun, error)

	// CreateRecoveryWorkflowRun mints a child WorkflowRun spawned by Cross's
	// failure dispatch (FEAT-20260602-007). The child carries
	// parent_workflow_run_id and root_workflow_run_id; the budget counter
	// lives on the root and is incremented by IncrementFailureBudget.
	CreateRecoveryWorkflowRun(ctx context.Context, agencyID, name, triggerEvent, initiator, parentRunID, rootRunID string) (WorkflowRun, error)

	// SetFailureBudget locks the failure_pipeline_budget on a root
	// WorkflowRun (FEAT-20260602-007). The budget is frozen for the lifetime
	// of the run; resetting it returns [ErrFailureBudgetAlreadySet]. Acting
	// on a non-root run returns [ErrNotRootWorkflowRun].
	SetFailureBudget(ctx context.Context, agencyID, runID string, budget int) (WorkflowRun, error)

	// IncrementFailureBudget atomically increments failure_pipelines_used on
	// the root WorkflowRun (FEAT-20260602-007). Idempotent on childRunID:
	// repeated calls with the same childRunID return the current counter
	// state without double-incrementing. The exhausted return is true iff
	// `used > budget` after the increment; Cross must skip the recovery
	// dispatch and fail the run on exhausted.
	IncrementFailureBudget(ctx context.Context, agencyID, rootRunID, childRunID string) (used, budget int, exhausted bool, err error)

	// GetWorkflowRun reads a single WorkflowRun by entity ID within the
	// given agency. Returns [ErrWorkflowRunNotFound] when no matching
	// entity exists.
	GetWorkflowRun(ctx context.Context, agencyID, runID string) (WorkflowRun, error)

	// GetWorkflowRunByName looks up a single run by its unique (agencyID,
	// name) pair. Returns [ErrWorkflowRunNotFound] when no match exists.
	GetWorkflowRunByName(ctx context.Context, agencyID, name string) (WorkflowRun, error)

	// ListWorkflowRuns returns every WorkflowRun in the agency, newest first.
	// When name is non-empty the result is filtered to runs whose Name
	// matches exactly (at most one row). Used by the frontend list view
	// (FEAT-20260601-002) and by the QA test scripts that correlate a run
	// by a caller-supplied label.
	ListWorkflowRuns(ctx context.Context, agencyID, name string) ([]WorkflowRun, error)

	// LinkTaskToRun writes the `started_task` edge from runID to taskID
	// (and relies on the schema-declared inverse `part_of_run` for reverse
	// lookups). Idempotent — re-linking is a no-op.
	LinkTaskToRun(ctx context.Context, agencyID, runID, taskID string) error

	// LinkTodoToRun writes the `started_todo` edge from runID to todoID.
	// Idempotent.
	LinkTodoToRun(ctx context.Context, agencyID, runID, todoID string) error

	// GetWorkflowRunClosure returns the run plus every entity and edge
	// reachable from it. See [WorkflowRunClosure] for the closure semantics.
	GetWorkflowRunClosure(ctx context.Context, agencyID, runID string) (WorkflowRunClosure, error)

	// UpdateWorkflowRunStatus transitions a WorkflowRun to a new lifecycle status.
	// Valid transitions are defined by [WorkflowRunStatus.CanTransitionTo].
	// reason is stored in the failure_reason field when transitioning to failed
	// or rollback_failed.
	UpdateWorkflowRunStatus(ctx context.Context, agencyID, runID string, newStatus WorkflowRunStatus, reason string) (WorkflowRun, error)

	// RollbackWorkflowRun orchestrates the compensation sequence for the given
	// run. Valid only when the run is in failed or completed status.
	//
	// Sequence:
	//  1. Transitions run → rolling_back; publishes work.run.rolling_back.
	//  2. Calls per-service compensation stubs (cross-service legs are no-ops
	//     until each service implements DELETE /by-workflow-run/{id}).
	//  3. Hard-deletes CodeValdWork artifacts (Tasks + TaskTodos + their edges)
	//     via [DeleteWorkflowRunArtifacts].
	//  4. On success: transitions run → rolled_back; publishes work.run.rolled_back.
	//     On partial failure: transitions run → rollback_failed; publishes
	//     work.run.rollback_failed. Operator can re-trigger after remediation.
	//
	// Returns [ErrWorkflowRunNotFound] when the run does not exist,
	// [ErrRollbackConflict] when already rolling_back,
	// [ErrInvalidRunStatusTransition] for any other disallowed source status,
	// [ErrForeignRunDependency] when a Task in the closure is depended on by
	// a Task from a different run (the dependent run must be rolled back first).
	RollbackWorkflowRun(ctx context.Context, agencyID, runID, reason string) (WorkflowRun, error)

	// DeleteWorkflowRunArtifacts hard-deletes every Task and TaskTodo whose
	// workflow_run_id matches runID, along with all edges incident on those
	// entities (started_task, started_todo, has_todo, assigned_to, depends_on).
	// Emits work.task.rolled_back for each deleted Task for observability.
	//
	// This is the CodeValdWork leg of the per-service DELETE contract described
	// in FEAT-20260602-004. Called by [RollbackWorkflowRun] as part of the
	// orchestration; may also be called directly for operator-driven remediation.
	//
	// Returns [ErrWorkflowRunNotFound] when the run does not exist.
	// A run with no Tasks is a valid no-op.
	DeleteWorkflowRunArtifacts(ctx context.Context, agencyID, runID string) error

	// CancelWorkflowRun flips an in_progress WorkflowRun to the cancelling
	// transient state, persists the (cancelledBy, reason, quiesceDeadline)
	// envelope on the run row, cascades work.task.cancelled to every
	// non-terminal Task in the run, and publishes work.run.cancelling
	// (FEAT-20260602-008).
	//
	// Idempotent: a repeated call on an already-cancelling run returns the
	// stored cancellation envelope without re-firing events or shifting the
	// quiesce deadline.
	//
	// Returns:
	//   - [ErrWorkflowRunNotFound] when the run does not exist.
	//   - [ErrCannotCancelTerminalRun] when the run is in any status other than
	//     in_progress or cancelling.
	CancelWorkflowRun(ctx context.Context, agencyID, runID, reason, cancelledBy string, quiesceDeadline time.Time) (WorkflowRun, error)

	// FinalizeWorkflowRunCancellation transitions a cancelling WorkflowRun to
	// the cancelled terminal state and publishes work.run.cancelled
	// (FEAT-20260602-008). Best-effort: called by the gRPC handler's
	// finalization goroutine after the quiesce deadline elapses, but may be
	// called directly by tests.
	//
	// Idempotent: a call on a run that is no longer in cancelling status (e.g.
	// already cancelled) returns the current run without effect.
	//
	// Returns [ErrWorkflowRunNotFound] when the run does not exist.
	FinalizeWorkflowRunCancellation(ctx context.Context, agencyID, runID string) (WorkflowRun, error)
}

// WorkSchemaManager is a type alias for [entitygraph.SchemaManager].
// Used by internal/app to seed [DefaultWorkSchema] on startup.
type WorkSchemaManager = entitygraph.SchemaManager

// CrossPublisher is a type alias for [eventbus.Publisher] — the SharedLib
// package that unifies the publish contract across CodeVald services.
type CrossPublisher = eventbus.Publisher

// taskManager is the concrete implementation of [TaskManager].
type taskManager struct {
	dm        entitygraph.DataManager
	publisher eventbus.Publisher // optional; nil = skip event publishing
}

// NewTaskManager constructs a [TaskManager] backed by the given
// [entitygraph.DataManager].
// pub may be nil — cross-service events are skipped when no publisher is set.
// Returns an error if dm is nil.
func NewTaskManager(dm entitygraph.DataManager, pub eventbus.Publisher) (TaskManager, error) {
	if dm == nil {
		return nil, fmt.Errorf("NewTaskManager: data manager must not be nil")
	}
	return &taskManager{dm: dm, publisher: pub}, nil
}
