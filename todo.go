package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// CreateTaskTodo creates a TaskTodo entity in the agency graph.
// Todos with non-empty DependsOn start as [TodoStatusBlocked] and are NOT
// dispatched — the caller must call [DispatchTaskTodo] once their predecessors
// complete. Todos with no dependencies start as [TodoStatusPending]; callers
// should call [DispatchTaskTodo] immediately after creation.
func (m *taskManager) CreateTaskTodo(ctx context.Context, agencyID string, todo TaskTodo) (TaskTodo, error) {
	if todo.Title == "" || todo.Instructions == "" || todo.ParentTaskID == "" {
		return TaskTodo{}, fmt.Errorf("%w: title, instructions, and parent_task_id are required", ErrInvalidTask)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	todo.AgencyID = agencyID
	if len(todo.DependsOn) > 0 {
		todo.Status = TodoStatusBlocked
	} else {
		todo.Status = TodoStatusPending
	}
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
	return taskTodoFromEntity(created), nil
}

// DispatchTaskTodo publishes [TopicTodoDispatched] for an existing todo so
// CodeValdAI agents can pick it up. If the todo is currently
// [TodoStatusBlocked], it is first advanced to [TodoStatusPending].
func (m *taskManager) DispatchTaskTodo(ctx context.Context, agencyID, todoID string) error {
	todo, err := m.GetTaskTodo(ctx, agencyID, todoID)
	if err != nil {
		return err
	}
	if todo.Status == TodoStatusBlocked {
		now := time.Now().UTC().Format(time.RFC3339)
		updated, err := m.dm.UpdateEntity(ctx, agencyID, todoID, entitygraph.UpdateEntityRequest{
			Properties: map[string]any{
				"status":     string(TodoStatusPending),
				"updated_at": now,
			},
		})
		if err != nil {
			return fmt.Errorf("DispatchTaskTodo: unblock %s: %w", todoID, err)
		}
		todo = taskTodoFromEntity(updated)
	}
	m.publish(ctx, TopicTodoDispatched, agencyID, TodoDispatchedPayload{
		TodoID:         todo.ID,
		TaskID:         todo.ParentTaskID,
		ParentTaskID:   todo.ParentTaskID,
		DecompRunID:    todo.DecompRunID,
		AgentID:        todo.AgentID,
		Title:          todo.Title,
		Instructions:   todo.Instructions,
		Ordinality:     todo.Ordinality,
		CanRunParallel: todo.CanRunParallel,
		DependsOn:      todo.DependsOn,
		Precalls:       todo.Precalls,
	})
	return nil
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
