package server_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

func createProtoGroup(t *testing.T, srv pb.TaskServiceServer, agencyID, name string) *pb.TaskGroup {
	t.Helper()
	res, err := srv.CreateTaskGroup(context.Background(), &pb.CreateTaskGroupRequest{
		AgencyId: agencyID,
		Group:    &pb.TaskGroup{Name: name},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}
	return res.Group
}

func TestCreateTaskGroup_EmptyName_ReturnsInvalidArgument(t *testing.T) {
	srv := newTestServer()
	_, err := srv.CreateTaskGroup(context.Background(), &pb.CreateTaskGroupRequest{
		AgencyId: "ag",
		Group:    &pb.TaskGroup{},
	})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
}

func TestCreateTaskGroup_RoundTrip(t *testing.T) {
	srv := newTestServer()
	res, err := srv.CreateTaskGroup(context.Background(), &pb.CreateTaskGroupRequest{
		AgencyId: "ag",
		Group:    &pb.TaskGroup{Name: "Sprint", Description: "Q2"},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}
	if res.Group.Id == "" {
		t.Error("group missing ID")
	}
	if res.Group.Name != "Sprint" || res.Group.Description != "Q2" {
		t.Errorf("unexpected group: %+v", res.Group)
	}
}

func TestGetTaskGroup_NotFound(t *testing.T) {
	srv := newTestServer()
	_, err := srv.GetTaskGroup(context.Background(), &pb.GetTaskGroupRequest{
		AgencyId: "ag", TaskGroupId: "missing",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestUpdateTaskGroup_PatchesAndReturnsUpdated(t *testing.T) {
	srv := newTestServer()
	g := createProtoGroup(t, srv, "ag", "Old")

	res, err := srv.UpdateTaskGroup(context.Background(), &pb.UpdateTaskGroupRequest{
		AgencyId: "ag",
		Group:    &pb.TaskGroup{Id: g.Id, Name: "New", Description: "patched"},
	})
	if err != nil {
		t.Fatalf("UpdateTaskGroup: %v", err)
	}
	if res.Group.Name != "New" || res.Group.Description != "patched" {
		t.Errorf("unexpected: %+v", res.Group)
	}
	if res.Group.Id != g.Id {
		t.Error("update created new vertex")
	}
}

func TestDeleteTaskGroup_NotFound(t *testing.T) {
	srv := newTestServer()
	_, err := srv.DeleteTaskGroup(context.Background(), &pb.DeleteTaskGroupRequest{
		AgencyId: "ag", TaskGroupId: "missing",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestListTaskGroups_AgencyIsolation(t *testing.T) {
	srv := newTestServer()
	createProtoGroup(t, srv, "agency-A", "g1")
	createProtoGroup(t, srv, "agency-A", "g2")
	createProtoGroup(t, srv, "agency-B", "g3")

	a, _ := srv.ListTaskGroups(context.Background(), &pb.ListTaskGroupsRequest{AgencyId: "agency-A"})
	if len(a.Groups) != 2 {
		t.Errorf("agency-A: want 2, got %d", len(a.Groups))
	}
	b, _ := srv.ListTaskGroups(context.Background(), &pb.ListTaskGroupsRequest{AgencyId: "agency-B"})
	if len(b.Groups) != 1 {
		t.Errorf("agency-B: want 1, got %d", len(b.Groups))
	}
}

func TestAddTaskToGroup_ListTasksInGroup_RoundTrip(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	g := createProtoGroup(t, srv, "ag", "Sprint")
	t1 := createProtoTask(t, srv, "ag", "task-1")

	if _, err := srv.AddTaskToGroup(ctx, &pb.AddTaskToGroupRequest{
		AgencyId: "ag", TaskId: t1.Id, TaskGroupId: g.Id,
	}); err != nil {
		t.Fatalf("AddTaskToGroup: %v", err)
	}

	res, err := srv.ListTasksInGroup(ctx, &pb.ListTasksInGroupRequest{
		AgencyId: "ag", TaskGroupId: g.Id,
	})
	if err != nil {
		t.Fatalf("ListTasksInGroup: %v", err)
	}
	if len(res.Tasks) != 1 || res.Tasks[0].Id != t1.Id {
		t.Errorf("got %v, want [%s]", res.Tasks, t1.Id)
	}
}

func TestRemoveTaskFromGroup_NoMembership_ReturnsNotFound(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	g := createProtoGroup(t, srv, "ag", "Sprint")
	t1 := createProtoTask(t, srv, "ag", "task-1")

	_, err := srv.RemoveTaskFromGroup(ctx, &pb.RemoveTaskFromGroupRequest{
		AgencyId: "ag", TaskId: t1.Id, TaskGroupId: g.Id,
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestListGroupsForTask_Empty(t *testing.T) {
	srv := newTestServer()
	t1 := createProtoTask(t, srv, "ag", "task-1")

	res, err := srv.ListGroupsForTask(context.Background(), &pb.ListGroupsForTaskRequest{
		AgencyId: "ag", TaskId: t1.Id,
	})
	if err != nil {
		t.Fatalf("ListGroupsForTask: %v", err)
	}
	if len(res.Groups) != 0 {
		t.Errorf("want 0 groups, got %d", len(res.Groups))
	}
}
