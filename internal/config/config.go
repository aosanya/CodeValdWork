// Package config loads CodeValdWork runtime configuration from environment
// variables. All values have sensible defaults so the service starts in
// standalone mode (no Cross registration, default ArangoDB target) with zero
// environment variables set apart from CODEVALDWORK_PORT.
package config

import (
	"time"

	"github.com/aosanya/CodeValdSharedLib/serverutil"
)

// Config holds all runtime configuration for the CodeValdWork service.
type Config struct {
	// GRPCPort is the port the gRPC server listens on. Required.
	GRPCPort string

	// ArangoEndpoint is the ArangoDB HTTP endpoint (default "http://localhost:8529").
	ArangoEndpoint string

	// ArangoUser is the ArangoDB username (default "root").
	ArangoUser string

	// ArangoPassword is the ArangoDB password.
	ArangoPassword string

	// ArangoDatabase is the ArangoDB database name (default "codevaldwork").
	ArangoDatabase string

	// CrossGRPCAddr is the CodeValdCross gRPC address used for heartbeat
	// registration. Empty string disables registration (standalone mode).
	CrossGRPCAddr string

	// AdvertiseAddr is the address CodeValdCross dials back on.
	// Defaults to ":GRPCPort" when unset.
	AdvertiseAddr string

	// AgencyID is the agency this instance is scoped to, sent in every
	// Register heartbeat. Empty string means this instance serves all agencies.
	AgencyID string

	// PingInterval is the heartbeat cadence sent to CodeValdCross (default 10s).
	// Set to 0 to send only the initial registration ping.
	PingInterval time.Duration

	// PingTimeout is the per-RPC timeout for each Register call (default 5s).
	PingTimeout time.Duration
}

// Load reads runtime configuration from environment variables, falling back to
// sensible defaults for any variable that is unset or empty.
//
// Environment variables:
//
//	CODEVALDWORK_PORT          gRPC listener port (required)
//	WORK_ARANGO_ENDPOINT       ArangoDB endpoint (default "http://localhost:8529")
//	WORK_ARANGO_USER           ArangoDB username (default "root")
//	WORK_ARANGO_PASSWORD       ArangoDB password
//	WORK_ARANGO_DATABASE       ArangoDB database name (default "codevaldwork")
//	CROSS_GRPC_ADDR            CodeValdCross gRPC address (default ""; disables registration)
//	WORK_GRPC_ADVERTISE_ADDR   address Cross dials back on (default ":PORT")
//	CODEVALDWORK_AGENCY_ID     agency scope for this instance (default "")
//	CROSS_PING_INTERVAL        heartbeat cadence Go duration string (default "10s")
//	CROSS_PING_TIMEOUT         per-RPC Register timeout, Go duration string (default "5s")
func Load() Config {
	port := serverutil.MustGetEnv("CODEVALDWORK_PORT")
	return Config{
		GRPCPort:       port,
		ArangoEndpoint: serverutil.EnvOrDefault("WORK_ARANGO_ENDPOINT", "http://localhost:8529"),
		ArangoUser:     serverutil.EnvOrDefault("WORK_ARANGO_USER", "root"),
		ArangoPassword: serverutil.EnvOrDefault("WORK_ARANGO_PASSWORD", ""),
		ArangoDatabase: serverutil.EnvOrDefault("WORK_ARANGO_DATABASE", "codevaldwork"),
		CrossGRPCAddr:  serverutil.EnvOrDefault("CROSS_GRPC_ADDR", ""),
		AdvertiseAddr:  serverutil.EnvOrDefault("WORK_GRPC_ADVERTISE_ADDR", ":"+port),
		AgencyID:       serverutil.EnvOrDefault("CODEVALDWORK_AGENCY_ID", ""),
		PingInterval:   serverutil.ParseDurationString("CROSS_PING_INTERVAL", 10*time.Second),
		PingTimeout:    serverutil.ParseDurationString("CROSS_PING_TIMEOUT", 5*time.Second),
	}
}
