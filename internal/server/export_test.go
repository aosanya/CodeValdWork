package server

import (
	"context"
	"time"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// ScheduleCancelFinalizationForTest returns the current schedule hook so
// tests can save and restore the production behaviour.
func ScheduleCancelFinalizationForTest() func(*Server, string, string, time.Duration) {
	return scheduleCancelFinalization
}

// SetScheduleCancelFinalizationForTest replaces the schedule hook. Tests use
// this to drive the finalize step synchronously (so assertions do not race
// the production time.AfterFunc goroutine).
func SetScheduleCancelFinalizationForTest(fn func(*Server, string, string, time.Duration)) {
	scheduleCancelFinalization = fn
}

// FinalizeForTest invokes FinalizeWorkflowRunCancellation directly on the
// manager. Wraps the manager call so tests can drive finalization without
// resorting to imported codevaldwork value type matching.
func (s *Server) FinalizeForTest(agencyID, runID string) (codevaldwork.WorkflowRun, error) {
	return s.mgr.FinalizeWorkflowRunCancellation(context.Background(), agencyID, runID)
}
