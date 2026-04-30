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
		"title":           t.Title,
		"description":     t.Description,
		"status":          string(t.Status),
		"priority":        string(t.Priority),
		"context":         t.Context,
		"task_name":       t.TaskName,
		"project_name":    t.ProjectName,
		"separate_branch": t.SeparateBranch,
		"branch_name":     t.BranchName,
		"created_at":      t.CreatedAt,
		"updated_at":      t.UpdatedAt,
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
		ID:             e.ID,
		AgencyID:       e.AgencyID,
		Title:          entitygraph.StringProp(e.Properties, "title"),
		Description:    entitygraph.StringProp(e.Properties, "description"),
		Status:         TaskStatus(entitygraph.StringProp(e.Properties, "status")),
		Priority:       TaskPriority(entitygraph.StringProp(e.Properties, "priority")),
		Context:        entitygraph.StringProp(e.Properties, "context"),
		DueAt:          entitygraph.StringProp(e.Properties, "due_at"),
		CompletedAt:    entitygraph.StringProp(e.Properties, "completed_at"),
		TaskName:       entitygraph.StringProp(e.Properties, "task_name"),
		ProjectName:    entitygraph.StringProp(e.Properties, "project_name"),
		SeparateBranch: entitygraph.BoolProp(e.Properties, "separate_branch"),
		BranchName:     entitygraph.StringProp(e.Properties, "branch_name"),
		CreatedAt:      entitygraph.StringProp(e.Properties, "created_at"),
		UpdatedAt:      entitygraph.StringProp(e.Properties, "updated_at"),
		EstimatedHours: entitygraph.Float64Prop(e.Properties, "estimated_hours"),
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
		"github_repo":  p.GithubRepo,
		"task_prefix":  p.TaskPrefix,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
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

