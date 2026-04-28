// Package server implements the TaskService gRPC handler.
// It wraps a codevaldwork.TaskManager and translates between proto messages
// and domain types. EntityServer (for the schema-driven generic CRUD path) is
// re-exported via entity_server.go.
package server

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// Server implements pb.TaskServiceServer by wrapping a codevaldwork.TaskManager.
// Construct via New; register with grpc.Server using
// pb.RegisterTaskServiceServer.
type Server struct {
	pb.UnimplementedTaskServiceServer
	mgr codevaldwork.TaskManager
}

// New constructs a Server backed by the given TaskManager.
func New(mgr codevaldwork.TaskManager) *Server {
	return &Server{mgr: mgr}
}

// CreateTask implements pb.TaskServiceServer.
func (s *Server) CreateTask(ctx context.Context, req *pb.CreateTaskRequest) (*pb.CreateTaskResponse, error) {
	task, err := s.mgr.CreateTask(ctx, req.AgencyId, protoToTask(req.Task))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.CreateTaskResponse{Task: taskToProto(task)}, nil
}

// GetTask implements pb.TaskServiceServer.
func (s *Server) GetTask(ctx context.Context, req *pb.GetTaskRequest) (*pb.GetTaskResponse, error) {
	task, err := s.mgr.GetTask(ctx, req.AgencyId, req.TaskId)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.GetTaskResponse{Task: taskToProto(task)}, nil
}

// UpdateTask implements pb.TaskServiceServer.
func (s *Server) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	task, err := s.mgr.UpdateTask(ctx, req.AgencyId, protoToTask(req.Task))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.UpdateTaskResponse{Task: taskToProto(task)}, nil
}

// DeleteTask implements pb.TaskServiceServer.
func (s *Server) DeleteTask(ctx context.Context, req *pb.DeleteTaskRequest) (*pb.DeleteTaskResponse, error) {
	if err := s.mgr.DeleteTask(ctx, req.AgencyId, req.TaskId); err != nil {
		return nil, mapError(err)
	}
	return &pb.DeleteTaskResponse{}, nil
}

// ListTasks implements pb.TaskServiceServer.
func (s *Server) ListTasks(ctx context.Context, req *pb.ListTasksRequest) (*pb.ListTasksResponse, error) {
	tasks, err := s.mgr.ListTasks(ctx, req.AgencyId, protoToFilter(req.Filter))
	if err != nil {
		return nil, mapError(err)
	}
	pbTasks := make([]*pb.Task, 0, len(tasks))
	for _, t := range tasks {
		pbTasks = append(pbTasks, taskToProto(t))
	}
	return &pb.ListTasksResponse{Tasks: pbTasks}, nil
}

// ── Conversion helpers ────────────────────────────────────────────────────────

func taskToProto(t codevaldwork.Task) *pb.Task {
	pt := &pb.Task{
		Id:             t.ID,
		AgencyId:       t.AgencyID,
		Title:          t.Title,
		Description:    t.Description,
		Status:         statusToProto(t.Status),
		Priority:       priorityToProto(t.Priority),
		Tags:           append([]string(nil), t.Tags...),
		EstimatedHours: t.EstimatedHours,
		Context:        t.Context,
		CreatedAt:      timestamppb.New(t.CreatedAt),
		UpdatedAt:      timestamppb.New(t.UpdatedAt),
	}
	if t.CompletedAt != nil {
		pt.CompletedAt = timestamppb.New(*t.CompletedAt)
	}
	if t.DueAt != nil {
		pt.DueAt = timestamppb.New(*t.DueAt)
	}
	return pt
}

