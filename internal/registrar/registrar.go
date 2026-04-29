// Package registrar provides the CodeValdWork service registrar.
// It wraps the shared-library heartbeat registrar and additionally implements
// [codevaldwork.CrossPublisher] so the [TaskManager] can notify CodeValdCross
// whenever a task lifecycle event occurs (created, updated, completed).
//
// Construct via [New]; start heartbeats by calling Run in a goroutine; stop
// by cancelling the context then calling Close.
package registrar

import (
	"context"
	"log"
	"time"

	codevaldwork "github.com/aosanya/CodeValdWork"
	egserver "github.com/aosanya/CodeValdSharedLib/entitygraph/server"
	"github.com/aosanya/CodeValdSharedLib/eventbus"
	sharedregistrar "github.com/aosanya/CodeValdSharedLib/registrar"
	"github.com/aosanya/CodeValdSharedLib/schemaroutes"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// Registrar handles two responsibilities:
//  1. Sending periodic heartbeat registrations to CodeValdCross via the
//     shared-library registrar (Run / Close).
//  2. Implementing [codevaldwork.CrossPublisher] so that TaskManager can
//     fire lifecycle events on successful operations.
type Registrar struct {
	heartbeat sharedregistrar.Registrar
}

// Compile-time assertion that *Registrar implements codevaldwork.CrossPublisher.
var _ codevaldwork.CrossPublisher = (*Registrar)(nil)

// New constructs a Registrar that heartbeats to the CodeValdCross gRPC server
// at crossAddr and can publish work lifecycle events.
//
//   - crossAddr     — host:port of the CodeValdCross gRPC server
//   - advertiseAddr — host:port that Cross dials back on
//   - agencyID      — agency this instance serves (may be empty)
//   - pingInterval  — heartbeat cadence; ≤ 0 means only the initial ping
//   - pingTimeout   — per-RPC timeout for each Register call
func New(
	crossAddr, advertiseAddr, agencyID string,
	pingInterval, pingTimeout time.Duration,
) (*Registrar, error) {
	hb, err := sharedregistrar.New(
		crossAddr,
		advertiseAddr,
		agencyID,
		"codevaldwork",
		codevaldwork.AllTopics(),
		[]string{"cross.task.requested", "cross.agency.created"},
		workRoutes(),
		pingInterval,
		pingTimeout,
	)
	if err != nil {
		return nil, err
	}
	return &Registrar{heartbeat: hb}, nil
}

// Run starts the heartbeat loop, sending an immediate Register ping to
// CodeValdCross then repeating at the configured interval until ctx is
// cancelled. Must be called inside a goroutine.
func (r *Registrar) Run(ctx context.Context) {
	r.heartbeat.Run(ctx)
}

// Close releases the underlying gRPC connection used for heartbeats.
// Call after the context passed to Run has been cancelled.
func (r *Registrar) Close() {
	r.heartbeat.Close()
}

// Publish implements [eventbus.Publisher].
// Best-effort notification — currently logs the event; a future iteration
// will call a CodeValdCross Publish RPC once CodeValdCross exposes one.
// Errors are always nil — the operation has already been persisted and
// must not be rolled back.
func (r *Registrar) Publish(_ context.Context, e eventbus.Event) error {
	log.Printf("registrar[codevaldwork]: publish topic=%q agencyID=%q payload=%T",
		e.Topic, e.AgencyID, e.Payload)
	// TODO(CROSS-XXX): call OrchestratorService.Publish RPC when available.
	return nil
}

// workRoutes returns the HTTP routes that CodeValdWork exposes via Cross.
//
// It combines:
//   - Static routes for the TaskService gRPC methods, grouped by domain
//     (Task lifecycle, Agent + assignment, Project CRUD + membership,
//     graph relationships).
//   - Dynamic entity CRUD routes generated from [codevaldwork.DefaultWorkSchema]
//     via a single [schemaroutes.RoutesFromSchema] call.
func workRoutes() []types.RouteInfo {
	routes := taskRoutes()
	routes = append(routes, agentRoutes()...)
	routes = append(routes, projectRoutes()...)
	routes = append(routes, relationshipRoutes()...)
	routes = append(routes, schemaroutes.RoutesFromSchema(
		codevaldwork.DefaultWorkSchema(),
		"/work/{agencyId}",
		"agencyId",
		egserver.GRPCServicePath,
	)...)
	return routes
}

// agencyBinding is the recurring {agencyId}→agency_id binding shared by every
// route under /work/{agencyId}/...
var agencyBinding = types.PathBinding{URLParam: "agencyId", Field: "agency_id"}

// taskRoutes covers Task CRUD plus the assignee sub-resource that writes the
// `assigned_to` graph edge (WORK-010).
func taskRoutes() []types.RouteInfo {
	return []types.RouteInfo{
		{
			Method:       "POST",
			Pattern:      "/work/{agencyId}/tasks",
			Capability:   "create_task",
			GrpcMethod:   "/codevaldwork.v1.TaskService/CreateTask",
			IsWrite:      true,
			PathBindings: []types.PathBinding{agencyBinding},
		},
		{
			Method:       "GET",
			Pattern:      "/work/{agencyId}/tasks",
			Capability:   "list_tasks",
			GrpcMethod:   "/codevaldwork.v1.TaskService/ListTasks",
			PathBindings: []types.PathBinding{agencyBinding},
		},
		{
			Method:       "POST",
			Pattern:      "/work/{agencyId}/tasks/search",
			Capability:   "search_tasks",
			GrpcMethod:   "/codevaldwork.v1.TaskService/ListTasks",
			PathBindings: []types.PathBinding{agencyBinding},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/tasks/{taskId}",
			Capability: "get_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/GetTask",
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "taskId", Field: "task_id"},
			},
		},
		{
			Method:     "PUT",
			Pattern:    "/work/{agencyId}/tasks/{taskId}",
			Capability: "update_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/UpdateTask",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "taskId", Field: "task_id"},
			},
		},
		{
			Method:     "DELETE",
			Pattern:    "/work/{agencyId}/tasks/{taskId}",
			Capability: "delete_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/DeleteTask",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "taskId", Field: "task_id"},
			},
		},
		{
			Method:     "PUT",
			Pattern:    "/work/{agencyId}/tasks/{taskId}/assignee/{agentId}",
			Capability: "assign_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/AssignTask",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "taskId", Field: "task_id"},
				{URLParam: "agentId", Field: "agent_id"},
			},
		},
		{
			Method:     "DELETE",
			Pattern:    "/work/{agencyId}/tasks/{taskId}/assignee",
			Capability: "unassign_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/UnassignTask",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "taskId", Field: "task_id"},
			},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/tasks/{taskId}/projects",
			Capability: "list_projects_for_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/ListProjectsForTask",
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "taskId", Field: "task_id"},
			},
		},
	}
}

