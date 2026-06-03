package codevaldwork_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// helpers -------------------------------------------------------------------

func newManagerWithPublisher(t *testing.T) (codevaldwork.TaskManager, *recordingPublisher) {
	t.Helper()
	pub := &recordingPublisher{}
	mgr, err := codevaldwork.NewTaskManager(newFakeDataManager(), pub)
	if err != nil {
		t.Fatalf("NewTaskManager: %v", err)
	}
	return mgr, pub
}

func createRunAtStatus(t *testing.T, mgr codevaldwork.TaskManager, agencyID string, target codevaldwork.WorkflowRunStatus) codevaldwork.WorkflowRun {
	t.Helper()
	ctx := context.Background()
	run, err := mgr.CreateWorkflowRun(ctx, agencyID, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	transitions := map[codevaldwork.WorkflowRunStatus][]codevaldwork.WorkflowRunStatus{
		codevaldwork.WorkflowRunStatusInProgress: {codevaldwork.WorkflowRunStatusInProgress},
		codevaldwork.WorkflowRunStatusCompleted:  {codevaldwork.WorkflowRunStatusInProgress, codevaldwork.WorkflowRunStatusCompleted},
		codevaldwork.WorkflowRunStatusFailed:     {codevaldwork.WorkflowRunStatusInProgress, codevaldwork.WorkflowRunStatusFailed},
	}
	for _, s := range transitions[target] {
		run, err = mgr.UpdateWorkflowRunStatus(ctx, agencyID, run.ID, s, "")
		if err != nil {
			t.Fatalf("UpdateWorkflowRunStatus → %s: %v", s, err)
		}
	}
	return run
}

// ── RollbackWorkflowRun ────────────────────────────────────────────────────

func TestRollbackWorkflowRun_FailedRun_ReachesRolledBack(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, "ag", codevaldwork.WorkflowRunStatusFailed)

	result, err := mgr.RollbackWorkflowRun(ctx, "ag", run.ID, "test rollback")
	if err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}
	if result.Status != codevaldwork.WorkflowRunStatusRolledBack {
		t.Errorf("status = %s, want rolled_back", result.Status)
	}

	topics := pub.topicList()
	if !contains(topics, codevaldwork.TopicRunRollingBack) {
		t.Errorf("expected work.run.rolling_back event; got %v", topics)
	}
	if !contains(topics, codevaldwork.TopicRunRolledBack) {
		t.Errorf("expected work.run.rolled_back event; got %v", topics)
	}
}

func TestRollbackWorkflowRun_CompletedRun_ReachesRolledBack(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, "ag", codevaldwork.WorkflowRunStatusCompleted)

	result, err := mgr.RollbackWorkflowRun(ctx, "ag", run.ID, "undo completed run")
	if err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}
	if result.Status != codevaldwork.WorkflowRunStatusRolledBack {
		t.Errorf("status = %s, want rolled_back", result.Status)
	}
}

func TestRollbackWorkflowRun_PendingRun_ReturnsInvalidTransition(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run, err := mgr.CreateWorkflowRun(ctx, "ag", "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	_, err = mgr.RollbackWorkflowRun(ctx, "ag", run.ID, "")
	if !errors.Is(err, codevaldwork.ErrInvalidRunStatusTransition) {
		t.Errorf("err = %v, want ErrInvalidRunStatusTransition", err)
	}
}

func TestRollbackWorkflowRun_AlreadyRollingBack_ReturnsConflict(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, "ag", codevaldwork.WorkflowRunStatusFailed)

	// Manually put the run into rolling_back.
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, "ag", run.ID, codevaldwork.WorkflowRunStatusRollingBack, ""); err != nil {
		t.Fatalf("UpdateWorkflowRunStatus: %v", err)
	}

	_, err := mgr.RollbackWorkflowRun(ctx, "ag", run.ID, "")
	if !errors.Is(err, codevaldwork.ErrRollbackConflict) {
		t.Errorf("err = %v, want ErrRollbackConflict", err)
	}
}

func TestRollbackWorkflowRun_NotFound_ReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)

	_, err := mgr.RollbackWorkflowRun(ctx, "ag", "no-such-id", "")
	if !errors.Is(err, codevaldwork.ErrWorkflowRunNotFound) {
		t.Errorf("err = %v, want ErrWorkflowRunNotFound", err)
	}
}

// ── DeleteWorkflowRunArtifacts ─────────────────────────────────────────────

