// Package codevaldwork — pre-delivered schema definition.
//
// This file exposes [DefaultWorkSchema], which returns the fixed [types.Schema]
// for CodeValdWork. cmd/server seeds this schema idempotently on startup via
// entitygraph.SeedSchema (see internal/app).
//
// The schema declares six TypeDefinitions:
//   - Task              — a unit of work assigned to an AI Agent (mutable)
//   - TaskTodo          — a decomposed sub-task produced by an AI decomposition run; carries todo_type and max_runs for per-type run-count enforcement (mutable)
//   - Project           — optional container that groups related Tasks via `member_of` edges
//   - Agent             — Work-domain projection of an AI agent; vertex for `assigned_to` edges
//   - Tag               — free-form label attached to Tasks via `has_tag` edges
//   - ImportProjectJob  — tracks async project-import operations
//
// Graph topology:
//
//	Task ──assigned_to──► Agent
//	Task ──member_of────► Project
//	Task ──blocks───────► Task
//	Task ──subtask_of───► Task
//	Task ──depends_on───► Task
//	Task ──has_tag──────► Tag
//	Task ──has_todo─────► TaskTodo
//
// Storage:
//   - Task             → "work_tasks"          document collection
//   - TaskTodo         → "work_task_todos"     document collection
//   - Project          → "work_projects"       document collection
//   - Agent            → "work_agents"         document collection
//   - Tag              → "work_tags"           document collection
//   - ImportProjectJob → "work_import_jobs"    document collection
//   - All edges        → "work_relationships"  edge collection
package codevaldwork

