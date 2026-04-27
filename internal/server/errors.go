package server

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

// mapError converts a domain error to its corresponding gRPC status error.
// Unknown errors map to codes.Internal.
func mapError(err error) error {
	switch {
	case errors.Is(err, codevaldwork.ErrTaskNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, codevaldwork.ErrTaskAlreadyExists):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, codevaldwork.ErrInvalidStatusTransition):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, codevaldwork.ErrInvalidTask):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
