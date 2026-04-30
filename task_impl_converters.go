// task_impl_converters.go — entity↔domain converters and property helpers.
package codevaldwork

import "github.com/aosanya/CodeValdSharedLib/entitygraph"

// ── Task ──────────────────────────────────────────────────────────────────────

// taskToProperties serialises a Task into the property map stored on its
// entitygraph Entity. All fields — including created_at and updated_at — are
// written as explicit schema properties (ISO 8601 / RFC 3339 strings).
func taskToProperties(t Task) map[string]any {
	props := map[string]any{
		"description":  t.Description,
		"status":       string(t.Status),
		"priority":     string(t.Priority),
		"context":      t.Context,
		"task_name":    t.TaskName,
		"project_name": t.ProjectName,
		"created_at":   t.CreatedAt,
		"updated_at":   t.UpdatedAt,
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
		Description:    strProp(e.Properties, "description"),
		Status:         TaskStatus(strProp(e.Properties, "status")),
		Priority:       TaskPriority(strProp(e.Properties, "priority")),
		Context:        strProp(e.Properties, "context"),
		DueAt:          strProp(e.Properties, "due_at"),
		CompletedAt:    strProp(e.Properties, "completed_at"),
		TaskName:       strProp(e.Properties, "task_name"),
		ProjectName:    strProp(e.Properties, "project_name"),
		CreatedAt:      strProp(e.Properties, "created_at"),
		UpdatedAt:      strProp(e.Properties, "updated_at"),
		EstimatedHours: float64Prop(e.Properties, "estimated_hours"),
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
		AgentID:     strProp(e.Properties, "agent_id"),
		DisplayName: strProp(e.Properties, "display_name"),
		Capability:  strProp(e.Properties, "capability"),
		CreatedAt:   strProp(e.Properties, "created_at"),
		UpdatedAt:   strProp(e.Properties, "updated_at"),
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
		Name:        strProp(e.Properties, "name"),
		ProjectName: strProp(e.Properties, "project_name"),
		Description: strProp(e.Properties, "description"),
		GithubRepo:  strProp(e.Properties, "github_repo"),
		TaskPrefix:  strProp(e.Properties, "task_prefix"),
		CreatedAt:   strProp(e.Properties, "created_at"),
		UpdatedAt:   strProp(e.Properties, "updated_at"),
	}
	// Backfill slug for entities created before project_name was added.
	if p.ProjectName == "" && p.Name != "" {
		p.ProjectName = toSlug(p.Name)
	}
	return p
}

// ── Property helpers ──────────────────────────────────────────────────────────

// strProp returns the string value of key in props, or "" if absent or wrong type.
func strProp(props map[string]any, key string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// float64Prop returns the float64 value of key in props, or 0 if absent.
// Handles float64, float32, int, int64 (JSON decode / ArangoDB wire forms).
func float64Prop(props map[string]any, key string) float64 {
	if v, ok := props[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case float32:
			return float64(n)
		case int:
			return float64(n)
		case int64:
			return float64(n)
		}
	}
	return 0
}
