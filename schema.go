// Package codevaldwork — pre-delivered schema definition.
//
// This file exposes [DefaultWorkSchema], which returns the fixed [types.Schema]
// for CodeValdWork. cmd/server seeds this schema idempotently on startup via
// entitygraph.SeedSchema (see internal/app).
//
// The schema declares three TypeDefinitions:
//   - Task    — a unit of work assigned to an AI Agent (mutable)
//   - Project — optional container that groups related Tasks via `member_of` edges
//   - Agent   — Work-domain projection of an AI agent; vertex for `assigned_to` edges
//
// Graph topology:
//
//	Task ──assigned_to──► Agent
//	Task ──member_of────► Project
//	Task ──blocks───────► Task
//	Task ──subtask_of───► Task
//	Task ──depends_on───► Task
//
// Storage:
//   - Task    → "work_tasks"          document collection
//   - Project → "work_projects"       document collection
//   - Agent   → "work_agents"         document collection
//   - All edges → "work_relationships" edge collection
package codevaldwork

import "github.com/aosanya/CodeValdSharedLib/types"

// DefaultWorkSchema returns the pre-delivered [types.Schema] seeded on startup.
// The operation is idempotent — calling it multiple times with the same schema
// ID is safe.
func DefaultWorkSchema() types.Schema {
	taskStatusOptions := []string{
		string(TaskStatusPending),
		string(TaskStatusInProgress),
		string(TaskStatusCompleted),
		string(TaskStatusFailed),
		string(TaskStatusCancelled),
	}
	taskPriorityOptions := []string{
		string(TaskPriorityLow),
		string(TaskPriorityMedium),
		string(TaskPriorityHigh),
		string(TaskPriorityCritical),
	}

	return types.Schema{
		ID:      "work-schema-v1",
		Version: 1,
		Tag:     "v1",
		Types: []types.TypeDefinition{
			{
				Name:              "Task",
				DisplayName:       "Task",
				PathSegment:       "tasks",
				EntityIDParam:     "taskId",
				StorageCollection: "work_tasks",
				Properties: []types.PropertyDefinition{
					// description provides additional context for the assigned agent.
					{Name: "description", Type: types.PropertyTypeString},
					// status is the current lifecycle state — see [TaskStatus].
					{Name: "status", Type: types.PropertyTypeOption, Options: taskStatusOptions},
					// priority indicates relative urgency — see [TaskPriority].
					{Name: "priority", Type: types.PropertyTypeOption, Options: taskPriorityOptions},
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
						PathSegment: "agent",
						ToType:      "Agent",
						ToMany:      false,
						Properties: []types.PropertyDefinition{
							{Name: "assignedAt", Type: types.PropertyTypeDatetime},
							{Name: "assignedBy", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelBlocks,
						Label:       "Blocks",
						PathSegment: "blocks",
						ToType:      "Task",
						ToMany:      true,
						Properties: []types.PropertyDefinition{
							{Name: "createdAt", Type: types.PropertyTypeDatetime},
							{Name: "reason", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelSubtaskOf,
						Label:       "Subtask of",
						PathSegment: "parent",
						ToType:      "Task",
						ToMany:      false,
						Properties: []types.PropertyDefinition{
							{Name: "createdAt", Type: types.PropertyTypeDatetime},
						},
					},
					{
						Name:        RelLabelDependsOn,
						Label:       "Depends on",
						PathSegment: "depends-on",
						ToType:      "Task",
						ToMany:      true,
						Properties: []types.PropertyDefinition{
							{Name: "createdAt", Type: types.PropertyTypeDatetime},
							{Name: "reason", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelMemberOf,
						Label:       "Member of",
						PathSegment: "projects",
						ToType:      "Project",
						ToMany:      true,
						Properties: []types.PropertyDefinition{
							{Name: "addedAt", Type: types.PropertyTypeDatetime},
						},
					},
				},
			},
			{
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
			},
			{
				Name:              "Agent",
				DisplayName:       "Agent",
				PathSegment:       "agents",
				EntityIDParam:     "agentId",
				StorageCollection: "work_agents",
				// UniqueKey on agentID makes the external identifier the natural key for
				// UpsertEntity — TaskManager.UpsertAgent relies on this for find-or-create.
				UniqueKey: []string{"agentID"},
				Properties: []types.PropertyDefinition{
					// agentID is the external agent identifier (e.g. CodeValdAI agent ID).
					// Unique per (agencyID, agentID) — enforced via UpsertEntity.
					{Name: "agentID", Type: types.PropertyTypeString, Required: true},
					// displayName is a human-readable label for the agent.
					{Name: "displayName", Type: types.PropertyTypeString},
					// capability is the agent's primary capability (e.g. "code", "research", "review").
					{Name: "capability", Type: types.PropertyTypeString},
				},
			},
		},
	}
}
