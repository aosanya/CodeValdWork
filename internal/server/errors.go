package server

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// mapError converts a domain error to its corresponding gRPC status error.
// Unknown errors map to codes.Internal.
//
// When err is a *BlockedError, the returned status carries a BlockedByInfo
// detail listing the non-terminal blocker task IDs so clients can surface
// them without a follow-up RPC.
func mapError(err error) error {
	var blocked *codevaldwork.BlockedError
	switch {
	case errors.Is(err, codevaldwork.ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, codevaldwork.ErrTaskAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.As(err, &blocked):
		return blockedStatus(blocked)
	case errors.Is(err, codevaldwork.ErrInvalidStatusTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, codevaldwork.ErrInvalidTask):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// blockedStatus builds a FailedPrecondition status with a BlockedByInfo
// detail containing the blocker IDs from the BlockedError. If detail
// attachment fails (proto marshalling), falls back to a plain status without
// the detail — the error code and message remain correct.
func blockedStatus(be *codevaldwork.BlockedError) error {
	st := status.New(codes.FailedPrecondition, be.Error())
	stWithDetails, err := st.WithDetails(&pb.BlockedByInfo{
		BlockerTaskIds: be.BlockerTaskIDs,
	})
	if err != nil {
		return st.Err()
	}
	return stWithDetails.Err()
}
