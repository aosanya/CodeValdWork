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

// ConsumedTopics is the closed list of topics CodeValdWork subscribes to.
//
// TopicTaskCompleted is a self-subscription: CodeValdWork publishes
// work.task.completed and also consumes it to drive the auto-unblock cascade
// in [server.TaskEventDispatcher]. The handler is idempotent so re-delivery
// (and self-receipts) are safely no-op.
func ConsumedTopics() []string {
	return []string{TopicTaskUpdate, TopicTaskCompleted}
}
