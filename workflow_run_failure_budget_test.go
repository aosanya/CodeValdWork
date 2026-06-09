package codevaldwork_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// TestSetFailureBudget_RoundTrip locks the budget on a fresh top-level run
// and verifies the value (plus the auto-stamped root_workflow_run_id) round
// trips through GetWorkflowRun.
func TestSetFailureBudget_RoundTrip(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, err := mgr.CreateWorkflowRun(ctx, "ag", "", "pipeline.requested", "operator")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	if run.FailurePipelineBudget != 0 {
		t.Fatalf("expected zero budget on fresh run, got %d", run.FailurePipelineBudget)
	}

	updated, err := mgr.SetFailureBudget(ctx, "ag", run.ID, 5)
	if err != nil {
		t.Fatalf("SetFailureBudget: %v", err)
	}
	if updated.FailurePipelineBudget != 5 {
		t.Errorf("expected budget=5, got %d", updated.FailurePipelineBudget)
	}
	if updated.RootWorkflowRunID != run.ID {
		t.Errorf("expected root=%s, got %s", run.ID, updated.RootWorkflowRunID)
	}

	got, err := mgr.GetWorkflowRun(ctx, "ag", run.ID)
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	if got.FailurePipelineBudget != 5 || got.RootWorkflowRunID != run.ID {
		t.Errorf("round trip lost data: %+v", got)
	}
}

// TestSetFailureBudget_AlreadySet refuses to overwrite a locked budget.
func TestSetFailureBudget_AlreadySet(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	run, _ := mgr.CreateWorkflowRun(ctx, "ag", "", "", "")
	if _, err := mgr.SetFailureBudget(ctx, "ag", run.ID, 5); err != nil {
		t.Fatalf("first SetFailureBudget: %v", err)
	}
	_, err := mgr.SetFailureBudget(ctx, "ag", run.ID, 9)
	if !errors.Is(err, codevaldwork.ErrFailureBudgetAlreadySet) {
		t.Fatalf("expected ErrFailureBudgetAlreadySet, got %v", err)
	}
}

// TestSetFailureBudget_NonRootRejected refuses to set a budget on a recovery
// (child) run — budget lives only on the root.
func TestSetFailureBudget_NonRootRejected(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	root, _ := mgr.CreateWorkflowRun(ctx, "ag", "root-run", "", "")
	if _, err := mgr.SetFailureBudget(ctx, "ag", root.ID, 5); err != nil {
		t.Fatalf("SetFailureBudget root: %v", err)
	}
	child, err := mgr.CreateRecoveryWorkflowRun(ctx, "ag", "child-run", "pipeline.requested", "cross", root.ID, root.ID)
	if err != nil {
		t.Fatalf("CreateRecoveryWorkflowRun: %v", err)
	}
	_, err = mgr.SetFailureBudget(ctx, "ag", child.ID, 3)
	if !errors.Is(err, codevaldwork.ErrNotRootWorkflowRun) {
		t.Fatalf("expected ErrNotRootWorkflowRun, got %v", err)
	}
}

// TestIncrementFailureBudget_Increments increments the root counter and
// reports exhaustion correctly across calls.
func TestIncrementFailureBudget_Increments(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	root, _ := mgr.CreateWorkflowRun(ctx, "ag", "", "", "")
	if _, err := mgr.SetFailureBudget(ctx, "ag", root.ID, 2); err != nil {
		t.Fatalf("SetFailureBudget: %v", err)
	}

	used, budget, exhausted, err := mgr.IncrementFailureBudget(ctx, "ag", root.ID, "child-1")
	if err != nil || used != 1 || budget != 2 || exhausted {
		t.Fatalf("first increment: used=%d budget=%d exhausted=%v err=%v", used, budget, exhausted, err)
	}

	used, _, exhausted, err = mgr.IncrementFailureBudget(ctx, "ag", root.ID, "child-2")
	if err != nil || used != 2 || exhausted {
		t.Fatalf("second increment: used=%d exhausted=%v err=%v", used, exhausted, err)
	}

	// Third increment crosses the cap; counter is side-effected then exhausted.
	used, _, exhausted, err = mgr.IncrementFailureBudget(ctx, "ag", root.ID, "child-3")
	if err != nil || used != 3 || !exhausted {
		t.Fatalf("third increment: used=%d exhausted=%v err=%v", used, exhausted, err)
	}
}

