package codevaldwork_test

import (
	"context"
	"errors"
	"testing"
	"time"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// ── CancelWorkflowRun (FEAT-20260602-008) ─────────────────────────────────

func TestCancelWorkflowRun_InProgress_ReachesCancelling(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	const agencyID = "ag"

	run := createRunAtStatus(t, mgr, agencyID, codevaldwork.WorkflowRunStatusInProgress)
	deadline := time.Now().UTC().Add(30 * time.Second)

	result, err := mgr.CancelWorkflowRun(ctx, agencyID, run.ID, "compile loop runaway", "alice", deadline)
	if err != nil {
		t.Fatalf("CancelWorkflowRun: %v", err)
	}
	if result.Status != codevaldwork.WorkflowRunStatusCancelling {
		t.Errorf("status = %s, want cancelling", result.Status)
	}
	if result.CancelledBy != "alice" {
		t.Errorf("CancelledBy = %q, want %q", result.CancelledBy, "alice")
	}
	if result.CancelReason != "compile loop runaway" {
		t.Errorf("CancelReason = %q, want %q", result.CancelReason, "compile loop runaway")
	}
	if result.CancellingUntil == "" {
		t.Error("CancellingUntil empty; want RFC3339 deadline")
	}
	if !contains(pub.topicList(), codevaldwork.TopicRunCancelling) {
		t.Errorf("expected work.run.cancelling event; got %v", pub.topicList())
	}
}

func TestCancelWorkflowRun_CascadesTaskCancelledForNonTerminalTasks(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	const agencyID = "ag"

	run := createRunAtStatus(t, mgr, agencyID, codevaldwork.WorkflowRunStatusInProgress)

	// Active task — should be cancelled.
	active, err := mgr.CreateTask(ctx, agencyID, codevaldwork.Task{
		Title:         "active",
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask active: %v", err)
	}
	// Already-terminal task — should be skipped.
	done, err := mgr.CreateTask(ctx, agencyID, codevaldwork.Task{
		Title:         "done",
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask done: %v", err)
	}
	done.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agencyID, done); err != nil {
		t.Fatalf("UpdateTask done → in_progress: %v", err)
	}
	doneInProgress, err := mgr.GetTask(ctx, agencyID, done.ID)
	if err != nil {
		t.Fatalf("GetTask done: %v", err)
	}
	doneInProgress.Status = codevaldwork.TaskStatusCompleted
	if _, err := mgr.UpdateTask(ctx, agencyID, doneInProgress); err != nil {
		t.Fatalf("UpdateTask done → completed: %v", err)
	}

	if _, err := mgr.CancelWorkflowRun(ctx, agencyID, run.ID, "test", "alice", time.Now().Add(30*time.Second)); err != nil {
		t.Fatalf("CancelWorkflowRun: %v", err)
	}

	got, err := mgr.GetTask(ctx, agencyID, active.ID)
	if err != nil {
		t.Fatalf("GetTask active: %v", err)
	}
	if got.Status != codevaldwork.TaskStatusCancelled {
		t.Errorf("active task status = %s, want cancelled", got.Status)
	}

	gotDone, err := mgr.GetTask(ctx, agencyID, done.ID)
	if err != nil {
		t.Fatalf("GetTask done: %v", err)
	}
	if gotDone.Status != codevaldwork.TaskStatusCompleted {
		t.Errorf("terminal task status = %s, want completed (untouched)", gotDone.Status)
	}

	// Exactly one work.task.cancelled event for the active task.
	var cancelledCount int
	for _, ev := range pub.full {
		if ev.Topic == codevaldwork.TopicTaskCancelled {
			cancelledCount++
			payload, ok := ev.Payload.(codevaldwork.TaskCancelledPayload)
			if !ok {
				t.Fatalf("payload type = %T, want TaskCancelledPayload", ev.Payload)
			}
			if payload.TaskID != active.ID {
				t.Errorf("payload TaskID = %q, want %q", payload.TaskID, active.ID)
			}
			if payload.WorkflowRunID != run.ID {
				t.Errorf("payload WorkflowRunID = %q, want %q", payload.WorkflowRunID, run.ID)
			}
		}
	}
	if cancelledCount != 1 {
		t.Errorf("work.task.cancelled fired %d times, want 1", cancelledCount)
	}
}

func TestCancelWorkflowRun_AlreadyCancelling_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	const agencyID = "ag"

	run := createRunAtStatus(t, mgr, agencyID, codevaldwork.WorkflowRunStatusInProgress)
	deadline := time.Now().UTC().Add(30 * time.Second)

	first, err := mgr.CancelWorkflowRun(ctx, agencyID, run.ID, "first", "alice", deadline)
	if err != nil {
		t.Fatalf("first cancel: %v", err)
	}

	priorEventCount := len(pub.full)

	second, err := mgr.CancelWorkflowRun(ctx, agencyID, run.ID, "second", "bob", deadline.Add(10*time.Second))
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if second.Status != codevaldwork.WorkflowRunStatusCancelling {
		t.Errorf("second.Status = %s, want cancelling", second.Status)
	}
	// Envelope is preserved from the first call — no overwrite.
	if second.CancelReason != first.CancelReason {
		t.Errorf("CancelReason changed: %q → %q", first.CancelReason, second.CancelReason)
	}
	if second.CancelledBy != first.CancelledBy {
		t.Errorf("CancelledBy changed: %q → %q", first.CancelledBy, second.CancelledBy)
	}
	if second.CancellingUntil != first.CancellingUntil {
		t.Errorf("CancellingUntil shifted: %q → %q", first.CancellingUntil, second.CancellingUntil)
	}
	// No new events fired.
	if len(pub.full) != priorEventCount {
		t.Errorf("second cancel published %d new event(s); want 0", len(pub.full)-priorEventCount)
	}
}

func TestCancelWorkflowRun_PendingRun_ReturnsCannotCancelTerminal(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run, err := mgr.CreateWorkflowRun(ctx, "ag", "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	_, err = mgr.CancelWorkflowRun(ctx, "ag", run.ID, "", "", time.Now().Add(time.Second))
	if !errors.Is(err, codevaldwork.ErrCannotCancelTerminalRun) {
		t.Errorf("err = %v, want ErrCannotCancelTerminalRun", err)
	}
}

func TestCancelWorkflowRun_CompletedRun_ReturnsCannotCancelTerminal(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, "ag", codevaldwork.WorkflowRunStatusCompleted)

	_, err := mgr.CancelWorkflowRun(ctx, "ag", run.ID, "", "", time.Now().Add(time.Second))
	if !errors.Is(err, codevaldwork.ErrCannotCancelTerminalRun) {
		t.Errorf("err = %v, want ErrCannotCancelTerminalRun", err)
	}
}

func TestCancelWorkflowRun_FailedRun_ReturnsCannotCancelTerminal(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, "ag", codevaldwork.WorkflowRunStatusFailed)

	_, err := mgr.CancelWorkflowRun(ctx, "ag", run.ID, "", "", time.Now().Add(time.Second))
	if !errors.Is(err, codevaldwork.ErrCannotCancelTerminalRun) {
		t.Errorf("err = %v, want ErrCannotCancelTerminalRun", err)
	}
}

