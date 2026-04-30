package codevaldwork

import (
	"reflect"
	"testing"

	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// ── DefaultWorkSchema ────────────────────────────────────────────────────────

func TestDefaultWorkSchema_TypeNames(t *testing.T) {
	s := DefaultWorkSchema()
	if got, want := len(s.Types), 3; got != want {
		t.Fatalf("len(Types) = %d, want %d", got, want)
	}
	got := make(map[string]bool, len(s.Types))
	for _, td := range s.Types {
		got[td.Name] = true
	}
	for _, want := range []string{"Task", "Project", "Agent"} {
		if !got[want] {
			t.Errorf("missing TypeDefinition %q", want)
		}
	}
}

func TestDefaultWorkSchema_TaskPropertyTypes(t *testing.T) {
	td := findType(t, DefaultWorkSchema(), "Task")
	want := map[string]types.PropertyType{
		"description":     types.PropertyTypeString,
		"status":          types.PropertyTypeString,
		"priority":        types.PropertyTypeString,
		"due_at":          types.PropertyTypeString,
		"tags":            types.PropertyTypeArray,
		"estimated_hours": types.PropertyTypeNumber,
		"context":         types.PropertyTypeString,
		"completed_at":    types.PropertyTypeString,
		"task_name":       types.PropertyTypeString,
		"project_name":    types.PropertyTypeString,
		"created_at":      types.PropertyTypeString,
		"updated_at":      types.PropertyTypeString,
	}
	got := propTypes(td)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Task property types mismatch:\n got=%v\nwant=%v", got, want)
	}
	tagsProp := findProp(t, td, "tags")
	if tagsProp.ElementType != types.PropertyTypeString {
		t.Errorf("tags ElementType = %v, want %v", tagsProp.ElementType, types.PropertyTypeString)
	}
	// Regression guard — `assigned_to` moves to a graph edge in MVP-WORK-010
	// and must no longer appear as a Task property.
	if _, ok := got["assigned_to"]; ok {
		t.Errorf("assigned_to property must be dropped from the Task schema")
	}
}

func TestDefaultWorkSchema_ProjectShape(t *testing.T) {
	td := findType(t, DefaultWorkSchema(), "Project")
	if td.StorageCollection != "work_projects" {
		t.Errorf("Project.StorageCollection = %q, want %q", td.StorageCollection, "work_projects")
	}
	if td.PathSegment != "projects" {
		t.Errorf("Project.PathSegment = %q, want %q", td.PathSegment, "projects")
	}
	want := map[string]types.PropertyType{
		"name":         types.PropertyTypeString,
		"project_name": types.PropertyTypeString,
		"description":  types.PropertyTypeString,
		"github_repo":  types.PropertyTypeString,
		"task_prefix":  types.PropertyTypeString,
		"created_at":   types.PropertyTypeString,
		"updated_at":   types.PropertyTypeString,
	}
	if got := propTypes(td); !reflect.DeepEqual(got, want) {
		t.Errorf("Project property types mismatch:\n got=%v\nwant=%v", got, want)
	}
	if !findProp(t, td, "name").Required {
		t.Errorf("Project.name must be Required")
	}
}

func TestDefaultWorkSchema_AgentShape(t *testing.T) {
	td := findType(t, DefaultWorkSchema(), "Agent")
	if td.StorageCollection != "work_agents" {
		t.Errorf("Agent.StorageCollection = %q, want %q", td.StorageCollection, "work_agents")
	}
	if td.PathSegment != "agents" {
		t.Errorf("Agent.PathSegment = %q, want %q", td.PathSegment, "agents")
	}
	want := map[string]types.PropertyType{
		"agent_id":     types.PropertyTypeString,
		"display_name": types.PropertyTypeString,
		"capability":   types.PropertyTypeString,
		"created_at":   types.PropertyTypeString,
		"updated_at":   types.PropertyTypeString,
	}
	if got := propTypes(td); !reflect.DeepEqual(got, want) {
		t.Errorf("Agent property types mismatch:\n got=%v\nwant=%v", got, want)
	}
	if !findProp(t, td, "agent_id").Required {
		t.Errorf("Agent.agent_id must be Required")
	}
}

func TestDefaultWorkSchema_TaskRelationships_HaveInverse(t *testing.T) {
	td := findType(t, DefaultWorkSchema(), "Task")
	inverses := map[string]string{
		RelLabelAssignedTo: "assigned_tasks",
		RelLabelBlocks:     "blocked_by",
		RelLabelSubtaskOf:  "has_subtask",
		RelLabelDependsOn:  "depended_on_by",
		RelLabelMemberOf:   "has_task",
	}
	for _, rel := range td.Relationships {
		want, ok := inverses[rel.Name]
		if !ok {
			continue
		}
		if rel.Inverse != want {
			t.Errorf("Task.%s Inverse = %q, want %q", rel.Name, rel.Inverse, want)
		}
	}
}

