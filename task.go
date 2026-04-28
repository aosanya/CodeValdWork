// Package codevaldwork provides task lifecycle management for CodeValdCortex
// agencies. It exposes [TaskManager] — the single interface for creating,
// reading, updating, deleting, and listing tasks assigned to AI agents.
//
// Storage is delegated to a [github.com/aosanya/CodeValdSharedLib/entitygraph.DataManager],
// so Tasks live in the agency-scoped graph alongside every other CodeVald entity
// type. Use storage/arangodb.NewBackend to construct a DataManager and pass it
// to [NewTaskManager].
package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// taskTypeID is the TypeDefinition.Name used for Task entities in the schema.
const taskTypeID = "Task"

// TaskManager is the primary interface for task lifecycle management.
// All operations are scoped to the manager's agencyID, fixed at construction.
//
// Implementations must be safe for concurrent use.
type TaskManager interface {
	// CreateTask creates a new task for the agency.
	// The task is assigned a server-generated ID and starts in [TaskStatusPending].
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

	// DeleteTask soft-deletes a task from the agency graph.
	// Returns [ErrTaskNotFound] if the task does not exist.
	DeleteTask(ctx context.Context, agencyID, taskID string) error

	// ListTasks returns all non-deleted tasks for the given agency that match
	// the filter. Returns an empty slice (not an error) when no tasks match.
	ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error)
}

// WorkSchemaManager is a type alias for [entitygraph.SchemaManager].
// Used by internal/app to seed [DefaultWorkSchema] on startup.
type WorkSchemaManager = entitygraph.SchemaManager

// CrossPublisher publishes Work lifecycle events to CodeValdCross.
// Implementations must be safe for concurrent use. A nil CrossPublisher is
// valid — publish calls are silently skipped.
type CrossPublisher interface {
	// Publish delivers an event for the given topic and agencyID to
	// CodeValdCross. Errors are non-fatal: implementations should log and
	// return nil for best-effort delivery.
	Publish(ctx context.Context, topic string, agencyID string) error
}

// taskManager is the concrete implementation of [TaskManager].
// It wraps an [entitygraph.DataManager] to persist Task entities in the
// agency graph and emits work.task.* events via the optional CrossPublisher.
type taskManager struct {
	dm        entitygraph.DataManager
	publisher CrossPublisher // optional; nil = skip event publishing
}

// NewTaskManager constructs a [TaskManager] backed by the given
// [entitygraph.DataManager].
// pub may be nil — cross-service events are skipped when no publisher is set.
// Returns an error if dm is nil.
func NewTaskManager(dm entitygraph.DataManager, pub CrossPublisher) (TaskManager, error) {
	if dm == nil {
		return nil, fmt.Errorf("NewTaskManager: data manager must not be nil")
	}
	return &taskManager{dm: dm, publisher: pub}, nil
}

// CreateTask creates a Task entity in the agency graph.
// The entity ID is assigned by the underlying DataManager; any ID supplied on
// the request is ignored.
func (m *taskManager) CreateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
	if task.Title == "" {
		return Task{}, ErrInvalidTask
	}
	now := time.Now().UTC()
	task.AgencyID = agencyID
	task.Status = TaskStatusPending
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.Priority == "" {
		task.Priority = TaskPriorityMedium
	}
	task.CompletedAt = nil

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
	m.publish(ctx, "work.task.created", agencyID)
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
	return taskFromEntity(e), nil
}

// UpdateTask validates the requested status transition then patches the
// stored entity properties.
func (m *taskManager) UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
	current, err := m.GetTask(ctx, agencyID, task.ID)
	if err != nil {
		return Task{}, err
	}
	if !current.Status.CanTransitionTo(task.Status) {
		return Task{}, ErrInvalidStatusTransition
	}

	now := time.Now().UTC()
	task.AgencyID = agencyID
	task.UpdatedAt = now
	task.CreatedAt = current.CreatedAt
	if isTerminalStatus(task.Status) && task.CompletedAt == nil {
		ts := now
		task.CompletedAt = &ts
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
	switch task.Status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		m.publish(ctx, "work.task.completed", agencyID)
	default:
		m.publish(ctx, "work.task.updated", agencyID)
	}
	return out, nil
}

