package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// TaskGroup is an optional container that groups related tasks (e.g. a sprint,
// a project milestone, or an epic). Tasks become members via the `member_of`
// graph edge — many-to-many; a Task may belong to multiple Groups.
type TaskGroup struct {
	// ID is the unique identifier for this group within the agency.
	// Set by the backend on creation.
	ID string

	// AgencyID is the agency that owns this group.
	AgencyID string

	// Name is the short human-readable label. Required.
	Name string

	// Description provides additional context for the group. Optional.
	Description string

	// DueAt is the target completion date for the group. Optional.
	DueAt *time.Time

	// CreatedAt is the UTC timestamp when the group was first created.
	CreatedAt time.Time

	// UpdatedAt is the UTC timestamp of the most recent mutation.
	UpdatedAt time.Time
}

// taskGroupToProperties serialises a TaskGroup into the property map stored on
// its entitygraph Entity. Time fields are encoded as RFC 3339 strings.
func taskGroupToProperties(g TaskGroup) map[string]any {
	props := map[string]any{
		"name":        g.Name,
		"description": g.Description,
	}
	if g.DueAt != nil && !g.DueAt.IsZero() {
		props["dueAt"] = g.DueAt.UTC().Format(time.RFC3339Nano)
	}
	return props
}

// taskGroupFromEntity reconstructs a TaskGroup from an entitygraph Entity.
func taskGroupFromEntity(e entitygraph.Entity) TaskGroup {
	g := TaskGroup{
		ID:        e.ID,
		AgencyID:  e.AgencyID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if v, ok := e.Properties["name"].(string); ok {
		g.Name = v
	}
	if v, ok := e.Properties["description"].(string); ok {
		g.Description = v
	}
	if v, ok := e.Properties["dueAt"].(string); ok && v != "" {
		if ts, err := time.Parse(time.RFC3339Nano, v); err == nil {
			g.DueAt = &ts
		}
	}
	return g
}

// CreateTaskGroup creates a new TaskGroup vertex in the agency graph.
// Returns [ErrInvalidTask] when Name is empty (re-using the Task error for
// "missing required fields" — consistent across the package), and
// [ErrTaskGroupAlreadyExists] if the underlying store reports a duplicate.
func (m *taskManager) CreateTaskGroup(ctx context.Context, agencyID string, g TaskGroup) (TaskGroup, error) {
	if g.Name == "" {
		return TaskGroup{}, fmt.Errorf("%w: TaskGroup.Name is required", ErrInvalidTask)
	}
	g.AgencyID = agencyID
	created, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   agencyID,
		TypeID:     taskGroupTypeID,
		Properties: taskGroupToProperties(g),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityAlreadyExists) {
			return TaskGroup{}, ErrTaskGroupAlreadyExists
		}
		return TaskGroup{}, fmt.Errorf("CreateTaskGroup: %w", err)
	}
	return taskGroupFromEntity(created), nil
}

// GetTaskGroup reads a single TaskGroup by its entity ID. Returns
// [ErrTaskGroupNotFound] when the entity does not exist or is not a
// TaskGroup vertex.
func (m *taskManager) GetTaskGroup(ctx context.Context, agencyID, groupID string) (TaskGroup, error) {
	e, err := m.dm.GetEntity(ctx, agencyID, groupID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return TaskGroup{}, ErrTaskGroupNotFound
		}
		return TaskGroup{}, fmt.Errorf("GetTaskGroup: %w", err)
	}
	if e.AgencyID != agencyID || e.TypeID != taskGroupTypeID {
		return TaskGroup{}, ErrTaskGroupNotFound
	}
	return taskGroupFromEntity(e), nil
}

// UpdateTaskGroup patches the mutable fields (Name, Description, DueAt) of an
// existing TaskGroup. Returns [ErrTaskGroupNotFound] if the group does not
// exist.
func (m *taskManager) UpdateTaskGroup(ctx context.Context, agencyID string, g TaskGroup) (TaskGroup, error) {
	current, err := m.GetTaskGroup(ctx, agencyID, g.ID)
	if err != nil {
		return TaskGroup{}, err
	}
	if g.Name == "" {
		return TaskGroup{}, fmt.Errorf("%w: TaskGroup.Name is required", ErrInvalidTask)
	}
	g.AgencyID = agencyID
	g.CreatedAt = current.CreatedAt

	updated, err := m.dm.UpdateEntity(ctx, agencyID, g.ID, entitygraph.UpdateEntityRequest{
		Properties: taskGroupToProperties(g),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return TaskGroup{}, ErrTaskGroupNotFound
		}
		return TaskGroup{}, fmt.Errorf("UpdateTaskGroup: %w", err)
	}
	return taskGroupFromEntity(updated), nil
}

