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

// taskGroupTypeID is the TypeDefinition.Name used for TaskGroup entities.
const taskGroupTypeID = "TaskGroup"

// agentTypeID is the TypeDefinition.Name used for Agent entities.
const agentTypeID = "Agent"

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

	// CreateRelationship creates a directed edge between two Work vertices.
	// rel.Label must be one of the RelLabel* constants and the (FromID, ToID)
	// vertex types must match the label's whitelist entry, otherwise
	// [ErrInvalidRelationship] is returned.
	//
	// Both endpoints must already exist in the same agency. A missing endpoint
	// returns [ErrTaskNotFound], [ErrAgentNotFound], or [ErrTaskGroupNotFound]
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
	// vertexID with the given label and direction. Multi-hop traversal is
	// out of scope — callers needing it should use entitygraph.DataManager.TraverseGraph
	// directly.
	//
	// Returns an empty slice (not an error) when no edges match. If vertexID
	// does not exist in the agency, the underlying graph traversal returns an
	// empty result — callers that need a strict not-found check should call
	// [TaskManager.GetTask] (or the equivalent type-specific lookup) first.
	TraverseRelationships(ctx context.Context, agencyID, vertexID, label string, dir Direction) ([]Relationship, error)

	// UpsertAgent creates or merges an Agent vertex keyed by the
	// (agencyID, agent.AgentID) natural key. On the merge branch, displayName
	// and capability are updated; agentID is immutable.
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

	// CreateTaskGroup creates a new TaskGroup vertex. Returns
	// [ErrInvalidTask] when Name is empty and [ErrTaskGroupAlreadyExists]
	// when the underlying store reports a duplicate.
	CreateTaskGroup(ctx context.Context, agencyID string, g TaskGroup) (TaskGroup, error)

	// GetTaskGroup retrieves a single TaskGroup by entity ID. Returns
	// [ErrTaskGroupNotFound] if no matching group exists.
	GetTaskGroup(ctx context.Context, agencyID, groupID string) (TaskGroup, error)

	// UpdateTaskGroup replaces the mutable fields of an existing TaskGroup.
	// Returns [ErrTaskGroupNotFound] if the group does not exist.
	UpdateTaskGroup(ctx context.Context, agencyID string, g TaskGroup) (TaskGroup, error)

	// DeleteTaskGroup soft-deletes the TaskGroup vertex AND removes every
	// inbound `member_of` edge so member Tasks lose the membership.
	// Member Tasks themselves are not deleted.
	DeleteTaskGroup(ctx context.Context, agencyID, groupID string) error

	// ListTaskGroups returns all non-deleted TaskGroups in the agency.
	ListTaskGroups(ctx context.Context, agencyID string) ([]TaskGroup, error)

	// AddTaskToGroup writes the `member_of` edge from taskID to groupID.
	// Idempotent — returns nil whether or not the edge already existed.
	AddTaskToGroup(ctx context.Context, agencyID, taskID, groupID string) error

	// RemoveTaskFromGroup removes the `member_of` edge from taskID to
	// groupID. Returns [ErrRelationshipNotFound] if no membership existed.
	RemoveTaskFromGroup(ctx context.Context, agencyID, taskID, groupID string) error

	// ListTasksInGroup returns the Tasks belonging to the given group via
	// inbound `member_of` edges.
	ListTasksInGroup(ctx context.Context, agencyID, groupID string) ([]Task, error)

	// ListGroupsForTask returns the TaskGroups the given Task belongs to
	// via outbound `member_of` edges.
	ListGroupsForTask(ctx context.Context, agencyID, taskID string) ([]TaskGroup, error)
}

// WorkSchemaManager is a type alias for [entitygraph.SchemaManager].
// Used by internal/app to seed [DefaultWorkSchema] on startup.
type WorkSchemaManager = entitygraph.SchemaManager

// CrossPublisher is the historical name for the event-publishing contract
// CodeValdWork callers inject. As of MVP-WORK-014 it is a type alias for
// [eventbus.Publisher] — the SharedLib package that unifies the publish
// contract across CodeValdAgency, CodeValdComm, CodeValdDT, and CodeValdWork.
//
// New callers should refer to [eventbus.Publisher] directly; this alias
// remains for source compatibility and reads well in the [NewTaskManager]
// signature.
type CrossPublisher = eventbus.Publisher

// taskManager is the concrete implementation of [TaskManager].
// It wraps an [entitygraph.DataManager] to persist Task entities in the
// agency graph and emits work.task.* events via the optional Publisher.
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
	m.publish(ctx, TopicTaskCreated, agencyID, TaskCreatedPayload{
		TaskID:   out.ID,
		Title:    out.Title,
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
// terminal status (completed / failed / cancelled) returns a *BlockedError
// (which wraps [ErrBlocked]) listing the offending blocker task IDs.
// Other transitions — including pending → cancelled — bypass the gate.
func (m *taskManager) UpdateTask(ctx context.Context, agencyID string, task Task) (Task, error) {
	current, err := m.GetTask(ctx, agencyID, task.ID)
	if err != nil {
		return Task{}, err
	}
	// Only validate the transition when status is actually changing —
	// same-status edits (e.g. patching title or description while pending)
	// are no-op transitions and must be permitted.
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

	// Publish hooks. Order matters: status.changed precedes completed when
	// both fire so subscribers see the transition before the terminal hook.
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
			completedAt := now
			if out.CompletedAt != nil {
				completedAt = *out.CompletedAt
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
// the filter. Status and Priority are pushed down to the DataManager's
// property filter; assignment-by-agent is no longer a property — callers
// needing it should traverse inbound `assigned_to` from the Agent vertex.
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
// A nil publisher is silently skipped; errors from the publisher are
// swallowed — events are best-effort and must not fail the originating
// operation.
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
// [TopicTaskStatusChanged]) and entity timestamps. Drives
// [TaskUpdatedPayload.ChangedFields] so subscribers know what changed
// without diffing the payload themselves. Returns nil when nothing
// non-status differs.
func nonStatusChangedFields(before, after Task) []string {
	var out []string
	if before.Title != after.Title {
		out = append(out, "title")
	}
	if before.Description != after.Description {
		out = append(out, "description")
	}
	if before.Priority != after.Priority {
		out = append(out, "priority")
	}
	if !timePtrEqual(before.DueAt, after.DueAt) {
		out = append(out, "dueAt")
	}
	if !stringSlicesEqual(before.Tags, after.Tags) {
		out = append(out, "tags")
	}
	if before.EstimatedHours != after.EstimatedHours {
		out = append(out, "estimatedHours")
	}
	if before.Context != after.Context {
		out = append(out, "context")
	}
	return out
}

// timePtrEqual reports whether two *time.Time pointers refer to the same
// instant (or are both nil).
func timePtrEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// stringSlicesEqual reports whether two string slices have identical
// length and elements in order. Used by [nonStatusChangedFields] to
// detect Task.Tags changes.
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
// `blocks` edges and are themselves still non-terminal. An empty slice
// (paired with nil error) means the gate is open.
func (m *taskManager) findActiveBlockers(ctx context.Context, agencyID, taskID string) ([]string, error) {
	edges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelBlocks, DirectionInbound)
	if err != nil {
		return nil, fmt.Errorf("findActiveBlockers: %w", err)
	}
	var nonTerminal []string
	for _, e := range edges {
		blocker, err := m.GetTask(ctx, agencyID, e.FromID)
		if err != nil {
			// A missing or non-Task source vertex cannot constrain
			// progress — skip it rather than fail the whole transition.
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
