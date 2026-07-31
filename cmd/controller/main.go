package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shui1iao/zeno/internal/controller/api"
)

type handlerConfig struct {
	DBPath                            string
	WebDir                            string
	SeedPreview                       bool
	NodeID                            string
	AgentToken                        string
	AdminToken                        string
	TrustedProxies                    string
	AgentBinaryPath                   string
	AgentVersion                      string
	DisableNotifications              bool
	NotificationAuthorityKey          string
	NotificationCredentialKey         []byte
	NotificationAuthorityActiveKeyID  string
	NotificationAuthorityKeys         map[string]string
	NotificationCredentialActiveKeyID string
	NotificationCredentialKeys        map[string][]byte
}

type controllerRuntime struct {
	Handler http.Handler
	Store   *api.SQLiteStore
	Cleanup func(context.Context) error
}

func buildController(config handlerConfig) (*controllerRuntime, error) {
	trustedProxies, err := api.ParseTrustedProxies(config.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("parse ZENO_TRUSTED_PROXIES: %w", err)
	}
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	cleanupHandlers := []func(context.Context) error{func(context.Context) error {
		backgroundCancel()
		return nil
	}}
	options := api.HandlerOptions{
		StaticDir: config.WebDir, AgentBinaryPath: config.AgentBinaryPath, AgentVersion: config.AgentVersion,
		BackgroundContext:    backgroundCtx,
		DisableNotifications: config.DisableNotifications,
		TrustedProxies:       trustedProxies,
	}
	if strings.TrimSpace(config.AdminToken) != "" {
		options.AdminTokenHash = api.HashAdminToken(config.AdminToken)
	}
	var store *api.SQLiteStore
	if config.DBPath != "" {
		opened, err := api.OpenSQLiteStore(config.DBPath)
		if err != nil {
			backgroundCancel()
			return nil, err
		}
		store = opened
		if len(config.NotificationCredentialKeys) > 0 {
			if err := store.ConfigureNotificationCredentialKeyring(context.Background(), config.NotificationCredentialActiveKeyID, config.NotificationCredentialKeys); err != nil {
				_ = store.Close()
				backgroundCancel()
				return nil, err
			}
		} else if len(config.NotificationCredentialKey) > 0 {
			if err := store.ConfigureNotificationCredentialEncryption(context.Background(), config.NotificationCredentialKey); err != nil {
				_ = store.Close()
				backgroundCancel()
				return nil, err
			}
		} else if err := store.RequireNotificationCredentialKeyForExistingCredentials(context.Background()); err != nil {
			_ = store.Close()
			backgroundCancel()
			return nil, err
		}
		var authorized bool
		if len(config.NotificationAuthorityKeys) > 0 {
			authorized, err = store.AuthorizeNotificationAuthorityKeyring(context.Background(), config.NotificationAuthorityActiveKeyID, config.NotificationAuthorityKeys)
		} else {
			authorized, err = store.AuthorizeNotificationAuthority(context.Background(), config.NotificationAuthorityKey)
		}
		if err != nil {
			_ = store.Close()
			backgroundCancel()
			return nil, err
		}
		if !authorized {
			config.DisableNotifications = true
			log.Printf("notification delivery disabled: external authority key is missing or does not match this database")
		}
		options.Store = store
		options.DisableNotifications = config.DisableNotifications
		options.StaleOfflineScanInterval = 5 * time.Second
		options.RenewalNotificationInterval = time.Hour
		options.HistoryRetentionInterval = time.Hour
		options.NotificationDispatchInterval = 5 * time.Second
		options.ExchangeRateRefreshInterval = 24 * time.Hour
		cleanupHandlers = append(cleanupHandlers, func(context.Context) error { return store.Close() })
		if config.SeedPreview {
			if err := store.SeedPreviewData(context.Background(), api.PreviewSeedOptions{NodeID: config.NodeID, DisplayName: "Example Node A", CountryCode: "HK", AgentToken: config.AgentToken}); err != nil {
				_ = store.Close()
				backgroundCancel()
				return nil, err
			}
		}
	}
	handler := api.NewHandler(options)
	if cleanupHandler, ok := handler.(interface{ Cleanup(context.Context) error }); ok {
		cleanupHandlers = append([]func(context.Context) error{cleanupHandler.Cleanup}, cleanupHandlers...)
	}
	cleanup := func(ctx context.Context) error {
		var firstErr error
		for _, cleanupHandler := range cleanupHandlers {
			if err := cleanupHandler(ctx); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	}
	return &controllerRuntime{Handler: handler, Store: store, Cleanup: cleanup}, nil
}

const (
	defaultDBCheckTimeout = 10 * time.Minute
	maximumDBCheckTimeout = 24 * time.Hour
)

func validateDBCheckTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("database check timeout must be greater than zero")
	}
	if timeout > maximumDBCheckTimeout {
		return fmt.Errorf("database check timeout must not exceed %s", maximumDBCheckTimeout)
	}
	return nil
}