import (
	"github.com/aosanya/CodeValdSharedLib/eventreceiver"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// DefaultWorkSchema returns the pre-delivered [types.Schema] seeded on startup.
// The operation is idempotent — calling it multiple times with the same schema
// ID is safe.
func DefaultWorkSchema() types.Schema {
	return types.Schema{
		ID:      "work-schema-v1",
		Version: 2,
		Tag:     "v2",
		Types: append([]types.TypeDefinition{
			{
				Name:              "Task",
				DisplayName:       "Task",
				PathSegment:       "tasks",
				EntityIDParam:     "taskId",
				StorageCollection: "work_tasks",
				PublishEvents:     true,
				Properties: []types.PropertyDefinition{
					// title is the short human-readable label (e.g. "Farm Dashboard").
					{Name: "title", Type: types.PropertyTypeString},
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
					// separate_branch indicates whether this task should be worked on in its own git branch.
					{Name: "separate_branch", Type: types.PropertyTypeBoolean},
					// branch_name is the git branch to create/use for this task (e.g. "feature/SF-001_scaffolding").
					{Name: "branch_name", Type: types.PropertyTypeString},
					// workflow_run_id denormalises the WorkflowRun anchor onto the
					// Task row so queries can filter by run-id without traversing
					// the started_task edge. Empty for tasks not produced under a
					// run (FEAT-20260602-002).
					{Name: "workflow_run_id", Type: types.PropertyTypeString},
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
					{
						Name:        RelLabelHasTag,
						Label:       "Has tag",
						PathSegment: "tags",
						ToType:      "Tag",
						ToMany:      true,
						Inverse:     "tagged_tasks",
						Properties: []types.PropertyDefinition{
							{Name: "tagged_at", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        "blocked_by",
						Label:       "Blocked by",
						PathSegment: "blockers",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     RelLabelBlocks,
					},
					{
						Name:        "has_subtask",
						Label:       "Subtasks",
						PathSegment: "subtasks",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     RelLabelSubtaskOf,
					},
					{
						Name:        "depended_on_by",
						Label:       "Depended on by",
						PathSegment: "dependents",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     RelLabelDependsOn,
					},
					{
						Name:        RelLabelHasTodo,
						Label:       "Todos",
						PathSegment: "todos",
						ToType:      "TaskTodo",
						ToMany:      true,
						Inverse:     "todo_of",
					},
					{
						Name:        RelLabelPartOfRun,
						Label:       "Part of run",
						PathSegment: "workflow-run",
						ToType:      "WorkflowRun",
						ToMany:      false,
						Inverse:     RelLabelStartedTask,
					},
				},
			},
			{
				Name:              "TaskTodo",
				DisplayName:       "Task Todo",
				PathSegment:       "todos",
				EntityIDParam:     "todoId",
				StorageCollection: "work_task_todos",
				PublishEvents:     true,
				Properties: []types.PropertyDefinition{
					// title is the short label for this sub-task.
					{Name: "title", Type: types.PropertyTypeString, Required: true},
					// description explains what this sub-task accomplishes.
					{Name: "description", Type: types.PropertyTypeString},
					// instructions is the fully self-contained agent prompt for executing this todo.
					{Name: "instructions", Type: types.PropertyTypeString, Required: true},
					// ordinality is the 1-based position of this todo within the decomposition.
					{Name: "ordinality", Type: types.PropertyTypeInteger, Required: true},
					// can_run_parallel is true when this todo has no predecessor dependency.
					{Name: "can_run_parallel", Type: types.PropertyTypeBoolean},
					// depends_on is a JSON-encoded []int of ordinality values that must complete first.
					{Name: "depends_on", Type: types.PropertyTypeArray, ElementType: types.PropertyTypeInteger},
					// status tracks the todo lifecycle: pending → dispatched → completed | failed.
					{Name: "status", Type: types.PropertyTypeString},
					// parent_task_id is the Work Task ID from which this todo was decomposed.
					{Name: "parent_task_id", Type: types.PropertyTypeString, Required: true},
					// decomp_run_id is the CodeValdAI AgentRun ID that produced this todo.
					{Name: "decomp_run_id", Type: types.PropertyTypeString},
					// agent_id is the CodeValdAI agent assigned to execute this todo.
					{Name: "agent_id", Type: types.PropertyTypeString},
					// precalls is a JSON-encoded []PrecallSpec: pre-execution fetch specs whose
					// results are injected into the LLM context by HydrateEventContext before
					// the agent runs. Each spec targets a specific service (e.g. "git") and
					// operation (e.g. "blob_search") with typed parameters.
					{Name: "precalls", Type: types.PropertyTypeString},
					// todo_type is the semantic type of this todo (e.g. "compile-fix").
					// CodeValdWork tracks run_count per (parent_task_id, todo_type) and enforces
					// max_runs at creation time — rejecting the injection when the limit is reached.
					{Name: "todo_type", Type: types.PropertyTypeString},
					// max_runs is the maximum number of todos of this todo_type that may be created
					// for the parent task. Enforced by CodeValdWork at creation time.
					// Zero means no limit.
					{Name: "max_runs", Type: types.PropertyTypeInteger},
					// workflow_run_id denormalises the WorkflowRun anchor onto the
					// TaskTodo row. Inherited from the parent Task at creation
					// time so the todo carries the run-id its parent belongs to
					// (FEAT-20260602-002).
					{Name: "workflow_run_id", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        "todo_of",
						Label:       "Parent Task",
						PathSegment: "task",
						ToType:      "Task",
						ToMany:      false,
						Required:    true,
						Inverse:     RelLabelHasTodo,
					},
					{
						Name:        RelLabelTodoAssignedTo,
						Label:       "Assigned to",
						PathSegment: "agent",
						ToType:      "Agent",
						ToMany:      false,
						Inverse:     "todo_assigned_tasks",
					},
					{
						Name:        RelLabelPartOfRun,
						Label:       "Part of run",
						PathSegment: "workflow-run",
						ToType:      "WorkflowRun",
						ToMany:      false,
						Inverse:     RelLabelStartedTodo,
					},
				},
			},
			{
				Name:              "Project",
				DisplayName:       "Project",
				PathSegment:       "projects",
				EntityIDParam:     "projectId",
				StorageCollection: "work_projects",
				PublishEvents:     true,
				Properties: []types.PropertyDefinition{
					// name is the short human-readable label. Required.
					{Name: "name", Type: types.PropertyTypeString, Required: true},
					// project_name is the URL-safe slug (lowercase, spaces→underscores).
					{Name: "project_name", Type: types.PropertyTypeString},
					// description provides additional context for the project.
					{Name: "description", Type: types.PropertyTypeString},
					// repo_name is the CodeValdGit repository name associated with this project.
					// Used by HydrateEventContext to scope file hydration to the correct repo.
					{Name: "repo_name", Type: types.PropertyTypeString},
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
				PublishEvents:     true,
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
					// role_name is the role this agent fulfils (e.g. "domain-expert").
					{Name: "role_name", Type: types.PropertyTypeString},
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
					{
						Name:        "todo_assigned_tasks",
						Label:       "Assigned Todo Tasks",
						PathSegment: "todos",
						ToType:      "TaskTodo",
						ToMany:      true,
						Inverse:     RelLabelTodoAssignedTo,
					},
				},
			},
			{
				Name:              "Tag",
				DisplayName:       "Tag",
				PathSegment:       "tags",
				EntityIDParam:     "tagId",
				StorageCollection: "work_tags",
				PublishEvents:     true,
				UniqueKey:         []string{"name"},
				Properties: []types.PropertyDefinition{
					// name is the unique label text (e.g. "setup", "auth").
					{Name: "name", Type: types.PropertyTypeString, Required: true},
					// color is an optional hex/CSS color for UI rendering.
					{Name: "color", Type: types.PropertyTypeString},
					// description provides additional context for the tag.
					{Name: "description", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        "tagged_tasks",
						Label:       "Tagged Tasks",
						PathSegment: "tasks",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     RelLabelHasTag,
					},
				},
			},
			{
				Name:        "WorkflowRun",
				DisplayName: "Workflow Run",
				// PathSegment / EntityIDParam intentionally omitted — the
				// schema-derived generic CRUD routes would collide with the
				// explicit closure endpoint in
				// internal/registrar/registrar.go::workflowRunRoutes.
				StorageCollection: "work_workflow_runs",
				PublishEvents:     true,
				// UniqueKey on name guarantees one (agency, name) pair maps to
				// at most one run vertex, so callers can correlate by a
				// caller-supplied or server-generated label.
				UniqueKey: []string{"name"},
				Properties: []types.PropertyDefinition{
					// name is a caller-supplied or server-generated label
					// unique per agency. Used as the correlation handle by
					// test scripts and the headline column in the UI list.
					{Name: "name", Type: types.PropertyTypeString},
					// status is the run lifecycle state: pending, in_progress,
					// completed, failed, rolled_back.
					{Name: "status", Type: types.PropertyTypeString},
					// trigger_event names the event that started the run
					// (e.g. "work.next.requested").
					{Name: "trigger_event", Type: types.PropertyTypeString},
					// initiator is an opaque caller identifier (operator email,
					// service name, etc.). May be empty.
					{Name: "initiator", Type: types.PropertyTypeString},
					// notes is free-form human-readable context.
					{Name: "notes", Type: types.PropertyTypeString},
					// agent_run_ids, function_job_ids, branch_names are stored
					// as JSON-encoded string arrays — the cross-service references
					// the closure endpoint surfaces.
					{Name: "agent_run_ids", Type: types.PropertyTypeArray, ElementType: types.PropertyTypeString},
					{Name: "function_job_ids", Type: types.PropertyTypeArray, ElementType: types.PropertyTypeString},
					{Name: "branch_names", Type: types.PropertyTypeArray, ElementType: types.PropertyTypeString},
					{Name: "started_at", Type: types.PropertyTypeString},
					{Name: "completed_at", Type: types.PropertyTypeString},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
				Relationships: []types.RelationshipDefinition{
					{
						Name:        RelLabelStartedTask,
						Label:       "Started task",
						PathSegment: "tasks",
						ToType:      "Task",
						ToMany:      true,
						Inverse:     RelLabelPartOfRun,
						Properties: []types.PropertyDefinition{
							{Name: "created_at", Type: types.PropertyTypeString},
						},
					},
					{
						Name:        RelLabelStartedTodo,
						Label:       "Started todo",
						PathSegment: "todos",
						ToType:      "TaskTodo",
						ToMany:      true,
						Inverse:     RelLabelPartOfRun,
						Properties: []types.PropertyDefinition{
							{Name: "created_at", Type: types.PropertyTypeString},
						},
					},
				},
			},
			{
				Name:              "ImportProjectJob",
				DisplayName:       "Import Project Job",
				StorageCollection: "work_import_jobs",
				PublishEvents:     true,
				Properties: []types.PropertyDefinition{
					// status tracks the async lifecycle: pending, running, completed, failed, cancelled.
					{Name: "status", Type: types.PropertyTypeString},
					// error_message is populated when status is "failed".
					{Name: "error_message", Type: types.PropertyTypeString},
					// tasks_created is the number of Task vertices written on completion.
					{Name: "tasks_created", Type: types.PropertyTypeNumber},
					// deps_created is the number of depends_on edges written on completion.
					{Name: "deps_created", Type: types.PropertyTypeNumber},
					{Name: "created_at", Type: types.PropertyTypeString},
					{Name: "updated_at", Type: types.PropertyTypeString},
				},
			},
		}, eventreceiver.ReceivedEventTypeDefinition("work")),
	}
}
