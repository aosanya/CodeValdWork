// task_impl_converters.go — entity↔domain converters.
//
// Property helpers (StringProp, Float64Prop, …) live in
// [github.com/aosanya/CodeValdSharedLib/entitygraph] and are used directly.
package codevaldwork

import "github.com/aosanya/CodeValdSharedLib/entitygraph"

// ── Task ──────────────────────────────────────────────────────────────────────

// taskToProperties serialises a Task into the property map stored on its
// entitygraph Entity. All fields — including created_at and updated_at — are
// written as explicit schema properties (ISO 8601 / RFC 3339 strings).
func taskToProperties(t Task) map[string]any {
	props := map[string]any{
		"title":              t.Title,
		"description":        t.Description,
		"status":             string(t.Status),
		"priority":           string(t.Priority),
		"context":            t.Context,
		"task_name":          t.TaskName,
		"project_name":       t.ProjectName,
		"separate_branch":    t.SeparateBranch,
		"branch_name":        t.BranchName,
		"workflow_run_id":    t.WorkflowRunID,
		"recovery_runs_used": t.RecoveryRunsUsed,
		"blocker_note":       t.BlockerNote,
		"direction_history":  t.DirectionHistory,
		"parent_task_id":     t.ParentTaskID,
		"created_at":         t.CreatedAt,
		"updated_at":         t.UpdatedAt,
	}
	if t.DueAt != "" {
		props["due_at"] = t.DueAt
	}
	if len(t.Tags) > 0 {
		tags := make([]string, len(t.Tags))
		copy(tags, t.Tags)
		props["tags"] = tags
	}
	if t.EstimatedHours != 0 {
		props["estimated_hours"] = t.EstimatedHours
	}
	if t.CompletedAt != "" {
		props["completed_at"] = t.CompletedAt
	}
	return props
}

