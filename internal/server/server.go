// Package server implements the TaskService gRPC handler.
// It wraps a codevaldwork.TaskManager and translates between proto messages
// and domain types. EntityServer (for the schema-driven generic CRUD path) is
// re-exported via entity_server.go.
package server

import (
	"context"
	"log"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// Server implements pb.TaskServiceServer by wrapping a codevaldwork.TaskManager.
// Construct via New; register with grpc.Server using
// pb.RegisterTaskServiceServer.
type Server struct {
	pb.UnimplementedTaskServiceServer
	mgr        codevaldwork.TaskManager
	dispatcher *TaskEventDispatcher // optional; required for FailTodo cascade
}

// New constructs a Server backed by the given TaskManager.
func New(mgr codevaldwork.TaskManager) *Server {
	return &Server{mgr: mgr}
}

// WithDispatcher wires a TaskEventDispatcher into the server, enabling the
// FailTodo RPC to run blockDependentTodos and maybeCompleteParentTask.
func (s *Server) WithDispatcher(d *TaskEventDispatcher) *Server {
	s.dispatcher = d
	return s
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

// resolveTaskID returns the entity ID for a task, preferring a direct ID but
// falling back to a project-scoped name lookup. Mirrors resolveProjectID.
func (s *Server) resolveTaskID(ctx context.Context, agencyID, taskID, taskName, projectName string) (string, error) {
	if taskID != "" {
		return taskID, nil
	}
	t, err := s.mgr.GetTaskByName(ctx, agencyID, projectName, taskName)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}

// UpdateTask implements pb.TaskServiceServer.
//
// When req.update_mask is set, only the listed fields are overwritten on the
// stored task — every other field is preserved from the current entity. When
// update_mask is empty, the handler falls back to the legacy replace-all
// behaviour: every mutable field of the incoming task is written, so omitted
// fields are silently zeroed (the proto3 footgun that BUG-09-023 documents).
// The fallback emits a deprecation log line so the call site can be migrated.
func (s *Server) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.UpdateTaskResponse, error) {
	incoming := protoToTask(req.Task)
	taskID, err := s.resolveTaskID(ctx, req.AgencyId, incoming.ID, incoming.TaskName, req.ProjectName)
	if err != nil {
		return nil, mapError(err)
	}
	incoming.ID = taskID

	toUpdate := incoming
	if paths := req.GetUpdateMask().GetPaths(); len(paths) > 0 {
		current, err := s.mgr.GetTask(ctx, req.AgencyId, taskID)
		if err != nil {
			return nil, mapError(err)
		}
		toUpdate = applyTaskMask(current, incoming, paths)
	} else {
		log.Printf("codevaldwork: UpdateTask: update_mask not set on task=%s — replace-all behaviour is deprecated and will be removed once all callers migrate", taskID)
	}

	task, err := s.mgr.UpdateTask(ctx, req.AgencyId, toUpdate)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.UpdateTaskResponse{Task: taskToProto(task)}, nil
}

// applyTaskMask returns current with only the masked fields overwritten from
// incoming. Path names match the snake_case proto field names of the Task
// message. Unknown paths are logged and skipped — consistent with FieldMask's
// forward-compat semantics and the gRPC API design guide.
func applyTaskMask(current, incoming codevaldwork.Task, paths []string) codevaldwork.Task {
	out := current
	for _, p := range paths {
		switch p {
		case "title":
			out.Title = incoming.Title
		case "description":
			out.Description = incoming.Description
		case "status":
			out.Status = incoming.Status
		case "priority":
			out.Priority = incoming.Priority
		case "tags":
			out.Tags = append([]string(nil), incoming.Tags...)
		case "estimated_hours":
			out.EstimatedHours = incoming.EstimatedHours
		case "context":
			out.Context = incoming.Context
		case "task_name":
			out.TaskName = incoming.TaskName
		case "project_name":
			out.ProjectName = incoming.ProjectName
		case "separate_branch":
			out.SeparateBranch = incoming.SeparateBranch
		case "branch_name":
			out.BranchName = incoming.BranchName
		case "due_at":
			out.DueAt = incoming.DueAt
		case "completed_at":
			out.CompletedAt = incoming.CompletedAt
		default:
			log.Printf("codevaldwork: UpdateTask: ignoring unknown update_mask path %q", p)
		}
	}
	return out
}

// DeleteTask implements pb.TaskServiceServer.
func (s *Server) DeleteTask(ctx context.Context, req *pb.DeleteTaskRequest) (*pb.DeleteTaskResponse, error) {
	taskID, err := s.resolveTaskID(ctx, req.AgencyId, req.TaskId, req.TaskName, req.ProjectName)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.mgr.DeleteTask(ctx, req.AgencyId, taskID); err != nil {
		return nil, mapError(err)
	}
	return &pb.DeleteTaskResponse{}, nil
}

// GetTaskByName implements pb.TaskServiceServer.
func (s *Server) GetTaskByName(ctx context.Context, req *pb.GetTaskByNameRequest) (*pb.GetTaskByNameResponse, error) {
	task, err := s.mgr.GetTaskByName(ctx, req.AgencyId, req.ProjectName, req.TaskName)
	if err != nil {
		// Fallback: value might be a UUID from a pre-name caller.
		task, err = s.mgr.GetTask(ctx, req.AgencyId, req.TaskName)
		if err != nil {
			return nil, mapError(err)
		}
	}
	return &pb.GetTaskByNameResponse{Task: taskToProto(task)}, nil
}

// CreateTaskInProject implements pb.TaskServiceServer.
func (s *Server) CreateTaskInProject(ctx context.Context, req *pb.CreateTaskInProjectRequest) (*pb.CreateTaskInProjectResponse, error) {
	task, err := s.mgr.CreateTaskInProject(ctx, req.AgencyId, req.ProjectName, protoToTask(req.Task))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.CreateTaskInProjectResponse{Task: taskToProto(task)}, nil
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

// FailTodo implements pb.TaskServiceServer.
// Marks the todo as failed and runs blockDependentTodos + maybeCompleteParentTask
// via the wired TaskEventDispatcher. Returns FAILED_PRECONDITION if no dispatcher
// has been wired (server constructed without WithDispatcher).
func (s *Server) FailTodo(ctx context.Context, req *pb.FailTodoRequest) (*pb.FailTodoResponse, error) {
	if s.dispatcher == nil {
		return nil, mapError(codevaldwork.ErrInvalidTask) // dispatcher not wired
	}
	if err := s.dispatcher.FailTodoWithCascade(ctx, req.GetTodoId()); err != nil {
		return nil, mapError(err)
	}
	return &pb.FailTodoResponse{}, nil
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
		TaskName:       t.TaskName,
		ProjectName:    t.ProjectName,
		SeparateBranch: t.SeparateBranch,
		BranchName:     t.BranchName,
	}
	if t.CreatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, t.CreatedAt); err == nil {
			pt.CreatedAt = timestamppb.New(ts)
		}
	}
	if t.UpdatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, t.UpdatedAt); err == nil {
			pt.UpdatedAt = timestamppb.New(ts)
		}
	}
	if t.CompletedAt != "" {
		if ts, err := time.Parse(time.RFC3339, t.CompletedAt); err == nil {
			pt.CompletedAt = timestamppb.New(ts)
		}
	}
	if t.DueAt != "" {
		if ts, err := time.Parse(time.RFC3339, t.DueAt); err == nil {
			pt.DueAt = timestamppb.New(ts)
		}
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
		TaskName:       pt.TaskName,
		ProjectName:    pt.ProjectName,
		SeparateBranch: pt.SeparateBranch,
		BranchName:     pt.BranchName,
	}
	if pt.CreatedAt != nil {
		t.CreatedAt = pt.CreatedAt.AsTime().UTC().Format(time.RFC3339)
	}
	if pt.UpdatedAt != nil {
		t.UpdatedAt = pt.UpdatedAt.AsTime().UTC().Format(time.RFC3339)
	}
	if pt.CompletedAt != nil {
		t.CompletedAt = pt.CompletedAt.AsTime().UTC().Format(time.RFC3339)
	}
	if pt.DueAt != nil {
		t.DueAt = pt.DueAt.AsTime().UTC().Format(time.RFC3339)
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
