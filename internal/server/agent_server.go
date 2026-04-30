package server

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// AssignTask implements pb.TaskServiceServer.
func (s *Server) AssignTask(ctx context.Context, req *pb.AssignTaskRequest) (*pb.AssignTaskResponse, error) {
	taskID, err := s.resolveTaskID(ctx, req.AgencyId, req.TaskId, req.TaskName, req.ProjectName)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.mgr.AssignTask(ctx, req.AgencyId, taskID, req.AgentId); err != nil {
		return nil, mapError(err)
	}
	return &pb.AssignTaskResponse{}, nil
}

// UnassignTask implements pb.TaskServiceServer.
func (s *Server) UnassignTask(ctx context.Context, req *pb.UnassignTaskRequest) (*pb.UnassignTaskResponse, error) {
	taskID, err := s.resolveTaskID(ctx, req.AgencyId, req.TaskId, req.TaskName, req.ProjectName)
	if err != nil {
		return nil, mapError(err)
	}
	if err := s.mgr.UnassignTask(ctx, req.AgencyId, taskID); err != nil {
		return nil, mapError(err)
	}
	return &pb.UnassignTaskResponse{}, nil
}

// UpsertAgent implements pb.TaskServiceServer.
func (s *Server) UpsertAgent(ctx context.Context, req *pb.UpsertAgentRequest) (*pb.UpsertAgentResponse, error) {
	agent, err := s.mgr.UpsertAgent(ctx, req.AgencyId, protoToAgent(req.Agent))
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.UpsertAgentResponse{Agent: agentToProto(agent)}, nil
}

// GetAgent implements pb.TaskServiceServer.
func (s *Server) GetAgent(ctx context.Context, req *pb.GetAgentRequest) (*pb.GetAgentResponse, error) {
	agent, err := s.mgr.GetAgent(ctx, req.AgencyId, req.AgentId)
	if err != nil {
		return nil, mapError(err)
	}
	return &pb.GetAgentResponse{Agent: agentToProto(agent)}, nil
}

// ListAgents implements pb.TaskServiceServer.
func (s *Server) ListAgents(ctx context.Context, req *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {
	agents, err := s.mgr.ListAgents(ctx, req.AgencyId)
	if err != nil {
		return nil, mapError(err)
	}
	out := make([]*pb.Agent, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentToProto(a))
	}
	return &pb.ListAgentsResponse{Agents: out}, nil
}

// ── Conversion helpers ────────────────────────────────────────────────────────

func agentToProto(a codevaldwork.Agent) *pb.Agent {
	pt := &pb.Agent{
		Id:          a.ID,
		AgencyId:    a.AgencyID,
		AgentId:     a.AgentID,
		DisplayName: a.DisplayName,
		Capability:  a.Capability,
	}
	if a.CreatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, a.CreatedAt); err == nil {
			pt.CreatedAt = timestamppb.New(ts)
		}
	}
	if a.UpdatedAt != "" {
		if ts, err := time.Parse(time.RFC3339, a.UpdatedAt); err == nil {
			pt.UpdatedAt = timestamppb.New(ts)
		}
	}
	return pt
}

func protoToAgent(pa *pb.Agent) codevaldwork.Agent {
	if pa == nil {
		return codevaldwork.Agent{}
	}
	a := codevaldwork.Agent{
		ID:          pa.Id,
		AgencyID:    pa.AgencyId,
		AgentID:     pa.AgentId,
		DisplayName: pa.DisplayName,
		Capability:  pa.Capability,
	}
	if pa.CreatedAt != nil {
		a.CreatedAt = pa.CreatedAt.AsTime().UTC().Format(time.RFC3339)
	}
	if pa.UpdatedAt != nil {
		a.UpdatedAt = pa.UpdatedAt.AsTime().UTC().Format(time.RFC3339)
	}
	return a
}
