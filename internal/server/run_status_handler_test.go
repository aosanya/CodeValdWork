package server_test

import (
	"context"
	"encoding/json"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdWork/internal/server"
)

// ── matchesTerminalEvent helper ───────────────────────────────────────────────
// matchesTerminalEvent is an unexported function in the server package; we test
// it indirectly through RunStatusHandler.HandleEvent below.

// ── stubMgrForHandler is a minimal TaskManager stub ──────────────────────────

type stubMgrForHandler struct {
	codevaldwork.TaskManager                    // embed to satisfy interface
	getRunFn    func(agencyID, runID string) (codevaldwork.WorkflowRun, error)
	updateRunFn func(agencyID, runID string, s codevaldwork.WorkflowRunStatus, reason string) (codevaldwork.WorkflowRun, error)
	lastStatus  codevaldwork.WorkflowRunStatus
	lastReason  string
	called      bool
}

func (s *stubMgrForHandler) GetWorkflowRun(_ context.Context, agencyID, runID string) (codevaldwork.WorkflowRun, error) {
	return s.getRunFn(agencyID, runID)
}

func (s *stubMgrForHandler) UpdateWorkflowRunStatus(_ context.Context, agencyID, runID string, status codevaldwork.WorkflowRunStatus, reason string) (codevaldwork.WorkflowRun, error) {
	s.called = true
	s.lastStatus = status
	s.lastReason = reason
	return s.updateRunFn(agencyID, runID, status, reason)
}

