// Package arangodb_test provides integration tests for the ArangoDB backend.
//
// Tests in this file require a running ArangoDB instance. They connect to a
// single persistent database (WORK_ARANGO_DATABASE_TEST, default codevald_tests)
// and use unique agency IDs per test for isolation.
//
// Tests are skipped automatically when WORK_ARANGO_ENDPOINT is not set or the
// server is unreachable.
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

	driver "github.com/arangodb/go-driver"
	driverhttp "github.com/arangodb/go-driver/http"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdWork/storage/arangodb"
)

// openTestManager connects to the ArangoDB instance at WORK_ARANGO_ENDPOINT
// (default http://localhost:8529), opens WORK_ARANGO_DATABASE_TEST (default
// codevald_tests), constructs an entitygraph-backed Backend, and wraps it in
// a TaskManager. Skips the test if the server is unreachable.
func openTestManager(t *testing.T) codevaldwork.TaskManager {
	t.Helper()
	endpoint := os.Getenv("WORK_ARANGO_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8529"
	}

	conn, err := driverhttp.NewConnection(driverhttp.ConnectionConfig{
		Endpoints: []string{endpoint},
	})
	if err != nil {
		t.Skipf("ArangoDB connection config error (WORK_ARANGO_ENDPOINT=%s): %v", endpoint, err)
	}

	user := envOrDefault("WORK_ARANGO_USER", "root")
	pass := os.Getenv("WORK_ARANGO_PASSWORD")

	client, err := driver.NewClient(driver.ClientConfig{
		Connection:     conn,
		Authentication: driver.BasicAuthentication(user, pass),
	})
	if err != nil {
		t.Skipf("ArangoDB client error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.Version(ctx); err != nil {
		t.Skipf("ArangoDB unreachable at %s: %v", endpoint, err)
	}

	dbName := envOrDefault("WORK_ARANGO_DATABASE_TEST", "codevald_tests")
	ctx2 := context.Background()
	exists, err := client.DatabaseExists(ctx2, dbName)
	if err != nil {
		t.Fatalf("DatabaseExists: %v", err)
	}
	var db driver.Database
	if exists {
		db, err = client.Database(ctx2, dbName)
	} else {
		db, err = client.CreateDatabase(ctx2, dbName, nil)
	}
	if err != nil {
		t.Fatalf("open/create test database %q: %v", dbName, err)
	}

	backend, err := arangodb.NewBackendFromDB(db, codevaldwork.DefaultWorkSchema())
	if err != nil {
		t.Fatalf("NewBackendFromDB: %v", err)
	}
	mgr, err := codevaldwork.NewTaskManager(backend, nil)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr
}

// uniqueAgency returns a unique agency ID for test isolation.
func uniqueAgency(prefix string) string {
	return prefix + "-" + time.Now().Format("20060102T150405.000000")
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestArangoDB_CreateGet_RoundTrip(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("roundtrip")

	created, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{
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

	got, err := mgr.GetTask(ctx, agency, created.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Title != created.Title {
		t.Errorf("title mismatch: want %q, got %q", created.Title, got.Title)
	}
	if got.Priority != codevaldwork.TaskPriorityHigh {
		t.Errorf("priority mismatch: want high, got %s", got.Priority)
	}
	if got.AgencyID != agency {
		t.Errorf("agency mismatch: want %s, got %s", agency, got.AgencyID)
	}
}

func TestArangoDB_CreateUpdate_ValidTransition(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("update")

	created, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{
		Title: "Task to update",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	created.Status = codevaldwork.TaskStatusInProgress

	updated, err := mgr.UpdateTask(ctx, agency, created)
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if updated.Status != codevaldwork.TaskStatusInProgress {
		t.Errorf("want in_progress, got %s", updated.Status)
	}

	got, err := mgr.GetTask(ctx, agency, created.ID)
	if err != nil {
		t.Fatalf("GetTask after update: %v", err)
	}
	if got.Status != codevaldwork.TaskStatusInProgress {
		t.Errorf("persisted status: want in_progress, got %s", got.Status)
	}
}

func TestArangoDB_DeleteThenGet_NotFound(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("delete")

	created, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{
		Title: "Soon deleted",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := mgr.DeleteTask(ctx, agency, created.ID); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	_, err = mgr.GetTask(ctx, agency, created.ID)
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound after delete, got %v", err)
	}
}

func TestArangoDB_GetNonExistent_NotFound(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("notfound")

	_, err := mgr.GetTask(ctx, agency, "does-not-exist")
	if !errors.Is(err, codevaldwork.ErrTaskNotFound) {
		t.Fatalf("want ErrTaskNotFound, got %v", err)
	}
}

func TestArangoDB_ListTasks_SameAgency(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("listsame")

	for i := 0; i < 3; i++ {
		_, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{
			Title: fmt.Sprintf("Task %d", i),
		})
		if err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
	}

	tasks, err := mgr.ListTasks(ctx, agency, codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("want 3 tasks, got %d", len(tasks))
	}
}

func TestArangoDB_ListTasks_AgencyIsolation(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agencyA := uniqueAgency("isolA")
	agencyB := uniqueAgency("isolB")

	if _, err := mgr.CreateTask(ctx, agencyA, codevaldwork.Task{Title: "A1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateTask(ctx, agencyA, codevaldwork.Task{Title: "A2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateTask(ctx, agencyB, codevaldwork.Task{Title: "B1"}); err != nil {
		t.Fatal(err)
	}

	tasksA, err := mgr.ListTasks(ctx, agencyA, codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks agency-A: %v", err)
	}
	if len(tasksA) != 2 {
		t.Errorf("agency-A: want 2 tasks, got %d", len(tasksA))
	}
	for _, task := range tasksA {
		if task.AgencyID != agencyA {
			t.Errorf("agency-A list contains task from %s", task.AgencyID)
		}
	}

	tasksB, err := mgr.ListTasks(ctx, agencyB, codevaldwork.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks agency-B: %v", err)
	}
	if len(tasksB) != 1 {
		t.Errorf("agency-B: want 1 task, got %d", len(tasksB))
	}
}

func TestArangoDB_ListTasks_FilterByStatus(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("filterstatus")

	var taskIDs []string
	for i := 0; i < 3; i++ {
		created, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{
			Title: fmt.Sprintf("Task %d", i),
		})
		if err != nil {
			t.Fatalf("CreateTask %d: %v", i, err)
		}
		taskIDs = append(taskIDs, created.ID)
	}

	first, err := mgr.GetTask(ctx, agency, taskIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	first.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agency, first); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	pending, err := mgr.ListTasks(ctx, agency, codevaldwork.TaskFilter{
		Status: codevaldwork.TaskStatusPending,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 2 {
		t.Errorf("pending filter: want 2, got %d", len(pending))
	}

	inProgress, err := mgr.ListTasks(ctx, agency, codevaldwork.TaskFilter{
		Status: codevaldwork.TaskStatusInProgress,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inProgress) != 1 {
		t.Errorf("in_progress filter: want 1, got %d", len(inProgress))
	}
}

func TestArangoDB_ListTasks_EmptyResult(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("empty")

	tasks, err := mgr.ListTasks(ctx, agency, codevaldwork.TaskFilter{})
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

// TestArangoDB_Relationship_RoundTrip exercises the WORK-009 edge API end to
// end against a live ArangoDB: create two Tasks, connect them with a
// `blocks` edge, verify the edge surfaces via TraverseRelationships in both
// directions, and that DeleteRelationship removes it.
func TestArangoDB_Relationship_RoundTrip(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("rel")

	taskA, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "A"})
	if err != nil {
		t.Fatalf("CreateTask A: %v", err)
	}
	taskB, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "B"})
	if err != nil {
		t.Fatalf("CreateTask B: %v", err)
	}

	created, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
		Label:  codevaldwork.RelLabelBlocks,
		FromID: taskA.ID,
		ToID:   taskB.ID,
		Properties: map[string]any{
			"reason": "A must finish first",
		},
	})
	if err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created edge missing ID")
	}

	// Outbound traversal from A — should yield the A→B edge.
	outbound, err := mgr.TraverseRelationships(ctx, agency, taskA.ID, codevaldwork.RelLabelBlocks, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships outbound: %v", err)
	}
	if len(outbound) != 1 {
		t.Fatalf("want 1 outbound edge from A, got %d", len(outbound))
	}
	if outbound[0].FromID != taskA.ID || outbound[0].ToID != taskB.ID {
		t.Errorf("wrong edge: %+v", outbound[0])
	}

	// Inbound traversal on B — should also yield exactly one edge.
	inbound, err := mgr.TraverseRelationships(ctx, agency, taskB.ID, codevaldwork.RelLabelBlocks, codevaldwork.DirectionInbound)
	if err != nil {
		t.Fatalf("TraverseRelationships inbound: %v", err)
	}
	if len(inbound) != 1 {
		t.Fatalf("want 1 inbound edge on B, got %d", len(inbound))
	}

	// Idempotent re-create.
	again, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: taskA.ID, ToID: taskB.ID,
	})
	if err != nil {
		t.Fatalf("second CreateRelationship: %v", err)
	}
	if again.ID != created.ID {
		t.Errorf("idempotent re-create returned new edge: first=%s second=%s", created.ID, again.ID)
	}

	if err := mgr.DeleteRelationship(ctx, agency, taskA.ID, taskB.ID, codevaldwork.RelLabelBlocks); err != nil {
		t.Fatalf("DeleteRelationship: %v", err)
	}

	// After delete, traversal should yield no edges.
	after, err := mgr.TraverseRelationships(ctx, agency, taskA.ID, codevaldwork.RelLabelBlocks, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships after delete: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("want 0 edges after delete, got %d", len(after))
	}

	// Deleting again returns ErrRelationshipNotFound.
	if err := mgr.DeleteRelationship(ctx, agency, taskA.ID, taskB.ID, codevaldwork.RelLabelBlocks); !errors.Is(err, codevaldwork.ErrRelationshipNotFound) {
		t.Errorf("second DeleteRelationship: got %v, want ErrRelationshipNotFound", err)
	}
}
