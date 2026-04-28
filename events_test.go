package codevaldwork_test

import (
	"context"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
)

// findEvent returns the first recorded Event whose Topic matches.
// Returns the zero Event and false when no match is found.
func findEvent(events []eventbus.Event, topic string) (eventbus.Event, bool) {
	for _, e := range events {
		if e.Topic == topic {
			return e, true
		}
	}
	return eventbus.Event{}, false
}

func TestCreateTask_PublishesTypedTaskCreatedPayload(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", codevaldwork.Task{
		Title:    "build it",
		Priority: codevaldwork.TaskPriorityHigh,
	})

	ev, ok := findEvent(pub.full, codevaldwork.TopicTaskCreated)
	if !ok {
		t.Fatal("no work.task.created event published")
	}
	if ev.AgencyID != "ag" {
		t.Errorf("AgencyID = %s, want ag", ev.AgencyID)
	}
	p, ok := ev.Payload.(codevaldwork.TaskCreatedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want TaskCreatedPayload", ev.Payload)
	}
	if p.TaskID != created.ID || p.Title != "build it" || p.Priority != codevaldwork.TaskPriorityHigh {
		t.Errorf("payload = %+v", p)
	}
	if ev.Timestamp.IsZero() {
		t.Error("Event.Timestamp not stamped")
	}
}

func TestUpdateTask_NoStatusChange_PublishesUpdatedNotStatusChanged(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", codevaldwork.Task{Title: "x"})

	created.Description = "patched"
	if _, err := mgr.UpdateTask(context.Background(), "ag", created); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if _, ok := findEvent(pub.full, codevaldwork.TopicTaskStatusChanged); ok {
		t.Error("status.changed fired when no status change occurred")
	}
	ev, ok := findEvent(pub.full, codevaldwork.TopicTaskUpdated)
	if !ok {
		t.Fatal("no work.task.updated event")
	}
	p, _ := ev.Payload.(codevaldwork.TaskUpdatedPayload)
	if len(p.ChangedFields) != 1 || p.ChangedFields[0] != "description" {
		t.Errorf("ChangedFields = %v, want [description]", p.ChangedFields)
	}
}

func TestUpdateTask_StatusChange_FiresStatusChangedWithFromTo(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", codevaldwork.Task{Title: "x"})

	created.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "ag", created); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	ev, ok := findEvent(pub.full, codevaldwork.TopicTaskStatusChanged)
	if !ok {
		t.Fatal("no status.changed event")
	}
	p, _ := ev.Payload.(codevaldwork.TaskStatusChangedPayload)
	if p.From != codevaldwork.TaskStatusPending || p.To != codevaldwork.TaskStatusInProgress {
		t.Errorf("from/to = %s→%s, want pending→in_progress", p.From, p.To)
	}
	// In_progress is not terminal; completed must NOT have fired.
	if _, ok := findEvent(pub.full, codevaldwork.TopicTaskCompleted); ok {
		t.Error("completed event fired on non-terminal transition")
	}
}

func TestUpdateTask_StatusChangeOnly_DoesNotFireUpdated(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	created, _ := mgr.CreateTask(context.Background(), "ag", codevaldwork.Task{Title: "x"})

	// Only the status field differs.
	created.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "ag", created); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if _, ok := findEvent(pub.full, codevaldwork.TopicTaskUpdated); ok {
		t.Error("updated event fired when only status changed")
	}
}

func TestAssignTask_Replacement_FiresAssignedOnce(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	task, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "t"})
	a1, _ := mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "a1"})
	a2, _ := mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "a2"})

	// Reset captured events so we count only the reassign hits.
	if err := mgr.AssignTask(ctx, "ag", task.ID, a1.ID); err != nil {
		t.Fatalf("AssignTask a1: %v", err)
	}
	pub.full = nil
	pub.events = nil

	if err := mgr.AssignTask(ctx, "ag", task.ID, a2.ID); err != nil {
		t.Fatalf("AssignTask a2: %v", err)
	}

	count := 0
	for _, e := range pub.full {
		if e.Topic == codevaldwork.TopicTaskAssigned {
			count++
		}
	}
	if count != 1 {
		t.Errorf("reassignment fired %d assigned events, want 1", count)
	}
	ev, _ := findEvent(pub.full, codevaldwork.TopicTaskAssigned)
	p, _ := ev.Payload.(codevaldwork.TaskAssignedPayload)
	if p.AgentID != a2.ID {
		t.Errorf("AgentID = %s, want %s (the new assignee)", p.AgentID, a2.ID)
	}
}

func TestCreateRelationship_PublishesTypedRelationshipCreatedPayload(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "a"})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "b"})

	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a.ID, ToID: b.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	ev, ok := findEvent(pub.full, codevaldwork.TopicRelationshipCreated)
	if !ok {
		t.Fatal("no relationship.created event")
	}
	p, ok := ev.Payload.(codevaldwork.RelationshipCreatedPayload)
	if !ok {
		t.Fatalf("payload type = %T", ev.Payload)
	}
	if p.FromID != a.ID || p.ToID != b.ID || p.Label != codevaldwork.RelLabelBlocks {
		t.Errorf("payload = %+v", p)
	}
}

func TestFailedValidation_DoesNotPublish(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	// Empty title fails validation; no event must fire.
	if _, err := mgr.CreateTask(context.Background(), "ag", codevaldwork.Task{}); err == nil {
		t.Fatal("want error on empty title, got nil")
	}
	if len(pub.full) != 0 {
		t.Errorf("validation failure published events: %v", pub.full)
	}
}

