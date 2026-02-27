// Package registrar sends periodic availability heartbeats to CodeValdCross.
// On startup it immediately registers, then repeats at the configured interval
// until the context is cancelled. All errors are logged and retried on the
// next tick — a transient CodeValdCross outage never crashes the CodeValdWork
// server.
package registrar

import (
	"context"
	"log"
	"time"

	crossv1 "github.com/aosanya/CodeValdWork/gen/go/codevaldcross/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	serviceName = "codevaldwork"

	// DefaultPingInterval is the fallback heartbeat cadence when
	// CROSS_PING_INTERVAL is not set.
	DefaultPingInterval = 10 * time.Second

	// DefaultPingTimeout is the fallback per-RPC timeout when
	// CROSS_PING_TIMEOUT is not set.
	DefaultPingTimeout = 5 * time.Second
)

// producedTopics are the pub/sub topics that CodeValdWork events emit on
// the CodeValdCross bus.
var producedTopics = []string{
	"work.task.created",
	"work.task.updated",
	"work.task.completed",
}

// consumedTopics are the pub/sub topics that CodeValdWork subscribes to.
var consumedTopics = []string{
	"cross.task.requested",
	"cross.agency.created",
}

// Registrar holds a persistent gRPC connection to CodeValdCross and sends
// periodic Register heartbeats. Create with New; start with Run in a goroutine.
type Registrar struct {
	crossAddr    string
	listenAddr   string
	agencyID     string
	pingInterval time.Duration
	pingTimeout  time.Duration
	conn         *grpc.ClientConn
	client       crossv1.OrchestratorServiceClient
}

// New constructs a Registrar that will heartbeat to the CodeValdCross gRPC
// address at crossAddr. listenAddr is the host:port on which this service
// listens — it is sent in each Register heartbeat so CodeValdCross can dial
// back. agencyID is the agency this instance serves; empty is valid for
// unscoped instances. pingInterval controls the heartbeat cadence;
// pingTimeout caps each Register RPC.
// Returns an error if the address cannot be parsed.
func New(crossAddr, listenAddr, agencyID string, pingInterval, pingTimeout time.Duration) (*Registrar, error) {
	conn, err := grpc.NewClient(crossAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return &Registrar{
		crossAddr:    crossAddr,
		listenAddr:   listenAddr,
		agencyID:     agencyID,
		pingInterval: pingInterval,
		pingTimeout:  pingTimeout,
		conn:         conn,
		client:       crossv1.NewOrchestratorServiceClient(conn),
	}, nil
}

// Close releases the underlying gRPC connection.
func (r *Registrar) Close() {
	if r.conn != nil {
		r.conn.Close()
	}
}

// Run sends an initial Register call immediately, then repeats at r.pingInterval
// until ctx is cancelled. All errors are logged; the loop never panics.
func (r *Registrar) Run(ctx context.Context) {
	r.ping(ctx)

	if r.pingInterval <= 0 {
		return
	}

	ticker := time.NewTicker(r.pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.ping(ctx)
		}
	}
}

// ping sends a single Register RPC to CodeValdCross.
func (r *Registrar) ping(ctx context.Context) {
	rpcCtx, cancel := context.WithTimeout(ctx, r.pingTimeout)
	defer cancel()

	_, err := r.client.Register(rpcCtx, &crossv1.RegisterRequest{
		ServiceName: serviceName,
		Addr:        r.listenAddr,
		AgencyId:    r.agencyID,
		Produces:    producedTopics,
		Consumes:    consumedTopics,
	})
	if err != nil {
		log.Printf("codevaldwork: ping CodeValdCross at %s: %v", r.crossAddr, err)
		return
	}
	log.Printf("codevaldwork: registered with CodeValdCross at %s (self=%s agencyID=%s)",
		r.crossAddr, r.listenAddr, r.agencyID)
}
