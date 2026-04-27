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
		[]string{
			"work.task.created",
			"work.task.updated",
			"work.task.completed",
		},
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

// Publish implements [codevaldwork.CrossPublisher].
// It fires a best-effort notification for topic and agencyID.
// Currently logs the event; a future iteration will call a Cross Publish RPC
// once CodeValdCross exposes one. Errors are always nil — the operation has
// already been persisted and must not be rolled back.
func (r *Registrar) Publish(ctx context.Context, topic string, agencyID string) error {
	log.Printf("registrar[codevaldwork]: publish topic=%q agencyID=%q", topic, agencyID)
	// TODO(CROSS-007): call OrchestratorService.Publish RPC when available.
	return nil
}

// workRoutes returns the HTTP routes that CodeValdWork exposes via Cross.
//
// It combines:
//   - Static routes for the TaskService gRPC methods (CRUD over tasks).
//   - Dynamic entity CRUD routes generated from [codevaldwork.DefaultWorkSchema]
//     via a single [schemaroutes.RoutesFromSchema] call.
func workRoutes() []types.RouteInfo {
	static := []types.RouteInfo{
		{
			Method:     "POST",
			Pattern:    "/work/{agencyId}/tasks",
			Capability: "create_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/CreateTask",
			IsWrite:    true,
			PathBindings: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
			},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/tasks",
			Capability: "list_tasks",
			GrpcMethod: "/codevaldwork.v1.TaskService/ListTasks",
			PathBindings: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
			},
		},
		{
			Method:     "GET",
			Pattern:    "/work/{agencyId}/tasks/{taskId}",
			Capability: "get_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/GetTask",
			PathBindings: []types.PathBinding{
				{URLParam: "agencyId", Field: "agency_id"},
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
				{URLParam: "agencyId", Field: "agency_id"},
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
				{URLParam: "agencyId", Field: "agency_id"},
				{URLParam: "taskId", Field: "task_id"},
			},
		},
	}

	dynamic := schemaroutes.RoutesFromSchema(
		codevaldwork.DefaultWorkSchema(),
		"/work/{agencyId}",
		"agencyId",
		egserver.GRPCServicePath,
	)

	return append(static, dynamic...)
}
