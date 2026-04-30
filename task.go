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
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// taskTypeID is the TypeDefinition.Name used for Task entities in the schema.
const taskTypeID = "Task"

// projectTypeID is the TypeDefinition.Name used for Project entities.
const projectTypeID = "Project"

// agentTypeID is the TypeDefinition.Name used for Agent entities.
const agentTypeID = "Agent"

// TaskManager is the primary interface for task lifecycle management.
// All operations are scoped to the manager's agencyID, fixed at construction.
//
// Implementations must be safe for concurrent use.
type TaskManager interface {
	// CreateTask creates a new task for the agency.
	// The task is assigned a server-generated ID and starts in [TaskStatusPending].
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

	// CreateRelationship creates a directed edge between two Work vertices.
	// rel.Label must be one of the RelLabel* constants and the (FromID, ToID)
	// vertex types must match the label's whitelist entry, otherwise
	// [ErrInvalidRelationship] is returned.
	//
	// Both endpoints must already exist in the same agency. A missing endpoint
	// returns [ErrTaskNotFound], [ErrAgentNotFound], or [ErrProjectNotFound]
	// depending on the label's expected vertex type.
	//
	// Re-creating an existing (FromID, ToID, Label) edge is idempotent — the
	// existing edge is returned with no error and no second edge is written.
	CreateRelationship(ctx context.Context, agencyID string, rel Relationship) (Relationship, error)

	// DeleteRelationship removes a single edge identified by the
	// (fromID, toID, label) triple within the agency. Returns
	// [ErrRelationshipNotFound] if no such edge exists.
	DeleteRelationship(ctx context.Context, agencyID, fromID, toID, label string) error

	// TraverseRelationships returns the single-hop edges incident on
	// vertexID with the given label and direction.
	//
	// Returns an empty slice (not an error) when no edges match.
	TraverseRelationships(ctx context.Context, agencyID, vertexID, label string, dir Direction) ([]Relationship, error)

	// UpsertAgent creates or merges an Agent vertex keyed by the
	// (agencyID, agent.AgentID) natural key. On the merge branch, display_name
	// and capability are updated; agent_id is immutable.
	UpsertAgent(ctx context.Context, agencyID string, agent Agent) (Agent, error)

	// GetAgent retrieves a single Agent by its entity ID within the given
	// agency. Returns [ErrAgentNotFound] if no matching entity exists.
	GetAgent(ctx context.Context, agencyID, entityID string) (Agent, error)

	// ListAgents returns all non-deleted Agents in the agency. Returns an
	// empty slice (not an error) when none exist.
	ListAgents(ctx context.Context, agencyID string) ([]Agent, error)

	// AssignTask sets the assignee of a Task by writing the `assigned_to`
	// edge (Task → Agent). Replaces any prior assignee — a Task has at
	// most one outbound `assigned_to` edge. Returns [ErrTaskNotFound] or
	// [ErrAgentNotFound] when the respective vertex is missing.
	AssignTask(ctx context.Context, agencyID, taskID, agentID string) error

	// UnassignTask removes any outbound `assigned_to` edge from the Task.
	// Idempotent — returns nil whether or not an edge was present.
	UnassignTask(ctx context.Context, agencyID, taskID string) error

	// CreateProject creates a new Project vertex. Returns
	// [ErrInvalidTask] when Name is empty and [ErrProjectAlreadyExists]
	// when the underlying store reports a duplicate.
	CreateProject(ctx context.Context, agencyID string, p Project) (Project, error)

	// GetProject retrieves a single Project by entity ID. Returns
	// [ErrProjectNotFound] if no matching project exists.
	GetProject(ctx context.Context, agencyID, projectID string) (Project, error)

	// GetProjectByName retrieves a single Project by its slug (project_name).
	// Returns [ErrProjectNotFound] if no project with that slug exists.
	GetProjectByName(ctx context.Context, agencyID, projectName string) (Project, error)

	// UpdateProject replaces the mutable fields of an existing Project.
	// Returns [ErrProjectNotFound] if the project does not exist.
	UpdateProject(ctx context.Context, agencyID string, p Project) (Project, error)

	// DeleteProject soft-deletes the Project vertex AND removes every
	// inbound `member_of` edge so member Tasks lose the membership.
	// Member Tasks themselves are not deleted.
	DeleteProject(ctx context.Context, agencyID, projectID string) error

	// ListProjects returns all non-deleted Projects in the agency.
	ListProjects(ctx context.Context, agencyID string) ([]Project, error)

	// AddTaskToProject writes the `member_of` edge from taskID to projectID.
	// Idempotent — returns nil whether or not the edge already existed.
	AddTaskToProject(ctx context.Context, agencyID, taskID, projectID string) error

	// RemoveTaskFromProject removes the `member_of` edge from taskID to
	// projectID. Returns [ErrRelationshipNotFound] if no membership existed.
	RemoveTaskFromProject(ctx context.Context, agencyID, taskID, projectID string) error

	// ListTasksInProject returns the Tasks belonging to the given project via
	// inbound `member_of` edges.
	ListTasksInProject(ctx context.Context, agencyID, projectID string) ([]Task, error)

	// ListProjectsForTask returns the Projects the given Task belongs to
	// via outbound `member_of` edges.
	ListProjectsForTask(ctx context.Context, agencyID, taskID string) ([]Project, error)

	// GetTaskByName retrieves a task by its project-scoped name (task_name)
	// within a project (project_name). Returns [ErrTaskNotFound] if no task
	// with that name exists in the project.
	GetTaskByName(ctx context.Context, agencyID, projectName, taskName string) (Task, error)

	// CreateTaskInProject creates a task, auto-generates its task_name from the
	// project's task_prefix, and writes the member_of edge in one atomic sequence.
	// Returns [ErrProjectNotFound] if the project does not exist.
	CreateTaskInProject(ctx context.Context, agencyID, projectName string, task Task) (Task, error)

	// ImportProject parses a JSON import document and creates a Project,
	// Tasks, member_of edges, and depends_on edges in a single call.
	// Returns [ErrInvalidImport] when the document is malformed.
	ImportProject(ctx context.Context, agencyID, markdown string) (ImportResult, error)
}

// WorkSchemaManager is a type alias for [entitygraph.SchemaManager].
// Used by internal/app to seed [DefaultWorkSchema] on startup.
type WorkSchemaManager = entitygraph.SchemaManager

// CrossPublisher is a type alias for [eventbus.Publisher] — the SharedLib
// package that unifies the publish contract across CodeVald services.
type CrossPublisher = eventbus.Publisher

// taskManager is the concrete implementation of [TaskManager].
type taskManager struct {
	dm        entitygraph.DataManager
	publisher eventbus.Publisher // optional; nil = skip event publishing
}

// NewTaskManager constructs a [TaskManager] backed by the given
// [entitygraph.DataManager].
// pub may be nil — cross-service events are skipped when no publisher is set.
// Returns an error if dm is nil.
func NewTaskManager(dm entitygraph.DataManager, pub eventbus.Publisher) (TaskManager, error) {
	if dm == nil {
		return nil, fmt.Errorf("NewTaskManager: data manager must not be nil")
	}
	return &taskManager{dm: dm, publisher: pub}, nil
}

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
	return taskFromEntity(e), nil
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
		tasks = append(tasks, taskFromEntity(e))
	}
	return tasks, nil
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
