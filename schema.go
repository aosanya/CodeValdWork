// Package codevaldwork — pre-delivered schema definition.
//
// This file exposes [DefaultWorkSchema], which returns the fixed [types.Schema]
// for CodeValdWork. The schema is seeded idempotently on startup via
// entitygraph.SeedSchema (see internal/app).
//
// The schema declares three TypeDefinitions:
//   - Task — task assigned to an Agent (mutable)
//   - Project — optional container that groups related tasks
//   - Agent — Work-domain projection of an AI agent (vertex for the
//     `assigned_to` graph edge added in MVP-WORK-010)
//
// All edges live in the "work_relationships" edge collection (declared by
// MVP-WORK-009). Tasks live in the "work_tasks" document collection,
// Projects in "work_projects", and Agents in "work_agents".
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
			projectTypeDefinition(),
			agentTypeDefinition(),
		},
	}
}

// taskTypeDefinition declares the Task entity class. Status transitions and the
// blocker gate are enforced by [TaskManager], not by the schema.
//
// The Relationships block declares the Work edge-label whitelist. Only the
// (label, fromType, toType) triples listed here may be created on a Task —
// anything else is rejected with [ErrInvalidRelationship] by the underlying
// entitygraph.DataManager.CreateRelationship.
func taskTypeDefinition() types.TypeDefinition {
	return types.TypeDefinition{
		Name:              "Task",
		DisplayName:       "Task",
		PathSegment:       "tasks",
		EntityIDParam:     "taskId",
		StorageCollection: "work_tasks",
		Properties: []types.PropertyDefinition{
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
			// taskName is the project-scoped auto-generated name (e.g. "MVP-001").
			{Name: "taskName", Type: types.PropertyTypeString},
			// projectName is the URL-safe slug of the project this task belongs to.
			{Name: "projectName", Type: types.PropertyTypeString},
		},
		Relationships: []types.RelationshipDefinition{
			{
				Name:        RelLabelAssignedTo,
				Label:       "Assigned to",
				ToType:      "Agent",
				ToMany:      false,
				PathSegment: "agent",
				Properties: []types.PropertyDefinition{
					{Name: "assignedAt", Type: types.PropertyTypeDatetime},
					{Name: "assignedBy", Type: types.PropertyTypeString},
				},
			},
			{
				Name:        RelLabelBlocks,
				Label:       "Blocks",
				ToType:      "Task",
				ToMany:      true,
				PathSegment: "blocks",
				Properties: []types.PropertyDefinition{
					{Name: "createdAt", Type: types.PropertyTypeDatetime},
					{Name: "reason", Type: types.PropertyTypeString},
				},
			},
			{
				Name:        RelLabelSubtaskOf,
				Label:       "Subtask of",
				ToType:      "Task",
				ToMany:      false,
				PathSegment: "parent",
				Properties: []types.PropertyDefinition{
					{Name: "createdAt", Type: types.PropertyTypeDatetime},
				},
			},
			{
				Name:        RelLabelDependsOn,
				Label:       "Depends on",
				ToType:      "Task",
				ToMany:      true,
				PathSegment: "depends-on",
				Properties: []types.PropertyDefinition{
					{Name: "createdAt", Type: types.PropertyTypeDatetime},
					{Name: "reason", Type: types.PropertyTypeString},
				},
			},
			{
				Name:        RelLabelMemberOf,
				Label:       "Member of",
				ToType:      "Project",
				ToMany:      true,
				PathSegment: "projects",
				Properties: []types.PropertyDefinition{
					{Name: "addedAt", Type: types.PropertyTypeDatetime},
				},
			},
		},
	}
}

// projectTypeDefinition declares the Project entity class — an optional
// container that groups related tasks via the `member_of` graph edge added in
// MVP-WORK-012.
func projectTypeDefinition() types.TypeDefinition {
	return types.TypeDefinition{
		Name:              "Project",
		DisplayName:       "Project",
		PathSegment:       "projects",
		EntityIDParam:     "projectId",
		StorageCollection: "work_projects",
		Properties: []types.PropertyDefinition{
			// name is the short human-readable label. Required.
			{Name: "name", Type: types.PropertyTypeString, Required: true},
			// projectName is the URL-safe slug (lowercase, spaces→underscores).
			{Name: "projectName", Type: types.PropertyTypeString},
			// description provides additional context for the project.
			{Name: "description", Type: types.PropertyTypeString},
			// githubRepo is the canonical GitHub repository for the project,
			// e.g. "owner/name" or a full https URL.
			{Name: "githubRepo", Type: types.PropertyTypeString},
			// taskPrefix is prepended to the counter when auto-generating task names.
			{Name: "taskPrefix", Type: types.PropertyTypeString},
		},
	}
}

// agentTypeDefinition declares the Agent entity class — the Work-domain
// projection of an AI agent. Each Agent becomes a graph vertex so that
// `assigned_to` edges are first-class graph relationships rather than string
// fields on the Task document.
//
// UniqueKey: ["agentID"] makes the external agent identifier the natural key
// for [DataManager.UpsertEntity] — [TaskManager.UpsertAgent] (added in
// MVP-WORK-010) relies on this to find-or-create.
func agentTypeDefinition() types.TypeDefinition {
	return types.TypeDefinition{
		Name:              "Agent",
		DisplayName:       "Agent",
		PathSegment:       "agents",
		EntityIDParam:     "agentId",
		StorageCollection: "work_agents",
		UniqueKey:         []string{"agentID"},
		Properties: []types.PropertyDefinition{
			// agentID is the external agent identifier (e.g. CodeValdAI agent ID).
			// Unique per (agencyID, agentID) — enforced via UpsertEntity.
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