// DeleteTask soft-deletes the Task entity (entitygraph never hard-deletes).
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
// the filter. Filtering on Status / Priority / AssignedTo is pushed down to
// the DataManager's property filter.
func (m *taskManager) ListTasks(ctx context.Context, agencyID string, filter TaskFilter) ([]Task, error) {
	props := map[string]any{}
	if filter.Status != "" {
		props["status"] = string(filter.Status)
	}
	if filter.Priority != "" {
		props["priority"] = string(filter.Priority)
	}
	if filter.AssignedTo != "" {
		props["assigned_to"] = filter.AssignedTo
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
		tasks = append(tasks, taskFromEntity(e))
	}
	return tasks, nil
}

// publish emits an event via the optional CrossPublisher.
// Errors are swallowed — events are best-effort and must not fail the
// originating operation.
func (m *taskManager) publish(ctx context.Context, topic, agencyID string) {
	if m.publisher == nil {
		return
	}
	_ = m.publisher.Publish(ctx, topic, agencyID)
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

// taskToProperties serialises a Task into the property map stored on its
// entitygraph Entity. The schema declares datetime fields as
// PropertyTypeDatetime; this layer encodes them as RFC 3339 strings — the
// canonical wire form for ISO 8601 datetimes. Entity-level timestamps
// (CreatedAt / UpdatedAt) are not written as properties — they are tracked
// by entitygraph natively.
//
// The legacy `assigned_to` property is intentionally not written:
// MVP-WORK-009/010 replace it with an `assigned_to` graph edge between Task
// and Agent vertices.
func taskToProperties(t Task) map[string]any {
	props := map[string]any{
		"title":       t.Title,
		"description": t.Description,
		"status":      string(t.Status),
		"priority":    string(t.Priority),
		"context":     t.Context,
	}
	if t.DueAt != nil && !t.DueAt.IsZero() {
		props["dueAt"] = t.DueAt.UTC().Format(time.RFC3339Nano)
	}
	if len(t.Tags) > 0 {
		tags := make([]string, len(t.Tags))
		copy(tags, t.Tags)
		props["tags"] = tags
	}
	if t.EstimatedHours != 0 {
		props["estimatedHours"] = t.EstimatedHours
	}
	if t.CompletedAt != nil && !t.CompletedAt.IsZero() {
		props["completedAt"] = t.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return props
}

// taskFromEntity reconstructs a Task from an entitygraph Entity.
// Tags and estimatedHours accept both the native Go type (used by the unit
// fakeDataManager) and the JSON-decoded form ([]any / float64) the ArangoDB
// backend returns.
func taskFromEntity(e entitygraph.Entity) Task {
	t := Task{
		ID:        e.ID,
		AgencyID:  e.AgencyID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if v, ok := e.Properties["title"].(string); ok {
		t.Title = v
	}
	if v, ok := e.Properties["description"].(string); ok {
		t.Description = v
	}
	if v, ok := e.Properties["status"].(string); ok {
		t.Status = TaskStatus(v)
	}
	if v, ok := e.Properties["priority"].(string); ok {
		t.Priority = TaskPriority(v)
	}
	if v, ok := e.Properties["context"].(string); ok {
		t.Context = v
	}
	if v, ok := e.Properties["dueAt"].(string); ok && v != "" {
		if ts, err := time.Parse(time.RFC3339Nano, v); err == nil {
			t.DueAt = &ts
		}
	}
	if v, ok := e.Properties["tags"]; ok {
		switch tags := v.(type) {
		case []string:
			t.Tags = append([]string(nil), tags...)
		case []any:
			out := make([]string, 0, len(tags))
			for _, x := range tags {
				if s, ok := x.(string); ok {
					out = append(out, s)
				}
			}
			t.Tags = out
		}
	}
	if v, ok := e.Properties["estimatedHours"]; ok {
		switch n := v.(type) {
		case float64:
			t.EstimatedHours = n
		case float32:
			t.EstimatedHours = float64(n)
		case int:
			t.EstimatedHours = float64(n)
		case int64:
			t.EstimatedHours = float64(n)
		}
	}
	if v, ok := e.Properties["completedAt"].(string); ok && v != "" {
		if ts, err := time.Parse(time.RFC3339Nano, v); err == nil {
			t.CompletedAt = &ts
		}
	}
	return t
}
