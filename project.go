package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// Project is an optional container that groups related tasks (e.g. a sprint,
// a milestone, or an epic). Tasks become members via the `member_of`
// graph edge — many-to-many; a Task may belong to multiple Projects.
type Project struct {
	// ID is the unique identifier for this project within the agency.
	// Set by the backend on creation.
	ID string

	// AgencyID is the agency that owns this project.
	AgencyID string

	// Name is the short human-readable label. Required.
	Name string

	// Description provides additional context for the project. Optional.
	Description string

	// GithubRepo is the canonical GitHub repository for the project, e.g.
	// "owner/name" or a full https URL. Optional.
	GithubRepo string

	// DueAt is the target completion date for the project. Optional.
	DueAt *time.Time

	// CreatedAt is the UTC timestamp when the project was first created.
	CreatedAt time.Time

	// UpdatedAt is the UTC timestamp of the most recent mutation.
	UpdatedAt time.Time
}

// projectToProperties serialises a Project into the property map stored on
// its entitygraph Entity. Time fields are encoded as RFC 3339 strings.
func projectToProperties(p Project) map[string]any {
	props := map[string]any{
		"name":        p.Name,
		"description": p.Description,
		"githubRepo":  p.GithubRepo,
	}
	if p.DueAt != nil && !p.DueAt.IsZero() {
		props["dueAt"] = p.DueAt.UTC().Format(time.RFC3339Nano)
	}
	return props
}

// projectFromEntity reconstructs a Project from an entitygraph Entity.
func projectFromEntity(e entitygraph.Entity) Project {
	p := Project{
		ID:        e.ID,
		AgencyID:  e.AgencyID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if v, ok := e.Properties["name"].(string); ok {
		p.Name = v
	}
	if v, ok := e.Properties["description"].(string); ok {
		p.Description = v
	}
	if v, ok := e.Properties["githubRepo"].(string); ok {
		p.GithubRepo = v
	}
	if v, ok := e.Properties["dueAt"].(string); ok && v != "" {
		if ts, err := time.Parse(time.RFC3339Nano, v); err == nil {
			p.DueAt = &ts
		}
	}
	return p
}

// CreateProject creates a new Project vertex in the agency graph.
// Returns [ErrInvalidTask] when Name is empty (re-using the Task error for
// "missing required fields" — consistent across the package), and
// [ErrProjectAlreadyExists] if the underlying store reports a duplicate.
func (m *taskManager) CreateProject(ctx context.Context, agencyID string, p Project) (Project, error) {
	if p.Name == "" {
		return Project{}, fmt.Errorf("%w: Project.Name is required", ErrInvalidTask)
	}
	p.AgencyID = agencyID
	created, err := m.dm.CreateEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   agencyID,
		TypeID:     projectTypeID,
		Properties: projectToProperties(p),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityAlreadyExists) {
			return Project{}, ErrProjectAlreadyExists
		}
		return Project{}, fmt.Errorf("CreateProject: %w", err)
	}
	return projectFromEntity(created), nil
}

// GetProject reads a single Project by its entity ID. Returns
// [ErrProjectNotFound] when the entity does not exist or is not a
// Project vertex.
func (m *taskManager) GetProject(ctx context.Context, agencyID, projectID string) (Project, error) {
	e, err := m.dm.GetEntity(ctx, agencyID, projectID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("GetProject: %w", err)
	}
	if e.AgencyID != agencyID || e.TypeID != projectTypeID {
		return Project{}, ErrProjectNotFound
	}
	return projectFromEntity(e), nil
}

