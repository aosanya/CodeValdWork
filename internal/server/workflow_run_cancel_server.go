package server

import (
	"context"
	"log/slog"
	"time"

	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// defaultCancelQuiesceTimeout is applied when the request's
// quiesce_timeout_ms is zero or negative (FEAT-20260602-008 §4).
const defaultCancelQuiesceTimeout = 30 * time.Second

// CancelWorkflowRun implements pb.TaskServiceServer (FEAT-20260602-008).
//
// Flips an in_progress WorkflowRun to the cancelling transient state,
// cascades work.task.cancelled per non-terminal Task, and publishes
// work.run.cancelling. Returns the run carrying the cancellation envelope.
//
// After the synchronous half returns, the handler schedules a finalization
// goroutine that, after the quiesce deadline elapses, transitions the run
// to cancelled and publishes work.run.cancelled. The finalization step is
// idempotent — late events or a separately-triggered finalize are no-ops.
//
// Errors:
//   - NOT_FOUND when the run does not exist.
//   - FAILED_PRECONDITION when the run is in any status other than
//     in_progress or cancelling.
func (s *Server) CancelWorkflowRun(ctx context.Context, req *pb.CancelWorkflowRunRequest) (*pb.CancelWorkflowRunResponse, error) {
	timeout := time.Duration(req.QuiesceTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultCancelQuiesceTimeout
	}
	deadline := time.Now().UTC().Add(timeout)

	run, err := s.mgr.CancelWorkflowRun(ctx, req.AgencyId, req.WorkflowRunId, req.Reason, req.CancelledBy, deadline)
	if err != nil {
		return nil, mapError(err)
	}

	// Schedule the finalize step. Detached from the request context — the
	// caller's deadline is irrelevant; what matters is the run's quiesce
	// deadline. A nil-channel time.AfterFunc would block forever if the
	// process exits first; that is acceptable for v1 (FEAT spec §4 documents
	// the crash-resilient sweep-on-startup as future work).
	scheduleCancelFinalization(s, req.AgencyId, req.WorkflowRunId, timeout)

	return &pb.CancelWorkflowRunResponse{Run: workflowRunToProto(run)}, nil
}

// scheduleCancelFinalization fires the finalize call after timeout elapses.
// Pulled out so tests can override via package-level variable.
var scheduleCancelFinalization = func(s *Server, agencyID, runID string, timeout time.Duration) {
	time.AfterFunc(timeout, func() {
		// Use a detached background context; the gRPC request context is
		// long dead by the time this fires.
		ctx := context.Background()
		if _, err := s.mgr.FinalizeWorkflowRunCancellation(ctx, agencyID, runID); err != nil {
			slog.ErrorContext(ctx, "CancelWorkflowRun: finalize failed", "run_id", runID, "err", err)
		}
	})
}
