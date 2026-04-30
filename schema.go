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
					// Well-known values: "pending", "in_progress", "completed", "failed", "cancelled".
					{Name: "status", Type: types.PropertyTypeString},
					// priority indicates relative urgency — see [TaskPriority].
					// Well-known values: "low", "medium", "high", "critical".
					{Name: "priority", Type: types.PropertyTypeString},
					// due_at is the RFC 3339 deadline; empty when no deadline is set.
					{Name: "due_at", Type: types.PropertyTypeString},
					// tags are free-form labels associated with the task.
					{Name: "tags", Type: types.PropertyTypeArray, ElementType: types.PropertyTypeString},
					// estimated_hours is the planned effort to complete the task, in hours.
					{Name: "estimated_hours", Type: types.PropertyTypeNumber},
					// context is the AI agent's working memory blob.
					{Name: "context", Type: types.PropertyTypeString},
					// completed_at is set when status reaches a terminal state (RFC 3339).
					{Name: "completed_at", Type: types.PropertyTypeString},
					// task_name is the project-scoped auto-generated name (e.g. "MVP-001").
					{Name: "task_name", Type: types.PropertyTypeString},
					// project_name is the URL-safe slug of the project this task belongs to.
					{Name: "project_name", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        RelLabelAssignedTo,
						Label:       "Assigned to",
						PathSegment: "agent",
						ToType:      "Agent",
						ToMany:      false,
						Inverse:     "assigned_tasks",
						Properties: []types.PropertyDefinition{
							{Name: "assigned_at", Type: types.PropertyTypeString},
							{Name: "assigned_by", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelBlocks,
						Label:       "Blocks",
						PathSegment: "blocks",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     "blocked_by",
						Properties: []types.PropertyDefinition{
							{Name: "created_at", Type: types.PropertyTypeString},
							{Name: "reason", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelSubtaskOf,
						Label:       "Subtask of",
						PathSegment: "parent",
						ToType:      "Task",
						ToMany:      false,
						Inverse:     "has_subtask",
						Properties: []types.PropertyDefinition{
							{Name: "created_at", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelDependsOn,
						Label:       "Depends on",
						PathSegment: "depends-on",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     "depended_on_by",
						Properties: []types.PropertyDefinition{
							{Name: "created_at", Type: types.PropertyTypeString},
							{Name: "reason", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelMemberOf,
						Label:       "Member of",
						PathSegment: "projects",
						ToType:      "Project",
						ToMany:      true,
						Inverse:     "has_task",
						Properties: []types.PropertyDefinition{
							{Name: "added_at", Type: types.PropertyTypeString},
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
					// project_name is the URL-safe slug (lowercase, spaces→underscores).
					{Name: "project_name", Type: types.PropertyTypeString},
					// description provides additional context for the project.
					{Name: "description", Type: types.PropertyTypeString},
					// github_repo is the canonical GitHub repository, e.g. "owner/name".
					{Name: "github_repo", Type: types.PropertyTypeString},
					// task_prefix is prepended to the counter when auto-generating task names.
					{Name: "task_prefix", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        "has_task",
						Label:       "Tasks",
						PathSegment: "tasks",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     RelLabelMemberOf,
					},
				},
			},
			{
				Name:              "Agent",
				DisplayName:       "Agent",
				PathSegment:       "agents",
				EntityIDParam:     "agentId",
				StorageCollection: "work_agents",
				// UniqueKey on agent_id makes the external identifier the natural key for
				// UpsertEntity — UpsertAgent relies on this for find-or-create.
				UniqueKey: []string{"agent_id"},
				Properties: []types.PropertyDefinition{
					// agent_id is the external agent identifier. Unique per agency.
					{Name: "agent_id", Type: types.PropertyTypeString, Required: true},
					// display_name is a human-readable label for the agent.
					{Name: "display_name", Type: types.PropertyTypeString},
					// capability is the agent's primary capability (e.g. "code", "research").
					{Name: "capability", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        "assigned_tasks",
						Label:       "Assigned Tasks",
						PathSegment: "tasks",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     RelLabelAssignedTo,
					},
				},
			},
		},
	}
}
