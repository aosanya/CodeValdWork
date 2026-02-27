// Package arangodb_test provides integration tests for the ArangoDB backend.
//
// Tests in this file require a running ArangoDB instance. They are skipped
// automatically when WORK_ARANGO_ENDPOINT is not set.
//
// To run:
//
//	WORK_ARANGO_ENDPOINT=http://localhost:8529 go test -v -race ./storage/arangodb/
package arangodb_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdWork/storage/arangodb"
)

// newTestBackend skips the test if WORK_ARANGO_ENDPOINT is unset and returns
// a connected ArangoBackend pointing at a per-test database to ensure isolation.
func newTestBackend(t *testing.T) *arangodb.ArangoBackend {
	t.Helper()
	endpoint := os.Getenv("WORK_ARANGO_ENDPOINT")
	if endpoint == "" {
		t.Skip("WORK_ARANGO_ENDPOINT not set — skipping ArangoDB integration test")
	}

	// Use a short timestamp-based name to stay within ArangoDB's 64-char limit.
	dbName := fmt.Sprintf("wtest_%d", time.Now().UnixNano())
	b, err := arangodb.NewArangoBackend(arangodb.Config{
		Endpoint: endpoint,
		Username: envOrDefault("WORK_ARANGO_USER", "root"),
		Password: os.Getenv("WORK_ARANGO_PASSWORD"),
		Database: dbName,
	})
	if err != nil {
		t.Fatalf("NewArangoBackend: %v", err)
	}
	return b
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ── Create → Get round-trip ───────────────────────────────────────────────────

func TestArangoDB_CreateGet_RoundTrip(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	created, err := b.CreateTask(ctx, "agency-1", codevaldwork.Task{
		Title:       "Integration test task",
		Description: "Created by TestArangoDB_CreateGet_RoundTrip",
		Priority:    codevaldwork.TaskPriorityHigh,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID after creation")
	}
	if created.Status != codevaldwork.TaskStatusPending {
		t.Errorf("want status pending, got %s", created.Status)
	}

	got, err := b.GetTask(ctx, "agency-1", created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Title != created.Title {
		t.Errorf("title mismatch: want %q, got %q", created.Title, got.Title)
	}
	if got.Priority != codevaldwork.TaskPriorityHigh {
		t.Errorf("priority mismatch: want high, got %s", got.Priority)
	}
	if got.AgencyID != "agency-1" {
		t.Errorf("agency mismatch: want agency-1, got %s", got.AgencyID)
	}
}

// ── Create → Update (valid transition) ───────────────────────────────────────

func TestArangoDB_CreateUpdate_ValidTransition(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	created, err := b.CreateTask(ctx, "agency-1", codevaldwork.Task{
		Title: "Task to update",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	// Backend does not validate transitions — that is the taskManager's job.
	// We update directly and verify persistence.
	created.Status = codevaldwork.TaskStatusInProgress
	created.AssignedTo = "agent-007"

	updated, err := b.UpdateTask(ctx, "agency-1", created)
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Status != codevaldwork.TaskStatusInProgress {
		t.Errorf("want in_progress, got %s", updated.Status)
	}
	if updated.AssignedTo != "agent-007" {
		t.Errorf("want assigned_to=agent-007, got %s", updated.AssignedTo)
	}

	// Read back from DB to confirm persistence.
	got, err := b.GetTask(ctx, "agency-1", created.ID)
	if err != nil {
		t.Fatalf("GetTask after update: %v", err)
	}
	if got.Status != codevaldwork.TaskStatusInProgress {
		t.Errorf("persisted status: want in_progress, got %s", got.Status)
	}
}

// ── Create → Delete → Get (NOT_FOUND) ────────────────────────────────────────

func TestArangoDB_DeleteThenGet_NotFound(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	created, err := b.CreateTask(ctx, "agency-1", codevaldwork.Task{
		Title: "Soon deleted",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := b.DeleteTask(ctx, "agency-1", created.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	_, err = b.GetTask(ctx, "agency-1", created.ID)
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound after delete, got %v", err)
	}
}

// ── Get non-existent → ErrTaskNotFound ───────────────────────────────────────

func TestArangoDB_GetNonExistent_NotFound(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	_, err := b.GetTask(ctx, "agency-1", "does-not-exist")
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

// ── Create duplicate ID → ErrTaskAlreadyExists ───────────────────────────────

func TestArangoDB_DuplicateCreate_AlreadyExists(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	task := codevaldwork.Task{Title: "Original"}
	created, err := b.CreateTask(ctx, "agency-1", task)
	if err != nil {
		t.Fatalf("first CreateTask: %v", err)
	}

	// Force same key by setting ID on the second call.
	dup := codevaldwork.Task{ID: created.ID, Title: "Duplicate"}
	_, err = b.CreateTask(ctx, "agency-1", dup)
	if !errors.Is(err, codevaldwork.ErrTaskAlreadyExists) {
		t.Fatalf("want ErrTaskAlreadyExists, got %v", err)
	}
}

// ── List multiple tasks for same agency ───────────────────────────────────────

func TestArangoDB_ListTasks_SameAgency(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := b.CreateTask(ctx, "agency-1", codevaldwork.Task{
			Title: fmt.Sprintf("Task %d", i),
		})
		if err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	tasks, err := b.ListTasks(ctx, "agency-1", codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("want 3 tasks, got %d", len(tasks))
	}
}

// ── Agency isolation: ListTasks for agency A must not return agency B tasks ──

func TestArangoDB_ListTasks_AgencyIsolation(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	if _, err := b.CreateTask(ctx, "agency-A", codevaldwork.Task{Title: "A1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateTask(ctx, "agency-A", codevaldwork.Task{Title: "A2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateTask(ctx, "agency-B", codevaldwork.Task{Title: "B1"}); err != nil {
		t.Fatal(err)
	}

	tasksA, err := b.ListTasks(ctx, "agency-A", codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks agency-A: %v", err)
	}
	if len(tasksA) != 2 {
		t.Errorf("agency-A: want 2 tasks, got %d", len(tasksA))
	}
	for _, task := range tasksA {
		if task.AgencyID != "agency-A" {
			t.Errorf("agency-A list contains task from %s", task.AgencyID)
		}
	}

	tasksB, err := b.ListTasks(ctx, "agency-B", codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks agency-B: %v", err)
	}
	if len(tasksB) != 1 {
		t.Errorf("agency-B: want 1 task, got %d", len(tasksB))
	}
}

// ── ListTasks with status filter ─────────────────────────────────────────────

func TestArangoDB_ListTasks_FilterByStatus(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	// Create 3 tasks; promote 1 to in_progress.
	var taskIDs []string
	for i := 0; i < 3; i++ {
		created, err := b.CreateTask(ctx, "agency-1", codevaldwork.Task{
			Title: fmt.Sprintf("Task %d", i),
		})
		if err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
		taskIDs = append(taskIDs, created.ID)
	}

	// Directly update the first task to in_progress in the backend.
	first, err := b.GetTask(ctx, "agency-1", taskIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	first.Status = codevaldwork.TaskStatusInProgress
	if _, err := b.UpdateTask(ctx, "agency-1", first); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	pending, err := b.ListTasks(ctx, "agency-1", codevaldwork.TaskFilter{
		Status: codevaldwork.TaskStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("pending filter: want 2, got %d", len(pending))
	}

	inProgress, err := b.ListTasks(ctx, "agency-1", codevaldwork.TaskFilter{
		Status: codevaldwork.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inProgress) != 1 {
		t.Errorf("in_progress filter: want 1, got %d", len(inProgress))
	}
}

// ── ListTasks returns empty slice (not nil) when no matches ──────────────────

func TestArangoDB_ListTasks_EmptyResult(t *testing.T) {
	b := newTestBackend(t)
	ctx := context.Background()

	tasks, err := b.ListTasks(ctx, "empty-agency", codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if tasks == nil {
		t.Error("want empty slice, got nil")
	}
	if len(tasks) != 0 {
		t.Errorf("want 0 tasks, got %d", len(tasks))
	}
}
