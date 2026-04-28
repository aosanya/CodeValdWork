package server

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// CreateTaskGroup implements pb.TaskServiceServer.
func (s *Server) CreateTaskGroup(ctx context.Context, req *pb.CreateTaskGroupRequest) (*pb.CreateTaskGroupResponse, error) {
	g, err := s.mgr.CreateTaskGroup(ctx, req.AgencyId, protoToTaskGroup(req.Group))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.CreateTaskGroupResponse{Group: taskGroupToProto(g)}, nil
}

// GetTaskGroup implements pb.TaskServiceServer.
func (s *Server) GetTaskGroup(ctx context.Context, req *pb.GetTaskGroupRequest) (*pb.GetTaskGroupResponse, error) {
	g, err := s.mgr.GetTaskGroup(ctx, req.AgencyId, req.TaskGroupId)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.GetTaskGroupResponse{Group: taskGroupToProto(g)}, nil
}

// UpdateTaskGroup implements pb.TaskServiceServer.
func (s *Server) UpdateTaskGroup(ctx context.Context, req *pb.UpdateTaskGroupRequest) (*pb.UpdateTaskGroupResponse, error) {
	g, err := s.mgr.UpdateTaskGroup(ctx, req.AgencyId, protoToTaskGroup(req.Group))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.UpdateTaskGroupResponse{Group: taskGroupToProto(g)}, nil
}

// DeleteTaskGroup implements pb.TaskServiceServer.
func (s *Server) DeleteTaskGroup(ctx context.Context, req *pb.DeleteTaskGroupRequest) (*pb.DeleteTaskGroupResponse, error) {
	if err := s.mgr.DeleteTaskGroup(ctx, req.AgencyId, req.TaskGroupId); err != nil {
		return nil, mapError(err)
	}
	return &pb.DeleteTaskGroupResponse{}, nil
}

// ListTaskGroups implements pb.TaskServiceServer.
func (s *Server) ListTaskGroups(ctx context.Context, req *pb.ListTaskGroupsRequest) (*pb.ListTaskGroupsResponse, error) {
	groups, err := s.mgr.ListTaskGroups(ctx, req.AgencyId)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.TaskGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, taskGroupToProto(g))
	}
	return &pb.ListTaskGroupsResponse{Groups: out}, nil
}

// AddTaskToGroup implements pb.TaskServiceServer.
func (s *Server) AddTaskToGroup(ctx context.Context, req *pb.AddTaskToGroupRequest) (*pb.AddTaskToGroupResponse, error) {
	if err := s.mgr.AddTaskToGroup(ctx, req.AgencyId, req.TaskId, req.TaskGroupId); err != nil {
		return nil, mapError(err)
	}
	return &pb.AddTaskToGroupResponse{}, nil
}

// RemoveTaskFromGroup implements pb.TaskServiceServer.
func (s *Server) RemoveTaskFromGroup(ctx context.Context, req *pb.RemoveTaskFromGroupRequest) (*pb.RemoveTaskFromGroupResponse, error) {
	if err := s.mgr.RemoveTaskFromGroup(ctx, req.AgencyId, req.TaskId, req.TaskGroupId); err != nil {
		return nil, mapError(err)
	}
	return &pb.RemoveTaskFromGroupResponse{}, nil
}

// ListTasksInGroup implements pb.TaskServiceServer.
func (s *Server) ListTasksInGroup(ctx context.Context, req *pb.ListTasksInGroupRequest) (*pb.ListTasksInGroupResponse, error) {
	tasks, err := s.mgr.ListTasksInGroup(ctx, req.AgencyId, req.TaskGroupId)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.Task, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, taskToProto(t))
	}
	return &pb.ListTasksInGroupResponse{Tasks: out}, nil
}

// ListGroupsForTask implements pb.TaskServiceServer.
func (s *Server) ListGroupsForTask(ctx context.Context, req *pb.ListGroupsForTaskRequest) (*pb.ListGroupsForTaskResponse, error) {
	groups, err := s.mgr.ListGroupsForTask(ctx, req.AgencyId, req.TaskId)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.TaskGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, taskGroupToProto(g))
	}
	return &pb.ListGroupsForTaskResponse{Groups: out}, nil
}

// ── Conversion helpers ────────────────────────────────────────────────────────

func taskGroupToProto(g codevaldwork.TaskGroup) *pb.TaskGroup {
	pg := &pb.TaskGroup{
		Id:          g.ID,
		AgencyId:    g.AgencyID,
		Name:        g.Name,
		Description: g.Description,
		CreatedAt:   timestamppb.New(g.CreatedAt),
		UpdatedAt:   timestamppb.New(g.UpdatedAt),
	}
	if g.DueAt != nil {
		pg.DueAt = timestamppb.New(*g.DueAt)
	}
	return pg
}

func protoToTaskGroup(pg *pb.TaskGroup) codevaldwork.TaskGroup {
	if pg == nil {
		return codevaldwork.TaskGroup{}
	}
	g := codevaldwork.TaskGroup{
		ID:          pg.Id,
		AgencyID:    pg.AgencyId,
		Name:        pg.Name,
		Description: pg.Description,
	}
	if pg.CreatedAt != nil {
		g.CreatedAt = pg.CreatedAt.AsTime()
	}
	if pg.UpdatedAt != nil {
		g.UpdatedAt = pg.UpdatedAt.AsTime()
	}
	if pg.DueAt != nil {
		ts := pg.DueAt.AsTime()
		g.DueAt = &ts
	}
	return g
}
