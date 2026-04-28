// Package arangodb_test — Phase 2 end-to-end scenarios.
//
// Three WORK-016 scenarios live here that don't fit the round-trip shape of
// arangodb_graph_test.go: schema seeding, subtask hierarchy traversal, and
// the blocker gate driven through real status transitions. Other scenarios
// (assignment, group membership) stay in arangodb_graph_test.go.
package arangodb_test

import (
	"context"
	"errors"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	"github.com/aosanya/CodeValdWork/storage/arangodb"
)

// TestArangoDB_SchemaSeed_RegistersCollectionsAndGraph asserts that
// constructing a Backend against a fresh database lands all five Work
// collections (`work_entities`, `work_tasks`, `work_groups`, `work_agents`,
// `work_relationships`) plus the `work_graph` named graph with
// `work_relationships` registered as the edge collection. This is the
// load-bearing seeding contract — a regression here surfaces as runtime
// errors at the first CreateEntity / CreateRelationship call.
func TestArangoDB_SchemaSeed_RegistersCollectionsAndGraph(t *testing.T) {
	db := openTestDB(t)
	if _, err := arangodb.NewBackendFromDB(db, codevaldwork.DefaultWorkSchema()); err != nil {
		t.Fatalf("NewBackendFromDB: %v", err)
	}
	ctx := context.Background()

	wantCollections := []string{
		"work_entities",
		"work_tasks",
		"work_groups",
		"work_agents",
		"work_relationships",
	}
	for _, name := range wantCollections {
		ok, err := db.CollectionExists(ctx, name)
		if err != nil {
			t.Fatalf("CollectionExists(%q): %v", name, err)
		}
		if !ok {
			t.Errorf("collection %q not seeded", name)
		}
	}

	g, err := db.Graph(ctx, "work_graph")
	if err != nil {
		t.Fatalf("Graph(work_graph): %v", err)
	}
	defs := g.EdgeDefinitions()
	var found bool
	for _, ed := range defs {
		if ed.Collection == "work_relationships" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("work_graph does not register work_relationships as an edge collection (defs=%v)", defs)
	}
}

// TestArangoDB_SubtaskHierarchy_TraverseInboundReturnsChildren creates a
// parent Task plus two child Tasks linked via `subtask_of` (children →
// parent). Inbound traversal on the parent must surface both children.
// `subtask_of` is functional (`ToMany: false`); the test uses two distinct
// children to validate the inbound direction itself.
func TestArangoDB_SubtaskHierarchy_TraverseInboundReturnsChildren(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("subtask")

	parent, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "parent"})
	if err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}
	c1, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "c1"})
	if err != nil {
		t.Fatalf("CreateTask c1: %v", err)
	}
	c2, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "c2"})
	if err != nil {
		t.Fatalf("CreateTask c2: %v", err)
	}

	for _, child := range []string{c1.ID, c2.ID} {
		if _, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
			Label:  codevaldwork.RelLabelSubtaskOf,
			FromID: child,
			ToID:   parent.ID,
		}); err != nil {
			t.Fatalf("subtask_of from %s: %v", child, err)
		}
	}

	edges, err := mgr.TraverseRelationships(ctx, agency, parent.ID,
		codevaldwork.RelLabelSubtaskOf, codevaldwork.DirectionInbound)
	if err != nil {
		t.Fatalf("TraverseRelationships inbound: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("want 2 inbound subtask_of edges on parent, got %d (%v)", len(edges), edges)
	}
	seen := map[string]bool{}
	for _, e := range edges {
		if e.ToID != parent.ID {
			t.Errorf("edge %s ToID = %s, want parent %s", e.ID, e.ToID, parent.ID)
		}
		seen[e.FromID] = true
	}
	if !seen[c1.ID] || !seen[c2.ID] {
		t.Errorf("missing children in traversal: seen=%v want %s and %s", seen, c1.ID, c2.ID)
	}
}

// TestArangoDB_BlockerGate_BlocksThenOpens drives the WORK-011 hard blocker
// end-to-end against a live ArangoDB. Phase 2 unit coverage in blocker_test.go
// uses the in-memory fakeDataManager; this scenario validates that the same
// gate behaves correctly when the inbound `blocks` traversal runs against
// real AQL.
func TestArangoDB_BlockerGate_BlocksThenOpens(t *testing.T) {
	mgr := openTestManager(t)
	ctx := context.Background()
	agency := uniqueAgency("blocker")

	a, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "blocker"})
	if err != nil {
		t.Fatalf("CreateTask a: %v", err)
	}
	b, err := mgr.CreateTask(ctx, agency, codevaldwork.Task{Title: "blocked"})
	if err != nil {
		t.Fatalf("CreateTask b: %v", err)
	}
	if _, err := mgr.CreateRelationship(ctx, agency, codevaldwork.Relationship{
		Label: codevaldwork.RelLabelBlocks, FromID: a.ID, ToID: b.ID,
	}); err != nil {
		t.Fatalf("CreateRelationship blocks: %v", err)
	}

	// First attempt: A still pending, B → in_progress must surface ErrBlocked
	// with BlockerTaskIDs = [A.ID].
	b.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agency, b); !errors.Is(err, codevaldwork.ErrBlocked) {
		t.Fatalf("first attempt: want ErrBlocked, got %v", err)
	} else {
		var be *codevaldwork.BlockedError
		if !errors.As(err, &be) {
			t.Fatalf("first attempt: want *BlockedError, got %T", err)
		}
		if len(be.BlockerTaskIDs) != 1 || be.BlockerTaskIDs[0] != a.ID {
			t.Errorf("BlockerTaskIDs = %v, want [%s]", be.BlockerTaskIDs, a.ID)
		}
	}

	// Drive A through pending → in_progress → completed.
	a.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agency, a); err != nil {
		t.Fatalf("a → in_progress: %v", err)
	}
	a.Status = codevaldwork.TaskStatusCompleted
	if _, err := mgr.UpdateTask(ctx, agency, a); err != nil {
		t.Fatalf("a → completed: %v", err)
	}

	// Refetch B (we mutated the local copy above; the server still sees pending).
	b, err = mgr.GetTask(ctx, agency, b.ID)
	if err != nil {
		t.Fatalf("refetch b: %v", err)
	}
	b.Status = codevaldwork.TaskStatusInProgress
	if _, err := mgr.UpdateTask(ctx, agency, b); err != nil {
		t.Errorf("b → in_progress after blocker completed: got %v, want nil", err)
	}
}
