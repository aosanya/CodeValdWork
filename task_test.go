package codevaldwork_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// ── NewTaskManager ───────────────────────────────────────────────────────────

func TestNewTaskManager_NilDataManager(t *testing.T) {
	_, err := codevaldwork.NewTaskManager(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil data manager, got nil")
	}
}

func TestNewTaskManager_ValidDataManager(t *testing.T) {
	mgr, err := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil TaskManager")
	}
}

// ── CreateTask ───────────────────────────────────────────────────────────────

func TestCreateTask_EmptyTitle(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.CreateTask(context.Background(), "agency-1", codevaldwork.Task{})
	if !errors.Is(err, codevaldwork.ErrInvalidTask) {
		t.Fatalf("want ErrInvalidTask, got %v", err)
	}
}

func TestCreateTask_Success(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	task, err := mgr.CreateTask(context.Background(), "agency-1", codevaldwork.Task{
		Title: "Research topic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.ID == "" {
		t.Errorf("expected server-generated ID, got empty")
	}
	if task.Title != "Research topic" {
		t.Errorf("want title %q, got %q", "Research topic", task.Title)
	}
	if task.Status != codevaldwork.TaskStatusPending {
		t.Errorf("want status pending, got %s", task.Status)
	}
	if task.Priority != codevaldwork.TaskPriorityMedium {
		t.Errorf("want default priority medium, got %s", task.Priority)
	}
	if task.AgencyID != "agency-1" {
		t.Errorf("want agencyID agency-1, got %s", task.AgencyID)
	}
}

func TestCreateTask_PublishesEvent(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	if _, err := mgr.CreateTask(context.Background(), "agency-1", codevaldwork.Task{
		Title: "Hello",
	}); err != nil {
		t.Fatal(err)
	}
	if len(pub.events) != 1 || pub.events[0] != "work.task.created|agency-1" {
		t.Errorf("expected work.task.created event, got %v", pub.events)
	}
}

// ── GetTask ──────────────────────────────────────────────────────────────────

func TestGetTask_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetTask(context.Background(), "agency-1", "nonexistent")
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestGetTask_Found(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "agency-1", codevaldwork.Task{
		Title: "Find me",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(context.Background(), "agency-1", created.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Find me" {
		t.Errorf("want title %q, got %q", "Find me", got.Title)
	}
}

// ── UpdateTask ───────────────────────────────────────────────────────────────

func TestUpdateTask_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.UpdateTask(context.Background(), "agency-1", codevaldwork.Task{
		ID: "nonexistent", Title: "x", Status: codevaldwork.TaskStatusInProgress,
	})
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestUpdateTask_InvalidTransition_PendingToCompleted(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = codevaldwork.TaskStatusCompleted
	_, err = mgr.UpdateTask(context.Background(), "a", created)
	if !errors.Is(err, codevaldwork.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

func TestUpdateTask_ValidTransition_PendingToInProgress(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = codevaldwork.TaskStatusInProgress
	updated, err := mgr.UpdateTask(context.Background(), "a", created)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != codevaldwork.TaskStatusInProgress {
		t.Errorf("want in_progress, got %s", updated.Status)
	}
}

func TestUpdateTask_ValidTransition_InProgressToCompleted(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	created, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "a", created); err != nil {
		t.Fatal(err)
	}
	created.Status = codevaldwork.TaskStatusCompleted
	updated, err := mgr.UpdateTask(context.Background(), "a", created)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != codevaldwork.TaskStatusCompleted {
		t.Errorf("want completed, got %s", updated.Status)
	}
	// Expect: created, updated (in_progress), completed.
	if len(pub.events) != 3 || pub.events[2] != "work.task.completed|a" {
		t.Errorf("expected work.task.completed last, got %v", pub.events)
	}
}

func TestUpdateTask_InvalidTransition_CompletedToPending(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{Title: "x"})
	if err != nil {
		t.Fatal(err)
	}
	created.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "a", created); err != nil {
		t.Fatal(err)
	}
	created.Status = codevaldwork.TaskStatusCompleted
	if _, err := mgr.UpdateTask(context.Background(), "a", created); err != nil {
		t.Fatal(err)
	}
	created.Status = codevaldwork.TaskStatusPending
	_, err = mgr.UpdateTask(context.Background(), "a", created)
	if !errors.Is(err, codevaldwork.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

// ── DeleteTask ───────────────────────────────────────────────────────────────

func TestDeleteTask_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	err := mgr.DeleteTask(context.Background(), "agency-1", "nonexistent")
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestDeleteTask_Success(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	created, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{Title: "Delete me"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteTask(context.Background(), "a", created.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = mgr.GetTask(context.Background(), "a", created.ID)
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound after delete, got %v", err)
	}
}

// ── ListTasks ────────────────────────────────────────────────────────────────

func TestListTasks_EmptyAgency(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	tasks, err := mgr.ListTasks(context.Background(), "agency-1", codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("want 0 tasks, got %d", len(tasks))
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	var ids []string
	for range 3 {
		created, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{Title: "x"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.ID)
	}
	first, err := mgr.GetTask(context.Background(), "a", ids[0])
	if err != nil {
		t.Fatal(err)
	}
	first.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), "a", first); err != nil {
		t.Fatal(err)
	}

	pending, err := mgr.ListTasks(context.Background(), "a", codevaldwork.TaskFilter{
		Status: codevaldwork.TaskStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("want 2 pending tasks, got %d", len(pending))
	}

	inProgress, err := mgr.ListTasks(context.Background(), "a", codevaldwork.TaskFilter{
		Status: codevaldwork.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inProgress) != 1 {
		t.Errorf("want 1 in_progress task, got %d", len(inProgress))
	}
}

func TestListTasks_AgencyIsolation(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	if _, err := mgr.CreateTask(context.Background(), "agency-A", codevaldwork.Task{Title: "Agency A task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateTask(context.Background(), "agency-B", codevaldwork.Task{Title: "Agency B task"}); err != nil {
		t.Fatal(err)
	}

	tasksA, err := mgr.ListTasks(context.Background(), "agency-A", codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasksA) != 1 {
		t.Errorf("agency-A: want 1 task, got %d", len(tasksA))
	}
	if tasksA[0].AgencyID != "agency-A" {
		t.Errorf("expected agency-A task, got agencyID=%s", tasksA[0].AgencyID)
	}
}

// ── TaskStatus.CanTransitionTo ───────────────────────────────────────────────

func TestCanTransitionTo(t *testing.T) {
	tests := []struct {
		from  codevaldwork.TaskStatus
		to    codevaldwork.TaskStatus
		allow bool
	}{
		{codevaldwork.TaskStatusPending, codevaldwork.TaskStatusInProgress, true},
		{codevaldwork.TaskStatusPending, codevaldwork.TaskStatusCancelled, true},
		{codevaldwork.TaskStatusPending, codevaldwork.TaskStatusCompleted, false},
		{codevaldwork.TaskStatusPending, codevaldwork.TaskStatusFailed, false},
		{codevaldwork.TaskStatusInProgress, codevaldwork.TaskStatusCompleted, true},
		{codevaldwork.TaskStatusInProgress, codevaldwork.TaskStatusFailed, true},
		{codevaldwork.TaskStatusInProgress, codevaldwork.TaskStatusCancelled, true},
		{codevaldwork.TaskStatusInProgress, codevaldwork.TaskStatusPending, false},
		{codevaldwork.TaskStatusCompleted, codevaldwork.TaskStatusPending, false},
		{codevaldwork.TaskStatusCompleted, codevaldwork.TaskStatusInProgress, false},
		{codevaldwork.TaskStatusFailed, codevaldwork.TaskStatusPending, false},
		{codevaldwork.TaskStatusCancelled, codevaldwork.TaskStatusPending, false},
	}
	for _, tc := range tests {
		got := tc.from.CanTransitionTo(tc.to)
		if got != tc.allow {
			t.Errorf("CanTransitionTo(%s → %s): want %v, got %v", tc.from, tc.to, tc.allow, got)
		}
	}
}
