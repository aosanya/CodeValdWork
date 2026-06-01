// Tests for the field-mask aware UpdateTask handler (BUG-09-023).
// The legacy replace-all path silently wipes fields not present in the
// incoming Task because of the proto3 zero-value footgun. With update_mask
// set, only listed fields are overwritten; without it, the handler falls
// back to the deprecated replace-all behaviour.
package server_test

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
)

// seedTaskWithFields sets the title + description on a freshly created Task
// via the legacy (no-mask) UpdateTask path so subsequent partial PUTs have
// something to preserve.
func seedTaskWithFields(t *testing.T, srv pb.TaskServiceServer, agencyID, title, description string) *pb.Task {
	t.Helper()
	ctx := context.Background()
	created := createProtoTask(t, srv, agencyID, "seed")
	created.Title = title
	created.Description = description
	res, err := srv.UpdateTask(ctx, &pb.UpdateTaskRequest{AgencyId: agencyID, Task: created})
	if err != nil {
		t.Fatalf("seed UpdateTask: %v", err)
	}
	return res.Task
}

func TestUpdateTask_WithMask_PreservesUnlistedFields(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	seeded := seedTaskWithFields(t, srv, "ag", "Project Scaffolding", "flutter create sharedfarms; ...")

	// Mimic the BUG-09-023 reproducer: PUT only sends id + status + branch_name.
	// With update_mask listing exactly those fields, title and description
	// must survive untouched.
	partial := &pb.Task{
		Id:         seeded.Id,
		Status:     pb.TaskStatus_TASK_STATUS_IN_PROGRESS,
		BranchName: "feature/SF-001_scaffolding",
	}
	res, err := srv.UpdateTask(ctx, &pb.UpdateTaskRequest{
		AgencyId:   "ag",
		Task:       partial,
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status", "branch_name"}},
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	if res.Task.Title != seeded.Title {
		t.Errorf("title wiped: got %q, want %q", res.Task.Title, seeded.Title)
	}
	if res.Task.Description != seeded.Description {
		t.Errorf("description wiped: got %q, want %q", res.Task.Description, seeded.Description)
	}
	if res.Task.Status != pb.TaskStatus_TASK_STATUS_IN_PROGRESS {
		t.Errorf("status not updated: got %v", res.Task.Status)
	}
	if res.Task.BranchName != "feature/SF-001_scaffolding" {
		t.Errorf("branch_name not updated: got %q", res.Task.BranchName)
	}
}

func TestUpdateTask_WithMask_ClearsExplicitlyMaskedField(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	seeded := seedTaskWithFields(t, srv, "ag", "kept-title", "kept-desc")
	// First set a branch_name so we can verify the mask clears it.
	seeded.BranchName = "feature/old"
	if _, err := srv.UpdateTask(ctx, &pb.UpdateTaskRequest{AgencyId: "ag", Task: seeded}); err != nil {
		t.Fatalf("UpdateTask seed branch: %v", err)
	}

	// Mask includes branch_name but the incoming Task leaves it empty.
	res, err := srv.UpdateTask(ctx, &pb.UpdateTaskRequest{
		AgencyId:   "ag",
		Task:       &pb.Task{Id: seeded.Id, Status: pb.TaskStatus_TASK_STATUS_PENDING},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"branch_name"}},
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if res.Task.BranchName != "" {
		t.Errorf("branch_name not cleared by mask: got %q", res.Task.BranchName)
	}
	if res.Task.Title != "kept-title" {
		t.Errorf("title wiped: got %q", res.Task.Title)
	}
	if res.Task.Description != "kept-desc" {
		t.Errorf("description wiped: got %q", res.Task.Description)
	}
}

func TestUpdateTask_WithoutMask_ReplaceAllStillWipes(t *testing.T) {
	// Documents the legacy proto3 behaviour: when update_mask is unset, the
	// handler logs a deprecation warning and writes every field of the
	// incoming Task — so omitted fields are silently zeroed. This is the
	// foot-gun BUG-09-023 callers must avoid by sending update_mask.
	srv := newTestServer()
	ctx := context.Background()
	seeded := seedTaskWithFields(t, srv, "ag", "wipe-me-title", "wipe-me-desc")

	partial := &pb.Task{
		Id:         seeded.Id,
		Status:     pb.TaskStatus_TASK_STATUS_IN_PROGRESS,
		BranchName: "feature/SF-001",
	}
	res, err := srv.UpdateTask(ctx, &pb.UpdateTaskRequest{AgencyId: "ag", Task: partial})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if res.Task.Title != "" {
		t.Errorf("legacy replace-all should wipe title, got %q", res.Task.Title)
	}
	if res.Task.Description != "" {
		t.Errorf("legacy replace-all should wipe description, got %q", res.Task.Description)
	}
}

func TestUpdateTask_WithMask_UnknownPathIsSkipped(t *testing.T) {
	srv := newTestServer()
	ctx := context.Background()
	seeded := seedTaskWithFields(t, srv, "ag", "still-here", "also-here")

	res, err := srv.UpdateTask(ctx, &pb.UpdateTaskRequest{
		AgencyId:   "ag",
		Task:       &pb.Task{Id: seeded.Id, Status: pb.TaskStatus_TASK_STATUS_IN_PROGRESS},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"status", "no_such_field"}},
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	if res.Task.Title != "still-here" || res.Task.Description != "also-here" {
		t.Errorf("unknown mask path should not affect untouched fields: got title=%q desc=%q", res.Task.Title, res.Task.Description)
	}
	if res.Task.Status != pb.TaskStatus_TASK_STATUS_IN_PROGRESS {
		t.Errorf("status not updated: got %v", res.Task.Status)
	}
}
