package codevaldwork_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
)

// ── CreateProject ────────────────────────────────────────────────────────────

func TestCreateProject_EmptyName_ReturnsErrInvalidTask(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.CreateProject(context.Background(), "ag", codevaldwork.Project{})
	if !errors.Is(err, codevaldwork.ErrInvalidTask) {
		t.Fatalf("got %v, want ErrInvalidTask", err)
	}
}

func TestCreateProject_RoundTrip(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	p, err := mgr.CreateProject(context.Background(), "ag", codevaldwork.Project{
		Name:        "Sprint 7",
		Description: "Q2 push",
		GithubRepo:  "aosanya/CodeValdWork",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if p.ID == "" {
		t.Error("project missing ID")
	}
	if p.Name != "Sprint 7" || p.Description != "Q2 push" || p.GithubRepo != "aosanya/CodeValdWork" {
		t.Errorf("unexpected: %+v", p)
	}
}

// ── GetProject ───────────────────────────────────────────────────────────────

func TestGetProject_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetProject(context.Background(), "ag", "missing")
	if !errors.Is(err, codevaldwork.ErrProjectNotFound) {
		t.Fatalf("got %v, want ErrProjectNotFound", err)
	}
}

// ── UpdateProject ────────────────────────────────────────────────────────────

func TestUpdateProject_PatchesFields(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	p, _ := mgr.CreateProject(ctx, "ag", codevaldwork.Project{Name: "Old"})

	p.Name = "New"
	p.Description = "patched"
	p.GithubRepo = "aosanya/Foo"
	updated, err := mgr.UpdateProject(ctx, "ag", p)
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if updated.Name != "New" || updated.Description != "patched" || updated.GithubRepo != "aosanya/Foo" {
		t.Errorf("unexpected: %+v", updated)
	}
	if updated.ID != p.ID {
		t.Error("update created new vertex")
	}
}

func TestUpdateProject_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.UpdateProject(context.Background(), "ag", codevaldwork.Project{
		ID: "missing", Name: "x",
	})
	if !errors.Is(err, codevaldwork.ErrProjectNotFound) {
		t.Fatalf("got %v, want ErrProjectNotFound", err)
	}
}

// ── ListProjects ─────────────────────────────────────────────────────────────

func TestListProjects_AgencyIsolation(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	_, _ = mgr.CreateProject(ctx, "agency-A", codevaldwork.Project{Name: "A1"})
	_, _ = mgr.CreateProject(ctx, "agency-A", codevaldwork.Project{Name: "A2"})
	_, _ = mgr.CreateProject(ctx, "agency-B", codevaldwork.Project{Name: "B1"})

	a, _ := mgr.ListProjects(ctx, "agency-A")
	if len(a) != 2 {
		t.Errorf("agency-A: want 2, got %d", len(a))
	}
	b, _ := mgr.ListProjects(ctx, "agency-B")
	if len(b) != 1 {
		t.Errorf("agency-B: want 1, got %d", len(b))
	}
}

// ── AddTaskToProject / ListTasksInProject ────────────────────────────────────

func TestAddTaskToProject_ListTasksInProject_RoundTrip(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	p, _ := mgr.CreateProject(ctx, "ag", codevaldwork.Project{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})

	if err := mgr.AddTaskToProject(ctx, "ag", t1.ID, p.ID); err != nil {
		t.Fatalf("AddTaskToProject: %v", err)
	}
	tasks, err := mgr.ListTasksInProject(ctx, "ag", p.ID)
	if err != nil {
		t.Fatalf("ListTasksInProject: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != t1.ID {
		t.Errorf("got %v, want [task-1=%s]", tasks, t1.ID)
	}
}

func TestAddTaskToProject_Twice_IsIdempotent(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	ctx := context.Background()
	p, _ := mgr.CreateProject(ctx, "ag", codevaldwork.Project{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})

	if err := mgr.AddTaskToProject(ctx, "ag", t1.ID, p.ID); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := mgr.AddTaskToProject(ctx, "ag", t1.ID, p.ID); err != nil {
		t.Fatalf("second add: %v", err)
	}

	all, _ := fake.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: "ag", Name: codevaldwork.RelLabelMemberOf,
	})
	if len(all) != 1 {
		t.Errorf("want 1 member_of edge after re-add, got %d", len(all))
	}
}

