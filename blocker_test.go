package codevaldwork_test

import (
	"context"
	"errors"
	"sort"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// setBlocks wires source --[blocks]--> target inside the given agency.
func setBlocks(t *testing.T, mgr codevaldwork.TaskManager, agencyID, sourceID, targetID string) {
	t.Helper()
	if _, err := mgr.CreateRelationship(context.Background(), agencyID, codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: sourceID, ToID: targetID,
	}); err != nil {
		t.Fatalf("CreateRelationship blocks %s→%s: %v", sourceID, targetID, err)
	}
}

// startTask drives a freshly-created task pending → in_progress, returning
// any error from UpdateTask. Used to exercise the blocker gate.
func startTask(mgr codevaldwork.TaskManager, agencyID string, task codevaldwork.Task) error {
	task.Status = codevaldwork.TaskStatusInProgress
	_, err := mgr.UpdateTask(context.Background(), agencyID, task)
	return err
}

// completeBlocker drives a task pending → in_progress → completed. Used to
// stand up scenarios where a blocker has reached terminal state.
func completeBlocker(t *testing.T, mgr codevaldwork.TaskManager, agencyID string, task codevaldwork.Task) {
	t.Helper()
	task.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(context.Background(), agencyID, task); err != nil {
		t.Fatalf("blocker pending→in_progress: %v", err)
	}
	task.Status = codevaldwork.TaskStatusCompleted
	if _, err := mgr.UpdateTask(context.Background(), agencyID, task); err != nil {
		t.Fatalf("blocker in_progress→completed: %v", err)
	}
}

// cancelTask drives a task pending → cancelled.
func cancelTask(t *testing.T, mgr codevaldwork.TaskManager, agencyID string, task codevaldwork.Task) {
	t.Helper()
	task.Status = codevaldwork.TaskStatusCancelled
	if _, err := mgr.UpdateTask(context.Background(), agencyID, task); err != nil {
		t.Fatalf("pending→cancelled: %v", err)
	}
}

func TestUpdateTask_Blocked_PendingBlockerPreventsStart(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	setBlocks(t, mgr, "ag", a.ID, b.ID)

	err := startTask(mgr, "ag", b)
	if !errors.Is(err, codevaldwork.ErrBlocked) {
		t.Fatalf("want ErrBlocked, got %v", err)
	}
	var be *codevaldwork.BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("want *BlockedError via errors.As, got %T", err)
	}
	if len(be.BlockerTaskIDs) != 1 || be.BlockerTaskIDs[0] != a.ID {
		t.Errorf("BlockerTaskIDs = %v, want [%s]", be.BlockerTaskIDs, a.ID)
	}
}

func TestUpdateTask_Blocked_CompletedBlockerOpensGate(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	setBlocks(t, mgr, "ag", a.ID, b.ID)

	completeBlocker(t, mgr, "ag", a)
	if err := startTask(mgr, "ag", b); err != nil {
		t.Errorf("after blocker completed: got %v, want nil", err)
	}
}

func TestUpdateTask_Blocked_CancelledBlockerOpensGate(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	setBlocks(t, mgr, "ag", a.ID, b.ID)

	cancelTask(t, mgr, "ag", a)
	if err := startTask(mgr, "ag", b); err != nil {
		t.Errorf("after blocker cancelled: got %v, want nil", err)
	}
}

func TestUpdateTask_Blocked_MultipleBlockers_OnlyNonTerminalReported(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a1, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	a2, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	setBlocks(t, mgr, "ag", a1.ID, b.ID)
	setBlocks(t, mgr, "ag", a2.ID, b.ID)

	completeBlocker(t, mgr, "ag", a1) // a1 terminal, a2 still pending

	err := startTask(mgr, "ag", b)
	var be *codevaldwork.BlockedError
	if !errors.As(err, &be) {
		t.Fatalf("want *BlockedError, got %v", err)
	}
	if len(be.BlockerTaskIDs) != 1 {
		t.Fatalf("want 1 blocker, got %d (%v)", len(be.BlockerTaskIDs), be.BlockerTaskIDs)
	}
	if be.BlockerTaskIDs[0] != a2.ID {
		t.Errorf("blocker = %s, want %s (the still-pending one)", be.BlockerTaskIDs[0], a2.ID)
	}
}

func TestUpdateTask_Blocked_PendingToCancelledBypassesGate(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	setBlocks(t, mgr, "ag", a.ID, b.ID)

	// pending → cancelled must succeed even when the blocker is still pending.
	b.Status = codevaldwork.TaskStatusCancelled
	if _, err := mgr.UpdateTask(ctx, "ag", b); err != nil {
		t.Errorf("pending→cancelled with active blocker: got %v, want nil", err)
	}
}

func TestUpdateTask_Blocked_DependsOnIsNotGated(t *testing.T) {
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()
	a, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})
	b, _ := mgr.CreateTask(ctx, "ag", codevaldwork.Task{})

	// depends_on is informational — it must not block the transition.
	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label: codevaldwork.RelLabelDependsOn, FromID: b.ID, ToID: a.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship depends_on: %v", err)
	}
	_ = a

	if err := startTask(mgr, "ag", b); err != nil {
		t.Errorf("depends_on with non-terminal target: got %v, want nil", err)
	}
}

func TestBlockedError_IsAndAs(t *testing.T) {
	be := &codevaldwork.BlockedError{BlockerTaskIDs: []string{"x", "y"}}
	if !errors.Is(be, codevaldwork.ErrBlocked) {
		t.Error("errors.Is(BlockedError, ErrBlocked) = false, want true")
	}
	var got *codevaldwork.BlockedError
	if !errors.As(be, &got) {
		t.Error("errors.As did not extract *BlockedError")
	}
	want := []string{"x", "y"}
	gotIDs := append([]string(nil), got.BlockerTaskIDs...)
	sort.Strings(gotIDs)
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("ids: got %v, want %v", gotIDs, want)
			break
		}
	}
}
