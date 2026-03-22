// Package registrar provides the CodeValdWork service registrar.
// It wraps the shared-library heartbeat registrar so that cmd entry-points
// stay focused on startup wiring and contain no route declarations.
//
// Construct via [New]; start heartbeats by calling Run in a goroutine; stop
// by cancelling the context then calling Close.
package registrar

import (
	"context"
	"time"

	sharedregistrar "github.com/aosanya/CodeValdSharedLib/registrar"
	"github.com/aosanya/CodeValdSharedLib/types"
)

// Registrar sends periodic heartbeat registrations to CodeValdCross via the
// shared-library registrar.
type Registrar struct {
	heartbeat sharedregistrar.Registrar
}

// New constructs a Registrar that heartbeats to the CodeValdCross gRPC server
// at crossAddr.
//
//   - crossAddr    — host:port of the CodeValdCross gRPC server
//   - advertiseAddr — host:port that Cross dials back on
//   - agencyID     — agency this instance serves (may be empty)
//   - pingInterval — heartbeat cadence; ≤ 0 means only the initial ping
//   - pingTimeout  — per-RPC timeout for each Register call
func New(
	crossAddr, advertiseAddr, agencyID string,
	pingInterval, pingTimeout time.Duration,
) (*Registrar, error) {
	hb, err := sharedregistrar.New(
		crossAddr,
		advertiseAddr,
		agencyID,
		"codevaldwork",
		[]string{"work.task.created", "work.task.updated", "work.task.completed"},
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

// workRoutes returns the HTTP routes that CodeValdWork exposes via Cross.
// All routes are prefixed with /work/{agencyId} to disambiguate work endpoints
// from other services.
func workRoutes() []types.RouteInfo {
	return []types.RouteInfo{
		{
			Method:     "POST",
			Pattern:    "/work/{agencyId}/tasks",
			Capability: "create_task",
			GrpcMethod: "/codevaldwork.v1.TaskService/CreateTask",
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
	}
}