// agentRoutes covers Agent vertex upsert / read / list. The {agentId} path
// segment on UpsertAgent binds into the nested `agent.agent_id` natural key,
// while GetAgent uses the top-level entity-ID lookup.
func agentRoutes() []types.RouteInfo {
	return []types.RouteInfo{
		{
			Method:     "PUT",
			Pattern:    "/work/{agencyId}/agents/{agentId}",
			Capability: "upsert_agent",
			GrpcMethod: "/codevaldwork.v1.TaskService/UpsertAgent",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "agentId", Field: "agent.agent_id"},
			},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/agents/{agentId}",
			Capability: "get_agent",
			GrpcMethod: "/codevaldwork.v1.TaskService/GetAgent",
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "agentId", Field: "agent_id"},
			},
		},
		{
			Method:       "GET",
			Pattern:      "/work/{agencyId}/agents",
			Capability:   "list_agents",
			GrpcMethod:   "/codevaldwork.v1.TaskService/ListAgents",
			PathBindings: []types.PathBinding{agencyBinding},
		},
	}
}

// projectNameBinding is the {projectName}→project_name binding shared by every
// route under /work/{agencyId}/projects/{projectName}/...
var projectNameBinding = types.PathBinding{URLParam: "projectName", Field: "project_name"}

// projectRoutes covers Project CRUD plus the `member_of` membership edges.
// All per-project routes use {projectName} (the URL-safe slug) rather than a
// raw UUID, mirroring the git service's {repoName} convention.
func projectRoutes() []types.RouteInfo {
	return []types.RouteInfo{
		{
			Method:       "POST",
			Pattern:      "/work/{agencyId}/projects",
			Capability:   "create_project",
			GrpcMethod:   "/codevaldwork.v1.TaskService/CreateProject",
			IsWrite:      true,
			PathBindings: []types.PathBinding{agencyBinding},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/projects/{projectName}",
			Capability: "get_project",
			GrpcMethod: "/codevaldwork.v1.TaskService/GetProjectByName",
			PathBindings: []types.PathBinding{
				agencyBinding,
				projectNameBinding,
			},
		},
		{
			Method:     "PUT",
			Pattern:    "/work/{agencyId}/projects/{projectName}",
			Capability: "update_project",
			GrpcMethod: "/codevaldwork.v1.TaskService/UpdateProject",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "projectName", Field: "project.project_name"},
			},
		},
		{
			Method:     "DELETE",
			Pattern:    "/work/{agencyId}/projects/{projectName}",
			Capability: "delete_project",
			GrpcMethod: "/codevaldwork.v1.TaskService/DeleteProject",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				projectNameBinding,
			},
		},
		{
			Method:       "GET",
			Pattern:      "/work/{agencyId}/projects",
			Capability:   "list_projects",
			GrpcMethod:   "/codevaldwork.v1.TaskService/ListProjects",
			PathBindings: []types.PathBinding{agencyBinding},
		},
		{
			Method:     "PUT",
			Pattern:    "/work/{agencyId}/projects/{projectName}/tasks/{taskId}",
			Capability: "add_task_to_project",
			GrpcMethod: "/codevaldwork.v1.TaskService/AddTaskToProject",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				projectNameBinding,
				{URLParam: "taskId", Field: "task_id"},
			},
		},
		{
			Method:     "DELETE",
			Pattern:    "/work/{agencyId}/projects/{projectName}/tasks/{taskId}",
			Capability: "remove_task_from_project",
			GrpcMethod: "/codevaldwork.v1.TaskService/RemoveTaskFromProject",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				projectNameBinding,
				{URLParam: "taskId", Field: "task_id"},
			},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/projects/{projectName}/tasks",
			Capability: "list_tasks_in_project",
			GrpcMethod: "/codevaldwork.v1.TaskService/ListTasksInProject",
			PathBindings: []types.PathBinding{
				agencyBinding,
				projectNameBinding,
			},
		},
		{
			Method:       "POST",
			Pattern:      "/work/{agencyId}/projects/import",
			Capability:   "import_project",
			GrpcMethod:   "/codevaldwork.v1.TaskService/ImportProject",
			IsWrite:      true,
			PathBindings: []types.PathBinding{agencyBinding},
		},
	}
}

