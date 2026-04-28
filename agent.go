package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// Agent is the Work-domain projection of an AI agent. Each Agent becomes a
// graph vertex so that `assigned_to` edges are first-class graph relationships
// rather than string fields on the Task document.
//
// Uniqueness — at most one Agent per (AgencyID, AgentID) — is enforced at the
// schema level via [taskTypeDefinition]'s sibling agentTypeDefinition declaring
// UniqueKey: ["agentID"]. [TaskManager.UpsertAgent] relies on this for the
// find-or-create semantics.
type Agent struct {
	// ID is the entity-graph storage key — opaque to callers.
	ID string

	// AgencyID is the agency this agent serves.
	AgencyID string

	// AgentID is the external agent identifier (e.g. a CodeValdAI agent ID).
	// Required and unique within an agency.
	AgentID string

	// DisplayName is a human-readable label for the agent. Optional.
	DisplayName string

	// Capability is the agent's primary capability (e.g. "code", "research",
	// "review"). Optional.
	Capability string

	// CreatedAt is the UTC timestamp when the agent was first registered.
	CreatedAt time.Time

	// UpdatedAt is the UTC timestamp of the most recent upsert.
	UpdatedAt time.Time
}

// agentToProperties serialises an Agent into the property map stored on its
// entitygraph Entity.
func agentToProperties(a Agent) map[string]any {
	return map[string]any{
		"agentID":     a.AgentID,
		"displayName": a.DisplayName,
		"capability":  a.Capability,
	}
}

// agentFromEntity reconstructs an Agent from an entitygraph Entity.
func agentFromEntity(e entitygraph.Entity) Agent {
	a := Agent{
		ID:        e.ID,
		AgencyID:  e.AgencyID,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
	if v, ok := e.Properties["agentID"].(string); ok {
		a.AgentID = v
	}
	if v, ok := e.Properties["displayName"].(string); ok {
		a.DisplayName = v
	}
	if v, ok := e.Properties["capability"].(string); ok {
		a.Capability = v
	}
	return a
}

// UpsertAgent creates or merges an Agent vertex keyed by (agencyID, agentID).
//
// On the merge branch, displayName and capability are updated to the request
// values; agentID is treated as immutable (the natural key cannot change).
// CreatedAt is preserved from the original; UpdatedAt is bumped.
func (m *taskManager) UpsertAgent(ctx context.Context, agencyID string, agent Agent) (Agent, error) {
	if agent.AgentID == "" {
		return Agent{}, fmt.Errorf("%w: AgentID is required", ErrInvalidTask)
	}
	agent.AgencyID = agencyID

	upserted, err := m.dm.UpsertEntity(ctx, entitygraph.CreateEntityRequest{
		AgencyID:   agencyID,
		TypeID:     agentTypeID,
		Properties: agentToProperties(agent),
	})
	if err != nil {
		return Agent{}, fmt.Errorf("UpsertAgent: %w", err)
	}
	return agentFromEntity(upserted), nil
}

// GetAgent reads an Agent vertex by its entity ID (the storage key, not the
// external AgentID). Returns [ErrAgentNotFound] if the entity does not exist
// or is not an Agent.
func (m *taskManager) GetAgent(ctx context.Context, agencyID, entityID string) (Agent, error) {
	e, err := m.dm.GetEntity(ctx, agencyID, entityID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return Agent{}, ErrAgentNotFound
		}
		return Agent{}, fmt.Errorf("GetAgent: %w", err)
	}
	if e.AgencyID != agencyID || e.TypeID != agentTypeID {
		return Agent{}, ErrAgentNotFound
	}
	return agentFromEntity(e), nil
}

// ListAgents returns all non-deleted Agents in the agency.
func (m *taskManager) ListAgents(ctx context.Context, agencyID string) ([]Agent, error) {
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID: agencyID,
		TypeID:   agentTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("ListAgents: %w", err)
	}
	out := make([]Agent, 0, len(entities))
	for _, e := range entities {
		out = append(out, agentFromEntity(e))
	}
	return out, nil
}
