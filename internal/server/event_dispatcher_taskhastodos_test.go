package server_test

import (
	"context"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdWork/internal/server"
)

// TestApplyAITaskStatus_DefersCompletionWhenParentHasTodos is the BUG-20260610-002
// Phase 2 regression test. A planner AgentRun completing must NOT flip the parent
// Task to COMPLETED — even when the payload's HasSubtasks flag is unset — when
// the parent has any has_todo edges. Completion in decompose mode is the
// responsibility of maybeCompleteParentTask, which aggregates over todos.
//
// Repro of the scenario 12 finding: assign MVP-SF-001 → planner-assigned-handler
// runs → planner AgentRun completes → applyAITaskStatus receives task.completed
// with HasSubtasks=false (the planner doesn't set it). Before this fix the
// parent flipped to COMPLETED with zero actual work product. The presence of
// any TaskTodo on the parent is the signal that completion belongs to the
// aggregator, not to this AgentRun.
func TestApplyAITaskStatus_DefersCompletionWhenParentHasTodos(t *testing.T) {
	mgr := newInMemoryManager()
	dispatcher := server.NewTaskEventDispatcher(mgr, "agency-1", nil)
	ctx := context.Background()

	// Seed a parent Task and transition it into IN_PROGRESS so it's eligible
	// for an AI completion event (the state machine guards every transition;
	// CreateTask always returns PENDING, regardless of the input Status).
	parent, err := mgr.CreateTask(ctx, "agency-1", codevaldwork.Task{
		Title: "Project Scaffolding",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	parent.Status = codevaldwork.TaskStatusInProgress
	if parent, err = mgr.UpdateTask(ctx, "agency-1", parent); err != nil {
		t.Fatalf("UpdateTask → IN_PROGRESS: %v", err)
	}

	// Attach exactly one TaskTodo via the has_todo edge — this is the gate
	// signal. The todo itself stays PENDING; we are NOT testing the aggregator.
	todo, err := mgr.CreateTaskTodo(ctx, "agency-1", codevaldwork.TaskTodo{
		Title:        "subtask",
		Instructions: "do the thing",
		ParentTaskID: parent.ID,
		Status:       codevaldwork.TodoStatusPending,
	})
	if err != nil {
		t.Fatalf("CreateTaskTodo: %v", err)
	}
	if _, err := mgr.CreateRelationship(ctx, "agency-1", codevaldwork.Relationship{
		Label:  codevaldwork.RelLabelHasTodo,
		FromID: parent.ID,
		ToID:   todo.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship has_todo: %v", err)
	}

	// Dispatch task.completed for the parent with HasSubtasks=false — the exact
	// shape today's planner AgentRuns emit.
	payload := `{"TaskID":"` + parent.ID + `","RunID":"planner-run-001","has_subtasks":false}`
	dispatcher.Dispatch(ctx, "task.completed", payload)

	// Assert the parent did NOT flip to COMPLETED.
	got, err := mgr.GetTask(ctx, "agency-1", parent.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status == codevaldwork.TaskStatusCompleted {
		t.Errorf("parent must NOT be COMPLETED when it has decomposed todos; got status=%q", got.Status)
	}
	if got.Status != codevaldwork.TaskStatusInProgress {
		t.Errorf("parent should remain IN_PROGRESS (the AgentRun.completed is not the parent's completion signal); got %q", got.Status)
	}
}

// TestApplyAITaskStatus_CompletesTrivialTaskWithNoTodos preserves the happy
// path: a single-shot task with no todos and HasSubtasks=false legitimately
// transitions to COMPLETED on AI task.completed. The new gate only suppresses
// transitions when at least one todo signals decompose mode.
func TestApplyAITaskStatus_CompletesTrivialTaskWithNoTodos(t *testing.T) {
	mgr := newInMemoryManager()
	dispatcher := server.NewTaskEventDispatcher(mgr, "agency-1", nil)
	ctx := context.Background()

	parent, err := mgr.CreateTask(ctx, "agency-1", codevaldwork.Task{
		Title: "Trivial single-shot task",
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	parent.Status = codevaldwork.TaskStatusInProgress
	if parent, err = mgr.UpdateTask(ctx, "agency-1", parent); err != nil {
		t.Fatalf("UpdateTask → IN_PROGRESS: %v", err)
	}

	// No has_todo edges seeded — trivial task, no decompose mode.
	payload := `{"TaskID":"` + parent.ID + `","RunID":"trivial-run-001","has_subtasks":false}`
	dispatcher.Dispatch(ctx, "task.completed", payload)

	got, err := mgr.GetTask(ctx, "agency-1", parent.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.Status != codevaldwork.TaskStatusCompleted {
		t.Errorf("trivial task should flip to COMPLETED when no todos exist; got %q", got.Status)
	}
}
