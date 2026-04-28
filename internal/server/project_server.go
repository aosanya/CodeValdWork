package server

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// CreateProject implements pb.TaskServiceServer.
func (s *Server) CreateProject(ctx context.Context, req *pb.CreateProjectRequest) (*pb.CreateProjectResponse, error) {
	p, err := s.mgr.CreateProject(ctx, req.AgencyId, protoToProject(req.Project))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.CreateProjectResponse{Project: projectToProto(p)}, nil
}

// GetProject implements pb.TaskServiceServer.
func (s *Server) GetProject(ctx context.Context, req *pb.GetProjectRequest) (*pb.GetProjectResponse, error) {
	p, err := s.mgr.GetProject(ctx, req.AgencyId, req.ProjectId)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.GetProjectResponse{Project: projectToProto(p)}, nil
}

// UpdateProject implements pb.TaskServiceServer.
func (s *Server) UpdateProject(ctx context.Context, req *pb.UpdateProjectRequest) (*pb.UpdateProjectResponse, error) {
	p, err := s.mgr.UpdateProject(ctx, req.AgencyId, protoToProject(req.Project))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.UpdateProjectResponse{Project: projectToProto(p)}, nil
}

// DeleteProject implements pb.TaskServiceServer.
func (s *Server) DeleteProject(ctx context.Context, req *pb.DeleteProjectRequest) (*pb.DeleteProjectResponse, error) {
	if err := s.mgr.DeleteProject(ctx, req.AgencyId, req.ProjectId); err != nil {
		return nil, mapError(err)
	}
	return &pb.DeleteProjectResponse{}, nil
}

// ListProjects implements pb.TaskServiceServer.
func (s *Server) ListProjects(ctx context.Context, req *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	projects, err := s.mgr.ListProjects(ctx, req.AgencyId)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.Project, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectToProto(p))
	}
	return &pb.ListProjectsResponse{Projects: out}, nil
}

// AddTaskToProject implements pb.TaskServiceServer.
func (s *Server) AddTaskToProject(ctx context.Context, req *pb.AddTaskToProjectRequest) (*pb.AddTaskToProjectResponse, error) {
	if err := s.mgr.AddTaskToProject(ctx, req.AgencyId, req.TaskId, req.ProjectId); err != nil {
		return nil, mapError(err)
	}
	return &pb.AddTaskToProjectResponse{}, nil
}

// RemoveTaskFromProject implements pb.TaskServiceServer.
func (s *Server) RemoveTaskFromProject(ctx context.Context, req *pb.RemoveTaskFromProjectRequest) (*pb.RemoveTaskFromProjectResponse, error) {
	if err := s.mgr.RemoveTaskFromProject(ctx, req.AgencyId, req.TaskId, req.ProjectId); err != nil {
		return nil, mapError(err)
	}
	return &pb.RemoveTaskFromProjectResponse{}, nil
}

// ListTasksInProject implements pb.TaskServiceServer.
func (s *Server) ListTasksInProject(ctx context.Context, req *pb.ListTasksInProjectRequest) (*pb.ListTasksInProjectResponse, error) {
	tasks, err := s.mgr.ListTasksInProject(ctx, req.AgencyId, req.ProjectId)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskToProto(t))
	}
	return &pb.ListTasksInProjectResponse{Tasks: out}, nil
}

// ListProjectsForTask implements pb.TaskServiceServer.
func (s *Server) ListProjectsForTask(ctx context.Context, req *pb.ListProjectsForTaskRequest) (*pb.ListProjectsForTaskResponse, error) {
	projects, err := s.mgr.ListProjectsForTask(ctx, req.AgencyId, req.TaskId)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.Project, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectToProto(p))
	}
	return &pb.ListProjectsForTaskResponse{Projects: out}, nil
}

// ── Conversion helpers ────────────────────────────────────────────────────────

func projectToProto(p codevaldwork.Project) *pb.Project {
	pp := &pb.Project{
		Id:          p.ID,
		AgencyId:    p.AgencyID,
		Name:        p.Name,
		Description: p.Description,
		GithubRepo:  p.GithubRepo,
		CreatedAt:   timestamppb.New(p.CreatedAt),
		UpdatedAt:   timestamppb.New(p.UpdatedAt),
	}
	if p.DueAt != nil {
		pp.DueAt = timestamppb.New(*p.DueAt)
	}
	return pp
}

func protoToProject(pp *pb.Project) codevaldwork.Project {
	if pp == nil {
		return codevaldwork.Project{}
	}
	p := codevaldwork.Project{
		ID:          pp.Id,
		AgencyID:    pp.AgencyId,
		Name:        pp.Name,
		Description: pp.Description,
		GithubRepo:  pp.GithubRepo,
	}
	if pp.CreatedAt != nil {
		p.CreatedAt = pp.CreatedAt.AsTime()
	}
	if pp.UpdatedAt != nil {
		p.UpdatedAt = pp.UpdatedAt.AsTime()
	}
	if pp.DueAt != nil {
		ts := pp.DueAt.AsTime()
		p.DueAt = &ts
	}
	return p
}
