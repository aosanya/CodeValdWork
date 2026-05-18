package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// CreateTaskTodo creates a TaskTodo entity in the agency graph and publishes
// [TopicTaskTodo] so CodeValdAI agents can pick it up via work plans.
func (m *taskManager) CreateTaskTodo(ctx context.Context, agencyID string, todo TaskTodo) (TaskTodo, error) {
	if todo.Title == "" || todo.Instructions == "" || todo.ParentTaskID == "" {
		return TaskTodo{}, fmt.Errorf("%w: title, instructions, and parent_task_id are required", ErrInvalidTask)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	todo.AgencyID = agencyID
	todo.Status = TodoStatusPending
	todo.CreatedAt = now
	todo.UpdatedAt = now

	created, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   agencyID,
		TypeID:     taskTodoTypeID,
		Properties: taskTodoToProperties(todo),
	})
	if err != nil {
		return TaskTodo{}, fmt.Errorf("CreateTaskTodo: %w", err)
	}

	out := taskTodoFromEntity(created)
	m.publish(ctx, TopicTaskTodo, agencyID, TaskTodoPayload{
		TodoID:         out.ID,
		ParentTaskID:   out.ParentTaskID,
		DecompRunID:    out.DecompRunID,
		AgentID:        out.AgentID,
		Title:          out.Title,
		Instructions:   out.Instructions,
		Ordinality:     out.Ordinality,
		CanRunParallel: out.CanRunParallel,
		DependsOn:      out.DependsOn,
	})
	return out, nil
}

// GetTaskTodo reads a single TaskTodo entity from the agency graph.
func (m *taskManager) GetTaskTodo(ctx context.Context, agencyID, todoID string) (TaskTodo, error) {
	e, err := m.dm.GetEntity(ctx, agencyID, todoID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return TaskTodo{}, ErrTaskTodoNotFound
		}
		return TaskTodo{}, fmt.Errorf("GetTaskTodo: %w", err)
	}
	if e.AgencyID != agencyID || e.TypeID != taskTodoTypeID {
		return TaskTodo{}, ErrTaskTodoNotFound
	}
	return taskTodoFromEntity(e), nil
}

// UpdateTaskTodoStatus transitions a TaskTodo to a new [TodoStatus].
func (m *taskManager) UpdateTaskTodoStatus(ctx context.Context, agencyID, todoID string, status TodoStatus) (TaskTodo, error) {
	if _, err := m.GetTaskTodo(ctx, agencyID, todoID); err != nil {
		return TaskTodo{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	updated, err := m.dm.UpdateEntity(ctx, agencyID, todoID, entitygraph.UpdateEntityRequest{
		Properties: map[string]any{
			"status":     string(status),
			"updated_at": now,
		},
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return TaskTodo{}, ErrTaskTodoNotFound
		}
		return TaskTodo{}, fmt.Errorf("UpdateTaskTodoStatus: %w", err)
	}
	return taskTodoFromEntity(updated), nil
}
