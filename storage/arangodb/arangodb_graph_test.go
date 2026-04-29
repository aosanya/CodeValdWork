// Package arangodb_test — graph-edge integration tests.
//
// Split out of arangodb_test.go to keep each file under the 500-line
// guideline. Shared helpers (openTestManager, uniqueAgency, envOrDefault)
// live in arangodb_test.go.
package arangodb_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
)

func TestArangoDB_Relationship_RoundTrip(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("rel")

	taskA, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{})
	if err != nil {
		t.Fatalf("CreateTask A: %v", err)
	}
	taskB, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{})
	if err != nil {
		t.Fatalf("CreateTask B: %v", err)
	}

	created, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
		Label:  codevaldwork.RelLabelBlocks,
		FromID: taskA.ID,
		ToID:   taskB.ID,
		Properties: map[string]any{
			"reason": "A must finish first",
		},
	})
	if err != nil {
		t.Fatalf("CreateRelationship: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created edge missing ID")
	}

	// Outbound traversal from A — should yield the A→B edge.
	outbound, err := mgr.TraverseRelationships(ctx, agency, taskA.ID, codevaldwork.RelLabelBlocks, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships outbound: %v", err)
	}
	if len(outbound) != 1 {
		t.Fatalf("want 1 outbound edge from A, got %d", len(outbound))
	}
	if outbound[0].FromID != taskA.ID || outbound[0].ToID != taskB.ID {
		t.Errorf("wrong edge: %+v", outbound[0])
	}

	// Inbound traversal on B — should also yield exactly one edge.
	inbound, err := mgr.TraverseRelationships(ctx, agency, taskB.ID, codevaldwork.RelLabelBlocks, codevaldwork.DirectionInbound)
	if err != nil {
		t.Fatalf("TraverseRelationships inbound: %v", err)
	}
	if len(inbound) != 1 {
		t.Fatalf("want 1 inbound edge on B, got %d", len(inbound))
	}

	// Idempotent re-create.
	again, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: taskA.ID, ToID: taskB.ID,
	})
	if err != nil {
		t.Fatalf("second CreateRelationship: %v", err)
	}
	if again.ID != created.ID {
		t.Errorf("idempotent re-create returned new edge: first=%s second=%s", created.ID, again.ID)
	}

	if err := mgr.DeleteRelationship(ctx, agency, taskA.ID, taskB.ID, codevaldwork.RelLabelBlocks); err != nil {
		t.Fatalf("DeleteRelationship: %v", err)
	}

	// After delete, traversal should yield no edges.
	after, err := mgr.TraverseRelationships(ctx, agency, taskA.ID, codevaldwork.RelLabelBlocks, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("TraverseRelationships after delete: %v", err)
	}
	if len(after) != 0 {
		t.Errorf("want 0 edges after delete, got %d", len(after))
	}

	// Deleting again returns ErrRelationshipNotFound.
	if err := mgr.DeleteRelationship(ctx, agency, taskA.ID, taskB.ID, codevaldwork.RelLabelBlocks); !errors.Is(err, codevaldwork.ErrRelationshipNotFound) {
		t.Errorf("second DeleteRelationship: got %v, want ErrRelationshipNotFound", err)
	}
}