func TestCancelWorkflowRun_MissingRun_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	_, err := mgr.CancelWorkflowRun(ctx, "ag", "nope", "", "", time.Now().Add(time.Second))
	if !errors.Is(err, codevaldwork.ErrWorkflowRunNotFound) {
		t.Errorf("err = %v, want ErrWorkflowRunNotFound", err)
	}
}

// ── FinalizeWorkflowRunCancellation ───────────────────────────────────────

func TestFinalizeWorkflowRunCancellation_CancellingRun_ReachesCancelled(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	const agencyID = "ag"

	run := createRunAtStatus(t, mgr, agencyID, codevaldwork.WorkflowRunStatusInProgress)
	if _, err := mgr.CancelWorkflowRun(ctx, agencyID, run.ID, "quiesce", "alice", time.Now().Add(time.Second)); err != nil {
		t.Fatalf("CancelWorkflowRun: %v", err)
	}

	result, err := mgr.FinalizeWorkflowRunCancellation(ctx, agencyID, run.ID)
	if err != nil {
		t.Fatalf("FinalizeWorkflowRunCancellation: %v", err)
	}
	if result.Status != codevaldwork.WorkflowRunStatusCancelled {
		t.Errorf("status = %s, want cancelled", result.Status)
	}
	if result.CompletedAt == "" {
		t.Error("CompletedAt empty after finalize")
	}
	if !contains(pub.topicList(), codevaldwork.TopicRunCancelled) {
		t.Errorf("expected work.run.cancelled event; got %v", pub.topicList())
	}
}

