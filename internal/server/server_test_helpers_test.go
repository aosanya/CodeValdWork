package server_test

import (
	"context"
	"testing"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
	"github.com/aosanya/CodeValdWork/internal/server"
)

// newTestServer constructs a real Server backed by an in-process
// codevaldwork.TaskManager wired to a SharedLib in-memory entitygraph.
// The unit tests for the Go-domain layer cover the manager itself; here
// we exercise the proto translation + error mapping path.
func newTestServer() pb.TaskServiceServer {
	mgr := newInMemoryManager()
	return server.New(mgr)
}

// newInMemoryManager returns a TaskManager backed by the package's local
// fake DataManager defined alongside the unit tests. The fake is implemented
// in fakedm_test.go in this package — kept separate so it can be reused
// across all *_server_test.go files.
func newInMemoryManager() codevaldwork.TaskManager {
	mgr, err := codevaldwork.NewTaskManager(newFakeDataManager(), nil)
	if err != nil {
		panic(err)
	}
	return mgr
}

// createProtoTask creates a Task via the gRPC façade and returns the
// proto-shaped Task — used to seed Tasks for handler tests.
func createProtoTask(t *testing.T, srv pb.TaskServiceServer, agencyID, title string) *pb.Task {
	t.Helper()
	res, err := srv.CreateTask(context.Background(), &pb.CreateTaskRequest{
		AgencyId: agencyID,
		Task:     &pb.Task{Title: title},
	})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return res.Task
}
