package codevaldwork_test

import (
	"context"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// seedDependentScenario creates two tasks (a, b) where b depends_on a, an
// agent, and assigns b — which should land b in blocked because a is still
// pending. Returns refreshed values for assertions.
func seedDependentScenario(t *testing.T, mgr codevaldwork.TaskManager) (a, b codevaldwork.Task, agent codevaldwork.Agent) {
	t.Helper()
	ctx := context.Background()
	a, _ = mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "A"})
	b, _ = mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "B"})
	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelDependsOn, FromID: b.ID, ToID: a.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship depends_on: %v", err)
	}
	agent, _ = mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "worker", RoleName: "engineer"})
	if err := mgr.AssignTask(ctx, "ag", b.ID, agent.ID); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	b, _ = mgr.GetTask(ctx, "ag", b.ID)
	if b.Status != codevaldwork.TaskStatusBlocked {
		t.Fatalf("setup: expected B blocked, got %s", b.Status)
	}
	return a, b, agent
}

func TestUnblockDependents_FlipsBlockedToPending(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	a, b, _ := seedDependentScenario(t, mgr)

	// Drive A to completed so its outbound depends_on side is satisfied.
	completeBlocker(t, mgr, "ag", a)

	if err := mgr.UnblockDependents(ctx, "ag", a.ID); err != nil {
		t.Fatalf("UnblockDependents: %v", err)
	}

	got, _ := mgr.GetTask(ctx, "ag", b.ID)
	if got.Status != codevaldwork.TaskStatusPending {
		t.Errorf("B.Status = %s, want %s", got.Status, codevaldwork.TaskStatusPending)
	}
}

func TestUnblockDependents_RepublishesAssignedEvent(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	a, b, agent := seedDependentScenario(t, mgr)
	completeBlocker(t, mgr, "ag", a)
	pub.events = nil
	pub.full = nil

	if err := mgr.UnblockDependents(ctx, "ag", a.ID); err != nil {
		t.Fatalf("UnblockDependents: %v", err)
	}

	var sawStatus, sawAssigned bool
	for _, ev := range pub.full {
		if ev.Topic == codevaldwork.TopicTaskStatusChanged {
			if p, ok := ev.Payload.(codevaldwork.TaskStatusChangedPayload); ok && p.TaskID == b.ID &&
				p.From == codevaldwork.TaskStatusBlocked && p.To == codevaldwork.TaskStatusPending {
				sawStatus = true
			}
		}
		if ev.Topic == codevaldwork.TopicTaskAssigned {
			if p, ok := ev.Payload.(codevaldwork.TaskAssignedPayload); ok && p.TaskID == b.ID && p.AgentID == agent.ID {
				sawAssigned = true
			}
		}
	}
	if !sawStatus {
		t.Errorf("missing work.task.status.changed blocked→pending; got %v", pub.events)
	}
	if !sawAssigned {
		t.Errorf("missing work.task.assigned for unblocked task; got %v", pub.events)
	}
}

func TestUnblockDependents_LeavesBlockedWhenOtherDepUnmet(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	a1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	a2, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	for _, dep := range []string{a1.ID, a2.ID} {
		if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
			Label: codevaldwork.RelLabelDependsOn, FromID: b.ID, ToID: dep,
		}); err != nil {
			t.Fatalf("CreateRelationship depends_on: %v", err)
		}
	}
	agent, _ := mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "w"})
	if err := mgr.AssignTask(ctx, "ag", b.ID, agent.ID); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	completeBlocker(t, mgr, "ag", a1) // a2 still pending

	if err := mgr.UnblockDependents(ctx, "ag", a1.ID); err != nil {
		t.Fatalf("UnblockDependents: %v", err)
	}
	got, _ := mgr.GetTask(ctx, "ag", b.ID)
	if got.Status != codevaldwork.TaskStatusBlocked {
		t.Errorf("B.Status = %s, want still blocked (a2 unmet)", got.Status)
	}
}

func TestUnblockDependents_NoAssigneeStaysBlocked(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelDependsOn, FromID: b.ID, ToID: a.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship depends_on: %v", err)
	}
	// Manually park B in blocked without an assigned_to edge.
	b.Status = codevaldwork.TaskStatusBlocked
	if _, err := mgr.UpdateTask(ctx, "ag", b); err != nil {
		t.Fatalf("UpdateTask blocked: %v", err)
	}
	completeBlocker(t, mgr, "ag", a)

	if err := mgr.UnblockDependents(ctx, "ag", a.ID); err != nil {
		t.Fatalf("UnblockDependents: %v", err)
	}
	got, _ := mgr.GetTask(ctx, "ag", b.ID)
	if got.Status != codevaldwork.TaskStatusBlocked {
		t.Errorf("B.Status = %s, want still blocked (no assignee)", got.Status)
	}
}

func TestUnblockDependents_Idempotent(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()
	a, b, _ := seedDependentScenario(t, mgr)
	completeBlocker(t, mgr, "ag", a)

	if err := mgr.UnblockDependents(ctx, "ag", a.ID); err != nil {
		t.Fatalf("first UnblockDependents: %v", err)
	}
	pub.events = nil
	pub.full = nil
	if err := mgr.UnblockDependents(ctx, "ag", a.ID); err != nil {
		t.Fatalf("second UnblockDependents: %v", err)
	}
	got, _ := mgr.GetTask(ctx, "ag", b.ID)
	if got.Status != codevaldwork.TaskStatusPending {
		t.Errorf("B.Status = %s, want pending after redelivery", got.Status)
	}
	for _, ev := range pub.events {
		if ev == "work.task.assigned|ag" || ev == "work.task.status.changed|ag" {
			t.Errorf("second call republished %s — handler is not idempotent", ev)
		}
	}
}

func TestUnblockDependents_UnknownTaskReturnsError(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	if err := mgr.UnblockDependents(context.Background(), "ag", "no-such-task"); err == nil {
		t.Errorf("UnblockDependents on missing task: got nil, want error")
	}
}