// UpdateProject patches the mutable fields (Name, Description, GithubRepo,
// DueAt) of an existing Project. Returns [ErrProjectNotFound] if the project
// does not exist.
func (m *taskManager) UpdateProject(ctx context.Context, agencyID string, p Project) (Project, error) {
	current, err := m.GetProject(ctx, agencyID, p.ID)
	if err != nil {
		return Project{}, err
	}
	if p.Name == "" {
		return Project{}, fmt.Errorf("%w: Project.Name is required", ErrInvalidTask)
	}
	p.AgencyID = agencyID
	p.CreatedAt = current.CreatedAt

	updated, err := m.dm.UpdateEntity(ctx, agencyID, p.ID, entitygraph.UpdateEntityRequest{
		Properties: projectToProperties(p),
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, fmt.Errorf("UpdateProject: %w", err)
	}
	return projectFromEntity(updated), nil
}

// DeleteProject soft-deletes the Project vertex AND hard-deletes every
// inbound `member_of` edge. Member Tasks are not affected — they remain in
// the agency, just no longer attached to this project.
//
// Returns [ErrProjectNotFound] if the project does not exist.
func (m *taskManager) DeleteProject(ctx context.Context, agencyID, projectID string) error {
	if _, err := m.GetProject(ctx, agencyID, projectID); err != nil {
		return err
	}
	edges, err := m.TraverseRelationships(ctx, agencyID, projectID, RelLabelMemberOf, DirectionInbound)
	if err != nil {
		return fmt.Errorf("DeleteProject: traverse: %w", err)
	}
	for _, e := range edges {
		if err := m.dm.DeleteRelationship(ctx, agencyID, e.ID); err != nil {
			if errors.Is(err, entitygraph.ErrRelationshipNotFound) {
				continue
			}
			return fmt.Errorf("DeleteProject: delete edge %s: %w", e.ID, err)
		}
	}
	if err := m.dm.DeleteEntity(ctx, agencyID, projectID); err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return ErrProjectNotFound
		}
		return fmt.Errorf("DeleteProject: %w", err)
	}
	return nil
}

// ListProjects returns all non-deleted Projects in the agency.
func (m *taskManager) ListProjects(ctx context.Context, agencyID string) ([]Project, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: agencyID,
		TypeID:   projectTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("ListProjects: %w", err)
	}
	out := make([]Project, 0, len(entities))
	for _, e := range entities {
		out = append(out, projectFromEntity(e))
	}
	return out, nil
}

// AddTaskToProject creates the `member_of` edge from taskID to projectID.
// Idempotent — re-adding an existing membership returns nil. Returns
// [ErrTaskNotFound] / [ErrProjectNotFound] when the respective vertex is
// missing.
func (m *taskManager) AddTaskToProject(ctx context.Context, agencyID, taskID, projectID string) error {
	_, err := m.CreateRelationship(ctx, agencyID, Relationship{
		Label:  RelLabelMemberOf,
		FromID: taskID,
		ToID:   projectID,
		Properties: map[string]any{
			"addedAt": time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return fmt.Errorf("AddTaskToProject: %w", err)
	}
	return nil
}

// RemoveTaskFromProject removes the `member_of` edge from taskID to projectID.
// Returns [ErrRelationshipNotFound] if no membership existed.
func (m *taskManager) RemoveTaskFromProject(ctx context.Context, agencyID, taskID, projectID string) error {
	return m.DeleteRelationship(ctx, agencyID, taskID, projectID, RelLabelMemberOf)
}

// ListTasksInProject returns the Tasks that are members of the given project
// via inbound `member_of` edges. Tasks the caller has soft-deleted are
// skipped (they're not returned).
func (m *taskManager) ListTasksInProject(ctx context.Context, agencyID, projectID string) ([]Task, error) {
	edges, err := m.TraverseRelationships(ctx, agencyID, projectID, RelLabelMemberOf, DirectionInbound)
	if err != nil {
		return nil, fmt.Errorf("ListTasksInProject: traverse: %w", err)
	}
	out := make([]Task, 0, len(edges))
	for _, e := range edges {
		t, err := m.GetTask(ctx, agencyID, e.FromID)
		if err != nil {
			if errors.Is(err, ErrTaskNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListTasksInProject: get %s: %w", e.FromID, err)
		}
		out = append(out, t)
	}
	return out, nil
}

// ListProjectsForTask returns the Projects the given Task belongs to via
// outbound `member_of` edges.
func (m *taskManager) ListProjectsForTask(ctx context.Context, agencyID, taskID string) ([]Project, error) {
	edges, err := m.TraverseRelationships(ctx, agencyID, taskID, RelLabelMemberOf, DirectionOutbound)
	if err != nil {
		return nil, fmt.Errorf("ListProjectsForTask: traverse: %w", err)
	}
	out := make([]Project, 0, len(edges))
	for _, e := range edges {
		p, err := m.GetProject(ctx, agencyID, e.ToID)
		if err != nil {
			if errors.Is(err, ErrProjectNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListProjectsForTask: get %s: %w", e.ToID, err)
		}
		out = append(out, p)
	}
	return out, nil
}
