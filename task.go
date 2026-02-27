// Package codevaldwork provides task lifecycle management for CodeValdCortex
// agencies. It exposes [TaskManager] — the single interface for creating,
// reading, updating, deleting, and listing tasks assigned to AI agents.
//
// Usage:
//
//	b, err := arangodb.NewArangoBackend(arangodb.Config{...})
//	mgr, err := codevaldwork.NewTaskManager(b)
//	task, err := mgr.CreateTask(ctx, "agency-1", codevaldwork.Task{Title: "Research"})
package codevaldwork

import (
	"context"
	"fmt"
)

// TaskManager is the primary interface for task lifecycle management.
// All operations are scoped to an agency via agencyID.
//
// Implementations must be safe for concurrent use.
type TaskManager interface {
	// CreateTask creates a new task for the given agency.
	// The task is assigned a server-generated ID and starts in [TaskStatusPending].
	// Returns [ErrTaskAlreadyExists] if a task with the same ID already exists.
	// Returns [ErrInvalidTask] if required fields (Title) are missing.
	CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)

	// GetTask retrieves a single task by its ID within the given agency.
	// Returns [ErrTaskNotFound] if no matching task exists.
	GetTask(ctx context.Context, agencyID, taskID string) (Task, error)

	// UpdateTask replaces the mutable fields of an existing task.
	// Status transitions are validated — returns [ErrInvalidStatusTransition]
	// if the new status is not reachable from the current status.
	// Returns [ErrTaskNotFound] if the task does not exist.
	UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error)

	// DeleteTask permanently removes a task from the agency.
	// Returns [ErrTaskNotFound] if the task does not exist.
	DeleteTask(ctx context.Context, agencyID, taskID string) error

	// ListTasks returns all tasks for the given agency that match the filter.
	// Returns an empty slice (not an error) when no tasks match.
	ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error)
}

// Backend is the storage abstraction injected into [TaskManager].
// Callers construct a Backend from storage/arangodb and pass it to
// [NewTaskManager]. The root package never imports any storage driver.
type Backend interface {
	// CreateTask persists a new task document and returns it with any
	// server-assigned fields (ID, CreatedAt) populated.
	CreateTask(ctx context.Context, agencyID string, task Task) (Task, error)

	// GetTask retrieves a task by agencyID and taskID.
	// Returns [ErrTaskNotFound] if no matching document exists.
	GetTask(ctx context.Context, agencyID, taskID string) (Task, error)

	// UpdateTask replaces the stored task document and returns the updated task.
	// Returns [ErrTaskNotFound] if the task does not exist.
	UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error)

	// DeleteTask removes the task document.
	// Returns [ErrTaskNotFound] if the task does not exist.
	DeleteTask(ctx context.Context, agencyID, taskID string) error

	// ListTasks returns tasks for the agency matching the filter.
	ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error)
}

// taskManager is the concrete implementation of [TaskManager].
// It delegates all storage operations to the injected [Backend].
type taskManager struct {
	backend Backend
}

// NewTaskManager constructs a [TaskManager] backed by the given [Backend].
// Use storage/arangodb.NewArangoBackend to construct a Backend, then pass
// it here.
// Returns an error if b is nil.
func NewTaskManager(b Backend) (TaskManager, error) {
	if b == nil {
		return nil, fmt.Errorf("NewTaskManager: backend must not be nil")
	}
	return &taskManager{backend: b}, nil
}

// CreateTask validates the task and delegates to [Backend.CreateTask].
func (m *taskManager) CreateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
	if task.Title == "" {
		return Task{}, ErrInvalidTask
	}
	return m.backend.CreateTask(ctx, agencyID, task)
}

// GetTask delegates to [Backend.GetTask].
func (m *taskManager) GetTask(ctx context.Context, agencyID, taskID string) (Task, error) {
	return m.backend.GetTask(ctx, agencyID, taskID)
}

// UpdateTask validates the status transition and delegates to [Backend.UpdateTask].
func (m *taskManager) UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
	current, err := m.backend.GetTask(ctx, agencyID, task.ID)
	if err != nil {
		return Task{}, err
	}
	if !current.Status.CanTransitionTo(task.Status) {
		return Task{}, ErrInvalidStatusTransition
	}
	return m.backend.UpdateTask(ctx, agencyID, task)
}

// DeleteTask delegates to [Backend.DeleteTask].
func (m *taskManager) DeleteTask(ctx context.Context, agencyID, taskID string) error {
	return m.backend.DeleteTask(ctx, agencyID, taskID)
}

// ListTasks delegates to [Backend.ListTasks].
func (m *taskManager) ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error) {
	return m.backend.ListTasks(ctx, agencyID, filter)
}
