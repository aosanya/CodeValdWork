package server

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// CreateWorkflowRun implements pb.TaskServiceServer (FEAT-20260602-001).
// Mints a new WorkflowRun anchor for an orchestrated execution. When the
// request name is empty the server generates one; when it collides with
// an existing run in the same agency the call fails with ALREADY_EXISTS.
func (s *Server) CreateWorkflowRun(ctx context.Context, req *pb.CreateWorkflowRunRequest) (*pb.CreateWorkflowRunResponse, error) {
	run, err := s.mgr.CreateWorkflowRun(ctx, req.AgencyId, req.Name, req.TriggerEvent, req.Initiator)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.CreateWorkflowRunResponse{Run: workflowRunToProto(run)}, nil
}

// GetWorkflowRun implements pb.TaskServiceServer (FEAT-20260601-001).
// Returns the run plus its full closure: every Task, TaskTodo, and edge
// reachable, plus the IDs of foreign (cross-service) entities the run
// referenced (AgentRun IDs, FunctionsJob IDs, branch names).
func (s *Server) GetWorkflowRun(ctx context.Context, req *pb.GetWorkflowRunRequest) (*pb.GetWorkflowRunResponse, error) {
	closure, err := s.mgr.GetWorkflowRunClosure(ctx, req.AgencyId, req.WorkflowRunId)
	if err != nil {
		return nil, mapError(err)
	}
	tasks := make([]*pb.Task, 0, len(closure.Tasks))
	for _, t := range closure.Tasks {
		tasks = append(tasks, taskToProto(t))
	}
	todos := make([]*pb.TaskTodo, 0, len(closure.Todos))
	for _, td := range closure.Todos {
		todos = append(todos, taskTodoToProto(td))
	}
	edges := make([]*pb.Relationship, 0, len(closure.Edges))
	for _, e := range closure.Edges {
		edges = append(edges, relationshipToProto(e))
	}
	return &pb.GetWorkflowRunResponse{
		Run:            workflowRunToProto(closure.Run),
		Tasks:          tasks,
		Todos:          todos,
		Edges:          edges,
		AgentRunIds:    append([]string(nil), closure.AgentRunIDs...),
		FunctionJobIds: append([]string(nil), closure.FunctionJobIDs...),
		BranchNames:    append([]string(nil), closure.BranchNames...),
	}, nil
}

// ListWorkflowRuns implements pb.TaskServiceServer. When req.Name is
// non-empty the result is filtered to runs whose name matches exactly
// (at most one row, per the schema UniqueKey on name).
func (s *Server) ListWorkflowRuns(ctx context.Context, req *pb.ListWorkflowRunsRequest) (*pb.ListWorkflowRunsResponse, error) {
	runs, err := s.mgr.ListWorkflowRuns(ctx, req.AgencyId, req.Name)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.WorkflowRun, 0, len(runs))
	for _, r := range runs {
		out = append(out, workflowRunToProto(r))
	}
	return &pb.ListWorkflowRunsResponse{Runs: out}, nil
}

func workflowRunToProto(r codevaldwork.WorkflowRun) *pb.WorkflowRun {
	pt := &pb.WorkflowRun{
		Id:             r.ID,
		AgencyId:       r.AgencyID,
		Name:           r.Name,
		Status:         workflowRunStatusToProto(r.Status),
		TriggerEvent:   r.TriggerEvent,
		Initiator:      r.Initiator,
		Notes:          r.Notes,
		AgentRunIds:    append([]string(nil), r.AgentRunIDs...),
		FunctionJobIds: append([]string(nil), r.FunctionJobIDs...),
		BranchNames:    append([]string(nil), r.BranchNames...),
	}
	if r.StartedAt != "" {
		if ts, err := time.Parse(time.RFC3339, r.StartedAt); err == nil {
			pt.StartedAt = timestamppb.New(ts)
		}
	}
	if r.CompletedAt != "" {
		if ts, err := time.Parse(time.RFC3339, r.CompletedAt); err == nil {
			pt.CompletedAt = timestamppb.New(ts)
		}
	}
	if r.CreatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, r.CreatedAt); err == nil {
			pt.CreatedAt = timestamppb.New(ts)
		}
	}
	if r.UpdatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, r.UpdatedAt); err == nil {
			pt.UpdatedAt = timestamppb.New(ts)
		}
	}
	return pt
}

func workflowRunStatusToProto(s codevaldwork.WorkflowRunStatus) pb.WorkflowRunStatus {
	switch s {
	case codevaldwork.WorkflowRunStatusPending:
		return pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_PENDING
	case codevaldwork.WorkflowRunStatusInProgress:
		return pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_IN_PROGRESS
	case codevaldwork.WorkflowRunStatusCompleted:
		return pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_COMPLETED
	case codevaldwork.WorkflowRunStatusFailed:
		return pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_FAILED
	case codevaldwork.WorkflowRunStatusRolledBack:
		return pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_ROLLED_BACK
	default:
		return pb.WorkflowRunStatus_WORKFLOW_RUN_STATUS_UNSPECIFIED
	}
}

func taskTodoToProto(t codevaldwork.TaskTodo) *pb.TaskTodo {
	pt := &pb.TaskTodo{
		Id:             t.ID,
		AgencyId:       t.AgencyID,
		Title:          t.Title,
		Description:    t.Description,
		Instructions:   t.Instructions,
		Ordinality:     int32(t.Ordinality),
		CanRunParallel: t.CanRunParallel,
		Status:         todoStatusToProto(t.Status),
		ParentTaskId:   t.ParentTaskID,
		DecompRunId:    t.DecompRunID,
		AgentId:        t.AgentID,
		Precalls:       t.Precalls,
		WorkflowRunId:  t.WorkflowRunID,
	}
	for _, d := range t.DependsOn {
		pt.DependsOn = append(pt.DependsOn, int32(d))
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
	return pt
}

func todoStatusToProto(s codevaldwork.TodoStatus) pb.TodoStatus {
	switch s {
	case codevaldwork.TodoStatusPending:
		return pb.TodoStatus_TODO_STATUS_PENDING
	case codevaldwork.TodoStatusBlocked:
		return pb.TodoStatus_TODO_STATUS_BLOCKED
	case codevaldwork.TodoStatusDispatched:
		return pb.TodoStatus_TODO_STATUS_DISPATCHED
	case codevaldwork.TodoStatusCompleted:
		return pb.TodoStatus_TODO_STATUS_COMPLETED
	case codevaldwork.TodoStatusFailed:
		return pb.TodoStatus_TODO_STATUS_FAILED
	default:
		return pb.TodoStatus_TODO_STATUS_UNSPECIFIED
	}
}