// TestIncrementFailureBudget_Idempotent ensures a repeat call with the same
// child_run_id does not double-count.
func TestIncrementFailureBudget_Idempotent(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	root, _ := mgr.CreateWorkflowRun(ctx, "ag", "", "", "")
	if _, err := mgr.SetFailureBudget(ctx, "ag", root.ID, 5); err != nil {
		t.Fatalf("SetFailureBudget: %v", err)
	}
	if _, _, _, err := mgr.IncrementFailureBudget(ctx, "ag", root.ID, "child-1"); err != nil {
		t.Fatalf("first increment: %v", err)
	}

	used, budget, exhausted, err := mgr.IncrementFailureBudget(ctx, "ag", root.ID, "child-1")
	if err != nil {
		t.Fatalf("idempotent increment: %v", err)
	}
	if used != 1 || budget != 5 || exhausted {
		t.Fatalf("idempotent increment changed state: used=%d budget=%d exhausted=%v", used, budget, exhausted)
	}
}

// TestIncrementFailureBudget_ZeroBudgetNeverExhausts mirrors the spec —
// budget=0 means "unconfigured" and the gate stays open.
func TestIncrementFailureBudget_ZeroBudgetNeverExhausts(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	root, _ := mgr.CreateWorkflowRun(ctx, "ag", "", "", "")
	for i := 1; i <= 3; i++ {
		_, _, exhausted, err := mgr.IncrementFailureBudget(ctx, "ag", root.ID, "child-" + string(rune('a'-1+i)))
		if err != nil || exhausted {
			t.Fatalf("iter %d: exhausted=%v err=%v", i, exhausted, err)
		}
	}
}

// TestCreateRecoveryWorkflowRun_ChainsParentage stamps parent + root onto
// the new child run.
func TestCreateRecoveryWorkflowRun_ChainsParentage(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	root, _ := mgr.CreateWorkflowRun(ctx, "ag", "root", "", "")
	child, err := mgr.CreateRecoveryWorkflowRun(ctx, "ag", "recovery", "pipeline.requested", "cross", root.ID, root.ID)
	if err != nil {
		t.Fatalf("CreateRecoveryWorkflowRun: %v", err)
	}
	if child.ParentWorkflowRunID != root.ID {
		t.Errorf("parent: got %q, want %q", child.ParentWorkflowRunID, root.ID)
	}
	if child.RootWorkflowRunID != root.ID {
		t.Errorf("root: got %q, want %q", child.RootWorkflowRunID, root.ID)
	}
}

// TestCreateRecoveryWorkflowRun_DefaultsRootFromParent fills in the root
// when the caller omits it.
func TestCreateRecoveryWorkflowRun_DefaultsRootFromParent(t *testing.T) {
	t.Parallel()
	mgr, _ := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	ctx := context.Background()

	root, _ := mgr.CreateWorkflowRun(ctx, "ag", "root", "", "")
	if _, err := mgr.SetFailureBudget(ctx, "ag", root.ID, 5); err != nil {
		t.Fatalf("SetFailureBudget: %v", err)
	}
	child, err := mgr.CreateRecoveryWorkflowRun(ctx, "ag", "recovery", "", "", root.ID, "")
	if err != nil {
		t.Fatalf("CreateRecoveryWorkflowRun: %v", err)
	}
	if child.RootWorkflowRunID != root.ID {
		t.Errorf("default root: got %q, want %q", child.RootWorkflowRunID, root.ID)
	}
}
