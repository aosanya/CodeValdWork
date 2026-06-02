package server_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
	"github.com/aosanya/CodeValdWork/internal/server"
)

// driveRunToInProgress walks a fresh run from pending → in_progress so
// CancelWorkflowRun's precondition is satisfied.
func driveRunToInProgress(t *testing.T, mgr codevaldwork.TaskManager, agencyID, runID string) {
	t.Helper()
	if _, err := mgr.UpdateWorkflowRunStatus(context.Background(), agencyID, runID, codevaldwork.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatalf("→ in_progress: %v", err)
	}
}

// withSyncCancelScheduler swaps the package's scheduleCancelFinalization to
// a synchronous call so tests do not race the time.AfterFunc goroutine and
// can deterministically assert the finalized state.
func withSyncCancelScheduler(t *testing.T) {
	t.Helper()
	original := server.ScheduleCancelFinalizationForTest()
	server.SetScheduleCancelFinalizationForTest(func(s *server.Server, agencyID, runID string, _ time.Duration) {
		if _, err := s.FinalizeForTest(agencyID, runID); err != nil {
			t.Fatalf("finalize: %v", err)
		}
	})
	t.Cleanup(func() {
		server.SetScheduleCancelFinalizationForTest(original)
	})
}

// TestCancelWorkflowRun_RPC_RoundTrip covers the happy path: an in_progress
// run is cancelled via the gRPC handler and the response reflects the
// cancelling status with the envelope populated.
func TestCancelWorkflowRun_RPC_RoundTrip(t *testing.T) {
	// Swap the scheduler so the goroutine runs synchronously inside the
	// handler call — without this the request races the AfterFunc thread.
	var schedMu sync.Mutex
	schedMu.Lock()
	original := server.ScheduleCancelFinalizationForTest()
	server.SetScheduleCancelFinalizationForTest(func(s *server.Server, agencyID, runID string, _ time.Duration) {
		// Intentionally no-op — keep run in cancelling for assertion.
	})
	schedMu.Unlock()
	t.Cleanup(func() {
		schedMu.Lock()
		server.SetScheduleCancelFinalizationForTest(original)
		schedMu.Unlock()
	})

	srv, mgr := newTestServerWithManager()
	ctx := context.Background()

	created, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "cn-rpc",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	driveRunToInProgress(t, mgr, "ag", created.Run.Id)

	res, err := srv.CancelWorkflowRun(ctx, &pb.CancelWorkflowRunRequest{
		AgencyId:        "ag",
		WorkflowRunId:   created.Run.Id,
		Reason:          "regression",
		CancelledBy:     "alice",
		QuiesceTimeoutMs: 30000,
	})
	if err != nil {
		t.Fatalf("CancelWorkflowRun: %v", err)
	}
	if res.Run == nil {
		t.Fatalf("response missing run: %+v", res)
	}
	if res.Run.Status != pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_CANCELLING {
		t.Errorf("status = %v want CANCELLING", res.Run.Status)
	}
	if res.Run.CancelledBy != "alice" {
		t.Errorf("CancelledBy = %q, want %q", res.Run.CancelledBy, "alice")
	}
	if res.Run.CancelReason != "regression" {
		t.Errorf("CancelReason = %q, want %q", res.Run.CancelReason, "regression")
	}
	if res.Run.CancellingUntil == nil {
		t.Error("CancellingUntil nil; want deadline timestamp")
	}
}

// TestCancelWorkflowRun_RPC_NotFound verifies ErrWorkflowRunNotFound maps
// to gRPC NotFound.
func TestCancelWorkflowRun_RPC_NotFound(t *testing.T) {
	srv := newTestServer()
	_, err := srv.CancelWorkflowRun(context.Background(), &pb.CancelWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: "missing",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("err code = %v want NotFound (err=%v)", got, err)
	}
}

// TestCancelWorkflowRun_RPC_PendingRun_FailedPrecondition verifies a pending
// run cannot be cancelled and surfaces FAILED_PRECONDITION.
func TestCancelWorkflowRun_RPC_PendingRun_FailedPrecondition(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()

	created, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "cn-pending",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	_, err = srv.CancelWorkflowRun(ctx, &pb.CancelWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: created.Run.Id,
	})
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("err code = %v want FailedPrecondition (err=%v)", got, err)
	}
}

// TestCancelWorkflowRun_RPC_FinalizesAfterQuiesce verifies the finalization
// goroutine flips the run to CANCELLED. We drive the scheduler synchronously
// so the assertion does not race time.AfterFunc.
func TestCancelWorkflowRun_RPC_FinalizesAfterQuiesce(t *testing.T) {
	withSyncCancelScheduler(t)

	srv, mgr := newTestServerWithManager()
	ctx := context.Background()
	created, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "cn-finalize",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	driveRunToInProgress(t, mgr, "ag", created.Run.Id)

	if _, err := srv.CancelWorkflowRun(ctx, &pb.CancelWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: created.Run.Id,
	}); err != nil {
		t.Fatalf("CancelWorkflowRun: %v", err)
	}

	// After the synchronous scheduler finalizes, the run should be CANCELLED.
	got, err := mgr.GetWorkflowRun(ctx, "ag", created.Run.Id)
	if err != nil {
		t.Fatalf("GetWorkflowRun: %v", err)
	}
	if got.Status != codevaldwork.WorkflowRunStatusCancelled {
		t.Errorf("status = %s, want cancelled", got.Status)
	}
}
