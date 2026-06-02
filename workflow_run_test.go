package codevaldwork_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

func TestCreateWorkflowRun_DefaultsAndReadback(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, err := mgr.CreateWorkflowRun(ctx, "ag", "qa-scenario-09", "work.next.requested", "operator-1")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	if run.ID == "" {
		t.Fatal("created run missing ID")
	}
	if run.Name != "qa-scenario-09" {
		t.Errorf("name = %q want qa-scenario-09", run.Name)
	}
	if run.Status != codevaldwork.WorkflowRunStatusPending {
		t.Errorf("status = %q want pending", run.Status)
	}
	if run.TriggerEvent != "work.next.requested" || run.Initiator != "operator-1" {
		t.Errorf("unexpected run: %+v", run)
	}
	if run.StartedAt == "" || run.CreatedAt == "" {
		t.Errorf("timestamps not set: %+v", run)
	}

	got, err := mgr.GetWorkflowRun(ctx, "ag", run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	if got.ID != run.ID || got.TriggerEvent != run.TriggerEvent || got.Name != run.Name {
		t.Errorf("round-trip differs: created=%+v got=%+v", run, got)
	}
}

func TestCreateWorkflowRun_GeneratesName(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, err := mgr.CreateWorkflowRun(ctx, "ag", "", "work.next.requested", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	if !strings.HasPrefix(run.Name, "pipeline-") {
		t.Errorf("generated name = %q want pipeline-* prefix", run.Name)
	}
	// pipeline-YYYY-MM-DD-HHMMSS-<6hex> → 9 (pipeline-) + 19 (date+time) + 1 (-) + 6 (hex) = 35
	if got, want := len(run.Name), len("pipeline-2026-06-02-150412-a3f1c2"); got != want {
		t.Errorf("generated name length = %d want %d (got %q)", got, want, run.Name)
	}
}

func TestCreateWorkflowRun_DuplicateName_ReturnsExistsError(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	if _, err := mgr.CreateWorkflowRun(ctx, "ag", "qa-1", "trig", ""); err != nil {
		t.Fatalf("first CreateWorkflowRun: %v", err)
	}
	_, err := mgr.CreateWorkflowRun(ctx, "ag", "qa-1", "trig", "")
	if !errors.Is(err, codevaldwork.ErrWorkflowRunNameExists) {
		t.Fatalf("second CreateWorkflowRun err = %v want ErrWorkflowRunNameExists", err)
	}
}

func TestCreateWorkflowRun_DuplicateNameDifferentAgency_Allowed(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	if _, err := mgr.CreateWorkflowRun(ctx, "agency-A", "shared-name", "trig", ""); err != nil {
		t.Fatalf("agency-A CreateWorkflowRun: %v", err)
	}
	if _, err := mgr.CreateWorkflowRun(ctx, "agency-B", "shared-name", "trig", ""); err != nil {
		t.Errorf("agency-B CreateWorkflowRun (same name, different agency) should succeed: %v", err)
	}
}

func TestCreateWorkflowRun_LeadingWhitespace_Rejected(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.CreateWorkflowRun(context.Background(), "ag", " padded", "trig", "")
	if !errors.Is(err, codevaldwork.ErrInvalidTask) {
		t.Errorf("err = %v want ErrInvalidTask", err)
	}
}

func TestGetWorkflowRun_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetWorkflowRun(context.Background(), "ag", "missing-id")
	if !errors.Is(err, codevaldwork.ErrWorkflowRunNotFound) {
		t.Errorf("err = %v want ErrWorkflowRunNotFound", err)
	}
}

func TestGetWorkflowRunByName_RoundTrip(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	created, err := mgr.CreateWorkflowRun(ctx, "ag", "lookup-me", "trig", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	got, err := mgr.GetWorkflowRunByName(ctx, "ag", "lookup-me")
	if err != nil {
		t.Fatalf("GetWorkflowRunByName: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("got.ID = %q want %q", got.ID, created.ID)
	}
}

func TestGetWorkflowRunByName_NotFound(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	_, err := mgr.GetWorkflowRunByName(context.Background(), "ag", "no-such-run")
	if !errors.Is(err, codevaldwork.ErrWorkflowRunNotFound) {
		t.Errorf("err = %v want ErrWorkflowRunNotFound", err)
	}
}

func TestListWorkflowRuns_NewestFirst(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	for i, trig := range []string{"a", "b", "c"} {
		if _, err := mgr.CreateWorkflowRun(ctx, "ag", "", trig, ""); err != nil {
			t.Fatalf("CreateWorkflowRun %d: %v", i, err)
		}
	}
	runs, err := mgr.ListWorkflowRuns(ctx, "ag", "")
	if err != nil {
		t.Fatalf("ListWorkflowRuns: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	for i := 1; i < len(runs); i++ {
		if runs[i-1].CreatedAt < runs[i].CreatedAt {
			t.Errorf("runs not sorted newest first at %d: %s < %s",
				i, runs[i-1].CreatedAt, runs[i].CreatedAt)
		}
	}
}

func TestListWorkflowRuns_NameFilter(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	if _, err := mgr.CreateWorkflowRun(ctx, "ag", "match-me", "trig", ""); err != nil {
		t.Fatalf("CreateWorkflowRun match-me: %v", err)
	}
	if _, err := mgr.CreateWorkflowRun(ctx, "ag", "other", "trig", ""); err != nil {
		t.Fatalf("CreateWorkflowRun other: %v", err)
	}
	runs, err := mgr.ListWorkflowRuns(ctx, "ag", "match-me")
	if err != nil {
		t.Fatalf("ListWorkflowRuns filter: %v", err)
	}
	if len(runs) != 1 || runs[0].Name != "match-me" {
		t.Errorf("filtered runs = %+v want exactly one [match-me]", runs)
	}
}

func TestGetWorkflowRunClosure_TaskAndTodo(t *testing.T) {
	fake := newFakeDataManager()
	mgr, _ := codevaldwork.NewTaskManager(fake, nil)
	ctx := context.Background()

	run, err := mgr.CreateWorkflowRun(ctx, "ag", "", "work.next.requested", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	task, err := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "T1"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := mgr.LinkTaskToRun(ctx, "ag", run.ID, task.ID); err != nil {
		t.Fatalf("LinkTaskToRun: %v", err)
	}

	todo, err := mgr.CreateTaskTodo(ctx, "ag", codevaldwork.TaskTodo{
		Title:        "todo-1",
		Instructions: "do the thing",
		ParentTaskID: task.ID,
		Ordinality:   1,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo: %v", err)
	}
	if err := mgr.LinkTodoToRun(ctx, "ag", run.ID, todo.ID); err != nil {
		t.Fatalf("LinkTodoToRun: %v", err)
	}

	closure, err := mgr.GetWorkflowRunClosure(ctx, "ag", run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRunClosure: %v", err)
	}
	if closure.Run.ID != run.ID {
		t.Errorf("closure.Run.ID = %q want %q", closure.Run.ID, run.ID)
	}
	if len(closure.Tasks) != 1 || closure.Tasks[0].ID != task.ID {
		t.Errorf("closure.Tasks = %+v want one entry %q", closure.Tasks, task.ID)
	}
	if len(closure.Todos) != 1 || closure.Todos[0].ID != todo.ID {
		t.Errorf("closure.Todos = %+v want one entry %q", closure.Todos, todo.ID)
	}
	// At minimum the started_task and started_todo edges should be present.
	if len(closure.Edges) < 2 {
		t.Errorf("closure.Edges len = %d want >= 2 (started_task + started_todo)", len(closure.Edges))
	}
}

func TestLinkTaskToRun_Idempotent(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "ag", "", "trig", "")
	task, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "T"})

	if err := mgr.LinkTaskToRun(ctx, "ag", run.ID, task.ID); err != nil {
		t.Fatalf("first link: %v", err)
	}
	if err := mgr.LinkTaskToRun(ctx, "ag", run.ID, task.ID); err != nil {
		t.Fatalf("second link (should be no-op): %v", err)
	}

	closure, err := mgr.GetWorkflowRunClosure(ctx, "ag", run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRunClosure: %v", err)
	}
	startedTask := 0
	for _, e := range closure.Edges {
		if e.Label == codevaldwork.RelLabelStartedTask {
			startedTask++
		}
	}
	if startedTask != 1 {
		t.Errorf("expected exactly 1 started_task edge, got %d", startedTask)
	}
}