// relationshipRoutes covers the generic graph-edge surface. Direction on
// TraverseRelationships is supplied as a `?direction=` query parameter and is
// merged into the request body by Cross's dynamic proxy — no PathBinding
// required.
func relationshipRoutes() []types.RouteInfo {
	return []types.RouteInfo{
		{
			Method:       "POST",
			Pattern:      "/work/{agencyId}/relationships",
			Capability:   "create_relationship",
			GrpcMethod:   "/codevaldwork.v1.TaskService/CreateRelationship",
			IsWrite:      true,
			PathBindings: []types.PathBinding{agencyBinding},
		},
		{
			Method:     "DELETE",
			Pattern:    "/work/{agencyId}/relationships/{label}/from/{fromId}/to/{toId}",
			Capability: "delete_relationship",
			GrpcMethod: "/codevaldwork.v1.TaskService/DeleteRelationship",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "label", Field: "label"},
				{URLParam: "fromId", Field: "from_id"},
				{URLParam: "toId", Field: "to_id"},
			},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/vertices/{vertexId}/relationships/{label}",
			Capability: "traverse_relationships",
			GrpcMethod: "/codevaldwork.v1.TaskService/TraverseRelationships",
			PathBindings: []types.PathBinding{
				agencyBinding,
				{URLParam: "vertexId", Field: "vertex_id"},
				{URLParam: "label", Field: "label"},
			},
		},
	}
}
