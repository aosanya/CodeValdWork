// Command server starts the CodeValdWork gRPC microservice.
//
// Configuration is via environment variables:
//
//	CODEVALDWORK_PORT          gRPC listener port (required, set in .env)
//	CROSS_GRPC_ADDR            CodeValdCross gRPC address for service registration
//	                            heartbeats (optional; omit to disable registration)
//	CODEVALDWORK_AGENCY_ID     agency ID sent in every Register heartbeat
//	WORK_GRPC_ADVERTISE_ADDR   address CodeValdCross dials back (default ":PORT")
//	CROSS_PING_INTERVAL        heartbeat cadence sent to CodeValdCross (default 10s)
//	CROSS_PING_TIMEOUT         per-RPC timeout for each Register call (default 5s)
//
// ArangoDB backend:
//
//	WORK_ARANGO_ENDPOINT   ArangoDB endpoint URL (default http://localhost:8529)
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

	"github.com/aosanya/CodeValdSharedLib/serverutil"
	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
	"github.com/aosanya/CodeValdWork/internal/grpcserver"
	"github.com/aosanya/CodeValdWork/internal/registrar"
	"github.com/aosanya/CodeValdWork/storage/arangodb"
)

func main() {
	port := os.Getenv("CODEVALDWORK_PORT")
	if port == "" {
		log.Fatal("CODEVALDWORK_PORT must be set")
	}

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

	grpcServer, _ := serverutil.NewGRPCServer()
	pb.RegisterTaskServiceServer(grpcServer, grpcserver.New(mgr))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	crossAddr := serverutil.EnvOrDefault("CROSS_GRPC_ADDR", "")
	if crossAddr != "" {
		agencyID := serverutil.EnvOrDefault("CODEVALDWORK_AGENCY_ID", "")
		advertiseAddr := serverutil.EnvOrDefault("WORK_GRPC_ADVERTISE_ADDR", ":"+port)
		pingInterval := serverutil.ParseDurationString("CROSS_PING_INTERVAL", 10*time.Second)
		pingTimeout := serverutil.ParseDurationString("CROSS_PING_TIMEOUT", 5*time.Second)
		reg, err := registrar.New(crossAddr, advertiseAddr, agencyID, pingInterval, pingTimeout)
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
		<-quit
		log.Println("codevaldwork: shutdown signal received")
		cancel()
	}()

	log.Printf("CodeValdWork gRPC server listening on :%s", port)
	serverutil.RunWithGracefulShutdown(ctx, grpcServer, lis, 30*time.Second)
}

// initBackend constructs the ArangoDB storage backend from environment variables.
func initBackend() (codevaldwork.Backend, error) {
	return arangodb.NewArangoBackend(arangodb.Config{
		Endpoint: serverutil.EnvOrDefault("WORK_ARANGO_ENDPOINT", "http://localhost:8529"),
		Username: serverutil.EnvOrDefault("WORK_ARANGO_USER", "root"),
		Password: os.Getenv("WORK_ARANGO_PASSWORD"),
		Database: serverutil.EnvOrDefault("WORK_ARANGO_DATABASE", "codevaldwork"),
	})
}
