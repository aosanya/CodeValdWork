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
func (m *taskManager) AssignTask(ctx context.Context, agencyID, taskID, agentID string) error {
	if _, err := m.GetTask(ctx, agencyID, taskID); err != nil {
		return err
	}
	agent, err := m.GetAgent(ctx, agencyID, agentID)
	if err != nil {
		return err
	}

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
		ToID:     agentID,
		Properties: map[string]any{
			"assigned_at": time.Now().UTC().Format(time.RFC3339),
		},
	}); err != nil {
		return fmt.Errorf("AssignTask: create edge: %w", err)
	}

	m.publish(ctx, TopicTaskAssigned, agencyID, TaskAssignedPayload{
		TaskID:   taskID,
		AgentID:  agentID,
		RoleName: agent.RoleName,
	})
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
