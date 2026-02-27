package codevaldwork_test

import (
	"context"
	"errors"
	"testing"
	"time"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// ── Fake Backend ──────────────────────────────────────────────────────────────

// fakeBackend is an in-memory codevaldwork.Backend used for unit tests.
type fakeBackend struct {
	tasks map[string]codevaldwork.Task // key: agencyID+"/"+taskID
}

func newFakeBackend() *fakeBackend {
	return &fakeBackend{tasks: make(map[string]codevaldwork.Task)}
}

func (f *fakeBackend) key(agencyID, taskID string) string {
	return agencyID + "/" + taskID
}

func (f *fakeBackend) CreateTask(_ context.Context, agencyID string, task codevaldwork.Task) (codevaldwork.Task, error) {
	k := f.key(agencyID, task.ID)
	if _, exists := f.tasks[k]; exists {
		return codevaldwork.Task{}, codevaldwork.ErrTaskAlreadyExists
	}
	now := time.Now().UTC()
	task.AgencyID = agencyID
	task.Status = codevaldwork.TaskStatusPending
	task.CreatedAt = now
	task.UpdatedAt = now
	if task.Priority == "" {
		task.Priority = codevaldwork.TaskPriorityMedium
	}
	f.tasks[k] = task
	return task, nil
}

func (f *fakeBackend) GetTask(_ context.Context, agencyID, taskID string) (codevaldwork.Task, error) {
	t, ok := f.tasks[f.key(agencyID, taskID)]
	if !ok {
		return codevaldwork.Task{}, codevaldwork.ErrTaskNotFound
	}
	return t, nil
}

func (f *fakeBackend) UpdateTask(_ context.Context, agencyID string, task codevaldwork.Task) (codevaldwork.Task, error) {
	k := f.key(agencyID, task.ID)
	if _, ok := f.tasks[k]; !ok {
		return codevaldwork.Task{}, codevaldwork.ErrTaskNotFound
	}
	task.AgencyID = agencyID
	task.UpdatedAt = time.Now().UTC()
	f.tasks[k] = task
	return task, nil
}

func (f *fakeBackend) DeleteTask(_ context.Context, agencyID, taskID string) error {
	k := f.key(agencyID, taskID)
	if _, ok := f.tasks[k]; !ok {
		return codevaldwork.ErrTaskNotFound
	}
	delete(f.tasks, k)
	return nil
}

func (f *fakeBackend) ListTasks(_ context.Context, agencyID string, filter codevaldwork.TaskFilter) ([]codevaldwork.Task, error) {
	var out []codevaldwork.Task
	for _, t := range f.tasks {
		if t.AgencyID != agencyID {
			continue
		}
		if filter.Status != "" && t.Status != filter.Status {
			continue
		}
		if filter.Priority != "" && t.Priority != filter.Priority {
			continue
		}
		if filter.AssignedTo != "" && t.AssignedTo != filter.AssignedTo {
			continue
		}
		out = append(out, t)
	}
	if out == nil {
		out = []codevaldwork.Task{}
	}
	return out, nil
}

// ── NewTaskManager ────────────────────────────────────────────────────────────

func TestNewTaskManager_NilBackend(t *testing.T) {
	_, err := codevaldwork.NewTaskManager(nil)
	if err == nil {
		t.Fatal("expected error for nil backend, got nil")
	}
}

func TestNewTaskManager_ValidBackend(t *testing.T) {
	mgr, err := codevaldwork.NewTaskManager(newFakeBackend())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil TaskManager")
	}
}

// ── CreateTask ────────────────────────────────────────────────────────────────

func TestCreateTask_EmptyTitle(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	_, err := mgr.CreateTask(context.Background(), "agency-1", codevaldwork.Task{
		ID: "t-1",
	})
	if !errors.Is(err, codevaldwork.ErrInvalidTask) {
		t.Fatalf("want ErrInvalidTask, got %v", err)
	}
}

