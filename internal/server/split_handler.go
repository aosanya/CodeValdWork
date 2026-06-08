package server

import (
	"context"
	"encoding/json"
	"log"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// taskPlanSplitPayload mirrors codevaldwork.TaskPlanSplitPayload for JSON decoding.
type taskPlanSplitPayload struct {
	TaskID        string                            `json:"task_id"`
	WorkflowRunID string                            `json:"workflow_run_id,omitempty"`
	Children      []codevaldwork.TaskPlanSplitChildSpec `json:"children"`
}

// handleTaskPlanSplit processes a ai.task.split event:
//  1. Creates each child Task entity with parent_task_id set.
//  2. Writes a subtask_of edge from each child → parent.
//  3. Writes blocks edges between children that have DependsOn entries.
//  4. Transitions the parent task in_progress → split.
//  5. Dispatches (publishes work.task.assigned) for children with no unmet dependencies.
func (d *TaskEventDispatcher) handleTaskPlanSplit(ctx context.Context, payload string) {
	var p taskPlanSplitPayload
	if err := json.Unmarshal([]byte(payload), &p); err != nil || p.TaskID == "" {
		log.Printf("codevaldwork: handleTaskPlanSplit: bad payload: %v", err)
		return
	}
	if len(p.Children) == 0 {
		log.Printf("codevaldwork: handleTaskPlanSplit: task=%s has zero children — ignoring", p.TaskID)
		return
	}

	parent, err := d.mgr.GetTask(ctx, d.agencyID, p.TaskID)
	if err != nil {
		log.Printf("codevaldwork: handleTaskPlanSplit: GetTask %s: %v", p.TaskID, err)
		return
	}

	// ── 1. Create child tasks ─────────────────────────────────────────────────
	tempToTask := make(map[string]codevaldwork.Task, len(p.Children))
	for _, spec := range p.Children {
		child := codevaldwork.Task{
			Title:         spec.Title,
			Description:   spec.Description,
			Priority:      parent.Priority,
			WorkflowRunID: p.WorkflowRunID,
			TaskName:      spec.TaskName,
			ParentTaskID:  p.TaskID,
		}
		created, cErr := d.mgr.CreateTask(ctx, d.agencyID, child)
		if cErr != nil {
			log.Printf("codevaldwork: handleTaskPlanSplit: CreateTask %q: %v", spec.TempID, cErr)
			return
		}
		tempToTask[spec.TempID] = created
		log.Printf("codevaldwork: handleTaskPlanSplit: created child task=%s (temp=%s)", created.ID, spec.TempID)

		// Write subtask_of edge: child → parent.
		_, relErr := d.mgr.CreateRelationship(ctx, d.agencyID, codevaldwork.Relationship{
			FromID: created.ID,
			ToID:   p.TaskID,
			Label:  codevaldwork.RelLabelSubtaskOf,
		})
		if relErr != nil {
			log.Printf("codevaldwork: handleTaskPlanSplit: subtask_of edge child=%s parent=%s: %v", created.ID, p.TaskID, relErr)
		}
	}

	// ── 2. Write dependency (blocks) edges ────────────────────────────────────
	for _, spec := range p.Children {
		childTask := tempToTask[spec.TempID]
		for _, depTempID := range spec.DependsOn {
			depTask, ok := tempToTask[depTempID]
			if !ok {
				log.Printf("codevaldwork: handleTaskPlanSplit: unknown depends_on temp_id %q for child %q", depTempID, spec.TempID)
				continue
			}
			// depTask blocks childTask
			_, relErr := d.mgr.CreateRelationship(ctx, d.agencyID, codevaldwork.Relationship{
				FromID: depTask.ID,
				ToID:   childTask.ID,
				Label:  codevaldwork.RelLabelBlocks,
			})
			if relErr != nil {
				log.Printf("codevaldwork: handleTaskPlanSplit: blocks edge %s→%s: %v", depTask.ID, childTask.ID, relErr)
			}
		}
	}

	// ── 3. Transition parent → split ─────────────────────────────────────────
	if !parent.Status.CanTransitionTo(codevaldwork.TaskStatusSplit) {
		log.Printf("codevaldwork: handleTaskPlanSplit: parent task=%s status=%s cannot transition to split — skipping", p.TaskID, parent.Status)
	} else {
		parent.Status = codevaldwork.TaskStatusSplit
		parent.UpdatedAt = nowRFC3339()
		if _, uErr := d.mgr.UpdateTask(ctx, d.agencyID, parent); uErr != nil {
			log.Printf("codevaldwork: handleTaskPlanSplit: UpdateTask parent=%s: %v", p.TaskID, uErr)
		} else {
			log.Printf("codevaldwork: handleTaskPlanSplit: parent task=%s → split", p.TaskID)
		}
	}

	// ── 4. Dispatch independent children ─────────────────────────────────────
	for _, spec := range p.Children {
		if len(spec.DependsOn) > 0 {
			continue // blocked until siblings complete
		}
		childTask := tempToTask[spec.TempID]
		eventbus.SafePublish(ctx, d.pub, eventbus.Event{
			Topic:    codevaldwork.TopicTaskAssigned,
			AgencyID: d.agencyID,
			Payload: codevaldwork.TaskAssignedPayload{
				TaskID:        childTask.ID,
				RoleName:      spec.RoleName,
				TaskCode:      childTask.TaskName,
				Title:         childTask.Title,
				Description:   childTask.Description,
				WorkflowRunID: childTask.WorkflowRunID,
			},
		})
		log.Printf("codevaldwork: handleTaskPlanSplit: dispatched child task=%s", childTask.ID)
	}
}

// maybeCompleteSplitParent marks a SPLIT parent task completed or failed once
// all of its child tasks (linked via subtask_of edges) are terminal.
//
// Called whenever a child task reaches a terminal status (completed, failed, cancelled).
func (d *TaskEventDispatcher) maybeCompleteSplitParent(ctx context.Context, parentTaskID string) {
	edges, err := d.mgr.TraverseRelationships(ctx, d.agencyID, parentTaskID, codevaldwork.RelLabelSubtaskOf, codevaldwork.DirectionInbound)
	if err != nil {
		log.Printf("codevaldwork: maybeCompleteSplitParent: TraverseRelationships task=%s: %v", parentTaskID, err)
		return
	}
	if len(edges) == 0 {
		return
	}

	anyFailed := false
	for _, edge := range edges {
		child, cErr := d.mgr.GetTask(ctx, d.agencyID, edge.FromID)
		if cErr != nil {
			log.Printf("codevaldwork: maybeCompleteSplitParent: GetTask child=%s: %v", edge.FromID, cErr)
			return
		}
		switch child.Status {
		case codevaldwork.TaskStatusCompleted:
			// non-failing terminal — continue
		case codevaldwork.TaskStatusFailed, codevaldwork.TaskStatusCancelled:
			anyFailed = true
		default:
			return // pending, in_progress, split, blocked — not yet terminal
		}
	}

	parent, err := d.mgr.GetTask(ctx, d.agencyID, parentTaskID)
	if err != nil {
		log.Printf("codevaldwork: maybeCompleteSplitParent: GetTask parent=%s: %v", parentTaskID, err)
		return
	}
	if parent.Status == codevaldwork.TaskStatusCompleted || parent.Status == codevaldwork.TaskStatusFailed {
		return
	}
	if anyFailed {
		parent.Status = codevaldwork.TaskStatusFailed
	} else {
		parent.Status = codevaldwork.TaskStatusCompleted
	}
	parent.UpdatedAt = nowRFC3339()
	if _, uErr := d.mgr.UpdateTask(ctx, d.agencyID, parent); uErr != nil {
		log.Printf("codevaldwork: maybeCompleteSplitParent: UpdateTask parent=%s: %v", parentTaskID, uErr)
		return
	}
	log.Printf("codevaldwork: maybeCompleteSplitParent: parent task=%s → %s (all children terminal)", parentTaskID, parent.Status)
}