func payload(t *testing.T, m map[string]any) string {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// ── HandleEvent transitions ───────────────────────────────────────────────────

func TestRunStatusHandler_PendingToInProgress(t *testing.T) {
	run := codevaldwork.WorkflowRun{ID: "r1", Status: codevaldwork.WorkflowRunStatusPending}
	stub := &stubMgrForHandler{
		getRunFn: func(_, _ string) (codevaldwork.WorkflowRun, error) { return run, nil },
		updateRunFn: func(_, _ string, s codevaldwork.WorkflowRunStatus, _ string) (codevaldwork.WorkflowRun, error) {
			return codevaldwork.WorkflowRun{Status: s}, nil
		},
	}
	h := server.NewRunStatusHandler(stub, "ag")
	h.HandleEvent(context.Background(), codevaldwork.TopicTaskAssigned,
		payload(t, map[string]any{"workflow_run_id": "r1"}))

	if !stub.called {
		t.Fatal("UpdateWorkflowRunStatus not called")
	}
	if stub.lastStatus != codevaldwork.WorkflowRunStatusInProgress {
		t.Errorf("status = %v, want in_progress", stub.lastStatus)
	}
}

func TestRunStatusHandler_InProgressToFailed_TaskFailed(t *testing.T) {
	run := codevaldwork.WorkflowRun{ID: "r2", Status: codevaldwork.WorkflowRunStatusInProgress}
	stub := &stubMgrForHandler{
		getRunFn: func(_, _ string) (codevaldwork.WorkflowRun, error) { return run, nil },
		updateRunFn: func(_, _ string, s codevaldwork.WorkflowRunStatus, _ string) (codevaldwork.WorkflowRun, error) {
			return codevaldwork.WorkflowRun{Status: s}, nil
		},
	}
	h := server.NewRunStatusHandler(stub, "ag")
	h.HandleEvent(context.Background(), codevaldwork.TopicTaskFailed,
		payload(t, map[string]any{"workflow_run_id": "r2", "reason": "timeout"}))

	if !stub.called {
		t.Fatal("UpdateWorkflowRunStatus not called")
	}
	if stub.lastStatus != codevaldwork.WorkflowRunStatusFailed {
		t.Errorf("status = %v, want failed", stub.lastStatus)
	}
	if stub.lastReason != "timeout" {
		t.Errorf("reason = %q, want timeout", stub.lastReason)
	}
}

func TestRunStatusHandler_InProgressToFailed_FunctionsJobFailed(t *testing.T) {
	run := codevaldwork.WorkflowRun{ID: "r3", Status: codevaldwork.WorkflowRunStatusInProgress}
	stub := &stubMgrForHandler{
		getRunFn: func(_, _ string) (codevaldwork.WorkflowRun, error) { return run, nil },
		updateRunFn: func(_, _ string, s codevaldwork.WorkflowRunStatus, _ string) (codevaldwork.WorkflowRun, error) {
			return codevaldwork.WorkflowRun{Status: s}, nil
		},
	}
	h := server.NewRunStatusHandler(stub, "ag")
	h.HandleEvent(context.Background(), "functions.job.failed",
		payload(t, map[string]any{"workflow_run_id": "r3"}))

	if stub.lastStatus != codevaldwork.WorkflowRunStatusFailed {
		t.Errorf("status = %v, want failed", stub.lastStatus)
	}
}

func TestRunStatusHandler_InProgressToCompleted_TerminalEvent(t *testing.T) {
	run := codevaldwork.WorkflowRun{
		ID:            "r4",
		Status:        codevaldwork.WorkflowRunStatusInProgress,
		TerminalEvent: "functions.job.completed:function_name=merge-branch:status=ok",
	}
	stub := &stubMgrForHandler{
		getRunFn: func(_, _ string) (codevaldwork.WorkflowRun, error) { return run, nil },
		updateRunFn: func(_, _ string, s codevaldwork.WorkflowRunStatus, _ string) (codevaldwork.WorkflowRun, error) {
			return codevaldwork.WorkflowRun{Status: s}, nil
		},
	}
	h := server.NewRunStatusHandler(stub, "ag")
	h.HandleEvent(context.Background(), "functions.job.completed",
		payload(t, map[string]any{
			"workflow_run_id": "r4",
			"function_name":   "merge-branch",
			"status":          "ok",
		}))

	if !stub.called {
		t.Fatal("UpdateWorkflowRunStatus not called")
	}
	if stub.lastStatus != codevaldwork.WorkflowRunStatusCompleted {
		t.Errorf("status = %v, want completed", stub.lastStatus)
	}
}

func TestRunStatusHandler_TerminalEvent_FieldMismatch_NoTransition(t *testing.T) {
	run := codevaldwork.WorkflowRun{
		ID:            "r5",
		Status:        codevaldwork.WorkflowRunStatusInProgress,
		TerminalEvent: "functions.job.completed:function_name=merge-branch:status=ok",
	}
	stub := &stubMgrForHandler{
		getRunFn: func(_, _ string) (codevaldwork.WorkflowRun, error) { return run, nil },
		updateRunFn: func(_, _ string, s codevaldwork.WorkflowRunStatus, _ string) (codevaldwork.WorkflowRun, error) {
			return codevaldwork.WorkflowRun{Status: s}, nil
		},
	}
	h := server.NewRunStatusHandler(stub, "ag")
	// Wrong function_name — should NOT trigger completed
	h.HandleEvent(context.Background(), "functions.job.completed",
		payload(t, map[string]any{
			"workflow_run_id": "r5",
			"function_name":   "other-function",
			"status":          "ok",
		}))

	if stub.called {
		t.Errorf("UpdateWorkflowRunStatus should not be called when field qualifier doesn't match")
	}
}

func TestRunStatusHandler_NoRunID_Ignored(t *testing.T) {
	stub := &stubMgrForHandler{
		getRunFn: func(_, _ string) (codevaldwork.WorkflowRun, error) {
			panic("should not be called")
		},
		updateRunFn: func(_, _ string, s codevaldwork.WorkflowRunStatus, _ string) (codevaldwork.WorkflowRun, error) {
			panic("should not be called")
		},
	}
	h := server.NewRunStatusHandler(stub, "ag")
	// No workflow_run_id field — handler must silently ignore
	h.HandleEvent(context.Background(), codevaldwork.TopicTaskAssigned,
		payload(t, map[string]any{"task_id": "t1"}))
	// reaching here without panic means pass
}

func TestRunStatusHandler_PendingUnaffectedByFailure(t *testing.T) {
	run := codevaldwork.WorkflowRun{ID: "r6", Status: codevaldwork.WorkflowRunStatusPending}
	stub := &stubMgrForHandler{
		getRunFn: func(_, _ string) (codevaldwork.WorkflowRun, error) { return run, nil },
		updateRunFn: func(_, _ string, s codevaldwork.WorkflowRunStatus, _ string) (codevaldwork.WorkflowRun, error) {
			return codevaldwork.WorkflowRun{Status: s}, nil
		},
	}
	h := server.NewRunStatusHandler(stub, "ag")
	// Failure event on a pending run — no valid transition
	h.HandleEvent(context.Background(), codevaldwork.TopicTaskFailed,
		payload(t, map[string]any{"workflow_run_id": "r6"}))

	if stub.called {
		t.Errorf("UpdateWorkflowRunStatus should not be called: pending+failure has no transition")
	}
}