func checkSQLiteDatabase(parent context.Context, dbPath string, timeout time.Duration) error {
	if err := validateDBCheckTimeout(timeout); err != nil {
		return err
	}
	store, err := api.OpenSQLiteStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return store.QuickCheck(ctx)
}

func main() {
	addr := flag.String("addr", "127.0.0.1:18980", "controller listen address")
	webDir := flag.String("web-dir", "", "optional built web dashboard directory")
	dbPath := flag.String("db", "", "optional SQLite database path for real controller data")
	seedPreview := flag.Bool("seed-preview", false, "seed the Example Node A preview node and TCP probe targets into SQLite; requires -db")
	collectLocal := flag.Bool("collect-local", false, "run a controller-local TCP probe collector for preview real latency data; requires -db")
	nodeID := flag.String("node-id", "example-node-a", "node id for seeded preview data and controller-local collection")
	agentToken := flag.String("agent-token", "", "agent API bearer token for seeded preview node; prefer -agent-token-file in deployments")
	agentTokenFile := flag.String("agent-token-file", "", "file containing the agent API bearer token for seeded preview node")
	adminToken := flag.String("admin-token", "", "admin API token; prefer -admin-token-file in deployments")
	adminTokenFile := flag.String("admin-token-file", "", "file containing the admin API token")
	agentBinaryPath := flag.String("agent-binary", "", "optional Zeno agent linux/amd64 binary path served for dashboard install commands")
	agentVersion := flag.String("agent-version", "", "optional version string inserted into generated agent install commands")
	notificationAuthorityKeyFile := flag.String("notification-authority-key-file", "", "file containing the external notification authority key")
	notificationAuthorityKeyringFile := flag.String("notification-authority-keyring-file", "", "JSON file containing active and previous notification authority keys")
	notificationCredentialKeyFile := flag.String("notification-credential-key-file", "", "file containing the external notification credential encryption key")
	notificationCredentialKeyringFile := flag.String("notification-credential-keyring-file", "", "JSON file containing active and previous notification credential encryption keys")
	probeInterval := flag.Duration("probe-interval", time.Minute, "controller-local probe collection interval")
	checkDB := flag.Bool("check-db", false, "run SQLite schema setup and PRAGMA quick_check, then exit")
	checkDBTimeout := flag.Duration("check-db-timeout", defaultDBCheckTimeout, "maximum duration for -check-db (must be greater than zero and no more than 24h)")
	resetAdminPasswordFile := flag.String("reset-admin-password-file", "", "offline recovery: reset admin account using password from file, then exit")
	flag.Parse()

	if *checkDB {
		if *dbPath == "" {
			log.Fatal("-check-db requires -db")
		}
		if err := checkSQLiteDatabase(context.Background(), *dbPath, *checkDBTimeout); err != nil {
			log.Fatal(err)
		}
		return
	}
	if strings.TrimSpace(*resetAdminPasswordFile) != "" {
		if *dbPath == "" {
			log.Fatal("-reset-admin-password-file requires -db")
		}
		password, err := readSecret("", *resetAdminPasswordFile)
		if err != nil {
			log.Fatal(err)
		}
		store, err := api.OpenSQLiteStore(*dbPath)
		if err != nil {
			log.Fatal(err)
		}
		defer store.Close()
		if err := store.ResetAdminAccount(context.Background(), password); err != nil {
			log.Fatal(err)
		}
		log.Print("admin account reset; all admin sessions revoked")
		return
	}

	token, err := readAgentToken(*agentToken, *agentTokenFile)
	if err != nil {
		log.Fatal(err)
	}
	adminSecret, err := readSecret(*adminToken, *adminTokenFile)
	if err != nil {
		log.Fatal(err)
	}
	authorityKeyringFile := strings.TrimSpace(*notificationAuthorityKeyringFile)
	if authorityKeyringFile == "" {
		authorityKeyringFile = strings.TrimSpace(os.Getenv("ZENO_NOTIFICATION_AUTHORITY_KEYRING_FILE"))
	}
	var notificationAuthorityKey string
	var notificationAuthorityActiveKeyID string
	var notificationAuthorityKeys map[string]string
	if authorityKeyringFile != "" {
		notificationAuthorityActiveKeyID, notificationAuthorityKeys, err = readNotificationAuthorityKeyringFile(authorityKeyringFile)
	} else {
		notificationAuthorityKey, err = readNotificationAuthorityKeyFile(*notificationAuthorityKeyFile)
	}
	if err != nil {
		log.Fatal(err)
	}
	credentialKeyringFile := strings.TrimSpace(*notificationCredentialKeyringFile)
	if credentialKeyringFile == "" {
		credentialKeyringFile = strings.TrimSpace(os.Getenv("ZENO_NOTIFICATION_CREDENTIAL_KEYRING_FILE"))
	}
	var notificationCredentialKey []byte
	var notificationCredentialActiveKeyID string
	var notificationCredentialKeys map[string][]byte
	if credentialKeyringFile != "" {
		notificationCredentialActiveKeyID, notificationCredentialKeys, err = readNotificationCredentialKeyringFile(credentialKeyringFile)
	} else {
		credentialKeyFile := strings.TrimSpace(*notificationCredentialKeyFile)
		if credentialKeyFile == "" {
			credentialKeyFile = strings.TrimSpace(os.Getenv("ZENO_NOTIFICATION_CREDENTIAL_KEY_FILE"))
		}
		notificationCredentialKey, err = readNotificationCredentialKeyFile(credentialKeyFile)
	}
	if err != nil {
		log.Fatal(err)
	}

	disableNotifications := strings.EqualFold(strings.TrimSpace(os.Getenv("ZENO_NOTIFICATIONS_DISABLED")), "true") || strings.TrimSpace(os.Getenv("ZENO_NOTIFICATIONS_DISABLED")) == "1"
	runtime, err := buildController(handlerConfig{
		DBPath: *dbPath, WebDir: *webDir, SeedPreview: *seedPreview, NodeID: *nodeID,
		AgentToken: token, AdminToken: adminSecret, TrustedProxies: os.Getenv("ZENO_TRUSTED_PROXIES"),
		AgentBinaryPath: *agentBinaryPath, AgentVersion: *agentVersion, DisableNotifications: disableNotifications,
		NotificationAuthorityKey: notificationAuthorityKey, NotificationCredentialKey: notificationCredentialKey,
		NotificationAuthorityActiveKeyID: notificationAuthorityActiveKeyID, NotificationAuthorityKeys: notificationAuthorityKeys,
		NotificationCredentialActiveKeyID: notificationCredentialActiveKeyID, NotificationCredentialKeys: notificationCredentialKeys,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := runtime.Cleanup(ctx); err != nil {
			log.Printf("controller cleanup timed out or failed: %v", err)
		}
	}()

	if *collectLocal {
		if runtime.Store == nil {
			log.Fatal("-collect-local requires -db")
		}
		collector := api.NewLocalProbeCollector(runtime.Store, api.LocalProbeCollectorOptions{NodeID: *nodeID})
		var collectorWG sync.WaitGroup
		collectorCtx, stopCollector := context.WithCancel(context.Background())
		defer func() {
			stopCollector()
			done := make(chan struct{})
			go func() {
				collectorWG.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				log.Printf("controller-local probe collector cleanup timed out")
			}
		}()
		collectorWG.Add(1)
		go func() {
			defer collectorWG.Done()
			ticker := time.NewTicker(*probeInterval)
			defer ticker.Stop()
			for {
				if err := collector.CollectOnce(collectorCtx); err != nil {
					log.Printf("local probe collection failed: %v", err)
				}
				select {
				case <-collectorCtx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
		log.Printf("controller-local probe collector enabled for node %s every %s", *nodeID, probeInterval.String())
	}
	log.Printf("zeno controller listening on %s", *addr)
	if *webDir != "" {
		log.Printf("serving dashboard from %s", *webDir)
	}
	if *dbPath != "" {
		log.Printf("using SQLite data store %s", *dbPath)
	}
	if *agentBinaryPath != "" {
		log.Printf("serving agent binary from %s", *agentBinaryPath)
	}
	if *seedPreview {
		log.Printf("seeded preview data for node %s", *nodeID)
	}
	server := &http.Server{
		Addr:              *addr,
		Handler:           runtime.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	shutdownCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	case <-shutdownCtx.Done():
		log.Printf("shutdown signal received; draining HTTP requests")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("graceful shutdown timed out: %v; closing server", err)
			_ = server.Close()
		}
		if err := <-serverErr; err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}
}