// ── taskToProperties ─────────────────────────────────────────────────────────

func TestTaskToProperties_IncludesTimestamps(t *testing.T) {
	in := Task{
		CreatedAt: "2026-04-01T10:00:00Z",
		UpdatedAt: "2026-04-01T10:00:00Z",
	}
	props := taskToProperties(in)
	for _, key := range []string{"created_at", "updated_at"} {
		if _, ok := props[key]; !ok {
			t.Errorf("taskToProperties missing %q — timestamps must be explicit schema properties", key)
		}
	}
}

func TestTaskToProperties_RoundTrip_RichFields(t *testing.T) {
	in := Task{
		ID:             "task-1",
		AgencyID:       "agency-1",
		Description:    "World",
		Status:         TaskStatusInProgress,
		Priority:       TaskPriorityHigh,
		DueAt:          "2026-05-01T10:00:00Z",
		Tags:           []string{"alpha", "beta"},
		EstimatedHours: 4.5,
		Context:        "agent memory blob",
		CompletedAt:    "2026-04-30T09:00:00Z",
		CreatedAt:      "2026-04-01T00:00:00Z",
		UpdatedAt:      "2026-04-02T00:00:00Z",
	}
	e := entitygraph.Entity{
		ID:         in.ID,
		AgencyID:   in.AgencyID,
		TypeID:     taskTypeID,
		Properties: taskToProperties(in),
	}
	out := taskFromEntity(e)
	if !reflect.DeepEqual(out, in) {
		t.Errorf("Task round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestTaskFromEntity_AcceptsJSONDecodedTagsAndNumber(t *testing.T) {
	e := entitygraph.Entity{
		ID:       "task-1",
		AgencyID: "agency-1",
		TypeID:   taskTypeID,
		Properties: map[string]any{
			"description":     "",
			"status":          "pending",
			"priority":        "medium",
			"context":         "",
			"tags":            []any{"a", "b", "c"},
			"estimated_hours": 2.0,
		},
	}
	out := taskFromEntity(e)
	if got, want := out.Tags, []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tags = %v, want %v", got, want)
	}
	if got, want := out.EstimatedHours, 2.0; got != want {
		t.Errorf("EstimatedHours = %v, want %v", got, want)
	}
}

// ── projectToProperties / agentToProperties ──────────────────────────────────

func TestProjectToProperties_RoundTrip(t *testing.T) {
	in := Project{
		ID:          "proj-1",
		AgencyID:    "agency-1",
		Name:        "Sprint 14",
		ProjectName: "sprint_14",
		Description: "Push X out the door",
		GithubRepo:  "aosanya/CodeValdWork",
		CreatedAt:   "2026-04-01T00:00:00Z",
		UpdatedAt:   "2026-04-02T00:00:00Z",
	}
	e := entitygraph.Entity{
		ID:         in.ID,
		AgencyID:   in.AgencyID,
		TypeID:     "Project",
		Properties: projectToProperties(in),
	}
	out := projectFromEntity(e)
	if !reflect.DeepEqual(out, in) {
		t.Errorf("Project round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestAgentToProperties_RoundTrip(t *testing.T) {
	in := Agent{
		ID:          "agent-1",
		AgencyID:    "agency-1",
		AgentID:     "ai-bot-7",
		DisplayName: "Bot 7",
		Capability:  "code",
		CreatedAt:   "2026-04-01T00:00:00Z",
		UpdatedAt:   "2026-04-02T00:00:00Z",
	}
	e := entitygraph.Entity{
		ID:         in.ID,
		AgencyID:   in.AgencyID,
		TypeID:     "Agent",
		Properties: agentToProperties(in),
	}
	out := agentFromEntity(e)
	if !reflect.DeepEqual(out, in) {
		t.Errorf("Agent round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func findType(t *testing.T, s types.Schema, name string) types.TypeDefinition {
	t.Helper()
	for _, td := range s.Types {
		if td.Name == name {
			return td
		}
	}
	t.Fatalf("TypeDefinition %q not found in schema", name)
	return types.TypeDefinition{}
}

func findProp(t *testing.T, td types.TypeDefinition, name string) types.PropertyDefinition {
	t.Helper()
	for _, p := range td.Properties {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("PropertyDefinition %q not found on type %q", name, td.Name)
	return types.PropertyDefinition{}
}

func propTypes(td types.TypeDefinition) map[string]types.PropertyType {
	out := make(map[string]types.PropertyType, len(td.Properties))
	for _, p := range td.Properties {
		out[p.Name] = p.Type
	}
	return out
}