// ── RemoveTaskFromProject ────────────────────────────────────────────────────

func TestRemoveTaskFromProject_RemovesMembership(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	p, _ := mgr.CreateProject(ctx, "ag", codevaldwork.Project{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})
	_ = mgr.AddTaskToProject(ctx, "ag", t1.ID, p.ID)

	if err := mgr.RemoveTaskFromProject(ctx, "ag", t1.ID, p.ID); err != nil {
		t.Fatalf("RemoveTaskFromProject: %v", err)
	}
	tasks, _ := mgr.ListTasksInProject(ctx, "ag", p.ID)
	if len(tasks) != 0 {
		t.Errorf("want 0 members after remove, got %d", len(tasks))
	}
}

// ── DeleteProject ────────────────────────────────────────────────────────────

func TestDeleteProject_RemovesProjectAndAllMemberOfEdges_TasksRemain(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	ctx := context.Background()
	p, _ := mgr.CreateProject(ctx, "ag", codevaldwork.Project{Name: "Sprint"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-1"})
	t2, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "task-2"})
	_ = mgr.AddTaskToProject(ctx, "ag", t1.ID, p.ID)
	_ = mgr.AddTaskToProject(ctx, "ag", t2.ID, p.ID)

	if err := mgr.DeleteProject(ctx, "ag", p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	// Project is gone.
	if _, err := mgr.GetProject(ctx, "ag", p.ID); !errors.Is(err, codevaldwork.ErrProjectNotFound) {
		t.Errorf("project still resolvable: got %v, want ErrProjectNotFound", err)
	}

	// All member_of edges removed.
	rels, _ := fake.ListRelationships(ctx, entitygraph.RelationshipFilter{
		AgencyID: "ag", Name: codevaldwork.RelLabelMemberOf,
	})
	if len(rels) != 0 {
		t.Errorf("want 0 member_of edges after project delete, got %d", len(rels))
	}

	// Member Tasks themselves still resolve.
	for _, id := range []string{t1.ID, t2.ID} {
		if _, err := mgr.GetTask(ctx, "ag", id); err != nil {
			t.Errorf("task %s should survive project deletion: %v", id, err)
		}
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	err := mgr.DeleteProject(context.Background(), "ag", "missing")
	if !errors.Is(err, codevaldwork.ErrProjectNotFound) {
		t.Fatalf("got %v, want ErrProjectNotFound", err)
	}
}

// ── ListProjectsForTask ──────────────────────────────────────────────────────

func TestListProjectsForTask_TaskInMultipleProjects(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	p1, _ := mgr.CreateProject(ctx, "ag", codevaldwork.Project{Name: "Sprint"})
	p2, _ := mgr.CreateProject(ctx, "ag", codevaldwork.Project{Name: "Epic"})
	t1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "shared"})
	_ = mgr.AddTaskToProject(ctx, "ag", t1.ID, p1.ID)
	_ = mgr.AddTaskToProject(ctx, "ag", t1.ID, p2.ID)

	projects, err := mgr.ListProjectsForTask(ctx, "ag", t1.ID)
	if err != nil {
		t.Fatalf("ListProjectsForTask: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("want 2 projects, got %d", len(projects))
	}
}

// ── Whitelist regression guard ───────────────────────────────────────────────

// Re-asserting WORK-009's edge-label whitelist: a `member_of` edge from a
// Task to a Task (instead of Project) must be rejected with
// ErrInvalidRelationship. This guards against accidental whitelist drift.
func TestMemberOf_NonProjectTarget_Rejected(t *testing.T) {
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
