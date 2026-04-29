package codevaldwork

import "time"

// Event topic constants — the closed set CodeValdWork publishes per
// `architecture-domain.md §6`. Keep in sync with the `produces` list in
// `internal/registrar/registrar.go`.
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
	// (completed, failed, cancelled). It is published in addition to
	// [TopicTaskStatusChanged], not instead of — subscribers may listen
	// to either depending on their interest. Payload: [TaskCompletedPayload].
	TopicTaskCompleted = "work.task.completed"

	// TopicTaskAssigned fires when an `assigned_to` edge is created or
	// replaced. Payload: [TaskAssignedPayload].
	TopicTaskAssigned = "work.task.assigned"

	// TopicRelationshipCreated fires when any whitelisted graph edge is
	// created. Payload: [RelationshipCreatedPayload].
	TopicRelationshipCreated = "work.relationship.created"
)

// AllTopics is the closed list of topics this service publishes. Used by
// the registrar's `produces` declaration and by tests asserting every
// topic is covered.
func AllTopics() []string {
	return []string{
		TopicTaskCreated,
		TopicTaskUpdated,
		TopicTaskStatusChanged,
		TopicTaskCompleted,
		TopicTaskAssigned,
		TopicRelationshipCreated,
	}
}

// TaskCreatedPayload is the [Event.Payload] for [TopicTaskCreated].
type TaskCreatedPayload struct {
	TaskID   string
	Priority TaskPriority
}

// TaskUpdatedPayload is the [Event.Payload] for [TopicTaskUpdated].
//
// ChangedFields lists the property names that were observed to differ
// between the prior and updated state. Status changes are reported
// separately via [TopicTaskStatusChanged] and are NOT included here even
// when status is part of the same UpdateTask call.
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
	CompletedAt    time.Time
}

// TaskAssignedPayload is the [Event.Payload] for [TopicTaskAssigned].
type TaskAssignedPayload struct {
	TaskID  string
	AgentID string
}

// RelationshipCreatedPayload is the [Event.Payload] for [TopicRelationshipCreated].
type RelationshipCreatedPayload struct {
	FromID string
	ToID   string
	Label  string
}