func TestFinalizeWorkflowRunCancellation_NotCancelling_NoOp(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	const agencyID = "ag"

	// Run is in_progress, never cancelled.
	run := createRunAtStatus(t, mgr, agencyID, codevaldwork.WorkflowRunStatusInProgress)
	priorEventCount := len(pub.full)

	result, err := mgr.FinalizeWorkflowRunCancellation(ctx, agencyID, run.ID)
	if err != nil {
		t.Fatalf("FinalizeWorkflowRunCancellation: %v", err)
	}
	if result.Status != codevaldwork.WorkflowRunStatusInProgress {
		t.Errorf("status = %s, want in_progress (untouched)", result.Status)
	}
	if len(pub.full) != priorEventCount {
		t.Errorf("finalize on non-cancelling run published %d event(s); want 0", len(pub.full)-priorEventCount)
	}
}

func TestFinalizeWorkflowRunCancellation_DoubleFinalize_IsIdempotent(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	const agencyID = "ag"

	run := createRunAtStatus(t, mgr, agencyID, codevaldwork.WorkflowRunStatusInProgress)
	if _, err := mgr.CancelWorkflowRun(ctx, agencyID, run.ID, "", "", time.Now().Add(time.Second)); err != nil {
		t.Fatalf("CancelWorkflowRun: %v", err)
	}
	if _, err := mgr.FinalizeWorkflowRunCancellation(ctx, agencyID, run.ID); err != nil {
		t.Fatalf("first finalize: %v", err)
	}
	priorEventCount := len(pub.full)

	result, err := mgr.FinalizeWorkflowRunCancellation(ctx, agencyID, run.ID)
	if err != nil {
		t.Fatalf("second finalize: %v", err)
	}
	if result.Status != codevaldwork.WorkflowRunStatusCancelled {
		t.Errorf("status = %s, want cancelled", result.Status)
	}
	if len(pub.full) != priorEventCount {
		t.Errorf("second finalize published %d extra event(s); want 0", len(pub.full)-priorEventCount)
	}
}

// ── State-machine guard ───────────────────────────────────────────────────

func TestWorkflowRunStatus_InProgress_CanTransitionToCancelling(t *testing.T) {
	if !codevaldwork.WorkflowRunStatusInProgress.CanTransitionTo(codevaldwork.WorkflowRunStatusCancelling) {
		t.Error("in_progress → cancelling should be allowed")
	}
}

func TestWorkflowRunStatus_Cancelling_CanTransitionToCancelled(t *testing.T) {
	if !codevaldwork.WorkflowRunStatusCancelling.CanTransitionTo(codevaldwork.WorkflowRunStatusCancelled) {
		t.Error("cancelling → cancelled should be allowed")
	}
}

func TestWorkflowRunStatus_Cancelled_CanTransitionToRollingBack(t *testing.T) {
	if !codevaldwork.WorkflowRunStatusCancelled.CanTransitionTo(codevaldwork.WorkflowRunStatusRollingBack) {
		t.Error("cancelled → rolling_back should be allowed (cancel → optional rollback flow)")
	}
}

func TestWorkflowRunStatus_Pending_CannotTransitionToCancelling(t *testing.T) {
	if codevaldwork.WorkflowRunStatusPending.CanTransitionTo(codevaldwork.WorkflowRunStatusCancelling) {
		t.Error("pending → cancelling should be rejected")
	}
}
