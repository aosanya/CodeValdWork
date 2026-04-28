package codevaldwork

import (
	"reflect"
	"testing"
	"time"

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
		"title":          types.PropertyTypeString,
		"description":    types.PropertyTypeString,
		"status":         types.PropertyTypeOption,
		"priority":       types.PropertyTypeOption,
		"dueAt":          types.PropertyTypeDatetime,
		"tags":           types.PropertyTypeArray,
		"estimatedHours": types.PropertyTypeNumber,
		"context":        types.PropertyTypeString,
		"completedAt":    types.PropertyTypeDatetime,
	}
	got := propTypes(td)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Task property types mismatch:\n got=%v\nwant=%v", got, want)
	}
	tagsProp := findProp(t, td, "tags")
	if tagsProp.ElementType != types.PropertyTypeString {
		t.Errorf("tags ElementType = %v, want %v", tagsProp.ElementType, types.PropertyTypeString)
	}
	titleProp := findProp(t, td, "title")
	if !titleProp.Required {
		t.Errorf("title must be Required")
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
		"name":        types.PropertyTypeString,
		"description": types.PropertyTypeString,
		"githubRepo":  types.PropertyTypeString,
		"dueAt":       types.PropertyTypeDatetime,
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
		"agentID":     types.PropertyTypeString,
		"displayName": types.PropertyTypeString,
		"capability":  types.PropertyTypeString,
	}
	if got := propTypes(td); !reflect.DeepEqual(got, want) {
		t.Errorf("Agent property types mismatch:\n got=%v\nwant=%v", got, want)
	}
	if !findProp(t, td, "agentID").Required {
		t.Errorf("Agent.agentID must be Required")
	}
}

func TestDefaultWorkSchema_StatusOptionsMatchConstants(t *testing.T) {
	td := findType(t, DefaultWorkSchema(), "Task")
	got := findProp(t, td, "status").Options
	want := []string{
		string(TaskStatusPending),
		string(TaskStatusInProgress),
		string(TaskStatusCompleted),
		string(TaskStatusFailed),
		string(TaskStatusCancelled),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("status Options = %v, want %v", got, want)
	}
}

func TestDefaultWorkSchema_PriorityOptionsMatchConstants(t *testing.T) {
	td := findType(t, DefaultWorkSchema(), "Task")
	got := findProp(t, td, "priority").Options
	want := []string{
		string(TaskPriorityLow),
		string(TaskPriorityMedium),
		string(TaskPriorityHigh),
		string(TaskPriorityCritical),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("priority Options = %v, want %v", got, want)
	}
}

// ── taskToProperties ─────────────────────────────────────────────────────────

func TestTaskToProperties_DropsEntityTimestampKeys(t *testing.T) {
	now := time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC)
	props := taskToProperties(Task{
		Title:     "x",
		CreatedAt: now,
		UpdatedAt: now,
	})
	for _, key := range []string{"created_at", "updated_at"} {
		if _, ok := props[key]; ok {
			t.Errorf("taskToProperties wrote %q — entity timestamps must come from entitygraph.Entity, not properties", key)
		}
	}
}

func TestTaskToProperties_RoundTrip_RichFields(t *testing.T) {
	due := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	completed := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	in := Task{
		ID:             "task-1",
		AgencyID:       "agency-1",
		Title:          "Hello",
		Description:    "World",
		Status:         TaskStatusInProgress,
		Priority:       TaskPriorityHigh,
		DueAt:          &due,
		Tags:           []string{"alpha", "beta"},
		EstimatedHours: 4.5,
		Context:        "agent memory blob",
		CompletedAt:    &completed,
		CreatedAt:      time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	}
	e := entitygraph.Entity{
		ID:         in.ID,
		AgencyID:   in.AgencyID,
		TypeID:     taskTypeID,
		Properties: taskToProperties(in),
		CreatedAt:  in.CreatedAt,
		UpdatedAt:  in.UpdatedAt,
	}
	out := taskFromEntity(e)
	if !reflect.DeepEqual(out, in) {
		t.Errorf("Task round-trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestTaskFromEntity_AcceptsJSONDecodedTagsAndNumber(t *testing.T) {
	// Simulate the wire form returned by the ArangoDB backend after JSON decode.
	e := entitygraph.Entity{
		ID:       "task-1",
		AgencyID: "agency-1",
		TypeID:   taskTypeID,
		Properties: map[string]any{
			"title":          "x",
			"description":    "",
			"status":         "pending",
			"priority":       "medium",
			"context":        "",
			"tags":           []any{"a", "b", "c"},
			"estimatedHours": 2.0,
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
	due := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	in := Project{
		ID:          "proj-1",
		AgencyID:    "agency-1",
		Name:        "Sprint 14",
		Description: "Push X out the door",
		GithubRepo:  "aosanya/CodeValdWork",
		DueAt:       &due,
		CreatedAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	}
	e := entitygraph.Entity{
		ID:         in.ID,
		AgencyID:   in.AgencyID,
		TypeID:     "Project",
		Properties: projectToProperties(in),
		CreatedAt:  in.CreatedAt,
		UpdatedAt:  in.UpdatedAt,
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
		CreatedAt:   time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
	}
	e := entitygraph.Entity{
		ID:         in.ID,
		AgencyID:   in.AgencyID,
		TypeID:     "Agent",
		Properties: agentToProperties(in),
		CreatedAt:  in.CreatedAt,
		UpdatedAt:  in.UpdatedAt,
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
