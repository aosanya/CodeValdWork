package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// Relationship is the Work-domain projection of an entitygraph edge between
// two Work vertices (Task / Project / Agent).
type Relationship struct {
	// ID is the storage-assigned edge identifier.
	ID string

	// AgencyID is the agency that owns both endpoints and the edge itself.
	AgencyID string

	// Label is the edge label — one of the RelLabel* constants.
	Label string

	// FromID is the source vertex entity ID.
	FromID string

	// ToID is the target vertex entity ID.
	ToID string

	// Properties are caller-supplied edge metadata.
	Properties map[string]any

	// CreatedAt is the RFC 3339 timestamp the edge was created.
	CreatedAt string
}

// Direction selects edge orientation for [TaskManager.TraverseRelationships].
type Direction int

const (
	// DirectionInbound returns edges pointing AT the start vertex.
	DirectionInbound Direction = iota

	// DirectionOutbound returns edges pointing AWAY from the start vertex.
	DirectionOutbound
)

// String returns the entitygraph traversal direction string for d.
func (d Direction) String() string {
	switch d {
	case DirectionInbound:
		return "inbound"
	case DirectionOutbound:
		return "outbound"
	default:
		return "outbound"
	}
}

// Edge-label constants — the closed set of allowed Work relationship labels.
const (
	// RelLabelAssignedTo connects a Task to the Agent currently responsible
	// for it (functional — at most one per Task).
	RelLabelAssignedTo = "assigned_to"

	// RelLabelBlocks indicates the source Task must reach a terminal status
	// before the target Task may transition to in_progress.
	RelLabelBlocks = "blocks"

	// RelLabelSubtaskOf marks the source Task as a child of the target Task
	// (functional — a subtask has at most one parent).
	RelLabelSubtaskOf = "subtask_of"

	// RelLabelDependsOn is a soft dependency — informational only, no status gate.
	RelLabelDependsOn = "depends_on"

	// RelLabelMemberOf links a Task to a Project. Project membership is many-to-many.
	RelLabelMemberOf = "member_of"
)

