package codevaldwork_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// seedTask creates a real Task via the manager so that the resulting entity
// has the correct TypeID = "Task" in the underlying fake. Returns the task
// ID for use as a relationship endpoint.
func seedTask(t *testing.T, mgr codevaldwork.TaskManager, agencyID, title string) string {
	t.Helper()
	task, err := mgr.CreateTask(context.Background(), agencyID, codevaldwork.Task{Title: title})
	if err != nil {
		t.Fatalf("seedTask(%q): %v", title, err)
	}
	return task.ID
}

// seedVertex inserts a raw entity of the given TypeID into the fake — used to
// stand up Agent / Project vertices since the WORK-009 manager doesn't yet
// expose UpsertAgent / CreateProject (those land in WORK-010 / WORK-012).
func seedVertex(t *testing.T, fake *fakeDataManager, agencyID, typeID string) string {
	t.Helper()
	e, err := fake.CreateEntity(context.Background(), entitygraph.CreateEntityRequest{
		AgencyID:   agencyID,
		TypeID:     typeID,
		Properties: map[string]any{},
	})
	if err != nil {
		t.Fatalf("seedVertex(%s): %v", typeID, err)
	}
	return e.ID
}

// ── CreateRelationship ───────────────────────────────────────────────────────

func TestCreateRelationship_AllWhitelistedLabels(t *testing.T) {
	fake := newFakeDataManager()
	mgr, err := codevaldwork.NewTaskManager(fake, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	ctx := context.Background()
	const agency = "agency-1"

	taskA := seedTask(t, mgr, agency, "A")
	taskB := seedTask(t, mgr, agency, "B")
	agent := seedVertex(t, fake, agency, "Agent")
	project := seedVertex(t, fake, agency, "Project")

	cases := []struct {
		label  string
		fromID string
		toID   string
	}{
		{codevaldwork.RelLabelAssignedTo, taskA, agent},
		{codevaldwork.RelLabelBlocks, taskA, taskB},
		{codevaldwork.RelLabelSubtaskOf, taskA, taskB},
		{codevaldwork.RelLabelDependsOn, taskA, taskB},
		{codevaldwork.RelLabelMemberOf, taskA, project},
	}
	for _, tc := range cases {
		// Use a fresh source-target pair per label to avoid cardinality
		// constraints (assigned_to / subtask_of are functional).
		from := seedTask(t, mgr, agency, "from-"+tc.label)
		to := tc.toID
		if tc.label == codevaldwork.RelLabelBlocks ||
			tc.label == codevaldwork.RelLabelSubtaskOf ||
			tc.label == codevaldwork.RelLabelDependsOn {
			to = seedTask(t, mgr, agency, "to-"+tc.label)
		}
		out, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
			Label:  tc.label,
			FromID: from,
			ToID:   to,
		})
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.label, err)
			continue
		}
		if out.ID == "" {
			t.Errorf("%s: edge missing ID", tc.label)
		}
		if out.Label != tc.label || out.FromID != from || out.ToID != to {
			t.Errorf("%s: round-trip mismatch: %+v", tc.label, out)
		}
		_ = tc.fromID
	}
}

func TestCreateRelationship_UnknownLabel_ReturnsErrInvalidRelationship(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	a := seedTask(t, mgr, "ag", "a")
	b := seedTask(t, mgr, "ag", "b")

	_, err := mgr.CreateRelationship(context.Background(), "ag", codevaldwork.Relationship{
		Label: "not_a_real_label", FromID: a, ToID: b,
	})
	if !errors.Is(err, codevaldwork.ErrInvalidRelationship) {
		t.Fatalf("got %v, want ErrInvalidRelationship", err)
	}
}

func TestCreateRelationship_WrongVertexType_ReturnsErrInvalidRelationship(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	taskID := seedTask(t, mgr, "ag", "task")
	projectID := seedVertex(t, fake, "ag", "Project")

	// blocks must point Task→Task; using Project as the target is invalid.
	_, err := mgr.CreateRelationship(context.Background(), "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: taskID, ToID: projectID,
	})
	if !errors.Is(err, codevaldwork.ErrInvalidRelationship) {
		t.Fatalf("got %v, want ErrInvalidRelationship", err)
	}
}

func TestCreateRelationship_MissingTaskEndpoint_ReturnsErrTaskNotFound(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	taskID := seedTask(t, mgr, "ag", "a")

	_, err := mgr.CreateRelationship(context.Background(), "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: taskID, ToID: "no-such-task",
	})
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("got %v, want ErrTaskNotFound", err)
	}
}

func TestCreateRelationship_MissingAgentEndpoint_ReturnsErrAgentNotFound(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	taskID := seedTask(t, mgr, "ag", "a")

	_, err := mgr.CreateRelationship(context.Background(), "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelAssignedTo, FromID: taskID, ToID: "no-such-agent",
	})
	if !errors.Is(err, codevaldwork.ErrAgentNotFound) {
		t.Fatalf("got %v, want ErrAgentNotFound", err)
	}
}

func TestCreateRelationship_MissingProjectEndpoint_ReturnsErrProjectNotFound(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	taskID := seedTask(t, mgr, "ag", "a")

	_, err := mgr.CreateRelationship(context.Background(), "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelMemberOf, FromID: taskID, ToID: "no-such-project",
	})
	if !errors.Is(err, codevaldwork.ErrProjectNotFound) {
		t.Fatalf("got %v, want ErrProjectNotFound", err)
	}
}