func TestCreateTask_Success(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	task, err := mgr.CreateTask(context.Background(), "agency-1", codevaldwork.Task{
		ID:    "t-1",
		Title: "Research topic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Title != "Research topic" {
		t.Errorf("want title %q, got %q", "Research topic", task.Title)
	}
	if task.Status != codevaldwork.TaskStatusPending {
		t.Errorf("want status pending, got %s", task.Status)
	}
	if task.AgencyID != "agency-1" {
		t.Errorf("want agencyID agency-1, got %s", task.AgencyID)
	}
}

func TestCreateTask_DuplicateID(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	task := codevaldwork.Task{ID: "t-1", Title: "First"}
	if _, err := mgr.CreateTask(context.Background(), "agency-1", task); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := mgr.CreateTask(context.Background(), "agency-1", task)
	if !errors.Is(err, codevaldwork.ErrTaskAlreadyExists) {
		t.Fatalf("want ErrTaskAlreadyExists, got %v", err)
	}
}

// ── GetTask ───────────────────────────────────────────────────────────────────

func TestGetTask_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	_, err := mgr.GetTask(context.Background(), "agency-1", "nonexistent")
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestGetTask_Found(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	if _, err := mgr.CreateTask(context.Background(), "agency-1", codevaldwork.Task{
		ID: "t-1", Title: "Find me",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := mgr.GetTask(context.Background(), "agency-1", "t-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Title != "Find me" {
		t.Errorf("want title %q, got %q", "Find me", got.Title)
	}
}

// ── UpdateTask ────────────────────────────────────────────────────────────────

func TestUpdateTask_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	_, err := mgr.UpdateTask(context.Background(), "agency-1", codevaldwork.Task{
		ID: "nonexistent", Title: "x", Status: codevaldwork.TaskStatusInProgress,
	})
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestUpdateTask_InvalidTransition_PendingToCompleted(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	if _, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x",
	}); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x", Status: codevaldwork.TaskStatusCompleted,
	})
	if !errors.Is(err, codevaldwork.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

func TestUpdateTask_ValidTransition_PendingToInProgress(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	if _, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x",
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x", Status: codevaldwork.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != codevaldwork.TaskStatusInProgress {
		t.Errorf("want in_progress, got %s", updated.Status)
	}
}

func TestUpdateTask_ValidTransition_InProgressToCompleted(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	if _, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x", Status: codevaldwork.TaskStatusInProgress,
	}); err != nil {
		t.Fatal(err)
	}
	updated, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x", Status: codevaldwork.TaskStatusCompleted,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Status != codevaldwork.TaskStatusCompleted {
		t.Errorf("want completed, got %s", updated.Status)
	}
}

func TestUpdateTask_InvalidTransition_CompletedToPending(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	if _, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x", Status: codevaldwork.TaskStatusInProgress,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x", Status: codevaldwork.TaskStatusCompleted,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "x", Status: codevaldwork.TaskStatusPending,
	})
	if !errors.Is(err, codevaldwork.ErrInvalidStatusTransition) {
		t.Fatalf("want ErrInvalidStatusTransition, got %v", err)
	}
}

// ── DeleteTask ────────────────────────────────────────────────────────────────

func TestDeleteTask_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	err := mgr.DeleteTask(context.Background(), "agency-1", "nonexistent")
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestDeleteTask_Success(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	if _, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "Delete me",
	}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.DeleteTask(context.Background(), "a", "t-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := mgr.GetTask(context.Background(), "a", "t-1")
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound after delete, got %v", err)
	}
}

// ── ListTasks ─────────────────────────────────────────────────────────────────

func TestListTasks_EmptyAgency(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	tasks, err := mgr.ListTasks(context.Background(), "agency-1", codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("want 0 tasks, got %d", len(tasks))
	}
}

func TestListTasks_FilterByStatus(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	for _, id := range []string{"t-1", "t-2", "t-3"} {
		if _, err := mgr.CreateTask(context.Background(), "a", codevaldwork.Task{
			ID: id, Title: id,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Move t-1 to in_progress.
	if _, err := mgr.UpdateTask(context.Background(), "a", codevaldwork.Task{
		ID: "t-1", Title: "t-1", Status: codevaldwork.TaskStatusInProgress,
	}); err != nil {
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
	mgr, _ := codevaldwork.NewTaskManager(newFakeBackend())
	if _, err := mgr.CreateTask(context.Background(), "agency-A", codevaldwork.Task{
		ID: "t-1", Title: "Agency A task",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateTask(context.Background(), "agency-B", codevaldwork.Task{
		ID: "t-2", Title: "Agency B task",
	}); err != nil {
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

// ── TaskStatus.CanTransitionTo ────────────────────────────────────────────────

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