func protoToTask(pt *pb.Task) codevaldwork.Task {
	if pt == nil {
		return codevaldwork.Task{}
	}
	t := codevaldwork.Task{
		ID:             pt.Id,
		AgencyID:       pt.AgencyId,
		Title:          pt.Title,
		Description:    pt.Description,
		Status:         protoToStatus(pt.Status),
		Priority:       protoToPriority(pt.Priority),
		Tags:           append([]string(nil), pt.Tags...),
		EstimatedHours: pt.EstimatedHours,
		Context:        pt.Context,
	}
	if pt.CreatedAt != nil {
		t.CreatedAt = pt.CreatedAt.AsTime()
	}
	if pt.UpdatedAt != nil {
		t.UpdatedAt = pt.UpdatedAt.AsTime()
	}
	if pt.CompletedAt != nil {
		ts := pt.CompletedAt.AsTime()
		t.CompletedAt = &ts
	}
	if pt.DueAt != nil {
		ts := pt.DueAt.AsTime()
		t.DueAt = &ts
	}
	return t
}

func protoToFilter(pf *pb.TaskFilter) codevaldwork.TaskFilter {
	if pf == nil {
		return codevaldwork.TaskFilter{}
	}
	return codevaldwork.TaskFilter{
		Status:   protoToStatus(pf.Status),
		Priority: protoToPriority(pf.Priority),
	}
}

func statusToProto(s codevaldwork.TaskStatus) pb.TaskStatus {
	switch s {
	case codevaldwork.TaskStatusPending:
		return pb.TaskStatus_TASK_STATUS_PENDING
	case codevaldwork.TaskStatusInProgress:
		return pb.TaskStatus_TASK_STATUS_IN_PROGRESS
	case codevaldwork.TaskStatusCompleted:
		return pb.TaskStatus_TASK_STATUS_COMPLETED
	case codevaldwork.TaskStatusFailed:
		return pb.TaskStatus_TASK_STATUS_FAILED
	case codevaldwork.TaskStatusCancelled:
		return pb.TaskStatus_TASK_STATUS_CANCELLED
	default:
		return pb.TaskStatus_TASK_STATUS_UNSPECIFIED
	}
}

func protoToStatus(s pb.TaskStatus) codevaldwork.TaskStatus {
	switch s {
	case pb.TaskStatus_TASK_STATUS_PENDING:
		return codevaldwork.TaskStatusPending
	case pb.TaskStatus_TASK_STATUS_IN_PROGRESS:
		return codevaldwork.TaskStatusInProgress
	case pb.TaskStatus_TASK_STATUS_COMPLETED:
		return codevaldwork.TaskStatusCompleted
	case pb.TaskStatus_TASK_STATUS_FAILED:
		return codevaldwork.TaskStatusFailed
	case pb.TaskStatus_TASK_STATUS_CANCELLED:
		return codevaldwork.TaskStatusCancelled
	default:
		return ""
	}
}

func priorityToProto(p codevaldwork.TaskPriority) pb.TaskPriority {
	switch p {
	case codevaldwork.TaskPriorityLow:
		return pb.TaskPriority_TASK_PRIORITY_LOW
	case codevaldwork.TaskPriorityMedium:
		return pb.TaskPriority_TASK_PRIORITY_MEDIUM
	case codevaldwork.TaskPriorityHigh:
		return pb.TaskPriority_TASK_PRIORITY_HIGH
	case codevaldwork.TaskPriorityCritical:
		return pb.TaskPriority_TASK_PRIORITY_CRITICAL
	default:
		return pb.TaskPriority_TASK_PRIORITY_UNSPECIFIED
	}
}

func protoToPriority(p pb.TaskPriority) codevaldwork.TaskPriority {
	switch p {
	case pb.TaskPriority_TASK_PRIORITY_LOW:
		return codevaldwork.TaskPriorityLow
	case pb.TaskPriority_TASK_PRIORITY_MEDIUM:
		return codevaldwork.TaskPriorityMedium
	case pb.TaskPriority_TASK_PRIORITY_HIGH:
		return codevaldwork.TaskPriorityHigh
	case pb.TaskPriority_TASK_PRIORITY_CRITICAL:
		return codevaldwork.TaskPriorityCritical
	default:
		return ""
	}
}
