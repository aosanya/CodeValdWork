package codevaldwork

import "github.com/aosanya/CodeValdSharedLib/types"

// Event topic constants — the closed set CodeValdWork publishes.
const (
	// TopicTaskCreated fires after a Task entity is created.
	// Payload: [TaskCreatedPayload].
	TopicTaskCreated = "work.task.created"

	// TopicTaskUpdated fires after a non-status mutable field changes.
	// Payload: [TaskUpdatedPayload].
	TopicTaskUpdated = "work.task.updated"

	// TopicTaskStatusChanged fires on every successful status transition.
	// Payload: [TaskStatusChangedPayload].
	TopicTaskStatusChanged = "work.task.status.changed"

	// TopicTaskCompleted fires when a transition reaches a terminal status
	// (completed, failed, cancelled). Published in addition to
	// [TopicTaskStatusChanged]. Payload: [TaskCompletedPayload].
	TopicTaskCompleted = "work.task.completed"

	// TopicTaskFailed fires when an AI agent run fails to satisfy the required
	// output contract (e.g. no actions block emitted). Published in addition to
	// [TopicTaskCompleted] when the failure is agent-driven.
	// Payload: [TaskFailedPayload].
	TopicTaskFailed = "work.task.failed"

	// TopicTaskAssigned fires when an `assigned_to` edge is created or
	// replaced. Payload: [TaskAssignedPayload].
	TopicTaskAssigned = "work.task.assigned"

	// TopicRelationshipCreated fires when any whitelisted graph edge is
	// created. Payload: [RelationshipCreatedPayload].
	TopicRelationshipCreated = "work.relationship.created"

	// TopicTodoDispatched fires when a [TaskTodo] entity is created — once per
	// todo item produced by an ai.todo.created decomposition payload. CodeValdAI
	// agents subscribe to this topic via work plans and execute each todo.
	// Payload: [TodoDispatchedPayload].
	TopicTodoDispatched = "work.todo.dispatched"

	// TopicTodoCompleted fires when a TaskTodo reaches a terminal status
	// (completed or failed). Carries todo_type and max_runs so downstream
	// consumers (e.g. compile-on-todo-completed) can act without fetching
	// the entity separately.
	// Payload: [TodoCompletedPayload].
	TopicTodoCompleted = "work.todo.completed"

	// TopicTaskUpdate is consumed by CodeValdWork to patch mutable task fields.
	// Published by CodeValdAI when the LLM emits a work.task.update action
	// (e.g. after choosing a branch name). Currently only branch_name is patched.
	// Payload: [TaskUpdatePayload].
	TopicTaskUpdate = "work.task.update"

	// TopicTaskRolledBack fires once per Task deleted by [DeleteWorkflowRunArtifacts].
	// Payload: [TaskRolledBackPayload].
	TopicTaskRolledBack = "work.task.rolled_back"

	// TopicTaskCancelled fires once per non-terminal Task whose status was
	// flipped to cancelled by the run-cancel cascade (FEAT-20260602-008).
	// CodeValdAI / CodeValdFunctions subscribe to drop in-flight work for
	// the task. Payload: [TaskCancelledPayload].
	TopicTaskCancelled = "work.task.cancelled"

	// TopicPipelineStarted is published by start-pipeline (CodeValdFunctions)
	// immediately after a WorkflowRun is minted. CodeValdWork subscribes so
	// RunStatusHandler can flip the run from PENDING → IN_PROGRESS without
	// waiting for the first work.task.assigned event (BUG-09-025 fix).
	TopicPipelineStarted = "work.pipeline.started"

	// TopicTaskNeedsDirection is published by CodeValdWork when a task has
	// exhausted its retry budget and requires direction to resume. The payload
	// carries a [DirectionForm] JSON object renderable on any platform.
	// Payload: [TaskNeedsDirectionPayload].
	TopicTaskNeedsDirection = "work.task.needs-direction"

	// TopicTaskDirection is consumed by CodeValdWork from the bus — emitted by
	// CodeValdAI (failure-direction-handler) or the frontend after the human
	// resolves the direction form. Routes to the direction handler which resumes
	// the task according to selected_option.
	// Payload: [TaskDirectionPayload].
	TopicTaskDirection = "work.task.direction"

	// TopicTaskClassifyFailure is published by CodeValdWork after a task exhausts
	// its automatic retry budget. CodeValdAI subscribes and responds with
	// [TopicTaskFailureClassified]. Payload: [TaskClassifyFailurePayload].
	TopicTaskClassifyFailure = "work.task.classify-failure"

	// TopicTaskFailureClassified is consumed by CodeValdWork — emitted by
	// CodeValdAI in response to [TopicTaskClassifyFailure]. Carries a
	// failure_type of "transient" or "requires-human" so Work can decide
	// whether to grant one extra retry or escalate to human direction.
	// Payload: [TaskFailureClassifiedPayload].
	TopicTaskFailureClassified = "work.task.failure-classified"

	// TopicRunPaused fires when a WorkflowRun transitions to paused status
	// because at least one task is awaiting direction.
	// Payload: [RunPausedPayload].
	TopicRunPaused = "work.run.paused"

	// TopicRunResumed fires when a paused WorkflowRun transitions back to
	// in_progress after all awaiting-direction tasks have been resolved.
	// Payload: [RunResumedPayload].
	TopicRunResumed = "work.run.resumed"
)

