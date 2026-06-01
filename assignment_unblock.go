package codevaldwork

import (
	"context"
	"fmt"
	"log"
)

// UnblockDependents implements the inverse trigger of [taskManager.AssignTask]:
// whenever completedTaskID reaches a terminal status, walk every Task with an
// inbound `depends_on` edge from it and flip blocked → pending (and re-fire
// [TopicTaskAssigned]) for the ones whose entire `depends_on` set is now
// satisfied. Tasks that have lost their cached assignee or that still have
// other unmet dependencies are left blocked.
//
// Called by the work.task.completed handler in [server.TaskEventDispatcher].
// Idempotent: a dependent already in a non-blocked state is skipped, so
// PubSub redelivery and self-loop receipts are safely no-op.
func (m *taskManager) UnblockDependents(ctx context.Context, agencyID, completedTaskID string) error {
	if _, err := m.GetTask(ctx, agencyID, completedTaskID); err != nil {
		return err
	}
	edges, err := m.TraverseRelationships(ctx, agencyID, completedTaskID, RelLabelDependsOn, DirectionInbound)
	if err != nil {
		return fmt.Errorf("UnblockDependents: traverse depends_on: %w", err)
	}
	for _, e := range edges {
		m.maybeUnblockDependent(ctx, agencyID, completedTaskID, e.FromID)
	}
	return nil
}

// maybeUnblockDependent evaluates a single dependent task and, if every
// gating condition is satisfied, transitions it pending and republishes
// work.task.assigned. Failures are logged; the loop in UnblockDependents
// continues so one bad dependent does not stall the rest.
func (m *taskManager) maybeUnblockDependent(ctx context.Context, agencyID, completedTaskID, dependentID string) {
	dependent, err := m.GetTask(ctx, agencyID, dependentID)
	if err != nil {
		log.Printf("codevaldwork: UnblockDependents: GetTask %s: %v", dependentID, err)
		return
	}
	if dependent.Status != TaskStatusBlocked {
		return
	}
	unmet, err := m.findUnmetDependencies(ctx, agencyID, dependentID)
	if err != nil {
		log.Printf("codevaldwork: UnblockDependents: findUnmetDependencies %s: %v", dependentID, err)
		return
	}
	if len(unmet) > 0 {
		return
	}
	assignedEdges, err := m.TraverseRelationships(ctx, agencyID, dependentID, RelLabelAssignedTo, DirectionOutbound)
	if err != nil {
		log.Printf("codevaldwork: UnblockDependents: traverse assigned_to %s: %v", dependentID, err)
		return
	}
	if len(assignedEdges) == 0 {
		// No cached assignee — leave blocked; nothing to dispatch.
		return
	}
	agentEntityID := assignedEdges[0].ToID
	agent, err := m.GetAgent(ctx, agencyID, agentEntityID)
	if err != nil {
		log.Printf("codevaldwork: UnblockDependents: GetAgent %s: %v", agentEntityID, err)
		return
	}
	if err := m.setTaskStatus(ctx, agencyID, dependentID, TaskStatusPending); err != nil {
		log.Printf("codevaldwork: UnblockDependents: setTaskStatus %s pending: %v", dependentID, err)
		return
	}
	m.publish(ctx, TopicTaskStatusChanged, agencyID, TaskStatusChangedPayload{
		TaskID: dependentID,
		From:   TaskStatusBlocked,
		To:     TaskStatusPending,
	})
	m.publish(ctx, TopicTaskAssigned, agencyID, TaskAssignedPayload{
		TaskID:      dependentID,
		AgentID:     agentEntityID,
		RoleName:    agent.RoleName,
		TaskCode:    dependent.TaskName,
		Title:       dependent.Title,
		Description: dependent.Description,
	})
	log.Printf("codevaldwork: UnblockDependents: task %s blocked→pending (dep %s satisfied)",
		dependentID, completedTaskID)
}
