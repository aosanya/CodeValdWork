package codevaldwork

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// ListDeliverablesForTask returns all Deliverable entities linked to taskID
// via outbound has_deliverable edges.
func (m *taskManager) ListDeliverablesForTask(ctx context.Context, agencyID, taskID string) ([]Deliverable, error) {
	res, err := m.dm.TraverseGraph(ctx, entitygraph.TraverseGraphRequest{
		AgencyID:  agencyID,
		StartID:   taskID,
		Direction: DirectionOutbound.String(),
		Depth:     1,
		Names:     []string{RelLabelHasDeliverable},
	})
	if err != nil {
		return nil, fmt.Errorf("ListDeliverablesForTask: %w", err)
	}

	out := make([]Deliverable, 0, len(res.Edges))
	for _, edge := range res.Edges {
		e, err := m.dm.GetEntity(ctx, agencyID, edge.ToID)
		if err != nil {
			if errors.Is(err, entitygraph.ErrEntityNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListDeliverablesForTask: fetch %s: %w", edge.ToID, err)
		}
		out = append(out, deliverableFromEntity(e))
	}
	return out, nil
}

// ListAcceptanceCriteriaForTask returns all AcceptanceCriteria entities linked
// to taskID via outbound has_acceptance_criteria edges.
func (m *taskManager) ListAcceptanceCriteriaForTask(ctx context.Context, agencyID, taskID string) ([]AcceptanceCriteria, error) {
	res, err := m.dm.TraverseGraph(ctx, entitygraph.TraverseGraphRequest{
		AgencyID:  agencyID,
		StartID:   taskID,
		Direction: DirectionOutbound.String(),
		Depth:     1,
		Names:     []string{RelLabelHasAcceptanceCriteria},
	})
	if err != nil {
		return nil, fmt.Errorf("ListAcceptanceCriteriaForTask: %w", err)
	}

	out := make([]AcceptanceCriteria, 0, len(res.Edges))
	for _, edge := range res.Edges {
		e, err := m.dm.GetEntity(ctx, agencyID, edge.ToID)
		if err != nil {
			if errors.Is(err, entitygraph.ErrEntityNotFound) {
				continue
			}
			return nil, fmt.Errorf("ListAcceptanceCriteriaForTask: fetch %s: %w", edge.ToID, err)
		}
		out = append(out, acceptanceCriteriaFromEntity(e))
	}
	return out, nil
}

// WriteAcceptanceCriteriaResult writes the reviewer's result and result_notes
// onto an AcceptanceCriteria entity.
func (m *taskManager) WriteAcceptanceCriteriaResult(ctx context.Context, agencyID, criteriaID, result, notes string) error {
	e, err := m.dm.GetEntity(ctx, agencyID, criteriaID)
	if err != nil {
		if errors.Is(err, entitygraph.ErrEntityNotFound) {
			return ErrAcceptanceCriteriaNotFound
		}
		return fmt.Errorf("WriteAcceptanceCriteriaResult: %w", err)
	}

	props := acceptanceCriteriaToProperties(acceptanceCriteriaFromEntity(e))
	props["result"] = result
	props["result_notes"] = notes
	props["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	_, err = m.dm.UpdateEntity(ctx, agencyID, criteriaID, entitygraph.UpdateEntityRequest{
		Properties: props,
	})
	if err != nil {
		return fmt.Errorf("WriteAcceptanceCriteriaResult: update: %w", err)
	}
	return nil
}
