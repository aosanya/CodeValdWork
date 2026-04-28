package server

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

func TestMapError_BlockedError_AttachesBlockedByInfoDetail(t *testing.T) {
	domainErr := &codevaldwork.BlockedError{
		BlockerTaskIDs: []string{"task-A", "task-B"},
	}

	gErr := mapError(domainErr)

	st, ok := status.FromError(gErr)
	if !ok {
		t.Fatalf("mapError did not return a gRPC status, got %T: %v", gErr, gErr)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", st.Code())
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
	if len(info.BlockerTaskIds) != 2 || info.BlockerTaskIds[0] != "task-A" || info.BlockerTaskIds[1] != "task-B" {
		t.Errorf("BlockerTaskIds = %v, want [task-A task-B]", info.BlockerTaskIds)
	}
}

func TestMapError_WrappedBlockedError_StillExtracted(t *testing.T) {
	// Belt-and-braces: a fmt.Errorf wrap of *BlockedError must still produce
	// the FailedPrecondition + BlockedByInfo response, since UpdateTask may
	// wrap the error in real call paths.
	wrapped := errors.Join(errors.New("upstream context"), &codevaldwork.BlockedError{BlockerTaskIDs: []string{"x"}})
	gErr := mapError(wrapped)
	st, _ := status.FromError(gErr)
	if st.Code() != codes.FailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", st.Code())
	}
}

func TestMapError_NonBlockedErrors_Unchanged(t *testing.T) {
	cases := []struct {
		name string
		in   error
		want codes.Code
	}{
		{"task not found", codevaldwork.ErrTaskNotFound, codes.NotFound},
		{"already exists", codevaldwork.ErrTaskAlreadyExists, codes.AlreadyExists},
		{"invalid transition", codevaldwork.ErrInvalidStatusTransition, codes.FailedPrecondition},
		{"invalid task", codevaldwork.ErrInvalidTask, codes.InvalidArgument},
		{"unknown", errors.New("boom"), codes.Internal},
	}
	for _, tc := range cases {
		gErr := mapError(tc.in)
		st, _ := status.FromError(gErr)
		if st.Code() != tc.want {
			t.Errorf("%s: code = %v, want %v", tc.name, st.Code(), tc.want)
		}
	}
}
