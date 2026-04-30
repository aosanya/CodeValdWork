package codevaldwork

import (
	"context"
	"errors"
	"fmt"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// UpsertAgent creates or merges an Agent vertex keyed by (agencyID, agent_id).
//
// On the merge branch, display_name and capability are updated to the request
// values; agent_id is treated as immutable (the natural key cannot change).
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