// DeleteTaskGroup soft-deletes the TaskGroup vertex AND hard-deletes every
// inbound `member_of` edge. Member Tasks are not affected — they remain in
// the agency, just no longer attached to this group.
//
// Returns [ErrTaskGroupNotFound] if the group does not exist.
func (m *taskManager) DeleteTaskGroup(ctx context.Context, agencyID, groupID string) error {
	if _, err := m.GetTaskGroup(ctx, agencyID, groupID); err != nil {
		return err
	}
	edges, err := m.TraverseRelationships(ctx, agencyID, groupID, RelLabelMemberOf, DirectionInbound)
	if err != nil {
		return fmt.Errorf("DeleteTaskGroup: traverse: %w", err)
	}
	for _, e := range edges {
		if err := m.dm.DeleteRelationship(ctx, agencyID, e.ID); err != nil {
			if errors.Is(err, entitygraph.ErrRelationshipNotFound) {
				continue
			}
			return fmt.Errorf("DeleteTaskGroup: delete edge %s: %w", e.ID, err)
		}
	}
	if err := m.dm.DeleteEntity(ctx, agencyID, groupID); err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return ErrTaskGroupNotFound
		}
		return fmt.Errorf("DeleteTaskGroup: %w", err)
	}
	return nil
}

// ListTaskGroups returns all non-deleted TaskGroups in the agency.
func (m *taskManager) ListTaskGroups(ctx context.Context, agencyID string) ([]TaskGroup, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: agencyID,
		TypeID:   taskGroupTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("ListTaskGroups: %w", err)
	}
	out := make([]TaskGroup, 0, len(entities))
	for _, e := range entities {
		out = append(out, taskGroupFromEntity(e))
	}
	return out, nil
}

// AddTaskToGroup creates the `member_of` edge from taskID to groupID.
// Idempotent — re-adding an existing membership returns nil. Returns
// [ErrTaskNotFound] / [ErrTaskGroupNotFound] when the respective vertex is
// missing.
func (m *taskManager) AddTaskToGroup(ctx context.Context, agencyID, taskID, groupID string) error {
	_, err := m.CreateRelationship(ctx, agencyID, Relationship{
		Label:  RelLabelMemberOf,
		FromID: taskID,
		ToID:   groupID,
		Properties: map[string]any{
			"addedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return fmt.Errorf("AddTaskToGroup: %w", err)
	}
	return nil
}

// RemoveTaskFromGroup removes the `member_of` edge from taskID to groupID.
// Returns [ErrRelationshipNotFound] if no membership existed.
func (m *taskManager) RemoveTaskFromGroup(ctx context.Context, agencyID, taskID, groupID string) error {
	return m.DeleteRelationship(ctx, agencyID, taskID, groupID, RelLabelMemberOf)
}

// ListTasksInGroup returns the Tasks that are members of the given group via
// inbound `member_of` edges. Tasks the caller has soft-deleted are skipped
// (they're not returned).
func (m *taskManager) ListTasksInGroup(ctx context.Context, agencyID, groupID string) ([]Task, error) {
	edges, err := m.TraverseRelationships(ctx, agencyID, groupID, RelLabelMemberOf, DirectionInbound)
	if err != nil {
		return nil, fmt.Errorf("ListTasksInGroup: traverse: %w", err)
	}
	out := make([]Task, 0, len(edges))
	for _, e := range edges {
		t, err := m.GetTask(ctx, agencyID, e.FromID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListTasksInGroup: get %s: %w", e.FromID, err)
		}
		out = append(out, t)
	}
	return out, nil
}

// ListGroupsForTask returns the TaskGroups the given Task belongs to via
// outbound `member_of` edges.
func (m *taskManager) ListGroupsForTask(ctx context.Context, agencyID, taskID string) ([]TaskGroup, error) {
	edges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelMemberOf, DirectionOutbound)
	if err != nil {
		return nil, fmt.Errorf("ListGroupsForTask: traverse: %w", err)
	}
	out := make([]TaskGroup, 0, len(edges))
	for _, e := range edges {
		g, err := m.GetTaskGroup(ctx, agencyID, e.ToID)
		if err != nil {
			if errors.Is(err, ErrTaskGroupNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListGroupsForTask: get %s: %w", e.ToID, err)
		}
		out = append(out, g)
	}
	return out, nil
}
