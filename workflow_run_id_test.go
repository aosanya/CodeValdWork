// workflow_run_id_test.go — exercises the FEAT-20260602-002 propagation
// rules: persistence + edge write on create, list filter, chain-through on
// assign, mismatch rejection, and event payload propagation.
package codevaldwork_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

func TestCreateTask_WithWorkflowRunID_PersistsAndLinks(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, err := mgr.CreateWorkflowRun(ctx, "ag", "wfr-test", "next.requested", "tester")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	task, err := mgr.CreateTask(ctx, "ag", codevaldwork.Task{
		Title:         "t1",
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.WorkflowRunID != run.ID {
		t.Errorf("WorkflowRunID = %q, want %q", task.WorkflowRunID, run.ID)
	}

	// Re-read to confirm the property landed in storage, not just on the
	// returned struct.
	got, err := mgr.GetTask(ctx, "ag", task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.WorkflowRunID != run.ID {
		t.Errorf("re-read WorkflowRunID = %q, want %q", got.WorkflowRunID, run.ID)
	}

	// The started_task edge from run → task must exist after CreateTask.
	edges, err := mgr.TraverseRelationships(ctx, "ag", run.ID, codevaldwork.RelLabelStartedTask, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("traverse started_task: %v", err)
	}
	found := false
	for _, e := range edges {
		if e.ToID == task.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("started_task edge missing run=%s task=%s edges=%+v", run.ID, task.ID, edges)
	}
}

func TestCreateTask_WithoutWorkflowRunID_LeavesEmpty(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	task, err := mgr.CreateTask(context.Background(), "ag", codevaldwork.Task{Title: "t"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if task.WorkflowRunID != "" {
		t.Errorf("WorkflowRunID = %q, want empty", task.WorkflowRunID)
	}
}

func TestListTasks_WorkflowRunIDFilter_ReturnsOnlyMatches(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	runA, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-a", "", "")
	runB, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-b", "", "")

	a1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "a1", WorkflowRunID: runA.ID})
	_, _ = mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "a2", WorkflowRunID: runA.ID})
	_, _ = mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "b1", WorkflowRunID: runB.ID})
	_, _ = mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "free"})

	got, err := mgr.ListTasks(ctx, "ag", codevaldwork.TaskFilter{WorkflowRunID: runA.ID})
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filtered count = %d, want 2 (got %+v)", len(got), got)
	}
	for _, tk := range got {
		if tk.WorkflowRunID != runA.ID {
			t.Errorf("task %s has WorkflowRunID %q, want %q", tk.ID, tk.WorkflowRunID, runA.ID)
		}
	}

	// Sanity: unfiltered list returns all four.
	all, _ := mgr.ListTasks(ctx, "ag", codevaldwork.TaskFilter{})
	if len(all) != 4 {
		t.Errorf("unfiltered count = %d, want 4", len(all))
	}
	_ = a1 // silence unused (the ID could be used for ordering but is not asserted)
}

func TestAssignTask_InheritsRunIDWhenStoredEmpty(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-inherit", "", "")
	task, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "t"})
	agent, _ := mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "dev-01"})

	if err := mgr.AssignTask(ctx, "ag", task.ID, agent.ID, run.ID); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}

	got, _ := mgr.GetTask(ctx, "ag", task.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("Task.WorkflowRunID = %q, want %q (inherited)", got.WorkflowRunID, run.ID)
	}

	// started_task edge must have been written by AssignTask too.
	edges, _ := mgr.TraverseRelationships(ctx, "ag", run.ID, codevaldwork.RelLabelStartedTask, codevaldwork.DirectionOutbound)
	found := false
	for _, e := range edges {
		if e.ToID == task.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("AssignTask did not write started_task edge run=%s task=%s", run.ID, task.ID)
	}

	// work.task.assigned payload must carry the run-id.
	ev, ok := findEvent(pub.full, codevaldwork.TopicTaskAssigned)
	if !ok {
		t.Fatal("no work.task.assigned event")
	}
	p := ev.Payload.(codevaldwork.TaskAssignedPayload)
	if p.WorkflowRunID != run.ID {
		t.Errorf("TaskAssignedPayload.WorkflowRunID = %q, want %q", p.WorkflowRunID, run.ID)
	}
}

func TestAssignTask_SameRunID_IsIdempotent(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-same", "", "")
	task, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "t", WorkflowRunID: run.ID})
	agent, _ := mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "dev-01"})

	if err := mgr.AssignTask(ctx, "ag", task.ID, agent.ID, run.ID); err != nil {
		t.Fatalf("AssignTask with same run-id: %v", err)
	}
	got, _ := mgr.GetTask(ctx, "ag", task.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("WorkflowRunID drifted to %q", got.WorkflowRunID)
	}
}

func TestAssignTask_MismatchedRunID_ReturnsErrWorkflowRunMismatch(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	runA, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-A", "", "")
	runB, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-B", "", "")
	task, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "t", WorkflowRunID: runA.ID})
	agent, _ := mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "dev-01"})

	err := mgr.AssignTask(ctx, "ag", task.ID, agent.ID, runB.ID)
	if !errors.Is(err, codevaldwork.ErrWorkflowRunMismatch) {
		t.Fatalf("got %v, want ErrWorkflowRunMismatch", err)
	}

	got, _ := mgr.GetTask(ctx, "ag", task.ID)
	if got.WorkflowRunID != runA.ID {
		t.Errorf("WorkflowRunID changed despite rejected assign: got %q want %q", got.WorkflowRunID, runA.ID)
	}
}

func TestAssignTask_EmptyRunID_PreservesStored(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-pres", "", "")
	task, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "t", WorkflowRunID: run.ID})
	agent, _ := mgr.UpsertAgent(ctx, "ag", codevaldwork.Agent{AgentID: "dev-01"})

	if err := mgr.AssignTask(ctx, "ag", task.ID, agent.ID, ""); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
	got, _ := mgr.GetTask(ctx, "ag", task.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("WorkflowRunID = %q, want %q (preserved)", got.WorkflowRunID, run.ID)
	}
}

func TestCreateTaskTodo_PersistsWorkflowRunID(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-todo", "", "")
	parent, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "p", WorkflowRunID: run.ID})

	todo, err := mgr.CreateTaskTodo(ctx, "ag", codevaldwork.TaskTodo{
		Title:         "t",
		Instructions:  "do it",
		ParentTaskID:  parent.ID,
		Ordinality:    1,
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo: %v", err)
	}
	if todo.WorkflowRunID != run.ID {
		t.Errorf("todo WorkflowRunID = %q, want %q", todo.WorkflowRunID, run.ID)
	}

	got, _ := mgr.GetTaskTodo(ctx, "ag", todo.ID)
	if got.WorkflowRunID != run.ID {
		t.Errorf("re-read todo WorkflowRunID = %q, want %q", got.WorkflowRunID, run.ID)
	}
}

func TestTaskCreatedPayload_CarriesWorkflowRunID(t *testing.T) {
	pub := &recordingPublisher{}
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "ag", "wfr-ev", "", "")
	created, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "t", WorkflowRunID: run.ID})

	ev, ok := findEvent(pub.full, codevaldwork.TopicTaskCreated)
	if !ok {
		t.Fatal("no work.task.created event")
	}
	p := ev.Payload.(codevaldwork.TaskCreatedPayload)
	if p.TaskID != created.ID {
		t.Errorf("payload TaskID = %q, want %q", p.TaskID, created.ID)
	}
	if p.WorkflowRunID != run.ID {
		t.Errorf("payload WorkflowRunID = %q, want %q", p.WorkflowRunID, run.ID)
	}
}
