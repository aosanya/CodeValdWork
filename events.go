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
	)
}

// TaskCreatedPayload is the [Event.Payload] for [TopicTaskCreated].
type TaskCreatedPayload struct {
	TaskID   string
	Priority TaskPriority
}

// TaskUpdatedPayload is the [Event.Payload] for [TopicTaskUpdated].
type TaskUpdatedPayload struct {
	TaskID        string
	ChangedFields []string
}

// TaskStatusChangedPayload is the [Event.Payload] for [TopicTaskStatusChanged].
type TaskStatusChangedPayload struct {
	TaskID string
	From   TaskStatus
	To     TaskStatus
}

// TaskCompletedPayload is the [Event.Payload] for [TopicTaskCompleted].
type TaskCompletedPayload struct {
	TaskID         string
	TerminalStatus TaskStatus
	// CompletedAt is the RFC 3339 timestamp when the terminal status was set.
	CompletedAt string
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
}

// TaskAssignedPayload is the [Event.Payload] for [TopicTaskAssigned].
type TaskAssignedPayload struct {
	TaskID      string
	AgentID     string
	RoleName    string
	TaskCode    string // project-scoped code, e.g. "UTIL-001" — empty for tasks not in a project
	Title       string
	Description string
}

// RelationshipCreatedPayload is the [Event.Payload] for [TopicRelationshipCreated].
type RelationshipCreatedPayload struct {
	FromID string
	ToID   string
	Label  string
}

// TodoDispatchedPayload is the [Event.Payload] for [TopicTodoDispatched].
// Published once per TaskTodo entity created from an ai.todo.created decomposition.
type TodoDispatchedPayload struct {
	TodoID         string
	TaskID         string // alias for ParentTaskID — used by HydrateEventContext
	ParentTaskID   string
	DecompRunID    string
	AgentID        string
	Title          string
	Instructions   string
	Ordinality     int
	CanRunParallel bool
	DependsOn      []int
	Precalls       string // JSON-encoded []PrecallSpec stored on the TaskTodo
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
func ConsumedTopics() []string {
	return []string{TopicTaskUpdate}
}
