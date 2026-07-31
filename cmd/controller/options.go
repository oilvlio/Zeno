package main

import (
	"flag"
	"time"
)

// controllerOptions holds every command-line flag.
//
// main previously declared all 21 flags inline, which made the function long
// enough that its one-shot subcommands and its serving path were hard to tell
// apart. Grouping them keeps the flag surface in one place and lets main read
// as a sequence of phases.
type controllerOptions struct {
	addr        string
	webDir      string
	dbPath      string
	seedPreview bool

	collectLocal  bool
	nodeID        string
	probeInterval time.Duration

	agentToken     string
	agentTokenFile string
	adminToken     string
	adminTokenFile string

	agentBinaryPath string
	agentVersion    string

	notificationAuthorityKeyFile      string
	notificationAuthorityKeyringFile  string
	notificationCredentialKeyFile     string
	notificationCredentialKeyringFile string

	checkDB        bool
	checkDBTimeout time.Duration

	resetAdminPasswordFile string
}

// parseControllerOptions registers and parses the controller's flags.
func parseControllerOptions() *controllerOptions {
	options := &controllerOptions{}

	flag.StringVar(&options.addr, "addr", "127.0.0.1:18980", "controller listen address")
	flag.StringVar(&options.webDir, "web-dir", "", "optional built web dashboard directory")
	flag.StringVar(&options.dbPath, "db", "", "optional SQLite database path for real controller data")
	flag.BoolVar(&options.seedPreview, "seed-preview", false, "seed the Example Node A preview node and TCP probe targets into SQLite; requires -db")

	flag.BoolVar(&options.collectLocal, "collect-local", false, "run a controller-local TCP probe collector for preview real latency data; requires -db")
	flag.StringVar(&options.nodeID, "node-id", "example-node-a", "node id for seeded preview data and controller-local collection")
	flag.DurationVar(&options.probeInterval, "probe-interval", time.Minute, "controller-local probe collection interval")

	flag.StringVar(&options.agentToken, "agent-token", "", "agent API bearer token for seeded preview node; prefer -agent-token-file in deployments")
	flag.StringVar(&options.agentTokenFile, "agent-token-file", "", "file containing the agent API bearer token for seeded preview node")
	flag.StringVar(&options.adminToken, "admin-token", "", "admin API token; prefer -admin-token-file in deployments")
	flag.StringVar(&options.adminTokenFile, "admin-token-file", "", "file containing the admin API token")

	flag.StringVar(&options.agentBinaryPath, "agent-binary", "", "optional Zeno agent linux/amd64 binary path served for dashboard install commands")
	flag.StringVar(&options.agentVersion, "agent-version", "", "optional version string inserted into generated agent install commands")

	flag.StringVar(&options.notificationAuthorityKeyFile, "notification-authority-key-file", "", "file containing the external notification authority key")
	flag.StringVar(&options.notificationAuthorityKeyringFile, "notification-authority-keyring-file", "", "JSON file containing active and previous notification authority keys")
	flag.StringVar(&options.notificationCredentialKeyFile, "notification-credential-key-file", "", "file containing the external notification credential encryption key")
	flag.StringVar(&options.notificationCredentialKeyringFile, "notification-credential-keyring-file", "", "JSON file containing active and previous notification credential encryption keys")

	flag.BoolVar(&options.checkDB, "check-db", false, "run SQLite schema setup and PRAGMA quick_check, then exit")
	flag.DurationVar(&options.checkDBTimeout, "check-db-timeout", defaultDBCheckTimeout, "maximum duration for -check-db (must be greater than zero and no more than 24h)")

	flag.StringVar(&options.resetAdminPasswordFile, "reset-admin-password-file", "", "offline recovery: reset admin account using password from file, then exit")

	flag.Parse()
	return options
}
