package codevaldwork_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// ── CreateTaskGroup ──────────────────────────────────────────────────────────

func TestCreateTaskGroup_EmptyName_ReturnsErrInvalidTask(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.CreateTaskGroup(context.Background(), "ag", codevaldwork.TaskGroup{})
	if !errors.Is(err, codevaldwork.ErrInvalidTask) {
		t.Fatalf("got %v, want ErrInvalidTask", err)
	}
}

func TestCreateTaskGroup_RoundTrip(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	g, err := mgr.CreateTaskGroup(context.Background(), "ag", codevaldwork.TaskGroup{
		Name:        "Sprint 7",
		Description: "Q2 push",
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}
	if g.ID == "" {
		t.Error("group missing ID")
	}
	if g.Name != "Sprint 7" || g.Description != "Q2 push" {
		t.Errorf("unexpected: %+v", g)
	}
}

// ── GetTaskGroup ─────────────────────────────────────────────────────────────

func TestGetTaskGroup_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetTaskGroup(context.Background(), "ag", "missing")
	if !errors.Is(err, codevaldwork.ErrTaskGroupNotFound) {
		t.Fatalf("got %v, want ErrTaskGroupNotFound", err)
	}
}

// ── UpdateTaskGroup ──────────────────────────────────────────────────────────

func TestUpdateTaskGroup_PatchesFields(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	g, _ := mgr.CreateTaskGroup(ctx, "ag", codevaldwork.TaskGroup{Name: "Old"})

	g.Name = "New"
	g.Description = "patched"
	updated, err := mgr.UpdateTaskGroup(ctx, "ag", g)
	if err != nil {
		t.Fatalf("UpdateTaskGroup: %v", err)
	}
	if updated.Name != "New" || updated.Description != "patched" {
		t.Errorf("unexpected: %+v", updated)
	}
	if updated.ID != g.ID {
		t.Error("update created new vertex")
	}
}

func TestUpdateTaskGroup_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.UpdateTaskGroup(context.Background(), "ag", codevaldwork.TaskGroup{
		ID: "missing", Name: "x",
	})
	if !errors.Is(err, codevaldwork.ErrTaskGroupNotFound) {
		t.Fatalf("got %v, want ErrTaskGroupNotFound", err)
	}
}

// ── ListTaskGroups ───────────────────────────────────────────────────────────

func TestListTaskGroups_AgencyIsolation(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	_, _ = mgr.CreateTaskGroup(ctx, "agency-A", codevaldwork.TaskGroup{Name: "A1"})
	_, _ = mgr.CreateTaskGroup(ctx, "agency-A", codevaldwork.TaskGroup{Name: "A2"})
	_, _ = mgr.CreateTaskGroup(ctx, "agency-B", codevaldwork.TaskGroup{Name: "B1"})

	a, _ := mgr.ListTaskGroups(ctx, "agency-A")
	if len(a) != 2 {
		t.Errorf("agency-A: want 2, got %d", len(a))
	}
	b, _ := mgr.ListTaskGroups(ctx, "agency-B")
	if len(b) != 1 {
		t.Errorf("agency-B: want 1, got %d", len(b))
	}
}

// ── AddTaskToGroup / ListTasksInGroup ────────────────────────────────────────

func TestAddTaskToGroup_ListTasksInGroup_RoundTrip(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	g, _ := mgr.CreateTaskGroup(ctx, "ag", codevaldwork.TaskGroup{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})

	if err := mgr.AddTaskToGroup(ctx, "ag", t1.ID, g.ID); err != nil {
		t.Fatalf("AddTaskToGroup: %v", err)
	}
	tasks, err := mgr.ListTasksInGroup(ctx, "ag", g.ID)
	if err != nil {
		t.Fatalf("ListTasksInGroup: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != t1.ID {
		t.Errorf("got %v, want [task-1=%s]", tasks, t1.ID)
	}
}

func TestAddTaskToGroup_Twice_IsIdempotent(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	ctx := context.Background()
	g, _ := mgr.CreateTaskGroup(ctx, "ag", codevaldwork.TaskGroup{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})

	if err := mgr.AddTaskToGroup(ctx, "ag", t1.ID, g.ID); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := mgr.AddTaskToGroup(ctx, "ag", t1.ID, g.ID); err != nil {
		t.Fatalf("second add: %v", err)
	}

	all, _ := fake.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: "ag", Name: codevaldwork.RelLabelMemberOf,
	})
	if len(all) != 1 {
		t.Errorf("want 1 member_of edge after re-add, got %d", len(all))
	}
}

