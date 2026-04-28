package server_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// upsertProtoAgent calls UpsertAgent through the server façade and returns
// the proto Agent — used by tests that need an Agent vertex to assign to.
func upsertProtoAgent(t *testing.T, srv pb.TaskServiceServer, agencyID, agentID string) *pb.Agent {
	t.Helper()
	res, err := srv.UpsertAgent(context.Background(), &pb.UpsertAgentRequest{
		AgencyId: agencyID,
		Agent:    &pb.Agent{AgentId: agentID, DisplayName: agentID},
	})
	if err != nil {
		t.Fatalf("UpsertAgent: %v", err)
	}
	return res.Agent
}

func TestUpsertAgent_RoundTripsAndIsKeyedByAgentID(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()

	first, err := srv.UpsertAgent(ctx, &pb.UpsertAgentRequest{
		AgencyId: "ag",
		Agent:    &pb.Agent{AgentId: "agent-1", DisplayName: "First", Capability: "code"},
	})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if first.Agent.Id == "" {
		t.Error("missing entity ID")
	}
	second, err := srv.UpsertAgent(ctx, &pb.UpsertAgentRequest{
		AgencyId: "ag",
		Agent:    &pb.Agent{AgentId: "agent-1", DisplayName: "Second"},
	})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.Agent.Id != first.Agent.Id {
		t.Errorf("upsert created new vertex: first=%s second=%s", first.Agent.Id, second.Agent.Id)
	}
	if second.Agent.DisplayName != "Second" {
		t.Errorf("merge did not patch DisplayName: %+v", second.Agent)
	}
}

func TestGetAgent_NotFound_ReturnsNotFoundCode(t *testing.T) {
	srv := newTestServer()
	_, err := srv.GetAgent(context.Background(), &pb.GetAgentRequest{
		AgencyId: "ag", AgentId: "missing",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestListAgents_Empty_ReturnsEmptySlice(t *testing.T) {
	srv := newTestServer()
	res, err := srv.ListAgents(context.Background(), &pb.ListAgentsRequest{AgencyId: "ag"})
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(res.Agents) != 0 {
		t.Errorf("want 0 agents, got %d", len(res.Agents))
	}
}

func TestAssignTask_HappyPath_NoError(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	task := createProtoTask(t, srv, "ag", "assign-me")
	agent := upsertProtoAgent(t, srv, "ag", "agent-1")

	if _, err := srv.AssignTask(ctx, &pb.AssignTaskRequest{
		AgencyId: "ag", TaskId: task.Id, AgentId: agent.Id,
	}); err != nil {
		t.Fatalf("AssignTask: %v", err)
	}
}

func TestAssignTask_UnknownAgent_ReturnsNotFoundCode(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	task := createProtoTask(t, srv, "ag", "x")

	_, err := srv.AssignTask(ctx, &pb.AssignTaskRequest{
		AgencyId: "ag", TaskId: task.Id, AgentId: "no-such-agent",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestUnassignTask_OnUnassigned_IsIdempotent(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	task := createProtoTask(t, srv, "ag", "x")

	if _, err := srv.UnassignTask(ctx, &pb.UnassignTaskRequest{
		AgencyId: "ag", TaskId: task.Id,
	}); err != nil {
		t.Errorf("UnassignTask on unassigned: got %v, want nil", err)
	}
}

func TestUnassignTask_UnknownTask_ReturnsNotFoundCode(t *testing.T) {
	srv := newTestServer()
	_, err := srv.UnassignTask(context.Background(), &pb.UnassignTaskRequest{
		AgencyId: "ag", TaskId: "no-such-task",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

// Compile-time guard: the agency-isolation semantics carry through the gRPC
// surface — UpsertAgent in agency-A must not be visible from agency-B.
func TestUpsertAgent_AgencyIsolation(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	upsertProtoAgent(t, srv, "agency-A", "shared-id")

	res, _ := srv.ListAgents(ctx, &pb.ListAgentsRequest{AgencyId: "agency-B"})
	if len(res.Agents) != 0 {
		t.Errorf("agency-B sees %d agents from agency-A", len(res.Agents))
	}
	_ = codevaldwork.ErrAgentNotFound // pin import; used implicitly via mapError
}
