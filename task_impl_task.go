// task_impl_task.go — core Task CRUD implementation for [taskManager].
//
// Project operations are in project.go.
// Assignment operations are in assignment.go.
// Agent operations are in agent.go.
// Relationship operations are in relationship.go.
// Import operations are in task_impl_import.go.
// Entity↔domain converters are in task_impl_converters.go.
package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// CreateTask creates a Task entity in the agency graph.
func (m *taskManager) CreateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	task.AgencyID = agencyID
	task.Status = TaskStatusPending
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.Priority == "" {
		task.Priority = TaskPriorityMedium
	}
	task.CompletedAt = ""

	created, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   agencyID,
		TypeID:     taskTypeID,
		Properties: taskToProperties(task),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityAlreadyExists) {
			return Task{}, ErrTaskAlreadyExists
		}
		return Task{}, fmt.Errorf("CreateTask: %w", err)
	}

	out := taskFromEntity(created)
	m.publish(ctx, TopicTaskCreated, agencyID, TaskCreatedPayload{
		TaskID:   out.ID,
		Priority: out.Priority,
	})
	return out, nil
}

// GetTask reads a single Task entity from the agency graph.
func (m *taskManager) GetTask(ctx context.Context, agencyID, taskID string) (Task, error) {
	e, err := m.dm.GetEntity(ctx, agencyID, taskID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("GetTask: %w", err)
	}
	if e.AgencyID != agencyID || e.TypeID != taskTypeID {
		return Task{}, ErrTaskNotFound
	}
	t := taskFromEntity(e)
	t.Tags = m.loadTagNames(ctx, agencyID, t.ID)
	return t, nil
}

// UpdateTask validates the requested status transition then patches the
// stored entity properties.
//
// The pending → in_progress transition is additionally gated by the blocker
// rule: any inbound `blocks` edge whose source task has not reached a
// terminal status returns a *BlockedError listing the offending blocker task IDs.
func (m *taskManager) UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
	current, err := m.GetTask(ctx, agencyID, task.ID)
	if err != nil {
		return Task{}, err
	}
	if current.Status != task.Status && !current.Status.CanTransitionTo(task.Status) {
		return Task{}, ErrInvalidStatusTransition
	}
	if current.Status == TaskStatusPending && task.Status == TaskStatusInProgress {
		if blockers, err := m.findActiveBlockers(ctx, agencyID, task.ID); err != nil {
			return Task{}, err
		} else if len(blockers) > 0 {
			return Task{}, &BlockedError{BlockerTaskIDs: blockers}
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	task.AgencyID = agencyID
	task.UpdatedAt = now
	task.CreatedAt = current.CreatedAt
	if isTerminalStatus(task.Status) && task.CompletedAt == "" {
		task.CompletedAt = now
	}

	updated, err := m.dm.UpdateEntity(ctx, agencyID, task.ID, entitygraph.UpdateEntityRequest{
		Properties: taskToProperties(task),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Task{}, ErrTaskNotFound
		}
		return Task{}, fmt.Errorf("UpdateTask: %w", err)
	}

	out := taskFromEntity(updated)

	if changed := nonStatusChangedFields(current, out); len(changed) > 0 {
		m.publish(ctx, TopicTaskUpdated, agencyID, TaskUpdatedPayload{
			TaskID:        out.ID,
			ChangedFields: changed,
		})
	}
	if current.Status != out.Status {
		m.publish(ctx, TopicTaskStatusChanged, agencyID, TaskStatusChangedPayload{
			TaskID: out.ID,
			From:   current.Status,
			To:     out.Status,
		})
		if isTerminalStatus(out.Status) {
			completedAt := out.CompletedAt
			if completedAt == "" {
				completedAt = now
			}
			m.publish(ctx, TopicTaskCompleted, agencyID, TaskCompletedPayload{
				TaskID:         out.ID,
				TerminalStatus: out.Status,
				CompletedAt:    completedAt,
			})
		}
	}
	return out, nil
}

// DeleteTask soft-deletes the Task entity.
func (m *taskManager) DeleteTask(ctx context.Context, agencyID, taskID string) error {
	if _, err := m.GetTask(ctx, agencyID, taskID); err != nil {
		return err
	}
	if err := m.dm.DeleteEntity(ctx, agencyID, taskID); err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return ErrTaskNotFound
		}
		return fmt.Errorf("DeleteTask: %w", err)
	}
	return nil
}

