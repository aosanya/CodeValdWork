package server_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

func createProtoProject(t *testing.T, srv pb.TaskServiceServer, agencyID, name string) *pb.Project {
	t.Helper()
	res, err := srv.CreateProject(context.Background(), &pb.CreateProjectRequest{
		AgencyId: agencyID,
		Project:  &pb.Project{Name: name},
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return res.Project
}

func TestCreateProject_EmptyName_ReturnsInvalidArgument(t *testing.T) {
	srv := newTestServer()
	_, err := srv.CreateProject(context.Background(), &pb.CreateProjectRequest{
		AgencyId: "ag",
		Project:  &pb.Project{},
	})
	if got := status.Code(err); got != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", got)
	}
}

func TestCreateProject_RoundTrip(t *testing.T) {
	srv := newTestServer()
	res, err := srv.CreateProject(context.Background(), &pb.CreateProjectRequest{
		AgencyId: "ag",
		Project: &pb.Project{
			Name:        "Sprint",
			Description: "Q2",
			GithubRepo:  "aosanya/CodeValdWork",
		},
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if res.Project.Id == "" {
		t.Error("project missing ID")
	}
	if res.Project.Name != "Sprint" || res.Project.Description != "Q2" || res.Project.GithubRepo != "aosanya/CodeValdWork" {
		t.Errorf("unexpected project: %+v", res.Project)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	srv := newTestServer()
	_, err := srv.GetProject(context.Background(), &pb.GetProjectRequest{
		AgencyId: "ag", ProjectId: "missing",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestUpdateProject_PatchesAndReturnsUpdated(t *testing.T) {
	srv := newTestServer()
	p := createProtoProject(t, srv, "ag", "Old")

	res, err := srv.UpdateProject(context.Background(), &pb.UpdateProjectRequest{
		AgencyId: "ag",
		Project:  &pb.Project{Id: p.Id, Name: "New", Description: "patched", GithubRepo: "aosanya/Foo"},
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if res.Project.Name != "New" || res.Project.Description != "patched" || res.Project.GithubRepo != "aosanya/Foo" {
		t.Errorf("unexpected: %+v", res.Project)
	}
	if res.Project.Id != p.Id {
		t.Error("update created new vertex")
	}
}

func TestDeleteProject_NotFound(t *testing.T) {
	srv := newTestServer()
	_, err := srv.DeleteProject(context.Background(), &pb.DeleteProjectRequest{
		AgencyId: "ag", ProjectId: "missing",
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestListProjects_AgencyIsolation(t *testing.T) {
	srv := newTestServer()
	createProtoProject(t, srv, "agency-A", "p1")
	createProtoProject(t, srv, "agency-A", "p2")
	createProtoProject(t, srv, "agency-B", "p3")

	a, _ := srv.ListProjects(context.Background(), &pb.ListProjectsRequest{AgencyId: "agency-A"})
	if len(a.Projects) != 2 {
		t.Errorf("agency-A: want 2, got %d", len(a.Projects))
	}
	b, _ := srv.ListProjects(context.Background(), &pb.ListProjectsRequest{AgencyId: "agency-B"})
	if len(b.Projects) != 1 {
		t.Errorf("agency-B: want 1, got %d", len(b.Projects))
	}
}

func TestAddTaskToProject_ListTasksInProject_RoundTrip(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	p := createProtoProject(t, srv, "ag", "Sprint")
	t1 := createProtoTask(t, srv, "ag", "task-1")

	if _, err := srv.AddTaskToProject(ctx, &pb.AddTaskToProjectRequest{
		AgencyId: "ag", TaskId: t1.Id, ProjectId: p.Id,
	}); err != nil {
		t.Fatalf("AddTaskToProject: %v", err)
	}

	res, err := srv.ListTasksInProject(ctx, &pb.ListTasksInProjectRequest{
		AgencyId: "ag", ProjectId: p.Id,
	})
	if err != nil {
		t.Fatalf("ListTasksInProject: %v", err)
	}
	if len(res.Tasks) != 1 || res.Tasks[0].Id != t1.Id {
		t.Errorf("got %v, want [%s]", res.Tasks, t1.Id)
	}
}

func TestRemoveTaskFromProject_NoMembership_ReturnsNotFound(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	p := createProtoProject(t, srv, "ag", "Sprint")
	t1 := createProtoTask(t, srv, "ag", "task-1")

	_, err := srv.RemoveTaskFromProject(ctx, &pb.RemoveTaskFromProjectRequest{
		AgencyId: "ag", TaskId: t1.Id, ProjectId: p.Id,
	})
	if got := status.Code(err); got != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", got)
	}
}

func TestListProjectsForTask_Empty(t *testing.T) {
	srv := newTestServer()
	t1 := createProtoTask(t, srv, "ag", "task-1")

	res, err := srv.ListProjectsForTask(context.Background(), &pb.ListProjectsForTaskRequest{
		AgencyId: "ag", TaskId: t1.Id,
	})
	if err != nil {
		t.Fatalf("ListProjectsForTask: %v", err)
	}
	if len(res.Projects) != 0 {
		t.Errorf("want 0 projects, got %d", len(res.Projects))
	}
}
