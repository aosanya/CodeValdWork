// Package app holds the shared runtime wiring for CodeValdWork. Both the
// production binary (cmd/server) and the local dev binary (cmd/dev) call
// Run; they differ only in which environment variables they set before
// loading config.
package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	codevaldwork "github.com/aosanya/CodeValdWork"
	pb "github.com/aosanya/CodeValdWork/gen/go/codevaldwork/v1"
	"github.com/aosanya/CodeValdWork/internal/config"
	"github.com/aosanya/CodeValdWork/internal/registrar"
	"github.com/aosanya/CodeValdWork/internal/server"
	workarangodb "github.com/aosanya/CodeValdWork/storage/arangodb"
	aipb "github.com/aosanya/CodeValdAI/gen/go/codevaldai/v1"
	commpb "github.com/aosanya/CodeValdComm/gen/go/codevaldcomm/v1"
	functionspb "github.com/aosanya/CodeValdFunctions/gen/go/codevaldfunctions/v1"
	gitpb "github.com/aosanya/CodeValdGit/gen/go/codevaldgit/v1"
	"github.com/aosanya/CodeValdSharedLib/entitygraph"
	healthpb "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldhealth/v1"
	entitygraphpb "github.com/aosanya/CodeValdSharedLib/gen/go/entitygraph/v1"
	sharedev1 "github.com/aosanya/CodeValdSharedLib/gen/go/codevaldshared/v1"
	"github.com/aosanya/CodeValdSharedLib/health"
	"github.com/aosanya/CodeValdSharedLib/serverutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Run starts all CodeValdWork subsystems (Cross registrar, ArangoDB
// entitygraph backend, gRPC server) and blocks until SIGINT/SIGTERM triggers
// graceful shutdown.
func Run(cfg config.Config) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Cross registrar (optional) ───────────────────────────────────────────
	var pub codevaldwork.CrossPublisher
	if cfg.CrossGRPCAddr != "" {
		reg, err := registrar.New(
			cfg.CrossGRPCAddr,
			cfg.AdvertiseAddr,
			cfg.AgencyID,
			cfg.PingInterval,
			cfg.PingTimeout,
			cfg.SubscribeTopics,
		)
		if err != nil {
			log.Printf("codevaldwork: registrar: failed to create: %v — continuing without registration", err)
		} else {
			defer reg.Close()
			go reg.Run(ctx)
			pub = reg
		}
	} else {
		log.Println("codevaldwork: CROSS_GRPC_ADDR not set — skipping CodeValdCross registration")
	}

	// ── ArangoDB entitygraph backend ─────────────────────────────────────────
	backend, err := workarangodb.NewBackend(workarangodb.Config{
		Endpoint: cfg.ArangoEndpoint,
		Username: cfg.ArangoUser,
		Password: cfg.ArangoPassword,
		Database: cfg.ArangoDatabase,
		Schema:   codevaldwork.DefaultWorkSchema(),
	})
	if err != nil {
		return fmt.Errorf("ArangoDB backend: %w", err)
	}

	// ── Schema seed (idempotent on startup) ──────────────────────────────────
	if cfg.AgencyID != "" {
		seedCtx, seedCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := entitygraph.SeedSchema(seedCtx, backend, cfg.AgencyID, codevaldwork.DefaultWorkSchema()); err != nil {
			log.Printf("codevaldwork: schema seed: %v", err)
		}
		seedCancel()
	} else {
		log.Println("codevaldwork: CODEVALDWORK_AGENCY_ID not set — skipping schema seed")
	}

	// ── Cross-service rollback gRPC clients (all optional) ───────────────────
	rollback := buildRollbackClients(cfg)

	// ── TaskManager ──────────────────────────────────────────────────────────
	mgr, err := codevaldwork.NewTaskManager(backend, pub, rollback)
	if err != nil {
		return fmt.Errorf("NewTaskManager: %w", err)
	}

	// ── gRPC server ───────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		return fmt.Errorf("listen on :%s: %w", cfg.GRPCPort, err)
	}

	grpcServer, _ := serverutil.NewGRPCServer()
	taskServer := server.New(mgr)
	if cfg.AgencyID != "" {
		dispatcher := server.NewTaskEventDispatcher(mgr, cfg.AgencyID, pub)
		taskServer.WithDispatcher(dispatcher)
		sharedev1.RegisterEventReceiverServiceServer(grpcServer, server.NewEventReceiver(backend, cfg.AgencyID, dispatcher))
	}
	pb.RegisterTaskServiceServer(grpcServer, taskServer)
	entitygraphpb.RegisterEntityServiceServer(grpcServer, server.NewEntityServer(backend))
	healthpb.RegisterHealthServiceServer(grpcServer, health.New("codevaldwork"))

	// ── Signal handling ───────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-quit
		log.Println("codevaldwork: shutdown signal received")
		cancel()
	}()

	log.Printf("codevaldwork: gRPC server listening on :%s", cfg.GRPCPort)
	serverutil.RunWithGracefulShutdown(ctx, grpcServer, lis, 30*time.Second)
	return nil
}