// ListTasks returns all non-deleted Task entities for the agency that match
// the filter.
func (m *taskManager) ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error) {
	props := map[string]any{}
	if filter.Status != "" {
		props["status"] = string(filter.Status)
	}
	if filter.Priority != "" {
		props["priority"] = string(filter.Priority)
	}

	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID:   agencyID,
		TypeID:     taskTypeID,
		Properties: props,
	})
	if err != nil {
		return nil, fmt.Errorf("ListTasks: %w", err)
	}

	tasks := make([]Task, 0, len(entities))
	for _, e := range entities {
		t := taskFromEntity(e)
		t.Tags = m.loadTagNames(ctx, agencyID, t.ID)
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// loadTagNames traverses outbound has_tag edges from taskID and returns the
// name of each linked Tag entity. Errors are silently swallowed — a missing
// or unreadable tag is omitted rather than failing the parent call.
func (m *taskManager) loadTagNames(ctx context.Context, agencyID, taskID string) []string {
	edges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelHasTag, DirectionOutbound)
	if err != nil || len(edges) == 0 {
		return nil
	}
	names := make([]string, 0, len(edges))
	for _, edge := range edges {
		tagEntity, err := m.dm.GetEntity(ctx, agencyID, edge.ToID)
		if err != nil {
			continue
		}
		if name := entitygraph.StringProp(tagEntity.Properties, "name"); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// publish emits a typed [eventbus.Event] via the optional Publisher.
// A nil publisher is silently skipped; errors are swallowed — events are
// best-effort and must not fail the originating operation.
func (m *taskManager) publish(ctx context.Context, topic, agencyID string, payload any) {
	eventbus.SafePublish(ctx, m.publisher, eventbus.Event{
		Topic:    topic,
		AgencyID: agencyID,
		Payload:  payload,
	})
}

// isTerminalStatus reports whether the status is one of the terminal lifecycle
// states (completed, failed, cancelled).
func isTerminalStatus(s TaskStatus) bool {
	switch s {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

// nonStatusChangedFields lists the mutable Task property names that differ
// between before and after, excluding Status (reported separately via
// [TopicTaskStatusChanged]). Returns nil when nothing non-status differs.
func nonStatusChangedFields(before, after Task) []string {
	var out []string
	if before.Description != after.Description {
		out = append(out, "description")
	}
	if before.Priority != after.Priority {
		out = append(out, "priority")
	}
	if before.DueAt != after.DueAt {
		out = append(out, "due_at")
	}
	if !stringSlicesEqual(before.Tags, after.Tags) {
		out = append(out, "tags")
	}
	if before.EstimatedHours != after.EstimatedHours {
		out = append(out, "estimated_hours")
	}
	if before.Context != after.Context {
		out = append(out, "context")
	}
	return out
}

// stringSlicesEqual reports whether two string slices have identical
// length and elements in order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// findActiveBlockers returns the IDs of tasks that block taskID via inbound
// `blocks` edges and are themselves still non-terminal.
func (m *taskManager) findActiveBlockers(ctx context.Context, agencyID, taskID string) ([]string, error) {
	edges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelBlocks, DirectionInbound)
	if err != nil {
		return nil, fmt.Errorf("findActiveBlockers: %w", err)
	}
	var nonTerminal []string
	for _, e := range edges {
		blocker, err := m.GetTask(ctx, agencyID, e.FromID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				continue
			}
			return nil, fmt.Errorf("findActiveBlockers: get %s: %w", e.FromID, err)
		}
		if !isTerminalStatus(blocker.Status) {
			nonTerminal = append(nonTerminal, blocker.ID)
		}
	}
	return nonTerminal, nil
}
