package codevaldwork_test

import (
	"context"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/google/uuid"
)

// ── Fake DataManager ─────────────────────────────────────────────────────────

// fakeDataManager is an in-memory entitygraph.DataManager used for unit tests.
// Entity-side methods plus relationship CRUD and a single-hop TraverseGraph
// are implemented; UpsertEntity (Agent path, WORK-010) still panics.
type fakeDataManager struct {
	entities      map[string]entitygraph.Entity       // key: agencyID + "/" + entityID
	relationships map[string]entitygraph.Relationship // key: agencyID + "/" + relID
}

func newFakeDataManager() *fakeDataManager {
	return &fakeDataManager{
		entities:      make(map[string]entitygraph.Entity),
		relationships: make(map[string]entitygraph.Relationship),
	}
}

func (f *fakeDataManager) key(agencyID, entityID string) string {
	return agencyID + "/" + entityID
}

func (f *fakeDataManager) CreateEntity(_ context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	props := make(map[string]any, len(req.Properties))
	for k, v := range req.Properties {
		props[k] = v
	}
	e := entitygraph.Entity{
		ID:         id,
		AgencyID:   req.AgencyID,
		TypeID:     req.TypeID,
		Properties: props,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	f.entities[f.key(req.AgencyID, id)] = e
	return e, nil
}

func (f *fakeDataManager) GetEntity(_ context.Context, agencyID, entityID string) (entitygraph.Entity, error) {
	e, ok := f.entities[f.key(agencyID, entityID)]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	return e, nil
}

func (f *fakeDataManager) UpdateEntity(_ context.Context, agencyID, entityID string, req entitygraph.UpdateEntityRequest) (entitygraph.Entity, error) {
	k := f.key(agencyID, entityID)
	e, ok := f.entities[k]
	if !ok || e.Deleted {
		return entitygraph.Entity{}, entitygraph.ErrEntityNotFound
	}
	if e.Properties == nil {
		e.Properties = map[string]any{}
	}
	for k2, v := range req.Properties {
		e.Properties[k2] = v
	}
	e.UpdatedAt = time.Now().UTC()
	f.entities[k] = e
	return e, nil
}

func (f *fakeDataManager) DeleteEntity(_ context.Context, agencyID, entityID string) error {
	k := f.key(agencyID, entityID)
	e, ok := f.entities[k]
	if !ok || e.Deleted {
		return entitygraph.ErrEntityNotFound
	}
	now := time.Now().UTC()
	e.Deleted = true
	e.DeletedAt = &now
	f.entities[k] = e
	return nil
}

func (f *fakeDataManager) ListEntities(_ context.Context, filter entitygraph.EntityFilter) ([]entitygraph.Entity, error) {
	var out []entitygraph.Entity
	for _, e := range f.entities {
		if e.Deleted {
			continue
		}
		if filter.AgencyID != "" && e.AgencyID != filter.AgencyID {
			continue
		}
		if filter.TypeID != "" && e.TypeID != filter.TypeID {
			continue
		}
		match := true
		for k, want := range filter.Properties {
			got, ok := e.Properties[k]
			if !ok || got != want {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		out = append(out, e)
	}
	if out == nil {
		out = []entitygraph.Entity{}
	}
	return out, nil
}

// UpsertEntity in the fake matches a non-deleted entity by every property
// listed in the type's UniqueKey. The fake doesn't carry the schema, so
// callers pass UniqueKey-relevant property values via req.Properties and
// the fake key-matches against those — sufficient for unit tests where the
// caller knows which keys are unique.
func (f *fakeDataManager) UpsertEntity(ctx context.Context, req entitygraph.CreateEntityRequest) (entitygraph.Entity, error) {
	uniqueKey := uniqueKeyFor(req.TypeID)
	if len(uniqueKey) == 0 {
		return entitygraph.Entity{}, entitygraph.ErrUniqueKeyNotDefined
	}
	for _, e := range f.entities {
		if e.Deleted || e.AgencyID != req.AgencyID || e.TypeID != req.TypeID {
			continue
		}
		match := true
		for _, k := range uniqueKey {
			if e.Properties[k] != req.Properties[k] {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		// Merge: patch properties onto the existing entity.
		if e.Properties == nil {
			e.Properties = map[string]any{}
		}
		for k, v := range req.Properties {
			e.Properties[k] = v
		}
		e.UpdatedAt = time.Now().UTC()
		f.entities[f.key(req.AgencyID, e.ID)] = e
		return e, nil
	}
	// Insert path.
	return f.CreateEntity(ctx, req)
}

// uniqueKeyFor returns the property names that form the natural key for
// the given Work type. Mirrors the schema.go UniqueKey declarations.
func uniqueKeyFor(typeID string) []string {
	switch typeID {
	case "Agent":
		return []string{"agentID"}
	default:
		return nil
	}
}

func (f *fakeDataManager) CreateRelationship(_ context.Context, req entitygraph.CreateRelationshipRequest) (entitygraph.Relationship, error) {
	if _, ok := f.entities[f.key(req.AgencyID, req.FromID)]; !ok {
		return entitygraph.Relationship{}, entitygraph.ErrEntityNotFound
	}
	if _, ok := f.entities[f.key(req.AgencyID, req.ToID)]; !ok {
		return entitygraph.Relationship{}, entitygraph.ErrEntityNotFound
	}
	id := uuid.NewString()
	props := make(map[string]any, len(req.Properties))
	for k, v := range req.Properties {
		props[k] = v
	}
	r := entitygraph.Relationship{
		ID:         id,
		AgencyID:   req.AgencyID,
		Name:       req.Name,
		FromID:     req.FromID,
		ToID:       req.ToID,
		Properties: props,
		CreatedAt:  time.Now().UTC(),
	}
	f.relationships[f.key(req.AgencyID, id)] = r
	return r, nil
}

func (f *fakeDataManager) GetRelationship(_ context.Context, agencyID, relID string) (entitygraph.Relationship, error) {
	r, ok := f.relationships[f.key(agencyID, relID)]
	if !ok {
		return entitygraph.Relationship{}, entitygraph.ErrRelationshipNotFound
	}
	return r, nil
}

func (f *fakeDataManager) DeleteRelationship(_ context.Context, agencyID, relID string) error {
	k := f.key(agencyID, relID)
	if _, ok := f.relationships[k]; !ok {
		return entitygraph.ErrRelationshipNotFound
	}
	delete(f.relationships, k)
	return nil
}

func (f *fakeDataManager) ListRelationships(_ context.Context, filter entitygraph.RelationshipFilter) ([]entitygraph.Relationship, error) {
	out := make([]entitygraph.Relationship, 0)
	for _, r := range f.relationships {
		if filter.AgencyID != "" && r.AgencyID != filter.AgencyID {
			continue
		}
		if filter.FromID != "" && r.FromID != filter.FromID {
			continue
		}
		if filter.ToID != "" && r.ToID != filter.ToID {
			continue
		}
		if filter.Name != "" && r.Name != filter.Name {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

// TraverseGraph in the fake supports the single-hop traversal needed by
// TaskManager.TraverseRelationships: depth=1 with a single name filter.
// Direction values "inbound" / "outbound" select the orientation; "any"
// returns both. Visited vertices are not populated — only Edges, which is
// what TraverseRelationships consumes.
func (f *fakeDataManager) TraverseGraph(_ context.Context, req entitygraph.TraverseGraphRequest) (entitygraph.TraverseGraphResult, error) {
	res := entitygraph.TraverseGraphResult{Edges: []entitygraph.Relationship{}}
	allowedNames := map[string]struct{}{}
	for _, n := range req.Names {
		allowedNames[n] = struct{}{}
	}
	for _, r := range f.relationships {
		if r.AgencyID != req.AgencyID {
			continue
		}
		if len(allowedNames) > 0 {
			if _, ok := allowedNames[r.Name]; !ok {
				continue
			}
		}
		switch req.Direction {
		case "outbound":
			if r.FromID != req.StartID {
				continue
			}
		case "inbound":
			if r.ToID != req.StartID {
				continue
			}
		default: // "any" or empty
			if r.FromID != req.StartID && r.ToID != req.StartID {
				continue
			}
		}
		res.Edges = append(res.Edges, r)
	}
	return res, nil
}

// ── recordingPublisher ───────────────────────────────────────────────────────

type recordingPublisher struct {
	events []string // "topic|agencyID"
}

func (p *recordingPublisher) Publish(_ context.Context, topic, agencyID string) error {
	p.events = append(p.events, topic+"|"+agencyID)
	return nil
}