// taskFromEntity reconstructs a Task from an entitygraph Entity.
// All timestamps are read from e.Properties (schema properties), not from the
// entity's native CreatedAt/UpdatedAt fields.
// Tags accept both the native Go type (used by the unit fakeDataManager) and
// the JSON-decoded form ([]any) the ArangoDB backend returns.
func taskFromEntity(e entitygraph.Entity) Task {
	t := Task{
		ID:               e.ID,
		AgencyID:         e.AgencyID,
		Title:            entitygraph.StringProp(e.Properties, "title"),
		Description:      entitygraph.StringProp(e.Properties, "description"),
		Status:           TaskStatus(entitygraph.StringProp(e.Properties, "status")),
		Priority:         TaskPriority(entitygraph.StringProp(e.Properties, "priority")),
		Context:          entitygraph.StringProp(e.Properties, "context"),
		DueAt:            entitygraph.StringProp(e.Properties, "due_at"),
		CompletedAt:      entitygraph.StringProp(e.Properties, "completed_at"),
		TaskName:         entitygraph.StringProp(e.Properties, "task_name"),
		ProjectName:      entitygraph.StringProp(e.Properties, "project_name"),
		SeparateBranch:   entitygraph.BoolProp(e.Properties, "separate_branch"),
		BranchName:       entitygraph.StringProp(e.Properties, "branch_name"),
		WorkflowRunID:    entitygraph.StringProp(e.Properties, "workflow_run_id"),
		RecoveryRunsUsed: int(entitygraph.Int64Prop(e.Properties, "recovery_runs_used")),
		BlockerNote:      entitygraph.StringProp(e.Properties, "blocker_note"),
		DirectionHistory: entitygraph.StringProp(e.Properties, "direction_history"),
		ParentTaskID:     entitygraph.StringProp(e.Properties, "parent_task_id"),
		CreatedAt:        entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:        entitygraph.StringProp(e.Properties, "updated_at"),
		EstimatedHours:   entitygraph.Float64Prop(e.Properties, "estimated_hours"),
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
	return t
}

// ── TaskTodo ──────────────────────────────────────────────────────────────────

func taskTodoToProperties(t TaskTodo) map[string]any {
	props := map[string]any{
		"title":            t.Title,
		"description":      t.Description,
		"instructions":     t.Instructions,
		"ordinality":       t.Ordinality,
		"can_run_parallel": t.CanRunParallel,
		"status":           string(t.Status),
		"parent_task_id":   t.ParentTaskID,
		"decomp_run_id":    t.DecompRunID,
		"agent_id":         t.AgentID,
		"precalls":         t.Precalls,
		"workflow_run_id":  t.WorkflowRunID,
		"created_at":       t.CreatedAt,
		"updated_at":       t.UpdatedAt,
	}
	if len(t.DependsOn) > 0 {
		deps := make([]int, len(t.DependsOn))
		copy(deps, t.DependsOn)
		props["depends_on"] = deps
	}
	return props
}

func taskTodoFromEntity(e entitygraph.Entity) TaskTodo {
	t := TaskTodo{
		ID:             e.ID,
		AgencyID:       e.AgencyID,
		Title:          entitygraph.StringProp(e.Properties, "title"),
		Description:    entitygraph.StringProp(e.Properties, "description"),
		Instructions:   entitygraph.StringProp(e.Properties, "instructions"),
		Ordinality:     int(entitygraph.Float64Prop(e.Properties, "ordinality")),
		CanRunParallel: entitygraph.BoolProp(e.Properties, "can_run_parallel"),
		Status:         TodoStatus(entitygraph.StringProp(e.Properties, "status")),
		ParentTaskID:   entitygraph.StringProp(e.Properties, "parent_task_id"),
		DecompRunID:    entitygraph.StringProp(e.Properties, "decomp_run_id"),
		AgentID:        entitygraph.StringProp(e.Properties, "agent_id"),
		Precalls:       entitygraph.StringProp(e.Properties, "precalls"),
		WorkflowRunID:  entitygraph.StringProp(e.Properties, "workflow_run_id"),
		CreatedAt:      entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:      entitygraph.StringProp(e.Properties, "updated_at"),
	}
	if v, ok := e.Properties["depends_on"]; ok {
		switch deps := v.(type) {
		case []int:
			t.DependsOn = append([]int(nil), deps...)
		case []any:
			out := make([]int, 0, len(deps))
			for _, x := range deps {
				switch n := x.(type) {
				case float64:
					out = append(out, int(n))
				case int:
					out = append(out, n)
				}
			}
			t.DependsOn = out
		}
	}
	return t
}

// ── Tag ───────────────────────────────────────────────────────────────────────

// tagToProperties serialises a Tag into the property map stored on its
// entitygraph Entity.
func tagToProperties(t Tag) map[string]any {
	return map[string]any{
		"name":        t.Name,
		"color":       t.Color,
		"description": t.Description,
		"created_at":  t.CreatedAt,
		"updated_at":  t.UpdatedAt,
	}
}

// tagFromEntity reconstructs a Tag from an entitygraph Entity.
func tagFromEntity(e entitygraph.Entity) Tag {
	return Tag{
		ID:          e.ID,
		AgencyID:    e.AgencyID,
		Name:        entitygraph.StringProp(e.Properties, "name"),
		Color:       entitygraph.StringProp(e.Properties, "color"),
		Description: entitygraph.StringProp(e.Properties, "description"),
		CreatedAt:   entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:   entitygraph.StringProp(e.Properties, "updated_at"),
	}
}

// ── Agent ─────────────────────────────────────────────────────────────────────

// agentToProperties serialises an Agent into the property map stored on its
// entitygraph Entity.
func agentToProperties(a Agent) map[string]any {
	return map[string]any{
		"agent_id":     a.AgentID,
		"display_name": a.DisplayName,
		"capability":   a.Capability,
		"role_name":    a.RoleName,
		"created_at":   a.CreatedAt,
		"updated_at":   a.UpdatedAt,
	}
}

// agentFromEntity reconstructs an Agent from an entitygraph Entity.
func agentFromEntity(e entitygraph.Entity) Agent {
	return Agent{
		ID:          e.ID,
		AgencyID:    e.AgencyID,
		AgentID:     entitygraph.StringProp(e.Properties, "agent_id"),
		DisplayName: entitygraph.StringProp(e.Properties, "display_name"),
		Capability:  entitygraph.StringProp(e.Properties, "capability"),
		RoleName:    entitygraph.StringProp(e.Properties, "role_name"),
		CreatedAt:   entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:   entitygraph.StringProp(e.Properties, "updated_at"),
	}
}

// ── Project ───────────────────────────────────────────────────────────────────

// projectToProperties serialises a Project into the property map stored on its
// entitygraph Entity.
func projectToProperties(p Project) map[string]any {
	return map[string]any{
		"name":         p.Name,
		"project_name": p.ProjectName,
		"description":  p.Description,
		"repo_name":    p.RepoName,
		"github_repo":  p.GithubRepo,
		"task_prefix":  p.TaskPrefix,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
	}
}

// ── Deliverable ───────────────────────────────────────────────────────────────

func deliverableToProperties(d Deliverable) map[string]any {
	return map[string]any{
		"title":            d.Title,
		"description":      d.Description,
		"deliverable_type": d.DeliverableType,
		"parent_id":        d.ParentID,
		"ordinality":       d.Ordinality,
		"workflow_run_id":  d.WorkflowRunID,
		"created_at":       d.CreatedAt,
		"updated_at":       d.UpdatedAt,
	}
}

func deliverableFromEntity(e entitygraph.Entity) Deliverable {
	return Deliverable{
		ID:              e.ID,
		AgencyID:        e.AgencyID,
		Title:           entitygraph.StringProp(e.Properties, "title"),
		Description:     entitygraph.StringProp(e.Properties, "description"),
		DeliverableType: entitygraph.StringProp(e.Properties, "deliverable_type"),
		ParentID:        entitygraph.StringProp(e.Properties, "parent_id"),
		Ordinality:      int(entitygraph.Int64Prop(e.Properties, "ordinality")),
		WorkflowRunID:   entitygraph.StringProp(e.Properties, "workflow_run_id"),
		CreatedAt:       entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:       entitygraph.StringProp(e.Properties, "updated_at"),
	}
}

// ── AcceptanceCriteria ────────────────────────────────────────────────────────

func acceptanceCriteriaToProperties(a AcceptanceCriteria) map[string]any {
	return map[string]any{
		"title":           a.Title,
		"description":     a.Description,
		"parent_id":       a.ParentID,
		"ordinality":      a.Ordinality,
		"workflow_run_id": a.WorkflowRunID,
		"result":          a.Result,
		"result_notes":    a.ResultNotes,
		"created_at":      a.CreatedAt,
		"updated_at":      a.UpdatedAt,
	}
}

func acceptanceCriteriaFromEntity(e entitygraph.Entity) AcceptanceCriteria {
	return AcceptanceCriteria{
		ID:            e.ID,
		AgencyID:      e.AgencyID,
		Title:         entitygraph.StringProp(e.Properties, "title"),
		Description:   entitygraph.StringProp(e.Properties, "description"),
		ParentID:      entitygraph.StringProp(e.Properties, "parent_id"),
		Ordinality:    int(entitygraph.Int64Prop(e.Properties, "ordinality")),
		WorkflowRunID: entitygraph.StringProp(e.Properties, "workflow_run_id"),
		Result:        entitygraph.StringProp(e.Properties, "result"),
		ResultNotes:   entitygraph.StringProp(e.Properties, "result_notes"),
		CreatedAt:     entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:     entitygraph.StringProp(e.Properties, "updated_at"),
	}
}

// projectFromEntity reconstructs a Project from an entitygraph Entity.
func projectFromEntity(e entitygraph.Entity) Project {
	p := Project{
		ID:          e.ID,
		AgencyID:    e.AgencyID,
		Name:        entitygraph.StringProp(e.Properties, "name"),
		ProjectName: entitygraph.StringProp(e.Properties, "project_name"),
		Description: entitygraph.StringProp(e.Properties, "description"),
		RepoName:    entitygraph.StringProp(e.Properties, "repo_name"),
		GithubRepo:  entitygraph.StringProp(e.Properties, "github_repo"),
		TaskPrefix:  entitygraph.StringProp(e.Properties, "task_prefix"),
		CreatedAt:   entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:   entitygraph.StringProp(e.Properties, "updated_at"),
	}
	// Backfill slug for entities created before project_name was added.
	if p.ProjectName == "" && p.Name != "" {
		p.ProjectName = toSlug(p.Name)
	}
	return p
}

