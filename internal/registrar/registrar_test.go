package registrar

import (
	"sort"
	"strings"
	"testing"

	"github.com/aosanya/CodeValdSharedLib/types"
)

// TestWorkRoutes_StaticCapabilitySnapshot pins the exact set of static HTTP
// capabilities exposed via Cross's dynamic proxy. Adding a new RPC must update
// both this list and the workRoutes table so callers can review the surface
// in a single diff.
func TestWorkRoutes_StaticCapabilitySnapshot(t *testing.T) {
	want := []string{
		// Task lifecycle (Phase 1)
		"create_task",
		"list_tasks",
		"search_tasks",
		"get_task",
		"update_task",
		"delete_task",
		// Agent + assignment (WORK-010)
		"assign_task",
		"unassign_task",
		"upsert_agent",
		"get_agent",
		"list_agents",
		// Project CRUD + membership (WORK-012)
		"create_project",
		"get_project",
		"update_project",
		"delete_project",
		"list_projects",
		"add_task_to_project",
		"remove_task_from_project",
		"list_tasks_in_project",
		"list_projects_for_task",
		// JSON import
		"import_project",
		// Generic graph relationships (WORK-009)
		"create_relationship",
		"delete_relationship",
		"traverse_relationships",
	}

	got := staticCapabilities(t)

	if len(got) != len(want) {
		t.Fatalf("static route count = %d, want %d\n got: %v\nwant: %v",
			len(got), len(want), got, want)
	}

	gotSorted := append([]string(nil), got...)
	wantSorted := append([]string(nil), want...)
	sort.Strings(gotSorted)
	sort.Strings(wantSorted)
	for i := range gotSorted {
		if gotSorted[i] != wantSorted[i] {
			t.Errorf("capability[%d] = %q, want %q", i, gotSorted[i], wantSorted[i])
		}
	}
}

// TestWorkRoutes_GrpcMethodPaths verifies every static route maps to a
// /codevaldwork.v1.TaskService/<Method> path. Cross can't proxy a route whose
// gRPC method path is malformed, so this catches typos before a heartbeat
// reaches Cross at runtime.
func TestWorkRoutes_GrpcMethodPaths(t *testing.T) {
	for _, r := range staticRoutes() {
		if !strings.HasPrefix(r.GrpcMethod, "/codevaldwork.v1.TaskService/") {
			t.Errorf("route %s %s: GrpcMethod=%q does not target TaskService",
				r.Method, r.Pattern, r.GrpcMethod)
		}
	}
}

// TestWorkRoutes_IsWriteFlags pins the read/write classification of every
// static route. The flag drives Cross's interim Basic-auth gate, so any change
// here must be a deliberate one.
func TestWorkRoutes_IsWriteFlags(t *testing.T) {
	writes := map[string]bool{
		"create_task":            true,
		"update_task":            true,
		"delete_task":            true,
		"assign_task":            true,
		"unassign_task":          true,
		"upsert_agent":           true,
		"create_project":           true,
		"update_project":           true,
		"delete_project":           true,
		"add_task_to_project":      true,
		"remove_task_from_project": true,
		"import_project":           true,
		"create_relationship":    true,
		"delete_relationship":    true,
	}
	for _, r := range staticRoutes() {
		if r.IsWrite != writes[r.Capability] {
			t.Errorf("route %s: IsWrite=%v, want %v", r.Capability, r.IsWrite, writes[r.Capability])
		}
	}
}

// TestWorkRoutes_PathBindingSpotChecks verifies the trickier path bindings —
// the nested-field bindings on UpsertAgent / UpdateProject, plus the
// multi-segment delete_relationship pattern.
func TestWorkRoutes_PathBindingSpotChecks(t *testing.T) {
	cases := []struct {
		capability string
		want       []types.PathBinding
	}{
		{
			capability: "upsert_agent",
			want: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
				{URLParam: "agentId", Field: "agent.agent_id"},
			},
		},
		{
			capability: "get_agent",
			want: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
				{URLParam: "agentId", Field: "agent_id"},
			},
		},
		{
			capability: "update_project",
			want: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
				{URLParam: "projectName", Field: "project.project_name"},
			},
		},
		{
			capability: "assign_task",
			want: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
				{URLParam: "taskId", Field: "task_id"},
				{URLParam: "agentId", Field: "agent_id"},
			},
		},
		{
			capability: "delete_relationship",
			want: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
				{URLParam: "label", Field: "label"},
				{URLParam: "fromId", Field: "from_id"},
				{URLParam: "toId", Field: "to_id"},
			},
		},
		{
			capability: "traverse_relationships",
			want: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
				{URLParam: "vertexId", Field: "vertex_id"},
				{URLParam: "label", Field: "label"},
			},
		},
	}
	byCap := indexByCapability(staticRoutes())
	for _, tc := range cases {
		r, ok := byCap[tc.capability]
		if !ok {
			t.Errorf("missing route for capability %q", tc.capability)
			continue
		}
		if !pathBindingsEqual(r.PathBindings, tc.want) {
			t.Errorf("%s bindings = %#v, want %#v",
				tc.capability, r.PathBindings, tc.want)
		}
	}
}

// staticRoutes returns the routes declared by the registrar (excluding the
// schema-driven dynamic routes, which are covered by schemaroutes' own tests).
func staticRoutes() []types.RouteInfo {
	out := taskRoutes()
	out = append(out, agentRoutes()...)
	out = append(out, projectRoutes()...)
	out = append(out, relationshipRoutes()...)
	return out
}

func staticCapabilities(t *testing.T) []string {
	t.Helper()
	rs := staticRoutes()
	caps := make([]string, 0, len(rs))
	for _, r := range rs {
		if r.Capability == "" {
			t.Errorf("route %s %s missing capability", r.Method, r.Pattern)
			continue
		}
		caps = append(caps, r.Capability)
	}
	return caps
}

func indexByCapability(rs []types.RouteInfo) map[string]types.RouteInfo {
	out := make(map[string]types.RouteInfo, len(rs))
	for _, r := range rs {
		out[r.Capability] = r
	}
	return out
}

func pathBindingsEqual(a, b []types.PathBinding) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
