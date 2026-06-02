package server_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// TestCreateWorkflowRun_RPC_RoundTrip covers the happy path: the server
// forwards the request to the manager, the response carries the proto-shaped
// run, and the generated name has the documented prefix.
func TestCreateWorkflowRun_RPC_RoundTrip(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()

	res, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId:     "ag",
		Name:         "",
		TriggerEvent: "work.pipeline.requested",
		Initiator:    "qa-runner",
	})
	if err != nil {
		t.Fatalf("CreateWorkflowRun: %v", err)
	}
	if res.Run == nil || res.Run.Id == "" {
		t.Fatalf("response missing run: %+v", res)
	}
	if !strings.HasPrefix(res.Run.Name, "pipeline-") {
		t.Errorf("generated name = %q want pipeline-* prefix", res.Run.Name)
	}
	if res.Run.Status != pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_PENDING {
		t.Errorf("status = %v want PENDING", res.Run.Status)
	}
}

// TestCreateWorkflowRun_RPC_DuplicateName_AlreadyExists verifies the error
// mapping promotes ErrWorkflowRunNameExists to gRPC ALREADY_EXISTS.
func TestCreateWorkflowRun_RPC_DuplicateName_AlreadyExists(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()

	if _, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "dup",
	}); err != nil {
		t.Fatalf("first CreateWorkflowRun: %v", err)
	}
	_, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: "dup",
	})
	if err == nil {
		t.Fatal("expected error on duplicate name, got nil")
	}
	if got := status.Code(err); got != codes.AlreadyExists {
		t.Errorf("err code = %v want AlreadyExists (err=%v)", got, err)
	}
}

// TestCreateWorkflowRun_RPC_WhitespaceName_InvalidArgument verifies the
// validation surfaces as gRPC INVALID_ARGUMENT.
func TestCreateWorkflowRun_RPC_WhitespaceName_InvalidArgument(t *testing.T) {
	srv := newTestServer()
	_, err := srv.CreateWorkflowRun(context.Background(), &pb.CreateWorkflowRunRequest{
		AgencyId: "ag", Name: " leading",
	})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Errorf("err code = %v want InvalidArgument (err=%v)", got, err)
	}
}

// TestListWorkflowRuns_RPC_NameFilter verifies the request name field is
// forwarded to the manager filter.
func TestListWorkflowRuns_RPC_NameFilter(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()

	for _, n := range []string{"find-me", "other-a", "other-b"} {
		if _, err := srv.CreateWorkflowRun(ctx, &pb.CreateWorkflowRunRequest{
			AgencyId: "ag", Name: n,
		}); err != nil {
			t.Fatalf("seed CreateWorkflowRun %q: %v", n, err)
		}
	}

	all, err := srv.ListWorkflowRuns(ctx, &pb.ListWorkflowRunsRequest{AgencyId: "ag"})
	if err != nil {
		t.Fatalf("ListWorkflowRuns (no filter): %v", err)
	}
	if len(all.Runs) != 3 {
		t.Errorf("unfiltered list len = %d want 3", len(all.Runs))
	}

	filtered, err := srv.ListWorkflowRuns(ctx, &pb.ListWorkflowRunsRequest{AgencyId: "ag", Name: "find-me"})
	if err != nil {
		t.Fatalf("ListWorkflowRuns (filter): %v", err)
	}
	if len(filtered.Runs) != 1 || filtered.Runs[0].Name != "find-me" {
		t.Errorf("filtered list = %+v want exactly one [find-me]", filtered.Runs)
	}
}