func TestDeleteWorkflowRunArtifacts_ResetsTasksToPendingAndEmitsEvents(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	const agencyID = "ag"

	run, err := mgr.CreateWorkflowRun(ctx, agencyID, "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	task, err := mgr.CreateTask(ctx, agencyID, codevaldwork.Task{
		Title:         "t1",
		WorkflowRunID: run.ID,
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := mgr.LinkTaskToRun(ctx, agencyID, run.ID, task.ID); err != nil {
		t.Fatalf("LinkTaskToRun: %v", err)
	}

	if err := mgr.DeleteWorkflowRunArtifacts(ctx, agencyID, run.ID); err != nil {
		t.Fatalf("DeleteWorkflowRunArtifacts: %v", err)
	}

	// Task must still exist, reset to pending with workflow_run_id cleared.
	after, err := mgr.GetTask(ctx, agencyID, task.ID)
	if err != nil {
		t.Fatalf("GetTask after rollback: %v", err)
	}
	if after.Status != codevaldwork.TaskStatusPending {
		t.Errorf("task status = %s, want pending", after.Status)
	}
	if after.WorkflowRunID != "" {
		t.Errorf("task.WorkflowRunID = %q, want empty", after.WorkflowRunID)
	}
	// work.task.rolled_back event must have fired.
	if !contains(pub.topicList(), codevaldwork.TopicTaskRolledBack) {
		t.Errorf("expected work.task.rolled_back event; got %v", pub.topicList())
	}
}

func TestDeleteWorkflowRunArtifacts_NoArtifacts_IsNoOp(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	run, err := mgr.CreateWorkflowRun(ctx, "ag", "", "", "")
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	if err := mgr.DeleteWorkflowRunArtifacts(ctx, "ag", run.ID); err != nil {
		t.Errorf("DeleteWorkflowRunArtifacts on empty run: %v", err)
	}
}

func TestDeleteWorkflowRunArtifacts_ForeignRunDependency_ReturnsError(t *testing.T) {
	ctx := context.Background()
	mgr, _ := newManagerWithPublisher(t)
	const agencyID = "ag"

	runA, _ := mgr.CreateWorkflowRun(ctx, agencyID, "run-a", "", "")
	runB, _ := mgr.CreateWorkflowRun(ctx, agencyID, "run-b", "", "")

	taskA, _ := mgr.CreateTask(ctx, agencyID, codevaldwork.Task{Title: "a", WorkflowRunID: runA.ID})
	taskB, _ := mgr.CreateTask(ctx, agencyID, codevaldwork.Task{Title: "b", WorkflowRunID: runB.ID})

	// taskB (run B) depends on taskA (run A).
	// Deleting run A's artifacts would break run B's dependency.
	if _, err := mgr.CreateRelationship(ctx, agencyID, codevaldwork.Relationship{
		Label:  codevaldwork.RelLabelDependsOn,
		FromID: taskB.ID,
		ToID:   taskA.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	err := mgr.DeleteWorkflowRunArtifacts(ctx, agencyID, runA.ID)
	if !errors.Is(err, codevaldwork.ErrForeignRunDependency) {
		t.Errorf("err = %v, want ErrForeignRunDependency", err)
	}
}

// ── RollbackWorkflowRun gRPC roundtrip ────────────────────────────────────

func TestRollbackWorkflowRun_PublishesRollingBackBeforeRolledBack(t *testing.T) {
	ctx := context.Background()
	mgr, pub := newManagerWithPublisher(t)
	run := createRunAtStatus(t, mgr, "ag", codevaldwork.WorkflowRunStatusFailed)

	if _, err := mgr.RollbackWorkflowRun(ctx, "ag", run.ID, "ordered events test"); err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}

	topics := pub.topicList()
	rollingIdx, rolledIdx := -1, -1
	for i, tp := range topics {
		if tp == codevaldwork.TopicRunRollingBack {
			rollingIdx = i
		}
		if tp == codevaldwork.TopicRunRolledBack {
			rolledIdx = i
		}
	}
	if rollingIdx < 0 || rolledIdx < 0 {
		t.Fatalf("missing events: rolling_back=%d rolled_back=%d in %v", rollingIdx, rolledIdx, topics)
	}
	if rollingIdx >= rolledIdx {
		t.Errorf("rolling_back (idx %d) must precede rolled_back (idx %d)", rollingIdx, rolledIdx)
	}
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (p *recordingPublisher) topicList() []string {
	out := make([]string, len(p.full))
	for i, e := range p.full {
		out[i] = e.Topic
	}
	return out
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
