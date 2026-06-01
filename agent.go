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

// GetAgent reads an Agent vertex by either its entity ID (the storage UUID) or
// its external AgentID slug (e.g. "developer-01"). UUID lookup is tried first;
// on NotFound it falls back to a slug match against the agency's Agents. This
// mirrors UpsertAgent's slug-first semantics so HTTP routes that bind
// {agentId} can be passed either form without the caller knowing which.
// Returns [ErrAgentNotFound] if no match is found.
func (m *taskManager) GetAgent(ctx context.Context, agencyID, idOrSlug string) (Agent, error) {
	e, err := m.dm.GetEntity(ctx, agencyID, idOrSlug)
	if err == nil {
		if e.AgencyID != agencyID || e.TypeID != agentTypeID {
			return Agent{}, ErrAgentNotFound
		}
		return agentFromEntity(e), nil
	}
	if !errors.Is(err, entitygraph.ErrEntityNotFound) {
		return Agent{}, fmt.Errorf("GetAgent: %w", err)
	}
	// Fallback: treat the argument as the AgentID slug.
	return m.GetAgentByAgentID(ctx, agencyID, idOrSlug)
}

// GetAgentByAgentID reads an Agent vertex by its external AgentID slug
// (e.g. "developer-01") — the same field UpsertAgent uses as the natural key.
// Returns [ErrAgentNotFound] if no Agent in the agency has that slug.
func (m *taskManager) GetAgentByAgentID(ctx context.Context, agencyID, agentIDSlug string) (Agent, error) {
	if agentIDSlug == "" {
		return Agent{}, ErrAgentNotFound
	}
	entities, err := m.dm.ListEntities(ctx, entitygraph.EntityFilter{
		AgencyID:   agencyID,
		TypeID:     agentTypeID,
		Properties: map[string]any{"agent_id": agentIDSlug},
	})
	if err != nil {
		return Agent{}, fmt.Errorf("GetAgentByAgentID: %w", err)
	}
	if len(entities) == 0 {
		return Agent{}, ErrAgentNotFound
	}
	return agentFromEntity(entities[0]), nil
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