// AllTopics is the full list of topics this service publishes.
// Schema-derived lifecycle topics are generated automatically from
// [DefaultWorkSchema]; business-semantic extras are appended below.
func AllTopics() []string {
	derived := types.TopicsFromSchema("work", DefaultWorkSchema())
	return append(derived,
		TopicTaskCompleted,
		TopicTaskFailed,
		TopicTaskAssigned,
		TopicRelationshipCreated,
		TopicTodoDispatched,
		TopicTodoCompleted,
		TopicTaskRolledBack,
		TopicRunRollingBack,
		TopicRunRolledBack,
		TopicRunRollbackFailed,
		TopicTaskCancelled,
		TopicRunCancelling,
		TopicRunCancelled,
		TopicRunTimeout,
		TopicTaskTimeout,
		TopicTaskNeedsDirection,
		TopicRunPaused,
		TopicRunResumed,
		TopicTaskClassifyFailure,
	)
}

// TaskCreatedPayload is the [Event.Payload] for [TopicTaskCreated].
type TaskCreatedPayload struct {
	TaskID   string
	Priority TaskPriority
	// WorkflowRunID is the WorkflowRun anchor this task belongs to, or empty
	// when the task was created outside an orchestrated run
	// (FEAT-20260602-002 chain-through rule).
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskUpdatedPayload is the [Event.Payload] for [TopicTaskUpdated].
type TaskUpdatedPayload struct {
	TaskID        string
	ChangedFields []string
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskStatusChangedPayload is the [Event.Payload] for [TopicTaskStatusChanged].
type TaskStatusChangedPayload struct {
	TaskID string
	From   TaskStatus
	To     TaskStatus
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskCompletedPayload is the [Event.Payload] for [TopicTaskCompleted].
type TaskCompletedPayload struct {
	TaskID         string
	TerminalStatus TaskStatus
	// CompletedAt is the RFC 3339 timestamp when the terminal status was set.
	CompletedAt string
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskFailedBy identifies the agent and work plan responsible for a task failure.
type TaskFailedBy struct {
	AgentID      string
	WorkPlanID   string
	WorkPlanCode string
}

// TaskFailedPayload is the [Event.Payload] for [TopicTaskFailed].
// Consumers needing the raw LLM output should fetch the AgentRun from CodeValdAI using RunID.
type TaskFailedPayload struct {
	TaskID   string
	RunID    string
	Reason   string
	FailedBy TaskFailedBy
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskRolledBackPayload is the [Event.Payload] for [TopicTaskRolledBack].
// Emitted once per Task deleted by [DeleteWorkflowRunArtifacts].
type TaskRolledBackPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id"`
}

// TaskAssignedPayload is the [Event.Payload] for [TopicTaskAssigned].
type TaskAssignedPayload struct {
	TaskID      string
	AgentID     string
	RoleName    string
	TaskCode    string // project-scoped code, e.g. "UTIL-001" — empty for tasks not in a project
	Title       string
	Description string
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// RelationshipCreatedPayload is the [Event.Payload] for [TopicRelationshipCreated].
type RelationshipCreatedPayload struct {
	FromID string
	ToID   string
	Label  string
}

// TodoDispatchedPayload is the [Event.Payload] for [TopicTodoDispatched].
// Published once per TaskTodo entity created from an ai.todo.created decomposition.
//
// Field identity contract (important for consumers):
//   - TodoID        is the identity of the todo itself. CodeValdAI sets the
//     AgentRun.TaskID to this value so that ai.task.completed/failed events
//     route back to the todo (via updateTodoStatus), not to the parent task.
//   - TaskID        equals ParentTaskID and exists only for HydrateEventContext
//     which needs the parent task ID under the key "TaskID" to fetch task
//     context. It MUST NOT be used to set AgentRun.TaskID.
//   - ParentTaskID  is the canonical parent task identifier.
type TodoDispatchedPayload struct {
	TodoID         string
	TaskID         string // equals ParentTaskID; for HydrateEventContext only — do not use as AgentRun.TaskID
	ParentTaskID   string
	DecompRunID    string
	AgentID        string
	Title          string
	Instructions   string
	Ordinality     int
	CanRunParallel bool
	DependsOn      []int
	Precalls       string // JSON-encoded []PrecallSpec stored on the TaskTodo
	TodoType       string // semantic type label (e.g. "compile-fix"); used for per-type run-count enforcement
	MaxRuns        int    // maximum spawns of this todo type within the parent task; 0 means no limit
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TodoCompletedPayload is the [Event.Payload] for [TopicTodoCompleted].
// Published when a TaskTodo reaches a terminal status (completed or failed).
type TodoCompletedPayload struct {
	TodoID       string `json:"todo_id"`
	ParentTaskID string `json:"task_id"`    // keyed "task_id" so CodeValdFunctions' dispatcher can extract it
	Title        string `json:"title"`
	Status       string `json:"status"`     // "completed" or "failed"
	TodoType     string `json:"todo_type"`  // forwarded from the TaskTodo entity; empty when no type was set
	MaxRuns      int    `json:"max_runs,omitempty"`  // forwarded from the TaskTodo entity; 0 means no limit
	RunCount     int    `json:"run_count,omitempty"` // current count of todos of this TodoType for the parent task at the time of completion
	// WorkflowRunID propagates the run anchor onto every work.* event payload.
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
}

// TaskUpdatePayload is the [Event.Payload] for [TopicTaskUpdate].
// Published by CodeValdAI when the LLM emits a work.task.update action.
type TaskUpdatePayload struct {
	// TaskID is the Work Task entity ID to patch.
	TaskID string `json:"task_id"`
	// BranchName is the git branch the AI agent created for this task.
	// Written back so HydrateEventContext can use it for file hydration.
	BranchName string `json:"branch_name,omitempty"`
}

// WorkflowRun status event topic constants.
const (
	// TopicRunInProgress fires when a WorkflowRun first transitions to in_progress.
	TopicRunInProgress = "work.run.in_progress"
	// TopicRunCompleted fires when a WorkflowRun reaches the completed terminal state.
	TopicRunCompleted = "work.run.completed"
	// TopicRunFailed fires when a WorkflowRun reaches the failed terminal state.
	TopicRunFailed = "work.run.failed"
	// TopicRunRolledBack fires when a WorkflowRun reaches the rolled_back terminal state.
	TopicRunRolledBack = "work.run.rolled_back"
	// TopicRunRollingBack fires when the rollback coordinator begins compensating artifacts.
	// In-flight handlers should check the run status and quiesce on receiving this event.
	TopicRunRollingBack = "work.run.rolling_back"
	// TopicRunRollbackFailed fires when the rollback coordinator encountered a partial
	// failure and the run reached rollback_failed. Operator intervention is required.
	TopicRunRollbackFailed = "work.run.rollback_failed"
	// TopicRunCancelling fires when an operator-issued cancel transitions a
	// WorkflowRun from in_progress to the cancelling transient state
	// (FEAT-20260602-008). In-flight subscribers should quiesce their work
	// on behalf of the run.
	TopicRunCancelling = "work.run.cancelling"
	// TopicRunCancelled fires when the cancellation finalization step
	// transitions a WorkflowRun from cancelling to the cancelled terminal
	// state (FEAT-20260602-008).
	TopicRunCancelled = "work.run.cancelled"

	// TopicRunTimeout fires when the watchdog detects a WorkflowRun has been
	// in_progress for longer than its inactivity timeout without any event.
	// CodeValdWork subscribes and flips the run to failed (FEAT-20260602-006).
	TopicRunTimeout = "work.run.timeout"

	// TopicTaskTimeout fires when the watchdog detects a per-step stall
	// (current_step_started_at older than step_timeout) (FEAT-20260602-006).
	TopicTaskTimeout = "work.task.timeout"
)

// WorkflowRunInProgressPayload is the Payload for [TopicRunInProgress].
type WorkflowRunInProgressPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	StartedAt     string `json:"started_at"`
}

// WorkflowRunCompletedPayload is the Payload for [TopicRunCompleted].
type WorkflowRunCompletedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	CompletedAt   string `json:"completed_at"`
	DurationMs    int64  `json:"duration_ms,omitempty"`
}

// WorkflowRunFailedPayload is the Payload for [TopicRunFailed].
type WorkflowRunFailedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	FailedAt      string `json:"failed_at"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// WorkflowRunRolledBackPayload is the Payload for [TopicRunRolledBack].
type WorkflowRunRolledBackPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	RolledBackAt  string `json:"rolled_back_at"`
	Reason        string `json:"reason,omitempty"`
}

// WorkflowRunRollingBackPayload is the Payload for [TopicRunRollingBack].
// In-flight handlers that receive this should check the run status and stop
// further work on behalf of this run.
type WorkflowRunRollingBackPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	Reason        string `json:"reason,omitempty"`
}

// WorkflowRunRollbackFailedPayload is the Payload for [TopicRunRollbackFailed].
type WorkflowRunRollbackFailedPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	FailedAt      string `json:"failed_at"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// WorkflowRunCancellingPayload is the Payload for [TopicRunCancelling]
// (FEAT-20260602-008). Subscribers should quiesce in-flight work for the
// run; the finalization step transitions the run to cancelled at or after
// QuiesceDeadline regardless of acknowledgement.
type WorkflowRunCancellingPayload struct {
	WorkflowRunID   string `json:"workflow_run_id"`
	Reason          string `json:"reason,omitempty"`
	CancelledBy     string `json:"cancelled_by,omitempty"`
	QuiesceDeadline string `json:"quiesce_deadline,omitempty"`
}

// WorkflowRunCancelledPayload is the Payload for [TopicRunCancelled]
// (FEAT-20260602-008). Marks the run as terminally cancelled.
type WorkflowRunCancelledPayload struct {
	WorkflowRunID string `json:"workflow_run_id"`
	CancelledAt   string `json:"cancelled_at"`
	Reason        string `json:"reason,omitempty"`
	CancelledBy   string `json:"cancelled_by,omitempty"`
}

// TaskCancelledPayload is the Payload for [TopicTaskCancelled]
// (FEAT-20260602-008). Emitted once per Task whose status was flipped to
// cancelled by the run-cancel cascade.
type TaskCancelledPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id"`
	Reason        string `json:"reason,omitempty"`
}

// WorkflowRunTimeoutPayload is the Payload for [TopicRunTimeout]
// (FEAT-20260602-006). Published by the Cross watchdog when a WorkflowRun
// exceeds its inactivity timeout.
type WorkflowRunTimeoutPayload struct {
	WorkflowRunID    string `json:"workflow_run_id"`
	AgencyID         string `json:"agency_id"`
	LastEventAt      string `json:"last_event_at,omitempty"`
	InactivityWindow string `json:"inactivity_window,omitempty"`
	DetectedAt       string `json:"detected_at"`
}

// WorkflowRunTaskTimeoutPayload is the Payload for [TopicTaskTimeout]
// (FEAT-20260602-006). Published by the Cross watchdog when a per-step
// stall is detected.
type WorkflowRunTaskTimeoutPayload struct {
	WorkflowRunID        string `json:"workflow_run_id"`
	AgencyID             string `json:"agency_id"`
	StepID               string `json:"step_id"`
	CurrentStepStartedAt string `json:"current_step_started_at,omitempty"`
	StepTimeout          string `json:"step_timeout,omitempty"`
	DetectedAt           string `json:"detected_at"`
}

// TaskClassifyFailurePayload is the Payload for [TopicTaskClassifyFailure].
// Emitted by CodeValdWork when a task exhausts its retry budget; CodeValdAI
// responds with [TaskFailureClassifiedPayload].
type TaskClassifyFailurePayload struct {
	TaskID               string `json:"task_id"`
	WorkflowRunID        string `json:"workflow_run_id,omitempty"`
	FailureCount         int    `json:"failure_count"`
	LastFailureReason    string `json:"last_failure_reason,omitempty"`
	TaskDescription      string `json:"task_description,omitempty"`
	FailedTodoTitle      string `json:"failed_todo_title,omitempty"`
	FailedTodoInstructions string `json:"failed_todo_instructions,omitempty"`
}

// TaskFailureClassifiedPayload is the Payload for [TopicTaskFailureClassified].
// Emitted by CodeValdAI in response to [TopicTaskClassifyFailure].
type TaskFailureClassifiedPayload struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id,omitempty"`
	// FailureType is "transient" (allow one more retry) or "requires-human" (escalate).
	FailureType string `json:"failure_type"`
	Reasoning   string `json:"reasoning,omitempty"`
}

// ConsumedTopics is the closed list of topics CodeValdWork subscribes to.
//
// Self-subscriptions (work.task.completed, work.task.assigned, work.task.failed)
// allow intra-service coordination without importing other service packages.
func ConsumedTopics() []string {
	return []string{
		TopicTaskUpdate,
		TopicTaskCompleted,
		TopicTaskAssigned, // self-subscription: pending→in_progress run transition
		TopicTaskFailed,   // self-subscription: in_progress→failed run transition
		// AI bridge topics (consumed by TaskEventDispatcher):
		"ai.task.started",
		"ai.task.completed",
		"ai.task.failed",
		"ai.todo.created",
		// External failure topics for run → failed transitions:
		"functions.job.failed",
		"ai.run.failed",
		"git.merge.failed",
		// External completion topics for terminal_event matching:
		"functions.job.completed",
		"git.merge.completed",
		"ai.run.completed",
		// Git file-written gate (BUG-09-020 Phase 2):
		"git.file.written",
		// Watchdog timeout topics (FEAT-20260602-006):
		TopicRunTimeout,
		TopicTaskTimeout,
		// Pipeline start: flip PENDING → IN_PROGRESS immediately (BUG-09-025):
		TopicPipelineStarted,
		// Failure recovery: direction response from AI reviewer or human form submission:
		TopicTaskDirection,
		// Failure classification response from CodeValdAI:
		TopicTaskFailureClassified,
	}
}
