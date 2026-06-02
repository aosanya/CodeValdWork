package server_test

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// driveRunToFailed walks a fresh run through pending → in_progress → failed
// using the manager directly, since the gRPC surface does not expose a status
// transition RPC.
func driveRunToFailed(t *testing.T, mgr codevaldwork.TaskManager, agencyID, runID string) {
	t.Helper()
	ctx := context.Background()
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, agencyID, runID, codevaldwork.WorkflowRunStatusInProgress, ""); err != nil {
		t.Fatalf("→ in_progress: %v", err)
	}
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, agencyID, runID, codevaldwork.WorkflowRunStatusFailed, ""); err != nil {
		t.Fatalf("→ failed: %v", err)
	}
}

// TestRollbackWorkflowRun_RPC_RoundTrip covers the happy path: a failed run
// is rolled back via the gRPC handler and the response reflects the
// rolled_back terminal status.
func TestRollbackWorkflowRun_RPC_RoundTrip(t *testing.T) {
	srv, mgr := newTestServerWithManager()
	ctx := context.Background()

	created, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "rb-rpc",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	driveRunToFailed(t, mgr, "ag", created.Run.Id)

	res, err := srv.RollbackWorkflowRun(ctx, &pb.RollbackWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: created.Run.Id,
		Reason:        "regression",
	})
	if err != nil {
		t.Fatalf("RollbackWorkflowRun: %v", err)
	}
	if res.Run == nil {
		t.Fatalf("response missing run: %+v", res)
	}
	if res.Run.Status != pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_ROLLED_BACK {
		t.Errorf("status = %v want ROLLED_BACK", res.Run.Status)
	}
}

// TestRollbackWorkflowRun_RPC_NotFound verifies ErrWorkflowRunNotFound maps
// to gRPC NotFound.
func TestRollbackWorkflowRun_RPC_NotFound(t *testing.T) {
	srv := newTestServer()
	_, err := srv.RollbackWorkflowRun(context.Background(), &pb.RollbackWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: "missing",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Errorf("err code = %v want NotFound (err=%v)", got, err)
	}
}

// TestRollbackWorkflowRun_RPC_PendingRun_FailedPrecondition verifies that
// rolling back from a non-terminal status surfaces as FailedPrecondition
// (ErrInvalidRunStatusTransition is mapped to FailedPrecondition).
func TestRollbackWorkflowRun_RPC_PendingRun_FailedPrecondition(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()

	created, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "rb-pending",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}

	_, err = srv.RollbackWorkflowRun(ctx, &pb.RollbackWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: created.Run.Id,
	})
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("err code = %v want FailedPrecondition (err=%v)", got, err)
	}
}

// TestRollbackWorkflowRun_RPC_AlreadyRollingBack_FailedPrecondition verifies
// the rollback conflict surfaces as FailedPrecondition (matches errors.go).
func TestRollbackWorkflowRun_RPC_AlreadyRollingBack_FailedPrecondition(t *testing.T) {
	srv, mgr := newTestServerWithManager()
	ctx := context.Background()

	created, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "rb-conflict",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	driveRunToFailed(t, mgr, "ag", created.Run.Id)
	if _, err := mgr.UpdateWorkflowRunStatus(ctx, "ag", created.Run.Id, codevaldwork.WorkflowRunStatusRollingBack, ""); err != nil {
		t.Fatalf("→ rolling_back: %v", err)
	}

	_, err = srv.RollbackWorkflowRun(ctx, &pb.RollbackWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: created.Run.Id,
	})
	if got := status.Code(err); got != codes.FailedPrecondition {
		t.Errorf("err code = %v want FailedPrecondition (err=%v)", got, err)
	}
}

// TestRollbackWorkflowRun_RPC_ForeignDependency_Aborted verifies that a
// foreign-run dependency surfaces as gRPC Aborted (matches errors.go) and
// leaves the run in rollback_failed so the operator can remediate and retry.
func TestRollbackWorkflowRun_RPC_ForeignDependency_Aborted(t *testing.T) {
	srv, mgr := newTestServerWithManager()
	ctx := context.Background()

	runA, _ := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{AgencyId: "ag", Name: "rb-A"})
	runB, _ := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{AgencyId: "ag", Name: "rb-B"})

	taskA, err := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "a", WorkflowRunID: runA.Run.Id})
	if err != nil {
		t.Fatalf("CreateTask A: %v", err)
	}
	taskB, err := mgr.CreateTask(ctx, "ag", codevaldwork.Task{Title: "b", WorkflowRunID: runB.Run.Id})
	if err != nil {
		t.Fatalf("CreateTask B: %v", err)
	}
	if _, err := mgr.CreateRelationship(ctx, "ag", codevaldwork.Relationship{
		Label:  codevaldwork.RelLabelDependsOn,
		FromID: taskB.ID,
		ToID:   taskA.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship depends_on: %v", err)
	}

	driveRunToFailed(t, mgr, "ag", runA.Run.Id)

	_, err = srv.RollbackWorkflowRun(ctx, &pb.RollbackWorkflowRunRequest{
		AgencyId:      "ag",
		WorkflowRunId: runA.Run.Id,
		Reason:        "blocked by run B",
	})
	if got := status.Code(err); got != codes.Aborted {
		t.Errorf("err code = %v want Aborted (err=%v)", got, err)
	}

	// Verify the run persists in rollback_failed for operator retry.
	persisted, gerr := mgr.GetWorkflowRun(ctx, "ag", runA.Run.Id)
	if gerr != nil {
		t.Fatalf("GetWorkflowRun: %v", gerr)
	}
	if persisted.Status != codevaldwork.WorkflowRunStatusRollbackFailed {
		t.Errorf("persisted status = %q want rollback_failed", persisted.Status)
	}

	// Sanity: domain error sentinel propagated through the manager.
	if _, err := mgr.RollbackWorkflowRun(ctx, "ag", runA.Run.Id, ""); !errors.Is(err, codevaldwork.ErrForeignRunDependency) {
		t.Errorf("retry err = %v want ErrForeignRunDependency", err)
	}
}
