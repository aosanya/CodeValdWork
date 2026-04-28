// Package codevaldwork — pre-delivered schema definition.
//
// This file exposes [DefaultWorkSchema], which returns the fixed [types.Schema]
// for CodeValdWork. The schema is seeded idempotently on startup via
// entitygraph.SeedSchema (see internal/app).
//
// The schema declares three TypeDefinitions:
//   - Task — task assigned to an Agent (mutable)
//   - TaskGroup — optional container that groups related tasks
//   - Agent — Work-domain projection of an AI agent (vertex for the
//     `assigned_to` graph edge added in MVP-WORK-010)
//
// All edges live in the "work_relationships" edge collection (declared by
// MVP-WORK-009). Tasks live in the "work_tasks" document collection,
// TaskGroups in "work_groups", and Agents in "work_agents".
package codevaldwork

import "github.com/aosanya/CodeValdSharedLib/types"

// DefaultWorkSchema returns the pre-delivered [types.Schema] seeded on startup.
// The operation is idempotent — calling it multiple times with the same schema
// ID is safe.
func DefaultWorkSchema() types.Schema {
	return types.Schema{
		ID:      "work-schema-v1",
		Version: 1,
		Tag:     "v1",
		Types: []types.TypeDefinition{
			taskTypeDefinition(),
			taskGroupTypeDefinition(),
			agentTypeDefinition(),
		},
	}
}

// taskTypeDefinition declares the Task entity class. Status transitions and the
// blocker gate are enforced by [TaskManager], not by the schema.
func taskTypeDefinition() types.TypeDefinition {
	return types.TypeDefinition{
		Name:              "Task",
		DisplayName:       "Task",
		PathSegment:       "tasks",
		EntityIDParam:     "taskId",
		StorageCollection: "work_tasks",
		Properties: []types.PropertyDefinition{
			// title is the short human-readable summary of the task. Required.
			{Name: "title", Type: types.PropertyTypeString, Required: true},
			// description provides additional context for the assigned agent.
			{Name: "description", Type: types.PropertyTypeString},
			// status is the current lifecycle state — see [TaskStatus].
			{Name: "status", Type: types.PropertyTypeOption, Options: taskStatusOptions()},
			// priority indicates relative urgency — see [TaskPriority].
			{Name: "priority", Type: types.PropertyTypeOption, Options: taskPriorityOptions()},
			// dueAt is the deadline by which the task should be completed.
			{Name: "dueAt", Type: types.PropertyTypeDatetime},
			// tags are free-form labels associated with the task.
			{Name: "tags", Type: types.PropertyTypeArray, ElementType: types.PropertyTypeString},
			// estimatedHours is the planned effort to complete the task, in hours.
			{Name: "estimatedHours", Type: types.PropertyTypeNumber},
			// context is the AI agent's working memory blob.
			{Name: "context", Type: types.PropertyTypeString},
			// completedAt is set by [TaskManager] when status reaches a terminal state.
			{Name: "completedAt", Type: types.PropertyTypeDatetime},
		},
	}
}

// taskGroupTypeDefinition declares the TaskGroup entity class — an optional
// container that groups related tasks via the `member_of` graph edge added in
// MVP-WORK-012.
func taskGroupTypeDefinition() types.TypeDefinition {
	return types.TypeDefinition{
		Name:              "TaskGroup",
		DisplayName:       "Task Group",
		PathSegment:       "task-groups",
		EntityIDParam:     "taskGroupId",
		StorageCollection: "work_groups",
		Properties: []types.PropertyDefinition{
			// name is the short human-readable label. Required.
			{Name: "name", Type: types.PropertyTypeString, Required: true},
			// description provides additional context for the group.
			{Name: "description", Type: types.PropertyTypeString},
			// dueAt is the target completion date for the group.
			{Name: "dueAt", Type: types.PropertyTypeDatetime},
		},
	}
}

// agentTypeDefinition declares the Agent entity class — the Work-domain
// projection of an AI agent. Each Agent becomes a graph vertex so that
// `assigned_to` edges (added in MVP-WORK-010) are first-class graph
// relationships rather than string fields on the Task document.
func agentTypeDefinition() types.TypeDefinition {
	return types.TypeDefinition{
		Name:              "Agent",
		DisplayName:       "Agent",
		PathSegment:       "agents",
		EntityIDParam:     "agentId",
		StorageCollection: "work_agents",
		Properties: []types.PropertyDefinition{
			// agentID is the external agent identifier (e.g. CodeValdAI agent ID).
			// Unique per (agencyID, agentID) — enforced at the UpsertAgent boundary.
			{Name: "agentID", Type: types.PropertyTypeString, Required: true},
			// displayName is a human-readable label for the agent.
			{Name: "displayName", Type: types.PropertyTypeString},
			// capability is the agent's primary capability (e.g. "code", "research", "review").
			{Name: "capability", Type: types.PropertyTypeString},
		},
	}
}

// taskStatusOptions returns the closed set of allowed string values for the
// Task "status" PropertyTypeOption — must stay in sync with the [TaskStatus]
// constants declared in types.go.
func taskStatusOptions() []string {
	return []string{
		string(TaskStatusPending),
		string(TaskStatusInProgress),
		string(TaskStatusCompleted),
		string(TaskStatusFailed),
		string(TaskStatusCancelled),
	}
}

// taskPriorityOptions returns the closed set of allowed string values for the
// Task "priority" PropertyTypeOption — must stay in sync with the
// [TaskPriority] constants declared in types.go.
func taskPriorityOptions() []string {
	return []string{
		string(TaskPriorityLow),
		string(TaskPriorityMedium),
		string(TaskPriorityHigh),
		string(TaskPriorityCritical),
	}
}
