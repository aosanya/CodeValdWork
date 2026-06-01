package codevaldwork

import (
	"context"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// AssignTask sets the Agent currently responsible for a Task by writing the
// `assigned_to` graph edge (Task → Agent). A Task has at most one assignee —
// any pre-existing outbound `assigned_to` edge from the Task is removed
// before the new one is created.
//
// If the task has any unmet outbound `depends_on` edges (i.e. depends on a
// source task that has not reached a terminal status), the assignment is
// still recorded but the task is transitioned to TaskStatusBlocked and the
// usual work.task.assigned dispatch is suppressed. The caller (operator or
// frontend) sees the assignment immediately; the AI does not see a dispatch
// it could not act on. When every blocking dependency eventually reaches a
// terminal status, a separate unblock path can flip blocked → pending and
// re-fire work.task.assigned.
func (m *taskManager) AssignTask(ctx context.Context, agencyID, taskID, agentID string) error {
	task, err := m.GetTask(ctx, agencyID, taskID)
	if err != nil {
		return err
	}
	agent, err := m.GetAgent(ctx, agencyID, agentID)
	if err != nil {
		return err
	}
	// GetAgent accepts either the entity UUID or the AgentID slug; downstream
	// edge writes + the work.task.assigned payload need the entity UUID so
	// subscribers can resolve the agent via GetEntity without slug knowledge.
	resolvedAgentID := agent.ID

	existing, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelAssignedTo, DirectionOutbound)
	if err != nil {
		return fmt.Errorf("AssignTask: traverse: %w", err)
	}
	for _, edge := range existing {
		if err := m.dm.DeleteRelationship(ctx, agencyID, edge.ID); err != nil {
			return fmt.Errorf("AssignTask: delete prior edge: %w", err)
		}
	}

	if _, err := m.dm.CreateRelationship(ctx, entitygraph.CreateRelationshipRequest{
		AgencyID: agencyID,
		Name:     RelLabelAssignedTo,
		FromID:   taskID,
		ToID:     resolvedAgentID,
		Properties: map[string]any{
			"assigned_at": time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		return fmt.Errorf("AssignTask: create edge: %w", err)
	}

	// Check outbound depends_on edges. Any non-terminal target blocks dispatch.
	unmet, err := m.findUnmetDependencies(ctx, agencyID, taskID)
	if err != nil {
		return fmt.Errorf("AssignTask: check deps: %w", err)
	}
	if len(unmet) > 0 {
		if task.Status.CanTransitionTo(TaskStatusBlocked) {
			if err := m.setTaskStatus(ctx, agencyID, taskID, TaskStatusBlocked); err != nil {
				return fmt.Errorf("AssignTask: set blocked: %w", err)
			}
			m.publish(ctx, TopicTaskStatusChanged, agencyID, TaskStatusChangedPayload{
				TaskID: taskID,
				From:   task.Status,
				To:     TaskStatusBlocked,
			})
		}
		// Suppress work.task.assigned — the AI must not run a task whose
		// dependencies have not landed. The assigned_to edge is preserved
		// so an operator viewing the task sees the chosen agent.
		return nil
	}

	m.publish(ctx, TopicTaskAssigned, agencyID, TaskAssignedPayload{
		TaskID:      taskID,
		AgentID:     resolvedAgentID,
		RoleName:    agent.RoleName,
		TaskCode:    task.TaskName,
		Title:       task.Title,
		Description: task.Description,
	})
	return nil
}

// findUnmetDependencies returns the IDs of tasks this task depends on
// (via outbound depends_on edges) that have not reached a terminal status.
// The Y task has an outbound depends_on edge pointing to X when the
// gittesting.json import declares Y.depends_on includes X — so an outbound
// traversal yields the dependency targets that must complete first.
func (m *taskManager) findUnmetDependencies(ctx context.Context, agencyID, taskID string) ([]string, error) {
	edges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelDependsOn, DirectionOutbound)
	if err != nil {
		return nil, fmt.Errorf("findUnmetDependencies: %w", err)
	}
	var unmet []string
	for _, e := range edges {
		dep, err := m.GetTask(ctx, agencyID, e.ToID)
		if err != nil {
			// A missing dep target is treated as unmet (defensive — the
			// import wrote the edge, but the entity was later deleted).
			unmet = append(unmet, e.ToID)
			continue
		}
		if !isTerminalStatus(dep.Status) {
			unmet = append(unmet, dep.ID)
		}
	}
	return unmet, nil
}

// setTaskStatus updates only the status property on the Task entity, leaving
// every other property untouched. Reads the current task, mutates Status,
// and writes back via UpdateEntity — heavier than a single-field write but
// matches the surface area the rest of taskManager uses, and avoids the
// "zero-other-fields" replace-all foot-gun the proto3 UpdateTask path has.
func (m *taskManager) setTaskStatus(ctx context.Context, agencyID, taskID string, status TaskStatus) error {
	entity, err := m.dm.GetEntity(ctx, agencyID, taskID)
	if err != nil {
		return fmt.Errorf("setTaskStatus: get entity: %w", err)
	}
	if entity.Properties == nil {
		entity.Properties = map[string]any{}
	}
	entity.Properties["status"] = string(status)
	entity.Properties["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	if _, err := m.dm.UpdateEntity(ctx, agencyID, taskID, entitygraph.UpdateEntityRequest{
		Properties: entity.Properties,
	}); err != nil {
		return fmt.Errorf("setTaskStatus: update entity: %w", err)
	}
	return nil
}

// UnassignTask removes any outbound `assigned_to` edge from the Task.
// Idempotent — returns nil whether or not an edge was present.
func (m *taskManager) UnassignTask(ctx context.Context, agencyID, taskID string) error {
	if _, err := m.GetTask(ctx, agencyID, taskID); err != nil {
		return err
	}
	existing, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelAssignedTo, DirectionOutbound)
	if err != nil {
		return fmt.Errorf("UnassignTask: traverse: %w", err)
	}
	for _, edge := range existing {
		if err := m.dm.DeleteRelationship(ctx, agencyID, edge.ID); err != nil {
			return fmt.Errorf("UnassignTask: delete edge: %w", err)
		}
	}
	return nil
}
