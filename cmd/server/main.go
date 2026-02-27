// Command server starts the CodeValdWork gRPC microservice.
//
// Configuration is via environment variables:
//
//	CODEVALDWORK_PORT          gRPC listener port (default 50054)
//	CODEVALDWORK_BACKEND       storage backend: "arangodb" (default)
//	CROSS_GRPC_ADDR            CodeValdCross gRPC address for service registration
//	                            heartbeats (optional; omit to disable registration)
//	CROSS_PING_INTERVAL        heartbeat cadence sent to CodeValdCross (default 10s)
//	CROSS_PING_TIMEOUT         per-RPC timeout for each Register call (default 5s)
//
// ArangoDB backend:
//
//	WORK_ARANGO_ENDPOINT   ArangoDB endpoint URL (required)
//	WORK_ARANGO_USER       ArangoDB username (default root)
//	WORK_ARANGO_PASSWORD   ArangoDB password
//	WORK_ARANGO_DATABASE   ArangoDB database name (default codevaldwork)
package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
	"github.com/aosanya/CodeValdWork/internal/grpcserver"
	"github.com/aosanya/CodeValdWork/internal/registrar"
	"github.com/aosanya/CodeValdWork/storage/arangodb"
)

func main() {
	port := envOrDefault("CODEVALDWORK_PORT", "50054")
	backendName := "arangodb"

	backend, err := initBackend()
	if err != nil {
		log.Fatalf("failed to initialise backend: %v", err)
	}

	mgr, err := codevaldwork.NewTaskManager(backend)
	if err != nil {
		log.Fatalf("failed to create TaskManager: %v", err)
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on :%s: %v", port, err)
	}

	grpcServer := grpc.NewServer()

	// Register TaskService.
	pb.RegisterTaskServiceServer(grpcServer, grpcserver.New(mgr))

	// Register gRPC health service.
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)

	// Enable server reflection for development tooling (e.g. grpcurl).
	reflection.Register(grpcServer)

	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	// Start CodeValdCross registration in background.
	crossAddr := envOrDefault("CROSS_GRPC_ADDR", "")
	listenAddr := envOrDefault("WORK_GRPC_ADVERTISE_ADDR", ":"+port)
	agencyID := envOrDefault("CODEVALDWORK_AGENCY_ID", "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if crossAddr != "" {
		pingInterval := parseDuration(envOrDefault("CROSS_PING_INTERVAL", "10s"))
		pingTimeout := parseDuration(envOrDefault("CROSS_PING_TIMEOUT", "5s"))

		reg, err := registrar.New(crossAddr, listenAddr, agencyID, pingInterval, pingTimeout)
		if err != nil {
			log.Printf("registrar: failed to create: %v — continuing without registration", err)
		} else {
			defer reg.Close()
			go reg.Run(ctx)
		}
	} else {
		log.Println("codevaldwork: CROSS_GRPC_ADDR not set — skipping CodeValdCross registration")
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		log.Printf("CodeValdWork gRPC server listening on :%s (backend: %s)", port, backendName)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	<-quit
	cancel() // stop registrar goroutine before draining gRPC
	log.Println("shutdown signal received — draining in-flight RPCs (up to 30 s)")

	done := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		log.Println("server stopped cleanly")
	case <-time.After(30 * time.Second):
		log.Println("drain timeout exceeded — forcing stop")
		grpcServer.Stop()
	}
}

func initBackend() (codevaldwork.Backend, error) {
	endpoint := envOrDefault("WORK_ARANGO_ENDPOINT", "http://localhost:8529")
	user := envOrDefault("WORK_ARANGO_USER", "root")
	pass := envOrDefault("WORK_ARANGO_PASSWORD", "")
	dbName := envOrDefault("WORK_ARANGO_DATABASE", "codevaldwork")

	return arangodb.NewArangoBackend(arangodb.Config{
		Endpoint: endpoint,
		Username: user,
		Password: pass,
		Database: dbName,
	})
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 10 * time.Second
	}
	return d
}
