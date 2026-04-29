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
// Detail attachments:
//   - *BlockedError → FailedPrecondition + BlockedByInfo
//   - ErrInvalidRelationship → InvalidArgument + InvalidRelationshipInfo
//     (best-effort: only the reason field is always populated)
func mapError(err error) error {
	var blocked *codevaldwork.BlockedError
	switch {
	case errors.Is(err, codevaldwork.ErrTaskNotFound),
		errors.Is(err, codevaldwork.ErrAgentNotFound),
		errors.Is(err, codevaldwork.ErrProjectNotFound),
		errors.Is(err, codevaldwork.ErrRelationshipNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, codevaldwork.ErrTaskAlreadyExists),
		errors.Is(err, codevaldwork.ErrAgentAlreadyExists),
		errors.Is(err, codevaldwork.ErrProjectAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.As(err, &blocked):
		return blockedStatus(blocked)
	case errors.Is(err, codevaldwork.ErrInvalidRelationship):
		return invalidRelationshipStatus(err)
	case errors.Is(err, codevaldwork.ErrInvalidStatusTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, codevaldwork.ErrInvalidTask),
		errors.Is(err, codevaldwork.ErrInvalidImport):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// invalidRelationshipStatus builds an InvalidArgument status with an
// InvalidRelationshipInfo detail. The reason field is populated from the
// wrapped error message; finer-grained fields are left empty since the
// domain error doesn't currently carry structured info (a follow-up could
// promote ErrInvalidRelationship to a typed error with more context, the
// same way WORK-011 did for ErrBlocked).
func invalidRelationshipStatus(err error) error {
	st := status.New(codes.InvalidArgument, err.Error())
	stWithDetails, dErr := st.WithDetails(&pb.InvalidRelationshipInfo{
		Reason: err.Error(),
	})
	if dErr != nil {
		return st.Err()
	}
	return stWithDetails.Err()
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
