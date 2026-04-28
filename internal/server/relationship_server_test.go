package server_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

func TestCreateRelationship_HappyPath_ReturnsEdge(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	a := createProtoTask(t, srv, "ag", "a")
	b := createProtoTask(t, srv, "ag", "b")

	res, err := srv.CreateRelationship(ctx, &pb.CreateRelationshipRequest{
		AgencyId: "ag", Label: codevaldwork.RelLabelBlocks, FromId: a.Id, ToId: b.Id,
	})
	if err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}
	if res.Relationship.Id == "" {
		t.Error("missing edge ID")
	}
	if res.Relationship.Label != codevaldwork.RelLabelBlocks {
		t.Errorf("label = %s", res.Relationship.Label)
	}
}

func TestCreateRelationship_UnknownLabel_ReturnsInvalidArgumentWithDetail(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	a := createProtoTask(t, srv, "ag", "a")
	b := createProtoTask(t, srv, "ag", "b")

	_, err := srv.CreateRelationship(ctx, &pb.CreateRelationshipRequest{
		AgencyId: "ag", Label: "not_a_label", FromId: a.Id, ToId: b.Id,
	})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
	st, _ := status.FromError(err)
	var found *pb.InvalidRelationshipInfo
	for _, d := range st.Details() {
		if info, ok := d.(*pb.InvalidRelationshipInfo); ok {
			found = info
			break
		}
	}
	if found == nil {
		t.Fatal("status carries no InvalidRelationshipInfo detail")
	}
	if found.Reason == "" {
		t.Error("InvalidRelationshipInfo.Reason is empty")
	}
}

func TestCreateRelationship_MissingEndpoint_ReturnsNotFound(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	a := createProtoTask(t, srv, "ag", "a")

	_, err := srv.CreateRelationship(ctx, &pb.CreateRelationshipRequest{
		AgencyId: "ag", Label: codevaldwork.RelLabelBlocks, FromId: a.Id, ToId: "no-such-task",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestDeleteRelationship_Missing_ReturnsNotFound(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	a := createProtoTask(t, srv, "ag", "a")
	b := createProtoTask(t, srv, "ag", "b")

	_, err := srv.DeleteRelationship(ctx, &pb.DeleteRelationshipRequest{
		AgencyId: "ag", Label: codevaldwork.RelLabelBlocks, FromId: a.Id, ToId: b.Id,
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestTraverseRelationships_RespectsDirection(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	a := createProtoTask(t, srv, "ag", "a")
	b := createProtoTask(t, srv, "ag", "b")
	if _, err := srv.CreateRelationship(ctx, &pb.CreateRelationshipRequest{
		AgencyId: "ag", Label: codevaldwork.RelLabelBlocks, FromId: a.Id, ToId: b.Id,
	}); err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}

	out, err := srv.TraverseRelationships(ctx, &pb.TraverseRelationshipsRequest{
		AgencyId: "ag", VertexId: a.Id, Label: codevaldwork.RelLabelBlocks,
		Direction: pb.Direction_DIRECTION_OUTBOUND,
	})
	if err != nil {
		t.Fatalf("Traverse outbound: %v", err)
	}
	if len(out.Relationships) != 1 || out.Relationships[0].ToId != b.Id {
		t.Errorf("outbound from a: %v", out.Relationships)
	}

	in, _ := srv.TraverseRelationships(ctx, &pb.TraverseRelationshipsRequest{
		AgencyId: "ag", VertexId: a.Id, Label: codevaldwork.RelLabelBlocks,
		Direction: pb.Direction_DIRECTION_INBOUND,
	})
	if len(in.Relationships) != 0 {
		t.Errorf("inbound on a: want 0, got %d", len(in.Relationships))
	}
}

func TestUpdateTask_Blocked_RoundTripsBlockedByInfoOverGRPC(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	a := createProtoTask(t, srv, "ag", "blocker")
	b := createProtoTask(t, srv, "ag", "blocked")
	if _, err := srv.CreateRelationship(ctx, &pb.CreateRelationshipRequest{
		AgencyId: "ag", Label: codevaldwork.RelLabelBlocks, FromId: a.Id, ToId: b.Id,
	}); err != nil {
		t.Fatalf("setup blocks edge: %v", err)
	}

	// Try to start b while a is still pending — should fail with FailedPrecondition
	// + BlockedByInfo detail.
	b.Status = pb.TaskStatus_TASK_STATUS_IN_PROGRESS
	_, err := srv.UpdateTask(ctx, &pb.UpdateTaskRequest{AgencyId: "ag", Task: b})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("not a gRPC status error: %v", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("code = %v, want FailedPrecondition", st.Code())
	}
	var info *pb.BlockedByInfo
	for _, d := range st.Details() {
		if bi, ok := d.(*pb.BlockedByInfo); ok {
			info = bi
			break
		}
	}
	if info == nil {
		t.Fatal("status carries no BlockedByInfo detail")
	}
	if len(info.BlockerTaskIds) != 1 || info.BlockerTaskIds[0] != a.Id {
		t.Errorf("BlockerTaskIds = %v, want [%s]", info.BlockerTaskIds, a.Id)
	}
}
