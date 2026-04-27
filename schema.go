// Package codevaldwork — pre-delivered schema definition.
//
// This file exposes [DefaultWorkSchema], which returns the fixed [types.Schema]
// for CodeValdWork. The schema is seeded idempotently on startup via
// entitygraph.SeedSchema (see internal/app).
//
// The schema declares one TypeDefinition:
//   - Task — task assigned to an AI agent (mutable)
//
// All edges live in the "work_relationships" edge collection.
// Tasks live in the "work_tasks" document collection.
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
					// title is the short human-readable summary of the task. Required.
					{Name: "title", Type: types.PropertyTypeString, Required: true},
					// description provides additional context for the assigned agent.
					{Name: "description", Type: types.PropertyTypeString},
					// status is the current lifecycle state — see TaskStatus.
					// Valid values: pending | in_progress | completed | failed | cancelled.
					{Name: "status", Type: types.PropertyTypeString, Required: true},
					// priority indicates relative urgency — see TaskPriority.
					// Valid values: low | medium | high | critical.
					{Name: "priority", Type: types.PropertyTypeString},
					// assigned_to is the agent ID currently responsible for this task.
					// Empty when unassigned.
					{Name: "assigned_to", Type: types.PropertyTypeString},
					// created_at is the ISO 8601 timestamp at which the task was created.
					{Name: "created_at", Type: types.PropertyTypeString},
					// updated_at is the ISO 8601 timestamp of the most recent mutation.
					{Name: "updated_at", Type: types.PropertyTypeString},
					// completed_at is the ISO 8601 timestamp at which the task reached a
					// terminal status (completed, failed, cancelled). Empty until then.
					{Name: "completed_at", Type: types.PropertyTypeString},
				},
			},
		},
	}
}
