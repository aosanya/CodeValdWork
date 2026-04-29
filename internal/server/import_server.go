package server

import (
	"context"

	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// ImportProject implements pb.TaskServiceServer.
func (s *Server) ImportProject(ctx context.Context, req *pb.ImportProjectRequest) (*pb.ImportProjectResponse, error) {
	result, err := s.mgr.ImportProject(ctx, req.AgencyId, req.Document)
	if err != nil {
		return nil, mapError(err)
	}

	tasks := make([]*pb.Task, 0, len(result.Tasks))
	for _, t := range result.Tasks {
		tasks = append(tasks, taskToProto(t))
	}

	return &pb.ImportProjectResponse{
		Project:     projectToProto(result.Project),
		Tasks:       tasks,
		DepsCreated: int32(result.DepsCreated),
	}, nil
}