// relationshipFromEntitygraph adapts a SharedLib edge into the Work-domain
// Relationship type.
func relationshipFromEntitygraph(r entitygraph.Relationship) Relationship {
	props := r.Properties
	if props != nil {
		dup := make(map[string]any, len(props))
		for k, v := range props {
			dup[k] = v
		}
		props = dup
	}
	return Relationship{
		ID:         r.ID,
		AgencyID:   r.AgencyID,
		Label:      r.Name,
		FromID:     r.FromID,
		ToID:       r.ToID,
		Properties: props,
		CreatedAt:  r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// relationshipEndpointTypes maps each Work edge label to the (FromType, ToType)
// pair declared in the schema's Relationships whitelist.
var relationshipEndpointTypes = map[string]struct {
	fromType string
	toType   string
}{
	RelLabelAssignedTo: {fromType: taskTypeID, toType: agentTypeID},
	RelLabelBlocks:     {fromType: taskTypeID, toType: taskTypeID},
	RelLabelSubtaskOf:  {fromType: taskTypeID, toType: taskTypeID},
	RelLabelDependsOn:  {fromType: taskTypeID, toType: taskTypeID},
	RelLabelMemberOf:   {fromType: taskTypeID, toType: projectTypeID},
}

// notFoundForType returns the typed sentinel error for a vertex TypeID.
func notFoundForType(typeID string) error {
	switch typeID {
	case taskTypeID:
		return ErrTaskNotFound
	case agentTypeID:
		return ErrAgentNotFound
	case projectTypeID:
		return ErrProjectNotFound
	default:
		return entitygraph.ErrEntityNotFound
	}
}

// labelHasCreatedAt reports whether the schema's RelationshipDefinition for
// label declares a "created_at" property the manager should default-populate.
// member_of uses "added_at", assigned_to uses "assigned_at" — those labels
// return false (the caller supplies the timestamp on the Properties map).
func labelHasCreatedAt(label string) bool {
	switch label {
	case RelLabelBlocks, RelLabelSubtaskOf, RelLabelDependsOn:
		return true
	default:
		return false
	}
}

// CreateRelationship validates the (label, FromID, ToID) triple and creates
// the edge via the underlying DataManager. Re-creating an existing edge is
// idempotent — the existing edge is returned with no error.
func (m *taskManager) CreateRelationship(ctx context.Context, agencyID string, rel Relationship) (Relationship, error) {
	allowed, ok := relationshipEndpointTypes[rel.Label]
	if !ok {
		return Relationship{}, fmt.Errorf("%w: unknown label %q", ErrInvalidRelationship, rel.Label)
	}
	if rel.FromID == "" || rel.ToID == "" {
		return Relationship{}, fmt.Errorf("%w: FromID and ToID are required", ErrInvalidRelationship)
	}

	from, err := m.dm.GetEntity(ctx, agencyID, rel.FromID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Relationship{}, notFoundForType(allowed.fromType)
		}
		return Relationship{}, fmt.Errorf("CreateRelationship: get from: %w", err)
	}
	if from.AgencyID != agencyID || from.TypeID != allowed.fromType {
		return Relationship{}, fmt.Errorf("%w: from-vertex type %q does not match label %q", ErrInvalidRelationship, from.TypeID, rel.Label)
	}

	to, err := m.dm.GetEntity(ctx, agencyID, rel.ToID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Relationship{}, notFoundForType(allowed.toType)
		}
		return Relationship{}, fmt.Errorf("CreateRelationship: get to: %w", err)
	}
	if to.AgencyID != agencyID || to.TypeID != allowed.toType {
		return Relationship{}, fmt.Errorf("%w: to-vertex type %q does not match label %q", ErrInvalidRelationship, to.TypeID, rel.Label)
	}

	existing, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: agencyID,
		FromID:   rel.FromID,
		ToID:     rel.ToID,
		Name:     rel.Label,
	})
	if err != nil {
		return Relationship{}, fmt.Errorf("CreateRelationship: list: %w", err)
	}
	if len(existing) > 0 {
		return relationshipFromEntitygraph(existing[0]), nil
	}

	props := map[string]any{}
	for k, v := range rel.Properties {
		props[k] = v
	}
	if _, ok := props["created_at"]; !ok && labelHasCreatedAt(rel.Label) {
		props["created_at"] = time.Now().UTC().Format(time.RFC3339)
	}

	created, err := m.dm.CreateRelationship(ctx, entitygraph.CreateRelationshipRequest{
		AgencyID:   agencyID,
		Name:       rel.Label,
		FromID:     rel.FromID,
		ToID:       rel.ToID,
		Properties: props,
	})
	if err != nil {
		if errors.Is(err, entitygraph.ErrInvalidRelationship) {
			return Relationship{}, fmt.Errorf("%w: %v", ErrInvalidRelationship, err)
		}
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Relationship{}, notFoundForType(allowed.toType)
		}
		return Relationship{}, fmt.Errorf("CreateRelationship: %w", err)
	}

	out := relationshipFromEntitygraph(created)
	m.publish(ctx, TopicRelationshipCreated, agencyID, RelationshipCreatedPayload{
		FromID: out.FromID,
		ToID:   out.ToID,
		Label:  out.Label,
	})
	return out, nil
}

// DeleteRelationship removes the single edge identified by (fromID, toID, label).
func (m *taskManager) DeleteRelationship(ctx context.Context, agencyID, fromID, toID, label string) error {
	edges, err := m.dm.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: agencyID,
		FromID:   fromID,
		ToID:     toID,
		Name:     label,
	})
	if err != nil {
		return fmt.Errorf("DeleteRelationship: list: %w", err)
	}
	if len(edges) == 0 {
		return ErrRelationshipNotFound
	}
	if err := m.dm.DeleteRelationship(ctx, agencyID, edges[0].ID); err != nil {
		if errors.Is(err, entitygraph.ErrRelationshipNotFound) {
			return ErrRelationshipNotFound
		}
		return fmt.Errorf("DeleteRelationship: %w", err)
	}
	return nil
}

// TraverseRelationships returns the single-hop edges incident on vertexID
// matching label and direction.
func (m *taskManager) TraverseRelationships(ctx context.Context, agencyID, vertexID, label string, dir Direction) ([]Relationship, error) {
	res, err := m.dm.TraverseGraph(ctx, entitygraph.TraverseGraphRequest{
		AgencyID:  agencyID,
		StartID:   vertexID,
		Direction: dir.String(),
		Depth:     1,
		Names:     []string{label},
	})
	if err != nil {
		return nil, fmt.Errorf("TraverseRelationships: %w", err)
	}
	out := make([]Relationship, 0, len(res.Edges))
	for _, e := range res.Edges {
		out = append(out, relationshipFromEntitygraph(e))
	}
	return out, nil
}