func TestCreateRelationship_CrossAgencyEdge_ReturnsErrTaskNotFound(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)

	a := seedTask(t, mgr, "agency-A", "a")
	b := seedTask(t, mgr, "agency-B", "b")

	// Looking up b under agency-A returns ErrTaskNotFound — the cross-agency
	// edge is rejected because the To endpoint is invisible from agency A.
	_, err := mgr.CreateRelationship(context.Background(), "agency-A", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a, ToID: b,
	})
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("got %v, want ErrTaskNotFound", err)
	}
}

func TestCreateRelationship_RecreateExisting_IsIdempotent(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	a := seedTask(t, mgr, "ag", "a")
	b := seedTask(t, mgr, "ag", "b")
	ctx := context.Background()

	first, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a, ToID: b,
	})
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	second, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a, ToID: b,
	})
	if err != nil {
		t.Fatalf("second create: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("idempotent re-create returned new edge: first=%s second=%s", first.ID, second.ID)
	}

	all, _ := fake.ListRelationships(ctx, entitygraph.RelationshipFilter{AgencyID: "ag"})
	if len(all) != 1 {
		t.Errorf("want exactly 1 edge in store, got %d", len(all))
	}
}

func TestCreateRelationship_PublishesEvent(t *testing.T) {
	fake := newFakeDataManager()
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(fake, pub)
	a := seedTask(t, mgr, "ag", "a")
	b := seedTask(t, mgr, "ag", "b")

	if _, err := mgr.CreateRelationship(context.Background(), "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a, ToID: b,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	var found bool
	for _, ev := range pub.events {
		if ev == "work.relationship.created|ag" {
			found = true
		}
	}
	if !found {
		t.Errorf("want work.relationship.created event, got %v", pub.events)
	}
}

// ── DeleteRelationship ───────────────────────────────────────────────────────

func TestDeleteRelationship_Existing_RemovesEdge(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	a := seedTask(t, mgr, "ag", "a")
	b := seedTask(t, mgr, "ag", "b")
	ctx := context.Background()

	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a, ToID: b,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}
	if err := mgr.DeleteRelationship(ctx, "ag", a, b, codevaldwork.RelLabelBlocks); err != nil {
		t.Fatalf("DeleteRelationship: %v", err)
	}
	edges, _ := fake.ListRelationships(ctx, entitygraph.RelationshipFilter{AgencyID: "ag"})
	if len(edges) != 0 {
		t.Errorf("want 0 edges after delete, got %d", len(edges))
	}
}

func TestDeleteRelationship_Missing_ReturnsErrRelationshipNotFound(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	a := seedTask(t, mgr, "ag", "a")
	b := seedTask(t, mgr, "ag", "b")

	err := mgr.DeleteRelationship(context.Background(), "ag", a, b, codevaldwork.RelLabelBlocks)
	if !errors.Is(err, codevaldwork.ErrRelationshipNotFound) {
		t.Fatalf("got %v, want ErrRelationshipNotFound", err)
	}
}

// ── TraverseRelationships ────────────────────────────────────────────────────

func TestTraverseRelationships_Outbound_ReturnsAllMatchingEdges(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	source := seedTask(t, mgr, "ag", "source")
	target1 := seedTask(t, mgr, "ag", "t1")
	target2 := seedTask(t, mgr, "ag", "t2")
	ctx := context.Background()

	for _, to := range []string{target1, target2} {
		if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
			Label: codevaldwork.RelLabelBlocks, FromID: source, ToID: to,
		}); err != nil {
			t.Fatalf("CreateRelationship: %v", err)
		}
	}

	edges, err := mgr.TraverseRelationships(ctx, "ag", source, codevaldwork.RelLabelBlocks, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("want 2 edges, got %d", len(edges))
	}
	gotTargets := []string{edges[0].ToID, edges[1].ToID}
	sort.Strings(gotTargets)
	wantTargets := []string{target1, target2}
	sort.Strings(wantTargets)
	for i := range gotTargets {
		if gotTargets[i] != wantTargets[i] {
			t.Errorf("targets mismatch: got %v, want %v", gotTargets, wantTargets)
			break
		}
	}
}

func TestTraverseRelationships_Inbound_ReturnsOnlyEdgesPointingAtVertex(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	a := seedTask(t, mgr, "ag", "a")
	b := seedTask(t, mgr, "ag", "b") // a blocks b
	c := seedTask(t, mgr, "ag", "c") // b blocks c (b is on the OUTBOUND side here)
	ctx := context.Background()

	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a, ToID: b,
	}); err != nil {
		t.Fatalf("a→b: %v", err)
	}
	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: b, ToID: c,
	}); err != nil {
		t.Fatalf("b→c: %v", err)
	}

	// Inbound on b → should return only the a→b edge.
	edges, err := mgr.TraverseRelationships(ctx, "ag", b, codevaldwork.RelLabelBlocks, codevaldwork.DirectionInbound)
	if err != nil {
		t.Fatalf("TraverseRelationships: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 inbound edge on b, got %d", len(edges))
	}
	if edges[0].FromID != a || edges[0].ToID != b {
		t.Errorf("wrong edge: %+v", edges[0])
	}
}

func TestTraverseRelationships_NoMatches_ReturnsEmpty(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	a := seedTask(t, mgr, "ag", "a")

	edges, err := mgr.TraverseRelationships(context.Background(), "ag", a, codevaldwork.RelLabelBlocks, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("want 0 edges, got %d", len(edges))
	}
}