// TestArangoDB_AgentAssignment_RoundTrip exercises the WORK-010 Agent +
// AssignTask surface end to end against a live ArangoDB: upsert two Agents
// (verify the second upsert with the same agentID merges rather than
// duplicating), assign a Task to A1, reassign to A2, verify only one outbound
// edge remains, then unassign.
func TestArangoDB_AgentAssignment_RoundTrip(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("assign")

	a1, err := mgr.UpsertAgent(ctx, agency, codevaldwork.Agent{
		AgentID: "agent-1", DisplayName: "First", Capability: "code",
	})
	if err != nil {
		t.Fatalf("UpsertAgent a1: %v", err)
	}

	// Idempotent upsert (same agentID, different display name).
	a1again, err := mgr.UpsertAgent(ctx, agency, codevaldwork.Agent{
		AgentID: "agent-1", DisplayName: "Renamed", Capability: "code",
	})
	if err != nil {
		t.Fatalf("UpsertAgent a1 again: %v", err)
	}
	if a1again.ID != a1.ID {
		t.Errorf("upsert created new vertex: first=%s second=%s", a1.ID, a1again.ID)
	}
	if a1again.DisplayName != "Renamed" {
		t.Errorf("merge did not patch displayName: %+v", a1again)
	}

	a2, err := mgr.UpsertAgent(ctx, agency, codevaldwork.Agent{
		AgentID: "agent-2", DisplayName: "Second",
	})
	if err != nil {
		t.Fatalf("UpsertAgent a2: %v", err)
	}

	task, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := mgr.AssignTask(ctx, agency, task.ID, a1.ID); err != nil {
		t.Fatalf("AssignTask a1: %v", err)
	}
	edges, err := mgr.TraverseRelationships(ctx, agency, task.ID, codevaldwork.RelLabelAssignedTo, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("traverse after a1: %v", err)
	}
	if len(edges) != 1 || edges[0].ToID != a1.ID {
		t.Fatalf("after a1 assignment: edges=%v", edges)
	}

	// Reassign to a2 — should replace the prior edge.
	if err := mgr.AssignTask(ctx, agency, task.ID, a2.ID); err != nil {
		t.Fatalf("AssignTask a2: %v", err)
	}
	edges, err = mgr.TraverseRelationships(ctx, agency, task.ID, codevaldwork.RelLabelAssignedTo, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("traverse after a2: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("want 1 edge after reassign, got %d", len(edges))
	}
	if edges[0].ToID != a2.ID {
		t.Errorf("after reassign: edge points to %s, want %s", edges[0].ToID, a2.ID)
	}

	// Unassign — edge gone.
	if err := mgr.UnassignTask(ctx, agency, task.ID); err != nil {
		t.Fatalf("UnassignTask: %v", err)
	}
	edges, err = mgr.TraverseRelationships(ctx, agency, task.ID, codevaldwork.RelLabelAssignedTo, codevaldwork.DirectionOutbound)
	if err != nil {
		t.Fatalf("traverse after unassign: %v", err)
	}
	if len(edges) != 0 {
		t.Errorf("want 0 edges after unassign, got %d", len(edges))
	}

	// Unassign again — idempotent.
	if err := mgr.UnassignTask(ctx, agency, task.ID); err != nil {
		t.Errorf("UnassignTask second call: got %v, want nil", err)
	}
}

// TestArangoDB_Project_RoundTrip exercises the WORK-012 Project +
// member_of surface end to end against a live ArangoDB: create a project,
// add two task members, verify membership listings in both directions,
// idempotent re-add, remove one, then DeleteProject and verify the project
// is gone, all member_of edges are gone, and the member tasks themselves
// still exist.
func TestArangoDB_Project_RoundTrip(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("project")

	p, err := mgr.CreateProject(ctx, agency, codevaldwork.Project{
		Name: "Sprint 7", Description: "Q2 push", GithubRepo: "aosanya/CodeValdWork",
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	t1, _ := mgr.CreateTask(ctx, agency, codevaldwork.Task{})
	t2, _ := mgr.CreateTask(ctx, agency, codevaldwork.Task{})

	if err := mgr.AddTaskToProject(ctx, agency, t1.ID, p.ID); err != nil {
		t.Fatalf("AddTaskToProject t1: %v", err)
	}
	if err := mgr.AddTaskToProject(ctx, agency, t2.ID, p.ID); err != nil {
		t.Fatalf("AddTaskToProject t2: %v", err)
	}

	// Idempotent re-add.
	if err := mgr.AddTaskToProject(ctx, agency, t1.ID, p.ID); err != nil {
		t.Errorf("idempotent AddTaskToProject: %v", err)
	}

	tasks, err := mgr.ListTasksInProject(ctx, agency, p.ID)
	if err != nil {
		t.Fatalf("ListTasksInProject: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("want 2 members, got %d", len(tasks))
	}

	projects, err := mgr.ListProjectsForTask(ctx, agency, t1.ID)
	if err != nil {
		t.Fatalf("ListProjectsForTask: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != p.ID {
		t.Errorf("ListProjectsForTask: got %v, want [%s]", projects, p.ID)
	}

	if err := mgr.RemoveTaskFromProject(ctx, agency, t1.ID, p.ID); err != nil {
		t.Fatalf("RemoveTaskFromProject: %v", err)
	}
	tasks, _ = mgr.ListTasksInProject(ctx, agency, p.ID)
	if len(tasks) != 1 || tasks[0].ID != t2.ID {
		t.Fatalf("after remove: want only t2, got %v", tasks)
	}

	if err := mgr.DeleteProject(ctx, agency, p.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	if _, err := mgr.GetProject(ctx, agency, p.ID); !errors.Is(err, codevaldwork.ErrProjectNotFound) {
		t.Errorf("project still resolves: got %v, want ErrProjectNotFound", err)
	}
	for _, id := range []string{t1.ID, t2.ID} {
		if _, err := mgr.GetTask(ctx, agency, id); err != nil {
			t.Errorf("task %s should survive project delete: %v", id, err)
		}
	}
	// Inbound member_of from a non-existent project returns no edges.
	edges, _ := mgr.TraverseRelationships(ctx, agency, p.ID, codevaldwork.RelLabelMemberOf, codevaldwork.DirectionInbound)
	if len(edges) != 0 {
		t.Errorf("want 0 inbound member_of edges after project delete, got %d", len(edges))
	}
}