// buildRollbackClients creates gRPC-backed compensation functions for each
// cross-service rollback leg. Any leg whose address is empty is left nil —
// the coordinator will log a skip for that service rather than failing.
func buildRollbackClients(cfg config.Config) codevaldwork.RollbackClients {
	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	var rc codevaldwork.RollbackClients

	if cfg.GitGRPCAddr != "" {
		conn, err := grpc.NewClient(cfg.GitGRPCAddr, dialOpts...)
		if err != nil {
			log.Printf("codevaldwork: git rollback client: %v — skipping git compensation", err)
		} else {
			c := gitpb.NewGitServiceClient(conn)
			rc.Git = func(ctx context.Context, runID string) error {
				_, err := c.RollbackByWorkflowRun(ctx, &gitpb.RollbackByWorkflowRunRequest{WorkflowRunId: runID})
				return err
			}
		}
	}

	if cfg.AIGRPCAddr != "" {
		conn, err := grpc.NewClient(cfg.AIGRPCAddr, dialOpts...)
		if err != nil {
			log.Printf("codevaldwork: ai rollback client: %v — skipping ai compensation", err)
		} else {
			c := aipb.NewAIServiceClient(conn)
			rc.AI = func(ctx context.Context, runID, reason string) error {
				_, err := c.RollbackByWorkflowRun(ctx, &aipb.RollbackByWorkflowRunRequest{WorkflowRunId: runID, Reason: reason})
				return err
			}
		}
	}

	if cfg.CommGRPCAddr != "" {
		conn, err := grpc.NewClient(cfg.CommGRPCAddr, dialOpts...)
		if err != nil {
			log.Printf("codevaldwork: comm rollback client: %v — skipping comm compensation", err)
		} else {
			c := commpb.NewCommServiceClient(conn)
			rc.Comm = func(ctx context.Context, agencyID, runID, reason string) error {
				_, err := c.RollbackByWorkflowRun(ctx, &commpb.RollbackByWorkflowRunRequest{AgencyId: agencyID, WorkflowRunId: runID, Reason: reason})
				return err
			}
		}
	}

	if cfg.FunctionsGRPCAddr != "" {
		conn, err := grpc.NewClient(cfg.FunctionsGRPCAddr, dialOpts...)
		if err != nil {
			log.Printf("codevaldwork: functions rollback client: %v — skipping functions compensation", err)
		} else {
			c := functionspb.NewFunctionsServiceClient(conn)
			rc.Functions = func(ctx context.Context, agencyID, runID, reason string) error {
				_, err := c.RollbackByWorkflowRun(ctx, &functionspb.RollbackByWorkflowRunRequest{AgencyId: agencyID, WorkflowRunId: runID, Reason: reason})
				return err
			}
		}
	}

	return rc
}