// ── RemoveTaskFromGroup ──────────────────────────────────────────────────────

func TestRemoveTaskFromGroup_RemovesMembership(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	g, _ := mgr.CreateTaskGroup(ctx, "ag", codevaldwork.TaskGroup{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})
	_ = mgr.AddTaskToGroup(ctx, "ag", t1.ID, g.ID)

	if err := mgr.RemoveTaskFromGroup(ctx, "ag", t1.ID, g.ID); err != nil {
		t.Fatalf("RemoveTaskFromGroup: %v", err)
	}
	tasks, _ := mgr.ListTasksInGroup(ctx, "ag", g.ID)
	if len(tasks) != 0 {
		t.Errorf("want 0 members after remove, got %d", len(tasks))
	}
}

// ── DeleteTaskGroup ──────────────────────────────────────────────────────────

func TestDeleteTaskGroup_RemovesGroupAndAllMemberOfEdges_TasksRemain(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	ctx := context.Background()
	g, _ := mgr.CreateTaskGroup(ctx, "ag", codevaldwork.TaskGroup{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})
	t2, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-2"})
	_ = mgr.AddTaskToGroup(ctx, "ag", t1.ID, g.ID)
	_ = mgr.AddTaskToGroup(ctx, "ag", t2.ID, g.ID)

	if err := mgr.DeleteTaskGroup(ctx, "ag", g.ID); err != nil {
		t.Fatalf("DeleteTaskGroup: %v", err)
	}

	// Group is gone.
	if _, err := mgr.GetTaskGroup(ctx, "ag", g.ID); !errors.Is(err, codevaldwork.ErrTaskGroupNotFound) {
		t.Errorf("group still resolvable: got %v, want ErrTaskGroupNotFound", err)
	}

	// All member_of edges removed.
	rels, _ := fake.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: "ag", Name: codevaldwork.RelLabelMemberOf,
	})
	if len(rels) != 0 {
		t.Errorf("want 0 member_of edges after group delete, got %d", len(rels))
	}

	// Member Tasks themselves still resolve.
	for _, id := range []string{t1.ID, t2.ID} {
		if _, err := mgr.GetTask(ctx, "ag", id); err != nil {
			t.Errorf("task %s should survive group deletion: %v", id, err)
		}
	}
}

func TestDeleteTaskGroup_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	err := mgr.DeleteTaskGroup(context.Background(), "ag", "missing")
	if !errors.Is(err, codevaldwork.ErrTaskGroupNotFound) {
		t.Fatalf("got %v, want ErrTaskGroupNotFound", err)
	}
}

// ── ListGroupsForTask ────────────────────────────────────────────────────────

func TestListGroupsForTask_TaskInMultipleGroups(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	g1, _ := mgr.CreateTaskGroup(ctx, "ag", codevaldwork.TaskGroup{Name: "Sprint"})
	g2, _ := mgr.CreateTaskGroup(ctx, "ag", codevaldwork.TaskGroup{Name: "Epic"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "shared"})
	_ = mgr.AddTaskToGroup(ctx, "ag", t1.ID, g1.ID)
	_ = mgr.AddTaskToGroup(ctx, "ag", t1.ID, g2.ID)

	groups, err := mgr.ListGroupsForTask(ctx, "ag", t1.ID)
	if err != nil {
		t.Fatalf("ListGroupsForTask: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("want 2 groups, got %d", len(groups))
	}
}

// ── Whitelist regression guard ───────────────────────────────────────────────

// Re-asserting WORK-009's edge-label whitelist: a `member_of` edge from a
// Task to a Task (instead of TaskGroup) must be rejected with
// ErrInvalidRelationship. This guards against accidental whitelist drift.
func TestMemberOf_NonTaskGroupTarget_Rejected(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "a"})
	t2, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "b"})

	_, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelMemberOf, FromID: t1.ID, ToID: t2.ID,
	})
	if !errors.Is(err, codevaldwork.ErrInvalidRelationship) {
		t.Fatalf("got %v, want ErrInvalidRelationship", err)
	}
}