// TestEventSequence_FullPhase2Flow_EmitsExactOrderedTopics drives the
// canonical Phase 2 lifecycle (create → update → assign → status changes
// to completed → create blocks edge) past a recordingPublisher and asserts
// the *exact* list of topics emitted, in order. Single-event tests above
// cover payload shape; this test pins the cross-event ordering — in
// particular, that status.changed precedes completed on a terminal
// transition, and that an update with no status change does not surface
// a status.changed event.
func TestEventSequence_FullPhase2Flow_EmitsExactOrderedTopics(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	const agency = "ag"

	// Step 1 — create a Task.
	task, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{
		Title: "x", Priority: codevaldwork.TaskPriorityHigh,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Step 2 — non-status update (description only).
	task.Description = "patched"
	if _, err := mgr.UpdateTask(ctx, agency, task); err != nil {
		t.Fatalf("UpdateTask description: %v", err)
	}

	// Step 3 — assign to a fresh agent.
	agent, err := mgr.UpsertAgent(ctx, agency, codevaldwork.Agent{AgentID: "a1"})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	if err := mgr.AssignTask(ctx, agency, task.ID, agent.ID); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	// Step 4 — drive task to completed via in_progress.
	cur, err := mgr.GetTask(ctx, agency, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	cur.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agency, cur); err != nil {
		t.Fatalf("→ in_progress: %v", err)
	}
	cur.Status = codevaldwork.TaskStatusCompleted
	if _, err := mgr.UpdateTask(ctx, agency, cur); err != nil {
		t.Fatalf("→ completed: %v", err)
	}

	// Step 5 — create a blocks edge to a sibling Task.
	other, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "other"})
	if err != nil {
		t.Fatalf("CreateTask other: %v", err)
	}
	if _, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: task.ID, ToID: other.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	wantTopics := []string{
		codevaldwork.TopicTaskCreated,         // step 1
		codevaldwork.TopicTaskUpdated,         // step 2 (no status change)
		codevaldwork.TopicTaskAssigned,        // step 3
		codevaldwork.TopicTaskStatusChanged,   // step 4a pending → in_progress
		codevaldwork.TopicTaskStatusChanged,   // step 4b in_progress → completed
		codevaldwork.TopicTaskCompleted,       // step 4b terminal hook
		codevaldwork.TopicTaskCreated,         // step 5 sibling
		codevaldwork.TopicRelationshipCreated, // step 5 edge
	}
	gotTopics := make([]string, len(pub.full))
	for i, e := range pub.full {
		gotTopics[i] = e.Topic
	}
	if len(gotTopics) != len(wantTopics) {
		t.Fatalf("event count: got %d, want %d\n got=%v\nwant=%v",
			len(gotTopics), len(wantTopics), gotTopics, wantTopics)
	}
	for i := range wantTopics {
		if gotTopics[i] != wantTopics[i] {
			t.Errorf("event[%d] topic: got %q, want %q (full sequence: %v)",
				i, gotTopics[i], wantTopics[i], gotTopics)
		}
	}

	// Spot-check key payloads — the full sequence is locked above; here we
	// confirm the typed payloads are intact at the load-bearing positions.
	if p, ok := pub.full[0].Payload.(codevaldwork.TaskCreatedPayload); !ok ||
		p.TaskID != task.ID || p.Title != "x" || p.Priority != codevaldwork.TaskPriorityHigh {
		t.Errorf("event[0] TaskCreatedPayload = %+v", pub.full[0].Payload)
	}
	if p, ok := pub.full[2].Payload.(codevaldwork.TaskAssignedPayload); !ok ||
		p.TaskID != task.ID || p.AgentID != agent.ID {
		t.Errorf("event[2] TaskAssignedPayload = %+v", pub.full[2].Payload)
	}
	if p, ok := pub.full[3].Payload.(codevaldwork.TaskStatusChangedPayload); !ok ||
		p.From != codevaldwork.TaskStatusPending || p.To != codevaldwork.TaskStatusInProgress {
		t.Errorf("event[3] TaskStatusChangedPayload = %+v", pub.full[3].Payload)
	}
	if p, ok := pub.full[4].Payload.(codevaldwork.TaskStatusChangedPayload); !ok ||
		p.From != codevaldwork.TaskStatusInProgress || p.To != codevaldwork.TaskStatusCompleted {
		t.Errorf("event[4] TaskStatusChangedPayload = %+v", pub.full[4].Payload)
	}
	if p, ok := pub.full[5].Payload.(codevaldwork.TaskCompletedPayload); !ok ||
		p.TaskID != task.ID || p.TerminalStatus != codevaldwork.TaskStatusCompleted {
		t.Errorf("event[5] TaskCompletedPayload = %+v", pub.full[5].Payload)
	}
	if p, ok := pub.full[7].Payload.(codevaldwork.RelationshipCreatedPayload); !ok ||
		p.FromID != task.ID || p.ToID != other.ID || p.Label != codevaldwork.RelLabelBlocks {
		t.Errorf("event[7] RelationshipCreatedPayload = %+v", pub.full[7].Payload)
	}
}

// AllTopics must list every topic constant exactly once. The registrar's
// produces declaration depends on this — drift here silently breaks
// subscriber discovery.
func TestAllTopics_StableSurface(t *testing.T) {
	got := codevaldwork.AllTopics()
	want := []string{
		codevaldwork.TopicTaskCreated,
		codevaldwork.TopicTaskUpdated,
		codevaldwork.TopicTaskStatusChanged,
		codevaldwork.TopicTaskCompleted,
		codevaldwork.TopicTaskAssigned,
		codevaldwork.TopicRelationshipCreated,
	}
	if len(got) != len(want) {
		t.Fatalf("count: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("topic[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}
